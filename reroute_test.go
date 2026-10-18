package skgo

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRerouteSelectsGoHandlersAndPreservesOriginalURL(t *testing.T) {
	manifest := Manifest{Nodes: []string{"page"}, Routes: []ManifestRoute{
		{ID: "/target/[id]", Pattern: `^/target/([^/]+)/?$`, Params: []ManifestParam{{Name: "id"}}, Page: &ManifestPage{Leaf: 0}},
		{ID: "/api/[id]", Pattern: `^/api/([^/]+)/?$`, Params: []ManifestParam{{Name: "id"}}, Endpoint: &ManifestEndpoint{Methods: []string{"GET"}}},
		{ID: "/resolver", Pattern: `^/resolver/?$`, Endpoint: &ManifestEndpoint{Methods: []string{"GET"}}},
	}}
	receipt := func(event *Event) string {
		return fmt.Sprintf("route=%s|id=%s|url=%s", event.RouteID(), event.Param("id"), event.URL().RequestURI())
	}
	loads, err := NewLoads(manifest.LoadConfig("http://app.test"), NewServerLoad(LoadSpec{Module: "page", Run: func(ctx context.Context) (any, error) { return map[string]any{"receipt": receipt(EventFrom(ctx))}, nil }}))
	if err != nil {
		t.Fatal(err)
	}
	endpoints, err := NewEndpoints(manifest.EndpointConfig("http://app.test"), NewEndpoint("/api/[id]", "GET", func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, receipt(EventFrom(r.Context()))) }), NewEndpoint("/resolver", "GET", func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "/target/73") }))
	if err != nil {
		t.Fatal(err)
	}
	var handled, preRouteFetch atomic.Int32
	cfg := manifest.HandleConfig()
	cfg.Loads = loads
	cfg.Origin = "http://app.test"
	cfg.Reroute = func(ctx context.Context, event RerouteEvent) (*string, error) {
		path := event.URL.Path
		event.URL.Path = "/mutation-must-not-escape"
		if path == "/alias" {
			req, _ := http.NewRequestWithContext(ctx, "GET", "/resolver", nil)
			response, err := event.Fetch(req)
			if err != nil {
				return nil, err
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil {
				return nil, err
			}
			target := string(body)
			return &target, nil
		}
		if path == "/api-alias" {
			target := "/api/91"
			return &target, nil
		}
		if path == "/failure" {
			return nil, fmt.Errorf("REROUTE_PRIVATE_FAILURE_167")
		}
		if path == "/panic" {
			panic("REROUTE_PRIVATE_PANIC_167")
		}
		return nil, nil
	}
	hook := Middleware(func(ctx context.Context, event *Event, resolve Resolve) (*http.Response, error) {
		handled.Add(1)
		return resolve(ctx)
	})
	page := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		event := EventFrom(r.Context())
		if event.RouteID() == "" {
			http.NotFound(w, r)
			return
		}
		io.WriteString(w, receipt(event))
	})
	var app http.Handler
	stack := hook.Intercept(cfg, loads.Intercept(endpoints.Intercept(page)))
	app = FetchConfig{Origin: "http://app.test", Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { app.ServeHTTP(w, r) }), HandleFetch: func(ctx context.Context, r *http.Request, next Fetch) (*http.Response, error) {
		event := EventFrom(ctx)
		if event.RouteID() != "" || len(event.params) != 0 || RequestLocals[struct{}](ctx) != nil {
			t.Errorf("reroute fetch acquired post-route event: %#v", event)
		}
		preRouteFetch.Add(1)
		return next(r)
	}}.Intercept(stack)
	for _, test := range []struct{ path, want string }{
		{"/alias?q=one", "route=/target/[id]|id=73|url=/alias?q=one"},
		{"/alias/__data.json?q=one&x-sveltekit-invalidated=1", "route=/target/[id]|id=73|url=/alias?q=one"},
		{"/api-alias?q=two", "route=/api/[id]|id=91|url=/api-alias?q=two"},
		{"/target/42?q=direct", "route=/target/[id]|id=42|url=/target/42?q=direct"},
	} {
		rec := get(t, app, test.path)
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), test.want) {
			t.Fatalf("%s status=%d body=%s want %s", test.path, rec.Code, rec.Body.String(), test.want)
		}
	}
	if preRouteFetch.Load() != 2 {
		t.Fatalf("pre-route fetch calls %d, want 2", preRouteFetch.Load())
	}
	for _, path := range []string{"/failure", "/panic"} {
		before := handled.Load()
		rec := get(t, app, path)
		if rec.Code != 500 || rec.Body.String() != "Internal Server Error" || handled.Load() != before {
			t.Fatalf("reroute failure %s status=%d body=%s hook calls before=%d after=%d", path, rec.Code, rec.Body.String(), before, handled.Load())
		}
	}
	missing := get(t, app, "/unmapped")
	if missing.Code != 404 || !strings.Contains(missing.Body.String(), "404 page not found") {
		t.Fatalf("unmapped status=%d body=%s", missing.Code, missing.Body.String())
	}
	bridge := get(t, app, "/_app/reroute?url="+url.QueryEscape("http://app.test/api-alias?q=browser"))
	if bridge.Code != 200 || strings.TrimSpace(bridge.Body.String()) != `{"path":"/api/91"}` {
		t.Fatalf("Kit hook transport status=%d body=%s", bridge.Code, bridge.Body.String())
	}
}

func TestRerouteSkipsRemoteAndRouteIDResolution(t *testing.T) {
	var calls atomic.Int32
	cfg := HandleConfig{Reroute: func(context.Context, RerouteEvent) (*string, error) { calls.Add(1); return nil, nil }}
	app := Middleware(nil).Intercept(cfg, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ordinary-control") }))
	for _, path := range []string{"/_app/remote/hash/name", "/_app/routes/target/[id]/__route.js"} {
		rec := get(t, app, path)
		if rec.Code != 200 || rec.Body.String() != "ordinary-control" || calls.Load() != 0 {
			t.Fatalf("%s status=%d body=%s calls=%d", path, rec.Code, rec.Body.String(), calls.Load())
		}
	}
	rec := get(t, app, "/target/__route.js")
	if rec.Code != 200 || calls.Load() != 1 {
		t.Fatalf("pathname route resolution skipped reroute: %d calls=%d", rec.Code, calls.Load())
	}
}

type rerouteRoundTrip func(*http.Request) (*http.Response, error)

func (f rerouteRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRerouteFetchObservesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered := make(chan struct{})
	var cancelled atomic.Bool
	hook := Reroute(func(ctx context.Context, event RerouteEvent) (*string, error) {
		go func() { <-entered; cancel() }()
		request, _ := http.NewRequestWithContext(ctx, "GET", "https://lookup.example/target", nil)
		_, err := event.Fetch(request)
		return nil, err
	})
	cfg := HandleConfig{Origin: "http://app.test", Reroute: hook}
	client := &http.Client{Timeout: time.Second, Transport: rerouteRoundTrip(func(r *http.Request) (*http.Response, error) {
		close(entered)
		<-r.Context().Done()
		err := r.Context().Err()
		cancelled.Store(errors.Is(err, context.Canceled))
		return nil, err
	})}
	app := FetchConfig{Origin: "http://app.test", Client: client}.Intercept(Middleware(nil).Intercept(cfg, http.NotFoundHandler()))
	request := httptest.NewRequest("GET", "/alias", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, request)
	if rec.Code != 500 || rec.Body.String() != "Internal Server Error" || !cancelled.Load() {
		t.Fatalf("cancelled reroute status=%d body=%s cancellation propagated=%v", rec.Code, rec.Body.String(), cancelled.Load())
	}
}
