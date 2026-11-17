// Command brand-proxy is a LOCAL DEVELOPMENT helper for trying the brand
// storefront without a TLS certificate. A brand host only accepts
// state-changing requests whose Origin is exactly https://<host>; a browser on
// http://brand.localhost sends http://, so this proxy presents it as https://
// and forwards everything else unchanged. Never run it in front of a real site.
//
//	go run ./ops/dev/brand-proxy -listen 127.0.0.1:3312 -gateway http://127.0.0.1:3000
package main

import (
	"flag"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:3312", "address the browser connects to")
	gateway := flag.String("gateway", "http://127.0.0.1:3000", "local gateway URL")
	flag.Parse()
	target, err := url.Parse(*gateway)
	if err != nil || target.Hostname() != "127.0.0.1" && target.Hostname() != "localhost" {
		log.Fatal("gateway must be a local URL")
	}
	proxy := &httputil.ReverseProxy{Rewrite: func(r *httputil.ProxyRequest) {
		r.SetURL(target)
		r.Out.Host = r.In.Host
		if origin, ok := strings.CutPrefix(r.In.Header.Get("Origin"), "http://"); ok {
			r.Out.Header.Set("Origin", "https://"+origin)
		}
	}}
	log.Printf("brand-proxy: http://brand.localhost:%s -> %s", (*listen)[strings.LastIndex(*listen, ":")+1:], *gateway)
	log.Fatal(http.ListenAndServe(*listen, proxy))
}
