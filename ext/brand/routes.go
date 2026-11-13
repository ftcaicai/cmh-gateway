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

// serveBrandPage renders the brand page for GET/HEAD "/" on a brand host and
// aborts, so the official index is never returned on a brand domain.
// Other allowlisted brand paths fall through in P0; T2 wires brandweb.
func serveBrandPage(c *gin.Context) {
	id := BrandID(c)
	if id == "" || c.Request.URL.Path != "/" {
		c.Next()
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("X-CMH-Tenant", id)
	c.Data(http.StatusOK, "text/html; charset=utf-8",
		[]byte("<!doctype html><html lang=\"zh-CN\"><meta charset=\"utf-8\"><title>brand prototype</title>"+
			"<h1>brand: "+html.EscapeString(id)+"</h1><p>served by ext/brand via hooks H2+H5</p></html>"))
	c.Abort()
}
