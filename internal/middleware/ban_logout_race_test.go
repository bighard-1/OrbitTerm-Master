package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"orbitterm-server/internal/model"
	"orbitterm-server/internal/repository"
	"orbitterm-server/internal/utils"
)

type expiredBanLogoutRaceRepo struct {
	repository.UserRepository
	stored model.User
	reads  int
}

func (r *expiredBanLogoutRaceRepo) FindByID(uint) (*model.User, error) {
	r.reads++
	copy := r.stored
	if r.reads == 1 {
		r.stored.TokenVersion++
	}
	return &copy, nil
}
func (r *expiredBanLogoutRaceRepo) Save(user *model.User) error { r.stored = *user; return nil }
func (r *expiredBanLogoutRaceRepo) ClearExpiredBanIfDue(uint, time.Time) (bool, error) {
	r.stored.IsBanned = false
	r.stored.BanUntil = nil
	return true, nil
}

func TestExpiredBanCheckCannotUndoConcurrentLogout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	until := time.Now().Add(-time.Minute)
	repo := &expiredBanLogoutRaceRepo{stored: model.User{ID: 1, Username: "race@example.com", TokenVersion: 5, IsBanned: true, BanUntil: &until}}
	jwt := utils.NewJWTManager("long-test-secret-for-ban-race-0123456789", "test", 15, 30, 24)
	pair, err := jwt.GenerateTokenPair(1, repo.stored.Username, 5)
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.GET("/protected", JWTAuthMiddleware(jwt, repo), func(c *gin.Context) { c.Status(http.StatusOK) })
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized || repo.stored.TokenVersion != 6 {
		t.Fatalf("logout was undone or stale token accepted: status=%d version=%d", res.Code, repo.stored.TokenVersion)
	}
}
