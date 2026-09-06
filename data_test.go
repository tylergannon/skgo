package skgo

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The golden bodies below were taken from kit's own dev server answering the
// same shapes (`@sveltejs/kit` 3.0.0-next.25, `packages/kit/src/runtime/server/data`),
// not from this implementation.

// section is the route table the tests serve: a root layout with no server
// file, a layout with one, and three pages under it.
func section() Manifest {
	return Manifest{
		AppDir:  "_app",
		Version: "",
		Nodes: []string{
			"",
			"src/routes/a/+layout.server.ts",
			"src/routes/a/+page.server.ts",
			"src/routes/a/b/+page.server.ts",
			"",
		},
		Routes: []ManifestRoute{
			{ID: "/", Pattern: `^\/$`, Page: &ManifestPage{Layouts: []int{0}, Leaf: 4}},
			{ID: "/a", Pattern: `^\/a\/?$`, Page: &ManifestPage{Layouts: []int{0, 1}, Leaf: 2}},
			{ID: "/a/b", Pattern: `^\/a\/b\/?$`, Page: &ManifestPage{Layouts: []int{0, 1}, Leaf: 3}},
			{ID: "/api", Pattern: `^\/api\/?$`},
			{
				ID:      "/items/[id]",
				Pattern: `^\/items\/([^/]+?)\/?$`,
				Params:  []ManifestParam{{Name: "id"}},
				Page:    &ManifestPage{Layouts: []int{0}, Leaf: 4},
			},
		},
	}
}

func mustLoads(t *testing.T, tweak func(*LoadConfig), loads ...*ServerLoad) *Loads {
	t.Helper()
	cfg := section().LoadConfig("http://127.0.0.1:8080")
	cfg.Dev = true // the drift check is about a real build, not a fixture
	if tweak != nil {
		tweak(&cfg)
	}
	ls, err := NewLoads(cfg, loads...)
	if err != nil {
		t.Fatalf("NewLoads: %v", err)
	}
	return ls
}

func get(t *testing.T, h http.Handler, target string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Host = "127.0.0.1:8080"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func recorded(rec *httptest.ResponseRecorder) string {
	return strings.TrimSuffix(rec.Body.String(), "\n")
}

type layoutData struct {
	Who string `json:"who"`
}

type pageData struct {
	Greeting string `json:"greeting"`
}

func TestDataSuffix(t *testing.T) {
	// `/foo.html__data.json` is a sibling of `/foo.html`, not a child, which is
	// the case a naive TrimSuffix gets wrong.
	cases := []struct{ in, want string }{
		{"/a/__data.json", "/a"},
		{"/__data.json", ""},
		{"/a/b.html__data.json", "/a/b.html"},
	}
	for _, tc := range cases {
		if !hasDataSuffix(tc.in) {
			t.Errorf("hasDataSuffix(%q) = false", tc.in)
		}
		if got := stripDataSuffix(tc.in); got != tc.want {
			t.Errorf("stripDataSuffix(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	if hasDataSuffix("/a/data.json") {
		t.Error("hasDataSuffix matched a path that is not a data request")
	}
}

func TestNodesArrayIsPositional(t *testing.T) {
	layout := NewLoad("src/routes/a/+layout.server.ts", func(ctx context.Context) (layoutData, error) {
		return layoutData{Who: "ada"}, nil
	})
	page := NewLoad("src/routes/a/+page.server.ts", func(ctx context.Context) (pageData, error) {
		return pageData{Greeting: "hello"}, nil
	})
	ls := mustLoads(t, nil, layout, page)

	// Node 0 has no server file at all, so it is `null`; the other two ran.
	rec := get(t, ls, "/a/__data.json?x-sveltekit-invalidated=111")
	want := `{"type":"data","nodes":[null,{"type":"data","data":[{"who":1},"ada"],"uses":{}},{"type":"data","data":[{"greeting":1},"hello"],"uses":{}}]}`
	if got := recorded(rec); got != want {
		t.Errorf("body =\n%s\nwant\n%s", got, want)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "private, no-store" {
		t.Errorf("Cache-Control = %q", cc)
	}
	// Kit's head line ends in a newline and counts it.
	if rec.Body.String() != want+"\n" {
		t.Error("the head line does not end with a newline")
	}
	if rec.Header().Get("Content-Length") == "" {
		t.Error("a non-streaming response must carry a Content-Length")
	}
}

func TestUninvalidatedNodeIsSkipped(t *testing.T) {
	var layoutRuns int
	layout := NewLoad("src/routes/a/+layout.server.ts", func(ctx context.Context) (layoutData, error) {
		layoutRuns++
		return layoutData{Who: "ada"}, nil
	})
	page := NewLoad("src/routes/a/+page.server.ts", func(ctx context.Context) (pageData, error) {
		return pageData{Greeting: "hello"}, nil
	})
	ls := mustLoads(t, nil, layout, page)

	rec := get(t, ls, "/a/__data.json?x-sveltekit-invalidated=001")
	want := `{"type":"data","nodes":[{"type":"skip"},{"type":"skip"},{"type":"data","data":[{"greeting":1},"hello"],"uses":{}}]}`
	if got := recorded(rec); got != want {
		t.Errorf("body =\n%s\nwant\n%s", got, want)
	}
	if layoutRuns != 0 {
		t.Errorf("the skipped layout ran %d time(s); nothing asked for its data", layoutRuns)
	}
}

func TestAbsentInvalidatedParamRunsEveryNode(t *testing.T) {
	layout := NewLoad("src/routes/a/+layout.server.ts", func(ctx context.Context) (layoutData, error) {
		return layoutData{Who: "ada"}, nil
	})
	ls := mustLoads(t, nil, layout)

	rec := get(t, ls, "/a/__data.json")
	if strings.Contains(recorded(rec), `"skip"`) {
		t.Errorf("without the parameter every node runs; got %s", recorded(rec))
	}
}

// A node the client told the server to skip still has to run when a descendant
// asks for its data, and its result still must not be sent.
func TestSkippedNodeStillRunsForParent(t *testing.T) {
	var layoutRuns int
	layout := NewLoad("src/routes/a/+layout.server.ts", func(ctx context.Context) (layoutData, error) {
		layoutRuns++
		return layoutData{Who: "ada"}, nil
	})
	page := NewLoad("src/routes/a/+page.server.ts", func(ctx context.Context) (pageData, error) {
		parent, err := Parent[layoutData](ctx)
		if err != nil {
			return pageData{}, err
		}
		return pageData{Greeting: "hello " + parent.Who}, nil
	})
	ls := mustLoads(t, nil, layout, page)

	rec := get(t, ls, "/a/__data.json?x-sveltekit-invalidated=001")
	want := `{"type":"data","nodes":[{"type":"skip"},{"type":"skip"},{"type":"data","data":[{"greeting":1},"hello ada"],"uses":{"parent":1}}]}`
	if got := recorded(rec); got != want {
		t.Errorf("body =\n%s\nwant\n%s", got, want)
	}
	if layoutRuns != 1 {
		t.Errorf("the layout ran %d time(s), want exactly 1", layoutRuns)
	}
}

func TestParentMergesOutermostFirst(t *testing.T) {
	type both struct {
		Who   string `json:"who"`
		Where string `json:"where"`
	}
	layout := NewLoad("src/routes/a/+layout.server.ts", func(ctx context.Context) (both, error) {
		return both{Who: "layout", Where: "outer"}, nil
	})
	var got both
	page := NewLoad("src/routes/a/+page.server.ts", func(ctx context.Context) (pageData, error) {
		var err error
		got, err = Parent[both](ctx)
		return pageData{Greeting: "ok"}, err
	})
	ls := mustLoads(t, nil, layout, page)
	get(t, ls, "/a/__data.json?x-sveltekit-invalidated=111")

	if got.Who != "layout" || got.Where != "outer" {
		t.Errorf("Parent returned %+v", got)
	}
}

func TestLoadErrorIsANodeAtHTTP200(t *testing.T) {
	layout := NewLoad("src/routes/a/+layout.server.ts", func(ctx context.Context) (layoutData, error) {
		return layoutData{Who: "ada"}, nil
	})
	page := NewLoad("src/routes/a/+page.server.ts", func(ctx context.Context) (pageData, error) {
		return pageData{}, Errorf(402, "Your account is in arrears")
	})
	ls := mustLoads(t, nil, layout, page)

	rec := get(t, ls, "/a/__data.json?x-sveltekit-invalidated=111")
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200: a failed load is a node, not an HTTP error", rec.Code)
	}
	want := `{"type":"data","nodes":[null,{"type":"data","data":[{"who":1},"ada"],"uses":{}},{"type":"error","error":{"status":402,"message":"Your account is in arrears"}}]}`
	if got := recorded(rec); got != want {
		t.Errorf("body =\n%s\nwant\n%s", got, want)
	}
}

func TestAnUnexpectedErrorSaysNothing(t *testing.T) {
	page := NewLoad("src/routes/a/+page.server.ts", func(ctx context.Context) (pageData, error) {
		return pageData{}, errors.New("SECRET-INTERNAL-DETAIL: the database is on fire")
	})
	ls := mustLoads(t, nil, page)

	got := recorded(get(t, ls, "/a/__data.json?x-sveltekit-invalidated=111"))
	if strings.Contains(got, "SECRET") {
		t.Fatalf("the error message reached the client: %s", got)
	}
	if !strings.Contains(got, `{"type":"error","error":{"status":500,"message":"Internal Error"}}`) {
		t.Errorf("body = %s", got)
	}
}

func TestRedirectIsJSONAtHTTP200(t *testing.T) {
	layout := NewLoad("src/routes/a/+layout.server.ts", func(ctx context.Context) (layoutData, error) {
		return layoutData{}, &Redirect{Status: 307, Location: "/"}
	})
	page := NewLoad("src/routes/a/+page.server.ts", func(ctx context.Context) (pageData, error) {
		return pageData{Greeting: "never seen"}, nil
	})
	ls := mustLoads(t, nil, layout, page)

	rec := get(t, ls, "/a/__data.json?x-sveltekit-invalidated=111")
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200: a real 3xx breaks kit's client here", rec.Code)
	}
	if got, want := recorded(rec), `{"type":"redirect","status":307,"location":"/"}`; got != want {
		t.Errorf("body = %s, want %s", got, want)
	}
	if strings.Contains(rec.Body.String(), "never seen") {
		t.Error("the page's data escaped alongside the redirect")
	}
}

func TestRedirectDefaultsTo307(t *testing.T) {
	layout := NewLoad("src/routes/a/+layout.server.ts", func(ctx context.Context) (layoutData, error) {
		return layoutData{}, &Redirect{Location: "/elsewhere"}
	})
	ls := mustLoads(t, nil, layout)
	if got, want := recorded(get(t, ls, "/a/__data.json?x-sveltekit-invalidated=110")), `{"type":"redirect","status":307,"location":"/elsewhere"}`; got != want {
		t.Errorf("body = %s, want %s", got, want)
	}
}

// A page whose parent redirected must not be reachable through Parent either.
func TestParentPropagatesTheRedirect(t *testing.T) {
	layout := NewLoad("src/routes/a/+layout.server.ts", func(ctx context.Context) (layoutData, error) {
		return layoutData{}, &Redirect{Status: 303, Location: "/login"}
	})
	var seen error
	page := NewLoad("src/routes/a/+page.server.ts", func(ctx context.Context) (pageData, error) {
		_, seen = Parent[layoutData](ctx)
		return pageData{}, seen
	})
	ls := mustLoads(t, nil, layout, page)
	get(t, ls, "/a/__data.json?x-sveltekit-invalidated=111")

	var redirect *Redirect
	if !errors.As(seen, &redirect) || redirect.Location != "/login" {
		t.Errorf("Parent returned %v, want the layout's redirect", seen)
	}
}

func TestDeferredValueStreams(t *testing.T) {
	type streamed struct {
		Fast string                   `json:"fast"`
		Slow Deferred[[]string]       `json:"slow"`
		More Deferred[map[string]int] `json:"-"`
	}
	page := NewLoad("src/routes/a/+page.server.ts", func(ctx context.Context) (streamed, error) {
		return streamed{
			Fast: "now",
			Slow: Async(ctx, func(ctx context.Context) ([]string, error) {
				time.Sleep(20 * time.Millisecond)
				return []string{"later"}, nil
			}),
		}, nil
	})
	ls := mustLoads(t, nil, page)

	rec := get(t, ls, "/a/__data.json?x-sveltekit-invalidated=111")
	if ct := rec.Header().Get("Content-Type"); ct != "text/sveltekit-data" {
		t.Errorf("Content-Type = %q, want text/sveltekit-data", ct)
	}
	if rec.Header().Get("Content-Length") != "" {
		t.Error("a streamed response must not claim a Content-Length")
	}

	lines := readLines(t, rec.Body.String())
	if len(lines) != 2 {
		t.Fatalf("got %d lines:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	// A promise is a devalue custom node whose payload is the chunk id, and
	// kit's ids start at 1 — devalue treats a reducer returning 0 as no match.
	wantHead := `{"type":"data","nodes":[null,null,{"type":"data","data":[{"fast":1,"slow":2},"now",["Promise",3],1],"uses":{}}]}`
	if lines[0] != wantHead {
		t.Errorf("head =\n%s\nwant\n%s", lines[0], wantHead)
	}
	if want := `{"type":"chunk","id":1,"data":[[1],"later"]}`; lines[1] != want {
		t.Errorf("chunk = %s, want %s", lines[1], want)
	}
}

func TestDeferredFailureIsAnErrorChunk(t *testing.T) {
	type streamed struct {
		Slow Deferred[string] `json:"slow"`
	}
	page := NewLoad("src/routes/a/+page.server.ts", func(ctx context.Context) (streamed, error) {
		return streamed{Slow: Async(ctx, func(ctx context.Context) (string, error) {
			return "", Errorf(404, "gone")
		})}, nil
	})
	ls := mustLoads(t, nil, page)

	lines := readLines(t, get(t, ls, "/a/__data.json?x-sveltekit-invalidated=111").Body.String())
	if len(lines) != 2 {
		t.Fatalf("got %d lines:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	if want := `{"type":"chunk","id":1,"error":[{"status":1,"message":2},404,"gone"]}`; lines[1] != want {
		t.Errorf("chunk = %s, want %s", lines[1], want)
	}
}

func TestDeferredBelowTheTopLevelIsRefused(t *testing.T) {
	type nested struct {
		Inner struct {
			Slow Deferred[string] `json:"slow"`
		} `json:"inner"`
	}
	page := NewLoad("src/routes/a/+page.server.ts", func(ctx context.Context) (nested, error) {
		var out nested
		out.Inner.Slow = Resolved("x")
		return out, nil
	})
	ls := mustLoads(t, nil, page)

	got := recorded(get(t, ls, "/a/__data.json?x-sveltekit-invalidated=111"))
	if !strings.Contains(got, `"type":"error"`) {
		t.Errorf("a Deferred nested below the top level must be refused, not serialized as null; got %s", got)
	}
}

func TestUsesRecordsWhatTheLoadRead(t *testing.T) {
	page := NewLoad("src/routes/a/+page.server.ts", func(ctx context.Context) (pageData, error) {
		e := EventFrom(ctx)
		e.Depends("app:section", "/relative")
		e.SearchParam("q")
		e.RouteID()
		e.Untrack(func() { e.URL() })
		return pageData{Greeting: "ok"}, nil
	})
	ls := mustLoads(t, nil, page)

	got := recorded(get(t, ls, "/a/__data.json?q=hi&x-sveltekit-invalidated=111"))
	// Sparse, exactly as kit serializes it: empty collections and false flags
	// are left out, and a flag travels as the number 1. `url` is absent because
	// the read was untracked; the dependency is not, because kit's untrack
	// turns off the implicit tracking and never the explicit call.
	want := `"uses":{"dependencies":["app:section","http://127.0.0.1:8080/relative"],"search_params":["q"],"route":1}`
	if !strings.Contains(got, want) {
		t.Errorf("body = %s\nwant it to contain %s", got, want)
	}
}

func TestKitsPrivateParametersNeverReachTheLoad(t *testing.T) {
	var seen string
	page := NewLoad("src/routes/a/+page.server.ts", func(ctx context.Context) (pageData, error) {
		seen = EventFrom(ctx).URL().String()
		return pageData{Greeting: "ok"}, nil
	})
	ls := mustLoads(t, nil, page)
	get(t, ls, "/a/__data.json?q=hi&x-sveltekit-invalidated=111")

	if want := "http://127.0.0.1:8080/a?q=hi"; seen != want {
		t.Errorf("the load saw %q, want %q", seen, want)
	}
}

func TestTrailingSlashIsRestoredOntoThePath(t *testing.T) {
	var seen string
	page := NewLoad("src/routes/a/+page.server.ts", func(ctx context.Context) (pageData, error) {
		seen = EventFrom(ctx).URL().Path
		return pageData{Greeting: "ok"}, nil
	})
	ls := mustLoads(t, nil, page)
	get(t, ls, "/a/__data.json?x-sveltekit-trailing-slash=1&x-sveltekit-invalidated=111")

	if seen != "/a/" {
		t.Errorf("the load saw path %q, want /a/", seen)
	}
}

func TestUnmatchedPathAsksForTheRootLayoutOnly(t *testing.T) {
	ls := mustLoads(t, nil)

	// Exactly one invalidated node is kit's signal that the client wants the
	// root layout's data for an error page.
	rec := get(t, ls, "/nope/nope/__data.json?x-sveltekit-invalidated=1")
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if got, want := recorded(rec), `{"type":"data","nodes":[null]}`; got != want {
		t.Errorf("body = %s, want %s", got, want)
	}

	// Anything else is a miss, and the client turns a 404 into `Not Found`
	// rather than reloading the page.
	if rec := get(t, ls, "/nope/nope/__data.json?x-sveltekit-invalidated=11"); rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestEndpointRouteHasNoData(t *testing.T) {
	ls := mustLoads(t, nil)
	rec := get(t, ls, "/api/__data.json?x-sveltekit-invalidated=1")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404: a route with no page has no data", rec.Code)
	}
}

func TestDataRequestsNeverReachTheNextHandler(t *testing.T) {
	reached := false
	ls := mustLoads(t, nil)
	h := ls.Intercept(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true }))

	// This is the near miss the boot document used to satisfy: kit's route
	// patterns end `\/?$`, so a one-segment data path matches its own page.
	get(t, h, "/a/__data.json?x-sveltekit-invalidated=11")
	if reached {
		t.Error("a data request fell through to the page handler")
	}
}

func TestRouteParameters(t *testing.T) {
	var id string
	page := NewLoad("src/routes/a/+page.server.ts", func(ctx context.Context) (pageData, error) {
		return pageData{}, nil
	})
	_ = page
	ls := mustLoads(t, nil)
	route, params, ok := ls.match("/items/42%2Fb")
	if !ok {
		t.Fatal("/items/42%2Fb did not match")
	}
	if route.id != "/items/[id]" {
		t.Errorf("route = %s", route.id)
	}
	id = params["id"]
	if id != "42/b" {
		t.Errorf("id = %q, want %q (the value is percent-decoded, as kit decodes it)", id, "42/b")
	}
}

func TestHandleShapesItsRefusalForEachKindOfRequest(t *testing.T) {
	refuse := func(ctx context.Context) error { return &Redirect{Status: 307, Location: "/login"} }
	ls := mustLoads(t, func(cfg *LoadConfig) { cfg.Handle = refuse })
	h := ls.Intercept(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("the refused request reached the next handler")
	}))

	// A data request gets kit's redirect node at HTTP 200 …
	rec := get(t, h, "/a/__data.json?x-sveltekit-invalidated=11")
	if rec.Code != http.StatusOK || recorded(rec) != `{"type":"redirect","status":307,"location":"/login"}` {
		t.Errorf("data request: %d %s", rec.Code, recorded(rec))
	}
	// … a remote call gets the same node …
	rec = get(t, h, "/_app/remote/abc/def")
	if rec.Code != http.StatusOK || recorded(rec) != `{"type":"redirect","status":307,"location":"/login"}` {
		t.Errorf("remote call: %d %s", rec.Code, recorded(rec))
	}
	// … and a page request gets a real HTTP redirect.
	rec = get(t, h, "/a")
	if rec.Code != http.StatusTemporaryRedirect || rec.Header().Get("Location") != "/login" {
		t.Errorf("page request: %d %s", rec.Code, rec.Header().Get("Location"))
	}
}

func TestHandleDoesNotRunForTheBundle(t *testing.T) {
	ran := false
	ls := mustLoads(t, func(cfg *LoadConfig) {
		cfg.Handle = func(ctx context.Context) error { ran = true; return nil }
	})
	served := false
	h := ls.Intercept(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { served = true }))

	get(t, h, "/_app/immutable/chunks/abc.js")
	if ran {
		t.Error("the trust decision ran for a static asset; kit's adapters answer those before the server sees them")
	}
	if !served {
		t.Error("the asset never reached the handler that serves it")
	}
}

func TestLocalsCrossFromHandleToLoad(t *testing.T) {
	type session struct{ User string }
	var seen session
	page := NewLoad("src/routes/a/+page.server.ts", func(ctx context.Context) (pageData, error) {
		seen, _ = LocalOf[session](ctx)
		return pageData{Greeting: seen.User}, nil
	})
	ls := mustLoads(t, func(cfg *LoadConfig) {
		cfg.Handle = func(ctx context.Context) error { return SetLocal(ctx, session{User: "ada"}) }
	}, page)

	get(t, ls.Intercept(http.NotFoundHandler()), "/a/__data.json?x-sveltekit-invalidated=111")
	if seen.User != "ada" {
		t.Errorf("the load saw %+v, want the session the hook stored", seen)
	}
}

func TestHandleMayNotWriteCookies(t *testing.T) {
	var got error
	ls := mustLoads(t, func(cfg *LoadConfig) {
		cfg.Handle = func(ctx context.Context) error {
			got = EventFrom(ctx).SetCookie("x", "1", CookieOptions{})
			return nil
		}
	})
	get(t, ls.Intercept(http.NotFoundHandler()), "/a")
	if got == nil {
		t.Error("the hook was allowed to write a cookie; a navigation writes one from a load and a mutation from a command")
	}
}

func TestALoadMayWriteCookiesAndHeaders(t *testing.T) {
	page := NewLoad("src/routes/a/+page.server.ts", func(ctx context.Context) (pageData, error) {
		e := EventFrom(ctx)
		if err := e.SetCookie("seen", "1", CookieOptions{}); err != nil {
			return pageData{}, err
		}
		if err := e.SetHeader("X-Probe", "yes"); err != nil {
			return pageData{}, err
		}
		if err := e.SetHeader("X-Probe", "again"); err == nil {
			return pageData{}, errors.New("a repeated header was accepted")
		}
		return pageData{Greeting: "ok"}, nil
	})
	ls := mustLoads(t, nil, page)

	rec := get(t, ls, "/a/__data.json?x-sveltekit-invalidated=111")
	if strings.Contains(recorded(rec), `"error"`) {
		t.Fatalf("body = %s", recorded(rec))
	}
	if h := rec.Header().Get("X-Probe"); h != "yes" {
		t.Errorf("X-Probe = %q", h)
	}
	if c := rec.Header().Get("Set-Cookie"); !strings.HasPrefix(c, "seen=1;") {
		t.Errorf("Set-Cookie = %q", c)
	}
	// Cookies are the one header a load may not set directly.
	if err := (&Event{load: &loadState{shared: &loadRequest{headers: http.Header{}}}}).SetHeader("set-cookie", "x=1"); err == nil {
		t.Error("SetHeader accepted set-cookie")
	}
}

func TestVersionHeaderIsOnlySentWhenConfigured(t *testing.T) {
	ls := mustLoads(t, nil)
	if got := get(t, ls, "/a/__data.json?x-sveltekit-invalidated=11").Header().Get("X-Sveltekit-Version"); got != "" {
		t.Errorf("X-Sveltekit-Version = %q; a value the client did not build against makes it reload", got)
	}

	ls = mustLoads(t, func(cfg *LoadConfig) { cfg.Version = "1712345678901" })
	if got := get(t, ls, "/a/__data.json?x-sveltekit-invalidated=11").Header().Get("X-Sveltekit-Version"); got != "1712345678901" {
		t.Errorf("X-Sveltekit-Version = %q", got)
	}
}

func TestDriftBetweenTheBuildAndTheBinaryStopsTheServer(t *testing.T) {
	cfg := section().LoadConfig("http://127.0.0.1:8080")
	_, err := NewLoads(cfg)
	if err == nil {
		t.Fatal("a binary answering none of the build's server loads was accepted")
	}
	for _, want := range []string{"src/routes/a/+layout.server.ts", "src/routes/a/+page.server.ts", "go generate"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the message does not mention %q:\n%s", want, err)
		}
	}

	cfg.Nodes = []string{""}
	cfg.Routes = nil
	_, err = NewLoads(cfg, NewLoad("src/routes/ghost/+page.server.ts", func(ctx context.Context) (pageData, error) {
		return pageData{}, nil
	}))
	if err == nil || !strings.Contains(err.Error(), "src/routes/ghost/+page.server.ts") {
		t.Errorf("a load the frontend never compiled was accepted: %v", err)
	}
}

func TestOnlyGETAndHEAD(t *testing.T) {
	ls := mustLoads(t, nil)
	req := httptest.NewRequest(http.MethodPost, "/a/__data.json", nil)
	rec := httptest.NewRecorder()
	ls.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func readLines(t *testing.T, s string) []string {
	t.Helper()
	var out []string
	scanner := bufio.NewScanner(strings.NewReader(s))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var probe map[string]any
		if err := json.Unmarshal([]byte(line), &probe); err != nil {
			t.Fatalf("line is not JSON: %s", line)
		}
		out = append(out, line)
	}
	return out
}
