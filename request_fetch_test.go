package skgo

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tylergannon/skgo/internal/ssr"
)

// These tests drive Event.Fetch through a composed handler built the way the
// binary composes one: FetchConfig over Handle over the load and endpoint
// registries. Every destination, credential and expected value below is a
// literal written in this file, never read back from the code under test.

type whoLocal string
type middlewareLocal string

// seen is what the echo endpoint reports about the request it was handed.
type seen struct {
	Method         string `json:"method"`
	Path           string `json:"path"`
	Query          string `json:"query"`
	Cookie         string `json:"cookie"`
	Authorization  string `json:"authorization"`
	Origin         string `json:"origin"`
	Accept         string `json:"accept"`
	AcceptLanguage string `json:"acceptLanguage"`
	Custom         string `json:"custom"`
	Body           string `json:"body"`
	Local          string `json:"local"`
	HasLocal       bool   `json:"hasLocal"`
}

type externalRequest struct {
	Host, Method, Path, Cookie, Authorization, Origin, Accept, AcceptLanguage string
}

// externalService stands in for every host outside the app. Its client sends
// every connection to one local listener whatever host the URL names, so the
// destination host a request carries is observable and nothing leaves the
// machine.
type externalService struct {
	server   *httptest.Server
	mu       sync.Mutex
	requests []externalRequest
}

func newExternalService(t *testing.T) *externalService {
	t.Helper()
	e := &externalService{}
	e.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		e.mu.Lock()
		e.requests = append(e.requests, externalRequest{
			Host: r.Host, Method: r.Method, Path: r.URL.RequestURI(),
			Cookie: r.Header.Get("Cookie"), Authorization: r.Header.Get("Authorization"),
			Origin: r.Header.Get("Origin"), Accept: r.Header.Get("Accept"),
			AcceptLanguage: r.Header.Get("Accept-Language"),
		})
		e.mu.Unlock()
		switch r.URL.Path {
		case "/ext/binary":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte{0x00, 0xff, 0xfe, 0x80})
		case "/ext/hang":
			<-r.Context().Done()
		default:
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Header().Set("X-External", "yes")
			w.Header().Add("Set-Cookie", "ext=leak; Path=/")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte("external text"))
		}
	}))
	t.Cleanup(e.server.Close)
	return e
}

func (e *externalService) client() *http.Client {
	dialer := &net.Dialer{}
	return &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, e.server.Listener.Addr().String())
		},
	}}
}

func (e *externalService) last(t *testing.T) externalRequest {
	t.Helper()
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.requests) == 0 {
		t.Fatal("the external service received no request")
	}
	return e.requests[len(e.requests)-1]
}

func (e *externalService) count() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.requests)
}

type fetchApp struct {
	handler  http.Handler
	external *externalService

	// loadFetch is the body of the load behind /a/b; each test sets it.
	loadFetch func(ctx context.Context, e *Event) (loaded, error)

	hookCalls    atomic.Int32
	echoHits     atomic.Int32
	slowEntered  chan struct{}
	slowCanceled atomic.Bool

	hookMu                                            sync.Mutex
	hookWho, hookLocal, hookURL, hookCookie, hookAuth string
}

type loaded struct {
	Status int    `json:"status"`
	Body   string `json:"body"`
	Header string `json:"header"`
	Error  string `json:"error"`
}

type fetchAppOptions struct {
	base     string
	hook     func(app *fetchApp) HandleFetch
	noHandle bool
	mwFetch  bool
}

// fetchBody drains and closes a fetch response, the way every caller must.
func fetchBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading a fetch response: %v", err)
	}
	return string(b)
}

func newFetchApp(t *testing.T, o fetchAppOptions) *fetchApp {
	t.Helper()
	app := &fetchApp{external: newExternalService(t), slowEntered: make(chan struct{}, 4)}
	const origin = "http://app.test"
	routes := []ManifestRoute{{ID: "/a/b", Pattern: `^\/a\/b\/?$`, Page: &ManifestPage{Layouts: []int{0}, Leaf: 1}}}
	for _, ep := range []struct {
		id      string
		methods []string
	}{
		{"/api/echo", []string{"GET", "POST", "DELETE"}}, {"/a/echo", []string{"GET"}},
		{"/api/teapot", []string{"GET"}}, {"/api/slow", []string{"GET"}},
		{"/api/relay", []string{"GET", "POST"}}, {"/api/boom", []string{"GET"}}, {"/api/mw", []string{"GET"}},
		{"/api/cookies", []string{"GET"}}, {"/a/cookies", []string{"GET"}}, {"/cookies", []string{"GET"}},
		{"/pre/[slug]", []string{"GET"}},
	} {
		routes = append(routes, ManifestRoute{
			ID: ep.id, Pattern: `^` + strings.ReplaceAll(strings.ReplaceAll(ep.id, "/", `\/`), "[slug]", `[^\/]+`) + `\/?$`,
			Endpoint: &ManifestEndpoint{Methods: ep.methods},
		})
	}
	m := Manifest{AppDir: "_app", Base: o.base, Nodes: []string{"", "src/routes/a/b/+page.server.ts"}, Routes: routes,
		Prerendered: []string{o.base + "/pre/static", o.base + "/pre/hello"}}

	loadCfg := m.LoadConfig(origin)
	loadCfg.Dev = true
	loads, err := NewLoads(loadCfg, NewLoad("src/routes/a/b/+page.server.ts", func(ctx context.Context) (loaded, error) {
		return app.loadFetch(ctx, EventFrom(ctx))
	}))
	if err != nil {
		t.Fatal(err)
	}

	epCfg := m.EndpointConfig(origin)
	epCfg.Dev = true
	epCfg.OnPanic = func(string, string, any, []byte) {}
	epCfg.HandleError = func(ctx context.Context, caught CaughtError) map[string]any {
		resp, err := EventFrom(ctx).Fetch(ctx, mustRequest(t, "GET", "/api/echo?from=handleError", nil))
		if err != nil {
			return map[string]any{"message": "fetch failed: " + err.Error()}
		}
		var s seen
		_ = json.Unmarshal([]byte(fetchBody(t, resp)), &s)
		return map[string]any{"message": "handled via " + s.Path + " cookie=" + s.Cookie}
	}
	endpoints, err := NewEndpoints(epCfg,
		NewEndpoint("/api/echo", "GET", app.echo), NewEndpoint("/api/echo", "POST", app.echo),
		NewEndpoint("/api/echo", "DELETE", app.echo), NewEndpoint("/a/echo", "GET", app.echo),
		NewEndpoint("/api/teapot", "GET", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("X-Tea", "pot")
			w.WriteHeader(http.StatusTeapot)
			_, _ = w.Write([]byte{0x00, 0xff, 0x7f, 'A'})
		}),
		NewEndpoint("/api/slow", "GET", func(w http.ResponseWriter, r *http.Request) {
			app.slowEntered <- struct{}{}
			<-r.Context().Done()
			app.slowCanceled.Store(true)
		}),
		NewEndpoint("/api/relay", "GET", app.relay), NewEndpoint("/api/relay", "POST", app.relay),
		NewEndpoint("/api/boom", "GET", func(w http.ResponseWriter, r *http.Request) { panic("boom") }),
		NewEndpoint("/api/cookies", "GET", app.cookies), NewEndpoint("/a/cookies", "GET", app.cookies),
		NewEndpoint("/cookies", "GET", app.cookies),
		NewEndpoint("/pre/[slug]", "GET", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("dynamic-endpoint"))
		}),
		NewEndpoint("/api/mw", "GET", func(w http.ResponseWriter, r *http.Request) {
			v := fetchLocal(r.Context()).Middleware
			_, _ = w.Write([]byte(v))
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	handleCfg := m.HandleConfig()
	handle := Handle(func(ctx context.Context) error {
		e := EventFrom(ctx)
		who := e.Request().Header.Get("X-Who")
		if who == "" {
			who = "subrequest-default"
		}
		RequestLocals[fetchLocals](ctx).Who = whoLocal(who)
		RequestLocals[fetchLocals](ctx).HasWho = true
		if target := e.Request().Header.Get("X-Middleware-Cookie"); target != "" {
			resp, err := e.Fetch(ctx, mustRequest(t, "GET", target, nil))
			if err != nil {
				return err
			}
			_ = fetchBody(t, resp)
		}
		if e.Request().Header.Get("X-Middleware-Fetch") != "" {
			resp, err := e.Fetch(ctx, mustRequest(t, "GET", "/api/echo?from=handle", nil))
			if err != nil {
				return err
			}
			RequestLocals[fetchLocals](ctx).Middleware = middlewareLocal(fetchBody(t, resp))
			return nil
		}
		return nil
	})

	var hook HandleFetch
	if o.hook != nil {
		hook = o.hook(app)
	}
	// The same in-process handler SSROptions.Fetch takes: the whole stack,
	// Handle outermost when it is mounted at all, late-bound by reference.
	var stack http.Handler
	inner := http.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { stack.ServeHTTP(w, r) }))
	stack = loads.Intercept(endpoints.Intercept(staticStandIn()))
	outer := stack
	if o.noHandle {
		// A stand-in for an outer request that has locals of its own, so a
		// subrequest that wrongly inherited them would be visible.
		parent := outer
		outer = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			l := &fetchLocals{Who: "outer-user", HasWho: true}
			parent.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), localsKey{}, l)))
		})
	} else {
		stack = RequestMiddleware[struct{}, fetchLocals](nil).Intercept(handleCfg, emptyHookParams, handle.Intercept(handleCfg, stack))
		outer = stack
	}
	app.handler = FetchConfig{
		Origin: origin, Base: o.base, Handler: inner, HandleFetch: hook, Client: app.external.client(),
		Prerendered: m.Prerendered,
	}.Intercept(outer)
	return app
}

// staticStandIn is the part of the stack the static handler is: prerendered
// files, a public asset, and a page for everything else. It reports what the
// request it received carried.
func staticStandIn() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Saw-Cookie", r.Header.Get("Cookie"))
		w.Header().Set("X-Saw-Auth", r.Header.Get("Authorization"))
		w.Header().Set("X-Saw-Accept", r.Header.Get("Accept"))
		w.Header().Set("X-Remote", r.RemoteAddr)
		switch strings.TrimPrefix(r.URL.Path, "/app") {
		case "/pre/static", "/pre/hello":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("X-Static", "prerendered")
			w.Header().Add("Set-Cookie", "leak=prerendered; Path=/")
			_, _ = w.Write([]byte("prerendered:" + strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/app"), "/pre/")))
		case "/robots.txt":
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write([]byte("User-agent: *\nDisallow: /private\n"))
		default:
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte("page"))
		}
	})
}

// cookies answers with a Set-Cookie header for every `set` parameter, exactly
// as written, and reports the Cookie header it received.
func (app *fetchApp) cookies(w http.ResponseWriter, r *http.Request) {
	for _, v := range r.URL.Query()["set"] {
		w.Header().Add("Set-Cookie", v)
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("cookie-header=" + r.Header.Get("Cookie")))
}

func mustRequest(t *testing.T, method, target string, body io.Reader) *http.Request {
	t.Helper()
	r, err := http.NewRequest(method, target, body)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// echo reports what the subrequest looked like to the handler that answered it.
func (app *fetchApp) echo(w http.ResponseWriter, r *http.Request) {
	app.echoHits.Add(1)
	var body []byte
	if r.Body != nil {
		body, _ = io.ReadAll(r.Body)
	}
	local, has := fetchLocal(r.Context()).Who, fetchLocal(r.Context()).HasWho
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Echo", "header-"+r.Method)
	w.Header().Set("X-Remote", r.RemoteAddr)
	_ = json.NewEncoder(w).Encode(seen{
		Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery,
		Cookie: r.Header.Get("Cookie"), Authorization: r.Header.Get("Authorization"),
		Origin: r.Header.Get("Origin"), Accept: r.Header.Get("Accept"),
		AcceptLanguage: r.Header.Get("Accept-Language"), Custom: r.Header.Get("X-Explicit"),
		Body: string(body), Local: string(local), HasLocal: has,
	})
}

// relay is a server route that fetches whatever its query names with Event.Fetch
// and answers with exactly what came back: status, headers and bytes.
//
//	to=<url>   destination        m=<method>   b=<body>
//	omit=1     credentials: omit  h.<Name>=<v> explicit request header
func (app *fetchApp) relay(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	method := q.Get("m")
	if method == "" {
		method = http.MethodGet
	}
	var body io.Reader
	if b := q.Get("b"); b != "" {
		body = strings.NewReader(b)
	}
	targets := q["to"]
	var opts []FetchOption
	if q.Get("omit") != "" {
		opts = append(opts, WithFetchCredentials(FetchOmit))
	}
	for _, earlier := range targets[:len(targets)-1] {
		req, err := http.NewRequest(http.MethodGet, earlier, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		resp, err := EventFrom(r.Context()).Fetch(r.Context(), req, opts...)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	req, err := http.NewRequest(method, targets[len(targets)-1], body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for name, values := range q {
		if strings.HasPrefix(name, "h.") {
			req.Header.Set(strings.TrimPrefix(name, "h."), values[0])
		}
	}
	resp, err := EventFrom(r.Context()).Fetch(r.Context(), req, opts...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	local := fetchLocal(r.Context()).Who
	w.Header().Set("X-Relay-Local", string(local))
	for _, name := range []string{"Content-Type", "X-Echo", "X-Tea", "X-External", "X-Hook", "X-Saw-Cookie", "X-Saw-Auth", "X-Saw-Accept", "X-Remote", "X-Static"} {
		if v := resp.Header.Get(name); v != "" {
			w.Header().Set(name, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (app *fetchApp) serve(method, target string, header http.Header) *httptest.ResponseRecorder {
	return app.serveCtx(context.Background(), method, target, header)
}

func (app *fetchApp) serveCtx(ctx context.Context, method, target string, header http.Header) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil).WithContext(ctx)
	req.Host = "app.test"
	for k, vs := range header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	rec := httptest.NewRecorder()
	app.handler.ServeHTTP(rec, req)
	return rec
}

// loadResult serves the page's `__data.json` and returns the load's fields,
// resolved from kit's devalue array, plus the raw `uses` object.
func (app *fetchApp) loadResult(t *testing.T, base string, header http.Header) (loaded, string) {
	t.Helper()
	rec := app.serve("GET", base+"/a/b/__data.json?x-sveltekit-invalidated=01", header)
	if rec.Code != http.StatusOK {
		t.Fatalf("__data.json status = %d: %s", rec.Code, rec.Body.String())
	}
	var envelope struct {
		Nodes []struct {
			Type string            `json:"type"`
			Data []json.RawMessage `json:"data"`
			Uses json.RawMessage   `json:"uses"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal([]byte(strings.SplitN(rec.Body.String(), "\n", 2)[0]), &envelope); err != nil {
		t.Fatalf("decoding the data response %q: %v", rec.Body.String(), err)
	}
	node := envelope.Nodes[1]
	var shape map[string]int
	if err := json.Unmarshal(node.Data[0], &shape); err != nil {
		t.Fatalf("decoding the data shape: %v", err)
	}
	var out loaded
	_ = json.Unmarshal(node.Data[shape["status"]], &out.Status)
	_ = json.Unmarshal(node.Data[shape["body"]], &out.Body)
	_ = json.Unmarshal(node.Data[shape["header"]], &out.Header)
	_ = json.Unmarshal(node.Data[shape["error"]], &out.Error)
	return out, string(node.Uses)
}

func (app *fetchApp) loadFetching(target string, opts ...FetchOption) {
	app.loadFetch = func(ctx context.Context, e *Event) (loaded, error) {
		resp, err := e.Fetch(ctx, mustRequestNoT("GET", target), opts...)
		if err != nil {
			return loaded{Error: err.Error()}, nil
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return loaded{Status: resp.StatusCode, Body: string(b), Header: resp.Header.Get("X-Echo") + resp.Header.Get("X-External") + resp.Header.Get("X-Hook")}, nil
	}
}

func mustRequestNoT(method, target string) *http.Request {
	r, err := http.NewRequest(method, target, nil)
	if err != nil {
		panic(err)
	}
	return r
}

func decodeSeen(t *testing.T, body string) seen {
	t.Helper()
	var s seen
	if err := json.Unmarshal([]byte(body), &s); err != nil {
		t.Fatalf("not an echo body: %q (%v)", body, err)
	}
	return s
}

var outerHeaders = http.Header{
	"Cookie":          {"session=abc123; theme=light"},
	"Authorization":   {"Bearer outer-secret"},
	"Accept-Language": {"fr-CA"},
	"X-Who":           {"outer-user"},
}

func TestLoadFetchesRelativeEndpointInProcess(t *testing.T) {
	app := newFetchApp(t, fetchAppOptions{})
	// The page is /a/b, so "echo" is a sibling of it and "../api/echo?q=7" climbs
	// out; the data request's own URL (/a/b/__data.json) must not be the base.
	for _, tc := range []struct{ target, path, query string }{
		{"echo", "/a/echo", ""},
		{"../api/echo?q=7", "/api/echo", "q=7"},
		{"/api/echo?q=8", "/api/echo", "q=8"},
		{"http://app.test/api/echo", "/api/echo", ""},
	} {
		app.loadFetching(tc.target)
		got, uses := app.loadResult(t, "", outerHeaders)
		if got.Status != 200 || got.Header != "header-GET" {
			t.Errorf("%s: status %d header %q, want 200 and the endpoint's own X-Echo", tc.target, got.Status, got.Header)
		}
		s := decodeSeen(t, got.Body)
		if s.Path != tc.path || s.Query != tc.query || s.Method != "GET" {
			t.Errorf("%s: endpoint saw %s %s?%s, want GET %s?%s", tc.target, s.Method, s.Path, s.Query, tc.path, tc.query)
		}
		// Kit adds no dependency for a server load's fetch.
		if uses != `{}` {
			t.Errorf("%s: a fetch recorded dependencies %s; kit records none for server loads", tc.target, uses)
		}
	}
	if app.external.count() != 0 {
		t.Error("a same-origin fetch went out over HTTP")
	}
}

func TestEndpointFetchesAnotherEndpointAndKeepsStatusHeadersAndBytes(t *testing.T) {
	app := newFetchApp(t, fetchAppOptions{})

	rec := app.serve("GET", "/api/relay?to=/api/teapot", nil)
	if rec.Code != http.StatusTeapot {
		t.Errorf("status = %d, want 418", rec.Code)
	}
	if got := rec.Header().Get("X-Tea"); got != "pot" {
		t.Errorf("X-Tea = %q, want pot", got)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/octet-stream" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := rec.Body.Bytes(); string(got) != string([]byte{0x00, 0xff, 0x7f, 'A'}) {
		t.Errorf("body = % x, want 00 ff 7f 41", got)
	}

	rec = app.serve("GET", "/api/relay?to=/api/echo?via=relay", nil)
	if rec.Code != 200 || decodeSeen(t, rec.Body.String()).Query != "via=relay" {
		t.Errorf("relay of echo = %d %s", rec.Code, rec.Body.String())
	}
}

func TestInternalFetchInheritsCredentialsAsKitDoes(t *testing.T) {
	app := newFetchApp(t, fetchAppOptions{})

	rec := app.serve("GET", "/api/relay?to=/api/echo", outerHeaders)
	s := decodeSeen(t, rec.Body.String())
	want := seen{
		Method: "GET", Path: "/api/echo",
		Cookie: "session=abc123; theme=light", Authorization: "Bearer outer-secret",
		Accept: "*/*", AcceptLanguage: "fr-CA",
		// A same-origin GET carries no Origin. The subrequest has no X-Who, so
		// its own Handle gave it its own local.
		Origin: "", Local: "subrequest-default", HasLocal: true,
	}
	if s != want {
		t.Errorf("subrequest saw\n%+v\nwant\n%+v", s, want)
	}
	// The originating request's local is untouched by the subrequest's Handle.
	if got := rec.Header().Get("X-Relay-Local"); got != "outer-user" {
		t.Errorf("the relay's own local became %q after fetching", got)
	}

	// A same-origin POST does carry Origin.
	rec = app.serve("GET", "/api/relay?to=/api/echo&m=POST&b=payload", outerHeaders)
	s = decodeSeen(t, rec.Body.String())
	if s.Method != "POST" || s.Body != "payload" || s.Origin != "http://app.test" {
		t.Errorf("POST subrequest = %+v, want method POST, body payload, Origin http://app.test", s)
	}
}

func TestCredentialsOmitInheritsNoCookiesOrAuthorization(t *testing.T) {
	app := newFetchApp(t, fetchAppOptions{})

	rec := app.serve("GET", "/api/relay?to=/api/echo&omit=1", outerHeaders)
	s := decodeSeen(t, rec.Body.String())
	if s.Cookie != "" || s.Authorization != "" {
		t.Errorf("credentials: omit leaked cookie %q / authorization %q", s.Cookie, s.Authorization)
	}
	// Accept-Language is not a credential and is inherited either way.
	if s.AcceptLanguage != "fr-CA" || s.Accept != "*/*" {
		t.Errorf("accept-language %q accept %q, want fr-CA and */*", s.AcceptLanguage, s.Accept)
	}
}

func TestExplicitHeadersBeatInheritedOnes(t *testing.T) {
	app := newFetchApp(t, fetchAppOptions{})

	target := "/api/relay?to=/api/echo&h.Cookie=session%3Dexplicit&h.Authorization=Bearer+mine&h.Accept-Language=de&h.Accept=text/plain&h.X-Explicit=yes"
	s := decodeSeen(t, app.serve("GET", target, outerHeaders).Body.String())
	// The explicit Cookie header has the highest precedence but does not hide
	// the cookies it does not mention.
	want := seen{
		Method: "GET", Path: "/api/echo", Cookie: "session=explicit; theme=light",
		Authorization: "Bearer mine", Accept: "text/plain", AcceptLanguage: "de", Custom: "yes",
		Local: "subrequest-default", HasLocal: true,
	}
	if s != want {
		t.Errorf("subrequest saw\n%+v\nwant\n%+v", s, want)
	}
}

func TestCookiesWrittenByTheLoadReachTheSubrequestInKitsOrder(t *testing.T) {
	app := newFetchApp(t, fetchAppOptions{})
	app.loadFetch = func(ctx context.Context, e *Event) (loaded, error) {
		for _, c := range []struct {
			name, value string
			opts        CookieOptions
		}{
			{"pref", "dark", CookieOptions{}},
			{"theme", "dark", CookieOptions{}},                       // replaces the browser's
			{"scoped", "no", CookieOptions{Path: "/private"}},        // not for /api/echo
			{"elsewhere", "no", CookieOptions{Domain: "other.test"}}, // not for app.test
		} {
			if err := e.SetCookie(c.name, c.value, c.opts); err != nil {
				return loaded{}, err
			}
		}
		resp, err := e.Fetch(ctx, mustRequestNoT("GET", "/api/echo"))
		if err != nil {
			return loaded{}, err
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return loaded{Status: resp.StatusCode, Body: string(b)}, nil
	}
	got, _ := app.loadResult(t, "", outerHeaders)
	if cookie := decodeSeen(t, got.Body).Cookie; cookie != "session=abc123; theme=dark; pref=dark" {
		t.Errorf("subrequest cookie = %q, want %q", cookie, "session=abc123; theme=dark; pref=dark")
	}
}

func TestExternalFetchUsesGoHTTPAndLeaksNothing(t *testing.T) {
	app := newFetchApp(t, fetchAppOptions{})
	app.loadFetching("http://svc.other.test/ext/text?x=1")

	got, _ := app.loadResult(t, "", outerHeaders)
	if got.Status != 201 || got.Body != "external text" || got.Header != "yes" {
		t.Errorf("external answer = %+v, want 201 / external text / X-External yes", got)
	}
	r := app.external.last(t)
	want := externalRequest{Host: "svc.other.test", Method: "GET", Path: "/ext/text?x=1", Origin: "http://app.test"}
	if r != want {
		t.Errorf("the external service saw\n%+v\nwant\n%+v (no cookies, no Authorization, no Accept-Language)", r, want)
	}
	if app.echoHits.Load() != 0 {
		t.Error("an external URL reached an in-process handler")
	}
}

func TestExternalBinaryBodyIsIntact(t *testing.T) {
	app := newFetchApp(t, fetchAppOptions{})
	rec := app.serve("GET", "/api/relay?to=http://svc.other.test/ext/binary", nil)
	if rec.Code != 200 || string(rec.Body.Bytes()) != string([]byte{0x00, 0xff, 0xfe, 0x80}) {
		t.Errorf("binary relay = %d % x", rec.Code, rec.Body.Bytes())
	}
}

func TestCookiesFollowKitsDomainRulesOnExternalRequests(t *testing.T) {
	app := newFetchApp(t, fetchAppOptions{})
	const inherited = "session=abc123; theme=light"
	for _, tc := range []struct {
		destination, cookie string
		omit                bool
		why                 string
	}{
		{"http://app.test:9000/ext/text", inherited, false, "same host, another port: ports do not matter"},
		{"http://api.app.test/ext/text", inherited, false, "a subdomain receives them"},
		{"http://a.b.app.test/ext/text", inherited, false, "any depth of subdomain receives them"},
		{"http://api.app.test/ext/text", "", true, "credentials: omit stops them"},
		{"http://other.test/ext/text", "", false, "an unrelated host never does"},
		{"http://notapp.test/ext/text", "", false, "a leading dot keeps notapp.test from matching app.test"},
		{"http://app.test.evil.test/ext/text", "", false, "a host that merely starts with ours is not ours"},
		{"http://test/ext/text", "", false, "a parent domain does not"},
	} {
		target := "/api/relay?to=" + url.QueryEscape(tc.destination)
		if tc.omit {
			target += "&omit=1"
		}
		before := app.external.count()
		rec := app.serve("GET", target, outerHeaders)
		if rec.Code != 201 {
			t.Fatalf("%s: relay status %d: %s", tc.destination, rec.Code, rec.Body.String())
		}
		if app.external.count() != before+1 {
			t.Fatalf("%s: expected one external request", tc.destination)
		}
		r := app.external.last(t)
		if r.Cookie != tc.cookie {
			t.Errorf("%s (%s): cookie = %q, want %q", tc.destination, tc.why, r.Cookie, tc.cookie)
		}
		if r.Authorization != "" {
			t.Errorf("%s: Authorization %q reached an external host; Kit inherits it only for in-app requests", tc.destination, r.Authorization)
		}
	}

	// An explicit Cookie header is the caller's own business and goes through.
	app.serve("GET", "/api/relay?to=http://other.test/ext/text&h.Cookie=mine%3D1", outerHeaders)
	if got := app.external.last(t).Cookie; got != "mine=1" {
		t.Errorf("explicit cookie to an unrelated host = %q, want mine=1 and nothing inherited", got)
	}
}

func TestConfiguredBaseDecidesWhatIsInternal(t *testing.T) {
	app := newFetchApp(t, fetchAppOptions{base: "/app"})
	for _, tc := range []struct {
		target   string
		external bool
		status   int
	}{
		{"/app/api/echo", false, 200},
		{"/app", false, 200},         // the base itself is the app's: its page answers
		{"/app/nothing", false, 200}, // a page of the app
		{"/other/path", true, 201},
		{"/apple/pie", true, 201}, // a prefix of the base is not under it
		{"http://app.test/outside", true, 201},
	} {
		app.loadFetching(tc.target)
		before, hits := app.external.count(), app.echoHits.Load()
		got, _ := app.loadResult(t, "/app", outerHeaders)
		if got.Status != tc.status {
			t.Errorf("%s: status %d, want %d (error %q)", tc.target, got.Status, tc.status, got.Error)
		}
		if wentOut := app.external.count() == before+1; wentOut != tc.external {
			t.Errorf("%s: left the process = %v, want %v", tc.target, wentOut, tc.external)
		}
		if tc.target == "/app/api/echo" && app.echoHits.Load() != hits+1 {
			t.Errorf("%s: the app's own endpoint was not called", tc.target)
		}
	}
	// A same-origin URL outside the base is still this host: it gets the cookies
	// but not Authorization, and no Origin on a GET.
	app.loadFetching("/other/path")
	app.loadResult(t, "/app", outerHeaders)
	want := externalRequest{Host: "app.test", Method: "GET", Path: "/other/path", Cookie: "session=abc123; theme=light"}
	if r := app.external.last(t); r != want {
		t.Errorf("outside-base request = %+v, want %+v", r, want)
	}
}

func TestHandleFetchSeesTheOriginalEventAndNextSkipsTheHook(t *testing.T) {
	app := newFetchApp(t, fetchAppOptions{hook: func(app *fetchApp) HandleFetch {
		return func(ctx context.Context, request *http.Request, next Fetch) (*http.Response, error) {
			app.hookCalls.Add(1)
			who := fetchLocal(ctx).Who
			app.hookMu.Lock()
			app.hookWho = EventFrom(ctx).Request().Header.Get("X-Who")
			app.hookLocal = string(who)
			app.hookURL = request.URL.String()
			app.hookCookie, app.hookAuth = request.Header.Get("Cookie"), request.Header.Get("Authorization")
			app.hookMu.Unlock()
			resp, err := next(request)
			if err == nil {
				resp.Header.Set("X-Hook", "ran")
			}
			return resp, err
		}
	}})
	app.loadFetching("/api/echo?hooked=1")
	got, _ := app.loadResult(t, "", outerHeaders)

	if n := app.hookCalls.Load(); n != 1 {
		t.Fatalf("the hook ran %d times for one fetch, want 1 (next must not re-enter it)", n)
	}
	app.hookMu.Lock()
	defer app.hookMu.Unlock()
	if app.hookWho != "outer-user" || app.hookLocal != "outer-user" {
		t.Errorf("the hook saw request X-Who %q / local %q, want the originating request's outer-user", app.hookWho, app.hookLocal)
	}
	if app.hookURL != "http://app.test/api/echo?hooked=1" {
		t.Errorf("the hook was handed %q, want the normalized absolute URL", app.hookURL)
	}
	if app.hookCookie != "" || app.hookAuth != "" {
		t.Errorf("the hook saw cookie %q / authorization %q: credentials are inherited after the hook, not before", app.hookCookie, app.hookAuth)
	}
	s := decodeSeen(t, got.Body)
	if s.Cookie != "session=abc123; theme=light" || s.Authorization != "Bearer outer-secret" {
		t.Errorf("after next() the subrequest saw cookie %q authorization %q, want them inherited", s.Cookie, s.Authorization)
	}
	if s.Local != "subrequest-default" {
		t.Errorf("subrequest local = %q: it shared the originating request's locals", s.Local)
	}
	if got.Header != "header-GETran" {
		t.Errorf("headers = %q, want the endpoint's X-Echo and the hook's X-Hook", got.Header)
	}
}

func TestHandleFetchRewriteIsAppliedBeforeCredentialInheritance(t *testing.T) {
	app := newFetchApp(t, fetchAppOptions{hook: func(app *fetchApp) HandleFetch {
		return func(ctx context.Context, request *http.Request, next Fetch) (*http.Response, error) {
			if request.URL.Path == "/api/alias" {
				request = request.Clone(ctx)
				request.URL.Scheme, request.URL.Host, request.URL.Path = "http", "svc.other.test", "/ext/text"
				request.Host = "svc.other.test"
			}
			return next(request)
		}
	}})
	app.loadFetching("/api/alias")
	got, _ := app.loadResult(t, "", outerHeaders)
	if got.Status != 201 || got.Body != "external text" {
		t.Fatalf("rewritten fetch = %+v, want the external service's answer", got)
	}
	r := app.external.last(t)
	if r.Host != "svc.other.test" || r.Cookie != "" || r.Authorization != "" {
		t.Errorf("rewritten request = %+v: it was an internal URL when the hook ran, but inheritance must follow the rewritten destination", r)
	}

	// A rewrite the other way: an external URL pulled inside inherits.
	app2 := newFetchApp(t, fetchAppOptions{hook: func(app *fetchApp) HandleFetch {
		return func(ctx context.Context, request *http.Request, next Fetch) (*http.Response, error) {
			request = request.Clone(ctx)
			request.URL.Scheme, request.URL.Host, request.URL.Path = "http", "app.test", "/api/echo"
			request.Host = "app.test"
			return next(request)
		}
	}})
	app2.loadFetching("http://svc.other.test/anything")
	got, _ = app2.loadResult(t, "", outerHeaders)
	if s := decodeSeen(t, got.Body); s.Path != "/api/echo" || s.Authorization != "Bearer outer-secret" {
		t.Errorf("an external URL rewritten into the app saw %+v, want the app's echo with inherited credentials", s)
	}
	if app2.external.count() != 0 {
		t.Error("the rewritten-inward fetch still went out over HTTP")
	}
}

func TestHandleFetchCanShortCircuit(t *testing.T) {
	app := newFetchApp(t, fetchAppOptions{hook: func(app *fetchApp) HandleFetch {
		return func(ctx context.Context, request *http.Request, next Fetch) (*http.Response, error) {
			app.hookCalls.Add(1)
			if request.URL.Path == "/api/shortcut" {
				return &http.Response{StatusCode: 202, Body: io.NopCloser(strings.NewReader("from the hook"))}, nil
			}
			return next(request)
		}
	}})
	app.loadFetching("/api/shortcut")
	got, _ := app.loadResult(t, "", outerHeaders)
	if got.Status != 202 || got.Body != "from the hook" {
		t.Errorf("short-circuited fetch = %+v, want 202 / from the hook", got)
	}
	if app.echoHits.Load() != 0 || app.external.count() != 0 {
		t.Error("a short-circuited fetch still reached a handler or the network")
	}
}

func TestHandleFetchErrorAndMissingResponseFailTheFetch(t *testing.T) {
	boom := errors.New("hook refused")
	app := newFetchApp(t, fetchAppOptions{hook: func(app *fetchApp) HandleFetch {
		return func(ctx context.Context, request *http.Request, next Fetch) (*http.Response, error) {
			if request.URL.Path == "/api/none" {
				return nil, nil
			}
			return nil, boom
		}
	}})
	app.loadFetching("/api/echo")
	if got, _ := app.loadResult(t, "", nil); got.Error != "hook refused" {
		t.Errorf("error = %q, want the hook's own error", got.Error)
	}
	app.loadFetching("/api/none")
	if got, _ := app.loadResult(t, "", nil); !strings.Contains(got.Error, "no response") {
		t.Errorf("error = %q, want a refusal of a nil response", got.Error)
	}
}

func TestSubrequestHasFreshLocalsWithoutAHandleHook(t *testing.T) {
	app := newFetchApp(t, fetchAppOptions{noHandle: true})
	// The outer request has a whoLocal of "outer-user"; nothing in the in-process
	// handler sets one, so the subrequest must see none rather than that.
	rec := app.serve("GET", "/api/relay?to=/api/echo", nil)
	s := decodeSeen(t, rec.Body.String())
	if s.HasLocal || s.Local != "" {
		t.Errorf("subrequest local = %q (has=%v): it inherited the originating request's locals", s.Local, s.HasLocal)
	}
	if got := rec.Header().Get("X-Relay-Local"); got != "outer-user" {
		t.Errorf("the originating request's local = %q after the fetch, want outer-user", got)
	}
}

func TestMiddlewareAndErrorHooksCanFetch(t *testing.T) {
	app := newFetchApp(t, fetchAppOptions{})

	h := http.Header{"Cookie": {"session=abc123"}, "X-Middleware-Fetch": {"1"}}
	rec := app.serve("GET", "/api/mw", h)
	s := decodeSeen(t, rec.Body.String())
	if s.Path != "/api/echo" || s.Query != "from=handle" || s.Cookie != "session=abc123" {
		t.Errorf("the Handle hook's fetch = %+v, want /api/echo?from=handle with the request's cookie", s)
	}

	rec = app.serve("GET", "/api/boom", http.Header{"Accept": {"application/json"}, "Cookie": {"session=abc123"}})
	if rec.Code != 500 || !strings.Contains(rec.Body.String(), "handled via /api/echo cookie=session=abc123") {
		t.Errorf("handleError fetch = %d %s", rec.Code, rec.Body.String())
	}
}

func TestFetchCancellation(t *testing.T) {
	app := newFetchApp(t, fetchAppOptions{})

	t.Run("an in-process handler is cancelled and the fetch returns promptly", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan *httptest.ResponseRecorder, 1)
		go func() { done <- app.serveCtx(ctx, "GET", "/api/relay?to=/api/slow", nil) }()
		select {
		case <-app.slowEntered:
		case <-time.After(5 * time.Second):
			t.Fatal("the slow endpoint was never reached")
		}
		cancel()
		select {
		case rec := <-done:
			if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "context canceled") {
				t.Errorf("relay = %d %q, want 502 with context canceled", rec.Code, rec.Body.String())
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Fetch did not return after its context was cancelled")
		}
		deadline := time.Now().Add(5 * time.Second)
		for !app.slowCanceled.Load() {
			if time.Now().After(deadline) {
				t.Fatal("the handler answering the fetch never saw the cancellation")
			}
			time.Sleep(5 * time.Millisecond)
		}
	})

	t.Run("an external request is cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan *httptest.ResponseRecorder, 1)
		go func() { done <- app.serveCtx(ctx, "GET", "/api/relay?to=http://svc.other.test/ext/hang", nil) }()
		deadline := time.Now().Add(5 * time.Second)
		for app.external.count() == 0 {
			if time.Now().After(deadline) {
				t.Fatal("the external request was never made")
			}
			time.Sleep(5 * time.Millisecond)
		}
		cancel()
		select {
		case rec := <-done:
			if rec.Code != http.StatusBadGateway {
				t.Errorf("relay = %d %q, want 502", rec.Code, rec.Body.String())
			}
		case <-time.After(5 * time.Second):
			t.Fatal("an external fetch ignored cancellation")
		}
	})

	t.Run("a cancelled context never reaches the handler", func(t *testing.T) {
		before := len(app.slowEntered)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		rec := app.serveCtx(ctx, "GET", "/api/relay?to=/api/slow", nil)
		if len(app.slowEntered) != before {
			t.Error("the handler ran for a fetch whose context was already cancelled")
		}
		_ = rec
	})
}

func TestEventFetchNeedsAConfiguredFetcher(t *testing.T) {
	var nilEvent *Event
	if _, err := nilEvent.Fetch(context.Background(), mustRequestNoT("GET", "/x")); err == nil {
		t.Error("a nil event fetched")
	}
	e := &Event{req: httptest.NewRequest("GET", "http://app.test/x", nil)}
	if _, err := e.Fetch(context.Background(), mustRequestNoT("GET", "/x")); err == nil || !strings.Contains(err.Error(), "FetchConfig.Intercept") {
		t.Errorf("an event with no FetchConfig mounted fetched, err = %v", err)
	}
}

// TestRenderFetchSharesTheSameRules proves the render-time bridge is the same
// capability: an external URL goes out over HTTP, and the page's credentials
// stay home.
func TestRenderFetchSharesTheSameRules(t *testing.T) {
	ext := newExternalService(t)
	s := &SSR{fetch: fetchFixture(), handleFetch: func(ctx context.Context, r *http.Request, next Fetch) (*http.Response, error) {
		resp, err := next(r)
		if err == nil {
			resp.Header.Set("X-Hook", "bridge")
		}
		return resp, err
	}}
	page, _ := url.Parse("http://app.test/page")
	parentRequest := httptest.NewRequest("GET", "http://app.test/page", nil)
	parentRequest.Header.Set("Cookie", "session=abc123")
	parentRequest.Header.Set("Authorization", "Bearer outer-secret")
	ctx := context.WithValue(context.Background(), fetchParentKey{}, fetchParent{request: parentRequest, url: page})

	// Aimed at the controlled service directly, as a different host.
	target := ext.server.URL + "/ext/text"
	answer := s.fetchAnswer(ctx, ssr.FetchRequest{Method: "GET", URL: target})
	if answer.Error != "" || answer.Response == nil {
		t.Fatalf("external render fetch = %+v", answer)
	}
	if answer.Response.Status != 201 || answer.Response.Body != "external text" || responseHeader(answer.Response, "x-hook") != "bridge" {
		t.Errorf("answer = %+v", answer.Response)
	}
	if r := ext.last(t); r.Cookie != "" || r.Authorization != "" {
		t.Errorf("render-time fetch leaked cookie %q / authorization %q to an unrelated origin", r.Cookie, r.Authorization)
	}

	// And the same-origin path still reaches the in-process handler with the cookie.
	answer = s.fetchAnswer(ctx, ssr.FetchRequest{Method: "GET", URL: "/api/anything"})
	if answer.Response == nil || responseHeader(answer.Response, "x-saw-cookie") != "session=abc123" {
		t.Errorf("same-origin render fetch = %+v, want the cookie inherited", answer)
	}
}

type fetchLocals struct {
	Who        whoLocal
	HasWho     bool
	Middleware middlewareLocal
}

func fetchLocal(ctx context.Context) fetchLocals {
	if p := RequestLocals[fetchLocals](ctx); p != nil {
		return *p
	}
	return fetchLocals{}
}
