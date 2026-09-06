package skgo

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// NewDevProxy forwards every request to a running SvelteKit dev server so that
// the browser only ever talks to Go. HTTP/1.1 upgrades are tunnelled by
// net/http itself, which is what carries Vite's HMR WebSocket: its client
// dials the page's own port, so the upgrade lands here and must reach Vite.
//
// logf, if non-nil, is called once per upgrade request with a printf-style
// format and arguments. Observing that line is the only way to tell a real
// proxied HMR socket from Vite's direct-to-5173 fallback.
func NewDevProxy(target *url.URL, logf func(format string, args ...any)) http.Handler {
	proxy := &httputil.ReverseProxy{
		Rewrite: func(p *httputil.ProxyRequest) {
			p.SetURL(target)
			// Vite checks the inbound Host against its allowed hosts; keeping
			// the browser's host makes 127.0.0.1:8080 a loopback host it
			// accepts without configuration.
			p.Out.Host = p.In.Host
			p.SetXForwarded()
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			http.Error(w, "skgo: dev server at "+target.String()+" is unreachable: "+err.Error(), http.StatusBadGateway)
		},
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if logf != nil && isUpgrade(r) {
			logf("ws upgrade %s (%s)", r.URL.Path, r.Header.Get("Upgrade"))
		}
		proxy.ServeHTTP(w, r)
	})
}

func isUpgrade(r *http.Request) bool {
	for _, token := range strings.Split(r.Header.Get("Connection"), ",") {
		if strings.EqualFold(strings.TrimSpace(token), "upgrade") {
			return r.Header.Get("Upgrade") != ""
		}
	}
	return false
}
