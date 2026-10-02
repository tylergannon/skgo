package example_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/tylergannon/skgo/internal/kithash"
)

// The /middleware pages are authenticated by the app's Go middleware
// (example.VisitMiddleware), which wraps `handle`: it refreshes a visit
// cookie before anything answers and marks the response it gets back. Every
// expectation is a literal written here.

func TestMiddlewareAuthenticatesAColdDocumentAndRefreshesTheStaleCookie(t *testing.T) {
	h := newProdHandler(t)
	rec := actionRequest(h, http.MethodGet, "/middleware", "", http.Header{"Accept": {"text/html"}},
		&http.Cookie{Name: "skgo_visit", Value: "stale-token"})
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	requireMarkup(t, "/middleware", rec.Body.String(),
		tagged("p", "mw-token", "visit-token-7"),
		tagged("p", "mw-route", "/middleware"),
		tagged("p", "mw-cookie", "visit-token-7"))
	if got := rec.Header().Get("X-Skgo-Middleware"); got != "/middleware data=false" {
		t.Errorf("X-Skgo-Middleware = %q", got)
	}
	visit := rec.Result().Cookies()
	if len(visit) != 1 || visit[0].Name != "skgo_visit" || visit[0].Value != "visit-token-7" {
		t.Errorf("cookies = %v, want exactly skgo_visit=visit-token-7", visit)
	}
}

func TestMiddlewareKeepsAValidVisitCookieAndSendsNoSecondOne(t *testing.T) {
	h := newProdHandler(t)
	rec := actionRequest(h, http.MethodGet, "/middleware", "", http.Header{"Accept": {"text/html"}},
		&http.Cookie{Name: "skgo_visit", Value: "visit-from-yesterday"})
	requireMarkup(t, "/middleware", rec.Body.String(), tagged("p", "mw-token", "visit-from-yesterday"))
	if c := rec.Result().Cookies(); len(c) != 0 {
		t.Errorf("a valid cookie was reissued: %v", c)
	}
}

func TestMiddlewareSeesTheMatchedRouteOnAClientNavigationsDataRequest(t *testing.T) {
	h := newProdHandler(t)
	rec := actionRequest(h, http.MethodGet, "/middleware/alpha/__data.json?x-sveltekit-invalidated=011", "", nil)
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-Skgo-Middleware"); got != "/middleware/[slug] data=true" {
		t.Errorf("X-Skgo-Middleware = %q", got)
	}
	// The load answered with what the middleware observed: the route's param
	// from the event, the fresh token it issued, and that this was a data request.
	for _, want := range []string{`"alpha"`, `"visit-token-7"`, `"/middleware/[slug]"`, "true"} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("data body lacks %s: %s", want, rec.Body.String())
		}
	}
}

func TestMiddlewareReachesPageActionsNativeAndEnhanced(t *testing.T) {
	h := newProdHandler(t)
	form := url.Values{"note": {"remember-this"}}.Encode()
	post := func(accept string, extra http.Header) *httptest.ResponseRecorder {
		header := http.Header{"Accept": {accept}, "Origin": {prodOrigin}, "Content-Type": {"application/x-www-form-urlencoded"}}
		for k, v := range extra {
			header[k] = v
		}
		return actionRequest(h, http.MethodPost, "/middleware", form, header, &http.Cookie{Name: "skgo_visit", Value: "visit-form-3"})
	}

	rec := post("text/html", nil)
	if rec.Code != 200 {
		t.Fatalf("native: %d", rec.Code)
	}
	requireMarkup(t, "native action", rec.Body.String(), tagged("h2", "mw-receipt", "Noted remember-this for visit-form-3"))
	if got := rec.Header().Get("X-Skgo-Middleware"); got != "/middleware data=false" {
		t.Errorf("native X-Skgo-Middleware = %q", got)
	}

	rec = post("application/json", http.Header{"X-Sveltekit-Action": {"true"}})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"type":"success"`) || !strings.Contains(rec.Body.String(), "Noted remember-this for visit-form-3") {
		t.Errorf("enhanced: %d %s", rec.Code, rec.Body.String())
	}
}

func TestMiddlewareTransformsOnlyTheMiddlewarePagesDocumentsAndNothingElse(t *testing.T) {
	h := newProdHandler(t)
	const marked = `<html lang="en" data-middleware-transformed="yes">`
	html := http.Header{"Accept": {"text/html"}}

	rec := actionRequest(h, http.MethodGet, "/middleware", "", html)
	if !strings.Contains(rec.Body.String(), marked) {
		t.Errorf("the /middleware document was not transformed:\n%.400s", rec.Body.String())
	}
	// The document carries a fresh CSP nonce per render, so its ETag is not
	// repeatable; what it must be is the hash of the transformed bytes sent.
	if got, want := rec.Header().Get("ETag"), `"`+kithash.Kit(rec.Body.String())+`"`; got != want {
		t.Errorf("ETag = %s, want the hash of the transformed document %s", got, want)
	}

	for _, target := range []string{"/", "/about"} {
		if got := actionRequest(h, http.MethodGet, target, "", html).Body.String(); strings.Contains(got, "data-middleware-transformed") {
			t.Errorf("%s was transformed, but only /middleware chose to", target)
		}
	}
	data := actionRequest(h, http.MethodGet, "/middleware/alpha/__data.json?x-sveltekit-invalidated=011", "", nil)
	if strings.Contains(data.Body.String(), "data-middleware-transformed") {
		t.Errorf("a data response was transformed: %s", data.Body.String())
	}

	form := url.Values{"note": {"remember-this"}}.Encode()
	native := actionRequest(h, http.MethodPost, "/middleware", form, http.Header{
		"Accept": {"text/html"}, "Origin": {prodOrigin}, "Content-Type": {"application/x-www-form-urlencoded"},
	}, &http.Cookie{Name: "skgo_visit", Value: "visit-form-3"})
	if !strings.Contains(native.Body.String(), marked) || !strings.Contains(native.Body.String(), "Noted remember-this for visit-form-3") {
		t.Errorf("the scripting-disabled form's result page: %d\n%.600s", native.Code, native.Body.String())
	}
}
