package skgo

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// endpointFixture is a two-route app: `/api/thing` is an endpoint and nothing
// else, and `/dual` has both a page and an endpoint — the case where kit decides
// between the two by content negotiation.
func endpointFixture(handlers map[string]http.HandlerFunc, methods, dualMethods []string) EndpointConfig {
	cfg := EndpointConfig{
		AppDir: "_app",
		Origin: "https://example.test",
		Routes: []ManifestRoute{
			{ID: "/api/thing", Pattern: `^\/api\/thing\/?$`, Endpoint: &ManifestEndpoint{Methods: methods}},
			{ID: "/dual", Pattern: `^\/dual\/?$`, Page: &ManifestPage{Layouts: []int{0}, Leaf: 1},
				Endpoint: &ManifestEndpoint{Methods: dualMethods}},
			{ID: "/plain", Pattern: `^\/plain\/?$`, Page: &ManifestPage{Layouts: []int{0}, Leaf: 2}},
		},
		manifest: true,
	}
	_ = handlers
	return cfg
}

func echo(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(name))
	}
}

// pageSentinel stands in for whatever serves pages. Anything that reaches it is
// something the endpoint registry decided not to answer.
func pageSentinel() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("page"))
	})
}

func newEndpoints(t *testing.T, cfg EndpointConfig, eps ...*Endpoint) http.Handler {
	t.Helper()
	es, err := NewEndpoints(cfg, eps...)
	if err != nil {
		t.Fatalf("NewEndpoints: %v", err)
	}
	return es.Intercept(pageSentinel())
}

func request(t *testing.T, h http.Handler, method, target string, header http.Header) *http.Response {
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

// TestAnEndpointRouteNeverGetsThePageHandler is issue #19 in one assertion. The
// bug was not the 405 on POST; it was the 200 on GET, which handed kit's client
// an HTML document where its caller expected an API response.
func TestAnEndpointRouteNeverGetsThePageHandler(t *testing.T) {
	h := newEndpoints(t, endpointFixture(nil, []string{"GET"}, []string{"GET"}),
		NewEndpoint("/api/thing", "GET", echo("thing")),
		NewEndpoint("/dual", "GET", echo("dual")),
	)

	resp := request(t, h, http.MethodGet, "/api/thing", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := body(t, resp); got != "thing" {
		t.Fatalf("body = %q, want the endpoint's own body", got)
	}
}

// Kit answers HEAD with the GET handler when the module exports no HEAD of its
// own (`runtime/server/endpoint.js`), and lists the synthesized HEAD in `Allow`
// (`allowed_methods`).
func TestHeadIsAnsweredByTheGetHandler(t *testing.T) {
	h := newEndpoints(t, endpointFixture(nil, []string{"GET"}, nil),
		NewEndpoint("/api/thing", "GET", echo("thing")),
	)

	// Over a real connection, not an httptest.ResponseRecorder: kit returns the
	// GET response verbatim and lets the HTTP layer drop the body, and net/http
	// is that layer. A recorder is not, so it would keep the body and this test
	// would be about the recorder.
	server := httptest.NewServer(h)
	defer server.Close()

	resp, err := server.Client().Head(server.URL + "/api/thing")
	if err != nil {
		t.Fatalf("HEAD: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := body(t, resp); got != "" {
		t.Errorf("HEAD returned a body %q", got)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q; HEAD must carry the headers GET would", ct)
	}
}

func TestAnUndeclaredMethodGetsKitsMethodNotAllowed(t *testing.T) {
	h := newEndpoints(t, endpointFixture(nil, []string{"GET", "PUT"}, nil),
		NewEndpoint("/api/thing", "GET", echo("thing")),
		NewEndpoint("/api/thing", "PUT", echo("put")),
	)

	// Same-origin, because a cross-site DELETE is refused earlier and for a
	// different reason; see TestACrossSiteFormPostIsRefused.
	resp := request(t, h, http.MethodDelete, "/api/thing", http.Header{"Origin": {"https://example.test"}})
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
	// Kit's order is ENDPOINT_METHODS' order, with the synthesized HEAD last.
	if got := resp.Header.Get("Allow"); got != "GET, PUT, HEAD" {
		t.Errorf("Allow = %q, want %q", got, "GET, PUT, HEAD")
	}
	if got := body(t, resp); got != "DELETE method not allowed" {
		t.Errorf("body = %q, want kit's own text", got)
	}

	// Kit synthesizes nothing for OPTIONS on an endpoint route: a route answers
	// it only if its module exports it, and otherwise it is another 405.
	resp = request(t, h, http.MethodOptions, "/api/thing", nil)
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("OPTIONS: status = %d, want 405", resp.StatusCode)
	}
}

// A `fallback` export answers every method the module does not name, and it
// does so before the 405 — but it never appears in `Allow`, because kit builds
// that header from the method exports alone.
func TestAFallbackAnswersEveryOtherMethod(t *testing.T) {
	h := newEndpoints(t, endpointFixture(nil, []string{"GET", "*"}, nil),
		NewEndpoint("/api/thing", "GET", echo("get")),
		NewEndpoint("/api/thing", "*", echo("fallback")),
	)

	sameOrigin := http.Header{"Origin": {"https://example.test"}}
	for _, method := range []string{http.MethodDelete, http.MethodPatch, "MOVE"} {
		resp := request(t, h, method, "/api/thing", sameOrigin)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s: status = %d, want 200", method, resp.StatusCode)
			continue
		}
		if got := body(t, resp); got != "fallback" {
			t.Errorf("%s: body = %q, want the fallback's", method, got)
		}
	}

	if got := body(t, request(t, h, http.MethodGet, "/api/thing", nil)); got != "get" {
		t.Errorf("GET reached the fallback rather than its own handler: %q", got)
	}
}

// A route with both a `+page` and a `+server` is decided by the Accept header,
// with kit's own preference: `*/*` — a curl, a fetch with no Accept — takes the
// endpoint, and a browser navigation takes the page.
func TestARouteWithBothAPageAndAnEndpointNegotiates(t *testing.T) {
	h := newEndpoints(t, endpointFixture(nil, nil, []string{"GET"}),
		NewEndpoint("/dual", "GET", echo("endpoint")),
	)

	cases := []struct {
		accept string
		want   string
	}{
		{"", "endpoint"},
		{"*/*", "endpoint"},
		{"application/json", "endpoint"},
		{"text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8", "page"},
		{"text/html", "page"},
	}
	for _, tc := range cases {
		header := http.Header{}
		if tc.accept != "" {
			header.Set("Accept", tc.accept)
		}
		resp := request(t, h, http.MethodGet, "/dual", header)
		if got := body(t, resp); got != tc.want {
			t.Errorf("Accept %q: %q answered, want %q", tc.accept, got, tc.want)
		}
		if got := resp.Header.Get("Vary"); !strings.Contains(got, "Accept") {
			t.Errorf("Accept %q: Vary = %q, want it to name Accept", tc.accept, got)
		}
	}
}

// Kit prefers the page whenever the endpoint cannot handle the GET, HEAD or
// POST in front of it, rather than answering 405 on a route that has a page.
func TestAPageAnswersWhatItsEndpointCannot(t *testing.T) {
	h := newEndpoints(t, endpointFixture(nil, nil, []string{"POST"}),
		NewEndpoint("/dual", "POST", echo("endpoint")),
	)

	resp := request(t, h, http.MethodGet, "/dual", http.Header{"Accept": {"*/*"}})
	if got := body(t, resp); got != "page" {
		t.Errorf("GET on a POST-only endpoint answered %q, want the page", got)
	}

	// A method no page can answer still reaches the endpoint, and still 405s
	// there rather than falling through to a page that cannot serve it either:
	// kit's "prefer the page" override covers GET, HEAD and POST and nothing
	// else.
	resp = request(t, h, http.MethodDelete, "/dual", http.Header{"Origin": {"https://example.test"}})
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("DELETE on a dual route: status = %d, want 405", resp.StatusCode)
	}
}

// A page route with no endpoint is untouched, and gets no Vary: nothing about
// it depends on Accept.
func TestAPageRouteIsPassedStraightThrough(t *testing.T) {
	h := newEndpoints(t, endpointFixture(nil, nil, nil))

	resp := request(t, h, http.MethodGet, "/plain", nil)
	if got := body(t, resp); got != "page" {
		t.Errorf("body = %q, want the page", got)
	}
	if got := resp.Header.Get("Vary"); got != "" {
		t.Errorf("Vary = %q on a route with no endpoint", got)
	}
}

// Kit normalizes the trailing slash of every route it matches, before it decides
// whether a page or an endpoint answers, and the redirect is a 308 with a
// relative location so a path prefix the server cannot see survives it.
func TestTheTrailingSlashIsNormalizedBeforeAnythingAnswers(t *testing.T) {
	h := newEndpoints(t, endpointFixture(nil, []string{"GET"}, nil),
		NewEndpoint("/api/thing", "GET", echo("thing")),
	)

	for _, target := range []string{"/api/thing/", "/plain/"} {
		resp := request(t, h, http.MethodGet, target, nil)
		if resp.StatusCode != http.StatusPermanentRedirect {
			t.Errorf("%s: status = %d, want 308", target, resp.StatusCode)
		}
		if got := resp.Header.Get("Location"); !strings.HasPrefix(got, "../") {
			t.Errorf("%s: Location = %q, want a relative location", target, got)
		}
	}

	// The redirect precedes method resolution, so a method the route does not
	// declare is redirected rather than refused — 308 preserves the method, so
	// the 405 arrives on the next request.
	resp := request(t, h, http.MethodDelete, "/api/thing/", http.Header{"Origin": {"https://example.test"}})
	if resp.StatusCode != http.StatusPermanentRedirect {
		t.Errorf("DELETE on the wrong slash: status = %d, want 308", resp.StatusCode)
	}

	// The query survives, and `/` itself is never redirected.
	resp = request(t, h, http.MethodGet, "/plain/?q=1", nil)
	if got := resp.Header.Get("Location"); got != "../plain?q=1" {
		t.Errorf("Location = %q, want ../plain?q=1", got)
	}
}

// Kit refuses a form-shaped mutation from another origin before the handler
// runs (`runtime/server/csrf.js`), and it applies that check to endpoints, not
// only to form actions.
func TestACrossSiteFormPostIsRefused(t *testing.T) {
	h := newEndpoints(t, endpointFixture(nil, []string{"POST"}, nil),
		NewEndpoint("/api/thing", "POST", echo("thing")),
	)

	cases := []struct {
		name        string
		origin      string
		contentType string
		want        int
	}{
		{"no origin, form body", "", "application/x-www-form-urlencoded", http.StatusForbidden},
		{"other origin, form body", "https://evil.test", "multipart/form-data", http.StatusForbidden},
		{"other origin, no content type", "https://evil.test", "", http.StatusForbidden},
		{"own origin, form body", "https://example.test", "application/x-www-form-urlencoded", http.StatusOK},
		// JSON is not a content type a cross-origin <form> can send, so kit
		// leaves it to the endpoint. An API that accepts JSON has CORS to lean
		// on; one that accepts a form does not.
		{"other origin, json body", "https://evil.test", "application/json", http.StatusOK},
	}
	for _, tc := range cases {
		header := http.Header{}
		if tc.origin != "" {
			header.Set("Origin", tc.origin)
		}
		if tc.contentType != "" {
			header.Set("Content-Type", tc.contentType)
		}
		resp := request(t, h, http.MethodPost, "/api/thing", header)
		if resp.StatusCode != tc.want {
			t.Errorf("%s: status = %d, want %d", tc.name, resp.StatusCode, tc.want)
		}
	}

	// The refusal comes before the route is matched at all, so a cross-site
	// mutation never learns which methods the route answers. Refusing it after
	// method resolution would answer this with a 405 and an `Allow` header.
	resp := request(t, h, http.MethodDelete, "/api/thing", nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("cross-site DELETE: status = %d, want 403", resp.StatusCode)
	}
	if got := resp.Header.Get("Allow"); got != "" {
		t.Errorf("a refused cross-site request was told Allow: %q", got)
	}
}

// The event a server route reaches through its context is the route's, not the
// request path's: the id kit matched and the parameters kit captured.
func TestAnEndpointReachesItsRouteThroughTheEvent(t *testing.T) {
	cfg := EndpointConfig{
		AppDir: "_app",
		Routes: []ManifestRoute{{
			ID:      "/items/[id]",
			Pattern: `^\/items\/([^/]+?)\/?$`,
			Params:  []ManifestParam{{Name: "id"}},
			Endpoint: &ManifestEndpoint{
				Methods: []string{"GET"},
			},
		}},
		manifest: true,
	}

	h := newEndpoints(t, cfg, NewEndpoint("/items/[id]", "GET", func(w http.ResponseWriter, r *http.Request) {
		e := EventFrom(r.Context())
		if e == nil {
			t.Error("the handler got no event")
			return
		}
		_, _ = w.Write([]byte(e.RouteID() + " " + e.Param("id")))
	}))

	if got := body(t, request(t, h, http.MethodGet, "/items/42", nil)); got != "/items/[id] 42" {
		t.Errorf("the handler saw %q", got)
	}
}

// A server route writes its own response, so the event's setters refuse rather
// than accept something nothing would ever apply.
func TestAnEndpointsEventRefusesToWrite(t *testing.T) {
	cfg := endpointFixture(nil, []string{"GET"}, nil)
	h := newEndpoints(t, cfg, NewEndpoint("/api/thing", "GET", func(w http.ResponseWriter, r *http.Request) {
		e := EventFrom(r.Context())
		if err := e.SetCookie("a", "b", CookieOptions{}); err == nil {
			t.Error("SetCookie was accepted from a server route")
		}
		if err := e.SetHeader("x-thing", "1"); err == nil {
			t.Error("SetHeader was accepted from a server route")
		}
		// The ordinary Go way still works, and is the only way.
		http.SetCookie(w, &http.Cookie{Name: "a", Value: "b"})
	}))

	resp := request(t, h, http.MethodGet, "/api/thing", nil)
	if got := resp.Header.Get("Set-Cookie"); !strings.HasPrefix(got, "a=b") {
		t.Errorf("Set-Cookie = %q; http.SetCookie must reach the response", got)
	}
}

// A handler will panic one day. On its own goroutine that is a dead process,
// taking every other visitor with it; kit answers an unexpected throw with an
// opaque 500 and so does this.
func TestAPanickingEndpointBecomesA500(t *testing.T) {
	var reported string
	cfg := endpointFixture(nil, []string{"GET"}, nil)
	cfg.OnPanic = func(routeID, method string, value any, stack []byte) {
		reported = method + " " + routeID
	}

	h := newEndpoints(t, cfg, NewEndpoint("/api/thing", "GET", func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))

	resp := request(t, h, http.MethodGet, "/api/thing", nil)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
	if reported != "GET /api/thing" {
		t.Errorf("OnPanic was told %q", reported)
	}
	if got := body(t, resp); strings.Contains(got, "boom") {
		t.Errorf("the panic reached the client: %q", got)
	}
}

// The registry refuses a frontend it cannot serve, for the same reason the
// remote and load registries do: a route kit compiled a `+server.ts` for and Go
// does not answer is a route that answers nothing.
func TestTheRegistryRefusesAFrontendItCannotServe(t *testing.T) {
	cfg := endpointFixture(nil, []string{"GET", "POST"}, nil)

	if _, err := NewEndpoints(cfg, NewEndpoint("/api/thing", "GET", echo("thing"))); err == nil {
		t.Error("a binary that answers only GET accepted a frontend declaring GET and POST")
	} else if !strings.Contains(err.Error(), "POST /api/thing") {
		t.Errorf("the error does not name the missing method: %v", err)
	}

	extra := []*Endpoint{
		NewEndpoint("/api/thing", "GET", echo("thing")),
		NewEndpoint("/api/thing", "POST", echo("thing")),
		NewEndpoint("/plain", "GET", echo("plain")),
	}
	if _, err := NewEndpoints(cfg, extra...); err == nil {
		t.Error("a binary answering a route the frontend has no +server.ts for was accepted")
	} else if !strings.Contains(err.Error(), "GET /plain") {
		t.Errorf("the error does not name the extra route: %v", err)
	}

	// Dev is the one place the two halves are allowed to disagree, because the
	// frontend is vite's rather than this build's.
	dev := cfg
	dev.Dev = true
	if _, err := NewEndpoints(dev, NewEndpoint("/api/thing", "GET", echo("thing"))); err != nil {
		t.Errorf("dev refused a partial registry: %v", err)
	}
}

func TestNegotiateFollowsKitsPreference(t *testing.T) {
	cases := []struct {
		accept string
		want   string
	}{
		{"*/*", "*"},
		{"text/html", "text/html"},
		{"text/html;q=0.1,*/*;q=0.9", "*"},
		{"text/*", "text/html"},
		{"application/json", ""},
		// A concrete subtype outranks a wildcard at the same q, which is why a
		// browser's Accept resolves to text/html.
		{"text/html,*/*", "text/html"},
		{"not a media type", ""},
	}
	for _, tc := range cases {
		if got := negotiate(tc.accept, "*", "text/html"); got != tc.want {
			t.Errorf("negotiate(%q) = %q, want %q", tc.accept, got, tc.want)
		}
	}
}
