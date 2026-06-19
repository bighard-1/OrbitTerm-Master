package adminweb

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestEmbeddedAdminConsoleAssets(t *testing.T) {
	for _, path := range []string{"static/index.html", "static/styles.css", "static/app.js"} {
		data, err := content.ReadFile(path)
		if err != nil {
			t.Fatalf("expected embedded asset %s: %v", path, err)
		}
		if len(data) == 0 {
			t.Fatalf("embedded asset %s is empty", path)
		}
	}
}

func TestAdminConsoleRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	Register(engine)

	redirectResp := httptest.NewRecorder()
	redirectReq := httptest.NewRequest(http.MethodGet, "/admin-console", nil)
	engine.ServeHTTP(redirectResp, redirectReq)
	if redirectResp.Code != http.StatusPermanentRedirect {
		t.Fatalf("expected redirect status 308, got %d", redirectResp.Code)
	}
	if redirectResp.Header().Get("Location") != "/admin-console/" {
		t.Fatalf("unexpected redirect location: %q", redirectResp.Header().Get("Location"))
	}

	indexResp := httptest.NewRecorder()
	indexReq := httptest.NewRequest(http.MethodGet, "/admin-console/", nil)
	engine.ServeHTTP(indexResp, indexReq)
	if indexResp.Code != http.StatusOK {
		t.Fatalf("expected index status 200, got %d", indexResp.Code)
	}
	if !strings.Contains(indexResp.Body.String(), "OrbitTerm Admin Console") {
		t.Fatal("admin console index did not render expected title")
	}

	assetResp := httptest.NewRecorder()
	assetReq := httptest.NewRequest(http.MethodGet, "/admin-console/assets/styles.css", nil)
	engine.ServeHTTP(assetResp, assetReq)
	if assetResp.Code != http.StatusOK {
		t.Fatalf("expected asset status 200, got %d", assetResp.Code)
	}

	traversalResp := httptest.NewRecorder()
	traversalReq := httptest.NewRequest(http.MethodGet, "/admin-console/assets/../index.html", nil)
	engine.ServeHTTP(traversalResp, traversalReq)
	if traversalResp.Code != http.StatusBadRequest && traversalResp.Code != http.StatusNotFound {
		t.Fatalf("expected traversal request to be rejected, got %d", traversalResp.Code)
	}
}
