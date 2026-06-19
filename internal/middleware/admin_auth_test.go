package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"orbitterm-server/internal/model"

	"github.com/gin-gonic/gin"
)

func TestRequireAdminRoleRejectsNormalUser(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/admin", func(ctx *gin.Context) {
		ctx.Set(ContextUserRoleKey, model.UserRoleUser)
	}, RequireAdminRole(), func(ctx *gin.Context) {
		ctx.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.Code)
	}
}

func TestRequireAdminRoleAllowsSupport(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/admin", func(ctx *gin.Context) {
		ctx.Set(ContextUserRoleKey, model.UserRoleSupport)
	}, RequireAdminRole(), func(ctx *gin.Context) {
		ctx.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
}
