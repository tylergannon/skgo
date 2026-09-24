package skgo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestKitReservedQueryParametersAreRefusedBeforeTheHandleHook(t *testing.T) {
	ran := false
	hook := Handle(func(context.Context) error { ran = true; return nil })
	h := hook.Intercept(HandleConfig{}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	for _, target := range []string{
		"/?x-sveltekit-private=1",
		"/?ordinary=1&x-sveltekit-private=1",
		"/?x%2Dsveltekit-private=1",
		"/?%78-sveltekit-private%zz=1",
		"/a/__data.json?x-sveltekit-invalidated=1&x-sveltekit-private=1",
		"/_app/remote/example/query?x-sveltekit-private=1",
	} {
		ran = false
		rec := get(t, h, target)
		want := `Cannot use reserved query parameter "x-sveltekit-private"`
		if target == "/?%78-sveltekit-private%zz=1" {
			want = `Cannot use reserved query parameter "x-sveltekit-private%zz"`
		}
		if rec.Code != http.StatusBadRequest || recorded(rec) != want {
			t.Errorf("%s: got %d %q", target, rec.Code, recorded(rec))
		}
		if ran {
			t.Errorf("%s: handle hook ran before query was refused", target)
		}
	}
}

func TestKitDataRequestParametersRemainAvailableToKit(t *testing.T) {
	loads := mustLoads(t, nil)
	h := loads.Intercept(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rejectReservedQuery(w, r, false, false) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	rec := get(t, h, "/a/__data.json?x-sveltekit-invalidated=1&x-sveltekit-trailing-slash=1")
	if rec.Code != http.StatusOK {
		t.Fatalf("Kit's own data parameters were refused: %d %q", rec.Code, recorded(rec))
	}

	rec = get(t, h, "/a?x-sveltekit-invalidated=1")
	if rec.Code != http.StatusBadRequest || recorded(rec) != `Cannot use reserved query parameter "x-sveltekit-invalidated"` {
		t.Errorf("ordinary page: got %d %q", rec.Code, recorded(rec))
	}
}

func TestRemotePathnameHeaderSelectsTheSearchKitChecks(t *testing.T) {
	query := NewQueryNoArg(testModule, "reservedSearch", func(context.Context) (int, error) { return 7, nil })
	remotes := testRemotes(t, RemoteConfig{}, query)
	for _, tc := range []struct {
		name, endpointQuery, headerSearch string
		wantCode                          int
	}{
		{"endpoint query is replaced", "x-sveltekit-hidden=1", "?ordinary=1", http.StatusOK},
		{"header query is checked", "ordinary=1", "?x-sveltekit-hidden=1", http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, remotes.Prefix()+query.ID()+"?"+tc.endpointQuery, nil)
			req.Header.Set("x-sveltekit-pathname", "/")
			req.Header.Set("x-sveltekit-search", tc.headerSearch)
			rec := httptest.NewRecorder()
			remotes.ServeHTTP(rec, req)
			if rec.Code != tc.wantCode {
				t.Errorf("got %d %q, want %d", rec.Code, recorded(rec), tc.wantCode)
			}
		})
	}
}

func TestEndpointRefusesKitReservedQueryBeforeItsHandler(t *testing.T) {
	called := false
	h := newEndpoints(t, endpointFixture(nil, []string{"GET"}, nil),
		NewEndpoint("/api/thing", "GET", func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusNoContent)
		}),
	)
	rec := get(t, h, "/api/thing?x-sveltekit-private=1")
	if rec.Code != http.StatusBadRequest || recorded(rec) != `Cannot use reserved query parameter "x-sveltekit-private"` {
		t.Errorf("endpoint: got %d %q", rec.Code, recorded(rec))
	}
	if called {
		t.Error("endpoint ran before Kit's reserved query check")
	}
}

func TestRouteResolutionRequestChecksReservedQueryBeforeItsFallback(t *testing.T) {
	rec := get(t, newTestHandler(t), "/about/__route.js?x-sveltekit-private=1")
	if rec.Code != http.StatusBadRequest || recorded(rec) != `Cannot use reserved query parameter "x-sveltekit-private"` {
		t.Errorf("route resolution: got %d %q", rec.Code, recorded(rec))
	}
}

func TestReservedQueryPrecedesPageMethodResolution(t *testing.T) {
	resp := do(t, newTestHandler(t), http.MethodPut, "/about?x-sveltekit-private=1", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("page PUT: got %d, want reserved-query 400", resp.StatusCode)
	}
}
