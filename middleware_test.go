package skgo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// These tests drive the real composed handler — Middleware over Loads over
// Remotes over Endpoints over a page stand-in — and compare against literals
// the tests supply. The expected values come from the pinned kit's
// `runtime/server/respond.js` and `cookie.js`, not from the implementation.

const mwOrigin = "http://127.0.0.1:8080"

type mwUser string

func mwManifest(base string) Manifest {
	return Manifest{
		AppDir: "_app",
		Base:   base,
		Nodes:  []string{"", "src/routes/items/[id]/+page.server.ts", ""},
		Routes: []ManifestRoute{
			{ID: "/", Pattern: `^\/$`, Page: &ManifestPage{Layouts: []int{0}, Leaf: 2}},
			{
				ID: "/items/[id]", Pattern: `^\/items\/([^/]+?)\/?$`,
				Params: []ManifestParam{{Name: "id"}},
				Page:   &ManifestPage{Layouts: []int{0}, Leaf: 1},
			},
			{
				ID: "/api/items/[id]", Pattern: `^\/api\/items\/([^/]+?)\/?$`,
				Params:   []ManifestParam{{Name: "id"}},
				Endpoint: &ManifestEndpoint{Methods: []string{"GET"}},
			},
		},
	}
}

type mwStack struct {
	handler http.Handler
	remotes *Remotes
	cfg     HandleConfig
}

// mwBuild composes the stack. The item page's load reports what the cookie jar
// and locals looked like to it; the endpoint reports the same.
func mwBuild(t *testing.T, base string, mw Middleware, pages http.Handler, tweak func(*HandleConfig), endpoint http.HandlerFunc, remote ...*Remote) mwStack {
	t.Helper()
	m := mwManifest(base)

	load := NewLoad("src/routes/items/[id]/+page.server.ts", func(ctx context.Context) (pageData, error) {
		e := EventFrom(ctx)
		who, _ := LocalOf[mwUser](ctx)
		session, _ := e.Cookie("session")
		return pageData{Greeting: fmt.Sprintf("user=%s session=%s id=%s", who, session, e.Param("id"))}, nil
	})
	loadCfg := m.LoadConfig(mwOrigin)
	loadCfg.Dev = true
	ls, err := NewLoads(loadCfg, load)
	if err != nil {
		t.Fatal(err)
	}

	if endpoint == nil {
		endpoint = func(w http.ResponseWriter, r *http.Request) {
			who, _ := LocalOf[mwUser](r.Context())
			session, _ := EventFrom(r.Context()).Cookie("session")
			_, _ = fmt.Fprintf(w, "user=%s session=%s", who, session)
		}
	}
	es, err := NewEndpoints(m.EndpointConfig(mwOrigin), NewEndpoint("/api/items/[id]", "GET", endpoint))
	if err != nil {
		t.Fatal(err)
	}
	rs := testRemotes(t, RemoteConfig{AppDir: "_app", Base: base, Origin: mwOrigin}, remote...)

	if pages == nil {
		pages = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte("page"))
		})
	}
	cfg := m.HandleConfig()
	cfg.Origin = mwOrigin
	cfg.Loads = ls
	cfg.OnPanic = func(string, any, []byte) {}
	if tweak != nil {
		tweak(&cfg)
	}
	return mwStack{
		handler: mw.Intercept(cfg, ls.Intercept(rs.Intercept(es.Intercept(pages)))),
		remotes: rs,
		cfg:     cfg,
	}
}

func mwDo(h http.Handler, method, target string, header http.Header) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	req.Host = "127.0.0.1:8080"
	for k, vs := range header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func cookieNamed(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// loadGreeting reads the one string the item page's load returned out of a
// `__data.json` body.
func loadGreeting(t *testing.T, body string) string {
	t.Helper()
	var doc struct {
		Nodes []*struct {
			Type string `json:"type"`
			Data []any  `json:"data"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(body)), &doc); err != nil {
		t.Fatalf("data body %q: %v", body, err)
	}
	for _, n := range doc.Nodes {
		if n != nil && n.Type == "data" && len(n.Data) == 2 {
			if s, ok := n.Data[1].(string); ok {
				return s
			}
		}
	}
	t.Fatalf("no load data in %q", body)
	return ""
}

func TestMiddlewareRunsBeforeResolveAndAfterInOrder(t *testing.T) {
	var mu sync.Mutex
	var log []string
	note := func(s string) { mu.Lock(); log = append(log, s); mu.Unlock() }
	layer := func(name string) Middleware {
		return func(ctx context.Context, e *Event, resolve Resolve) (*http.Response, error) {
			note(name + " before")
			resp, err := resolve(ctx)
			if err != nil {
				return nil, err
			}
			note(name + " after " + resp.Header.Get("X-Layer") + "/" + fmt.Sprint(resp.StatusCode))
			resp.Header.Set("X-Layer", name)
			return resp, nil
		}
	}
	pages := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		note("application")
		w.Header().Set("X-Layer", "app")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("body"))
	})
	s := mwBuild(t, "", Sequence(layer("outer"), layer("inner")), pages, nil, nil)

	rec := mwDo(s.handler, "GET", "/", nil)

	want := []string{"outer before", "inner before", "application", "inner after app/202", "outer after inner/202"}
	if strings.Join(log, "|") != strings.Join(want, "|") {
		t.Errorf("order = %q, want %q", log, want)
	}
	if rec.Code != http.StatusAccepted || rec.Body.String() != "body" || rec.Header().Get("X-Layer") != "outer" {
		t.Errorf("response = %d %q X-Layer=%q", rec.Code, rec.Body.String(), rec.Header().Get("X-Layer"))
	}
}

func TestMiddlewareCanWrapTheBody(t *testing.T) {
	pages := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("inner page")) })
	mw := func(ctx context.Context, e *Event, resolve Resolve) (*http.Response, error) {
		resp, err := resolve(ctx)
		if err != nil {
			return nil, err
		}
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(strings.NewReader("[" + string(raw) + "]"))
		return resp, nil
	}
	s := mwBuild(t, "", mw, pages, nil, nil)
	if got := mwDo(s.handler, "GET", "/", nil).Body.String(); got != "[inner page]" {
		t.Errorf("body = %q", got)
	}
}

func TestMiddlewareShortCircuitNeverCallsDownstream(t *testing.T) {
	var reached int
	pages := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached++ })
	mw := func(ctx context.Context, e *Event, resolve Resolve) (*http.Response, error) {
		if e.Request().URL.Path != "/blocked" {
			return resolve(ctx)
		}
		// Kit adds cookies and `setHeaders` to the response `resolve` made and
		// to nothing a hook builds itself.
		_ = e.SetCookie("session", "never-delivered", CookieOptions{})
		h := http.Header{"Content-Type": {"text/plain"}, "X-Reason": {"maintenance"}}
		return NewResponse(http.StatusTeapot, h, strings.NewReader("short and stout")), nil
	}
	s := mwBuild(t, "", mw, pages, nil, nil)

	rec := mwDo(s.handler, "GET", "/blocked", nil)
	if rec.Code != http.StatusTeapot || rec.Body.String() != "short and stout" || rec.Header().Get("X-Reason") != "maintenance" {
		t.Errorf("deliberate response = %d %q %q", rec.Code, rec.Body.String(), rec.Header().Get("X-Reason"))
	}
	if reached != 0 {
		t.Errorf("the application ran %d time(s) behind a short-circuit", reached)
	}
	if c := cookieNamed(rec, "session"); c != nil {
		t.Errorf("a deliberate response carried the cookie %v; kit adds cookies only to a resolved response", c)
	}

	// The same middleware resolves everything else.
	if rec := mwDo(s.handler, "GET", "/", nil); rec.Body.String() != "" || reached != 1 {
		t.Errorf("resolved request: body %q, application ran %d", rec.Body.String(), reached)
	}
}

func TestMiddlewareLocalsAndRefreshedCookiesReachEveryDownstreamKind(t *testing.T) {
	whoAmI := NewQueryNoArg(testModule, "whoAmI", func(ctx context.Context) (string, error) {
		who, _ := LocalOf[mwUser](ctx)
		session, _ := EventFrom(ctx).Cookie("session")
		return fmt.Sprintf("user=%s session=%s", who, session), nil
	})
	mw := func(ctx context.Context, e *Event, resolve Resolve) (*http.Response, error) {
		if err := SetLocal(ctx, mwUser("ada-lovelace")); err != nil {
			return nil, err
		}
		if err := e.SetCookie("session", "fresh-9", CookieOptions{}); err != nil {
			return nil, err
		}
		if err := e.SetHeader("X-Early", "before-resolve"); err != nil {
			return nil, err
		}
		return resolve(ctx)
	}
	s := mwBuild(t, "", mw, nil, nil, nil, whoAmI)
	stale := http.Header{"Cookie": {"session=stale-1"}}
	const want = "user=ada-lovelace session=fresh-9"

	cases := []struct {
		name, target string
		header       http.Header
		check        func(t *testing.T, body string)
	}{
		{"server load", "/items/42/__data.json?x-sveltekit-invalidated=01", stale, func(t *testing.T, body string) {
			if got := loadGreeting(t, body); got != want+" id=42" {
				t.Errorf("load saw %q", got)
			}
		}},
		{"server route", "/api/items/7", stale, func(t *testing.T, body string) {
			if body != want {
				t.Errorf("endpoint saw %q, want %q", body, want)
			}
		}},
		{"remote query", s.remotes.Prefix() + whoAmI.ID(), stale, func(t *testing.T, body string) {
			_, data, _ := envelope(t, []byte(body))
			if got := field(t, data, "_"); got != want {
				t.Errorf("query saw %#v, want %q", got, want)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := mwDo(s.handler, "GET", tc.target, tc.header)
			if rec.Code != http.StatusOK {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
			tc.check(t, rec.Body.String())
			if c := cookieNamed(rec, "session"); c == nil || c.Value != "fresh-9" {
				t.Errorf("the visitor got session cookie %v, want fresh-9", c)
			}
			if got := rec.Header().Values("Set-Cookie"); len(got) != 1 {
				t.Errorf("Set-Cookie lines = %q, want exactly one: the hook's jar is the one jar", got)
			}
			if got := rec.Header().Get("X-Early"); got != "before-resolve" {
				t.Errorf("X-Early = %q; a header set before resolve belongs on the application's response", got)
			}
		})
	}
}

func TestMiddlewareSeesTheMatchedEventForEachRequestShape(t *testing.T) {
	type seen struct {
		Route                string
		Params               map[string]string
		URL                  string
		Data, Remote, Sub    bool
		Method, SubstitutedP string
	}
	var mu sync.Mutex
	var last seen
	mw := func(ctx context.Context, e *Event, resolve Resolve) (*http.Response, error) {
		mu.Lock()
		last = seen{
			Route: e.RouteID(), Params: e.Params(), URL: e.URL().String(),
			Data: e.IsDataRequest(), Remote: e.IsRemoteRequest(), Sub: e.IsSubRequest(),
		}
		mu.Unlock()
		return resolve(ctx)
	}
	probe := NewQueryNoArg(testModule, "probe", func(ctx context.Context) (string, error) { return "ok", nil })

	for _, withLoads := range []bool{true, false} {
		for _, base := range []string{"", "/app"} {
			name := fmt.Sprintf("loads=%v base=%q", withLoads, base)
			s := mwBuild(t, base, mw, nil, func(c *HandleConfig) {
				if !withLoads {
					c.Loads = nil
				}
			}, nil, probe)
			cases := []struct {
				target string
				header http.Header
				want   seen
			}{
				{base + "/items/42?x=1", nil, seen{Route: "/items/[id]", Params: map[string]string{"id": "42"}, URL: mwOrigin + base + "/items/42?x=1"}},
				{base + "/api/items/7", nil, seen{Route: "/api/items/[id]", Params: map[string]string{"id": "7"}, URL: mwOrigin + base + "/api/items/7"}},
				{base + "/items/42/__data.json?x-sveltekit-invalidated=01&x-sveltekit-trailing-slash=1&keep=yes", nil,
					seen{Route: "/items/[id]", Params: map[string]string{"id": "42"}, URL: mwOrigin + base + "/items/42/?keep=yes", Data: true}},
				{base + "/nowhere/at/all", nil, seen{URL: mwOrigin + base + "/nowhere/at/all"}},
				{base + "/_app/remote/" + probe.ID(), http.Header{
					"X-Sveltekit-Pathname": {base + "/items/9"}, "X-Sveltekit-Search": {"?z=1"},
				}, seen{Route: "/items/[id]", Params: map[string]string{"id": "9"}, URL: mwOrigin + base + "/items/9?z=1", Remote: true}},
				{base + "/_app/remote/" + probe.ID(), nil, seen{URL: mwOrigin + base + "/_app/remote/" + probe.ID(), Remote: true}},
			}
			for _, tc := range cases {
				mu.Lock()
				last = seen{Route: "unset"}
				mu.Unlock()
				rec := mwDo(s.handler, "GET", tc.target, tc.header)
				mu.Lock()
				got := last
				mu.Unlock()
				if got.Route != tc.want.Route || got.URL != tc.want.URL || got.Data != tc.want.Data || got.Remote != tc.want.Remote || got.Sub != tc.want.Sub ||
					fmt.Sprint(got.Params) != fmt.Sprint(tc.want.Params) {
					t.Errorf("%s %s:\n got %+v\nwant %+v (status %d)", name, tc.target, got, tc.want, rec.Code)
				}
			}
		}
	}
}

func TestMiddlewareAnswersAPathOutsideBaseBeforeTheHook(t *testing.T) {
	ran := false
	mw := func(ctx context.Context, e *Event, resolve Resolve) (*http.Response, error) {
		ran = true
		return resolve(ctx)
	}
	s := mwBuild(t, "/app", mw, nil, nil, nil)
	rec := mwDo(s.handler, "GET", "/elsewhere", nil)
	if rec.Code != http.StatusNotFound || rec.Body.String() != "Not found" || ran {
		t.Errorf("outside base: %d %q, hook ran = %v; kit answers 404 'Not found' before `handle`", rec.Code, rec.Body.String(), ran)
	}
}

func TestMiddlewareStaysOutOfStaticFilesAndTheBundle(t *testing.T) {
	ran := 0
	mw := func(ctx context.Context, e *Event, resolve Resolve) (*http.Response, error) {
		ran++
		return resolve(ctx)
	}
	s := mwBuild(t, "", mw, nil, func(c *HandleConfig) {
		c.Static = func(r *http.Request) bool { return r.URL.Path == "/robots.txt" }
	}, nil)
	mwDo(s.handler, "GET", "/robots.txt", nil)
	mwDo(s.handler, "GET", "/_app/immutable/entry/start.js", nil)
	if ran != 0 {
		t.Errorf("hook ran %d times for a static file and the bundle", ran)
	}
	mwDo(s.handler, "GET", "/", nil)
	if ran != 1 {
		t.Errorf("hook ran %d times for a page, want 1", ran)
	}
}

func TestMiddlewareCookieAndHeaderLifecycleMatchesKit(t *testing.T) {
	var lateCookie, lateHeader, dupHeader, setCookieHeader error
	mw := func(ctx context.Context, e *Event, resolve Resolve) (*http.Response, error) {
		if err := e.SetHeader("X-Early", "one"); err != nil {
			return nil, err
		}
		dupHeader = e.SetHeader("x-early", "two")
		setCookieHeader = e.SetHeader("Set-Cookie", "a=b")
		resp, err := resolve(ctx)
		if err != nil {
			return nil, err
		}
		// `resolve`'s `finally` has run: kit's cookies and setHeaders throw,
		// while the response object itself is still the middleware's to wrap.
		lateCookie = e.SetCookie("late", "1", CookieOptions{})
		lateHeader = e.SetHeader("X-Late", "1")
		resp.Header.Set("X-Wrapped", "yes")
		return resp, nil
	}
	s := mwBuild(t, "", mw, nil, nil, nil)
	rec := mwDo(s.handler, "GET", "/", nil)

	for name, err := range map[string]error{"cookie": lateCookie, "header": lateHeader} {
		if err == nil || !strings.Contains(err.Error(), "after the response has been generated") && !strings.Contains(err.Error(), "already been generated") {
			t.Errorf("a %s written after resolve: %v, want kit's after-the-response-has-been-generated refusal", name, err)
		}
	}
	if dupHeader == nil || !strings.Contains(dupHeader.Error(), "already set") {
		t.Errorf("setting a header twice: %v, want an already-set error", dupHeader)
	}
	if setCookieHeader == nil {
		t.Error("SetHeader accepted set-cookie; kit says to use cookies.set")
	}
	if rec.Header().Get("X-Early") != "one" || rec.Header().Get("X-Wrapped") != "yes" {
		t.Errorf("headers = %v, want X-Early=one and X-Wrapped=yes", rec.Header())
	}
	if rec.Header().Get("X-Late") != "" || cookieNamed(rec, "late") != nil {
		t.Errorf("a write after the response was generated reached the visitor: %v", rec.Header())
	}
}

func TestMiddlewareServerTimingAccumulatesLikeKit(t *testing.T) {
	mw := func(ctx context.Context, e *Event, resolve Resolve) (*http.Response, error) {
		_ = e.SetHeader("Server-Timing", "a;dur=1")
		if err := e.SetHeader("Server-Timing", "b;dur=2"); err != nil {
			return nil, err
		}
		return resolve(ctx)
	}
	s := mwBuild(t, "", mw, nil, nil, nil)
	if got := mwDo(s.handler, "GET", "/", nil).Header().Get("Server-Timing"); got != "a;dur=1, b;dur=2" {
		t.Errorf("Server-Timing = %q", got)
	}
}

func TestMiddlewareRefusalsKeepKitsShapesPerRequestKind(t *testing.T) {
	redirect := func(ctx context.Context, e *Event, resolve Resolve) (*http.Response, error) {
		_ = e.SetCookie("session", "kept-with-redirect", CookieOptions{})
		return nil, &Redirect{Status: http.StatusSeeOther, Location: "/login"}
	}
	probe := NewQueryNoArg(testModule, "probe", func(ctx context.Context) (string, error) { return "ok", nil })
	s := mwBuild(t, "", redirect, nil, nil, nil, probe)

	t.Run("native page navigation", func(t *testing.T) {
		rec := mwDo(s.handler, "GET", "/items/42", nil)
		if rec.Code != 303 || rec.Header().Get("Location") != "/login" {
			t.Errorf("%d %q", rec.Code, rec.Header().Get("Location"))
		}
		if c := cookieNamed(rec, "session"); c == nil || c.Value != "kept-with-redirect" {
			t.Errorf("a thrown redirect lost the cookie the request wrote: %v", c)
		}
	})
	t.Run("data request", func(t *testing.T) {
		rec := mwDo(s.handler, "GET", "/items/42/__data.json?x-sveltekit-invalidated=01", nil)
		if rec.Code != 200 || recorded(rec) != `{"type":"redirect","status":303,"location":"/login"}` {
			t.Errorf("%d %s", rec.Code, recorded(rec))
		}
	})
	t.Run("enhanced action", func(t *testing.T) {
		rec := mwDo(s.handler, "POST", "/items/42", http.Header{"Accept": {"application/json"}, "X-Sveltekit-Action": {"true"}})
		if rec.Code != 200 || recorded(rec) != `{"type":"redirect","status":303,"location":"/login"}` {
			t.Errorf("%d %s", rec.Code, recorded(rec))
		}
	})
	t.Run("remote call", func(t *testing.T) {
		rec := mwDo(s.handler, "GET", s.remotes.Prefix()+probe.ID(), nil)
		if rec.Code != 200 || recorded(rec) != `{"type":"redirect","status":303,"location":"/login"}` {
			t.Errorf("%d %s", rec.Code, recorded(rec))
		}
	})

	deny := func(ctx context.Context, e *Event, resolve Resolve) (*http.Response, error) {
		_ = e.SetCookie("session", "dropped-with-error", CookieOptions{})
		return nil, Errorf(http.StatusForbidden, "Hook denied this")
	}
	d := mwBuild(t, "", deny, nil, nil, nil, probe)
	t.Run("error on a data request is JSON without cookies", func(t *testing.T) {
		rec := mwDo(d.handler, "GET", "/items/42/__data.json?x-sveltekit-invalidated=01", nil)
		if rec.Code != 403 || !strings.Contains(recorded(rec), `"Hook denied this"`) || len(rec.Result().Cookies()) != 0 {
			t.Errorf("%d %s cookies=%v", rec.Code, recorded(rec), rec.Result().Cookies())
		}
	})
	t.Run("enhanced action error", func(t *testing.T) {
		rec := mwDo(d.handler, "POST", "/items/42", http.Header{"Accept": {"application/json"}})
		if rec.Code != 403 || !strings.Contains(recorded(rec), `"Hook denied this"`) || rec.Header().Get("Content-Type") != "application/json" {
			t.Errorf("%d %s %q", rec.Code, recorded(rec), rec.Header().Get("Content-Type"))
		}
	})
	t.Run("remote error is an envelope at 200", func(t *testing.T) {
		rec := mwDo(d.handler, "GET", d.remotes.Prefix()+probe.ID(), nil)
		kind, _, httpErr := envelope(t, rec.Body.Bytes())
		if rec.Code != 200 || kind != "error" || httpErr["message"] != "Hook denied this" {
			t.Errorf("%d %s", rec.Code, recorded(rec))
		}
	})
}

func TestMiddlewareDeliberateRedirectOnADataRequestBecomesTheRedirectNodeWithoutCookies(t *testing.T) {
	mw := func(ctx context.Context, e *Event, resolve Resolve) (*http.Response, error) {
		_ = e.SetCookie("session", "lost", CookieOptions{})
		return NewResponse(http.StatusTemporaryRedirect, http.Header{"Location": {"/login"}}, nil), nil
	}
	s := mwBuild(t, "", mw, nil, nil, nil)

	rec := mwDo(s.handler, "GET", "/items/42/__data.json?x-sveltekit-invalidated=01", nil)
	if rec.Code != 200 || recorded(rec) != `{"type":"redirect","status":307,"location":"/login"}` {
		t.Errorf("data: %d %s", rec.Code, recorded(rec))
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Errorf("kit's redirect_json_response is a new response; cookies = %v", rec.Result().Cookies())
	}
	// A native navigation keeps the redirect a real redirect.
	rec = mwDo(s.handler, "GET", "/items/42", nil)
	if rec.Code != 307 || rec.Header().Get("Location") != "/login" {
		t.Errorf("page: %d %q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestMiddlewareAnswersAMatchingEtagWith304(t *testing.T) {
	pages := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Etag", `"v1"`)
		w.Header().Set("Cache-Control", "max-age=60")
		w.Header().Set("X-Dropped", "yes")
		_, _ = w.Write([]byte("a body that must not be sent"))
	})
	s := mwBuild(t, "", Handle(func(context.Context) error { return nil }).Middleware(), pages, nil, nil)

	rec := mwDo(s.handler, "GET", "/", http.Header{"If-None-Match": {`"v1"`}})
	if rec.Code != 304 || rec.Body.Len() != 0 || rec.Header().Get("Etag") != `"v1"` || rec.Header().Get("Cache-Control") != "max-age=60" || rec.Header().Get("X-Dropped") != "" {
		t.Errorf("304 = %d body %q headers %v", rec.Code, rec.Body.String(), rec.Header())
	}
	if rec := mwDo(s.handler, "GET", "/", http.Header{"If-None-Match": {`"other"`}}); rec.Code != 200 || rec.Body.Len() == 0 {
		t.Errorf("a different etag got %d", rec.Code)
	}
}

func TestMiddlewareKeepsConcurrentRequestsApart(t *testing.T) {
	const n = 40
	var barrier sync.WaitGroup
	barrier.Add(n)
	endpoint := func(w http.ResponseWriter, r *http.Request) {
		barrier.Done()
		barrier.Wait() // every request is inside the application at the same moment
		who, _ := LocalOf[mwUser](r.Context())
		session, _ := EventFrom(r.Context()).Cookie("session")
		_, _ = fmt.Fprintf(w, "%s|%s", who, session)
	}
	mw := func(ctx context.Context, e *Event, resolve Resolve) (*http.Response, error) {
		user := e.Request().Header.Get("X-User")
		_ = SetLocal(ctx, mwUser(user))
		_ = e.SetCookie("session", "s-"+user, CookieOptions{})
		_ = e.SetHeader("X-Echo-User", user)
		resp, err := resolve(ctx)
		if err == nil {
			resp.Header.Set("X-After-User", string(mustLocal[mwUser](ctx)))
		}
		return resp, err
	}
	s := mwBuild(t, "", mw, nil, nil, endpoint)

	var wg sync.WaitGroup
	failures := make(chan string, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			user := fmt.Sprintf("user-%d", i)
			rec := mwDo(s.handler, "GET", "/api/items/1", http.Header{"X-User": {user}})
			c := cookieNamed(rec, "session")
			if rec.Body.String() != user+"|s-"+user || c == nil || c.Value != "s-"+user ||
				rec.Header().Get("X-Echo-User") != user || rec.Header().Get("X-After-User") != user {
				failures <- fmt.Sprintf("%s got body %q cookie %v headers %v", user, rec.Body.String(), c, rec.Header())
			}
		}()
	}
	wg.Wait()
	close(failures)
	for f := range failures {
		t.Error(f)
	}
}

func mustLocal[T any](ctx context.Context) T {
	v, _ := LocalOf[T](ctx)
	return v
}

func TestRemoteFunctionBodiesGetNoRequestEventFromTheHook(t *testing.T) {
	type report struct {
		url, route string
		params     map[string]string
		remote     bool
		cookieErr  error
		local      mwUser
	}
	var got report
	q := NewQueryNoArg(testModule, "restricted", func(ctx context.Context) (string, error) {
		e := EventFrom(ctx)
		for _, read := range []func(){func() { e.RouteID() }, func() { e.Params() }, func() { e.URL() }, func() { e.Param("id") }} {
			func() {
				defer func() {
					if recover() == nil {
						t.Error("query request property did not throw")
					}
				}()
				read()
			}()
		}
		got = report{remote: e.IsRemoteRequest(), cookieErr: e.SetCookie("x", "1", CookieOptions{})}
		got.local, _ = LocalOf[mwUser](ctx)
		return "ok", nil
	})
	mw := func(ctx context.Context, e *Event, resolve Resolve) (*http.Response, error) {
		_ = SetLocal(ctx, mwUser("grace"))
		return resolve(ctx)
	}
	s := mwBuild(t, "", mw, nil, nil, nil, q)

	rec := mwDo(s.handler, "GET", s.remotes.Prefix()+q.ID(), http.Header{
		"X-Sveltekit-Pathname": {"/items/9"}, "X-Sveltekit-Search": {"?secret=1"},
	})
	if kind, _, _ := envelope(t, rec.Body.Bytes()); kind == "error" {
		t.Fatalf("query failed: %s", rec.Body.String())
	}
	if got.url != "" || got.route != "" || got.params != nil || got.remote {
		t.Errorf("a query's event exposed the request: %+v; kit's query event has no url, route or params because the result is cached by argument", got)
	}
	if got.cookieErr == nil {
		t.Error("a query was allowed to write a cookie")
	}
	if got.local != "grace" {
		t.Errorf("locals the hook set did not reach the query: %q", got.local)
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Errorf("a refused cookie write reached the visitor: %v", rec.Result().Cookies())
	}
}

func TestMiddlewareCalledResolveTwiceAndPanicsAreContained(t *testing.T) {
	var second error
	mw := func(ctx context.Context, e *Event, resolve Resolve) (*http.Response, error) {
		resp, err := resolve(ctx)
		if err != nil {
			return nil, err
		}
		_, second = resolve(ctx)
		return resp, nil
	}
	s := mwBuild(t, "", mw, nil, nil, nil)
	if rec := mwDo(s.handler, "GET", "/", nil); rec.Code != 200 || second == nil {
		t.Errorf("status %d, second resolve error = %v; resolve may be called once", rec.Code, second)
	}

	var reported any
	boom := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { panic("downstream boom") })
	p := mwBuild(t, "", Handle(func(context.Context) error { return nil }).Middleware(), boom, func(c *HandleConfig) {
		c.OnPanic = func(id string, v any, _ []byte) { reported = v }
	}, nil)
	rec := mwDo(p.handler, "GET", "/", nil)
	if rec.Code != 500 || strings.Contains(rec.Body.String(), "downstream boom") || reported != "downstream boom" {
		t.Errorf("panic before the response began: %d %q reported=%v; want an opaque 500 and the panic reported", rec.Code, rec.Body.String(), reported)
	}
}

// The remaining tests need a real connection: only a socket can show that a
// chunk arrives while the application is still running, and that a vanished
// client reaches the application's context.

func TestMiddlewareStreamsTheFirstChunkBeforeALaterValueResolves(t *testing.T) {
	release := make(chan struct{})
	afterRan := make(chan struct{})
	pages := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("first;"))
		w.(http.Flusher).Flush()
		select {
		case <-release:
			_, _ = w.Write([]byte("second;"))
		case <-r.Context().Done():
		}
	})
	mw := func(ctx context.Context, e *Event, resolve Resolve) (*http.Response, error) {
		resp, err := resolve(ctx)
		if err != nil {
			return nil, err
		}
		resp.Header.Set("X-After", "ran")
		close(afterRan)
		return resp, nil
	}
	srv := httptest.NewServer(mwBuild(t, "", mw, pages, nil, nil).handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	select {
	case <-afterRan:
	default:
		t.Fatal("the response reached the client before the middleware's after-logic ran")
	}
	if resp.Header.Get("X-After") != "ran" {
		t.Errorf("X-After = %q", resp.Header.Get("X-After"))
	}
	first := make([]byte, len("first;"))
	done := make(chan error, 1)
	go func() { _, err := io.ReadFull(resp.Body, first); done <- err }()
	select {
	case err := <-done:
		if err != nil || string(first) != "first;" {
			t.Fatalf("first chunk = %q, %v", first, err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the first chunk did not arrive while the application was still waiting; the middleware buffered the body")
	}
	close(release)
	rest, _ := io.ReadAll(resp.Body)
	if string(rest) != "second;" {
		t.Errorf("rest = %q", rest)
	}
}

func TestMiddlewareCancelsTheApplicationWhenTheClientGoesAway(t *testing.T) {
	canceled := make(chan struct{})
	pages := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("first;"))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(canceled)
	})
	srv := httptest.NewServer(mwBuild(t, "", Handle(func(context.Context) error { return nil }).Middleware(), pages, nil, nil).handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 6)
	if _, err := io.ReadFull(resp.Body, buf); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	srv.CloseClientConnections()
	select {
	case <-canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("the application never saw its context cancelled after the client disconnected")
	}
}

func TestMiddlewareThatDiscardsTheApplicationResponseCancelsIt(t *testing.T) {
	canceled := make(chan struct{})
	pages := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("never delivered"))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(canceled)
	})
	mw := func(ctx context.Context, e *Event, resolve Resolve) (*http.Response, error) {
		resp, err := resolve(ctx)
		if err != nil {
			return nil, err
		}
		_ = resp.Body.Close()
		return NewResponse(http.StatusOK, nil, strings.NewReader("replacement")), nil
	}
	s := mwBuild(t, "", mw, pages, nil, nil)
	rec := mwDo(s.handler, "GET", "/", nil)
	select {
	case <-canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("closing the application's body did not cancel it")
	}
	if rec.Body.String() != "replacement" {
		t.Errorf("body = %q", rec.Body.String())
	}
}

func TestMiddlewareAbortsTheConnectionWhenTheApplicationPanicsMidBody(t *testing.T) {
	pages := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("partial"))
		w.(http.Flusher).Flush()
		panic("late failure")
	})
	srv := httptest.NewServer(mwBuild(t, "", Handle(func(context.Context) error { return nil }).Middleware(), pages, nil, nil).handler)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err == nil {
		t.Errorf("a body that failed mid-stream ended cleanly with %q; the client cannot tell it from a complete page", body)
	}
}
