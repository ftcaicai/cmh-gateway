// Package console serves the independent web-cmh application under /cmh/.
// Built assets live in this package's dist directory for go:embed. Serving the
// shell grants no access: protected APIs enforce the official access/session
// contract independently of this static handler.
package console

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed all:dist
var distFS embed.FS

// Mount registers the independent console. A clean Go-only checkout embeds
// .gitkeep but not index.html and deliberately returns 503, not a false success.
func Mount(engine *gin.Engine) {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		sub = nil
	}
	mount(engine, sub)
}

func mount(engine *gin.Engine, files fs.FS) {
	var index []byte
	if files != nil {
		index, _ = fs.ReadFile(files, "index.html")
	}
	handler := func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Robots-Tag", "noindex")
		c.Header("Cache-Control", "no-store")
		if len(index) == 0 {
			c.Status(http.StatusServiceUnavailable)
			return
		}
		rel := strings.TrimPrefix(c.Request.URL.Path, "/cmh/")
		if c.Request.URL.Path == "/cmh" {
			rel = ""
		}
		// Do not serve directory listings, dotfiles, or traversal paths. The
		// SPA fallback is only for extensionless page routes, never missing JS.
		if rel != "" && (!fs.ValidPath(rel) || strings.HasPrefix(rel, ".") || strings.Contains(rel, "/.")) {
			c.Status(http.StatusNotFound)
			return
		}
		if rel != "" && rel != "index.html" {
			if stat, err := fs.Stat(files, rel); err == nil && !stat.IsDir() {
				if strings.HasPrefix(rel, "static/") {
					c.Header("Cache-Control", "public, max-age=31536000, immutable")
				}
				http.StripPrefix("/cmh", http.FileServer(http.FS(files))).ServeHTTP(c.Writer, c.Request)
				return
			}
			if strings.HasPrefix(rel, "static/") || path.Ext(rel) != "" {
				c.Status(http.StatusNotFound)
				return
			}
		}
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.Status(http.StatusOK)
		if c.Request.Method != http.MethodHead {
			_, _ = c.Writer.Write(index)
		}
	}
	engine.GET("/cmh", handler)
	engine.HEAD("/cmh", handler)
	engine.GET("/cmh/*any", handler)
	engine.HEAD("/cmh/*any", handler)
}
