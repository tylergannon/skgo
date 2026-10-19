package skgo

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/fstest"
)

// A literal build tests SKGO's choice between the native shell and its renderer.
// Kit's shell generation and hash-client navigation were separately qualified.
func TestHashRouterDocumentsPreserveGoRoutingAndSkipPathnameSSR(t *testing.T) {
	t.Parallel()
	const shell = `<!doctype html><meta name="fixture" content="HASH_ROUTER_SHELL_245"><script type="module">import("/_app/immutable/entry/start.js");import("/_app/immutable/entry/app.js");</script>`
	for _, mode := range []string{"hash", "pathname"} {
		t.Run(mode, func(t *testing.T) {
			manifest := Manifest{AppDir: "_app", RouterType: mode, Remotes: []string{}, Nodes: []string{""}, Routes: []ManifestRoute{
				{ID: "/about", Pattern: `^/about/?$`, Page: &ManifestPage{Leaf: 0}},
				{ID: "/slash", Pattern: `^/slash/?$`, TrailingSlash: "always", Page: &ManifestPage{Leaf: 0}},
				{ID: "/literal-endpoint", Pattern: `^/literal-endpoint/?$`, Endpoint: &ManifestEndpoint{Methods: []string{"GET"}}},
			}, SSR: &ManifestSSR{Bundle: "bundle.js", Template: "app.html", ErrorTemplate: "error.html", Target: ssrTarget, GlobalName: "__sveltekit_hash_fixture", Nodes: []ManifestSSRNode{{Index: 0, Component: true}}}}
			raw, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			build := fstest.MapFS{
				"skgo.manifest.json": {Data: raw}, "index.html": {Data: []byte(shell)},
				"client/asset-245.txt": {Data: []byte("HASH_ASSET_245")},
				"bundle.js":            {Data: []byte(`globalThis.__skgo_ping=()=>"ok";globalThis.__skgo_render=()=>({done:true,status:200,head:"",body:"<h1>PATHNAME_SSR_245</h1>"});`)},
				"app.html":             {Data: []byte(`<!doctype html><head>%sveltekit.head%</head><body>%sveltekit.body%</body>`)},
				"error.html":           {Data: []byte(`<!doctype html>%sveltekit.status% %sveltekit.error.message%`)},
			}
			loads, err := NewLoads(manifest.LoadConfig("http://example.test"))
			if err != nil {
				t.Fatal(err)
			}
			remotes, err := NewRemotes(manifest.RemoteConfig("http://example.test"))
			if err != nil {
				t.Fatal(err)
			}
			renderer, err := NewSSR(build, manifest, loads, remotes, SSROptions{Runtimes: 1})
			if err != nil {
				t.Fatal(err)
			}
			pages, err := NewStaticHandler(build, WithSSR(renderer))
			if err != nil {
				t.Fatal(err)
			}
			endpoints, err := NewEndpoints(manifest.EndpointConfig("http://example.test"), NewEndpoint("/literal-endpoint", "GET", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(209); io.WriteString(w, "GO_ENDPOINT_245") }))
			if err != nil {
				t.Fatal(err)
			}
			cfg := manifest.HandleConfig()
			cfg.Origin = "http://example.test"
			cfg.Reroute = func(_ context.Context, event RerouteEvent) (*string, error) {
				var target string
				switch event.URL.Path {
				case "/alias-asset":
					target = "/asset-245.txt"
				case "/alias-document":
					target = "/deep/document"
				default:
					return nil, nil
				}
				return &target, nil
			}
			hook := Middleware(func(ctx context.Context, event *Event, resolve Resolve) (*http.Response, error) { return resolve(ctx) })
			handler := hook.Intercept(cfg, endpoints.Intercept(pages))
			response := do(t, handler, "GET", "http://example.test/about", nil)
			document := body(t, response)
			if mode == "pathname" {
				if response.StatusCode != 200 || !strings.Contains(document, "PATHNAME_SSR_245") || strings.Contains(document, "HASH_ROUTER_SHELL_245") {
					t.Fatalf("pathname renderer: status=%d body=%s", response.StatusCode, document)
				}
				return
			}
			if response.StatusCode != 200 || document != shell {
				t.Fatalf("hash document: status=%d body=%s", response.StatusCode, document)
			}
			for _, path := range []string{"/", "/deep/document", "/alias-document"} {
				response := do(t, handler, "GET", "http://example.test"+path, nil)
				if got := body(t, response); response.StatusCode != 200 || got != shell {
					t.Errorf("hash %s status=%d body=%s", path, response.StatusCode, got)
				}
			}
			for _, test := range []struct {
				path   string
				status int
				want   string
			}{{"/asset-245.txt", 200, "HASH_ASSET_245"}, {"/alias-asset", 200, "HASH_ASSET_245"}, {"/literal-endpoint", 209, "GO_ENDPOINT_245"}, {"/_app/missing.js", 404, ""}} {
				response := do(t, handler, "GET", "http://example.test"+test.path, nil)
				if got := body(t, response); response.StatusCode != test.status || got != test.want {
					t.Errorf("precedence %s status=%d body=%s", test.path, response.StatusCode, got)
				}
			}
			redirect := do(t, handler, "GET", "http://example.test/slash?q=245", nil)
			if redirect.StatusCode != 308 || redirect.Header.Get("Location") != "./slash/?q=245" {
				t.Errorf("upstream slash normalization: status=%d location=%q", redirect.StatusCode, redirect.Header.Get("Location"))
			}
			body(t, redirect)
			head := do(t, handler, "HEAD", "http://example.test/about", nil)
			if head.StatusCode != 200 || head.Header.Get("Content-Type") != "text/html; charset=utf-8" || body(t, head) != "" {
				t.Error("hash HEAD must preserve document headers without body")
			}
			post := do(t, handler, "POST", "http://example.test/about", http.Header{"Origin": {"http://example.test"}})
			if post.StatusCode != 405 || post.Header.Get("Allow") != "GET, HEAD" {
				t.Errorf("hash POST: status=%d allow=%q", post.StatusCode, post.Header.Get("Allow"))
			}
			body(t, post)
		})
	}
}
