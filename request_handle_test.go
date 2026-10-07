package skgo

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type hookLocals struct{ Name string }
type hookEvent = RequestEvent[struct{}, hookLocals]
type hookMiddleware = RequestMiddleware[struct{}, hookLocals]
type hookResolve = RequestResolve[struct{}, hookLocals]

func localsEndpoint(t *testing.T) http.Handler {
	t.Helper()
	m := Manifest{Routes: []ManifestRoute{{ID: "/locals", Pattern: "^/locals/?$", Endpoint: &ManifestEndpoint{Methods: []string{"GET"}}}}}
	es, err := NewEndpoints(m.EndpointConfig("http://localhost"), NewEndpoint("/locals", "GET", func(w http.ResponseWriter, r *http.Request) {
		p := RequestLocals[hookLocals](r.Context())
		if p == nil {
			http.Error(w, "missing locals", 500)
			return
		}
		fmt.Fprint(w, p.Name)
	}))
	if err != nil {
		t.Fatal(err)
	}
	return es.Intercept(http.NotFoundHandler())
}

func TestTypedHookForwardingAndIncomingContexts(t *testing.T) {
	for _, forward := range []bool{false, true} {
		t.Run(fmt.Sprint(forward), func(t *testing.T) {
			var outerIncoming, innerIncoming *hookLocals
			outer := hookMiddleware(func(ctx context.Context, event hookEvent, resolve hookResolve) (*http.Response, error) {
				outerIncoming = event.Locals
				if RequestLocals[hookLocals](ctx) != outerIncoming {
					t.Fatal("outer context disagrees")
				}
				original := event
				event.Locals = &hookLocals{Name: "forwarded"}
				selected := original
				if forward {
					selected = event
				}
				response, err := resolve(ctx, selected)
				event.Locals = &hookLocals{Name: "assigned-after-resolve"}
				if RequestLocals[hookLocals](ctx) != outerIncoming || outerIncoming.Name != "shared-mutation" {
					t.Fatal("outer binding changed or mutation disappeared")
				}
				return response, err
			})
			inner := hookMiddleware(func(ctx context.Context, event hookEvent, resolve hookResolve) (*http.Response, error) {
				innerIncoming = event.Locals
				if RequestLocals[hookLocals](ctx) != innerIncoming {
					t.Fatal("inner context disagrees")
				}
				outerIncoming.Name = "shared-mutation"
				if forward {
					event.Locals = &hookLocals{Name: "inner-selected"}
				}
				return resolve(ctx, event)
			})
			rec := get(t, RequestSequence(outer, inner).Intercept(HandleConfig{}, emptyHookParams, localsEndpoint(t)), "/locals")
			want := "shared-mutation"
			if forward {
				want = "inner-selected"
			}
			if rec.Code != 200 || rec.Body.String() != want {
				t.Fatalf("got %d %q, want 200 %q", rec.Code, rec.Body.String(), want)
			}
			if (outerIncoming == innerIncoming) == forward {
				t.Fatal("replacement forwarding did not choose the incoming object")
			}
		})
	}
}

func TestTypedBeforeHookAndNoHookInitialization(t *testing.T) {
	before := RequestHandle[struct{}, hookLocals](func(ctx context.Context, event hookEvent) (hookEvent, error) {
		event.Locals = &hookLocals{Name: "before-returned"}
		return event, nil
	})
	if rec := get(t, before.Middleware().Intercept(HandleConfig{}, emptyHookParams, localsEndpoint(t)), "/locals"); rec.Code != 200 || rec.Body.String() != "before-returned" {
		t.Fatalf("before hook: %d %s", rec.Code, rec.Body.String())
	}
	var previous *hookLocals
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := RequestLocals[hookLocals](r.Context())
		if p == nil || p == previous || p.Name != "" {
			t.Error("no-hook object is not fresh")
		}
		previous = p
		p.Name = "request-only"
		fmt.Fprint(w, "fresh-empty")
	})
	h := hookMiddleware(nil).Intercept(HandleConfig{}, emptyHookParams, next)
	for range 2 {
		if rec := get(t, h, "/locals"); rec.Code != 200 || rec.Body.String() != "fresh-empty" {
			t.Fatal("no-hook response missing")
		}
	}
	if RequestLocals[hookLocals](context.Background()) != nil {
		t.Fatal("unbound helper allocated locals")
	}
}

func TestTypedRefusalNilAndResolveGuards(t *testing.T) {
	for _, tc := range []struct {
		name, path, body string
		status           int
	}{
		{"page", "/locals", "{\"message\":\"skgo: Locals must not be nil when resolving a request\",\"status\":500}\n", 500},
		{"data", "/locals/__data.json", "{\"status\":500,\"message\":\"skgo: Locals must not be nil when resolving a request\"}", 500},
		{"remote", "/_app/remote/test/query", "{\"type\":\"error\",\"error\":{\"status\":500,\"message\":\"skgo: Locals must not be nil when resolving a request\"}}", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reached := false
			hook := hookMiddleware(func(ctx context.Context, event hookEvent, resolve hookResolve) (*http.Response, error) {
				event.Locals = nil
				return resolve(ctx, event)
			})
			h := hook.Intercept(HandleConfig{}, emptyHookParams, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true }))
			rec := get(t, h, tc.path)
			if rec.Code != tc.status || rec.Body.String() != tc.body || reached {
				t.Fatalf("nil forwarding: %d %q, reached=%v", rec.Code, rec.Body.String(), reached)
			}
		})
	}
	t.Run("refusal-keeps-incoming", func(t *testing.T) {
		called := false
		h := hookMiddleware(func(ctx context.Context, event hookEvent, _ hookResolve) (*http.Response, error) {
			event.Locals = &hookLocals{Name: "unforwarded"}
			return nil, Errorf(403, "refused")
		}).Intercept(HandleConfig{HandleError: func(ctx context.Context, caught CaughtError) map[string]any {
			called = true
			if RequestLocals[hookLocals](ctx).Name != "" {
				t.Error("refusal rebound an unforwarded object")
			}
			return nil
		}}, emptyHookParams, localsEndpoint(t))
		rec := get(t, h, "/locals")
		if rec.Code != 403 || !strings.Contains(rec.Body.String(), "refused") || !called {
			t.Fatalf("refusal: %d %s called=%v", rec.Code, rec.Body.String(), called)
		}
	})
	t.Run("nested-refusal-keeps-inner-incoming", func(t *testing.T) {
		seen := ""
		outer := hookMiddleware(func(ctx context.Context, event hookEvent, resolve hookResolve) (*http.Response, error) {
			event.Locals = &hookLocals{Name: "outer-selected"}
			return resolve(ctx, event)
		})
		inner := hookMiddleware(func(ctx context.Context, event hookEvent, _ hookResolve) (*http.Response, error) {
			event.Locals = &hookLocals{Name: "inner-unforwarded"}
			return nil, Errorf(403, "inner-refused")
		})
		cfg := HandleConfig{HandleError: func(ctx context.Context, caught CaughtError) map[string]any {
			seen = RequestLocals[hookLocals](ctx).Name
			return nil
		}}
		rec := get(t, RequestSequence(outer, inner).Intercept(cfg, emptyHookParams, localsEndpoint(t)), "/locals")
		if rec.Code != 403 || seen != "outer-selected" {
			t.Fatalf("nested refusal: %d locals=%q", rec.Code, seen)
		}
	})
	for _, sequence := range []bool{false, true} {
		t.Run(fmt.Sprintf("resolve-guards-sequence=%v", sequence), func(t *testing.T) {
			h := hookMiddleware(func(ctx context.Context, event hookEvent, resolve hookResolve) (*http.Response, error) {
				response, err := resolve(ctx, event)
				if err != nil {
					return nil, err
				}
				if _, err := resolve(ctx, event); err == nil || !strings.Contains(err.Error(), "already called") {
					t.Error("second resolve accepted")
				}
				return response, nil
			})
			if sequence {
				h = RequestSequence(h, hookMiddleware(nil))
			}
			if rec := get(t, h.Intercept(HandleConfig{}, emptyHookParams, localsEndpoint(t)), "/locals"); rec.Code != 200 {
				t.Fatal(rec.Code)
			}
		})
	}
	hook := hookMiddleware(func(ctx context.Context, event hookEvent, resolve hookResolve) (*http.Response, error) {
		return resolve(ctx, event, ResolveOptions{}, ResolveOptions{})
	})
	rec := get(t, hook.Intercept(HandleConfig{}, emptyHookParams, localsEndpoint(t)), "/locals")
	if rec.Code != 500 || rec.Body.String() != "{\"message\":\"Internal Error\",\"status\":500}\n" {
		t.Fatalf("options arity: %d %s", rec.Code, rec.Body.String())
	}
}

func TestTypedLocalsMismatchUsesPanicBoundary(t *testing.T) {
	panics := 0
	cfg := HandleConfig{OnPanic: func(id string, value any, stack []byte) {
		panics++
		if !strings.Contains(fmt.Sprint(value), "hookLocals") || !strings.Contains(fmt.Sprint(value), "int") {
			t.Errorf("mismatch diagnostic: %v", value)
		}
	}}
	h := hookMiddleware(func(ctx context.Context, event hookEvent, resolve hookResolve) (*http.Response, error) {
		RequestLocals[int](ctx)
		return resolve(ctx, event)
	}).Intercept(cfg, emptyHookParams, localsEndpoint(t))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/locals", nil))
	if rec.Code != 500 || panics != 1 {
		t.Fatalf("mismatch: %d panics=%d", rec.Code, panics)
	}
}

func TestTypedHookUsesConvertedSnapshotOnce(t *testing.T) {
	manifest := Manifest{Routes: []ManifestRoute{{ID: "/locals/[n=Number]", Pattern: "^/locals/([^/]+?)/?$", Params: []ManifestParam{{Name: "n", Matcher: "Number"}}, Endpoint: &ManifestEndpoint{Methods: []string{"GET"}}}}}
	calls := 0
	matchers := map[string]ParamMatcher{"Number": func(value string) (any, bool) {
		calls++
		if value != "0042" {
			return nil, false
		}
		return 42, true
	}}
	endpointCfg := manifest.EndpointConfig("http://localhost")
	endpointCfg.Matchers = matchers
	endpoints, err := NewEndpoints(endpointCfg, NewEndpoint("/locals/[n=Number]", "GET", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, RequestLocals[hookLocals](r.Context()).Name)
	}))
	if err != nil {
		t.Fatal(err)
	}
	cfg := manifest.HandleConfig()
	cfg.Matchers = matchers
	constructor := func(event *Event) (int, error) {
		id, values := HookValues(event)
		if id != "/locals/[n=Number]" || values["n"] != 42 {
			return 0, fmt.Errorf("wrong converted snapshot %s %v", id, values)
		}
		return values["n"].(int), nil
	}
	hook := RequestMiddleware[int, hookLocals](func(ctx context.Context, event RequestEvent[int, hookLocals], resolve RequestResolve[int, hookLocals]) (*http.Response, error) {
		if event.Params != 42 {
			t.Fatal("hook received wrong converted params")
		}
		event.Locals.Name = "typed-42"
		return resolve(ctx, event)
	})
	rec := get(t, hook.Intercept(cfg, constructor, endpoints.Intercept(http.NotFoundHandler())), "/locals/0042")
	if rec.Code != 200 || rec.Body.String() != "typed-42" || calls != 1 {
		t.Fatalf("snapshot: %d %q matcher calls=%d", rec.Code, rec.Body.String(), calls)
	}
}

func TestTypedForwardingWithIndependentResolveContext(t *testing.T) {
	hook := hookMiddleware(func(ctx context.Context, event hookEvent, resolve hookResolve) (*http.Response, error) {
		event.Locals = &hookLocals{Name: "selected-with-new-context"}
		return resolve(context.Background(), event)
	})
	rec := get(t, hook.Intercept(HandleConfig{}, emptyHookParams, localsEndpoint(t)), "/locals")
	if rec.Code != 200 || rec.Body.String() != "selected-with-new-context" {
		t.Fatalf("independent context forwarding: %d %s", rec.Code, rec.Body.String())
	}
}

func TestTypedNestedHookPanicRetainsIncomingLocals(t *testing.T) {
	outer := hookMiddleware(func(ctx context.Context, event hookEvent, resolve hookResolve) (*http.Response, error) {
		event.Locals = &hookLocals{Name: "outer-selected"}
		return resolve(ctx, event)
	})
	inner := hookMiddleware(func(ctx context.Context, event hookEvent, resolve hookResolve) (*http.Response, error) {
		event.Locals = &hookLocals{Name: "unforwarded"}
		panic("inner panic")
	})
	seen := ""
	panics := 0
	cfg := HandleConfig{OnPanic: func(string, any, []byte) { panics++ }, HandleError: func(ctx context.Context, _ CaughtError) map[string]any {
		seen = RequestLocals[hookLocals](ctx).Name
		return nil
	}}
	rec := get(t, RequestSequence(outer, inner).Intercept(cfg, emptyHookParams, localsEndpoint(t)), "/locals")
	if rec.Code != 500 || seen != "outer-selected" || panics != 1 {
		t.Fatalf("nested panic: %d locals=%s panics=%d", rec.Code, seen, panics)
	}
}
