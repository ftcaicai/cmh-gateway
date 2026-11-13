package brand

import (
	"html"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Routes implements core.RouteRegistrar (H5) for the brand storefront.
//
// P0 prototype: one middleware-style handler that answers brand hosts before
// the official web fallback runs. We cannot (and must not) replace the
// official NoRoute, so brand pages are served from an early handler that only
// acts when Resolve has tagged the request as a brand request.
type Routes struct{}

func (Routes) Register(engine *gin.Engine) {
	engine.Use(serveBrandPage)
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
	if c.Request.URL.Path != "/" {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	c.Header("X-CMH-Tenant", id)
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Status(http.StatusOK)
	if c.Request.Method != http.MethodHead {
		_, _ = c.Writer.Write([]byte("<!doctype html><html lang=\"zh-CN\"><meta charset=\"utf-8\"><title>brand prototype</title>" +
			"<h1>brand: " + html.EscapeString(id) + "</h1></html>"))
	}
	c.Abort()
}
