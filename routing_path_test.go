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

// Literal Kit 3.0.0 parse_route_id/exec fixtures. The optional patterns force
// Lang to see abc; a required trailing segment would backtrack before Lang ran.
func TestLoadRoutingPathContracts(t *testing.T) {
	for _, fixture := range []struct {
		id, pattern, path, receipt string
		params                     []ManifestParam
		matcherCalls               int
	}{
		{"/items/[id]", "^/items/([^/]+?)/?$", "/items/%2525", "lang:absent|id:%25|rest:absent", []ManifestParam{{Name: "id"}}, 0},
		{"/items/[id]", "^/items/([^/]+?)/?$", "/items/a%2Fb", "lang:absent|id:a/b|rest:absent", []ManifestParam{{Name: "id"}}, 0},
		{"/items/[id]", "^/items/([^/]+?)/?$", "/items/%2F", "lang:absent|id:/|rest:absent", []ManifestParam{{Name: "id"}}, 0},
		{"/docs/[...rest]", "^/docs(?:/([^]*))?/?$", "/docs/a%2Fb", "lang:absent|id:absent|rest:a/b", []ManifestParam{{Name: "rest", Rest: true, Chained: true}}, 0},
		{"/items/[id]", "^/items/([^/]+?)/?$", "/items/%E2%9C%93", "lang:absent|id:✓|rest:absent", []ManifestParam{{Name: "id"}}, 0},
		{"/[[lang=Lang]]/[[id]]", "^(?:/([^/]+))?(?:/([^/]+))?/?$", "/abc", "lang:absent|id:abc|rest:absent", []ManifestParam{{Name: "lang", Matcher: "Lang", Optional: true, Chained: true}, {Name: "id", Optional: true, Chained: true}}, 1},
		{"/[[lang=Lang]]/[...rest]", "^(?:/([^/]+))?(?:/([^]*))?/?$", "/abc/def", "lang:absent|id:absent|rest:abc/def", []ManifestParam{{Name: "lang", Matcher: "Lang", Optional: true, Chained: true}, {Name: "rest", Rest: true, Chained: true}}, 1},
	} {
		t.Run(fixture.path+fixture.id, func(t *testing.T) {
			calls, matched := 0, 0
			routes := []ManifestRoute{{ID: fixture.id, Pattern: fixture.pattern, Params: fixture.params, Page: &ManifestPage{Leaf: 0}}}
			load := NewServerLoad(LoadSpec{Module: "fixture", Matchers: map[string]ParamMatcher{"Lang": func(value string) (any, bool) {
				matched++
				if value != "abc" {
					t.Errorf("Lang received %q, want abc", value)
				}
				return value, false
			}}, Run: func(ctx context.Context) (any, error) {
				calls++
				e := EventFrom(ctx)
				parts := []string{}
				for _, key := range []string{"lang", "id", "rest"} {
					value := "absent"
					if ptr := OptionalLoadParamValue[string](e, key); ptr != nil {
						value = *ptr
					}
					parts = append(parts, key+":"+value)
				}
				return struct {
					Receipt string `json:"receipt"`
				}{strings.Join(parts, "|")}, nil
			}})
			ls, err := NewLoads(LoadConfig{Nodes: []string{"fixture"}, Routes: routes}, load)
			if err != nil {
				t.Fatal(err)
			}
			engine, err := ssr.New("fixture.js", []byte(`globalThis.__skgo_ping=()=> 'ok';globalThis.__skgo_render=()=>({done:true,status:200,body:'<p>load fixture rendered</p>',head:''});`), 1, nil)
			if err != nil {
				t.Fatal(err)
			}
			s := streamer(nil)
			s.loads, s.engine = ls, engine
			s.remotes = testRemotes(t, RemoteConfig{})
			pages := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !s.serve(w, r, r.URL.Path) {
					http.NotFound(w, r)
				}
			})
			for _, data := range []bool{false, true} {
				t.Run(fmt.Sprintf("data=%t", data), func(t *testing.T) {
					calls, matched = 0, 0
					path := fixture.path
					if data {
						path += dataSuffix
					}
					rec := httptest.NewRecorder()
					ls.Intercept(pages).ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
					if rec.Code != 200 || !strings.Contains(rec.Body.String(), fixture.receipt) || calls != 1 || matched != fixture.matcherCalls {
						t.Fatalf("want %q and load=1 matcher=%d, got %d %s; load=%d matcher=%d", fixture.receipt, fixture.matcherCalls, rec.Code, rec.Body.String(), calls, matched)
					}
					for _, bad := range []string{"/items/%FF", "/items/%C0%AF", "/items/%ED%A0%80"} {
						calls, matched = 0, 0
						if data {
							bad += dataSuffix
						}
						rec = httptest.NewRecorder()
						ls.Intercept(pages).ServeHTTP(rec, httptest.NewRequest("GET", bad, nil))
						if rec.Code != 400 || calls != 0 || matched != 0 {
							t.Fatalf("malformed %q: %d; load=%d matcher=%d", bad, rec.Code, calls, matched)
						}
					}
				})
			}
		})
	}
}

func TestMatcherRoutesWithoutLoads(t *testing.T) {
	routes := []ManifestRoute{
		{ID: "/orders/[n=Order]", Pattern: "^/orders/([^/]+?)/?$", Params: []ManifestParam{{Name: "n", Matcher: "Order"}}, Page: &ManifestPage{Leaf: 0}},
		{ID: "/orders/[fallback]", Pattern: "^/orders/([^/]+?)/?$", Params: []ManifestParam{{Name: "fallback"}}, Page: &ManifestPage{Leaf: 0}},
	}
	matchers := map[string]ParamMatcher{"Order": func(value string) (any, bool) { return 42, value == "42" }}
	ls, err := NewLoads(LoadConfig{Nodes: []string{""}, Routes: routes, Matchers: matchers})
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"42", "banana"} {
		rec := httptest.NewRecorder()
		ls.Intercept(http.NotFoundHandler()).ServeHTTP(rec, httptest.NewRequest("GET", "/orders/"+value+dataSuffix, nil))
		if rec.Code != 200 || rec.Body.String() != "{\"type\":\"data\",\"nodes\":[null]}\n" {
			t.Fatalf("%s: %d %s", value, rec.Code, rec.Body.String())
		}
	}
	// The hook must select the same candidate even without a Loads registry.
	cfg := (Manifest{Routes: routes}).HandleConfig()
	cfg.Matchers = matchers
	for _, c := range []struct{ value, id string }{{"42", "/orders/[n=Order]"}, {"banana", "/orders/[fallback]"}} {
		route, _, _, ok := cfg.matchRoute("/orders/" + c.value)
		if !ok || route.id != c.id {
			t.Fatalf("hook %s: %+v %v", c.value, route, ok)
		}
	}
	if _, err := NewLoads(LoadConfig{Nodes: []string{""}, Routes: routes}); err == nil {
		t.Fatal("missing matcher must fail startup")
	}
}

func TestEndpointMatcherRejectionFallback(t *testing.T) {
	routes := []ManifestRoute{
		{ID: "/api/[n=Order]", Pattern: "^/api/([^/]+?)/?$", Params: []ManifestParam{{Name: "n", Matcher: "Order"}}, Endpoint: &ManifestEndpoint{Methods: []string{"GET"}}},
		{ID: "/api/[fallback]", Pattern: "^/api/([^/]+?)/?$", Params: []ManifestParam{{Name: "fallback"}}, Endpoint: &ManifestEndpoint{Methods: []string{"GET"}}},
	}
	cfg := EndpointConfig{Routes: routes, Matchers: map[string]ParamMatcher{"Order": func(value string) (any, bool) { return 42, value == "42" }}}
	es, err := NewEndpoints(cfg, NewEndpoint(routes[0].ID, "GET", echo("order42")), NewEndpoint(routes[1].ID, "GET", echo("fallbackbanana")))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ value, body string }{{"42", "order42"}, {"banana", "fallbackbanana"}} {
		rec := httptest.NewRecorder()
		es.Intercept(http.NotFoundHandler()).ServeHTTP(rec, httptest.NewRequest("GET", "/api/"+c.value, nil))
		if rec.Code != 200 || rec.Body.String() != c.body {
			t.Fatalf("%s: %d %s", c.value, rec.Code, rec.Body.String())
		}
	}
	cfg.Matchers = nil
	if _, err := NewEndpoints(cfg); err == nil {
		t.Fatal("missing matcher must fail startup")
	}
}
