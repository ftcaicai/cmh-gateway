// Package brandweb holds the OEM brand storefront: a Go-template static site
// rendered by ext/brand and never coupled to a React frontend (ext/AGENTS.md
// §6.5). Brand pages load nothing from the main site.
package brandweb

import "embed"

//go:embed page.html site.css site.js
var Assets embed.FS
