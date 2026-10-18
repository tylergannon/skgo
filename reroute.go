package skgo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
)

// Reroute selects a pathname before routing and handle run. Return nil to keep
// the pathname, or a pointer to the replacement pathname. It changes route and
// params, never the request URL. A returned error or panic produces Kit's opaque
// 500 response. Configure the same function on HandleConfig and export the
// adapter's reroute from src/hooks.ts for Kit client navigation.
//
// Like Kit's universal hook, it must be pure and idempotent: the client caches
// its result per full URL. Go calls it synchronously with request cancellation;
// the function may wait for its Fetch or other Go work before returning.
type Reroute func(context.Context, RerouteEvent) (*string, error)

// RerouteEvent has the pre-routing URL and fetch Kit supplies to reroute. URL
// is a copy; changing it does not change the request. Fetch resolves relative
// requests against that original URL. Its HandleFetch event has no route,
// params or initialized application locals because handle has not run yet.
type RerouteEvent struct {
	URL   *url.URL
	Fetch Fetch
}

type reroutePathKey struct{}
type rerouteHookKey struct{}

func runReroute(ctx context.Context, hook Reroute, request *http.Request, original *url.URL) (result *string, err error) {
	if hook == nil {
		return nil, nil
	}
	defer func() {
		if value := recover(); value != nil {
			err = fmt.Errorf("skgo: reroute panicked: %v", value)
		}
	}()
	copied := *original
	event := &Event{req: request, metadata: requestMetadataOf(request, original, "", false), jar: newCookieJarAt(request, original.Host, original.Path, secureCookieDefault(original.Scheme+"://"+original.Host))}
	return hook(withEvent(ctx, event), RerouteEvent{URL: &copied, Fetch: func(r *http.Request) (*http.Response, error) { return event.Fetch(ctx, r) }})
}

func rerouteBridgePath(base, appDir string) string {
	if appDir == "" {
		appDir = "_app"
	}
	return base + "/" + appDir + "/reroute"
}

func (cfg HandleConfig) serveReroute(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		w.Header().Set("Allow", "GET")
		http.Error(w, "Method Not Allowed", 405)
		return
	}
	target, err := url.Parse(r.URL.Query().Get("url"))
	if err != nil || target.Scheme == "" || target.Host == "" {
		http.Error(w, "Bad Request", 400)
		return
	}
	source := r.Clone(r.Context())
	source.URL = target
	result, err := runReroute(r.Context(), cfg.Reroute, source, target)
	if err != nil {
		rerouteFailure(w)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, map[string]*string{"path": result})
}

// The pre-route build callback has no request handle yet. Its fetch dispatcher
// uses the same Go endpoints and request boundary as other build subrequests.
func (s *prerenderRequests) reroute(ctx context.Context, raw []byte, out io.Writer) error {
	var input struct {
		URL      string      `json:"url"`
		Headers  http.Header `json:"headers"`
		Manifest Manifest    `json:"manifest"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return err
	}
	target, err := url.Parse(input.URL)
	if err != nil || target.Host == "" {
		return fmt.Errorf("skgo: invalid reroute URL")
	}
	cfg := input.Manifest.HandleConfig()
	cfg.Origin = target.Scheme + "://" + target.Host
	cfg.Reroute = s.options.Reroute
	cfg.Matchers = s.options.Matchers
	endpoints, err := NewEndpoints(EndpointConfig{Origin: cfg.Origin, Base: cfg.Base, AppDir: cfg.AppDir, Routes: input.Manifest.Routes, Matchers: s.options.Matchers}, s.options.Endpoints...)
	if err != nil {
		return err
	}
	var app http.Handler
	stack := s.options.BindRequest(endpoints.Intercept(http.NotFoundHandler()))
	app = FetchConfig{Origin: cfg.Origin, Base: cfg.Base, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { app.ServeHTTP(w, r) })}.Intercept(Middleware(nil).Intercept(cfg, stack))
	request, err := http.NewRequestWithContext(ctx, "GET", cfg.Origin+rerouteBridgePath(cfg.Base, cfg.AppDir)+"?url="+url.QueryEscape(input.URL), nil)
	if err != nil {
		return err
	}
	request.Header = input.Headers
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, request)
	if rec.Code != 200 {
		return fmt.Errorf("skgo: reroute failed (%d)", rec.Code)
	}
	_, err = out.Write(rec.Body.Bytes())
	return err
}

func rerouteFailure(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain;charset=UTF-8")
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = io.WriteString(w, "Internal Server Error")
}
