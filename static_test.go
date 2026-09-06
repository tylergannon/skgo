package skgo

import (
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

const testIndexHTML = "<!doctype html><html><body>boot</body></html>"

const testManifest = `{
	"appDir": "_app",
	"base": "",
	"version": "1788665374100",
	"routes": [
		{ "id": "/", "pattern": "^\\/$" },
		{ "id": "/about", "pattern": "^\\/about\\/?$" },
		{ "id": "/items/[id]", "pattern": "^\\/items\\/([^/]+?)\\/?$" },
		{ "id": "/docs/[...rest]", "pattern": "^\\/docs(?:\\/([^]*))?\\/?$" }
	]
}`

func testBuildFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":         {Data: []byte(testIndexHTML)},
		"skgo.manifest.json": {Data: []byte(testManifest)},
		"client/_app/version.json": {
			Data: []byte(`{"version":"1788665374100"}`),
		},
		"client/_app/immutable/entry/start.DWsVriQH.js": {
			Data: []byte("export const start = 1;\n"),
		},
		"client/favicon.svg": {Data: []byte("<svg/>")},
	}
}

func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	h, err := NewStaticHandler(testBuildFS())
	if err != nil {
		t.Fatalf("NewStaticHandler: %v", err)
	}
	return h
}

func do(t *testing.T, h http.Handler, method, target string, header http.Header) *http.Response {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	for k, vs := range header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Result()
}

func body(t *testing.T, resp *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	resp.Body.Close()
	return string(b)
}

func TestStaticHandlerServesExactClientFile(t *testing.T) {
	h := newTestHandler(t)

	resp := do(t, h, http.MethodGet, "/_app/version.json", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := body(t, resp); got != `{"version":"1788665374100"}` {
		t.Errorf("body = %q", got)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if resp.Header.Get("ETag") == "" {
		t.Error("missing ETag")
	}
}

func TestStaticHandlerImmutableCacheHeader(t *testing.T) {
	h := newTestHandler(t)

	resp := do(t, h, http.MethodGet, "/_app/immutable/entry/start.DWsVriQH.js", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
		t.Errorf("Cache-Control = %q", cc)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/javascript") {
		t.Errorf("Content-Type = %q, want text/javascript", ct)
	}
}

func TestStaticHandlerNonImmutableFileHasNoImmutableCache(t *testing.T) {
	h := newTestHandler(t)

	resp := do(t, h, http.MethodGet, "/favicon.svg", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if cc := resp.Header.Get("Cache-Control"); strings.Contains(cc, "immutable") {
		t.Errorf("Cache-Control = %q, should not be immutable", cc)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "image/svg+xml") {
		t.Errorf("Content-Type = %q", ct)
	}
}

func TestStaticHandlerAppDirMissIs404WithEmptyBody(t *testing.T) {
	h := newTestHandler(t)

	resp := do(t, h, http.MethodGet, "/_app/immutable/chunks/nope.js", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	if got := body(t, resp); got != "" {
		t.Errorf("body = %q, want empty (never the boot document)", got)
	}
}

func TestStaticHandlerKnownRoutesServeBootDocument(t *testing.T) {
	h := newTestHandler(t)

	for _, path := range []string{"/", "/about", "/about/", "/items/42", "/items/7"} {
		resp := do(t, h, http.MethodGet, path, nil)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s: status = %d, want 200", path, resp.StatusCode)
			continue
		}
		if ct := resp.Header.Get("Content-Type"); ct != "text/html; charset=utf-8" {
			t.Errorf("%s: Content-Type = %q", path, ct)
		}
		if got := body(t, resp); got != testIndexHTML {
			t.Errorf("%s: body = %q, want boot document", path, got)
		}
	}
}

func TestStaticHandlerUnknownPathServesBootDocumentWith404(t *testing.T) {
	h := newTestHandler(t)

	resp := do(t, h, http.MethodGet, "/nope", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}
	if got := body(t, resp); got != testIndexHTML {
		t.Errorf("body = %q, want boot document", got)
	}
}

func TestStaticHandlerETagAnswers304(t *testing.T) {
	h := newTestHandler(t)

	resp := do(t, h, http.MethodGet, "/_app/version.json", nil)
	etag := resp.Header.Get("ETag")
	if etag == "" {
		t.Fatal("missing ETag")
	}
	body(t, resp)

	resp2 := do(t, h, http.MethodGet, "/_app/version.json", http.Header{"If-None-Match": {etag}})
	if resp2.StatusCode != http.StatusNotModified {
		t.Fatalf("status = %d, want 304", resp2.StatusCode)
	}
	if got := body(t, resp2); got != "" {
		t.Errorf("304 body = %q, want empty", got)
	}
}

func TestStaticHandlerHEAD(t *testing.T) {
	h := newTestHandler(t)

	resp := do(t, h, http.MethodHead, "/", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := body(t, resp); got != "" {
		t.Errorf("HEAD body = %q, want empty", got)
	}

	resp2 := do(t, h, http.MethodHead, "/_app/version.json", nil)
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp2.StatusCode)
	}
	if got := body(t, resp2); got != "" {
		t.Errorf("HEAD body = %q, want empty", got)
	}
}

func TestStaticHandlerRejectsOtherMethods(t *testing.T) {
	h := newTestHandler(t)

	resp := do(t, h, http.MethodPost, "/", nil)
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
	if allow := resp.Header.Get("Allow"); allow != "GET, HEAD" {
		t.Errorf("Allow = %q", allow)
	}
}

func TestStaticHandlerRejectsTraversal(t *testing.T) {
	h := newTestHandler(t)

	// httptest.NewRequest parses the target; use the raw form the net/http
	// server would hand us for an unnormalized path.
	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	req.URL.Path = "/../secret"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	resp := rec.Result()
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("traversal returned 200")
	}
	if got := body(t, resp); strings.Contains(got, "secret") {
		t.Errorf("body leaked: %q", got)
	}
}

func TestNewStaticHandlerRequiresIndexAndManifest(t *testing.T) {
	t.Run("no index.html", func(t *testing.T) {
		build := testBuildFS()
		delete(build, "index.html")
		if _, err := NewStaticHandler(build); err == nil {
			t.Fatal("want error when index.html is missing")
		}
	})

	t.Run("no manifest", func(t *testing.T) {
		build := testBuildFS()
		delete(build, "skgo.manifest.json")
		if _, err := NewStaticHandler(build); err == nil {
			t.Fatal("want error when skgo.manifest.json is missing")
		}
	})

	t.Run("bad manifest json", func(t *testing.T) {
		build := testBuildFS()
		build["skgo.manifest.json"] = &fstest.MapFile{Data: []byte("{")}
		if _, err := NewStaticHandler(build); err == nil {
			t.Fatal("want error for malformed manifest")
		}
	})

	t.Run("bad route pattern", func(t *testing.T) {
		build := testBuildFS()
		build["skgo.manifest.json"] = &fstest.MapFile{
			Data: []byte(`{"appDir":"_app","routes":[{"id":"/","pattern":"("}]}`),
		}
		if _, err := NewStaticHandler(build); err == nil {
			t.Fatal("want error for uncompilable route pattern")
		}
	})
}

func TestStaticHandlerSubFSFromEmbedRoot(t *testing.T) {
	// The example embeds the parent of build/, so callers pass fs.Sub(...).
	outer := fstest.MapFS{}
	for name, file := range testBuildFS() {
		outer["build/"+name] = file
	}
	sub, err := fs.Sub(outer, "build")
	if err != nil {
		t.Fatalf("fs.Sub: %v", err)
	}
	h, err := NewStaticHandler(sub)
	if err != nil {
		t.Fatalf("NewStaticHandler: %v", err)
	}
	resp := do(t, h, http.MethodGet, "/items/1", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

// TestStaticHandlerServesARestParameterRoute: kit writes its route patterns as
// JavaScript regular expressions, and a `[...rest]` segment comes out as
// `(?:/([^]*))?`. Go's regexp cannot compile `[^]` at all, so before this was
// translated an app with one rest route could not start.
func TestStaticHandlerServesARestParameterRoute(t *testing.T) {
	h := newTestHandler(t)
	for _, path := range []string{"/docs", "/docs/guide", "/docs/guide/getting-started"} {
		resp := do(t, h, http.MethodGet, path, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s = %d, want 200 with the boot document", path, resp.StatusCode)
		}
		if got := body(t, resp); got != testIndexHTML {
			t.Fatalf("GET %s did not serve the boot document", path)
		}
	}
}

// TestKitPatternLeavesEverythingElseAlone: the translation is one construct,
// not a rewrite of kit's regular expressions.
func TestKitPatternLeavesEverythingElseAlone(t *testing.T) {
	for src, want := range map[string]string{
		`^\/$`:                     `^\/$`,
		`^\/items\/([^/]+?)\/?$`:   `^\/items\/([^/]+?)\/?$`,
		`^\/docs(?:\/([^]*))?\/?$`: `^\/docs(?:\/([\s\S]*))?\/?$`,
		`^\/x\/y([^]*?)z\/?$`:      `^\/x\/y([\s\S]*?)z\/?$`,
		// A literal `[`, `^` or `]` in a route segment arrives escaped, and an
		// escaped bracket must not be mistaken for the rest construct.
		`^\/a\[\^\]b\/?$`: `^\/a\[\^\]b\/?$`,
	} {
		if got := kitPattern(src); got != want {
			t.Fatalf("kitPattern(%q) = %q, want %q", src, got, want)
		}
	}
}

// TestTheDocumentAnswersItsOwnETag closes the loop the handler left open: it
// advertised a validator on every boot document and then ignored the one the
// browser offered back, so a reload transferred the whole document again. The
// ETag is only a promise if a conditional request can collect on it.
func TestTheDocumentAnswersItsOwnETag(t *testing.T) {
	h := newTestHandler(t)

	first := do(t, h, http.MethodGet, "/about", nil)
	etag := first.Header.Get("ETag")
	if etag == "" {
		t.Fatal("the boot document carries no ETag")
	}
	if got := body(t, first); got != testIndexHTML {
		t.Fatalf("the first response was not the boot document: %q", got)
	}

	second := do(t, h, http.MethodGet, "/about", http.Header{"If-None-Match": {etag}})
	if second.StatusCode != http.StatusNotModified {
		t.Errorf("a request offering the document's own ETag got %d, want 304", second.StatusCode)
	}
	if got := body(t, second); got != "" {
		t.Errorf("a 304 carried a body of %d bytes", len(got))
	}
	// RFC 9110: a 304 repeats the validator and must not claim a length it is
	// not sending.
	if got := second.Header.Get("ETag"); got != etag {
		t.Errorf("the 304 returned ETag %q, want %q", got, etag)
	}
	if got := second.Header.Get("Content-Length"); got != "" {
		t.Errorf("the 304 declared Content-Length %q", got)
	}
}

// A validator matches when it is one of the offered ones, or when the client
// offers `*`; a weak validator matches its own strong form, because the
// comparison a conditional GET uses is the weak one.
func TestTheDocumentETagIsComparedTheWayHTTPSaysToCompareIt(t *testing.T) {
	h := newTestHandler(t)
	etag := do(t, h, http.MethodGet, "/", nil).Header.Get("ETag")

	for _, tc := range []struct {
		name  string
		offer string
		want  int
	}{
		{"the exact validator", etag, http.StatusNotModified},
		{"a list containing it", `"someone-elses", ` + etag, http.StatusNotModified},
		{"the weak form of it", "W/" + etag, http.StatusNotModified},
		{"any validator at all", "*", http.StatusNotModified},
		{"a stale validator", `"0000000000000000"`, http.StatusOK},
		{"nothing offered", "", http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			header := http.Header{}
			if tc.offer != "" {
				header.Set("If-None-Match", tc.offer)
			}
			resp := do(t, h, http.MethodGet, "/", header)
			if resp.StatusCode != tc.want {
				t.Errorf("If-None-Match %q got %d, want %d", tc.offer, resp.StatusCode, tc.want)
			}
		})
	}
}

// A 404 boot document is still the same bytes, so it still validates — but it
// has to stay a 404. Answering 304 to a request the server would have refused
// tells the client the *error* page it already has is still current, which is
// true, and kit's router renders +error.svelte from it either way.
func TestAConditionalRequestForAMissingPageStays404(t *testing.T) {
	h := newTestHandler(t)

	first := do(t, h, http.MethodGet, "/no-such-page", nil)
	if first.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /no-such-page: status %d, want 404", first.StatusCode)
	}

	second := do(t, h, http.MethodGet, "/no-such-page", http.Header{"If-None-Match": {first.Header.Get("ETag")}})
	if second.StatusCode != http.StatusNotFound {
		t.Errorf("a conditional request for a missing page got %d, want 404", second.StatusCode)
	}
	if got := body(t, second); got != testIndexHTML {
		t.Errorf("the 404 stopped returning the document kit's client needs to render +error.svelte")
	}
}

// HEAD gets the same answer as GET, minus the body — including the 304.
func TestAConditionalHEADIsAnsweredLikeAConditionalGET(t *testing.T) {
	h := newTestHandler(t)
	etag := do(t, h, http.MethodGet, "/", nil).Header.Get("ETag")

	resp := do(t, h, http.MethodHead, "/", http.Header{"If-None-Match": {etag}})
	if resp.StatusCode != http.StatusNotModified {
		t.Errorf("conditional HEAD got %d, want 304", resp.StatusCode)
	}
}

// Kit recognises a data request by its pathname suffix before it routes
// anything (`has_data_suffix` in `src/pathname.js`, called at the top of
// `runtime/server/respond.js`), and then branches on `is_data_request` at
// every rendering decision. skgo did neither: it matched the data URL against
// the page routes, and because kit's route patterns end `\/?$` a one-segment
// data URL matches its own page. `/todos/__data.json` came back 200 text/html,
// and kit's client `JSON.parse`d the boot document.
//
// A 200 is the worst of the possible answers, because the failure lands inside
// kit's client instead of at this boundary.

func TestADataRequestIsNeverAnsweredWithTheDocument(t *testing.T) {
	h := newTestHandler(t)

	for _, path := range []string{
		"/about/__data.json",        // one segment: matched its own page route
		"/__data.json",              // the root route
		"/items/7/__data.json",      // a bracketed route
		"/docs/a/b/__data.json",     // swallowed by [...rest]
		"/about.html__data.json",    // kit's HTML_DATA_SUFFIX form
		"/no-such-page/__data.json", // no route at all
	} {
		t.Run(path, func(t *testing.T) {
			resp := do(t, h, http.MethodGet, path, nil)
			if ct := resp.Header.Get("Content-Type"); strings.HasPrefix(ct, "text/html") {
				t.Errorf("answered with Content-Type %q", ct)
			}
			if got := body(t, resp); strings.Contains(got, "<html") || got == testIndexHTML {
				t.Errorf("answered with the boot document: %q", got)
			}
			if resp.StatusCode == http.StatusOK {
				t.Errorf("answered 200; kit never answers a data request 200 with a page")
			}
		})
	}
}

// The status is the thing kit's client keys on. `load_data` refuses to parse
// anything but a 2xx, and its hydration path singles 404 out — "if
// __data.json returned 404, the route doesn't exist — don't reload or we
// loop" — so a 404 leaves the client rendering the route itself rather than
// reloading. Any other status sends it into a full page reload.
func TestADataRequestIsRefusedWithA404TheClientUnderstands(t *testing.T) {
	h := newTestHandler(t)

	resp := do(t, h, http.MethodGet, "/about/__data.json", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	// kit's client spreads the JSON body over `{status}` when the content type
	// says JSON, so the body has to be an App.Error and nothing else.
	var appError struct {
		Status  int    `json:"status"`
		Message string `json:"message"`
	}
	raw := body(t, resp)
	if err := json.Unmarshal([]byte(raw), &appError); err != nil {
		t.Fatalf("the body is not JSON, so kit's client cannot read it: %v\n%s", err, raw)
	}
	if appError.Status != 404 || appError.Message == "" {
		t.Errorf("body = %s, want an App.Error naming the 404", raw)
	}
	if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Errorf("Cache-Control = %q; a data response must not be cached", cc)
	}
}

// Kit's other pathname suffix, for `preloadCode` route resolution
// (`has_resolution_suffix`, `src/pathname.js`). It is a JavaScript module the
// client imports; the boot document served in its place is a syntax error
// inside a dynamic import, which is even harder to read than a bad JSON.parse.
func TestARouteResolutionRequestIsNeverAnsweredWithTheDocument(t *testing.T) {
	h := newTestHandler(t)

	for _, path := range []string{"/about/__route.js", "/__route.js", "/about.html__route.js"} {
		resp := do(t, h, http.MethodGet, path, nil)
		if resp.StatusCode == http.StatusOK {
			t.Errorf("%s answered 200", path)
		}
		if got := body(t, resp); got == testIndexHTML {
			t.Errorf("%s answered with the boot document", path)
		}
	}
}

// A page whose own name ends in the suffix is not a data request — the suffix
// is a whole final segment, or an `.html` variant of one. `/my__data.json` is
// an ordinary path and `/items/[id]` matches it.
func TestOnlyKitsOwnSuffixCountsAsADataRequest(t *testing.T) {
	h := newTestHandler(t)

	resp := do(t, h, http.MethodGet, "/items/my__data.json", nil)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("/items/my__data.json: status %d, want 200 — it is a page, not a data request", resp.StatusCode)
	}
	if got := body(t, resp); got != testIndexHTML {
		t.Errorf("/items/my__data.json did not get the boot document")
	}
}
