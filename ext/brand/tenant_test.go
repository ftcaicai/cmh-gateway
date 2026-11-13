package brand

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/ext/core"
	"github.com/gin-gonic/gin"
)

// newEngine mirrors the official wiring: H2 middleware first, H5 registration,
// then an official-style NoRoute fallback that must never run on a brand host.
func newEngine(t *testing.T) *gin.Engine {
	t.Helper()
	t.Setenv("CMH_BRAND_PROTOTYPE_HOSTS", "demo.brand.test=brand-demo")
	// sync.Once cannot be reset; load the map directly so the test env is honoured
	// regardless of whether another test already triggered the Once.
	loadHosts()
	hostsOnce.Do(func() {})

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	core.Tenant = Resolver{}
	core.Routes = Routes{}
	engine.Use(core.TenantMiddleware())
	core.RegisterRoutes(engine)
	engine.GET("/api/user/self", func(c *gin.Context) { c.String(200, "official api") })
	engine.NoRoute(func(c *gin.Context) { c.String(200, "official index") })
	return engine
}

func do(engine *gin.Engine, method, host, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.Host = host
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func TestMainSiteUntouched(t *testing.T) {
	e := newEngine(t)
	if w := do(e, http.MethodGet, "www.main.test", "/"); w.Code != 200 || w.Body.String() != "official index" {
		t.Fatalf("main site index: %d %q", w.Code, w.Body.String())
	}
	if w := do(e, http.MethodGet, "www.main.test", "/api/user/self"); w.Code != 200 || w.Body.String() != "official api" {
		t.Fatalf("main site api: %d %q", w.Code, w.Body.String())
	}
	if w := do(e, http.MethodGet, "www.main.test:3000", "/"); w.Header().Get("X-CMH-Tenant") != "" {
		t.Fatalf("main site must not be tagged as tenant")
	}
}

func TestBrandHostServesBrandPage(t *testing.T) {
	e := newEngine(t)
	w := do(e, http.MethodGet, "demo.brand.test", "/")
	if w.Code != 200 || w.Header().Get("X-CMH-Tenant") != "brand-demo" {
		t.Fatalf("brand index: %d tenant=%q", w.Code, w.Header().Get("X-CMH-Tenant"))
	}
	if body := w.Body.String(); body == "official index" || !contains(body, "brand: brand-demo") {
		t.Fatalf("brand page not served, got %q", body)
	}
	// Port suffix and case must not break host matching.
	if w := do(e, http.MethodGet, "DEMO.brand.test:443", "/"); w.Header().Get("X-CMH-Tenant") != "brand-demo" {
		t.Fatalf("host normalisation failed")
	}
}

func TestBrandHostBlocksOfficialRoutes(t *testing.T) {
	e := newEngine(t)
	// Official API must be unreachable through a brand domain.
	if w := do(e, http.MethodGet, "demo.brand.test", "/api/user/self"); w.Code != http.StatusNotFound {
		t.Fatalf("official api on brand host: want 404, got %d", w.Code)
	}
	// Non-GET on brand host is rejected in P0.
	if w := do(e, http.MethodPost, "demo.brand.test", "/"); w.Code != http.StatusNotFound {
		t.Fatalf("POST on brand host: want 404, got %d", w.Code)
	}
	// Allowlisted path other than "/" falls through to official fallback in P0
	// (T2 wires brandweb); it must still be tagged as tenant.
	if w := do(e, http.MethodGet, "demo.brand.test", "/pricing"); w.Code != 200 || BrandIDForHost("demo.brand.test") != "brand-demo" {
		t.Fatalf("allowlisted brand path: %d", w.Code)
	}
}

func contains(s, sub string) bool { return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0) }
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
