package devserver

import (
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
)

type target struct {
	URL     *url.URL
	Tooling []string
}
type router struct {
	app, frontend atomic.Pointer[target]
	message       atomic.Pointer[string]
	blocked       atomic.Bool
}

func (r *router) status(message string) { r.message.Store(&message) }
func (r *router) setApp(address string) error {
	u, e := url.Parse(address)
	if e != nil {
		return e
	}
	if u.Scheme != "http" || u.Host == "" {
		return fmt.Errorf("invalid private application URL %q", address)
	}
	r.app.Store(&target{URL: u})
	return nil
}
func (r *router) setFrontend(address string, prefixes []string) error {
	u, e := url.Parse(address)
	if e != nil {
		return e
	}
	if u.Scheme != "http" || u.Host == "" {
		return fmt.Errorf("invalid private Vite URL %q", address)
	}
	r.frontend.Store(&target{URL: u, Tooling: append([]string(nil), prefixes...)})
	return nil
}
func (r *router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	front := r.frontend.Load()
	tooling := strings.HasPrefix(req.URL.Path, "/__skgo/dev/")
	if front != nil {
		for _, p := range front.Tooling {
			if strings.HasPrefix(req.URL.Path, p) {
				tooling = true
				break
			}
		}
	}
	upstream := r.app.Load()
	name := "application"
	if tooling {
		upstream = front
		name = "frontend tooling"
	}
	unavailable := func(w http.ResponseWriter, _ *http.Request, e error) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusServiceUnavailable)
		message := "Starting " + name
		if m := r.message.Load(); m != nil {
			message = *m
		}
		fmt.Fprintf(w, "<!doctype html><title>skgo dev</title><h1>Application is updating</h1><pre>%s</pre>", html.EscapeString(message))
	}
	if upstream == nil || (!tooling && r.blocked.Load()) {
		unavailable(w, req, nil)
		return
	}
	proxy := httputil.ReverseProxy{Rewrite: func(p *httputil.ProxyRequest) { p.SetURL(upstream.URL); p.Out.Host = p.In.Host }, ErrorHandler: unavailable}
	proxy.ServeHTTP(w, req)
}

// The successful bind itself reserves the port. Origin-dependent frontend
// initialization happens only after the caller has this listener's Addr.
func listen(address string, strict bool) (net.Listener, error) {
	if address == "" {
		address = "127.0.0.1:8080"
	}
	host, portText, e := net.SplitHostPort(address)
	if e != nil {
		return nil, e
	}
	port, e := strconv.Atoi(portText)
	if e != nil {
		return nil, e
	}
	for {
		ln, e := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
		if e == nil {
			return ln, nil
		}
		if strict || port == 0 || port == 65535 || !errors.Is(e, syscall.EADDRINUSE) {
			return nil, e
		}
		port++
	}
}
