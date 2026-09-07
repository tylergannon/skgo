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
	"slices"
	"strings"
	"testing"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example"
	"github.com/tylergannon/skgo/example/businesslogic"
	"github.com/tylergannon/skgo/example/generated"
	"github.com/tylergannon/skgo/example/web"
)

// prodOrigin is the origin the handler under test is configured with. Nothing
// listens on it: every request below is served in-process.
const prodOrigin = "http://127.0.0.1:8080"

// newProdHandler is the production stack: not a copy of what cmd/main.go
// composes, but the same function it calls.
//
// The unit tests for the static handler and the registries feed them synthetic
// fixtures, which is why they cannot see a manifest the real adapter wrote.
// This one starts where the developer does: the artifacts in the tree.
func newProdHandler(t *testing.T) http.Handler {
	t.Helper()

	h, mode, err := example.NewHandler(prodDist(t), "", prodOrigin)
	if err != nil {
		t.Fatalf("assembling the production stack: %v", err)
	}
	if mode != "prod" {
		t.Fatalf("mode = %q, want prod", mode)
	}
	return h
}

func prodDist(t *testing.T) fs.FS {
	t.Helper()
	dist, err := fs.Sub(web.Build, "build")
	if err != nil {
		t.Fatalf("opening the embedded build: %v", err)
	}
	return dist
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
// asks for a concrete URL of each route.
//
// Kit's route patterns are JavaScript regular expressions and Go's are not the
// same dialect, so a route shape that Go cannot compile — or compiles but does
// not match — takes the whole server down at startup or serves a 404 to a page
// that exists. Both are invisible to a test that builds its own patterns.
//
// What each route is answered with comes from the manifest too, because that is
// where a page's own options live: a branch that turns SSR off is answered with
// kit's shell, one that turns CSR off is answered with a document carrying no
// script at all, and everything else is rendered and boots.
func TestEveryRouteInTheManifestIsServed(t *testing.T) {
	h := newProdHandler(t)
	session := businesslogic.Default.SignIn("ada")

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
	if manifest.SSR == nil {
		t.Fatal("the build carries no SSR bundle, so nothing below is about the server that ships")
	}

	shell, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		t.Fatalf("reading index.html: %v", err)
	}

	pages, rendered, errored := 0, 0, 0
	for _, route := range manifest.Routes {
		if route.Page == nil {
			// A route that is a `+server.ts` and nothing else has no page to
			// boot. Serving it the document is the bug this distinction
			// exists to prevent, and TestAServerRouteAnswersItsOwnMethods
			// pins what it does answer instead.
			continue
		}
		pages++

		path := samplePath(t, route.ID)
		req := httptest.NewRequest(http.MethodGet, path, nil)
		// Signed in, because a section of this app turns a signed-out visitor
		// away and a redirect is not what this test is about.
		req.AddCookie(&http.Cookie{Name: example.SessionCookie, Value: session})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		body := rec.Body.String()

		if want, deliberate := deliberateFailures[route.ID]; deliberate {
			errored++
			if rec.Code != want.status {
				t.Errorf("route %s: GET %s returned %d, want %d", route.ID, path, rec.Code, want.status)
			}
			if want.location != "" {
				// A redirect has no document at all: a Location and nothing
				// else, which is kit's `redirect_response`.
				if got := rec.Header().Get("Location"); got != want.location {
					t.Errorf("route %s: GET %s sent Location %q, want %q", route.ID, path, got, want.location)
				}
				if body != "" {
					t.Errorf("route %s: GET %s answered a redirect with %d bytes of body", route.ID, path, len(body))
				}
				continue
			}
			if body == string(shell) {
				t.Errorf("route %s: GET %s returned kit's shell rather than a rendered error document", route.ID, path)
			}
			if !strings.Contains(body, want.says) {
				t.Errorf("route %s: GET %s does not say %q", route.ID, path, want.says)
			}
			for _, leaked := range want.never {
				if strings.Contains(body, leaked) {
					t.Errorf("route %s: GET %s leaked %q into the document", route.ID, path, leaked)
				}
			}
			if want.static {
				// error.html is a whole document of its own: no app markup, and
				// no script, so nothing boots and nothing tries again.
				if strings.Contains(body, "<script") {
					t.Errorf("route %s: GET %s carries a script, so it is not kit's static error page", route.ID, path)
				}
				if strings.Contains(body, `data-testid="app-nav"`) {
					t.Errorf("route %s: GET %s rendered the root layout, which is the layout that failed", route.ID, path)
				}
			} else {
				if !strings.Contains(body, `data-testid="error-message"`) {
					t.Errorf("route %s: GET %s carries no rendered error page", route.ID, path)
				}
				if !strings.Contains(body, want.inside) {
					t.Errorf("route %s: GET %s did not render the error page inside %s", route.ID, path, want.inside)
				}
			}
			continue
		}

		if rec.Code != http.StatusOK {
			t.Errorf("route %s: GET %s returned %d, want 200", route.ID, path, rec.Code)
			continue
		}

		ssr, csr := true, true
		for _, index := range route.Page.Branch() {
			if index < 0 || index >= len(manifest.SSR.Nodes) {
				continue
			}
			if v := manifest.SSR.Nodes[index].SSR; v != nil {
				ssr = *v
			}
			if v := manifest.SSR.Nodes[index].CSR; v != nil {
				csr = *v
			}
		}

		switch {
		case !ssr:
			if body != string(shell) {
				t.Errorf("route %s turns SSR off: GET %s did not return kit's shell", route.ID, path)
			}
		case body == string(shell):
			// Nothing is answered with the shell any more except a branch that
			// turns SSR off. A page that quietly stopped rendering used to hide
			// here, behind "its load failed"; a load that fails is now a
			// rendered error document with the status it threw, and is listed
			// in deliberateFailures above.
			t.Errorf("route %s: GET %s returned kit's shell rather than a rendered page", route.ID, path)
		case !strings.Contains(body, `data-testid="app-nav"`):
			t.Errorf("route %s: GET %s carries no rendered markup from the root layout", route.ID, path)
		default:
			rendered++
		}

		boots := strings.Contains(body, "kit.start(app, element")
		if csr && ssr && !boots {
			t.Errorf("route %s: GET %s carries no boot script, so nothing would hydrate", route.ID, path)
		}
		if !csr && strings.Contains(body, "<script") {
			t.Errorf("route %s turns CSR off: GET %s still carries a script", route.ID, path)
		}
	}
	if pages == 0 {
		t.Fatal("the manifest lists no page routes, so this test asserted nothing")
	}
	if rendered == 0 {
		t.Fatal("not one page route was rendered, so this test asserted nothing about SSR")
	}
	if errored != len(deliberateFailures) {
		t.Errorf("%d of the %d routes this app fails on purpose were visited; the manifest no longer carries the rest",
			errored, len(deliberateFailures))
	}
}

// deliberateFailures is what this app does wrong on purpose, and what kit's own
// rules make of each one.
//
// The statuses are written here rather than read back off the response, because
// the failure this guards against — a page falling back to kit's shell — is
// answered with a perfectly ordinary 200 and a document that looks fine until
// the browser boots. Every fixture named below exists for one scenario and has
// exactly one source in the app.
var deliberateFailures = map[string]struct {
	status int
	// location, when set, is where a bare 3xx sends the visitor. There is no
	// document to check in that case.
	location string
	// says is text the document itself must carry.
	says string
	// inside is markup from the layout the error page must render within.
	inside string
	// never is text the document must not carry: what the error actually said,
	// where kit's rules say the visitor is told nothing.
	never []string
	// static reports that kit's `error.html` is the answer, which carries no
	// app markup and no script at all.
	static bool
}{
	// A load that throws error(402, ...) under /account, which declares its own
	// +error.svelte: the account layout survives and its error page renders
	// inside it.
	"/account/statement": {status: 402, says: "Your account is in arrears", inside: `data-testid="account-user"`},
	// The same thing with no error page nearer than the root's.
	"/error/expected": {status: 418, says: "This page is a teapot", inside: `data-testid="app-nav"`},
	// A load that fails with an ordinary error: 500 Internal Error, and not one
	// word of what actually went wrong.
	"/error/unexpected": {
		status: 500, says: "Internal Error", inside: `data-testid="app-nav"`,
		never: []string{"hunter2", "postgres://"},
	},
	// A command called while the page renders. Kit refuses it, transformError
	// turns the refusal into Internal Error, and the boundary the root error
	// page guards renders in the page's place.
	"/error/command": {
		status: 500, says: "Internal Error", inside: `data-testid="app-nav"`,
		never: []string{"Cannot call a command"},
	},
	// A query that redirects while the page renders. Kit turns that into a
	// redirect of the whole document, not into an error inside it.
	"/error/redirect": {status: 307, location: "/about"},
	// A throw in the root layout, which no error page can guard. Both the
	// render and the retry through respond_with_error fail, and kit's static
	// error.html is what is left.
	"/error/render": {status: 500, says: "Internal Error", static: true},
}

// TestAnUnknownPathIsAnsweredWithTheRenderedErrorPage is kit's `respond.js`
// answer to a path that matched no route: `respond_with_error` renders the root
// layout with the root error page inside it, at 404. The document says the page
// is missing before a line of JavaScript has run, which a shell that has to boot
// first cannot.
func TestAnUnknownPathIsAnsweredWithTheRenderedErrorPage(t *testing.T) {
	h := newProdHandler(t)

	rec := get(t, h, "/no-such-page")
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /no-such-page: status %d, want 404", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		// The root error page, showing the status and the message kit gives.
		`<h1 data-testid="title">Error 404</h1>`,
		`<p data-testid="error-message">Not Found</p>`,
		// Inside the app's own root layout.
		`data-testid="app-nav"`,
		// And it still boots, so the client takes over from the same page.
		"kit.start(app, element",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("GET /no-such-page: the document does not carry %q", want)
		}
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
// A data URL is the app's own endpoint, and every route has one. What must
// never happen is the defect this replaced: the boot document served at a data
// URL, which kit's client hands to JSON.parse.
//
// `__route.js` is a different case — kit's own 400 for the client-side route
// resolution skgo serves — and is pinned here beside it.
func TestEveryDataURLIsAnsweredByGoAndNeverByTheDocument(t *testing.T) {
	h := newProdHandler(t)

	document, err := fs.ReadFile(prodDist(t), "index.html")
	if err != nil {
		t.Fatalf("reading index.html: %v", err)
	}
	manifest, err := skgo.ReadManifest(prodDist(t))
	if err != nil {
		t.Fatalf("reading the build manifest: %v", err)
	}

	checked := 0
	for _, route := range manifest.Routes {
		page := strings.TrimSuffix(samplePath(t, route.ID), "/")

		if route.Page == nil {
			// Kit answers `__data.json` on an endpoint-only route with a bare
			// 404 and no body (`runtime/server/data/index.js`), because there
			// is no branch to run.
			rec := get(t, h, page+"/__data.json")
			if rec.Code != http.StatusNotFound {
				t.Errorf("route %s: GET %s/__data.json returned %d, want 404", route.ID, page, rec.Code)
			}
			if rec.Body.String() == string(document) {
				t.Errorf("route %s: GET %s/__data.json returned kit's boot document", route.ID, page)
			}
			continue
		}
		checked++

		rec := get(t, h, page+"/__data.json")
		if rec.Code != http.StatusOK {
			t.Errorf("route %s: GET %s/__data.json returned %d, want 200", route.ID, page, rec.Code)
		}
		if rec.Body.String() == string(document) {
			t.Errorf("route %s: GET %s/__data.json returned kit's boot document", route.ID, page)
		}
		// Kit's client reads the first line as one of three envelopes. A body
		// that is none of them is a body it cannot use.
		var envelope struct {
			Type string `json:"type"`
		}
		line, _, _ := strings.Cut(rec.Body.String(), "\n")
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			t.Errorf("route %s: GET %s/__data.json is not JSON: %v: %s", route.ID, page, err, line)
			continue
		}
		switch envelope.Type {
		case "data", "redirect", "error":
		default:
			t.Errorf("route %s: GET %s/__data.json answered %q, not one of kit's envelopes: %s",
				route.ID, page, envelope.Type, line)
		}

		rec = get(t, h, page+"/__route.js")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("route %s: GET %s/__route.js returned %d, want 400", route.ID, page, rec.Code)
		}
		if rec.Body.String() == string(document) {
			t.Errorf("route %s: GET %s/__route.js returned kit's boot document", route.ID, page)
		}
	}
	if checked == 0 {
		t.Fatal("the manifest lists no page routes, so this test asserted nothing")
	}
}

// The static handler answers a page with an ETag and honours a conditional
// request against it. A data URL is not that page, and the two live one
// `Intercept` apart: if a data request ever reached the static handler it would
// be matched against the document's validator and answered 304 with no body,
// which kit's client reads as an empty response.
//
// The load below returns data the browser can be seen to have used, so a 304
// or a document is not merely a different status but visibly the wrong bytes.
func TestAConditionalRequestForADataURLStillGetsKitsData(t *testing.T) {
	h := newProdHandler(t)

	// The document's own ETag, taken from the page this data URL belongs to.
	// It is the validator a confused conditional request would be matched
	// against, so it is the one worth sending.
	session := businesslogic.Default.SignIn("ada")

	page := httptest.NewRequest(http.MethodGet, "/account", nil)
	page.AddCookie(&http.Cookie{Name: example.SessionCookie, Value: session})
	pageRec := httptest.NewRecorder()
	h.ServeHTTP(pageRec, page)
	documentETag := pageRec.Header().Get("ETag")
	if documentETag == "" {
		t.Fatalf("GET /account served no ETag (status %d); this test would prove nothing", pageRec.Code)
	}

	for _, headers := range []map[string]string{
		{},
		{"If-None-Match": "*"},
		{"If-None-Match": documentETag},
		{"If-Modified-Since": "Wed, 21 Oct 2099 07:28:00 GMT"},
	} {
		req := httptest.NewRequest(http.MethodGet, "/account/__data.json?x-sveltekit-invalidated=111", nil)
		req.AddCookie(&http.Cookie{Name: example.SessionCookie, Value: session})
		for name, value := range headers {
			req.Header.Set(name, value)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("%v: status %d, want 200 — a data URL has no validator to match", headers, rec.Code)
			continue
		}
		if rec.Body.Len() == 0 {
			t.Errorf("%v: empty body", headers)
			continue
		}
		// `ada` is the name the fixture signed in with, and the layout's load
		// is the only thing that can put it on the wire.
		if !strings.Contains(rec.Body.String(), `"ada"`) {
			t.Errorf("%v: the signed-in visitor's data is not in the response: %s", headers, rec.Body.String())
		}
		if !strings.HasPrefix(rec.Body.String(), `{"type":"data","nodes":[`) {
			t.Errorf("%v: not kit's data envelope: %s", headers, rec.Body.String())
		}
	}
}

// TestAServerRouteAnswersItsOwnMethods is issue #19 through the real stack.
//
// The bug was not the 405 on POST; it was the 200 on GET, which handed kit's
// client an HTML document where the caller expected an API response — the same
// wrong-200 shape that made `__data.json` fail inside kit's client rather than
// at the boundary.
func TestAServerRouteAnswersItsOwnMethods(t *testing.T) {
	h := newProdHandler(t)

	document, err := fs.ReadFile(prodDist(t), "index.html")
	if err != nil {
		t.Fatalf("reading index.html: %v", err)
	}

	rec := get(t, h, "/api/todos")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/todos: status %d, want 200", rec.Code)
	}
	if rec.Body.String() == string(document) {
		t.Fatal("GET /api/todos returned kit's boot document")
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("GET /api/todos: Content-Type %q, want application/json", ct)
	}

	// The seeded todos are the store's own fixture, so this is anchored in
	// something the endpoint did not produce.
	var todos []businesslogic.Todo
	if err := json.Unmarshal(rec.Body.Bytes(), &todos); err != nil {
		t.Fatalf("GET /api/todos is not JSON: %v: %s", err, rec.Body.String())
	}
	var texts []string
	for _, todo := range todos {
		texts = append(texts, todo.Text)
	}
	if !slices.Contains(texts, "write the adapter") {
		t.Errorf("GET /api/todos returned %v, which does not include the seeded todo", texts)
	}

	// A method the route declares no handler for is refused with kit's own 405,
	// and the Allow header names what the route does answer — including the
	// HEAD kit synthesizes from GET.
	req := httptest.NewRequest(http.MethodDelete, "/api/todos", nil)
	req.Header.Set("Origin", prodOrigin)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("DELETE /api/todos: status %d, want 405", rec.Code)
	}
	if got := rec.Header().Get("Allow"); got != "GET, POST, HEAD" {
		t.Errorf("DELETE /api/todos: Allow %q, want %q", got, "GET, POST, HEAD")
	}

	// POST is one of the methods the route declares, and it used to be a 405.
	req = httptest.NewRequest(http.MethodPost, "/api/todos", strings.NewReader(`{"text":"go test wrote this"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", prodOrigin)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/todos: status %d, want 201: %s", rec.Code, rec.Body.String())
	}
	var created businesslogic.Todo
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("POST /api/todos is not JSON: %v", err)
	}
	if created.Text != "go test wrote this" {
		t.Errorf("POST /api/todos returned %q, want the text this test supplied", created.Text)
	}
	if got := rec.Header().Get("Location"); got != "/api/todos/"+created.ID {
		t.Errorf("POST /api/todos: Location %q", got)
	}
}

// TestEveryPrerenderedPathIsServedFromItsFile walks what the build recorded.
//
// Kit removes a prerendered route from the table the manifest is generated
// from, so a prerendered path that is not served here is not served at all —
// and the failure is a 404 on a page that exists, which kit's client papers
// over by rendering the route anyway.
func TestEveryPrerenderedPathIsServedFromItsFile(t *testing.T) {
	h := newProdHandler(t)
	dist := prodDist(t)

	manifest, err := skgo.ReadManifest(dist)
	if err != nil {
		t.Fatalf("reading the build manifest: %v", err)
	}
	if len(manifest.Prerendered) == 0 {
		t.Fatal("the build prerendered nothing, so this test asserts nothing")
	}

	document, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		t.Fatalf("reading index.html: %v", err)
	}

	for _, path := range manifest.Prerendered {
		rec := get(t, h, path)
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s: status %d, want 200", path, rec.Code)
			continue
		}
		// The boot document is what an unprerendered route gets. Serving it
		// here would look identical in a browser — kit's client would render
		// the route anyway — and would mean the prerendered file was never
		// used.
		if rec.Body.String() == string(document) {
			t.Errorf("GET %s served the single-page fallback, not the prerendered file", path)
		}
		if !strings.Contains(rec.Header().Get("Content-Type"), "text/html") {
			t.Errorf("GET %s: Content-Type %q", path, rec.Header().Get("Content-Type"))
		}
	}
}
