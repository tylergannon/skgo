package skgo

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
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

// NewDevPages answers page documents in this process and forwards everything
// else to a running SvelteKit dev server.
//
// It is what the dev arm serves under the registries, in the place the static
// handler holds in production, and the two answer a page the same way: the
// renderer runs the route's branch of Go loads, renders the components in the
// engine, and assembles kit's document around the result. The difference is
// only where the engine's modules come from — `vite dev` transformed them,
// one at a time, instead of a build bundling them — and what the document's
// boot script imports, which is kit's own dev client entry.
//
// Everything that is not a document is still vite's: modules, their CSS, the
// files in `static/`, and the HMR socket the browser dials on the page's own
// port. The browser only ever talks to Go, which is why the socket has to be
// tunnelled here rather than reached directly.
//
// Pass the app's endpoint registry when it has one so route additions and
// removals update endpoint matching with the rest of Kit's live graph. The
// variadic form preserves the original call for apps with pages only.
func NewDevPages(target *url.URL, m Manifest, renderer *SSR, logf func(format string, args ...any), endpointRegistries ...*Endpoints) http.Handler {
	renderer.loads.devRefresh = renderer.refreshDev
	if len(endpointRegistries) > 0 && endpointRegistries[0] != nil {
		renderer.devEndpoints = endpointRegistries[0]
		endpointRegistries[0].devRefresh = renderer.refreshDev
	}
	base := strings.TrimSuffix(m.Base, "/")
	appDir := m.AppDir
	if appDir == "" {
		appDir = "_app"
	}
	return &devPages{
		proxy:     NewDevProxy(target, logf),
		renderer:  renderer,
		base:      base,
		appPrefix: base + "/" + appDir + "/",
	}
}

type devPages struct {
	proxy     http.Handler
	renderer  *SSR
	base      string
	appPrefix string
}

func (h *devPages) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	urlPath, ok := normalizePath(r.URL.Path)
	// An upgrade is never a document, whatever path it names. Vite's HMR
	// client dials the page's own origin at `/`, which is the app's home
	// route: answered as a document it gets a 200 and an HTML body, and the
	// browser falls back to talking to vite directly — which is exactly the
	// arrangement the proxy exists to prevent.
	if ok && !isUpgrade(r) && h.isDocument(urlPath) && h.renderer.serveDev(w, r, urlPath) {
		return
	}
	// Everything the renderer did not answer is vite's, including the two it
	// declines on purpose: a branch that turns server rendering off, which kit's
	// own dev server answers with its shell, and a route that is an endpoint
	// and nothing else.
	h.proxy.ServeHTTP(w, r)
}

// viteOwned are the path prefixes that are the dev server's by construction:
// vite's own client and filesystem endpoints, the app's sources as modules, and
// kit's generated tree. A request for one of them is never a document, whatever
// it asks for in its Accept header.
var viteOwned = []string{"/@", "/node_modules/", "/src/", "/.svelte-kit/", "/__skgo_dev/"}

// isDocument reports whether Go should try to render this path.
//
// A path that matches a route is one, including a path that matches nothing —
// kit answers that with the app's own error page at 404 (`respond.js`,
// `respond_with_error`) rather than with whatever the dev server would say. A
// path with a file extension that matches no route is a file: something out of
// `static/`, which vite serves and a build would have copied into the client
// tree.
func (h *devPages) isDocument(urlPath string) bool {
	if h.base != "" && urlPath != h.base && !strings.HasPrefix(urlPath, h.base+"/") {
		return false
	}
	if strings.HasPrefix(urlPath, h.appPrefix) {
		return false
	}
	routePath := strings.TrimPrefix(urlPath, h.base)
	if routePath == "" {
		routePath = "/"
	}
	for _, prefix := range viteOwned {
		if strings.HasPrefix(routePath, prefix) {
			return false
		}
	}
	if _, _, matched := h.renderer.loads.match(routePath); matched {
		return true
	}
	return path.Ext(routePath) == ""
}
