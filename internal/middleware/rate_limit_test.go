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

func TestPruneExpiredRateLimitBuckets(t *testing.T) {
	now := time.Now()
	buckets := map[string]rateLimitBucket{
		"expired": {windowStart: now.Add(-2 * time.Minute), count: 10},
		"active":  {windowStart: now.Add(-10 * time.Second), count: 1},
	}

	pruneExpiredRateLimitBuckets(buckets, now, time.Minute)
	if _, exists := buckets["expired"]; exists {
		t.Fatal("expired rate limit bucket was not removed")
	}
	if _, exists := buckets["active"]; !exists {
		t.Fatal("active rate limit bucket was removed")
	}
}
