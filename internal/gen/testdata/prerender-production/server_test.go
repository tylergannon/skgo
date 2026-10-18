package example_test

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example"
	prerendercontract "github.com/tylergannon/skgo/example/internal/skgo/links/onzggl3sn52xizltf5yhezlsmvxgizlsfvrw63tuojqwg5a"
	"github.com/tylergannon/skgo/example/ui"
)

func TestProductionPrerenderedEndpointsAndPages(t *testing.T) {
	dist := buildFS(t)
	manifest, err := skgo.ReadManifest(dist)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.PrerenderedEndpoints) != 3 || strings.Join(manifest.PrerenderedEndpoints["/lifecycle-api"], ",") != "GET" || strings.Join(manifest.PrerenderedEndpoints["/lifecycle-cookies/[operation]"], ",") != "GET" {
		t.Fatalf("fully prerendered endpoint declarations: %v", manifest.PrerenderedEndpoints)
	}
	for _, route := range manifest.Routes {
		if route.ID == "/options-api/[slug]" || route.ID == "/lifecycle-api" || route.ID == "/lifecycle-cookies/[operation]" || route.ID == "/lifecycle/[slug]" {
			t.Fatalf("fully prerendered route retained for dynamic dispatch: %s", route.ID)
		}
	}
	handler := newHandler(t, dist, nil)
	for _, path := range []string{"/lifecycle-api", "/lifecycle-cookies/alpha-set", "/lifecycle-cookies/alpha-delete", "/lifecycle-cookies/beta-set", "/lifecycle-cookies/beta-delete"} {
		want := "cookies updated"
		if path == "/lifecycle-api" {
			want = "Go fetch: own locals 10"
		}
		response := get(handler, path)
		if response.Code != 200 || response.Body.String() != want || len(response.Header().Values("Set-Cookie")) != 0 {
			t.Fatalf("static endpoint %s: %d %v %s", path, response.Code, response.Header(), response.Body.String())
		}
	}
	for _, slug := range []string{"alpha", "beta"} {
		response := get(handler, "/lifecycle/"+slug)
		if response.Code != 200 || !strings.Contains(response.Body.String(), "/lifecycle/"+slug+":remote-13 cookies:scoped%20raw deleted") || !strings.Contains(response.Body.String(), "<!-- Go after hook: /lifecycle/"+slug+" cookies:scoped%20raw deleted cookie-forwarding:root -->") {
			t.Fatalf("static page %s: %d %s", slug, response.Code, response.Body.String())
		}
	}
	for _, path := range []string{"/lifecycle-cookies/unbuilt-set", "/lifecycle/unbuilt"} {
		response := get(handler, path)
		if response.Code != 404 || !strings.Contains(response.Body.String(), "Not Found") || response.Body.String() == "cookies updated" {
			t.Fatalf("unbuilt path %s dynamically answered: %d %s", path, response.Code, response.Body.String())
		}
	}
	ordinary := get(handler, "/ordinary")
	if ordinary.Code != 200 || !strings.Contains(ordinary.Body.String(), "<h1>Ordinary SSR route</h1>") {
		t.Fatalf("ordinary dynamic page: %d %s", ordinary.Code, ordinary.Body.String())
	}
}

// Literal ID from Kit 3.0.0's hash of the authored module path
// src/routes/prerender-contract/fixture.remote.ts, not discovered from output.
const remoteID = "18787o2/noargValue"
const artifactPath = "prerendered/_app/remote/18787o2/noargValue"
const nativeResult = `{"type":"result","data":"[{\"_\":1},\"live no-argument body ran\"]"}`
const builtError = `{"type":"error","error":{"status":409,"message":"built no-argument failure","marker":"native-prerender-error"}}`
const transformResult = `{"type":"result","data":"[{\"_\":1},\"built transform value\"]"}`

// Open overlays one file without changing the embedded build, its directory
// entries, or another handler's artifact store.
type oneFileOverlay struct {
	fs.FS
	data string
}

func (o oneFileOverlay) Open(name string) (fs.File, error) {
	if name == artifactPath {
		return (fstest.MapFS{"artifact": {Data: []byte(o.data)}}).Open("artifact")
	}
	return o.FS.Open(name)
}

func buildFS(t *testing.T) fs.FS {
	t.Helper()
	dist, err := fs.Sub(ui.Build, "build")
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := fs.ReadFile(dist, artifactPath)
	if err != nil || string(artifact) != nativeResult {
		t.Fatalf("native artifact at literal path = %s, err %v; want %s", artifact, err, nativeResult)
	}
	t.Cleanup(func() {
		if got := prerendercontract.Calls(); got != 0 {
			t.Errorf("the Go body ran %d times while serving built artifacts; want zero", got)
		}
	})
	return dist
}

func newHandler(t *testing.T, dist fs.FS, middleware skgo.Middleware) http.Handler {
	t.Helper()
	handler, err := example.NewHandler(dist, middleware)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func get(handler http.Handler, path string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	return recorder
}

func TestPrerenderFetchDestinationsMatchServedCapabilities(t *testing.T) {
	dist := buildFS(t)
	artifact, err := fs.ReadFile(dist, "prerendered/fetch-capabilities.html")
	if err != nil {
		t.Fatal(err)
	}
	handler := newHandler(t, dist, nil)
	served := get(handler, "/fetch-capabilities")
	if served.Code != 200 || served.Body.String() != string(artifact) {
		t.Fatalf("prerender artifact was not served unchanged: %d %s", served.Code, served.Body)
	}
	for _, literal := range []string{
		"<h1>Prerender fetch capability</h1>",
		"<p>skgo: prerender Event.Fetch cannot dispatch page destination GET /fetch-target: Kit request context is unavailable</p>",
		"<p>skgo: prerender Event.Fetch cannot dispatch data destination GET /fetch-target/__data.json: Kit request context is unavailable</p>",
		"<p>skgo: prerender Event.Fetch cannot dispatch remote destination GET /_app/remote/18787o2/noargValue: Kit request context is unavailable</p>",
		"<p>response:404:404 page not found</p>",
		"<p>response:200:Go fetch: own locals 10</p>",
	} {
		if !strings.Contains(string(artifact), literal) {
			t.Errorf("real build omitted %q: %s", literal, artifact)
		}
	}
	for _, tc := range []struct {
		path     string
		literals []string
	}{
		{"/fetch-target", []string{"<h1>Literal fetch target 272</h1>", "<p>literal-fetch-target-data-272</p>"}},
		{"/fetch-target/__data.json", []string{`"type":"data"`, `"receipt"`, `"literal-fetch-target-data-272"`}},
		{"/_app/remote/18787o2/noargValue", []string{nativeResult}},
	} {
		response := get(handler, tc.path)
		if response.Code != 200 {
			t.Fatalf("live destination %s status=%d body=%s", tc.path, response.Code, response.Body)
		}
		for _, literal := range tc.literals {
			if !strings.Contains(response.Body.String(), literal) {
				t.Errorf("live destination %s lacks %q: %s", tc.path, literal, response.Body)
			}
		}
	}
	missing := get(handler, "/literal-missing-destination-272")
	if missing.Code != 404 || !strings.Contains(missing.Body.String(), "Not Found") {
		t.Fatalf("genuinely missing destination: %d %s", missing.Code, missing.Body)
	}
}

func assertPEntry(t *testing.T, document, key string, required ...string) {
	t.Helper()
	start := strings.Index(document, "\""+key+"\":")
	if start < 0 {
		t.Fatalf("document omitted original p answer %q: %s", key, document)
	}
	tail := document[start:]
	if end := strings.Index(tail, "}}"); end >= 0 {
		tail = tail[:end+2]
	}
	for _, text := range required {
		if !strings.Contains(tail, text) {
			t.Fatalf("p answer %q omitted %q: %s", key, text, document)
		}
	}
}

func TestProductionNoArgumentNullUsesArtifactMiss(t *testing.T) {
	dist := buildFS(t)
	for _, path := range []string{"/prerender-contract", "/prerender-contract-missing"} {
		t.Run(path, func(t *testing.T) {
			recorder := get(newHandler(t, dist, nil), path)
			if recorder.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want opaque 500 (400 means Kit validated before artifact lookup): %s", recorder.Code, recorder.Body.String())
			}
			if strings.Contains(recorder.Body.String(), "live no-argument body ran") {
				t.Fatal("the null-key miss returned the no-argument value")
			}
			assertPEntry(t, recorder.Body.String(), remoteID+"/W251bGxd", "message:\"Internal Error\"", "status:500")
		})
	}
}

func assertStoredError(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	html := recorder.Body.String()
	if recorder.Code != 409 || !strings.Contains(html, "built no-argument failure") || !strings.Contains(html, "native-prerender-error") {
		t.Fatalf("handled remote error document status=%d body=%s", recorder.Code, html)
	}
	assertPEntry(t, html, remoteID+"/", "status:409", "message:\"built no-argument failure\"", "marker:\"native-prerender-error\"")
}

func assertNativeValue(t *testing.T, handler http.Handler) {
	t.Helper()
	recorder := get(handler, "/prerender-contract-error")
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "<h1>Prerender error consumer</h1><p>live no-argument body ran</p>") {
		t.Fatalf("unmodified handler status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	assertPEntry(t, recorder.Body.String(), remoteID+"/", "live no-argument body ran")
}

func TestProductionStoredError(t *testing.T) {
	dist := buildFS(t)
	ordinary := newHandler(t, dist, nil)
	errors := newHandler(t, oneFileOverlay{FS: dist, data: builtError}, nil)
	assertStoredError(t, get(errors, "/prerender-contract-error"))
	assertNativeValue(t, ordinary)
	assertStoredError(t, get(errors, "/prerender-contract-error"))
	assertNativeValue(t, newHandler(t, dist, nil))
}

func TestProductionTransformFailure(t *testing.T) {
	dist := buildFS(t)
	ordinary := newHandler(t, dist, nil)
	errorHandler := newHandler(t, oneFileOverlay{FS: dist, data: builtError}, nil)
	// Construct two handlers in the same process. Neither may inherit another
	// handler's one-shot transform counter or its overlay's decoded value.
	for i := 0; i < 2; i++ {
		transformCalls := 0
		middleware := skgo.Middleware(func(ctx context.Context, _ *skgo.Event, resolve skgo.Resolve) (*http.Response, error) {
			return resolve(ctx, skgo.ResolveOptions{TransformPageChunk: func(_ context.Context, html string, _ bool) (string, error) {
				transformCalls++
				if transformCalls == 1 {
					return "", errors.New("one-shot transform failure")
				}
				return html, nil
			}})
		})
		transformHandler := newHandler(t, oneFileOverlay{FS: dist, data: transformResult}, middleware)
		recorder := get(transformHandler, "/prerender-contract-transform")
		if recorder.Code != http.StatusInternalServerError || transformCalls != 2 {
			t.Fatalf("transform fallback status=%d calls=%d body=%s", recorder.Code, transformCalls, recorder.Body.String())
		}
		assertPEntry(t, recorder.Body.String(), remoteID+"/", "built transform value")
		assertStoredError(t, get(errorHandler, "/prerender-contract-error"))
		assertNativeValue(t, ordinary)
		if transformCalls != 2 {
			t.Fatalf("transform middleware leaked into another handler: calls=%d", transformCalls)
		}
	}
}

func TestProductionRedirectServing(t *testing.T) {
	handler := newHandler(t, buildFS(t), nil)
	for _, redirect := range []struct{ path, destination string }{
		{"/old", "/target?from=atlas"},
		{"/second", "/target?from=beacon"},
		{"/ordinary-old", "/ordinary?from=legacy"},
	} {
		recorder := get(handler, redirect.path)
		want := `<script>location.href="` + redirect.destination + `";</script><meta http-equiv="refresh" content="0;url=` + redirect.destination + `">`
		if recorder.Code != 200 || recorder.Body.String() != want {
			t.Errorf("native redirect %s status=%d body=%s, want %s", redirect.path, recorder.Code, recorder.Body.String(), want)
		}
	}
	for _, path := range []string{"/target?from=atlas", "/target?from=beacon", "/target"} {
		recorder := get(handler, path)
		if recorder.Code != 200 || !strings.Contains(recorder.Body.String(), "<h1>Target route</h1>") {
			t.Errorf("target %s status=%d body=%s", path, recorder.Code, recorder.Body.String())
		}
	}
	recorder := get(handler, "/ordinary?from=legacy")
	if recorder.Code != 200 || !strings.Contains(recorder.Body.String(), "<h1>Ordinary SSR route</h1>") {
		t.Fatalf("dynamic destination status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestProductionHookRefusalMapping(t *testing.T) {
	dist := buildFS(t)
	html, err := fs.ReadFile(dist, "prerendered/hook-refusals.html")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(html), `418 {"status":418,"message":"teapot literal"}`) {
		t.Errorf("prerendered hook refusal lacks Kit's literal 418 JSON body: %s", html)
	}
	if !strings.Contains(string(html), `<p>307 /hook-target</p>`) {
		t.Errorf("prerendered hook redirect lacks literal status 307 and Location /hook-target: %s", html)
	}
	marker := os.Getenv("SKGO_FIXTURE_REFUSED_LOAD_MARKER")
	if marker == "" {
		t.Fatal("missing independent refused-load marker path")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("the refused Go load ran: marker %q stat error=%v", marker, err)
	}
}

func TestSharedRendererDefaultFilter(t *testing.T) {
	dist := buildFS(t)
	// This served route has the same universal fetch as the prerendered default
	// route, and uses the renderer default without a hook-selected filter.
	handler := newHandler(t, dist, nil)
	response := get(handler, "/options-served")
	body := response.Body.String()
	if response.Code != 200 || !strings.Contains(body, "<h1>Served default options</h1>") || !strings.Contains(body, `"x-default":"default-literal"`) || strings.Contains(body, `"x-public"`) || strings.Contains(body, `"x-denied"`) {
		t.Fatalf("served default parity: %d %s", response.Code, body)
	}
}
