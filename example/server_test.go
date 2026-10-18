package example_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/tylergannon/devalue/v5"
	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example"
	"github.com/tylergannon/skgo/example/businesslogic"
	generated "github.com/tylergannon/skgo/example/internal/skgo"
	"github.com/tylergannon/skgo/example/web"
)

// docs: prod-handler
// prodOrigin is the origin the handler under test is configured with. Nothing
// listens on it: every request below is served in-process.
const prodOrigin = "http://127.0.0.1:8080"

// Requests use the same long-lived handler, as they do in the application.
// Recompiling the immutable bundle for every HTTP assertion adds no isolation:
// application state already lives in businesslogic.Default, and request state
// belongs to the request. Pool/capacity tests construct their own sized handler.
var productionHandler = sync.OnceValues(func() (http.Handler, error) {
	dist, err := fs.Sub(web.Build, "build")
	if err != nil {
		return nil, err
	}
	return example.NewHandlerSized(dist, prodOrigin, 2)
})

func newProdHandler(t *testing.T) http.Handler {
	t.Helper()

	h, err := productionHandler()
	if err != nil {
		t.Fatalf("assembling the production stack: %v", err)
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

// docs: end prod-handler

func TestTypedDependencySerialsBelongToEachCaller(t *testing.T) {
	h := newProdHandler(t)
	request := func(cookie *http.Cookie, browser bool, want int) *http.Cookie {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/typed-dependencies/0/42/first/__data.json", nil)
		if cookie != nil {
			req.AddCookie(cookie)
		}
		if browser {
			req.Header.Set("Sec-Fetch-Site", "none")
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		var wire struct {
			Nodes []struct {
				Data json.RawMessage
			}
		}
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &wire) != nil || len(wire.Nodes) != 2 {
			t.Fatalf("data response: %d %s", rec.Code, rec.Body.String())
		}
		value, err := devalue.Parse(string(wire.Nodes[1].Data), nil)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var data struct {
			Label, Ignored string
			Serial         int
		}
		if err := json.Unmarshal(raw, &data); err != nil {
			t.Fatal(err)
		}
		if data.Label != "Order #0" || data.Ignored != "first" || data.Serial != want {
			t.Fatalf("load data = %+v, want Order #0, first, serial %d", data, want)
		}
		for _, assigned := range rec.Result().Cookies() {
			if assigned.Name == "skgo_typed_dependency_visitor" {
				if assigned.Value == "" || assigned.Path != "/" {
					t.Fatalf("invalid assigned cookie: %v", assigned)
				}
				return assigned
			}
		}
		if cookie == nil {
			t.Fatal("new caller received no serial cookie")
		}
		return cookie
	}
	// Unrelated cookieless probes cannot advance a later browser's count.
	probe := request(nil, false, 1)
	request(nil, false, 1)
	browser := request(nil, true, 1)
	request(probe, false, 2)
	request(browser, true, 2)
	request(nil, true, 1)
}

func TestEveryPrerenderedPathIsServedFromItsFile(t *testing.T) {
	dist := prodDist(t)
	manifest, err := skgo.ReadManifest(dist)
	if err != nil {
		t.Fatal(err)
	}
	h := newProdHandler(t)
	if len(manifest.Prerendered) == 0 {
		t.Fatal("the real build has no prerendered paths")
	}
	for _, path := range manifest.Prerendered {
		name := "prerendered" + path
		if !strings.HasSuffix(path, "/__data.json") && !strings.HasPrefix(path, "/_app/remote/") {
			name += ".html"
		}
		written, err := fs.ReadFile(dist, strings.TrimPrefix(name, "/"))
		if err != nil {
			t.Fatalf("Kit did not write %s: %v", path, err)
		}
		rec := get(t, h, path)
		if rec.Code != http.StatusOK || rec.Body.String() != string(written) {
			t.Errorf("%s: status %d, server did not return Kit's file", path, rec.Code)
		}
	}
	page, err := fs.ReadFile(dist, "prerendered/about.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`data-testid="prerender-parent">skgo example`,
		`data-testid="prerender-price">$7.50`,
		`GO_PRERENDER_DEFERRED`,
		`Go prerender remote: atlas`,
	} {
		if !strings.Contains(string(page), want) {
			t.Errorf("Kit's /about file lacks %q", want)
		}
	}
}

func TestPrerenderedGoRemoteIsAStaticAnswer(t *testing.T) {
	manifest, err := skgo.ReadManifest(prodDist(t))
	if err != nil {
		t.Fatal(err)
	}
	h := newProdHandler(t)
	var remotePath string
	for _, path := range manifest.Prerendered {
		if strings.HasPrefix(path, "/_app/remote/") && strings.Contains(path, "/buildReceipt/") {
			remotePath = path
			break
		}
	}
	if remotePath == "" {
		t.Fatal("Kit did not write the fixed-argument Go prerender remote answer")
	}
	rec := get(t, h, remotePath)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Go prerender remote: atlas") {
		t.Fatalf("static prerender remote: status %d, body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "application/json") {
		t.Errorf("content type = %q", rec.Header().Get("Content-Type"))
	}
	missing := remotePath + "-missing"
	if rec := get(t, h, missing); rec.Code != http.StatusNotFound {
		t.Errorf("missing prerender remote returned %d, want 404", rec.Code)
	}
}

func TestDynamicPrerenderEntriesAreServedFromFiles(t *testing.T) {
	dist := prodDist(t)
	manifest, err := skgo.ReadManifest(dist)
	if err != nil {
		t.Fatal(err)
	}
	h := newProdHandler(t)
	for _, slug := range []string{"atlas", "beacon"} {
		path := "/prerender/" + slug
		if !slices.Contains(manifest.Prerendered, path) || !slices.Contains(manifest.Prerendered, path+"/__data.json") {
			t.Fatalf("Kit did not record both files for %s", path)
		}
		written, err := fs.ReadFile(dist, "prerendered/prerender/"+slug+".html")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(written), "Go entry load: "+slug) {
			t.Errorf("Kit's %s file lacks the Go load's literal receipt", path)
		}
		rec := get(t, h, path)
		if rec.Code != http.StatusOK || rec.Body.String() != string(written) {
			t.Errorf("%s: status %d, Go did not serve Kit's file", path, rec.Code)
		}
		data, err := fs.ReadFile(dist, "prerendered/prerender/"+slug+"/__data.json")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "Go entry load: "+slug) {
			t.Fatalf("data artifact %s lacks Go receipt", path)
		}
		rec = get(t, h, path+"/__data.json")
		if rec.Code != http.StatusOK || rec.Body.String() != string(data) {
			t.Errorf("%s/__data.json: status %d, Go did not serve Kit's file", path, rec.Code)
		}
	}
	if slices.Contains(manifest.Prerendered, "/prerender/third") {
		t.Error("the build listed a path entries() never returned")
	}
	for _, path := range []string{"/prerender/third", "/prerender/third/__data.json"} {
		if rec := get(t, h, path); rec.Code != http.StatusNotFound {
			t.Errorf("unlisted %s returned status %d, want 404", path, rec.Code)
		}
	}
}

func TestUniversalInvoiceLoadRendersFilteredDocument(t *testing.T) {
	h := newProdHandler(t)
	rec := get(t, h, "/invoices?overdue=1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	listStart := strings.Index(body, `<ul data-testid="invoices">`)
	if listStart < 0 {
		t.Fatalf("rendered document has no invoice list: %s", body)
	}
	listEnd := strings.Index(body[listStart:], "</ul>")
	if listEnd < 0 {
		t.Fatal("rendered invoice list was not closed")
	}
	list := body[listStart : listStart+listEnd]
	for _, want := range []string{"INV-100", "2026-09-10", "12500 cents"} {
		if !strings.Contains(list, want) {
			t.Errorf("rendered invoice list lacks %q", want)
		}
	}
	if !strings.Contains(body, `data-testid="invoice-total">Total: 12500 cents`) {
		t.Error("rendered document lacks the filtered total")
	}
	for _, absent := range []string{"INV-200", "INV-300", "Total: 19900 cents"} {
		if strings.Contains(list, absent) {
			t.Errorf("filtered document contains %q", absent)
		}
	}
}

// The ordinary rendered and CSR-disabled branches have focused value checks
// elsewhere. SSR-disabled pages must take the shell path through this stack.
func TestProductionSPARouteUsesTheClientShell(t *testing.T) {
	rec := get(t, newProdHandler(t), "/spa")
	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("SPA status = %d: %s", rec.Code, body)
	}
	for _, want := range []string{"<!doctype html>", "<script", "stylesheet"} {
		if !strings.Contains(body, want) {
			t.Errorf("SPA shell missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, `data-testid="app-nav"`) || strings.Contains(body, `data-testid="title"`) {
		t.Fatal("SSR-disabled page rendered application markup")
	}
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
		// Kit's built-in root error page, showing the status and message Kit gives.
		`<h1>404</h1>`,
		`<p>Not Found</p>`,
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

// TestAMissingImageIsNotAnsweredWithADocument keeps a rendered error page off
// the requests that cannot read one. Kit decides by `Sec-Fetch-Dest`
// (`runtime/server/respond.js`) and answers those with four words of plain
// text; a browser asking for an image and getting a full HTML document treats
// it as a broken image either way and pays for the render.
func TestAMissingImageIsNotAnsweredWithADocument(t *testing.T) {
	h := newProdHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/no-such-image.png", nil)
	req.Header.Set("Sec-Fetch-Dest", "image")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status %d, want 404", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/plain") {
		t.Errorf("Content-Type %q, want text/plain", got)
	}
	if got := rec.Header().Get("Vary"); got != "Sec-Fetch-Dest" {
		t.Errorf("Vary %q, want Sec-Fetch-Dest", got)
	}
	if strings.Contains(rec.Body.String(), "data-testid") {
		t.Errorf("a missing image was answered with a rendered document")
	}

	// And the same path without the header is still the visitor's 404 page.
	rec = get(t, h, "/no-such-image.png")
	if !strings.Contains(rec.Body.String(), `<p>Not Found</p>`) {
		t.Errorf("a document request for the same path did not get the error page")
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

// docs: remote-id
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

// docs: end remote-id

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

func TestProductionSpecialRequestsUseTheirOwnDispatch(t *testing.T) {
	h := newProdHandler(t)
	for _, test := range []struct {
		path   string
		status int
		body   string
	}{
		{"/api/todos/__data.json", http.StatusNotFound, ""},
		{"/request-fetch/__route.js", http.StatusBadRequest, "Server-side route resolution disabled"},
	} {
		rec := get(t, h, test.path)
		if rec.Code != test.status || strings.TrimSpace(rec.Body.String()) != test.body {
			t.Errorf("%s: status=%d body=%q, want %d %q", test.path, rec.Code, rec.Body.String(), test.status, test.body)
		}
	}
}

// TestADataErrorCarriesWhatTheAppsHandleErrorHookChose asks the production
// stack for the wire kit's client uses during navigation. The expectation is
// anchored in the app's literal support id and hook message, not in a document
// rendered from the same error object.
func TestADataErrorCarriesWhatTheAppsHandleErrorHookChose(t *testing.T) {
	rec := get(t, newProdHandler(t), "/error/unexpected/__data.json")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /error/unexpected/__data.json: status %d, want 200", rec.Code)
	}

	var envelope struct {
		Type  string `json:"type"`
		Nodes []struct {
			Type  string `json:"type"`
			Error struct {
				Status    int    `json:"status"`
				Message   string `json:"message"`
				SupportID string `json:"supportId"`
			} `json:"error"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("data response is not JSON: %v\n%s", err, rec.Body.String())
	}
	if envelope.Type != "data" {
		t.Fatalf("response type = %q, want data", envelope.Type)
	}
	for _, node := range envelope.Nodes {
		if node.Type != "error" {
			continue
		}
		if node.Error.Status != 500 {
			t.Errorf("error status = %d, want 500", node.Error.Status)
		}
		if node.Error.Message != "Something went wrong on our end." {
			t.Errorf("error message = %q, want the hook's own words", node.Error.Message)
		}
		if node.Error.SupportID != "case-1121" {
			t.Errorf("error supportId = %q, want case-1121", node.Error.SupportID)
		}
		for _, secret := range []string{"hunter2", "postgres://"} {
			if strings.Contains(rec.Body.String(), secret) {
				t.Errorf("data response leaked %q: %s", secret, rec.Body.String())
			}
		}
		return
	}
	t.Fatalf("no error node in data response: %s", rec.Body.String())
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
	if got := rec.Header().Get("Allow"); got != "GET, POST, QUERY, HEAD" {
		t.Errorf("DELETE /api/todos: Allow %q, want %q", got, "GET, POST, QUERY, HEAD")
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

// The endpoint registry is assembled before the renderer. A fatal response
// must still receive the renderer's error.html when the real app is composed.
func TestAFatalServerRouteUsesTheAppsErrorTemplate(t *testing.T) {
	server := httptest.NewServer(newProdHandler(t))
	defer server.Close()

	for _, tc := range []struct {
		accept      string
		contentType string
		bodyPart    string
	}{
		{"application/json", "application/json", `"message":"Something went wrong on our end."`},
		{"text/html", "text/html; charset=utf-8", "<html"},
	} {
		req, err := http.NewRequest(http.MethodGet, server.URL+"/api/fatal", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Accept", tc.accept)
		resp, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		payload, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusInternalServerError {
			t.Errorf("Accept %q: status %d, want 500", tc.accept, resp.StatusCode)
		}
		if got := resp.Header.Get("Content-Type"); got != tc.contentType {
			t.Errorf("Accept %q: Content-Type %q, want %q", tc.accept, got, tc.contentType)
		}
		if !strings.Contains(string(payload), tc.bodyPart) || !strings.Contains(string(payload), "Something went wrong on our end.") {
			t.Errorf("Accept %q: response does not contain the app's public error message: %s", tc.accept, payload)
		}
		if strings.Contains(string(payload), "private endpoint failure") {
			t.Errorf("Accept %q: panic text leaked: %s", tc.accept, payload)
		}
	}
}
