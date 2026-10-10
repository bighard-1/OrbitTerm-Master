package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"orbitterm-server/internal/controller"
	"orbitterm-server/internal/model"
	"orbitterm-server/internal/repository"
	"orbitterm-server/internal/service"
	"orbitterm-server/internal/utils"
)

type forcedPasswordUserRepo struct {
	repository.UserRepository
	user *model.User
}

func (r *forcedPasswordUserRepo) FindByID(uint) (*model.User, error) {
	copy := *r.user
	return &copy, nil
}

func (r *forcedPasswordUserRepo) IncrementTokenVersionByUser(uint) error {
	r.user.TokenVersion++
	return nil
}

type forcedPasswordSessions struct{ active map[string]bool }

func (s *forcedPasswordSessions) Create(_ uint, familyID, _ string, _ int64, _ time.Time) error {
	s.active[familyID] = true
	return nil
}

func (s *forcedPasswordSessions) Rotate(uint, string, string, string, int64, time.Time) (bool, error) {
	return false, nil
}

func (s *forcedPasswordSessions) Active(_ uint, familyID string, _ int64) (bool, error) {
	return s.active[familyID], nil
}

func (s *forcedPasswordSessions) RevokeFamily(_ uint, familyID string) error {
	s.active[familyID] = false
	return nil
}

func TestForcedPasswordChangeStillAllowsCurrentAndAllDeviceLogout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	user := &model.User{ID: 1, Username: "reset@example.com", Role: model.UserRoleUser, MustChangePassword: true}
	repo := &forcedPasswordUserRepo{user: user}
	sessions := &forcedPasswordSessions{active: make(map[string]bool)}
	jwt := utils.NewJWTManager("strong-router-test-secret-for-forced-logout", "test", 15, 30, 24)
	jwt.SetRefreshSessionStore(sessions, false)
	first, err := jwt.GenerateTokenPair(user.ID, user.Username, user.TokenVersion)
	if err != nil {
		t.Fatal(err)
	}
	second, err := jwt.GenerateTokenPair(user.ID, user.Username, user.TokenVersion)
	if err != nil {
		t.Fatal(err)
	}
	firstClaims, err := jwt.ParseAndVerifyAccessToken(first.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	secondClaims, err := jwt.ParseAndVerifyAccessToken(second.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if firstClaims.SessionID == secondClaims.SessionID {
		t.Fatal("test devices unexpectedly share a session family")
	}

	engine := gin.New()
	Register(engine, controller.NewAuthController(service.NewAuthService(repo, jwt, nil), nil, nil), nil, nil, nil, jwt, repo, false)
	request := func(method, path, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		res := httptest.NewRecorder()
		engine.ServeHTTP(res, req)
		return res
	}

	blocked := request(http.MethodGet, "/api/v1/config/pull", first.AccessToken)
	if blocked.Code != http.StatusForbidden || !strings.Contains(blocked.Body.String(), "PASSWORD_CHANGE_REQUIRED") {
		t.Fatalf("config gate: status=%d body=%s", blocked.Code, blocked.Body.String())
	}
	current := request(http.MethodPost, "/api/v1/auth/logout", first.AccessToken)
	if current.Code != http.StatusOK {
		t.Fatalf("current-device logout: status=%d body=%s", current.Code, current.Body.String())
	}
	if sessions.active[firstClaims.SessionID] || !sessions.active[secondClaims.SessionID] {
		t.Fatalf("current-device logout touched wrong family: %+v", sessions.active)
	}
	if response := request(http.MethodGet, "/api/v1/config/pull", first.AccessToken); response.Code != http.StatusUnauthorized {
		t.Fatalf("revoked device was accepted: status=%d", response.Code)
	}
	if response := request(http.MethodGet, "/api/v1/config/pull", second.AccessToken); response.Code != http.StatusForbidden {
		t.Fatalf("other device should remain active but gated: status=%d", response.Code)
	}

	all := request(http.MethodPost, "/api/v1/auth/logout-all", second.AccessToken)
	if all.Code != http.StatusOK || user.TokenVersion != 1 {
		t.Fatalf("all-device logout: status=%d version=%d body=%s", all.Code, user.TokenVersion, all.Body.String())
	}
	if response := request(http.MethodGet, "/api/v1/config/pull", second.AccessToken); response.Code != http.StatusUnauthorized {
		t.Fatalf("all-device logout left second token valid: status=%d", response.Code)
	}
}
