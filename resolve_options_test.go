package skgo

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tylergannon/skgo/internal/kithash"
	"github.com/tylergannon/skgo/internal/ssr"
)

// These tests drive the real composed handler — a Middleware over Loads over
// Remotes over Endpoints over the page renderer's own document path, with a
// tiny JavaScript bundle standing in for kit's — and compare against literals
// written out of the pinned kit (`respond.js`, `hooks/sequence.js`,
// `page/render.js`), never read back off the implementation.

const optionsBundle = `
globalThis.__skgo_ping = () => 'ok';
globalThis.__skgo_render = (json) => {
  const req = JSON.parse(json);
  const result = {done: false, status: req.status, head: '<title>t</title>', body: '', fetched: ''};
  if (req.route_id === '/fetching') {
    __skgo_fetch(JSON.stringify({method: 'GET', url: 'http://127.0.0.1:8080/api/probe'})).then(function (raw) {
      const answer = JSON.parse(raw);
      result.body = 'serialized=' + answer.response.headers.filter(function (h) { return h[2]; })
        .map(function (h) { return h[0]; }).join(',') + ';';
      result.done = true;
    }, function (e) { result.failure = String(e); result.done = true; });
    return result;
  }
  result.body = '<p>route=' + req.route_id + ' status=' + req.status + '</p>';
  result.done = true;
  return result;
};`

const optionsTemplate = "<!doctype html>\n<html>\n\t<head>\n\t\t%sveltekit.head%\n\t</head>\n\t<body>\n\t\t<div>%sveltekit.body%</div>\n\t</body>\n</html>\n"

type optionsStreamData struct {
	Now  string           `json:"now"`
	Slow Deferred[string] `json:"slow"`
}

type optionsApp struct {
	handler      http.Handler
	release      chan struct{}
	probes       *probeBarrier
	renderer     *SSR
	greeting     *Remote
	remotePrefix string
}

// probeBarrier answers /api/probe with three headers, and — when expecting
// is set — holds every caller until that many have arrived, which proves the
// renders it belongs to were in flight at once.
type probeBarrier struct {
	mu        sync.Mutex
	arrived   int
	expecting int
	gate      chan struct{}
}

func (p *probeBarrier) serve(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	expecting := p.expecting
	var wait chan struct{}
	if expecting > 0 {
		p.arrived++
		if p.arrived == expecting {
			close(p.gate)
		}
		wait = p.gate
	}
	p.mu.Unlock()
	if wait != nil {
		select {
		case <-wait:
		case <-time.After(5 * time.Second):
			http.Error(w, "the other render never arrived", http.StatusGatewayTimeout)
			return
		}
	}
	w.Header().Set("X-Public", "pub-1")
	w.Header().Set("X-Private", "secret-1")
	w.Header().Set("X-Extra", "extra-1")
	_, _ = io.WriteString(w, "probed")
}

func optionsStack(t *testing.T, mw Middleware) *optionsApp {
	t.Helper()
	const origin = "http://127.0.0.1:8080"
	no := false
	m := Manifest{
		AppDir: "_app",
		Nodes:  []string{"", "", "", "src/routes/stream/+page.server.ts", "", ""},
		Routes: []ManifestRoute{
			{ID: "/", Pattern: `^\/$`, Page: &ManifestPage{Layouts: []int{0}, Leaf: 2}},
			{ID: "/stream", Pattern: `^\/stream\/?$`, Page: &ManifestPage{Layouts: []int{0}, Leaf: 3}},
			{ID: "/spa", Pattern: `^\/spa\/?$`, Page: &ManifestPage{Layouts: []int{0}, Leaf: 4}},
			{ID: "/fetching", Pattern: `^\/fetching\/?$`, Page: &ManifestPage{Layouts: []int{0}, Leaf: 5}},
			{ID: "/api/echo", Pattern: `^\/api\/echo\/?$`, Endpoint: &ManifestEndpoint{Methods: []string{"GET"}}},
		},
	}
	app := &optionsApp{release: make(chan struct{}), probes: &probeBarrier{gate: make(chan struct{})}}

	stream := NewLoad("src/routes/stream/+page.server.ts", func(ctx context.Context) (optionsStreamData, error) {
		return optionsStreamData{
			Now: "now-value",
			Slow: Async(ctx, func(ctx context.Context) (string, error) {
				select {
				case <-app.release:
					return "late-literal-value", nil
				case <-ctx.Done():
					return "", ctx.Err()
				}
			}),
		}, nil
	})
	ls, err := NewLoads(m.LoadConfig(origin), stream)
	if err != nil {
		t.Fatal(err)
	}
	es, err := NewEndpoints(m.EndpointConfig(origin), NewEndpoint("/api/echo", "GET", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "endpoint-body")
	}))
	if err != nil {
		t.Fatal(err)
	}
	greeting := NewQueryNoArg(testModule, "greeting", func(context.Context) (string, error) { return "remote-value", nil })
	rs := testRemotes(t, RemoteConfig{AppDir: "_app", Origin: origin}, greeting)

	engine, err := ssr.New("options.js", []byte(optionsBundle), 4, nil)
	if err != nil {
		t.Fatal(err)
	}
	probe := http.NewServeMux()
	probe.HandleFunc("/api/probe", app.probes.serve)
	renderer := &SSR{
		loads:   ls,
		remotes: rs,
		engine:  engine,
		fetch:   probe,
		version: "v-test",
		info: ManifestSSR{
			GlobalName: "__sveltekit_t",
			Client: ManifestClient{
				Start:   "_app/immutable/entry/start.js",
				App:     "_app/immutable/entry/app.js",
				Imports: []string{"_app/immutable/chunks/shared.js"},
			},
			Nodes: []ManifestSSRNode{
				{Index: 0},
				{Index: 1},
				{
					Index:       2,
					Stylesheets: []string{"_app/immutable/assets/page.css"},
					Fonts:       []ManifestFont{{File: "_app/immutable/assets/inter.abc.woff2", Filename: "src/lib/inter.woff2"}},
				},
				{Index: 3},
				{Index: 4, SSR: &no},
				{Index: 5},
			},
		},
		template:  optionsTemplate,
		errorPage: "<!doctype html><h1>%sveltekit.status%</h1><p>%sveltekit.error.message%</p>",
	}
	pages := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !renderer.serve(w, r, r.URL.Path) {
			http.NotFound(w, r)
		}
	})
	cfg := m.HandleConfig()
	cfg.Origin = origin
	cfg.Loads = ls
	cfg.OnPanic = func(string, any, []byte) {}
	app.handler = mw.Intercept(cfg, ls.Intercept(rs.Intercept(es.Intercept(pages))))
	app.renderer = renderer
	app.greeting = greeting
	app.remotePrefix = rs.Prefix()
	return app
}

func optionsGet(h http.Handler, target string, header http.Header) *httptest.ResponseRecorder {
	return mwDo(h, "GET", target, header)
}

// chunkLog records what a transform callback was handed.
type chunkLog struct {
	mu    sync.Mutex
	calls []bool
	htmls []string
}

func (l *chunkLog) record(html string, done bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, done)
	l.htmls = append(l.htmls, html)
}

func (l *chunkLog) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.calls)
}

func (l *chunkLog) allDone() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, d := range l.calls {
		if !d {
			return false
		}
	}
	return true
}

type countingPolicy struct {
	mu       sync.Mutex
	headers  []string
	preloads []PreloadInput
}

func (c *countingPolicy) filter(allow ...string) func(string, string) bool {
	return func(name, _ string) bool {
		c.mu.Lock()
		c.headers = append(c.headers, name)
		c.mu.Unlock()
		for _, a := range allow {
			if a == name {
				return true
			}
		}
		return false
	}
}

func (c *countingPolicy) preload(answer func(PreloadInput) bool) func(PreloadInput) bool {
	return func(in PreloadInput) bool {
		c.mu.Lock()
		c.preloads = append(c.preloads, in)
		c.mu.Unlock()
		return answer(in)
	}
}

func (c *countingPolicy) counts() (headers, preloads int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.headers), len(c.preloads)
}

func plain(ctx context.Context, e *Event, resolve Resolve) (*http.Response, error) {
	return resolve(ctx)
}

func optionsTransform(old, new string, log *chunkLog) func(context.Context, string, bool) (string, error) {
	return func(_ context.Context, html string, done bool) (string, error) {
		if log != nil {
			log.record(html, done)
		}
		return strings.Replace(html, old, new, 1), nil
	}
}

const (
	fontLink   = `<link href="/_app/immutable/assets/inter.abc.woff2" rel="preload" as="font" type="font/woff2" crossorigin>`
	scriptLink = `<link href="/_app/immutable/chunks/shared.js" rel="modulepreload">`
	styleLink  = `<link href="/_app/immutable/assets/page.css" rel="stylesheet">`
	titleTag   = "<title>t</title>"
)

func TestResolveOptionsDefaultsPreloadScriptsButNotFontsAndKeepStylesheets(t *testing.T) {
	app := optionsStack(t, plain)
	rec := optionsGet(app.handler, "/", nil)
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, titleTag) {
		t.Fatalf("status %d, body %q", rec.Code, body)
	}
	if !strings.Contains(body, scriptLink) {
		t.Errorf("the default must preload JavaScript; got:\n%s", body)
	}
	if strings.Contains(body, fontLink) {
		t.Errorf("the default must not preload fonts; got:\n%s", body)
	}
	if !strings.Contains(body, styleLink) {
		t.Errorf("the stylesheet link is required; got:\n%s", body)
	}
}

func TestResolveOptionsExplicitFontPreloadCarriesTheFilename(t *testing.T) {
	var policy countingPolicy
	mw := func(ctx context.Context, e *Event, resolve Resolve) (*http.Response, error) {
		return resolve(ctx, ResolveOptions{Preload: policy.preload(func(in PreloadInput) bool { return in.Type == "font" })})
	}
	app := optionsStack(t, mw)
	body := optionsGet(app.handler, "/", nil).Body.String()
	if !strings.Contains(body, fontLink) {
		t.Errorf("the chosen font preload is missing:\n%s", body)
	}
	if strings.Contains(body, scriptLink) {
		t.Errorf("an explicit policy replaces the default, so JavaScript is no longer preloaded:\n%s", body)
	}
	if !strings.Contains(body, styleLink) {
		t.Errorf("the stylesheet link is required whatever preload answers:\n%s", body)
	}
	want := []PreloadInput{
		{Type: "font", Path: "/_app/immutable/assets/inter.abc.woff2", Filename: "src/lib/inter.woff2"},
		{Type: "js", Path: "/_app/immutable/chunks/shared.js"},
	}
	policy.mu.Lock()
	defer policy.mu.Unlock()
	if fmt.Sprint(policy.preloads) != fmt.Sprint(want) {
		t.Errorf("preload was asked %v, want %v", policy.preloads, want)
	}
}

func TestResolveOptionsDeniedPreloadRemovesLinksButNotTheStylesheet(t *testing.T) {
	mw := func(ctx context.Context, e *Event, resolve Resolve) (*http.Response, error) {
		return resolve(ctx, ResolveOptions{Preload: func(PreloadInput) bool { return false }})
	}
	body := optionsGet(optionsStack(t, mw).handler, "/", nil).Body.String()
	if strings.Contains(body, scriptLink) || strings.Contains(body, fontLink) {
		t.Errorf("a denied preload still produced a link:\n%s", body)
	}
	if !strings.Contains(body, styleLink) {
		t.Errorf("denying preloads removed the required stylesheet link:\n%s", body)
	}
}

func TestResolveOptionsTransformPageChunkRunsOnceDoneOnTheAssembledDocument(t *testing.T) {
	var log chunkLog
	mw := func(ctx context.Context, e *Event, resolve Resolve) (*http.Response, error) {
		return resolve(ctx, ResolveOptions{TransformPageChunk: optionsTransform("<html>", `<html data-mark="x">`, &log)})
	}
	rec := optionsGet(optionsStack(t, mw).handler, "/", nil)
	body := rec.Body.String()
	if log.count() != 1 || !log.allDone() {
		t.Fatalf("transform calls = %v, want exactly one with done:true", log.calls)
	}
	// What the transform was handed is the whole assembled document.
	handed := log.htmls[0]
	for _, part := range []string{"<!doctype html>", titleTag, "<p>route=/ status=200</p>", "</html>"} {
		if !strings.Contains(handed, part) {
			t.Errorf("the transform was not handed the whole document; missing %q in\n%s", part, handed)
		}
	}
	if !strings.Contains(body, `<html data-mark="x">`) || strings.Contains(body, "<html>") {
		t.Errorf("the transformed document was not served:\n%s", body)
	}
	etag := `"` + kithash.Kit(body) + `"`
	if got := rec.Header().Get("ETag"); got != etag {
		t.Errorf("ETag = %s, want the hash of the transformed finite document %s", got, etag)
	}
}

func TestResolveOptionsEtagFollowsTheTransformedDocumentAndStaysConditional(t *testing.T) {
	transformed := func(ctx context.Context, e *Event, resolve Resolve) (*http.Response, error) {
		return resolve(ctx, ResolveOptions{TransformPageChunk: optionsTransform("<html>", `<html data-mark="x">`, nil)})
	}
	withOptions := optionsStack(t, transformed)
	untouched := optionsStack(t, plain)
	a := optionsGet(withOptions.handler, "/", nil)
	b := optionsGet(untouched.handler, "/", nil)
	if a.Header().Get("ETag") == "" || a.Header().Get("ETag") == b.Header().Get("ETag") {
		t.Fatalf("etags %q and %q must differ: the documents differ", a.Header().Get("ETag"), b.Header().Get("ETag"))
	}
	again := optionsGet(withOptions.handler, "/", http.Header{"If-None-Match": {a.Header().Get("ETag")}})
	if again.Code != http.StatusNotModified || again.Body.Len() != 0 {
		t.Errorf("a matching ETag got %d with %d body bytes, want 304 and none", again.Code, again.Body.Len())
	}
	// The transformed document's ETag does not revalidate the untouched one.
	stale := optionsGet(untouched.handler, "/", http.Header{"If-None-Match": {a.Header().Get("ETag")}})
	if stale.Code != http.StatusOK {
		t.Errorf("the other document's ETag answered %d, want 200", stale.Code)
	}
}

func TestResolveOptionsTransformsErrorDocumentAndSPAShellButNotStaticOrOtherResponses(t *testing.T) {
	var log chunkLog
	mw := func(ctx context.Context, e *Event, resolve Resolve) (*http.Response, error) {
		return resolve(ctx, ResolveOptions{TransformPageChunk: optionsTransform("<html>", `<html data-mark="x">`, &log)})
	}
	app := optionsStack(t, mw)

	documents := []struct {
		name, target string
		status       int
	}{
		{"native 404 document", "/missing", 404},
		{"ssr=false shell", "/spa", 200},
	}
	for _, d := range documents {
		before := log.count()
		rec := optionsGet(app.handler, d.target, nil)
		if rec.Code != d.status || !strings.Contains(rec.Body.String(), `<html data-mark="x">`) {
			t.Errorf("%s: status %d, body:\n%s", d.name, rec.Code, rec.Body.String())
		}
		if log.count() != before+1 {
			t.Errorf("%s: transform ran %d times, want 1", d.name, log.count()-before)
		}
	}
	if !strings.Contains(optionsGet(app.handler, "/spa", nil).Body.String(), "__sveltekit_t") {
		t.Error("the SPA shell is no longer bootable after the transform")
	}

	before := log.count()
	data := optionsGet(app.handler, "/stream/__data.json?x-sveltekit-invalidated=1", nil)
	endpoint := optionsGet(app.handler, "/api/echo", nil)
	remote := optionsGet(app.handler, app.remotePrefix+app.greeting.ID(), nil)
	if log.count() != before {
		t.Errorf("transform ran %d time(s) for data, endpoint and remote responses; they are not documents", log.count()-before)
	}
	if endpoint.Body.String() != "endpoint-body" {
		t.Errorf("endpoint body = %q", endpoint.Body.String())
	}
	if strings.Contains(data.Body.String(), "data-mark") || strings.Contains(remote.Body.String(), "data-mark") {
		t.Error("a data or remote response was transformed")
	}
	if remote.Code != 200 || data.Code != 200 {
		t.Errorf("data %d, remote %d", data.Code, remote.Code)
	}
}

func TestSequenceOptionsComposeInnerTransformBeforeOuter(t *testing.T) {
	var inner, outer chunkLog
	a := func(ctx context.Context, e *Event, resolve Resolve) (*http.Response, error) {
		return resolve(ctx, ResolveOptions{TransformPageChunk: optionsTransform("<title>B(t)</title>", "<title>A(B(t))</title>", &outer)})
	}
	b := func(ctx context.Context, e *Event, resolve Resolve) (*http.Response, error) {
		return resolve(ctx, ResolveOptions{TransformPageChunk: optionsTransform("<title>t</title>", "<title>B(t)</title>", &inner)})
	}
	rec := optionsGet(optionsStack(t, Sequence(a, b)).handler, "/", nil)
	body := rec.Body.String()
	if !strings.Contains(body, "<title>A(B(t))</title>") {
		t.Errorf("x -> B(x) -> A(B(x)) did not happen:\n%s", body)
	}
	if inner.count() != 1 || outer.count() != 1 || !inner.allDone() || !outer.allDone() {
		t.Errorf("inner calls %v, outer calls %v; each must run once with done:true", inner.calls, outer.calls)
	}
	if !strings.Contains(outer.htmls[0], "<title>B(t)</title>") || !strings.Contains(inner.htmls[0], "<title>t</title>") {
		t.Errorf("the outer transform must be handed the inner one's output")
	}
	reversed := optionsGet(optionsStack(t, Sequence(b, a)).handler, "/", nil).Body.String()
	if !strings.Contains(reversed, "<title>B(t)</title>") || strings.Contains(reversed, "A(B(t))") {
		t.Errorf("swapping the order must change the result (A inner, B outer):\n%s", reversed)
	}
}

func TestSequenceOptionsOuterDenialWinsWithoutCallingInnerCallbacks(t *testing.T) {
	var outer, inner countingPolicy
	a := func(ctx context.Context, e *Event, resolve Resolve) (*http.Response, error) {
		return resolve(ctx, ResolveOptions{
			FilterSerializedResponseHeaders: outer.filter(),
			Preload:                         outer.preload(func(PreloadInput) bool { return false }),
		})
	}
	b := func(ctx context.Context, e *Event, resolve Resolve) (*http.Response, error) {
		return resolve(ctx, ResolveOptions{
			FilterSerializedResponseHeaders: inner.filter("x-public", "x-private", "x-extra"),
			Preload:                         inner.preload(func(PreloadInput) bool { return true }),
		})
	}
	app := optionsStack(t, Sequence(a, b))
	page := optionsGet(app.handler, "/", nil).Body.String()
	if strings.Contains(page, scriptLink) || strings.Contains(page, fontLink) || !strings.Contains(page, styleLink) {
		t.Errorf("the outer preload denial must win and keep the stylesheet:\n%s", page)
	}
	fetching := optionsGet(app.handler, "/fetching", nil).Body.String()
	if !strings.Contains(fetching, "serialized=;") {
		t.Errorf("the outer header denial must win:\n%s", fetching)
	}
	if h, p := outer.counts(); h == 0 || p == 0 {
		t.Errorf("the outer callbacks were never asked (headers %d, preloads %d)", h, p)
	}
	if h, p := inner.counts(); h != 0 || p != 0 {
		t.Errorf("inner callbacks were called (headers %d, preloads %d) although an outer one existed", h, p)
	}
}

func TestSequenceOptionsLaterCallbackAppliesWhenNoOuterOneIsDefined(t *testing.T) {
	var inner countingPolicy
	a := plain
	b := func(ctx context.Context, e *Event, resolve Resolve) (*http.Response, error) {
		return resolve(ctx, ResolveOptions{
			FilterSerializedResponseHeaders: inner.filter("x-extra"),
			Preload:                         inner.preload(func(in PreloadInput) bool { return in.Type == "font" }),
		})
	}
	app := optionsStack(t, Sequence(a, b))
	page := optionsGet(app.handler, "/", nil).Body.String()
	if !strings.Contains(page, fontLink) || strings.Contains(page, scriptLink) {
		t.Errorf("the inner preload choice was not applied:\n%s", page)
	}
	if got := optionsGet(app.handler, "/fetching", nil).Body.String(); !strings.Contains(got, "serialized=x-extra;") {
		t.Errorf("the inner header filter was not applied: %s", got)
	}
	if h, p := inner.counts(); h == 0 || p == 0 {
		t.Errorf("the inner callbacks were not asked (headers %d, preloads %d)", h, p)
	}
}

func TestResolveOptionsRequestPolicyOverridesAndDefaultsToTheRenderersFilter(t *testing.T) {
	// No options: the renderer's own filter is the compatibility default.
	app := optionsStack(t, plain)
	if got := optionsGet(app.handler, "/fetching", nil).Body.String(); !strings.Contains(got, "serialized=;") {
		t.Errorf("with no filter anywhere nothing is serialized: %s", got)
	}

	mw := func(ctx context.Context, e *Event, resolve Resolve) (*http.Response, error) {
		if e.Request().Header.Get("X-Pick") == "request" {
			return resolve(ctx, ResolveOptions{FilterSerializedResponseHeaders: func(name, _ string) bool { return name == "x-extra" }})
		}
		return resolve(ctx)
	}
	app = optionsStack(t, mw)
	app.renderer.filterSerializedResponseHeaders = func(name, _ string) bool { return name == "x-public" }
	if got := optionsGet(app.handler, "/fetching", nil).Body.String(); !strings.Contains(got, "serialized=x-public;") {
		t.Errorf("the SSR-level filter is the default when the request chose nothing: %s", got)
	}
	got := optionsGet(app.handler, "/fetching", http.Header{"X-Pick": {"request"}}).Body.String()
	if !strings.Contains(got, "serialized=x-extra;") {
		t.Errorf("an explicit request filter must win over the SSR-level one: %s", got)
	}
	if app.renderer.filterSerializedResponseHeaders("x-public", "") != true || app.renderer.filterSerializedResponseHeaders("x-extra", "") != false {
		t.Error("serving a request mutated the renderer's shared filter")
	}
	if got := optionsGet(app.handler, "/fetching", nil).Body.String(); !strings.Contains(got, "serialized=x-public;") {
		t.Errorf("a later request without options inherited the earlier request's choice: %s", got)
	}
}

func TestResolveOptionsConcurrentRequestsKeepTheirOwnPoliciesTransformsAndObservations(t *testing.T) {
	var logA, logB chunkLog
	var policyA, policyB countingPolicy
	mw := func(ctx context.Context, e *Event, resolve Resolve) (*http.Response, error) {
		switch e.Request().Header.Get("X-Tenant") {
		case "A":
			return resolve(ctx, ResolveOptions{
				TransformPageChunk:              optionsTransform("<html>", `<html data-tenant="A">`, &logA),
				FilterSerializedResponseHeaders: policyA.filter("x-public"),
			})
		case "B":
			return resolve(ctx, ResolveOptions{
				TransformPageChunk:              optionsTransform("<html>", `<html data-tenant="B">`, &logB),
				FilterSerializedResponseHeaders: policyB.filter("x-extra"),
			})
		}
		return resolve(ctx)
	}
	app := optionsStack(t, mw)
	app.probes.expecting = 2

	var wg sync.WaitGroup
	results := map[string]*httptest.ResponseRecorder{}
	var mu sync.Mutex
	for _, tenant := range []string{"A", "B"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := optionsGet(app.handler, "/fetching", http.Header{"X-Tenant": {tenant}})
			mu.Lock()
			results[tenant] = rec
			mu.Unlock()
		}()
	}
	wg.Wait()

	a, b := results["A"].Body.String(), results["B"].Body.String()
	if !strings.Contains(a, `<html data-tenant="A">`) || !strings.Contains(a, "serialized=x-public;") {
		t.Errorf("request A:\n%s", a)
	}
	if !strings.Contains(b, `<html data-tenant="B">`) || !strings.Contains(b, "serialized=x-extra;") {
		t.Errorf("request B:\n%s", b)
	}
	for _, body := range []string{a, b} {
		if strings.Contains(body, "secret-1") || strings.Contains(body, "x-private") {
			t.Errorf("a denied private header leaked:\n%s", body)
		}
	}
	if strings.Contains(a, `data-tenant="B"`) || strings.Contains(b, `data-tenant="A"`) {
		t.Error("one request's transform reached the other's document")
	}
	if logA.count() != 1 || logB.count() != 1 {
		t.Errorf("transform calls: A %d, B %d; want one each", logA.count(), logB.count())
	}
	policyA.mu.Lock()
	policyB.mu.Lock()
	defer policyA.mu.Unlock()
	defer policyB.mu.Unlock()
	if len(policyA.headers) == 0 || len(policyB.headers) == 0 {
		t.Errorf("each request's own filter must be asked: A %v, B %v", policyA.headers, policyB.headers)
	}
}

func readUntil(t *testing.T, body io.Reader, marker string, within time.Duration) string {
	t.Helper()
	type result struct {
		text string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		var out strings.Builder
		buf := make([]byte, 1)
		for !strings.HasSuffix(out.String(), marker) {
			n, err := body.Read(buf)
			out.Write(buf[:n])
			if err != nil {
				done <- result{out.String(), err}
				return
			}
		}
		done <- result{out.String(), nil}
	}()
	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("read %q before the marker: %v", r.text, r.err)
		}
		return r.text
	case <-time.After(within):
		t.Fatalf("%q did not arrive within %s", marker, within)
		return ""
	}
}

func TestResolveOptionsTransformedShellArrivesBeforeTheProducerIsReleasedAndChunksAreNotRetransformed(t *testing.T) {
	var log chunkLog
	mw := func(ctx context.Context, e *Event, resolve Resolve) (*http.Response, error) {
		return resolve(ctx, ResolveOptions{TransformPageChunk: optionsTransform("<html>", `<html data-mark="x">`, &log)})
	}
	app := optionsStack(t, mw)
	srv := httptest.NewServer(app.handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	shell := readUntil(t, resp.Body, "</html>\n", 5*time.Second)
	if !strings.Contains(shell, `<html data-mark="x">`) || !strings.Contains(shell, "<p>route=/stream status=200</p>") {
		t.Fatalf("the shell is not the transformed document:\n%s", shell)
	}
	if strings.Contains(shell, "late-literal-value") {
		t.Fatalf("the shell already holds the value that is still gated:\n%s", shell)
	}
	if log.count() != 1 {
		t.Fatalf("the transform ran %d times before the producer was released, want 1", log.count())
	}

	close(app.release)
	rest, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rest), "late-literal-value") || !strings.Contains(string(rest), "<script>") {
		t.Errorf("the gated value never arrived as a chunk:\n%s", rest)
	}
	if strings.Contains(string(rest), "data-mark") || strings.Contains(string(rest), "<html") {
		t.Errorf("a deferred chunk was transformed:\n%s", rest)
	}
	if log.count() != 1 || !log.allDone() {
		t.Errorf("the transform ran %d times (done=%v) over the whole response, want once with done:true", log.count(), log.calls)
	}
}

func TestResolveOptionsTransformErrorFallsBackToTheStaticErrorPage(t *testing.T) {
	mw := func(ctx context.Context, e *Event, resolve Resolve) (*http.Response, error) {
		return resolve(ctx, ResolveOptions{TransformPageChunk: func(context.Context, string, bool) (string, error) {
			return "", fmt.Errorf("transform exploded")
		}})
	}
	rec := optionsGet(optionsStack(t, mw).handler, "/", nil)
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "exploded") {
		t.Errorf("status %d, body %q", rec.Code, rec.Body.String())
	}
}

func TestResolveOptionsRejectsMoreThanOneOptionsValueAndASecondResolve(t *testing.T) {
	var got error
	mw := func(ctx context.Context, e *Event, resolve Resolve) (*http.Response, error) {
		_, got = resolve(ctx, ResolveOptions{}, ResolveOptions{})
		return NewResponse(http.StatusTeapot, nil, strings.NewReader("short")), nil
	}
	rec := optionsGet(optionsStack(t, mw).handler, "/", nil)
	if got == nil || rec.Code != http.StatusTeapot {
		t.Errorf("resolve with two option values: err %v, status %d", got, rec.Code)
	}
}
