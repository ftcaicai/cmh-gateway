// Package brand holds the OEM storefront: tenant identification, brand
// customers, pricing, teams and the brand static site. Rules: ext/AGENTS.md.
//
// P0 prototype scope (T0.3): prove that H2 + H5 alone are enough to serve a
// brand page on a brand host. The host→brand mapping comes from an environment
// variable; no database, no tables. Everything here is replaced in T2.
package brand

import (
	"net"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

// ContextKey carries the resolved brand id on the gin context. Empty means the
// request belongs to the main site.
const ContextKey = "cmh_brand_id"

// prototypeHosts parses CMH_BRAND_PROTOTYPE_HOSTS once:
//
//	CMH_BRAND_PROTOTYPE_HOSTS="demo.example.test=brand-demo,other.test=brand-2"
var (
	hostsOnce sync.Once
	hostMap   map[string]string
)

func loadHosts() {
	hostMap = map[string]string{}
	for _, pair := range strings.Split(os.Getenv("CMH_BRAND_PROTOTYPE_HOSTS"), ",") {
		host, id, ok := strings.Cut(strings.TrimSpace(pair), "=")
		if ok && host != "" && id != "" {
			hostMap[strings.ToLower(host)] = id
		}
	}
}

func hostName(raw string) string {
	if h, _, err := net.SplitHostPort(raw); err == nil {
		raw = h
	}
	return strings.TrimSuffix(strings.ToLower(raw), ".")
}

// BrandIDForHost returns the brand id mapped to host, or "" for the main site.
func BrandIDForHost(host string) string {
	hostsOnce.Do(loadHosts)
	return hostMap[hostName(host)]
}

// Resolver implements core.TenantResolver (H2).
//
// Main-site hosts pass straight through with zero allocations beyond the map
// lookup. Brand hosts get the brand id in the context and are restricted to the
// brand path allowlist; anything else on a brand host is 404 so that official
// admin/console routes can never be reached through a brand domain.
type Resolver struct{}

func (Resolver) Resolve(c *gin.Context) {
	id := BrandIDForHost(c.Request.Host)
	if id == "" {
		c.Next()
		return
	}
	c.Set(ContextKey, id)
	if !allowedOnBrandHost(c.Request.Method, c.Request.URL.Path) {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	c.Next()
}

// allowedOnBrandHost is the P0 allowlist. T2 replaces it with the full list
// ported from the old system's middleware/brand.go.
func allowedOnBrandHost(method, path string) bool {
	if method != http.MethodGet && method != http.MethodHead {
		return false
	}
	switch path {
	case "/", "/pricing", "/docs", "/login", "/register", "/favicon.ico", "/robots.txt":
		return true
	}
	return strings.HasPrefix(path, "/brand-assets/")
}

// BrandID reads the brand id set by Resolve; "" for the main site.
func BrandID(c *gin.Context) string {
	v, _ := c.Get(ContextKey)
	s, _ := v.(string)
	return s
}
