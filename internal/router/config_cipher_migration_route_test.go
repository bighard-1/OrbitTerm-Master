package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"orbitterm-server/internal/repository"
	"orbitterm-server/internal/utils"

	"github.com/gin-gonic/gin"
)

func TestConfigCipherV2MigrationRouteIsOptInAndProtected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	jwtManager := utils.NewJWTManager("router-test-secret", "router-test", 15, 30, 24)
	var userRepo repository.UserRepository

	disabled := gin.New()
	Register(disabled, nil, nil, nil, nil, jwtManager, userRepo, false)
	disabledRecorder := httptest.NewRecorder()
	disabled.ServeHTTP(disabledRecorder, httptest.NewRequest(http.MethodPost, "/api/v1/config/crypto/migrate-v2", nil))
	if disabledRecorder.Code != http.StatusNotFound {
		t.Fatalf("disabled migration route status = %d, want %d", disabledRecorder.Code, http.StatusNotFound)
	}

	enabled := gin.New()
	Register(enabled, nil, nil, nil, nil, jwtManager, userRepo, true)
	enabledRecorder := httptest.NewRecorder()
	enabled.ServeHTTP(enabledRecorder, httptest.NewRequest(http.MethodPost, "/api/v1/config/crypto/migrate-v2", nil))
	if enabledRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("enabled migration route status = %d, want %d", enabledRecorder.Code, http.StatusUnauthorized)
	}
}
