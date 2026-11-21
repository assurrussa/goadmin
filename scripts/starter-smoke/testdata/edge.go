// The isolated HTTPS fixture forwards only to its own starter container.
package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

func main() {
	addr := flag.String("listen", ":8443", "local TLS listener")
	flag.Parse()
	proxy := &httputil.ReverseProxy{Rewrite: func(r *httputil.ProxyRequest) {
		r.SetURL(&url.URL{Scheme: "http", Host: "admin:8080"})
		r.Out.Host = r.In.Host
		peer, _, err := net.SplitHostPort(r.In.RemoteAddr)
		if err != nil {
			return
		}
		r.Out.Header.Set("X-Real-IP", peer)
		r.Out.Header.Set("X-Forwarded-Proto", "https")
	}}
	server := http.Server{Addr: *addr, Handler: proxy, ReadHeaderTimeout: 5 * time.Second}
	log.Fatal(server.ListenAndServeTLS("/qa/leaf.pem", "/qa/leaf.key"))
}
