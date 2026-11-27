package brand

import (
	"html/template"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/brandweb"
	"github.com/QuantumNous/new-api/ext/migrations"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// Routes implements core.RouteRegistrar (H5) for the brand storefront.
//
// We cannot (and must not) replace the official NoRoute, so brand pages are
// served from an early handler that only acts when Resolve has tagged the
// request as a brand request. The one real route is the brand customer's own
// minimal profile, which reuses the official dashboard authentication.
type Routes struct{}

func (Routes) Register(engine *gin.Engine) {
	// Registered before Use: Gin attaches serveBrandPage to every later route,
	// and that handler answers 404 for anything on a brand host it does not own.
	engine.GET("/api/ext/brand/self", middleware.DisableCache(), middleware.UserAuth(), brandSelf)
	engine.GET("/api/ext/brand/pricing", brandPricing)
	engine.GET("/api/ext/brand/tls-allowed", tlsAllowed)
	engine.Use(serveBrandPage)
}

var brandPage = template.Must(template.ParseFS(brandweb.Assets, "page.html"))

// brandAssets are the only static files of the storefront; value is the type.
var brandAssets = map[string]string{
	"site.css": "text/css; charset=utf-8",
	"site.js":  "text/javascript; charset=utf-8",
}

// brandSelf is the signed-in brand customer's own minimal profile. UserAuth has
// already checked the session against this site and browser (H8). It exposes
// no quota, role, group or key of the official account.
func brandSelf(c *gin.Context) {
	siteID := BrandID(c)
	if siteID == "" || !authEnabled {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	user, err := model.GetUserById(c.GetInt("id"), false)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"username": user.Username, "display_name": user.DisplayName, "email": user.Email, "site_name": brandSiteName(siteID),
	}})
}

// tlsAllowed answers the reverse proxy's on-demand TLS question: may a
// certificate be issued for this domain? Only verified brand domains (and the
// operator's static mappings) qualify. It reveals nothing DNS does not, and it
// is not reachable through a brand host.
func tlsAllowed(c *gin.Context) {
	if BrandID(c) != "" || BrandIDForHost(c.Query("domain")) == "" {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	c.Status(http.StatusOK)
}

// brandSiteName is the public name of the site, or a neutral fallback while
// the mapped site does not exist. It never falls back to the main site's name.
func brandSiteName(siteID string) string {
	var site migrations.OEMSiteRecord
	if model.DB == nil || model.DB.Select("name").Where("id = ?", siteID).Take(&site).Error != nil || site.Name == "" {
		return "品牌站"
	}
	return site.Name
}

// serveBrandPage owns every page on a mapped brand host. Until a page is
// implemented, it returns 404 rather than exposing the official storefront.
func serveBrandPage(c *gin.Context) {
	id := BrandID(c)
	if id == "" {
		c.Next()
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	path := c.Request.URL.Path
	if name, ok := strings.CutPrefix(path, "/brand-assets/"); ok && brandAssets[name] != "" {
		content, err := brandweb.Assets.ReadFile(name)
		if err != nil {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		c.Data(http.StatusOK, brandAssets[name], content)
		c.Abort()
		return
	}
	switch path {
	case "/brand-assets/logo":
		serveLogo(c, id)
		c.Abort()
		return
	case "/robots.txt":
		serveRobots(c, loadStorefront(c.Request.Context(), id, "", path))
		c.Abort()
		return
	case "/sitemap.xml":
		serveSitemap(c, loadStorefront(c.Request.Context(), id, "", path))
		c.Abort()
		return
	}
	page, ok := brandPages[path]
	if !ok || (page == "account" && !authEnabled) {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	// Auth false renders the sign-in page as a notice without a form.
	renderStorefront(c, id, http.StatusOK, loadStorefront(c.Request.Context(), id, page, path))
}

// renderStorefront writes one storefront page with its security headers.
func renderStorefront(c *gin.Context, id string, status int, view storefrontView) {
	// The Turnstile widget is a script and an iframe from Cloudflare; nothing
	// else on the storefront ever loads from a third party.
	third := ""
	if view.TurnstileSiteKey != "" {
		third = turnstileSources
	}
	c.Header("X-CMH-Tenant", id)
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Header("Referrer-Policy", "same-origin")
	c.Header("Content-Security-Policy", "default-src 'none'; style-src 'self'; script-src 'self'"+third+"; frame-src"+third+"; connect-src 'self'; img-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	c.Status(status)
	if c.Request.Method != http.MethodHead {
		_ = brandPage.Execute(c.Writer, view)
	}
	c.Abort()
}

// servePausedSite answers every request on a paused site's hosts: a branded
// notice for pages, a fixed JSON error for APIs. Nothing else is reachable.
func servePausedSite(c *gin.Context, id string) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	if strings.HasPrefix(c.Request.URL.Path, "/api/") {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "code": "brand_site_suspended", "message": "站点已暂停服务"})
		return
	}
	view := loadStorefront(c.Request.Context(), id, "paused", c.Request.URL.Path)
	renderStorefront(c, id, http.StatusForbidden, view)
}

// brandPages maps a storefront path to its section of page.html.
var brandPages = map[string]string{
	"/": "home", "/pricing": "pricing", "/docs": "docs", "/login": "login", "/account": "account",
}
