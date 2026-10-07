package skgo

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tylergannon/polytype/devalue"
	"github.com/tylergannon/skgo/internal/formdata"
	"github.com/tylergannon/skgo/internal/ssr"
)

func metadataReceipt(e *Event) string {
	address, err := e.ClientAddress()
	if err != nil {
		address = "error:" + err.Error()
	}
	u := "nil"
	if logical := e.URL(); logical != nil {
		u = logical.String()
	}
	return fmt.Sprintf("%s|%s|%s|%t,%t,%t|%s", e.Request().URL.RequestURI(), u, e.RouteID(), e.IsDataRequest(), e.IsRemoteRequest(), e.IsSubRequest(), address)
}

func metadataManifest(base string) Manifest {
	return Manifest{AppDir: "_app", Base: base, Nodes: []string{"src/routes/+layout.server.ts", "src/routes/items/[id]/+page.server.ts"}, Routes: []ManifestRoute{
		{ID: "/items/[id]", Pattern: `^/items/([^/]+?)/?$`, Params: []ManifestParam{{Name: "id"}}, Page: &ManifestPage{Layouts: []int{0}, Leaf: 1}},
		{ID: "/api", Pattern: `^/api/?$`, Endpoint: &ManifestEndpoint{Methods: []string{"GET"}}},
		{ID: "/relay", Pattern: `^/relay/?$`, Endpoint: &ManifestEndpoint{Methods: []string{"GET"}}},
	}}
}

func TestRequestMetadataThroughServedLoadsEndpointsAndActions(t *testing.T) {
	for _, base := range []string{"", "/base"} {
		t.Run(base, func(t *testing.T) {
			manifest := metadataManifest(base)
			manifest.Routes = manifest.Routes[:2]
			load := func(ctx context.Context) (string, error) { return metadataReceipt(EventFrom(ctx)), nil }
			loads, err := NewLoads(manifest.LoadConfig("http://fixture.test"), NewLoad(manifest.Nodes[0], load), NewLoad(manifest.Nodes[1], load))
			if err != nil {
				t.Fatal(err)
			}
			actions, err := NewActions(NewPageAction(ActionSpec{Module: manifest.Nodes[1], Name: "save", Run: func(ctx context.Context) (any, error) {
				e := EventFrom(ctx)
				if q, ok := e.SearchParam("q"); !ok || q != "blue" || e.Param("id") != "42" {
					return nil, errors.New("classic action lost search or route params")
				}
				return metadataReceipt(e), nil
			}}))
			if err != nil {
				t.Fatal(err)
			}
			documents := &SSR{loads: loads, actions: actions, base: base}
			endpoints, err := NewEndpoints(manifest.EndpointConfig("http://fixture.test"), NewEndpoint("/api", "GET", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, metadataReceipt(EventFrom(r.Context()))) }))
			if err != nil {
				t.Fatal(err)
			}
			cfg := manifest.HandleConfig()
			cfg.Origin = "http://fixture.test"
			cfg.Loads = loads
			hook := hookMiddleware(func(ctx context.Context, event hookEvent, resolve hookResolve) (*http.Response, error) {
				receipt := metadataReceipt(event.Event)
				response, err := resolve(ctx, event)
				if err == nil {
					response.Header.Set("X-Hook-Metadata", receipt)
				}
				return response, err
			})
			served := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !documents.serve(w, r, r.URL.Path) {
					http.NotFound(w, r)
				}
			})
			h := hook.Intercept(cfg, emptyHookParams, loads.Intercept(endpoints.Intercept(served)))
			for _, tc := range []struct {
				name, method, path, logical, route, flags string
				status                                    int
			}{
				{"data", "GET", base + "/items/42/__data.json?q=blue&x-sveltekit-invalidated=11&x-sveltekit-trailing-slash=1", "http://fixture.test" + base + "/items/42/?q=blue", "/items/[id]", "true,false,false", 200},
				{"endpoint", "GET", base + "/api?q=blue", "http://fixture.test" + base + "/api?q=blue", "/api", "false,false,false", 200},
				{"action", "POST", base + "/items/42?/save&q=blue", "http://fixture.test" + base + "/items/42?/save&q=blue", "/items/[id]", "false,false,false", 200},
			} {
				t.Run(tc.name, func(t *testing.T) {
					r := httptest.NewRequest(tc.method, tc.path, strings.NewReader("name=Ada"))
					r.RemoteAddr = "[2001:db8::7]:1234"
					if tc.method == "POST" {
						r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
						r.Header.Set("Accept", "application/json")
						r.Header.Set("x-sveltekit-action", "true")
						r.Header.Set("Origin", "http://fixture.test")
					}
					rec := httptest.NewRecorder()
					h.ServeHTTP(rec, r)
					want := tc.path + "|" + tc.logical + "|" + tc.route + "|" + tc.flags + "|2001:db8::7"
					body := rec.Body.String()
					if tc.name == "action" {
						var answer struct {
							Data string `json:"data"`
						}
						if err := json.Unmarshal(rec.Body.Bytes(), &answer); err != nil {
							t.Fatal(err)
						}
						value, err := devalue.Parse(answer.Data, nil)
						if err != nil {
							t.Fatal(err)
						}
						body = fmt.Sprint(value)
					}
					if rec.Code != tc.status || rec.Header().Get("X-Hook-Metadata") != want || !strings.Contains(body, want) {
						t.Fatalf("want metadata %q; got %d hook=%q body=%s", want, rec.Code, rec.Header().Get("X-Hook-Metadata"), rec.Body.String())
					}
					if tc.name == "data" && strings.Count(rec.Body.String(), want) != 2 {
						t.Fatalf("both layout and page must report matched page: %s", rec.Body.String())
					}
				})
			}
			// A hook also observes unmatched requests. This literal body proves
			// the downstream handler ran; an empty response cannot satisfy it.
			h = hook.Intercept(cfg, emptyHookParams, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "unmatched-reached") }))
			r := httptest.NewRequest("GET", base+"/missing", nil)
			r.RemoteAddr = "192.0.2.8:2222"
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, r)
			want := base + "/missing|http://fixture.test" + base + "/missing||false,false,false|192.0.2.8"
			if rec.Code != 200 || rec.Body.String() != "unmatched-reached" || rec.Header().Get("X-Hook-Metadata") != want {
				t.Fatalf("unmatched metadata: %d %s %q", rec.Code, rec.Body.String(), rec.Header().Get("X-Hook-Metadata"))
			}
		})
	}
}

// Caller-property refusals are counted positively in both the current context
// and Request().Context(), so a query cannot recover the boundary's full event.
func restrictedMetadataReceipt(ctx context.Context) (string, error) {
	e := EventFrom(ctx)
	refused := 0
	for _, current := range []*Event{e, EventFrom(e.Request().Context())} {
		for _, read := range []func(){func() { current.URL() }, func() { current.RouteID() }, func() { current.SearchParam("q") }, func() { current.Params() }, func() { current.Param("id") }, func() { RemoteCallerValues(current) }} {
			func() {
				defer func() {
					if recover() != nil {
						refused++
					}
				}()
				read()
			}()
		}
	}
	if e.SetCookie("query", "forbidden", CookieOptions{}) == nil || e.SetHeader("X-Query", "forbidden") == nil {
		return "query-mutation-escaped", nil
	}
	address, err := e.ClientAddress()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("refused:%d|%s|%t,%t,%t|%s|%s", refused, e.Request().URL.RequestURI(), e.IsDataRequest(), e.IsRemoteRequest(), e.IsSubRequest(), address, RequestLocals[hookLocals](ctx).Name), nil
}

func TestRemoteRequestMetadataAndQueryRestrictionsAtServedEntries(t *testing.T) {
	const base = "/base"
	q := NewQueryNoArg(testModule, "metadataQuery", restrictedMetadataReceipt)
	b := NewBatchQuery(testModule, "metadataBatch", func(ctx context.Context, inputs []string) ([]string, error) {
		receipt, err := restrictedMetadataReceipt(ctx)
		return []string{receipt}, err
	})
	live := NewLiveQueryNoArg(testModule, "metadataLive", func(ctx context.Context, yield func(string) error) error {
		receipt, err := restrictedMetadataReceipt(ctx)
		if err != nil {
			return err
		}
		return yield(receipt)
	})
	command := NewCommandNoArg(testModule, "metadataCommand", func(ctx context.Context) (string, error) {
		e := EventFrom(ctx)
		// Generated command contexts deliberately narrow nested function access.
		derived := RequestEvent[struct{}, hookLocals]{Event: e, Locals: RequestLocals[hookLocals](ctx)}.Context()
		if receipt, err := restrictedMetadataReceipt(derived); err != nil || !strings.HasPrefix(receipt, "refused:12|") {
			return "", fmt.Errorf("nested context recovered caller: %s %v", receipt, err)
		}
		return metadataReceipt(e), RefreshNoArg(ctx, restrictedMetadataReceipt)
	})
	form := NewForm(testModule, "metadataForm", func(ctx context.Context, in draft) (string, error) { return metadataReceipt(EventFrom(ctx)), nil })
	manifest := metadataManifest(base)
	rs := testRemotes(t, RemoteConfig{Base: base, Origin: "http://fixture.test"}, q, b, live, command, form)
	cfg := manifest.HandleConfig()
	cfg.Origin = "http://fixture.test"
	hook := hookMiddleware(func(ctx context.Context, event hookEvent, resolve hookResolve) (*http.Response, error) {
		event.Locals.Name = "request-local"
		return resolve(ctx, event)
	})
	h := hook.Intercept(cfg, emptyHookParams, rs.Intercept(http.NotFoundHandler()))
	for _, tc := range []struct {
		name, method, path, body string
		query                    bool
	}{
		{"query", "GET", rs.Prefix() + q.ID(), "", true},
		{"batch", "POST", rs.Prefix() + b.ID(), `{"payloads":["WyJ4Il0"]}`, true},
		{"live", "GET", rs.Prefix() + live.ID(), "", true},
		{"command", "POST", rs.Prefix() + command.ID(), `{"payload":"","refreshes":[]}`, false},
		{"form", "POST", rs.Prefix() + form.ID(), formGoldenSubmission, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := io.Reader(strings.NewReader(tc.body))
			if tc.name == "form" {
				decoded, err := base64.StdEncoding.DecodeString(tc.body)
				if err != nil {
					t.Fatal(err)
				}
				body = strings.NewReader(string(decoded))
			}
			r := httptest.NewRequest(tc.method, tc.path, body)
			r.RemoteAddr = "192.0.2.11:1234"
			r.Header.Set("Origin", "http://fixture.test")
			r.Header.Set("x-sveltekit-pathname", base+"/items/42")
			r.Header.Set("x-sveltekit-search", "?q=blue")
			if tc.name == "form" {
				r.Header.Set("Content-Type", formdata.ContentType)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, r)
			want := tc.path + "|http://fixture.test/base/items/42?q=blue|/items/[id]|false,true,false|192.0.2.11"
			if tc.query {
				want = "refused:12|" + tc.path + "|false,true,false|192.0.2.11|request-local"
			}
			if rec.Code != 200 || !strings.Contains(rec.Body.String(), want) {
				t.Fatalf("want %q; got %d %s", want, rec.Code, rec.Body.String())
			}
			if tc.name == "command" {
				refresh := "refused:12|" + tc.path + "|false,true,false|192.0.2.11|request-local"
				if !strings.Contains(rec.Body.String(), refresh) {
					t.Fatalf("refresh metadata/restrictions lost: %s", rec.Body.String())
				}
			}
		})
	}
	// Kit's native/headerless remote caller has no matched page but keeps the
	// original remote transport URL as its logical URL.
	path := rs.Prefix() + command.ID()
	r := httptest.NewRequest("POST", path, strings.NewReader(`{"payload":"","refreshes":[]}`))
	r.RemoteAddr = "192.0.2.11:1234"
	r.Header.Set("Origin", "http://fixture.test")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	want := path + "|http://fixture.test" + path + "||false,true,false|192.0.2.11"
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), want) {
		t.Fatalf("headerless caller: want %q; got %d %s", want, rec.Code, rec.Body.String())
	}
}

func TestNestedSSRQueryKeepsPageRequestFlags(t *testing.T) {
	q := NewQueryNoArg(testModule, "metadataSSR", restrictedMetadataReceipt)
	rs := testRemotes(t, RemoteConfig{}, q)
	bundle := fmt.Sprintf(`globalThis.__skgo_ping=()=> 'ok';globalThis.__skgo_render=()=>{const result={done:false};__skgo_remote(%q,'').then(value=>{result.body=value;result.done=true},error=>{result.failure=String(error);result.done=true});return result;};`, q.ID())
	engine, err := ssr.New("metadata-query.js", []byte(bundle), 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	renderer := &SSR{remotes: rs, loads: &Loads{}, engine: engine}
	hook := hookMiddleware(func(ctx context.Context, event hookEvent, resolve hookResolve) (*http.Response, error) {
		event.Locals.Name = "ssr-local"
		return resolve(ctx, event)
	})
	h := hook.Intercept(metadataManifest("").HandleConfig(), emptyHookParams, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result, _, err := renderer.renderPlan(r, dataRequest{url: mustURL(t, "http://fixture.test/items/42")}, documentPlan{routeID: "/items/[id]", status: 200}, newDocumentCSPWithNonce(nil, "fixed"))
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		fmt.Fprint(w, result.Body)
	}))
	r := httptest.NewRequest("GET", "/items/42", nil)
	r.RemoteAddr = "192.0.2.11:1234"
	r.Header.Set("Origin", "http://fixture.test")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "refused:12|/items/42|false,false,false|192.0.2.11|ssr-local") {
		t.Fatalf("SSR query metadata: %d %s", rec.Code, rec.Body.String())
	}
}

func TestClientAddressProviderInheritedByInternalFetch(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			manifest := metadataManifest("/base")
			var calls atomic.Int32
			providerError := errors.New("fixture address unavailable")
			cfg := manifest.HandleConfig()
			cfg.Origin = "http://fixture.test"
			cfg.ClientAddress = func(r *http.Request) (string, error) {
				calls.Add(1)
				if fail {
					return "", providerError
				}
				return r.Header.Get("X-Fixture-Address"), nil
			}
			check := func(e *Event) (string, error) {
				value, err := e.ClientAddress()
				if fail {
					if !errors.Is(err, providerError) {
						return "", fmt.Errorf("lost provider error: %v", err)
					}
					return "provider-error-retained", nil
				}
				if err != nil {
					return "", err
				}
				return value, nil
			}
			endpoints, err := NewEndpoints(manifest.EndpointConfig("http://fixture.test"),
				NewEndpoint("/api", "GET", func(w http.ResponseWriter, r *http.Request) {
					if r.Header.Get("X-Fixture-Address") != "" || RequestLocals[hookLocals](r.Context()).Name != "child" {
						http.Error(w, "child inherited parent headers or locals", 500)
						return
					}
					e := EventFrom(r.Context())
					value, err := check(e)
					if err != nil {
						http.Error(w, err.Error(), 500)
						return
					}
					fmt.Fprintf(w, "child:%s:%t,%t,%t", value, e.IsDataRequest(), e.IsRemoteRequest(), e.IsSubRequest())
				}),
				NewEndpoint("/relay", "GET", func(w http.ResponseWriter, r *http.Request) {
					e := EventFrom(r.Context())
					request, _ := http.NewRequest("GET", "/base/api", nil)
					// The child makes the first address read. The provider must still
					// receive the original request and its forwarded header.
					response, err := e.Fetch(r.Context(), request)
					if err != nil {
						http.Error(w, err.Error(), 500)
						return
					}
					defer response.Body.Close()
					body, err := io.ReadAll(response.Body)
					if err != nil {
						http.Error(w, err.Error(), 500)
						return
					}
					value, err := check(e)
					if err != nil {
						http.Error(w, err.Error(), 500)
						return
					}
					fmt.Fprintf(w, "parent:%s:%s", value, body)
				}))
			if err != nil {
				t.Fatal(err)
			}
			hook := hookMiddleware(func(ctx context.Context, event hookEvent, resolve hookResolve) (*http.Response, error) {
				event.Locals.Name = "parent"
				if event.IsSubRequest() {
					event.Locals.Name = "child"
				}
				return resolve(ctx, event)
			})
			var h http.Handler
			boundary := hook.Intercept(cfg, emptyHookParams, endpoints.Intercept(http.NotFoundHandler()))
			h = FetchConfig{Origin: "http://fixture.test", Base: "/base", Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { h.ServeHTTP(w, r) })}.Intercept(boundary)
			r := httptest.NewRequest("GET", "/base/relay", nil)
			r.Header.Set("X-Fixture-Address", "203.0.113.42")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, r)
			value := "203.0.113.42"
			if fail {
				value = "provider-error-retained"
			}
			want := "parent:" + value + ":child:" + value + ":false,false,true"
			if rec.Code != 200 || rec.Body.String() != want || calls.Load() != 1 {
				t.Fatalf("got %d %q calls=%d; want %q calls=1", rec.Code, rec.Body.String(), calls.Load(), want)
			}
		})
	}
}

func TestClientAddressIsLazyAndUnavailableWithoutHostingRequest(t *testing.T) {
	var calls atomic.Int32
	cfg := HandleConfig{ClientAddress: func(*http.Request) (string, error) { calls.Add(1); return "203.0.113.9", nil }}
	h := hookMiddleware(nil).Intercept(cfg, emptyHookParams, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "no-address-read") }))
	rec := get(t, h, "/unused")
	if rec.Code != 200 || rec.Body.String() != "no-address-read" || calls.Load() != 0 {
		t.Fatalf("provider should remain lazy: %d %s calls=%d", rec.Code, rec.Body.String(), calls.Load())
	}
	for _, e := range []*Event{nil, {}, {req: httptest.NewRequest("GET", "/no-peer", nil)}} {
		if e != nil && e.req != nil {
			e.req.RemoteAddr = ""
		}
		value, err := e.ClientAddress()
		if err == nil || value != "" {
			t.Fatalf("unavailable address fabricated %q, error %v", value, err)
		}
	}
	load := NewLoad("src/routes/address/+page.server.ts", func(ctx context.Context) (string, error) {
		value, err := EventFrom(ctx).ClientAddress()
		if value != "" || err == nil {
			return "fabricated-build-address", nil
		}
		return "build-address-unavailable", nil
	})
	answer := callPrerenderLoad(t, PrerenderLoadInput{Module: "src/routes/address/+page.server.ts", URL: "http://fixture.test/address", RouteID: "/address"}, load)
	value, err := devalue.Parse(string(answer.Data), nil)
	if err != nil || value != "build-address-unavailable" {
		t.Fatalf("build address = %v %v", value, err)
	}
	remote := NewRemote(RemoteSpec{Kind: KindPrerender, Module: testModule, Name: "buildAddress", Call: func(ctx context.Context, _ Call) (any, error) {
		value, err := EventFrom(ctx).ClientAddress()
		if value != "" || err == nil {
			return "fabricated-build-address", nil
		}
		return "build-address-unavailable", nil
	}})
	var output strings.Builder
	if err := callBuildOperation("/remote", strings.NewReader(`{"module":"src/lib/todos.remote.ts","name":"buildAddress","url":"http://fixture.test/address"}`), &output, nil, nil, []*Remote{remote}); err != nil {
		t.Fatal(err)
	}
	_, data, _ := envelope(t, []byte(output.String()))
	if got := field(t, data, "_"); got != "build-address-unavailable" {
		t.Fatalf("prerender remote address = %v", got)
	}
}

func TestSSRClientAddressHostIsLazySharedAndPreservesErrors(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			var calls atomic.Int32
			cfg := HandleConfig{Origin: "http://fixture.test", ClientAddress: func(r *http.Request) (string, error) {
				calls.Add(1)
				if fail {
					return "", errors.New("fixture renderer address unavailable")
				}
				return r.Header.Get("X-Fixture-Address"), nil
			}}
			bundle := `globalThis.__skgo_ping=()=> 'ok';globalThis.__skgo_render=(raw)=>{const req=JSON.parse(raw);let address;try{address=__skgo_client_address()+':'+__skgo_client_address()}catch(error){address=String(error)}return {done:true,status:200,body:'render:'+address+':sub='+req.is_sub_request}};`
			engine, err := ssr.New("address.js", []byte(bundle), 1, nil)
			if err != nil {
				t.Fatal(err)
			}
			renderer := &SSR{engine: engine, remotes: testRemotes(t, RemoteConfig{}), loads: &Loads{}}
			h := hookMiddleware(nil).Intercept(cfg, emptyHookParams, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				result, _, err := renderer.renderPlan(r, dataRequest{url: mustURL(t, "http://fixture.test/render")}, documentPlan{routeID: "/render", status: 200}, newDocumentCSPWithNonce(nil, "fixed"))
				if err != nil {
					http.Error(w, err.Error(), 500)
					return
				}
				fmt.Fprint(w, result.Body)
			}))
			r := httptest.NewRequest("GET", "/render", nil)
			r.Header.Set("X-Fixture-Address", "203.0.113.17")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, r)
			if rec.Code != 200 || calls.Load() != 1 {
				t.Fatalf("render address: %d %s calls=%d", rec.Code, rec.Body.String(), calls.Load())
			}
			if !fail && rec.Body.String() != "render:203.0.113.17:203.0.113.17:sub=false" {
				t.Fatal(rec.Body.String())
			}
			if fail && !strings.Contains(rec.Body.String(), "fixture renderer address unavailable") {
				t.Fatal(rec.Body.String())
			}
		})
	}
}
