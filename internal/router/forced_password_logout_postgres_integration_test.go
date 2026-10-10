package router

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"orbitterm-server/internal/config"
	"orbitterm-server/internal/controller"
	"orbitterm-server/internal/model"
	"orbitterm-server/internal/repository"
	"orbitterm-server/internal/service"
	"orbitterm-server/internal/utils"
)

func TestPostgresForcedPasswordLogoutRevokesOnlyRequestedFamily(t *testing.T) {
	dsn := os.Getenv("ORBITTERM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ORBITTERM_TEST_DATABASE_URL is not configured")
	}
	root, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("forced_logout_%d", time.Now().UnixNano())
	if err := root.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	defer root.Exec("DROP SCHEMA " + schema + " CASCADE")
	db, err := gorm.Open(postgres.Open(dsn+" search_path="+schema), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := config.MigrateDatabase(db); err != nil {
		t.Fatal(err)
	}
	user := model.User{Username: "reset@example.com", PasswordHash: "test-hash", Role: model.UserRoleUser, Status: model.UserStatusNormal, MustChangePassword: true}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	repo := repository.NewUserRepository(db)
	jwt := utils.NewJWTManager("strong-postgres-test-secret-for-forced-logout", "test", 15, 30, 24)
	jwt.SetRefreshSessionStore(repository.NewRefreshSessionRepository(db), false)
	first, err := jwt.GenerateTokenPair(user.ID, user.Username, 0)
	if err != nil {
		t.Fatal(err)
	}
	second, err := jwt.GenerateTokenPair(user.ID, user.Username, 0)
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
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	Register(engine, controller.NewAuthController(service.NewAuthService(repo, jwt, nil), nil, nil), nil, nil, nil, jwt, repo, false)
	request := func(method, path, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		res := httptest.NewRecorder()
		engine.ServeHTTP(res, req)
		return res
	}
	if blocked := request(http.MethodGet, "/api/v1/config/pull", first.AccessToken); blocked.Code != http.StatusForbidden || !strings.Contains(blocked.Body.String(), "PASSWORD_CHANGE_REQUIRED") {
		t.Fatalf("config gate: status=%d body=%s", blocked.Code, blocked.Body.String())
	}
	if logout := request(http.MethodPost, "/api/v1/auth/logout", first.AccessToken); logout.Code != http.StatusOK {
		t.Fatalf("current-device logout: status=%d body=%s", logout.Code, logout.Body.String())
	}
	var sessions []model.RefreshSession
	if err := db.Where("family_id IN ?", []string{firstClaims.SessionID, secondClaims.SessionID}).Find(&sessions).Error; err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected two session families, found %d", len(sessions))
	}
	for _, session := range sessions {
		if session.FamilyID == firstClaims.SessionID && session.RevokedAt == nil {
			t.Fatal("current-device logout left its family active")
		}
		if session.FamilyID == secondClaims.SessionID && session.RevokedAt != nil {
			t.Fatal("current-device logout revoked another family")
		}
	}
	if other := request(http.MethodGet, "/api/v1/config/pull", second.AccessToken); other.Code != http.StatusForbidden {
		t.Fatalf("second device should remain active but gated: status=%d", other.Code)
	}
	if all := request(http.MethodPost, "/api/v1/auth/logout-all", second.AccessToken); all.Code != http.StatusOK {
		t.Fatalf("all-device logout: status=%d body=%s", all.Code, all.Body.String())
	}
	if invalid := request(http.MethodGet, "/api/v1/config/pull", second.AccessToken); invalid.Code != http.StatusUnauthorized {
		t.Fatalf("all-device logout left second token valid: status=%d", invalid.Code)
	}
}
