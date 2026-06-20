package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestParseSyncPullQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("GET", "/api/v1/config/sync/pull?cursor=42&limit=250", nil)

	cursor, limit, ok := parseSyncPullQuery(ctx)
	if !ok || cursor != 42 || limit != 250 {
		t.Fatalf("unexpected query result: cursor=%d limit=%d ok=%v", cursor, limit, ok)
	}
}

func TestParseSyncPullQueryRejectsInvalidValues(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, target := range []string{
		"/api/v1/config/sync/pull?cursor=-1",
		"/api/v1/config/sync/pull?limit=0",
		"/api/v1/config/sync/pull?limit=invalid",
	} {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest("GET", target, nil)
		if _, _, ok := parseSyncPullQuery(ctx); ok {
			t.Fatalf("expected invalid query to be rejected: %s", target)
		}
	}
}
