package devserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
)

type target struct {
	URL       *url.URL
	Transport *http.Transport
}
type router struct {
	app, frontend atomic.Pointer[target]
	remotePrefix  atomic.Pointer[string]
}

func (r *router) setRemotePrefix(prefix string) { r.remotePrefix.Store(&prefix) }

// Each target has its own transport, so pooled connections never cross instances.
func socketTarget(socket string) (*target, error) {
	if !filepath.IsAbs(socket) {
		return nil, fmt.Errorf("private socket must be absolute: %q", socket)
	}
	u, _ := url.Parse("http://skgo.invalid")
	return &target{URL: u, Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}}, nil
}
func (r *router) setApp(socket string) error {
	t, e := socketTarget(socket)
	if e != nil {
		return e
	}
	if old := r.app.Swap(t); old != nil {
		old.Transport.CloseIdleConnections()
	}
	return nil
}
func (r *router) setFrontend(socket string, _ []string) error {
	t, e := socketTarget(socket)
	if e != nil {
		return e
	}
	if old := r.frontend.Swap(t); old != nil {
		old.Transport.CloseIdleConnections()
	}
	return nil
}
func (r *router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	switch req.URL.Path {
	case "/__skgo/dev/state", "/__skgo/dev/snapshot", "/__skgo/dev/published":
		w.Header().Set("X-Skgo-Mode", "dev")
		http.NotFound(w, req)
		return
	}
	upstream := r.frontend.Load()
	prefix := "/_app/remote/"
	if p := r.remotePrefix.Load(); p != nil {
		prefix = *p
	}
	if strings.HasPrefix(req.URL.Path, prefix) {
		upstream = r.app.Load()
	}
	unavailable := func(w http.ResponseWriter, _ *http.Request, e error) {
		w.Header().Set("X-Skgo-Mode", "dev")
		http.Error(w, "Development upstream unavailable", http.StatusServiceUnavailable)
	}
	if upstream == nil {
		unavailable(w, req, nil)
		return
	}
	proxy := httputil.ReverseProxy{Transport: upstream.Transport, ModifyResponse: func(res *http.Response) error { res.Header.Set("X-Skgo-Mode", "dev"); return nil }, Rewrite: func(p *httputil.ProxyRequest) {
		p.SetURL(upstream.URL)
		p.Out.Host = p.In.Host
		p.SetXForwarded()
		p.Out.Header.Del("X-Skgo-Internal-Fetch")
		p.Out.Header.Del("X-Skgo-Fetch-Depth")
		// Headers from the public client are untrusted. Derive this from the socket.
		address, _, err := net.SplitHostPort(p.In.RemoteAddr)
		if err != nil {
			address = p.In.RemoteAddr
		}
		p.Out.Header.Set("X-Skgo-Client-Address", address)
	}, ErrorHandler: unavailable}
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
