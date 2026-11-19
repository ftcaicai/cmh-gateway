package brand

import (
	"context"
	"encoding/xml"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/ext/migrations"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// storefrontView is everything page.html needs. All text is escaped by
// html/template; owners cannot inject markup, scripts or verification tags.
type storefrontView struct {
	Name, Page, Theme            string
	Title, Description, Keywords string
	Canonical                    string
	Auth, Logo, NoIndex          bool
	LogoVersion                  int64
	// TurnstileSiteKey is set on the sign-in page while the main site requires
	// the Cloudflare check on /api/user/login; the brand page then has to
	// present the same token or the official middleware rejects the login.
	TurnstileSiteKey string
	indexable        bool   // the owner allows indexing at all
	origin           string // https://<verified domain>, or ""
}

// turnstileSources is the only third-party origin the storefront ever allows,
// and only on the sign-in page while the check is on.
const turnstileSources = " https://challenges.cloudflare.com"

// pageTitles are the inner pages; the home page uses the site title alone.
var pageTitles = map[string]string{"pricing": "模型价格", "docs": "接入文档", "login": "登录", "account": "我的账号"}

// loadStorefront builds the view of one page of a site. A site or setting that
// cannot be read falls back to neutral defaults instead of the main site's
// identity. The canonical address comes only from the site's verified domain,
// never from the request.
func loadStorefront(ctx context.Context, siteID, page, path string) storefrontView {
	view := storefrontView{Name: "品牌站", Page: page, Theme: brandThemes[0], Auth: authEnabled, indexable: true}
	if page == "login" && authEnabled && common.TurnstileCheckEnabled {
		view.TurnstileSiteKey = common.TurnstileSiteKey
	}
	if model.DB == nil {
		view.Title = view.Name
		return view
	}
	var site migrations.OEMSiteRecord
	if model.DB.WithContext(ctx).Select("name", "description").Where("id = ?", siteID).Take(&site).Error == nil && site.Name != "" {
		view.Name, view.Description = site.Name, site.Description
	}
	if appearance, err := loadAppearance(ctx, model.DB, siteID); err == nil {
		view.Theme, view.Keywords, view.indexable = appearance.Theme, appearance.SEOKeywords, appearance.AllowIndexing
		view.Logo, view.LogoVersion = appearance.HasLogo, appearance.LogoUpdatedAt
		view.Title = appearance.SEOTitle
		if appearance.SEODescription != "" {
			view.Description = appearance.SEODescription
		}
	}
	if view.Title == "" {
		view.Title = view.Name
	}
	if title := pageTitles[page]; title != "" {
		view.Title = title + " - " + view.Title
	}
	var domain migrations.OEMSiteDomainRecord
	if model.DB.WithContext(ctx).Select("verified_host").Where("site_id = ? AND verified_host IS NOT NULL", siteID).Take(&domain).Error == nil {
		view.origin = "https://" + *domain.VerifiedHost
	}
	// Sign-in and account pages are never indexed, whatever the owner chose.
	view.NoIndex = !view.indexable || page == "login" || page == "account"
	if !view.NoIndex && view.origin != "" {
		view.Canonical = view.origin + path
	}
	return view
}

// serveRobots and serveSitemap list only the three public pages, and only
// when the owner allows indexing. Without a verified domain there is no
// canonical address, so the sitemap stays empty.
func serveRobots(c *gin.Context, view storefrontView) {
	body := "User-agent: *\nDisallow: /\n"
	if view.indexable {
		body = "User-agent: *\nAllow: /\nDisallow: /login\nDisallow: /account\nDisallow: /api/\n"
		if view.origin != "" {
			body += "Sitemap: " + view.origin + "/sitemap.xml\n"
		}
	}
	c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(body))
}

func serveSitemap(c *gin.Context, view storefrontView) {
	var body strings.Builder
	body.WriteString(xml.Header + `<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	if view.indexable && view.origin != "" {
		for _, path := range []string{"/", "/pricing", "/docs"} {
			body.WriteString("<url><loc>")
			_ = xml.EscapeText(&body, []byte(view.origin+path))
			body.WriteString("</loc></url>")
		}
	}
	body.WriteString("</urlset>\n")
	c.Data(http.StatusOK, "application/xml; charset=utf-8", []byte(body.String()))
}

// serveLogo returns the uploaded logo. Its bytes were fully decoded as an
// image at upload time; nosniff is already set by the caller.
func serveLogo(c *gin.Context, siteID string) {
	if model.DB == nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	logo, err := siteLogo(c.Request.Context(), model.DB, siteID)
	if err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	// The page links the logo with its update time, so a change is a new URL.
	c.Header("Cache-Control", "public, max-age=3600")
	c.Data(http.StatusOK, logo.ContentType, logo.Data)
}
