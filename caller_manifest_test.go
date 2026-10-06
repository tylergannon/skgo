package skgo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/tylergannon/skgo/internal/vite"
)

func callerManifestFixture() ([]ManifestRoute, CallerRoutes) {
	routes := []ManifestRoute{
		{ID: "/choice/[id=Numeric]", Pattern: "^/choice/([^/]+?)/?$", Params: []ManifestParam{{Name: "id", Matcher: "Numeric"}}, Page: &ManifestPage{Leaf: 0}},
		{ID: "/choice/[id]", Pattern: "^/choice/([^/]+?)/?$", Params: []ManifestParam{{Name: "id"}}, Page: &ManifestPage{Leaf: 0}},
		{ID: "/elsewhere/[slug]", Pattern: "^/elsewhere/([^/]+?)/?$", Params: []ManifestParam{{Name: "slug"}}, Page: &ManifestPage{Leaf: 0}},
	}
	generated := CallerRoutes{}
	for _, route := range routes {
		generated[route.ID] = CallerRoute{Params: append([]ManifestParam(nil), route.Params...), NewParams: func(map[string]any) (any, error) { return struct{}{}, nil }}
	}
	return routes, generated
}

// Production refuses the served snapshot at startup, even when drift is on
// a route the request would never select. Dev refuses the live snapshot at
// each real entry before it can run the otherwise valid fallback or callback.
func TestCallerManifestDriftBeforeFallback(t *testing.T) {
	for _, defect := range []string{"unknown key", "unknown matcher first candidate", "known matcher changed", "optional changed", "missing constructor", "constructor rejects", "unknown route", "outside selected route"} {
		for _, dev := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/dev=%t", defect, dev), func(t *testing.T) {
				routes, generated := callerManifestFixture()
				calls, matcherCalls := 0, 0
				form := NewRemote(RemoteSpec{Kind: KindForm, Module: testModule, Name: "manifestForm", CallerRoutes: generated,
					CallerMatchers: CallerMatchers{"Numeric": func(string) (any, bool) { matcherCalls++; return 0, false }, "Other": func(string) (any, bool) { matcherCalls++; return "other", true }},
					Call:           func(context.Context, Call) (any, error) { calls++; return "callback must not run", nil },
				})
				// The running Go binary was generated for this original graph.
				rs := testRemotes(t, RemoteConfig{Dev: dev, Routes: routes}, form)
				want := "parameter metadata differs"
				switch defect {
				case "unknown key":
					routes[0].Params[0].Name = "newkey"
				case "unknown matcher first candidate":
					routes[0].Params[0].Matcher = "Unbuilt"
					want = "unknown matcher"
				case "known matcher changed":
					routes[0].Params[0].Matcher = "Other"
				case "optional changed":
					routes[0].Params[0].Optional = true
				case "missing constructor":
					row := generated[routes[0].ID]
					row.NewParams = nil
					generated[routes[0].ID] = row
					want = "no generated Params constructor"
				case "constructor rejects":
					row := generated[routes[0].ID]
					row.NewParams = func(map[string]any) (any, error) { return nil, fmt.Errorf("constructor removed") }
					generated[routes[0].ID] = row
					want = "constructor removed"
				case "unknown route":
					routes[2].ID = "/new/[slug]"
					want = "no generated Params constructor"
				case "outside selected route":
					routes[2].Params[0].Name = "newkey"
				}
				if !dev {
					m := Manifest{Routes: routes, Remotes: []string{form.ID()}}
					_, err := NewRemotes(m.RemoteConfig(""), form)
					if err == nil || !strings.Contains(err.Error(), "caller manifest drift") || !strings.Contains(err.Error(), want) {
						t.Fatalf("startup: %v, want %s", err, want)
					}
					// The renderer must validate the snapshot it actually serves,
					// even if the registry was built from an earlier valid snapshot.
					if _, err := NewSSR(nil, m, nil, rs, SSROptions{}); err == nil || !strings.Contains(err.Error(), want) {
						t.Fatalf("renderer startup accepted a different snapshot: %v", err)
					}
				} else {
					// Vite has published a new graph while the compiled Go registry
					// is still the old one. This is the live transport refresh uses.
					raw, err := json.Marshal(routes)
					if err != nil {
						t.Fatal(err)
					}
					answer := vite.Info{Entry: "entry.js", Client: vite.Client{Start: "/start.js", App: "/app.js"}, Manifest: vite.Manifest{Version: 1, Nodes: []string{}, Routes: raw, SSR: json.RawMessage(`{"nodes":[]}`), Template: "%sveltekit.head%%sveltekit.body%", ErrorTemplate: "error"}}
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.URL.Path != "/__skgo_dev/info" {
							t.Errorf("unexpected transport %s", r.URL.Path)
						}
						json.NewEncoder(w).Encode(answer)
					}))
					defer server.Close()
					if _, err := NewDevSSR(nil, Manifest{SSR: &ManifestSSR{}}, nil, rs, server.URL, SSROptions{}); err == nil || !strings.Contains(err.Error(), want) {
						t.Fatalf("dev startup accepted a live snapshot ahead of Go: %v", err)
					}
					// The live response is ahead of the running Go registry;
					// initialize loads from its original, valid graph too.
					originalRoutes, _ := callerManifestFixture()
					ls, err := NewLoads(LoadConfig{Dev: true, Routes: originalRoutes, Matchers: map[string]ParamMatcher(form.callerMatchers)})
					if err != nil {
						t.Fatal(err)
					}
					s := &SSR{dev: vite.NewDev(server.URL), loads: ls, remotes: rs}
					h := NewDevPages(mustURL(t, server.URL), Manifest{}, s, nil)
					served := ls.Intercept(rs.Intercept(h))
					for _, path := range []string{"/choice/word", "/choice/word/__data.json", "/choice/word?/remote=" + form.ID(), "/choice/word?/remote=" + form.ID() + "/key", rs.Prefix() + form.ID()} {
						method, body := "GET", ""
						if strings.Contains(path, "remote") {
							method, body = "POST", "name=fixture"
						}
						req := httptest.NewRequest(method, path, strings.NewReader(body))
						req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
						req.Header.Set("x-sveltekit-pathname", "/choice/word")
						rec := httptest.NewRecorder()
						served.ServeHTTP(rec, req)
						// Kit remote errors use a 200 error envelope; pages/data fail
						// with 502. Both must visibly carry the operator diagnosis.
						if !strings.Contains(rec.Body.String(), "caller manifest drift") || !strings.Contains(rec.Body.String(), want) {
							t.Fatalf("%s: %d %s, want %s", path, rec.Code, rec.Body.String(), want)
						}
						if strings.HasPrefix(path, rs.Prefix()) {
							if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"type":"error"`) || !strings.Contains(rec.Body.String(), `"status":503`) {
								t.Fatalf("enhanced error envelope: %d %s", rec.Code, rec.Body.String())
							}
						} else if rec.Code != http.StatusBadGateway {
							t.Fatalf("document/data drift status %d, want 502", rec.Code)
						}
						if s.devVersion != 0 {
							t.Fatal("invalid snapshot was accepted")
						}
					}
				}
				if calls != 0 || matcherCalls != 0 {
					t.Fatalf("callbacks remote=%d matcher=%d, want zero before fallback", calls, matcherCalls)
				}
			})
		}
	}
}

func TestCallerManifestRequiresGeneratedMetadata(t *testing.T) {
	form := NewForm(testModule, "unbuilt", func(context.Context, map[string]any) (string, error) { t.Fatal("callback ran"); return "", nil })
	for _, dev := range []bool{false, true} {
		cfg := (Manifest{Remotes: []string{form.ID()}}).RemoteConfig("")
		cfg.Dev = dev
		if _, err := NewRemotes(cfg, form); err == nil || !strings.Contains(err.Error(), "no generated caller metadata") {
			t.Fatalf("dev=%t: %v", dev, err)
		}
	}
}

// Query-only applications do not generate shared caller Params. Their loads
// still own the route matchers, and a query must never run caller conversion.
func TestCallerManifestDoesNotRequireQueryCallerMetadata(t *testing.T) {
	for _, dev := range []bool{false, true} {
		t.Run(fmt.Sprintf("dev=%t", dev), func(t *testing.T) {
			matcherCalls := 0
			query := NewQueryNoArg(testModule, "queryOnly", func(context.Context) (string, error) { return "query:headerless", nil })
			m := Manifest{Nodes: []string{"fixture"}, Loads: []string{"fixture"}, Remotes: []string{query.ID()}, Routes: []ManifestRoute{{ID: "/choice/[id=Numeric]", Pattern: "^/choice/([^/]+?)/?$", Params: []ManifestParam{{Name: "id", Matcher: "Numeric"}}, Page: &ManifestPage{Leaf: 0}}}}
			cfg := m.RemoteConfig("")
			cfg.Dev = dev
			rs, err := NewRemotes(cfg, query)
			if err != nil {
				t.Fatal(err)
			}
			load := NewServerLoad(LoadSpec{Module: "fixture", Matchers: map[string]ParamMatcher{"Numeric": func(value string) (any, bool) { matcherCalls++; n, e := strconv.Atoi(value); return n, e == nil }}, Run: func(ctx context.Context) (any, error) {
				return struct {
					Receipt string `json:"receipt"`
				}{fmt.Sprintf("load:%d", LoadParamValue[int](EventFrom(ctx), "id"))}, nil
			}})
			lc := m.LoadConfig("")
			lc.Dev = dev
			ls, err := NewLoads(lc, load)
			if err != nil {
				t.Fatal(err)
			}
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("GET", rs.Prefix()+query.ID()+"/", nil)
			req.Header.Set("x-sveltekit-pathname", "/choice/42")
			rs.ServeHTTP(rec, req)
			if rec.Code != 200 || !strings.Contains(rec.Body.String(), "query:headerless") || matcherCalls != 0 {
				t.Fatalf("query: %d %s matcher=%d", rec.Code, rec.Body.String(), matcherCalls)
			}
			rec = httptest.NewRecorder()
			ls.ServeHTTP(rec, httptest.NewRequest("GET", "/choice/42/__data.json", nil))
			if rec.Code != 200 || !strings.Contains(rec.Body.String(), "load:42") || matcherCalls != 1 {
				t.Fatalf("load: %d %s matcher=%d", rec.Code, rec.Body.String(), matcherCalls)
			}
		})
	}
}
