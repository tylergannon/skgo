package skgo

import (
	"context"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

// Every expectation here is a literal written from Kit's own behaviour
// (runtime/server/cookie.js and fetch.js, kit 3.0.0-next.28), never read from
// what the code under test produced.

const clientAddr = "192.0.2.1:1234" // what httptest.NewRequest gives every request

func setCookies(rec interface{ Result() *http.Response }) []string {
	return rec.Result().Header.Values("Set-Cookie")
}

// relayTo builds a relay request: each destination in order, the last one's
// answer is the response, and what the cookies did on the way is observable.
func relayTo(extra string, destinations ...string) string {
	var b strings.Builder
	b.WriteString("/api/relay?")
	for i, d := range destinations {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString("to=" + url.QueryEscape(d))
	}
	return b.String() + extra
}

func setting(cookies ...string) string {
	q := url.Values{}
	for _, c := range cookies {
		q.Add("set", c)
	}
	return "/api/cookies?" + q.Encode()
}

func TestReturnedCookiesAreWrittenBackRawAndSentOnWithKitsPathDefault(t *testing.T) {
	app := newFetchApp(t, fetchAppOptions{})

	for _, tc := range []struct {
		name, destination, returned string
		want                        []string
	}{
		{"raw value is kept, not re-encoded", "/api/cookies", "raw=a%20b+c==; Path=/", []string{"raw=a%20b+c==; Path=/"}},
		{"no Path: the directory of the subrequest URL", "/api/cookies", "p=1", []string{"p=1; Path=/api"}},
		{"no Path: one level down", "/a/cookies", "p=1", []string{"p=1; Path=/a"}},
		{"no Path: a top-level URL gives /", "/cookies", "p=1", []string{"p=1; Path=/"}},
	} {
		destination := strings.Replace(tc.destination, "cookies", "cookies?"+url.Values{"set": {tc.returned}}.Encode(), 1)
		rec := app.serve("GET", relayTo("", destination), outerHeaders)
		if got := setCookies(rec); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: the visitor's Set-Cookie = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestACookieReturnedInProcessIsSentOnTheNextFetchOfThatRequest(t *testing.T) {
	app := newFetchApp(t, fetchAppOptions{})
	returned := setting("raw=a%20b+c==; Path=/api")

	rec := app.serve("GET", relayTo("", returned, "/api/echo"), outerHeaders)
	if got := decodeSeen(t, rec.Body.String()).Cookie; got != "session=abc123; theme=light; raw=a%20b+c==" {
		t.Errorf("the next fetch carried %q; Kit sends the page's cookies then the returned one, unencoded", got)
	}
	rec = app.serve("GET", relayTo("", returned, "/a/echo"), outerHeaders)
	if got := decodeSeen(t, rec.Body.String()).Cookie; got != "session=abc123; theme=light" {
		t.Errorf("a path the cookie does not cover carried %q", got)
	}
}

func TestSameNameCookiesDifferingByDomainOrPathAreDistinctAndTheLatestMatchingWins(t *testing.T) {
	app := newFetchApp(t, fetchAppOptions{})
	returned := setting("n=api; Path=/api", "n=root; Path=/", "n=dom; Domain=app.test; Path=/api")

	rec := app.serve("GET", relayTo("", returned, "/api/echo"), outerHeaders)
	want := []string{"n=api; Path=/api", "n=root; Path=/", "n=dom; Domain=app.test; Path=/api"}
	if got := setCookies(rec); !reflect.DeepEqual(got, want) {
		t.Errorf("Set-Cookie = %q, want one line per domain/path/name %q", got, want)
	}
	if got := decodeSeen(t, rec.Body.String()).Cookie; got != "session=abc123; theme=light; n=dom" {
		t.Errorf("/api/echo carried %q, want the last matching n", got)
	}
	rec = app.serve("GET", relayTo("", returned, "/a/echo"), outerHeaders)
	if got := decodeSeen(t, rec.Body.String()).Cookie; got != "session=abc123; theme=light; n=root" {
		t.Errorf("/a/echo carried %q, want only the cookie whose path covers it", got)
	}

	// The same domain, path and name replaces the earlier cookie where it stood.
	rec = app.serve("GET", relayTo("", setting("n=1; Path=/api", "other=x; Path=/api", "n=2; Path=/api")), outerHeaders)
	if got, want := setCookies(rec), []string{"n=2; Path=/api", "other=x; Path=/api"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Set-Cookie = %q, want %q", got, want)
	}
}

func TestReturnedDeletionAndExpiryAreWrittenBackAndTracked(t *testing.T) {
	app := newFetchApp(t, fetchAppOptions{})

	rec := app.serve("GET", relayTo("", setting("session=; Max-Age=0; Path=/"), "/api/echo"), outerHeaders)
	if got, want := setCookies(rec), []string{"session=; Max-Age=0; Path=/"}; !reflect.DeepEqual(got, want) {
		t.Errorf("deletion Set-Cookie = %q, want %q", got, want)
	}
	// Kit keeps the tombstone and sends it as an empty value.
	if got := decodeSeen(t, rec.Body.String()).Cookie; got != "session=; theme=light" {
		t.Errorf("after the deletion the next fetch carried %q, want %q", got, "session=; theme=light")
	}

	rec = app.serve("GET", relayTo("", setting("old=1; Expires=Thu, 01 Jan 1970 00:00:00 GMT; Path=/"), "/api/echo"), outerHeaders)
	if got, want := setCookies(rec), []string{"old=1; Path=/; Expires=Thu, 01 Jan 1970 00:00:00 GMT"}; !reflect.DeepEqual(got, want) {
		t.Errorf("expiry Set-Cookie = %q, want %q", got, want)
	}
}

func TestReturnedSecurityAttributesSurviveInKitsOrder(t *testing.T) {
	app := newFetchApp(t, fetchAppOptions{})
	returned := setting("s=1; SameSite=Strict; Secure; HttpOnly; Path=/api; Domain=app.test; Max-Age=60; Priority=High; Partitioned")

	rec := app.serve("GET", relayTo("", returned), nil)
	want := []string{"s=1; Max-Age=60; Domain=app.test; Path=/api; HttpOnly; Secure; Partitioned; Priority=High; SameSite=Strict"}
	if got := setCookies(rec); !reflect.DeepEqual(got, want) {
		t.Errorf("Set-Cookie = %q, want %q", got, want)
	}
}

func TestCookiePrecedenceIsPageThenReturnedThenExplicitHeader(t *testing.T) {
	app := newFetchApp(t, fetchAppOptions{})
	returned := setting("session=fresh; Path=/")

	rec := app.serve("GET", relayTo("", returned, "/api/echo"), outerHeaders)
	if got := decodeSeen(t, rec.Body.String()).Cookie; got != "session=fresh; theme=light" {
		t.Errorf("returned cookie vs the page's: carried %q", got)
	}
	rec = app.serve("GET", relayTo("&h.Cookie=session%3Dexplicit", returned, "/api/echo"), outerHeaders)
	if got := decodeSeen(t, rec.Body.String()).Cookie; got != "session=explicit; theme=light" {
		t.Errorf("explicit header vs returned: carried %q", got)
	}
}

func TestReturnedCookiesAreCollectedWhateverTheCredentialsMode(t *testing.T) {
	app := newFetchApp(t, fetchAppOptions{})

	rec := app.serve("GET", relayTo("&omit=1", setting("kept=1; Path=/"), "/api/echo"), outerHeaders)
	if got, want := setCookies(rec), []string{"kept=1; Path=/"}; !reflect.DeepEqual(got, want) {
		t.Errorf("credentials: omit still collects what comes back: Set-Cookie = %q, want %q", got, want)
	}
	if got := decodeSeen(t, rec.Body.String()).Cookie; got != "" {
		t.Errorf("credentials: omit sent %q", got)
	}
}

func TestCookiesDoNotLeakAcrossExternalHookPrerenderedAndFailedBoundaries(t *testing.T) {
	hook := func(app *fetchApp) HandleFetch {
		return func(ctx context.Context, r *http.Request, next Fetch) (*http.Response, error) {
			if r.URL.Path == "/api/short-circuit" {
				h := http.Header{"Set-Cookie": {"hook=leak; Path=/"}, "Content-Type": {"text/plain"}}
				return &http.Response{StatusCode: 200, Header: h, Body: http.NoBody, Request: r}, nil
			}
			return next(r)
		}
	}
	app := newFetchApp(t, fetchAppOptions{hook: hook})

	for _, tc := range []struct{ name, first string }{
		{"an external answer", "http://svc.other.test/ext/text"},
		{"a HandleFetch short-circuit", "/api/short-circuit"},
		{"a prerendered file", "/pre/static"},
	} {
		rec := app.serve("GET", relayTo("", tc.first, "/api/echo"), outerHeaders)
		if got := setCookies(rec); len(got) != 0 {
			t.Errorf("%s: Set-Cookie %q reached the visitor", tc.name, got)
		}
		if got := decodeSeen(t, rec.Body.String()).Cookie; got != "session=abc123; theme=light" {
			t.Errorf("%s: the next in-app fetch carried %q, so the cookie leaked", tc.name, got)
		}
	}
}

func TestACookieReturnedToALoadsFetchReachesTheDataResponseAndHandle(t *testing.T) {
	app := newFetchApp(t, fetchAppOptions{})

	app.loadFetching(setting("fromload=1; Path=/"))
	rec := app.serve("GET", "/a/b/__data.json?x-sveltekit-invalidated=01", outerHeaders)
	if got, want := setCookies(rec), []string{"fromload=1; Path=/"}; !reflect.DeepEqual(got, want) {
		t.Errorf("__data.json Set-Cookie = %q, want %q", got, want)
	}

	h := http.Header{"X-Middleware-Cookie": {setting("fromhandle=1; Path=/")}}
	rec = app.serve("GET", "/api/echo", h)
	if got, want := setCookies(rec), []string{"fromhandle=1; Path=/"}; !reflect.DeepEqual(got, want) {
		t.Errorf("a Handle hook's fetch: Set-Cookie = %q, want %q", got, want)
	}
}

func TestEveryKindOfApplicationDestinationAnswersInProcess(t *testing.T) {
	app := newFetchApp(t, fetchAppOptions{})
	app.loadFetch = func(context.Context, *Event) (loaded, error) {
		return loaded{Status: 201, Body: "inner-load-literal"}, nil
	}

	for _, tc := range []struct {
		name, target string
		status       int
		contentType  string
		contains     string
	}{
		{"a page", "/a/b", 200, "text/html; charset=utf-8", "page"},
		{"a data route", "/a/b/__data.json?x-sveltekit-invalidated=01", 200, "application/json", "inner-load-literal"},
		{"a public asset", "/robots.txt", 200, "text/plain; charset=utf-8", "User-agent: *\nDisallow: /private\n"},
		{"a prerendered file", "/pre/static", 200, "text/html; charset=utf-8", "prerendered:static"},
		{"an endpoint", "/api/echo?x=1", 200, "application/json", `"query":"x=1"`},
	} {
		rec := app.serve("GET", relayTo("", tc.target), outerHeaders)
		if rec.Code != tc.status {
			t.Errorf("%s: status %d (%s)", tc.name, rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("Content-Type"); got != tc.contentType {
			t.Errorf("%s: Content-Type %q, want %q", tc.name, got, tc.contentType)
		}
		if !strings.Contains(rec.Body.String(), tc.contains) {
			t.Errorf("%s: body %q does not contain %q", tc.name, rec.Body.String(), tc.contains)
		}
	}
	if app.external.count() != 0 {
		t.Error("an application destination went out over HTTP")
	}
}

func TestAPrerenderedFileWinsOverAMatchingDynamicRouteForFetchesAndRequests(t *testing.T) {
	for _, base := range []string{"", "/app"} {
		app := newFetchApp(t, fetchAppOptions{base: base})

		for _, tc := range []struct {
			target, body string
		}{
			{"/pre/hello", "prerendered:hello"}, // also matches /pre/[slug]
			{"/pre/other", "dynamic-endpoint"},  // only the dynamic route does
		} {
			rec := app.serve("GET", base+tc.target, nil)
			if got := rec.Body.String(); got != tc.body {
				t.Errorf("base %q: GET %s = %q, want %q", base, tc.target, got, tc.body)
			}
			rec = app.serve("GET", base+relayTo("", base+tc.target), outerHeaders)
			if got := rec.Body.String(); got != tc.body {
				t.Errorf("base %q: fetch of %s = %q, want %q", base, tc.target, got, tc.body)
			}
		}
	}
}

func TestAPrerenderedFetchCarriesNothingOfThePageAndKeepsTheClientAddress(t *testing.T) {
	app := newFetchApp(t, fetchAppOptions{})

	rec := app.serve("GET", relayTo("", "/pre/static"), outerHeaders)
	for header, want := range map[string]string{
		"X-Saw-Cookie": "", "X-Saw-Auth": "", "X-Saw-Accept": "", "X-Static": "prerendered", "X-Remote": clientAddr,
	} {
		if got := rec.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q: Kit answers a prerendered path with a bare request", header, got, want)
		}
	}

	rec = app.serve("GET", relayTo("", "/api/echo"), outerHeaders)
	if got := rec.Header().Get("X-Remote"); got != clientAddr {
		t.Errorf("an endpoint's subrequest saw client address %q, want %q (Kit keeps getClientAddress)", got, clientAddr)
	}
}
