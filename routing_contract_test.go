package skgo

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tylergannon/skgo/internal/ssr"
)

// Kit 3.0.0 url.decode_pathname and routing.parse_route_id/exec specify these
// paths and receipts. In particular the optional capture must call Lang;
// /[[lang=Lang]]/[id] would backtrack without exercising rejection at all.
func TestSharedRoutingContracts(t *testing.T) {
	for _, dev := range []bool{false, true} {
		t.Run(fmt.Sprintf("dev=%t", dev), func(t *testing.T) {
			for _, fixture := range []struct {
				id, pattern, path, receipt string
				params                     []ManifestParam
				langCalls                  int
			}{
				{"/items/[id]", "^/items/([^/]+?)/?$", "/items/%2525", "lang:absent|id:%25|rest:absent", []ManifestParam{{Name: "id"}}, 0},
				{"/items/[id]", "^/items/([^/]+?)/?$", "/items/a%2Fb", "lang:absent|id:a/b|rest:absent", []ManifestParam{{Name: "id"}}, 0},
				{"/items/[id]", "^/items/([^/]+?)/?$", "/items/%E2%9C%93", "lang:absent|id:✓|rest:absent", []ManifestParam{{Name: "id"}}, 0},
				{"/[[lang=Lang]]/[[id]]", "^(?:/([^/]+))?(?:/([^/]+))?/?$", "/abc", "lang:absent|id:abc|rest:absent", []ManifestParam{{Name: "lang", Matcher: "Lang", Optional: true, Chained: true}, {Name: "id", Optional: true, Chained: true}}, 1},
				{"/[[lang=Lang]]/[...rest]", "^(?:/([^/]+))?(?:/([^]*))?/?$", "/abc/def", "lang:absent|id:absent|rest:abc/def", []ManifestParam{{Name: "lang", Matcher: "Lang", Optional: true, Chained: true}, {Name: "rest", Rest: true, Chained: true}}, 1},
			} {
				t.Run(fixture.path+fixture.id, func(t *testing.T) {
					langCalls, loadCalls, remoteCalls, hookCalls := 0, 0, 0, 0
					matchers := CallerMatchers{"Lang": func(value string) (any, bool) {
						langCalls++
						if value != "abc" {
							t.Errorf("Lang saw %q, want captured abc", value)
						}
						return value, false
					}}
					routes := []ManifestRoute{{ID: fixture.id, Pattern: fixture.pattern, Params: fixture.params, Page: &ManifestPage{Leaf: 0}}}
					load := NewServerLoad(LoadSpec{Module: "fixture", Matchers: matchers, Run: func(ctx context.Context) (any, error) {
						loadCalls++
						e := EventFrom(ctx)
						values := map[string]any{}
						for _, key := range []string{"lang", "id", "rest"} {
							if value := OptionalLoadParamValue[string](e, key); value != nil {
								values[key] = *value
								TrackLoadParam(e, key)
								TrackLoadParam(e, key) // A repeat read must not duplicate the dependency.
							}
						}
						return struct {
							Receipt string `json:"receipt"`
						}{routingReceipt(values)}, nil
					}})
					ls, err := NewLoads(LoadConfig{Dev: dev, Nodes: []string{"fixture"}, Routes: routes}, load)
					if err != nil {
						t.Fatal(err)
					}
					command := NewRemote(RemoteSpec{Kind: KindCommand, Module: testModule, Name: "routingReceipt", CallerMatchers: matchers, Call: func(ctx context.Context, _ Call) (any, error) {
						remoteCalls++
						id, values := RemoteCallerValues(EventFrom(ctx))
						if id != fixture.id {
							return nil, fmt.Errorf("wrong route %q", id)
						}
						return routingReceipt(values), nil
					}})
					rs := testRemotes(t, RemoteConfig{Dev: dev, Routes: routes}, command)
					// A small render fixture lets the real document handler run its
					// loads and assembly; the actual Kit renderer is exercised by the
					// production/dev example decoding tests.
					engine, err := ssr.New("routing.js", []byte(`globalThis.__skgo_ping=()=> 'ok';globalThis.__skgo_render=()=>({done:true,status:200,body:'<p>routing fixture rendered</p>',head:''});`), 1, nil)
					if err != nil {
						t.Fatal(err)
					}
					s := streamer(nil)
					s.loads, s.remotes, s.engine = ls, rs, engine
					pages := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if !s.serve(w, r, r.URL.Path) {
							http.NotFound(w, r)
						}
					})
					plain := ls.Intercept(rs.Intercept(pages))
					hook := Middleware(func(ctx context.Context, e *Event, resolve Resolve) (*http.Response, error) {
						hookCalls++
						values := map[string]any{}
						for k, v := range e.Params() {
							values[k] = v
						}
						if e.RouteID() != fixture.id || routingReceipt(values) != fixture.receipt {
							t.Errorf("hook route/params: %q %v", e.RouteID(), values)
						}
						return resolve(ctx)
					})
					cfg := (Manifest{Routes: routes}).HandleConfig()
					cfg.Loads = ls
					withHook := hook.Intercept(cfg, plain)
					for _, entry := range []struct {
						handler http.Handler
						hooks   int
					}{{plain, 0}, {withHook, 1}} {
						h := entry.handler
						for _, kind := range []string{"document", "data", "remote"} {
							langCalls, loadCalls, remoteCalls, hookCalls = 0, 0, 0, 0
							path, method, body := fixture.path, "GET", ""
							if kind == "data" {
								path += dataSuffix
							}
							if kind == "remote" {
								path, method, body = rs.Prefix()+command.ID(), "POST", `{"payload":"","refreshes":[]}`
							}
							req := httptest.NewRequest(method, path, strings.NewReader(body))
							if kind == "remote" {
								req.Header.Set("x-sveltekit-pathname", fixture.path)
							}
							rec := httptest.NewRecorder()
							h.ServeHTTP(rec, req)
							if rec.Code != 200 || !strings.Contains(rec.Body.String(), fixture.receipt) {
								t.Fatalf("%s: want literal %q, got %d %s", kind, fixture.receipt, rec.Code, rec.Body.String())
							}
							if hookCalls != entry.hooks {
								t.Fatalf("%s: hook called %d times, want %d", kind, hookCalls, entry.hooks)
							}
							wantLang := fixture.langCalls * (1 + entry.hooks)
							if langCalls != wantLang {
								t.Fatalf("%s: Lang called %d times, want %d", kind, langCalls, wantLang)
							}
							if kind == "remote" {
								if remoteCalls != 1 || loadCalls != 0 {
									t.Fatal("remote callback did not execute exactly once")
								}
							} else if loadCalls != 1 || remoteCalls != 0 {
								t.Fatal("load callback did not execute exactly once")
							}
						}
						for _, malformed := range []string{"/items/%FF", "/items/%C0%AF", "/items/%ED%A0%80"} {
							for _, kind := range []string{"document", "data", "remote"} {
								loadCalls, remoteCalls, hookCalls, langCalls = 0, 0, 0, 0
								path, method, body := malformed, "GET", ""
								if kind == "data" {
									path += dataSuffix
								}
								if kind == "remote" {
									path, method, body = rs.Prefix()+command.ID(), "POST", `{"payload":"","refreshes":[]}`
								}
								req := httptest.NewRequest(method, path, strings.NewReader(body))
								if kind == "remote" {
									req.Header.Set("x-sveltekit-pathname", malformed)
								}
								rec := httptest.NewRecorder()
								h.ServeHTTP(rec, req)
								if rec.Code != 400 || loadCalls != 0 || remoteCalls != 0 || hookCalls != 0 || langCalls != 0 {
									t.Fatalf("malformed %s %q: %d; callbacks load=%d remote=%d hook=%d matcher=%d", kind, malformed, rec.Code, loadCalls, remoteCalls, hookCalls, langCalls)
								}
							}
						}
					}
				})
			}
		})
	}
}

func routingReceipt(values map[string]any) string {
	parts := []string{}
	for _, key := range []string{"lang", "id", "rest"} {
		value, present := values[key]
		if !present {
			value = "absent"
		}
		parts = append(parts, fmt.Sprintf("%s:%v", key, value))
	}
	return strings.Join(parts, "|")
}
