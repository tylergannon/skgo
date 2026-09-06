package example_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/businesslogic"
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

// post drives a command through the production stack. Kit's client sends the
// argument as a base64url devalue payload in a JSON envelope, and the registry
// refuses a cross-site POST, so the Origin has to be the configured one.
func post(t *testing.T, h http.Handler, path, payload string, refreshes ...string) *httptest.ResponseRecorder {
	t.Helper()
	if refreshes == nil {
		refreshes = []string{}
	}
	body, err := json.Marshal(map[string]any{"payload": payload, "refreshes": refreshes})
	if err != nil {
		t.Fatalf("encoding the command envelope: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", prodOrigin)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// devaluePayload encodes a hand-written devalue tree the way kit's client
// does: base64url of `[root, ...pool]`, where index 0 is the root itself.
func devaluePayload(tree string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(tree))
}

// TestACommandRefusesTheTodoTheQueryRefuses is the authorization gate for the
// write path.
//
// A signed-out visitor asking to read `t3` gets a 404, because it is private.
// Asking to *rename* it used to succeed: the store applied the visibility rule
// in every reader and in no writer, so the command changed the row and
// returned the whole record — id, text and `private: true` — to the visitor
// who had just been told it does not exist. Both halves are asserted here,
// because refusing the write while still describing the row is still the leak.
func TestACommandRefusesTheTodoTheQueryRefuses(t *testing.T) {
	h := newProdHandler(t)

	const privateID = "t3"
	read := get(t, h, "/_app/remote/"+remoteID(t, "getTodo")+"?payload="+devaluePayload(`["`+privateID+`"]`))
	if status := errorStatus(t, read.Body.Bytes()); status != 404 {
		t.Fatalf("signed out, getTodo %s answered %d; this test needs it to be the refused fixture", privateID, status)
	}

	const newText = "renamed by a visitor who cannot see it"
	wrote := post(t, h, "/_app/remote/"+remoteID(t, "renameTodo"),
		devaluePayload(`[{"id":1,"text":2},"`+privateID+`","`+newText+`"]`))

	body := wrote.Body.String()
	if status := errorStatus(t, wrote.Body.Bytes()); status != 404 {
		t.Errorf("signed out, renameTodo %s answered %v, want the 404 getTodo gives: %s", privateID, status, body)
	}
	if strings.Contains(body, newText) {
		t.Errorf("the response echoes the private todo back to a signed-out visitor: %s", body)
	}

	// The write itself must not have landed, whatever the response said.
	signedIn := businesslogic.Default.Session(businesslogic.Default.SignIn("gate")).User != ""
	if !signedIn {
		t.Fatal("could not open a session to read the private todo back")
	}
	todo, ok := businesslogic.Default.Todo(privateID, true)
	if !ok {
		t.Fatalf("%s is gone", privateID)
	}
	if todo.Text == newText {
		t.Errorf("a signed-out visitor changed the private todo to %q", todo.Text)
	}
}

// errorStatus reads the status out of a remote-function error envelope, or
// reports 0 for a successful result.
func errorStatus(t *testing.T, body []byte) int {
	t.Helper()
	var envelope struct {
		Type  string `json:"type"`
		Error struct {
			Status int `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("the remote response is not JSON: %v\n%s", err, body)
	}
	if envelope.Type != "error" {
		return 0
	}
	return envelope.Error.Status
}

// TestADataURLIsRefusedAtTheBoundary is the same gate as the unit tests, but
// against the routes kit actually compiled. Kit's route patterns end `\/?$`,
// so `/todos/__data.json` matched the `/todos` page and came back 200
// text/html — a failure that landed inside kit's client rather than here.
func TestADataURLIsRefusedAtTheBoundary(t *testing.T) {
	h := newProdHandler(t)

	dist, err := fs.Sub(web.Build, "build")
	if err != nil {
		t.Fatalf("opening the embedded build: %v", err)
	}
	manifest, err := skgo.ReadManifest(dist)
	if err != nil {
		t.Fatalf("reading the build manifest: %v", err)
	}
	document, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		t.Fatalf("reading index.html: %v", err)
	}

	// `__data.json` is refused 404, the status kit's client survives here.
	// `__route.js` gets kit's own 400, verbatim. Both must stop being the boot
	// document, which is the defect; the statuses are pinned so a future change
	// to one of them has to be deliberate.
	for _, route := range manifest.Routes {
		page := samplePath(t, route.ID)
		for suffix, want := range map[string]int{
			"/__data.json": http.StatusNotFound,
			"/__route.js":  http.StatusBadRequest,
		} {
			url := strings.TrimSuffix(page, "/") + suffix
			rec := get(t, h, url)
			if rec.Code != want {
				t.Errorf("route %s: GET %s returned %d, want %d", route.ID, url, rec.Code, want)
			}
			if rec.Body.String() == string(document) {
				t.Errorf("route %s: GET %s returned kit's boot document", route.ID, url)
			}
		}
	}
}
