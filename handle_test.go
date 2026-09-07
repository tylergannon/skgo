package skgo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleShapesItsRefusalForEachKindOfRequest(t *testing.T) {
	refuse := Handle(func(ctx context.Context) error { return &Redirect{Status: 307, Location: "/login"} })
	h := refuse.Intercept(HandleConfig{AppDir: "_app"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	hook := Handle(func(ctx context.Context) error { ran = true; return nil })
	served := false
	h := hook.Intercept(HandleConfig{AppDir: "_app"}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { served = true }))

	get(t, h, "/_app/immutable/chunks/abc.js")
	if ran {
		t.Error("the trust decision ran for a static asset; kit's adapters answer those before the server sees them")
	}
	if !served {
		t.Error("the asset never reached the handler that serves it")
	}
}

func TestHandleMayNotWriteCookies(t *testing.T) {
	var got error
	hook := Handle(func(ctx context.Context) error {
		got = EventFrom(ctx).SetCookie("x", "1", CookieOptions{})
		return nil
	})
	get(t, hook.Intercept(HandleConfig{}, http.NotFoundHandler()), "/a")
	if got == nil {
		t.Error("the hook was allowed to write a cookie; a navigation writes one from a load and a mutation from a command")
	}
}

// TestLocalsCrossFromHandleToLoad proves Handle still reaches a page's server
// load when the app mounts both: Handle sits outermost, over Loads.Intercept,
// exactly as kit runs `handle` before it dispatches to anything.
func TestLocalsCrossFromHandleToLoad(t *testing.T) {
	type session struct{ User string }
	var seen session
	page := NewLoad("src/routes/a/+page.server.ts", func(ctx context.Context) (pageData, error) {
		seen, _ = LocalOf[session](ctx)
		return pageData{Greeting: seen.User}, nil
	})
	ls := mustLoads(t, nil, page)
	hook := Handle(func(ctx context.Context) error { return SetLocal(ctx, session{User: "ada"}) })

	h := hook.Intercept(HandleConfig{}, ls.Intercept(http.NotFoundHandler()))
	get(t, h, "/a/__data.json?x-sveltekit-invalidated=111")
	if seen.User != "ada" {
		t.Errorf("the load saw %+v, want the session the hook stored", seen)
	}
}

// TestLocalOfReachesARemoteFunctionWithoutALoadsRegistry is skgo issue #51: an
// app that answers nothing but remote functions has no server loads and no
// page routes, so it has no use for a Loads registry — and must not need to
// build one just to run its `handle` hook and have LocalOf work inside a
// query. There is no *Loads anywhere in this test.
func TestLocalOfReachesARemoteFunctionWithoutALoadsRegistry(t *testing.T) {
	type session struct{ User string }

	hook := Handle(func(ctx context.Context) error {
		id, _ := EventFrom(ctx).Cookie("session")
		return SetLocal(ctx, session{User: id})
	})

	whoAmI := NewQueryNoArg(testModule, "whoAmI", func(ctx context.Context) (string, error) {
		s, _ := LocalOf[session](ctx)
		return s.User, nil
	})
	rs := testRemotes(t, RemoteConfig{}, whoAmI)

	h := hook.Intercept(HandleConfig{}, rs.Intercept(http.NotFoundHandler()))

	req := httptest.NewRequest(http.MethodGet, rs.Prefix()+whoAmI.ID(), nil)
	// An independent fixture the test supplies, not anything derived from the
	// code under test: if LocalOf ever came back empty, this value is what
	// would go missing.
	req.AddCookie(&http.Cookie{Name: "session", Value: "quokka-42"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	kind, data, httpErr := envelope(t, rec.Body.Bytes())
	if kind == "error" {
		t.Fatalf("remote call refused: %v", httpErr)
	}
	if got := field(t, data, "_"); got != "quokka-42" {
		t.Errorf("LocalOf returned %#v, want %q — the cookie value the hook stored", got, "quokka-42")
	}
}

// TestHandleRunsExactlyOnceForOneRequest guards the composition itself: when
// an app mounts both Loads and Remotes under Handle, the hook must run once
// per request, not once per registry that happens to sit underneath it.
func TestHandleRunsExactlyOnceForOneRequest(t *testing.T) {
	runs := 0
	hook := Handle(func(ctx context.Context) error { runs++; return nil })

	ls := mustLoads(t, nil)
	rs := testRemotes(t, RemoteConfig{})

	h := hook.Intercept(HandleConfig{}, ls.Intercept(rs.Intercept(http.NotFoundHandler())))

	get(t, h, "/a/__data.json?x-sveltekit-invalidated=11")

	if runs != 1 {
		t.Errorf("hook ran %d times for one request, want exactly 1", runs)
	}
}
