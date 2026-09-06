package example_test

import (
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/generated"
	"github.com/tylergannon/skgo/example/web"
)

// prodOrigin is the origin the handler under test is configured with. Nothing
// listens on it: every request below is served in-process.
const prodOrigin = "http://127.0.0.1:8080"

// newProdHandler assembles the production stack exactly as cmd/main.go does,
// from the build embedded in this binary.
//
// The unit tests for the static handler and the registry feed them synthetic
// fixtures, which is why they cannot see a manifest the real adapter wrote.
// This one starts where the developer does: the artifacts in the tree.
func newProdHandler(t *testing.T) http.Handler {
	t.Helper()

	dist, err := fs.Sub(web.Build, "build")
	if err != nil {
		t.Fatalf("opening the embedded build: %v", err)
	}
	manifest, err := skgo.ReadManifest(dist)
	if err != nil {
		t.Fatalf("reading the build manifest: %v", err)
	}
	remotes, err := skgo.NewRemotes(manifest.RemoteConfig(prodOrigin), generated.Remotes()...)
	if err != nil {
		t.Fatalf("mounting the remote registry: %v", err)
	}
	static, err := skgo.NewStaticHandler(dist)
	if err != nil {
		t.Fatalf("building the static handler: %v", err)
	}
	return remotes.Intercept(static)
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

// TestTheEmbeddedBuildProducesAWorkingServer is the check the developer
// otherwise only performs by running the binary and looking at a browser.
//
// `go test` was green while `./cmd` exited on startup, because nothing here
// ever built the production handler out of the manifest the adapter actually
// wrote. Every assertion below is against those artifacts.
func TestTheEmbeddedBuildProducesAWorkingServer(t *testing.T) {
	h := newProdHandler(t)

	if rec := get(t, h, "/"); rec.Code != http.StatusOK {
		t.Fatalf("GET /: status %d", rec.Code)
	}
}

// TestEveryRouteInTheManifestIsServed walks the manifest the adapter wrote and
// requires the boot document for a concrete URL of each route.
//
// Kit's route patterns are JavaScript regular expressions and Go's are not the
// same dialect, so a route shape that Go cannot compile — or compiles but does
// not match — takes the whole server down at startup or serves a 404 to a page
// that exists. Both are invisible to a test that builds its own patterns.
func TestEveryRouteInTheManifestIsServed(t *testing.T) {
	h := newProdHandler(t)

	dist, err := fs.Sub(web.Build, "build")
	if err != nil {
		t.Fatalf("opening the embedded build: %v", err)
	}
	manifest, err := skgo.ReadManifest(dist)
	if err != nil {
		t.Fatalf("reading the build manifest: %v", err)
	}
	if len(manifest.Routes) == 0 {
		t.Fatal("the manifest lists no routes")
	}

	document, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		t.Fatalf("reading index.html: %v", err)
	}

	for _, route := range manifest.Routes {
		path := samplePath(t, route.ID)
		rec := get(t, h, path)
		if rec.Code != http.StatusOK {
			t.Errorf("route %s: GET %s returned %d, want 200", route.ID, path, rec.Code)
			continue
		}
		if got := rec.Body.String(); got != string(document) {
			t.Errorf("route %s: GET %s did not return kit's boot document", route.ID, path)
		}
	}
}

// TestAnUnknownPathStillBootsKitsClient keeps the 404 half of the contract
// honest: kit's client has to receive the document so it can render
// +error.svelte, and it has to receive it with a 404.
func TestAnUnknownPathStillBootsKitsClient(t *testing.T) {
	h := newProdHandler(t)

	rec := get(t, h, "/no-such-page")
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /no-such-page: status %d, want 404", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "<") {
		t.Errorf("GET /no-such-page returned no document")
	}
}

// TestTheServerAnswersARealRemoteCall runs one of the app's own remote
// functions through the production stack. Every `.remote.ts` body throws, so a
// value coming back here could only have come from Go.
func TestTheServerAnswersARealRemoteCall(t *testing.T) {
	h := newProdHandler(t)

	id := remoteID(t, "getTodos")
	rec := get(t, h, "/_app/remote/"+id)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /_app/remote/%s: status %d, body %s", id, rec.Code, rec.Body.String())
	}

	var envelope struct {
		Type string `json:"type"`
	}
	body, _ := io.ReadAll(rec.Body)
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("the remote response is not JSON: %v\n%s", err, body)
	}
	if envelope.Type != "result" {
		t.Fatalf("the remote call did not return a result: %s", body)
	}
	// The seeded list is what the Gherkin suite reads on screen.
	if !strings.Contains(string(body), "write the adapter") {
		t.Fatalf("the remote call returned no todos: %s", body)
	}
}

// remoteID finds the `<hash>/<name>` id of a remote function by its name, so
// the tests never hard-code a hash kit derives from a file path.
func remoteID(t *testing.T, name string) string {
	t.Helper()
	for _, fn := range generated.Remotes() {
		if strings.HasSuffix(fn.ID(), "/"+name) {
			return fn.ID()
		}
	}
	t.Fatalf("no remote function named %s is registered", name)
	return ""
}

var (
	restSegment  = regexp.MustCompile(`^\[\.\.\.[\w-]+\]$`)
	paramSegment = regexp.MustCompile(`^\[[\w-]+\]$`)
	groupSegment = regexp.MustCompile(`^\([^)]+\)$`)
)

// samplePath turns a kit route id into one concrete URL that route must serve.
//
// It deliberately fails on any segment shape it does not recognise: a new kind
// of route must extend this gate rather than slip past it.
func samplePath(t *testing.T, id string) string {
	t.Helper()
	if id == "/" {
		return "/"
	}

	var out []string
	for _, segment := range strings.Split(strings.TrimPrefix(id, "/"), "/") {
		switch {
		case groupSegment.MatchString(segment):
			// A group does not appear in the URL.
		case restSegment.MatchString(segment):
			out = append(out, "deep", "rest", "path")
		case paramSegment.MatchString(segment):
			out = append(out, "sample")
		case !strings.ContainsAny(segment, "[]()"):
			out = append(out, segment)
		default:
			t.Fatalf("route %s has segment %q, which this test does not know how to visit; teach it before adding the route", id, segment)
		}
	}
	return "/" + strings.Join(out, "/")
}
