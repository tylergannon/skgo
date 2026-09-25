package example_test

// The claims in this file used to be browser scenarios. Each one was about
// bytes — a status, a header, a redirect, the markup of a document before any
// script ran — so a browser added minutes and nothing a real-handler test
// cannot see. The Gherkin suite keeps what only kit's client can prove.
//
// Every expectation below is a literal written here: a fixture the app seeds,
// a string a scenario used to name, or text kit's own rules produce. None is
// read back from the response it checks.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/tylergannon/skgo/example"
	"github.com/tylergannon/skgo/example/businesslogic"
)

// tagged matches one element by its test id and its whole text, allowing the
// scoping class a compiler may add beside the test id.
func tagged(element, testid, text string) *regexp.Regexp {
	return regexp.MustCompile(`<` + element + ` data-testid="` + regexp.QuoteMeta(testid) + `"[^>]*>` +
		regexp.QuoteMeta(text) + `</` + element + `>`)
}

// signedIn asks for path as a visitor with a session for user.
func signedIn(t *testing.T, h http.Handler, user, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Accept", "text/html")
	req.AddCookie(&http.Cookie{Name: example.SessionCookie, Value: businesslogic.Default.SignIn(user)})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func requireMarkup(t *testing.T, what, body string, patterns ...*regexp.Regexp) {
	t.Helper()
	for _, p := range patterns {
		if !p.MatchString(body) {
			t.Errorf("%s: the document does not match %s", what, p)
		}
	}
}

func requireText(t *testing.T, what, body string, texts ...string) {
	t.Helper()
	for _, text := range texts {
		if !strings.Contains(body, text) {
			t.Errorf("%s: the document does not contain %q", what, text)
		}
	}
}

func forbidText(t *testing.T, what, body string, texts ...string) {
	t.Helper()
	for _, text := range texts {
		if strings.Contains(body, text) {
			t.Errorf("%s: the document contains %q", what, text)
		}
	}
}

// TestTheFrontPageIndexesEveryCapabilityItDemonstrates is the index a visitor
// lands on: one entry per capability, in order, linking to the page that shows
// it, with a sentence saying what to look for there. The list is written here
// so that a deleted entry, or a page that listed nothing, fails.
func TestTheFrontPageIndexesEveryCapabilityItDemonstrates(t *testing.T) {
	body := get(t, newProdHandler(t), "/").Body.String()

	want := [][2]string{
		{"Pages rendered in the Go process", "/items/42"},
		{"Route parameters", "/items/7"},
		{"Rest parameters", "/docs/guide/getting-started"},
		{"Remote functions: queries, commands and forms", "/todos"},
		{"Refreshing a query after a command", "/todos/gate"},
		{"One query, one answer per argument", "/todos/pair"},
		{"A query that goes on answering", "/live"},
		{"Many calls answered by one", "/batch"},
		{"Server loads, section-wide", "/account"},
		{"A nested error page", "/account/statement"},
		{"Values a load promises but does not have yet", "/stream"},
		{"Custom types that keep their methods", "/pricing"},
		{"A form that works with JavaScript switched off", "/contact"},
		{"Page form actions", "/actions"},
		{"HTTP endpoints written in Go", "/api"},
		{"Links and asset URLs worked out while the page renders", "/render-paths"},
		{"A page with no client-side JavaScript", "/plain"},
		{"A page rendered only in the browser", "/spa"},
		{"An empty list is still a list", "/empty"},
		{"What the renderer writes reaches Go's log", "/console"},
		{"A load that refuses", "/error/expected"},
		{"A failure the page catches itself", "/error/boundary"},
		{"An error that is a bug says nothing about itself", "/error/unexpected"},
		{"A failure no error page can catch", "/?boom=root-layout"},
	}
	entry := regexp.MustCompile(`<li data-testid="capability"[^>]*><a data-testid="capability-link" href="([^"]*)"([^>]*)>([^<]*)</a>\s*<p data-testid="capability-look"[^>]*>([^<]*)</p>`)
	found := entry.FindAllStringSubmatch(body, -1)
	if len(found) != len(want) {
		t.Fatalf("the front page lists %d capabilities, want %d", len(found), len(want))
	}
	for i, w := range want {
		href, attrs, name, look := found[i][1], found[i][2], found[i][3], found[i][4]
		if name != w[0] || href != w[1] {
			t.Errorf("entry %d is %q -> %s, want %q -> %s", i+1, name, href, w[0], w[1])
		}
		// A sentence rather than a word: what the page proves.
		if len(strings.TrimSpace(look)) <= 30 {
			t.Errorf("entry %d (%s) says only %q about what to look for", i+1, name, look)
		}
		// The root layout's failure replaces the whole document, so its entry
		// is a document request and not a client-side navigation.
		reload := strings.Contains(attrs, "data-sveltekit-reload")
		if reload != (w[1] == "/?boom=root-layout") {
			t.Errorf("entry %d (%s): data-sveltekit-reload present = %v", i+1, name, reload)
		}
	}

	requireMarkup(t, "/", body,
		tagged("p", "greeting", "Hello, skgo! This is Svelte, served by Go."),
		tagged("p", "site-name", "skgo"),
		tagged("p", "colocated", "src/routes/site.remote.go"))
	forbidText(t, "/", body, "skgo: implemented in Go")
}

// TestEveryPageArrivesWithItsOwnHeading walks the app's pages and asks each
// for the `<h1>` its own +page.svelte writes, inside the root layout. A bundle
// that joined the wrong node renders the wrong page's component with a 200 and
// the right layout around it; the heading is what tells them apart. The two
// /items paths and the rest parameter are there because the engine parses
// every URL with Go's `URL`.
func TestEveryPageArrivesWithItsOwnHeading(t *testing.T) {
	h := newProdHandler(t)
	for _, tc := range []struct{ path, heading string }{
		{"/", "Home"},
		{"/plain", "Plain"},
		{"/items/7", "Item 7"},
		{"/items/93", "Item 93"},
		{"/items/93?from=nav", "Item 93"},
		{"/todos", "Todos"},
		{"/todos/t1", "Todo"},
		{"/todos/gate", "The refresh gate"},
		{"/todos/pair", "A pair of todos"},
		{"/empty", "Empty"},
		{"/pricing", "Pricing"},
		{"/docs/guide/getting-started", "Docs"},
		{"/contact", "Contact"},
		{"/stream", "Stream"},
		{"/console", "Console"},
		{"/live", "Live"},
		{"/batch", "Batch"},
		{"/account/orders", "Orders"},
		{"/render-paths", "Render-time paths"},
	} {
		rec := signedIn(t, h, "ada", tc.path)
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s: status %d, want 200", tc.path, rec.Code)
			continue
		}
		body := rec.Body.String()
		requireMarkup(t, tc.path, body, tagged("h1", "title", tc.heading))
		requireText(t, tc.path, body, `data-testid="app-nav"`,
			// The root layout's own load, on every page's way through.
			`<footer data-testid="deployment">skgo example</footer>`)
	}

	// A route directory named `[id]` answers from the Go beside it.
	body := get(t, h, "/items/7").Body.String()
	requireMarkup(t, "/items/7", body,
		tagged("p", "item-name", "Widget 7"),
		tagged("p", "colocated", "src/routes/items/[id]/item.remote.go"))

	// csr = false: the document is all there will ever be.
	body = get(t, h, "/plain").Body.String()
	requireMarkup(t, "/plain", body, tagged("p", "site-name", "skgo"))
	forbidText(t, "/plain", body, "<script")

	// The todo list is Go's answer, rendered: the seeded rows are there, and
	// the live count above them counts exactly the rows the document carries.
	body = get(t, h, "/todos").Body.String()
	requireText(t, "/todos", body, ">write the adapter</a>", ">serve remote functions</a>")
	count := regexp.MustCompile(`<strong data-testid="count">(\d+)</strong>`).FindStringSubmatch(body)
	rows := strings.Count(body, `data-testid="todo"`)
	if count == nil || rows == 0 || count[1] != fmt.Sprint(rows) {
		t.Errorf("/todos: live count %v beside %d rendered todos", count, rows)
	}
}

// TestAFailingPageIsTheErrorPageKitWouldRender is each deliberate failure in
// the app, as the document says it: the status the failure carried, the error
// page kit chooses for it, and what the app's handleError hook adds (support id
// case-1121) or replaces (an unknown error's own words).
func TestAFailingPageIsTheErrorPageKitWouldRender(t *testing.T) {
	h := newProdHandler(t)
	for _, tc := range []struct {
		path   string
		status int
		want   []*regexp.Regexp
		text   []string
		never  []string
	}{
		{"/error/expected", 418,
			[]*regexp.Regexp{tagged("h1", "title", "Error 418"), tagged("p", "error-message", "This page is a teapot")},
			[]string{"case-1121", `data-testid="app-nav"`}, nil},
		{"/error/unexpected", 500,
			[]*regexp.Regexp{tagged("h1", "title", "Error 500"), tagged("p", "error-message", "Something went wrong on our end.")},
			[]string{"case-1121"}, []string{"Internal Error", "hunter2", "postgres://"}},
		{"/account/statement", 402,
			[]*regexp.Regexp{tagged("h1", "title", "Account error 402"), tagged("p", "error-message", "Your account is in arrears"), tagged("p", "account-user", "Account of ada")},
			[]string{"case-1121"}, nil},
		{"/error/boundary", 409,
			[]*regexp.Regexp{tagged("h1", "title", "Sensor")},
			[]string{"The sensor is being calibrated"}, nil},
		{"/error/command", 500,
			[]*regexp.Regexp{tagged("h1", "title", "Error 500"), tagged("p", "error-message", "Internal Error")},
			nil, []string{"Cannot call a command", `data-testid="tally"`}},
		{"/error/render", 500, nil,
			[]string{`<span class="status">500</span>`, "<h1>Something went wrong on our end.</h1>"},
			[]string{"case-1121", "<script"}},
		{"/no-such-page", 404, nil,
			[]string{"<h1>404</h1>", "<p>Not Found</p>", `data-testid="app-nav"`}, nil},
	} {
		rec := signedIn(t, h, "ada", tc.path)
		if rec.Code != tc.status {
			t.Errorf("GET %s: status %d, want %d", tc.path, rec.Code, tc.status)
		}
		body := rec.Body.String()
		requireMarkup(t, tc.path, body, tc.want...)
		requireText(t, tc.path, body, tc.text...)
		forbidText(t, tc.path, body, tc.never...)
	}
}

// TestTheAccountSectionTurnsASignedOutVisitorAway is the section's one rule,
// written once in its layout: every page under /account redirects a signed-out
// visitor home with kit's bare 307, and lets a signed-in one through.
func TestTheAccountSectionTurnsASignedOutVisitorAway(t *testing.T) {
	h := newProdHandler(t)
	for _, path := range []string{"/account", "/account/orders", "/account/statement"} {
		rec := get(t, h, path)
		if rec.Code != http.StatusTemporaryRedirect || rec.Header().Get("Location") != "/" || rec.Body.Len() != 0 {
			t.Errorf("signed out, GET %s: %d to %q with %d bytes, want 307 to \"/\" with no body",
				path, rec.Code, rec.Header().Get("Location"), rec.Body.Len())
		}
	}
	rec := signedIn(t, h, "grace", "/account/orders")
	if rec.Code != http.StatusOK {
		t.Fatalf("signed in, GET /account/orders: status %d", rec.Code)
	}
	requireMarkup(t, "/account/orders", rec.Body.String(), tagged("p", "account-user", "Account of grace"))
}

// TestHydrationDataCannotCloseItsScriptElement: a string the load returns may
// contain `</script>`, and the payload carrying it sits inside a script
// element. Written raw, it would end that element and run whatever followed.
func TestHydrationDataCannotCloseItsScriptElement(t *testing.T) {
	body := get(t, newProdHandler(t), "/?proof=script-safe").Body.String()
	const injected = "</script><script>globalThis.__skgo_injected=true</script>"
	forbidText(t, "/?proof=script-safe", body, injected)
	requireText(t, "/?proof=script-safe", body,
		`scriptSafe:"\u003C/script>\u003Cscript>globalThis.__skgo_injected=true\u003C/script>&\u003C>"`,
		`&lt;/script>&lt;script>globalThis.__skgo_injected=true&lt;/script>&amp;&lt;>`)
}

// TestPathsResolveDuringARenderByKitsRules: `resolve` and `asset` are kit's
// string logic over the compiled-in base, relative during server rendering
// (paths.relative defaults to true); `match` asks Go's route table.
func TestPathsResolveDuringARenderByKitsRules(t *testing.T) {
	body := get(t, newProdHandler(t), "/render-paths").Body.String()
	requireText(t, "/render-paths", body,
		"resolve('/api/todos') = ./api/todos",
		"resolve('/items/[id]', id: '42') = ./items/42",
		"asset('robots.txt') = ./robots.txt",
		`match('/items/77') = /items/[id] {"id":"77"}`)
}

// TestANilGoSliceRendersAsAnEmptyList: Go writes nil for "no rows", and the
// TypeScript it generates promises an array. The page counts with `.length`,
// so a null would have thrown during the render. The control beside it, same
// page and same encoder, is a list with rows in it.
func TestANilGoSliceRendersAsAnEmptyList(t *testing.T) {
	body := get(t, newProdHandler(t), "/empty").Body.String()
	requireMarkup(t, "/empty", body,
		tagged("h2", "report-title", "Parse failed"),
		tagged("p", "report-count", "0 diagnostics"),
		tagged("li", "report-empty", "No diagnostics."),
		tagged("p", "models-count", "0 models"),
		tagged("li", "models-empty", "No models."),
		tagged("p", "notes-count", "0 notes"),
		tagged("li", "notes-empty", "No notes."),
		tagged("h2", "known-title", "Parsed"),
		tagged("p", "known-count", "2 diagnostics"),
		tagged("li", "known-diagnostic", "unused import fmt"),
		tagged("li", "known-diagnostic", "missing return"))
	forbidText(t, "/empty", body, `data-testid="report-failed"`, "diagnostics:null", "notes:null")
}

// TestTheBatchPageIsAnsweredInOneCallOfFour: four components each ask for one
// symbol while the page renders, and Go is asked once with all four.
func TestTheBatchPageIsAnsweredInOneCallOfFour(t *testing.T) {
	body := get(t, newProdHandler(t), "/batch").Body.String()
	for _, q := range [][2]string{{"SKGO", "12.75"}, {"GOJA", "34.60"}, {"KITX", "56.10"}, {"SVLT", "78.45"}} {
		row := regexp.MustCompile(`<tr data-testid="quote" data-symbol="` + q[0] + `">.*?` +
			`<td data-testid="quote-price">` + regexp.QuoteMeta(q[1]) + `</td>.*?` +
			`<td data-testid="quote-batch">batch of 4</td>`)
		requireMarkup(t, "/batch", body, row)
	}
}

// TestALiveQueryRendersItsFirstValueIntoTheDocument: a `query.live` awaited
// during the render is answered by Go before any script runs, as frame 1.
func TestALiveQueryRendersItsFirstValueIntoTheDocument(t *testing.T) {
	h := newProdHandler(t)
	const text = "the board arrived rendered"
	addFixtureTodo(t, h, text)
	body := get(t, h, "/live").Body.String()
	requireMarkup(t, "/live", body,
		tagged("strong", "board-newest", text),
		tagged("strong", "board-push", "1"))
	forbidText(t, "/live", body, "skgo: implemented in Go")
}

// TestAWellFormedCommandArgumentIsAnswered is the control for the refusals in
// wire_test.go: the same command with an argument of the right shape runs.
func TestAWellFormedCommandArgumentIsAnswered(t *testing.T) {
	h := newProdHandler(t)
	id := addFixtureTodo(t, h, "a todo the rename control made")
	data := envelopeData(t, callCommand(t, h, remoteID(t, "renameTodo"),
		fmt.Sprintf(`[{"id":1,"text":2},%q,"renamed by a well-formed argument"]`, id)))
	if data == "" {
		t.Fatal("renameTodo answered no result")
	}
	requireTodoText(t, h, id, "renamed by a well-formed argument")
}

// TestWhatTheRendererReportsReachesTheServerLog: the engine's `console` is
// bound to Go's log. /console reports a failure while it renders, the way kit
// and Svelte report one, and the page still renders.
func TestWhatTheRendererReportsReachesTheServerLog(t *testing.T) {
	h := newProdHandler(t)
	var logged bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&logged)
	defer log.SetOutput(previous)

	body := get(t, h, "/console").Body.String()
	requireMarkup(t, "/console", body,
		tagged("h1", "title", "Console"),
		tagged("p", "console-note", "This page reported a failure to the console while it rendered."))
	if !strings.Contains(logged.String(), "skgo-console-probe: this page reported a failure while rendering") {
		t.Errorf("the server's log did not carry the render's report:\n%s", logged.String())
	}
}

// TestKitsReservedQueryParametersAreRefused: kit keeps `x-sveltekit-*` for
// itself and refuses a page request that uses one.
func TestKitsReservedQueryParametersAreRefused(t *testing.T) {
	rec := get(t, newProdHandler(t), "/?x-sveltekit-private=1")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status %d, want 400", rec.Code)
	}
	if got := rec.Body.String(); got != `Cannot use reserved query parameter "x-sveltekit-private"` {
		t.Errorf("body %q", got)
	}
}

// TestAnEndpointRouteAnswersEveryMethodAsWritten is /api/todos from the
// outside: what the route manifest says about it, and what each method gets.
// TestAServerRouteAnswersItsOwnMethods covers GET, POST's 201 and DELETE's 405.
func TestAnEndpointRouteAnswersEveryMethodAsWritten(t *testing.T) {
	h := newProdHandler(t)
	send := func(method, target, body string, headers map[string]string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set("Origin", prodOrigin)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	// SvelteKit 3's `$app/manifest`, rendered by the /api page.
	requireText(t, "/api", get(t, h, "/api").Body.String(),
		`<li data-testid="api-manifest-route"><code>/api</code> — page: true, endpoint: false</li>`,
		`<li data-testid="api-manifest-route"><code>/api/todos</code> — page: false, endpoint: true</li>`)

	// A browser asking for it directly asks for HTML, and still gets JSON:
	// the route has no page to offer.
	rec := send(http.MethodGet, "/api/todos", "", map[string]string{"Accept": "text/html"})
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/json" || !strings.Contains(rec.Body.String(), `"text":"serve remote functions"`) {
		t.Errorf("GET /api/todos as a browser: %d %q %s", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
	}

	// SvelteKit 3 routes the QUERY method.
	rec = send("QUERY", "/api/todos", `{"contains":"adapter"}`, map[string]string{"Content-Type": "application/json"})
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("QUERY /api/todos: %d %q %s", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
	}
	var matches []businesslogic.Todo
	if err := json.Unmarshal(rec.Body.Bytes(), &matches); err != nil || len(matches) != 1 || matches[0].Text != "write the adapter" {
		t.Errorf("QUERY /api/todos for \"adapter\" answered %s", rec.Body.String())
	}

	// What POST created is in the list, by the id it answered with.
	rec = send(http.MethodPost, "/api/todos", `{"text":"walk to the harbour"}`, map[string]string{"Content-Type": "application/json"})
	var created businesslogic.Todo
	if rec.Code != 201 || json.Unmarshal(rec.Body.Bytes(), &created) != nil || created.ID == "" {
		t.Fatalf("POST /api/todos: %d %s", rec.Code, rec.Body.String())
	}
	var all []businesslogic.Todo
	if err := json.Unmarshal(get(t, h, "/api/todos").Body.Bytes(), &all); err != nil {
		t.Fatal(err)
	}
	listed := false
	for _, todo := range all {
		listed = listed || (todo.ID == created.ID && todo.Text == "walk to the harbour")
	}
	if !listed {
		t.Errorf("the created todo %s is not in the list: %v", created.ID, all)
	}

	// An undeclared method is kit's plain-text 405, not a page.
	rec = send(http.MethodDelete, "/api/todos", "", nil)
	if rec.Code != 405 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain") {
		t.Errorf("DELETE /api/todos: %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}

	// A trailing slash is redirected, not served twice.
	rec = get(t, h, "/api/todos/")
	if rec.Code != http.StatusPermanentRedirect || rec.Header().Get("Location") != "../todos" {
		t.Errorf("GET /api/todos/: %d to %q, want 308 to ../todos", rec.Code, rec.Header().Get("Location"))
	}
}

// TestANativeActionIsAnsweredWithTheDocument is every page action a browser
// with no client posts, as the bytes Go sends back: the status, the headers,
// and the page rendered over what the action returned. A browser posting the
// same forms is in the *-form-noscript features; a browser whose client then
// boots over the answer is in actions.feature.
func TestANativeActionIsAnsweredWithTheDocument(t *testing.T) {
	h := newProdHandler(t)

	// Each subtest is a fresh visitor, with its own Ada workspace.
	workspace := func(t *testing.T) []*http.Cookie {
		t.Helper()
		page := actionRequest(h, http.MethodGet, "/actions", "", http.Header{"Accept": {"text/html"}})
		if page.Code != 200 || !strings.Contains(page.Body.String(), `<input name="name" value="Ada Lovelace"`) {
			t.Fatalf("the Ada fixture did not render: %d", page.Code)
		}
		return page.Result().Cookies()
	}
	native := func(t *testing.T, target, form, accept string, cookies []*http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		return actionRequest(h, http.MethodPost, target, form, http.Header{
			"Accept":       {accept},
			"Origin":       {prodOrigin},
			"Content-Type": {"application/x-www-form-urlencoded"},
		}, cookies...)
	}
	page := func(t *testing.T, path string, cookies []*http.Cookie) string {
		t.Helper()
		rec := actionRequest(h, http.MethodGet, path, "", http.Header{"Accept": {"text/html"}}, cookies...)
		if rec.Code != 200 {
			t.Fatalf("GET %s: status %d", path, rec.Code)
		}
		return rec.Body.String()
	}
	grace := url.Values{"name": {"Grace Hopper"}, "email": {"grace@example.test"}, "biography": {"Compiler pioneer"}}
	invalid := url.Values{"name": {"Grace Hopper"}, "email": {"grace-at-example"}, "biography": {"Keep this biography"}}
	profile := func(prefix, name, email, biography, state string) []*regexp.Regexp {
		return []*regexp.Regexp{
			tagged("p", prefix+"name", name), tagged("p", prefix+"email", email),
			tagged("p", prefix+"biography", biography), tagged("p", prefix+"state", state),
		}
	}
	ada := profile("saved-", "Ada Lovelace", "ada@example.test", "First programmer", "Active")

	t.Run("an action-only page saves through its unnamed action", func(t *testing.T) {
		cookies := workspace(t)
		rec := native(t, "/actions/default", grace.Encode(), "text/html", cookies)
		if rec.Code != 200 {
			t.Fatalf("status %d", rec.Code)
		}
		requireMarkup(t, "/actions/default", rec.Body.String(),
			append(profile("default-saved-", "Grace Hopper", "grace@example.test", "Compiler pioneer", "Active"),
				tagged("h2", "default-receipt", "Saved Grace Hopper"), tagged("p", "default-status", "Page status 200"))...)
		requireMarkup(t, "/actions/default/saved", page(t, "/actions/default/saved", cookies),
			profile("default-stored-", "Grace Hopper", "grace@example.test", "Compiler pioneer", "Active")...)
	})

	t.Run("the clicked button's formaction chooses the action", func(t *testing.T) {
		cookies := workspace(t)
		form := url.Values{"name": {"Grace Hopper"}, "email": {"grace@example.test"}, "biography": {"Compiler pioneer"}, "choice": {"save"}}
		rec := native(t, "/actions?/save", form.Encode(), "text/html", cookies)
		requireMarkup(t, "save", rec.Body.String(),
			append(profile("saved-", "Grace Hopper", "grace@example.test", "Compiler pioneer", "Active"),
				tagged("p", "action-receipt", "Saved Grace Hopper"), tagged("p", "action-money", "$7.50"))...)

		cookies = workspace(t)
		form.Set("choice", "archive")
		rec = native(t, "/actions?/archive", form.Encode(), "text/html", cookies)
		requireMarkup(t, "archive", rec.Body.String(),
			append(profile("saved-", "Ada Lovelace", "ada@example.test", "First programmer", "Archived"),
				tagged("p", "no-receipt", "No action receipt is present."))...)
	})

	for _, tc := range []struct{ selected, other, name, email, biography string }{
		{"ada", "grace", "Ada Byron", "ada.byron@example.test", "Analytical engine notes"},
		{"grace", "ada", "Grace Murray Hopper", "grace.murray@example.test", "COBOL language pioneer"},
	} {
		t.Run("a route parameter selects only "+tc.selected, func(t *testing.T) {
			cookies := workspace(t)
			path := "/actions/profiles/" + tc.selected
			form := url.Values{"name": {tc.name}, "email": {tc.email}, "biography": {tc.biography}}
			rec := native(t, path+"?/save", form.Encode(), "text/html", cookies)
			if rec.Code != 200 {
				t.Fatalf("status %d", rec.Code)
			}
			fixture := map[string][]*regexp.Regexp{
				"ada":   profile("ada-", "Ada Lovelace", "ada@example.test", "First programmer", "Active"),
				"grace": profile("grace-", "Grace Hopper", "grace@example.test", "Compiler pioneer", "Active"),
			}
			edited := profile(tc.selected+"-", tc.name, tc.email, tc.biography, "Active")
			requireMarkup(t, path, rec.Body.String(), append(append(edited, fixture[tc.other]...),
				tagged("p", "profile-receipt", "Saved "+tc.name))...)
			requireMarkup(t, path+" later", page(t, path, cookies), append(edited, fixture[tc.other]...)...)
		})
	}

	t.Run("an action's cookie reaches the load rendering the same document", func(t *testing.T) {
		cookies := workspace(t)
		rec := native(t, "/actions?/remember", "", "text/html", cookies)
		if rec.Code != 200 || !strings.Contains(rec.Header().Get("Set-Cookie"), "skgo_actions_feedback=violet-42") {
			t.Fatalf("status %d, Set-Cookie %q", rec.Code, rec.Header().Get("Set-Cookie"))
		}
		requireMarkup(t, "remember", rec.Body.String(),
			append(ada, tagged("p", "action-cookie-value", "violet-42"), tagged("p", "action-receipt", "Remembered violet-42"))...)
		later := page(t, "/actions", append(cookies, rec.Result().Cookies()...))
		requireMarkup(t, "remember later", later, append(ada, tagged("p", "action-cookie-value", "violet-42"))...)
	})

	t.Run("a native save carries the action's response header", func(t *testing.T) {
		rec := native(t, "/actions?/save", grace.Encode(), "text/html", workspace(t))
		if rec.Code != 200 || rec.Header().Get("X-Skgo-Action-Demo") != "profile-saved" {
			t.Fatalf("status %d, X-Skgo-Action-Demo %q", rec.Code, rec.Header().Get("X-Skgo-Action-Demo"))
		}
		requireMarkup(t, "save", rec.Body.String(),
			append(profile("saved-", "Grace Hopper", "grace@example.test", "Compiler pioneer", "Active"),
				tagged("p", "action-receipt", "Saved Grace Hopper"))...)
	})

	t.Run("the handle hook redirects a guarded edit before it runs", func(t *testing.T) {
		cookies := workspace(t)
		rec := native(t, "/actions?hook=sign-in&/save", grace.Encode(), "text/html", cookies)
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/actions/signed-in?required=1" {
			t.Fatalf("%d to %q, want 303 to /actions/signed-in?required=1", rec.Code, rec.Header().Get("Location"))
		}
		requireMarkup(t, "hook destination", page(t, "/actions/signed-in?required=1", cookies),
			tagged("h1", "hook-sign-in-title", "Sign in required"),
			tagged("p", "hook-sign-in-message", "The request was intercepted before the profile action ran."))
		requireMarkup(t, "after the hook redirect", page(t, "/actions", cookies), ada...)
	})

	t.Run("the handle hook refuses a guarded edit with kit's static page", func(t *testing.T) {
		cookies := workspace(t)
		rec := native(t, "/actions?hook=forbidden&/save", grace.Encode(), "text/html", cookies)
		if rec.Code != 403 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") {
			t.Fatalf("status %d, Content-Type %q", rec.Code, rec.Header().Get("Content-Type"))
		}
		requireText(t, "hook refusal", rec.Body.String(), `<span class="status">403</span>`, "<h1>Hook denied this edit</h1>")
		forbidText(t, "hook refusal", rec.Body.String(), `data-testid="action-error-title"`)
		requireMarkup(t, "after the hook refusal", page(t, "/actions", cookies), ada...)
	})

	t.Run("a GET-only sibling endpoint leaves POST to the default action", func(t *testing.T) {
		cookies := workspace(t)
		rec := actionRequest(h, http.MethodGet, "/actions/default", "", http.Header{"Accept": {"application/json"}}, cookies...)
		if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `{"answer":"Default sibling GET answered by Go"}` {
			t.Errorf("JSON GET: %d %s", rec.Code, rec.Body.String())
		}
		rec = native(t, "/actions/default", grace.Encode(), "text/html, application/json;q=0", cookies)
		if rec.Code != 200 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") {
			t.Fatalf("Accept with JSON at q=0: %d %q", rec.Code, rec.Header().Get("Content-Type"))
		}
		requireMarkup(t, "q=0", rec.Body.String(), tagged("h2", "default-receipt", "Saved Grace Hopper"))
	})

	t.Run("a page without client JavaScript answers with a script-free document", func(t *testing.T) {
		cookies := workspace(t)
		rec := native(t, "/actions/options/no-client?/save", grace.Encode(), "text/html", cookies)
		if rec.Code != 200 {
			t.Fatalf("save: status %d", rec.Code)
		}
		requireMarkup(t, "no-client save", rec.Body.String(),
			append(profile("no-client-saved-", "Grace Hopper", "grace@example.test", "Compiler pioneer", "Active"),
				tagged("p", "no-client-receipt", "Saved Grace Hopper"), tagged("p", "no-client-status", "Page status 200"))...)
		forbidText(t, "no-client save", rec.Body.String(), "<script")

		rec = native(t, "/actions/options/no-client?/save", invalid.Encode(), "text/html", workspace(t))
		if rec.Code != 422 {
			t.Fatalf("invalid save: status %d", rec.Code)
		}
		body := rec.Body.String()
		requireMarkup(t, "no-client rejection", body,
			append(profile("no-client-saved-", "Ada Lovelace", "ada@example.test", "First programmer", "Active"),
				tagged("p", "no-client-status", "Page status 422"))...)
		requireText(t, "no-client rejection", body, "Enter a valid email address",
			`<input name="email" value="grace-at-example"`, "Keep this biography</textarea>")
		forbidText(t, "no-client rejection", body, "<script")
	})

	t.Run("a client-rendered page answers a native action with kit's shell", func(t *testing.T) {
		for _, tc := range []struct {
			action, form string
			status       int
		}{
			{"save", grace.Encode(), 200},
			{"save", invalid.Encode(), 422},
			{"forbidden", "", 403},
			{"unavailable", "", 500},
		} {
			rec := native(t, "/actions/options/no-ssr?/"+tc.action, tc.form, "text/html", workspace(t))
			body := rec.Body.String()
			if rec.Code != tc.status {
				t.Errorf("%s: status %d, want %d", tc.action, rec.Code, tc.status)
			}
			// The shell still boots kit and still carries the branch's styles;
			// it renders none of the page and none of the action's outcome.
			requireText(t, tc.action, body, "<!doctype html>", "kit.start(app, element", `rel="stylesheet"`)
			forbidText(t, tc.action, body, `data-testid="no-ssr-title"`, "Saved Grace Hopper",
				"Enter a valid email address", "Action error 403", "Action error 500")
		}

		cookies := workspace(t)
		rec := native(t, "/actions/options/no-ssr?/signIn", "username=ada", "text/html", cookies)
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/actions/signed-in" ||
			!strings.Contains(rec.Header().Get("Set-Cookie"), "skgo_actions_signin=ada") {
			t.Fatalf("signIn: %d to %q, Set-Cookie %q", rec.Code, rec.Header().Get("Location"), rec.Header().Get("Set-Cookie"))
		}
		requireMarkup(t, "signed in", page(t, "/actions/signed-in", append(cookies, rec.Result().Cookies()...)),
			tagged("h1", "signed-in-title", "Signed in as ada"))
	})
}
