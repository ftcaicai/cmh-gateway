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

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/ext/core"
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
			// Same normalization as the lookup, so a key written with a port,
			// upper case or a trailing dot still selects the brand.
			hostMap[hostName(host)] = id
			if authEnabled && !validSiteID(id) {
				common.SysError(common.LogText("CMH_BRAND_PROTOTYPE_HOSTS: %q needs the 32-character lowercase hex site ID from /cmh/brands; brand login stays unavailable on that host", hostName(host)))
			}
		}
	}
}

func validSiteID(id string) bool {
	_, err := core.NewBrandAuthScope(id, "https://site.invalid", strings.Repeat("0", 64))
	return err == nil
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
	name := hostName(host)
	if id := hostMap[name]; id != "" {
		return id
	}
	// Self-service domains verified through their CNAME (domains.go).
	return verifiedSiteForHost(name)
}

// Resolver implements core.TenantResolver (H2).
//
// Main-site hosts pass straight through with zero allocations beyond the map
// lookup. Brand hosts get the brand id in the context and are restricted to the
// brand path allowlist; anything else on a brand host is 404 so that official
// admin/console routes can never be reached through a brand domain.
type Resolver struct{}

// With H8 enabled it also puts the request's authentication scope into the
// request context: the main-site scope for every other host, the brand scope
// (site, exact origin, browser) for a brand host.
func (Resolver) Resolve(c *gin.Context) {
	id := BrandIDForHost(c.Request.Host)
	if id == "" {
		if authEnabled {
			c.Request = c.Request.WithContext(core.WithAuthScope(c.Request.Context(), core.MainAuthScope()))
		}
		c.Next()
		return
	}
	c.Set(ContextKey, id)
	if !allowedOnBrandHost(c.Request.Method, c.Request.URL.Path) {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	// The public price list carries no identity, so it needs no auth scope
	// (and no browser cookie).
	if authEnabled && c.Request.URL.Path != "/api/ext/brand/pricing" {
		scope, ok := brandAuthScope(c, id)
		if !ok {
			return
		}
		c.Request = c.Request.WithContext(core.WithAuthScope(c.Request.Context(), scope))
	}
	c.Next()
}

// allowedOnBrandHost is the P0 allowlist. T2 replaces it with the full list
// ported from the old system's middleware/brand.go. With H8 enabled it also
// admits exactly the password/TOTP login, refresh, logout and own-profile
// calls; registration, recovery, OAuth and Passkey stay closed.
func allowedOnBrandHost(method, path string) bool {
	if authEnabled {
		switch method + " " + path {
		case "GET /account", "GET /api/user/login/encryption-key", "GET /api/ext/brand/self",
			"POST /api/user/login", "POST /api/user/login/verify",
			"POST /api/user/auth/refresh", "POST /api/user/auth/logout":
			return true
		}
	}
	if method != http.MethodGet && method != http.MethodHead {
		return false
	}
	switch path {
	case "/", "/pricing", "/docs", "/login", "/register", "/favicon.ico", "/robots.txt", "/sitemap.xml", "/api/ext/brand/pricing":
		return true
	}
	return strings.HasPrefix(path, "/brand-assets/")
}

// hostAuthority is the lower-case host with its port kept, as an origin needs.
func hostAuthority(raw string) string {
	host, port, err := net.SplitHostPort(raw)
	if err != nil {
		return hostName(raw)
	}
	return net.JoinHostPort(hostName(host), port)
}

// BrandID reads the brand id set by Resolve; "" for the main site.
func BrandID(c *gin.Context) string {
	v, _ := c.Get(ContextKey)
	s, _ := v.(string)
	return s
}
