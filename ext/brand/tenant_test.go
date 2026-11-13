package brand

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/ext/core"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Official API routes are registered BEFORE H5, as in router.SetRouter.
// This catches the Gin rule that engine.Use only attaches to later routes.
func newEngine(t *testing.T) *gin.Engine {
	t.Helper()
	t.Setenv("CMH_BRAND_PROTOTYPE_HOSTS", "demo.brand.test=brand-demo")
	loadHosts()
	hostsOnce.Do(func() {})
	previousTenant, previousRoutes := core.Tenant, core.Routes
	t.Cleanup(func() { core.Tenant, core.Routes = previousTenant, previousRoutes })
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	core.Tenant, core.Routes = Resolver{}, Routes{}
	engine.Use(core.TenantMiddleware())
	engine.GET("/api/user/self", func(c *gin.Context) { c.String(200, "official api") })
	core.RegisterRoutes(engine)
	engine.GET("/cmh/whoami", func(c *gin.Context) { c.String(200, "private console") })
	engine.NoRoute(func(c *gin.Context) { c.String(200, "official index") })
	return engine
}

func TestHostIsolation(t *testing.T) {
	e := newEngine(t)
	cases := []struct {
		name, method, host, path string
		status                   int
		body                     string
	}{
		{"main_index", "GET", "www.main.test", "/", 200, "official index"},
		{"main_api", "GET", "www.main.test", "/api/user/self", 200, "official api"},
		{"main_port", "GET", "www.main.test:3000", "/", 200, "official index"},
		{"brand_home", "GET", "demo.brand.test", "/", 200, "brand: brand-demo"},
		{"normalized_host", "GET", "DEMO.brand.test:443", "/", 200, "brand: brand-demo"},
		{"official_api_blocked", "GET", "demo.brand.test", "/api/user/self", 404, ""},
		{"console_blocked", "GET", "demo.brand.test", "/cmh/whoami", 404, ""},
		{"unimplemented_pricing", "GET", "demo.brand.test", "/pricing", 404, ""},
		{"unimplemented_asset", "GET", "demo.brand.test", "/brand-assets/missing.js", 404, ""},
		{"brand_post", "POST", "demo.brand.test", "/", 404, ""},
		{"brand_head", "HEAD", "demo.brand.test", "/", 200, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, nil)
			r.Host = tc.host
			w := httptest.NewRecorder()
			e.ServeHTTP(w, r)
			require.Equal(t, tc.status, w.Code)
			if tc.name == "brand_home" || tc.name == "normalized_host" {
				require.Contains(t, w.Body.String(), tc.body)
				require.NotContains(t, w.Body.String(), "official index")
			} else {
				require.Equal(t, tc.body, w.Body.String())
			}
		})
	}
}
