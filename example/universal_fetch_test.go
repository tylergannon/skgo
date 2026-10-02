package example_test

import (
	"encoding/json"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// The /universal-fetch page's universal load fetches every shape of answer
// the replay API has. Every expectation here is a literal this file states:
// base64 of the bytes the endpoint writes, the hash Kit's own hash.js gives
// for a request's headers and body, the exact text Kit's own serializer
// escapes. Nothing is read back from the page that produced it.

var fetchedTag = regexp.MustCompile(`<script type="application/json" data-sveltekit-fetched[^>]*>[^<]*</script>`)

// fetchedTags returns the replay scripts of a document keyed by their data-url.
func fetchedTags(t *testing.T, document string) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	for _, tag := range fetchedTag.FindAllString(document, -1) {
		m := regexp.MustCompile(`data-url="([^"]*)"`).FindStringSubmatch(tag)
		if m == nil {
			t.Fatalf("a replay script with no data-url: %s", tag)
		}
		out[html.UnescapeString(m[1])] = append(out[html.UnescapeString(m[1])], tag)
	}
	return out
}

func document(t *testing.T, h http.Handler, path string) string {
	t.Helper()
	rec := getAs(t, h, path, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: status = %d: %s", path, rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

func replayCounts(t *testing.T, h http.Handler, run string) map[string]int {
	t.Helper()
	rec := getAs(t, h, "/api/replay-count?run="+run, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("the counting endpoint answered %d: %s", rec.Code, rec.Body.String())
	}
	counts := map[string]int{}
	if err := json.Unmarshal(rec.Body.Bytes(), &counts); err != nil {
		t.Fatalf("decoding the counts: %v", err)
	}
	return counts
}

func TestAColdDocumentReplaysEveryAnswerTheWayKitSerializesIt(t *testing.T) {
	h := newProdHandler(t)
	body := document(t, h, "/universal-fetch?run=cold&step=1")
	tags := fetchedTags(t, body)

	const head = `<script type="application/json" data-sveltekit-fetched data-url=`
	const hook = `"headers":{"x-fetch-hook":"Go"}`
	want := map[string][]string{
		"/api/replay/text?run=cold&step=1": {
			head + `"/api/replay/text?run=cold&amp;step=1">{"status":200,"statusText":"OK",` + hook + `,"body":"plain-lantern-7"}</script>`,
		},
		"/api/replay/json?run=cold&step=1": {
			head + `"/api/replay/json?run=cold&amp;step=1">{"status":200,"statusText":"OK",` + hook + `,"body":"{\"lamp\":\"harbour\",\"step\":\"1\"}\n"}</script>`,
		},
		// A request the hook rewrote is keyed by the URL the load asked for,
		// and carries the answer to the one Go actually served.
		"/api/replay/json?run=cold&alias=1": {
			head + `"/api/replay/json?run=cold&amp;alias=1">{"status":200,"statusText":"OK",` + hook + `,"body":"{\"lamp\":\"harbour\",\"step\":\"rewritten\"}\n"}</script>`,
		},
		// Bytes that are not UTF-8 travel as base64 (data-b64), with only the
		// headers the app's filter lets through: x-replay-secret is not here.
		"/api/replay/binary?run=cold&step=1": {
			head + `"/api/replay/binary?run=cold&amp;step=1" data-b64>{"status":200,"statusText":"OK","headers":{"x-fetch-hook":"Go","x-replay-allowed":"shown-7"},"body":"AP8QgH/DKA=="}</script>`,
		},
		// A body the load read through the stream: "alpha-beta-gamma".
		"/api/replay/stream?run=cold&step=1": {
			head + `"/api/replay/stream?run=cold&amp;step=1" data-b64>{"status":200,"statusText":"OK",` + hook + `,"body":"YWxwaGEtYmV0YS1nYW1tYQ=="}</script>`,
		},
		// Script-breaking text: < is escaped by Kit's serializer, and so are
		// U+2028 and U+2029, which a JavaScript string literal may not hold.
		"/api/replay/breaking?run=cold&step=1": {
			head + `"/api/replay/breaking?run=cold&amp;step=1">{"status":200,"statusText":"OK",` + hook + `,"body":"\u003C/script>\u003C!-- \u2028 \u2029 \"q\" & ok"}</script>`,
		},
		// A 204 has no body at all: no "body" key.
		"/api/replay/empty?run=cold&step=1": {
			head + `"/api/replay/empty?run=cold&amp;step=1">{"status":204,"statusText":"No Content","headers":{"x-fetch-hook":"Go","x-replay-allowed":"shown-7"}}</script>`,
		},
		"/api/replay/missing?run=cold&step=1": {
			head + `"/api/replay/missing?run=cold&amp;step=1">{"status":404,"statusText":"Not Found",` + hook + `,"body":"no such lamp"}</script>`,
		},
		// Four requests to one URL, told apart by what the load sent: the
		// hashes are the ones Kit's hash.js gives for
		// {"x-replay":"a"}, {"x-replay":"b"}, body "alpha" and body "beta".
		"/api/replay/echo?run=cold&step=1": {
			head + `"/api/replay/echo?run=cold&amp;step=1" data-hash="1u97imm">{"status":200,"statusText":"OK",` + hook + `,"body":"{\"body\":\"\",\"contentType\":\"\",\"header\":\"a\",\"method\":\"GET\",\"step\":\"1\"}\n"}</script>`,
			head + `"/api/replay/echo?run=cold&amp;step=1" data-hash="10wn4dp">{"status":200,"statusText":"OK",` + hook + `,"body":"{\"body\":\"\",\"contentType\":\"\",\"header\":\"b\",\"method\":\"GET\",\"step\":\"1\"}\n"}</script>`,
			head + `"/api/replay/echo?run=cold&amp;step=1" data-hash="2t9x75">{"status":200,"statusText":"OK",` + hook + `,"body":"{\"body\":\"alpha\",\"contentType\":\"text/plain;charset=UTF-8\",\"header\":\"\",\"method\":\"POST\",\"step\":\"1\"}\n"}</script>`,
			head + `"/api/replay/echo?run=cold&amp;step=1" data-hash="yivt07">{"status":200,"statusText":"OK",` + hook + `,"body":"{\"body\":\"beta\",\"contentType\":\"text/plain;charset=UTF-8\",\"header\":\"\",\"method\":\"POST\",\"step\":\"1\"}\n"}</script>`,
		},
		// A response the browser may keep says so: max-age=60 is data-ttl.
		"/api/replay/cached?run=cold": {
			head + `"/api/replay/cached?run=cold" data-ttl="60">{"status":200,"statusText":"OK",` + hook + `,"body":"{\"lamp\":\"cached\"}"}</script>`,
		},
	}
	for key, wantTags := range want {
		got := tags[key]
		if len(got) != len(wantTags) {
			t.Errorf("%s: %d replay scripts, want %d", key, len(got), len(wantTags))
			continue
		}
		for i := range wantTags {
			if got[i] != wantTags[i] {
				t.Errorf("%s [%d]:\n got  %s\n want %s", key, i, got[i], wantTags[i])
			}
		}
	}

	// A URLSearchParams body is not one Kit can hash, so it is not
	// serialized: the browser repeats the request. Nothing is invented for it.
	if got := tags["/api/replay/form?run=cold&step=1"]; len(got) != 0 {
		t.Errorf("a body that cannot be replayed was serialized: %v", got)
	}
	// And what the filter withholds is nowhere in the document.
	for _, secret := range []string{"hidden-value-31", "x-replay-secret"} {
		if strings.Contains(body, secret) {
			t.Errorf("the document contains %q", secret)
		}
	}
	// The nested rendered page is a fetch too, replayed whole.
	nested := tags["/request-fetch?run=cold"]
	if len(nested) != 1 || !strings.Contains(nested[0], `harbour-lamp-4096`) || !strings.Contains(nested[0], `\u003C!doctype html>`) {
		t.Errorf("the rendered page fetched by the load is not replayed (%d scripts)", len(nested))
	}

	// The page shows what the load read, from the same answers.
	for _, wantText := range []string{
		`data-testid="uf-text">plain-lantern-7<`,
		`data-testid="uf-json">harbour@1<`,
		`data-testid="uf-aliased">harbour@rewritten<`,
		`data-testid="uf-binary">00ff10807fc328<`,
		`data-testid="uf-shown">shown-7<`,
		`data-testid="uf-streamed">alpha-beta-gamma<`,
		`data-testid="uf-empty">204:none<`,
		`data-testid="uf-missing">404:false:no such lamp<`,
		`data-testid="uf-header-a">a@1<`,
		`data-testid="uf-header-b">b@1<`,
		`data-testid="uf-body-alpha">POST:alpha@1<`,
		`data-testid="uf-body-beta">POST:beta@1<`,
		`data-testid="uf-form">POST:lamp=one:application/x-www-form-urlencoded;charset=UTF-8<`,
		`data-testid="uf-cached">cached<`,
		`data-testid="uf-nested">harbour-lamp-4096<`,
	} {
		if !strings.Contains(body, wantText) {
			t.Errorf("the document does not contain %s", wantText)
		}
	}

	// Every endpoint ran exactly as often as the load asked of it: the
	// document itself, each request once, the echo's four, the rendered page.
	wantCounts := map[string]int{
		"GET /universal-fetch":     1,
		"GET /api/replay/text":     1,
		"GET /api/replay/json":     2,
		"GET /api/replay/binary":   1,
		"GET /api/replay/stream":   1,
		"GET /api/replay/breaking": 1,
		"GET /api/replay/empty":    1,
		"GET /api/replay/missing":  1,
		"GET /api/replay/echo":     2,
		"POST /api/replay/echo":    2,
		"POST /api/replay/form":    1,
		"GET /api/replay/cached":   1,
		"GET /request-fetch":       1,
	}
	got := replayCounts(t, h, "cold")
	for key, n := range wantCounts {
		if got[key] != n {
			t.Errorf("%s ran %d times, want %d", key, got[key], n)
		}
	}
	for key := range got {
		if _, ok := wantCounts[key]; !ok {
			t.Errorf("%s ran %d times and was not expected", key, got[key])
		}
	}
}

func TestAHookRewrittenRequestIsReplayedUnderItsOriginalIdentity(t *testing.T) {
	h := newProdHandler(t)
	tags := fetchedTags(t, document(t, h, "/fetch"))
	// The load asked for ?via=universal with two headers; the hook sent Go
	// the same request without via. The replay is keyed by what was asked,
	// and its hash is Kit's for {x-demo, accept} (see hash.js).
	got := tags["/api/todos?via=universal&refresh=0"]
	if len(got) != 1 || !strings.Contains(got[0], `data-hash="k1f7fj"`) || !strings.Contains(got[0], `"x-fetch-hook":"Go"`) {
		t.Errorf("the hook-rewritten request is not replayed under its original identity: %v", got)
	}
	if len(tags["/api/todos"]) != 1 {
		t.Errorf("the public todos request is not replayed: %v", tags)
	}
}

// external is a second origin the page can fetch: it answers with whatever
// Access-Control-Allow-Origin a path names.
func external(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Replay-Allowed", "shown-7")
		h.Set("X-Replay-Secret", "hidden-value-31")
		switch r.URL.Path {
		case "/wrong":
			h.Set("Access-Control-Allow-Origin", "http://elsewhere.test")
		case "/ok":
			h.Set("Access-Control-Allow-Origin", prodOrigin)
		case "/star":
			h.Set("Access-Control-Allow-Origin", "*")
		}
		if r.URL.Path == "/lamp-cookie" {
			h.Set("Access-Control-Allow-Origin", "*")
			h.Add("Set-Cookie", "lamp=1; Path=/")
		} else {
			h.Add("Set-Cookie", "lamp=1; Path=/")
			h.Add("Set-Cookie", "other=2; Path=/")
		}
		_, _ = w.Write([]byte("external-" + strings.TrimPrefix(r.URL.Path, "/")))
	}))
	t.Cleanup(server.Close)
	return server
}

type probeResult struct {
	Status  int    `json:"status"`
	Body    string `json:"body"`
	Header  string `json:"header"`
	Cookies string `json:"cookies"`
	Error   string `json:"error"`
}

var probeText = regexp.MustCompile(`data-testid="uf-probe">([^<]*)<`)

func probeOf(t *testing.T, h http.Handler, path, query string) string {
	t.Helper()
	m := probeText.FindStringSubmatch(document(t, h, path+query))
	if m == nil {
		t.Fatalf("no probe in %s%s", path, query)
	}
	return html.UnescapeString(m[1])
}

func externalProbe(t *testing.T, h http.Handler, ext *httptest.Server, path string, params url.Values) probeResult {
	t.Helper()
	params.Set("probe", "external")
	params.Set("target", ext.URL)
	params.Set("path", path)
	if params.Get("run") == "" {
		params.Set("run", "ext")
	}
	var out probeResult
	if err := json.Unmarshal([]byte(probeOf(t, h, "/universal-fetch", "?"+params.Encode())), &out); err != nil {
		t.Fatalf("decoding the probe: %v", err)
	}
	return out
}

func TestAnExternalFetchIsHeldToCorsAndNoCorsHidesTheBody(t *testing.T) {
	h := newProdHandler(t)
	ext := external(t)

	for _, tc := range []struct {
		name, path, mode string
		want             probeResult
	}{
		{"no allow-origin header", "/none", "cors", probeResult{Error: "CORS error: No 'Access-Control-Allow-Origin' header is present on the requested resource"}},
		{"another origin allowed", "/wrong", "cors", probeResult{Error: "CORS error: Incorrect 'Access-Control-Allow-Origin' header is present on the requested resource"}},
		{"this origin allowed", "/ok", "cors", probeResult{Status: 200, Body: "external-ok"}},
		{"any origin allowed", "/star", "cors", probeResult{Status: 200, Body: "external-star"}},
		// A no-cors fetch succeeds whatever the header says, and the page
		// may not see the body.
		{"no-cors without the header", "/none", "no-cors", probeResult{Status: 200, Body: ""}},
		{"no-cors with the header", "/ok", "no-cors", probeResult{Status: 200, Body: ""}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := externalProbe(t, h, ext, tc.path, url.Values{"mode": {tc.mode}})
			if got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestAnExternalResponsesHeadersAreHeldToTheAppsFilter(t *testing.T) {
	h := newProdHandler(t)
	ext := external(t)

	if got := externalProbe(t, h, ext, "/ok", url.Values{"header": {"x-replay-allowed"}}); got.Header != "shown-7" {
		t.Errorf("an allowed header read %q, want shown-7", got.Header)
	}
	if got := externalProbe(t, h, ext, "/ok", url.Values{"header": {"x-replay-secret"}}); got.Header != "denied" {
		t.Errorf("a withheld header read %q, want it denied", got.Header)
	}
	// Two cookies, one the filter allows: Kit judges every value, so reading
	// them is denied; with only the allowed one it is readable.
	if got := externalProbe(t, h, ext, "/ok", url.Values{"cookies": {"1"}}); got.Cookies != "denied" {
		t.Errorf("cookies with a withheld value read %q, want denied", got.Cookies)
	}
	if got := externalProbe(t, h, ext, "/lamp-cookie", url.Values{"cookies": {"1"}}); got.Cookies != "lamp=1; Path=/" {
		t.Errorf("an allowed cookie read %q, want lamp=1; Path=/", got.Cookies)
	}
}

func TestADeniedHeaderIsDeniedOnTheInternalFetchAndOnlyThere(t *testing.T) {
	h := newProdHandler(t)
	if got := probeOf(t, h, "/universal-fetch", "?run=sec&probe=secret"); got != `"denied"` {
		t.Errorf("a same-origin withheld header probe read %s, want denied", got)
	}
	// A page with csr = false serializes nothing for the browser, so Kit
	// does not guard the read: the app's own server may see every header.
	body := document(t, h, "/universal-fetch/no-script?run=sec2&probe=secret")
	if !strings.Contains(body, `"read:hidden-value-31"`) {
		t.Errorf("the csr=false page could not read the header: %s", body)
	}
	if strings.Contains(body, "<script") {
		t.Errorf("a csr=false page was sent a script")
	}
	if strings.Contains(body, "data-sveltekit-fetched") {
		t.Errorf("a csr=false page carries replay data")
	}
}

func TestAGoEndpointsFetchIsNotSubjectToTheUniversalFetchsCors(t *testing.T) {
	h := newProdHandler(t)
	ext := external(t)
	rec := getAs(t, h, "/api/replay/raw?run=raw&target="+url.QueryEscape(ext.URL)+"&path=/none", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Status int
		Body   string
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Status != 200 || out.Body != "external-none" {
		t.Errorf("the raw fetch of an answer with no allow-origin header = %+v, want 200 external-none", out)
	}
}

func TestARequestFetchedWithABodyOrHeadersIsToldApartByThem(t *testing.T) {
	h := newProdHandler(t)
	// Each hash is Kit's hash.js over what the load sent. A Request object
	// carries its content-type into the hash; the same call made with
	// (url, init) would not.
	for _, tc := range []struct {
		kind, hash, answer string
	}{
		{"object", "1u1vmb3", `{"body":"object-body","contentType":"text/plain;charset=UTF-8","header":"r","method":"POST","step":"1"}`},
		{"bytes", "3n6rkx", `{"body":"bytes-body","contentType":"","header":"","method":"POST","step":"1"}`},
		{"header", "13013q", `{"body":"both","contentType":"text/plain;charset=UTF-8","header":"h","method":"POST","step":"1"}`},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			body := document(t, h, "/universal-fetch?run=req-"+tc.kind+"&step=1&probe=request&kind="+tc.kind)
			tags := fetchedTags(t, body)
			got := tags["/api/replay/echo?run=req-"+tc.kind+"&step=1"]
			if len(got) != 1 || !strings.Contains(got[0], `data-hash="`+tc.hash+`"`) {
				t.Errorf("the replay does not carry hash %s: %v", tc.hash, got)
			}
			if m := probeText.FindStringSubmatch(body); m == nil || html.UnescapeString(m[1]) != tc.answer {
				t.Errorf("the probe is %q, want %s", m, tc.answer)
			}
		})
	}
}

func TestCookiesAFetchSetsReachTheNextFetchAndTheDocument(t *testing.T) {
	h := newProdHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/universal-fetch?run=jar&step=1&probe=cookies", nil)
	req.AddCookie(&http.Cookie{Name: "visitor", Value: "ada"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	// The first fetch saw the browser's cookie; the second saw it and the two
	// the first one set, written back as the endpoint wrote them.
	m := probeText.FindStringSubmatch(rec.Body.String())
	if m == nil {
		t.Fatal("no probe in the document")
	}
	if got, want := html.UnescapeString(m[1]), `{"first":"visitor=ada","second":"visitor=ada; lamp=glow; other=dim"}`; got != want {
		t.Errorf("probe = %s, want %s", got, want)
	}
	// And both reach the visitor on the document's own response.
	var set []string
	for _, line := range rec.Result().Header.Values("Set-Cookie") {
		set = append(set, strings.SplitN(line, ";", 2)[0])
	}
	if strings.Join(set, ",") != "lamp=glow,other=dim" {
		t.Errorf("the document's Set-Cookie = %v, want lamp=glow and other=dim", set)
	}
}
