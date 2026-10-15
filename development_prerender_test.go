package skgo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestDevelopmentPrerenderHTTPUsesGoAndPathArguments(t *testing.T) {
	calls := 0
	fn := NewRemote(RemoteSpec{Kind: KindPrerender, Module: "src/routes/about/about.remote.ts", Name: "receipt", Call: func(_ context.Context, c Call) (any, error) {
		calls++
		if !c.Present {
			return "live:no-argument", nil
		}
		return "live:" + c.Arg.(string), nil
	}})
	for _, test := range []struct{ suffix, want string }{{"", "live:no-argument"}, {"/WyJhdGxhcyJd", "live:atlas"}, {"?payload=WyJhdGxhcyJd", "live:no-argument"}} {
		manifest := Manifest{Development: true, AppDir: "_app", Remotes: []string{fn.ID()}}
		remotes := testRemotes(t, manifest.RemoteConfig("http://example.test"), fn)
		r := httptest.NewRequest(http.MethodGet, "http://example.test/_app/remote/"+fn.ID()+test.suffix, nil)
		w := httptest.NewRecorder()
		remotes.Intercept(http.NotFoundHandler()).ServeHTTP(w, r)
		kind, data, failure := envelope(t, w.Body.Bytes())
		if w.Code != 200 || kind != "result" || failure != nil || field(t, data, "_") != test.want || w.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatalf("%s status%d body%s headers%v", test.suffix, w.Code, w.Body, w.Header())
		}
		key := fn.ID() + "/"
		if strings.HasPrefix(test.suffix, "/") {
			key += strings.TrimPrefix(test.suffix, "/")
		}
		if got := field(t, field(t, field(t, data, "p"), key), "v"); got != test.want {
			t.Fatalf("prerender cache key %q value%#v want%q", key, got, test.want)
		}
	}
	if calls != 3 {
		t.Fatalf("Go body calls%d want3", calls)
	}
	// Development does not authorize production fallback execution.
	production := testRemotes(t, Manifest{AppDir: "_app", Remotes: []string{fn.ID()}}.RemoteConfig(""), fn)
	for _, handler := range []http.Handler{production, production.Intercept(http.NotFoundHandler())} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/_app/remote/"+fn.ID()+"/WyJhdGxhcyJd", nil))
		if calls != 3 {
			t.Fatal("production executed Go prerender body")
		}
	}
}

func TestDevelopmentPrerenderHTTPPreservesErrorsAndRedirects(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		want string
	}{{"app-error", Errorf(409, "dev conflict"), "dev conflict"}, {"redirect", &Redirect{Status: 307, Location: "/go-receipt"}, "/go-receipt"}} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			fn := NewRemote(RemoteSpec{Kind: KindPrerender, Module: "src/routes/about/about.remote.ts", Name: "receipt", Call: func(context.Context, Call) (any, error) { calls++; return nil, test.err }})
			rs := testRemotes(t, RemoteConfig{Development: true}, fn)
			w := httptest.NewRecorder()
			rs.Intercept(http.NotFoundHandler()).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/_app/remote/"+fn.ID(), nil))
			if calls != 1 || w.Code != 200 || !strings.Contains(w.Body.String(), test.want) {
				t.Fatalf("Go calls%d status%d body%s", calls, w.Code, w.Body)
			}
		})
	}
}

func TestDevelopmentPrerenderSSRDocumentRunsGoWithoutRecordedArtifact(t *testing.T) {
	calls := 0
	fn := NewRemote(RemoteSpec{Kind: KindPrerender, Module: "src/routes/about/about.remote.ts", Name: "receipt", Call: func(_ context.Context, c Call) (any, error) {
		calls++
		if !c.Present || c.Arg != "atlas" {
			return nil, fmt.Errorf("wrong supplied argument: %#v", c)
		}
		return "GO_DEV_PRERENDER_7719", nil
	}})
	manifest := Manifest{Development: true, AppDir: "_app", Nodes: []string{""}, Remotes: []string{fn.ID()}, Routes: []ManifestRoute{{ID: "/about", Pattern: "^/about/?$", Page: &ManifestPage{Leaf: 0}}}, SSR: &ManifestSSR{Bundle: "bundle.js", Template: "app.html", ErrorTemplate: "error.html", Target: ssrTarget, GlobalName: "__sveltekit_dev_test", Nodes: []ManifestSSRNode{{Index: 0, Component: true}}}}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	bundle := fmt.Sprintf(`globalThis.__skgo_ping=()=>"ok";globalThis.__skgo_render=()=>{const result={done:false,status:200,head:""};__skgo_remote(%q,"WyJhdGxhcyJd").then(raw=>{const answer=JSON.parse(raw);if(!answer.v){result.body="missing Go value";}else{result.body="<h1>"+JSON.parse(answer.v)[0]+"</h1>";}result.done=true;});return result;};`, fn.ID())
	build := fstest.MapFS{"skgo.manifest.json": {Data: raw}, "index.html": {Data: []byte(testIndexHTML)}, "client/.keep": {Data: []byte("keep")}, "bundle.js": {Data: []byte(bundle)}, "app.html": {Data: []byte("<!doctype html><html><head>%sveltekit.head%</head><body>%sveltekit.body%</body></html>")}, "error.html": {Data: []byte("<!doctype html>%sveltekit.status% %sveltekit.error.message%")}}
	remotes := testRemotes(t, manifest.RemoteConfig("http://example.test"), fn)
	loads, err := NewLoads(manifest.LoadConfig("http://example.test"))
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := NewSSR(build, manifest, loads, remotes, SSROptions{Runtimes: 2})
	if err != nil {
		t.Fatal(err)
	}
	pages, err := NewStaticHandler(build, WithSSR(renderer))
	if err != nil {
		t.Fatal(err)
	}
	handler := loads.Intercept(remotes.Intercept(pages))
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "http://example.test/about", nil)
	r.Header.Set("Accept", "text/html")
	handler.ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "<h1>GO_DEV_PRERENDER_7719</h1>") || calls != 1 {
		t.Fatalf("real SSR status%d calls%d body%s", w.Code, calls, w.Body)
	}
	if !strings.Contains(w.Body.String(), fn.ID()+"/WyJhdGxhcyJd") || !strings.Contains(w.Body.String(), "GO_DEV_PRERENDER_7719") {
		t.Fatalf("document lost prerender hydration key/value: %s", w.Body)
	}
}
