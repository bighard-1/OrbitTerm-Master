package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"orbitterm-server/internal/model"
	"orbitterm-server/internal/repository"
	"orbitterm-server/internal/utils"
)

type passwordGateRepo struct {
	repository.UserRepository
	user *model.User
}

func (r passwordGateRepo) FindByID(id uint) (*model.User, error) { return r.user, nil }

func TestMustChangePasswordAllowsOnlyPasswordRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	user := &model.User{ID: 1, Username: "a@example.com", MustChangePassword: true}
	jwt := utils.NewJWTManager("strong-test-secret-for-password-gate", "test", 15, 30, 24)
	pair, err := jwt.GenerateTokenPair(1, user.Username, 0)
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.POST("/api/v1/auth/password", JWTAuthMiddleware(jwt, passwordGateRepo{user: user}), func(c *gin.Context) { c.Status(http.StatusOK) })
	r.POST("/api/v1/config/upload", JWTAuthMiddleware(jwt, passwordGateRepo{user: user}), func(c *gin.Context) { c.Status(http.StatusOK) })
	for path, want := range map[string]int{"/api/v1/auth/password": 200, "/api/v1/config/upload": 403} {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
		res := httptest.NewRecorder()
		r.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s: got %d, want %d", path, res.Code, want)
		}
		if want == 403 && !strings.Contains(res.Body.String(), "PASSWORD_CHANGE_REQUIRED") {
			t.Fatalf("missing stable error code: %s", res.Body.String())
		}
	}
}
