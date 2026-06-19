package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestIPRateLimitRejectsOverflow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/admin", IPRateLimit(1, time.Minute), func(ctx *gin.Context) {
		ctx.Status(http.StatusOK)
	})

	for i, want := range []int{http.StatusOK, http.StatusTooManyRequests} {
		req := httptest.NewRequest(http.MethodGet, "/admin", nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		if resp.Code != want {
			t.Fatalf("request %d expected %d, got %d", i+1, want, resp.Code)
		}
	}
}
