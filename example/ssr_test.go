package example_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example"
	"github.com/tylergannon/skgo/example/businesslogic"
	"github.com/tylergannon/skgo/example/generated"
)

// TestTheProductionBundleRunsInAFreshRuntime is the check that turns a
// SvelteKit or Svelte upgrade skgo's engine cannot run into a red build rather
// than a broken deployment.
//
// The engine is a pure ECMAScript interpreter with no published compatibility
// statement. A minor release that emits a syntax it cannot parse, or reaches
// for a web global the bundle does not carry, breaks at the moment the bundle
// is compiled into a runtime — which is here, on the bytes that would ship.
func TestTheProductionBundleRunsInAFreshRuntime(t *testing.T) {
	dist := prodDist(t)
	manifest, err := skgo.ReadManifest(dist)
	if err != nil {
		t.Fatalf("reading the build manifest: %v", err)
	}
	if manifest.SSR == nil {
		t.Fatal("the build carries no SSR bundle")
	}

	remotes, err := skgo.NewRemotes(manifest.RemoteConfig(prodOrigin), generated.Remotes()...)
	if err != nil {
		t.Fatalf("building the remote registry: %v", err)
	}
	loadCfg := manifest.LoadConfig(prodOrigin)
	loads, err := skgo.NewLoads(loadCfg, generated.Loads()...)
	if err != nil {
		t.Fatalf("building the load registry: %v", err)
	}

	if _, err := skgo.NewSSR(dist, manifest, loads, remotes, skgo.SSROptions{Runtimes: 1}); err != nil {
		t.Fatalf("the SSR bundle this build ships does not run: %v", err)
	}
}

// TestARenderedPageCarriesGosAnswerToTheBrowser is the whole point of the
// renderer, at the level a browser sees it: the markup is already in the
// document, and the value behind it is in the boot payload under the key kit's
// own query cache looks it up by — so the client has no reason to ask again.
//
// The fixture is named here rather than read off the page: `getSite` in
// src/routes/site.remote.go answers with this name and this path, and nothing
// else in the app can produce either string.
func TestARenderedPageCarriesGosAnswerToTheBrowser(t *testing.T) {
	h := newProdHandler(t)

	body := get(t, h, "/").Body.String()

	for _, want := range []string{
		`<p data-testid="site-name">skgo</p>`,
		`<p data-testid="colocated">src/routes/site.remote.go</p>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the document does not contain %s", want)
		}
	}

	// `<hash>/<name>/<payload>`, with an empty payload for a query that takes
	// no argument — kit's `create_remote_key`.
	site := remoteID(t, "getSite")
	if !strings.Contains(body, `"`+site+`/":{v:{colocated:"src/routes/site.remote.go",name:"skgo"}}`) {
		t.Errorf("the boot payload does not carry %s's answer; the client would fetch it again", site)
	}
}

// TestAQueryWithAnArgumentIsRenderedWithTheArgumentGoWasGiven pins the other
// half of the key: a query keyed by its argument has to be rendered with the
// argument the request carried, and cached under that argument, or the client
// looks up a key that is not there.
func TestAQueryWithAnArgumentIsRenderedWithTheArgumentGoWasGiven(t *testing.T) {
	h := newProdHandler(t)

	body := get(t, h, "/items/93").Body.String()

	if !strings.Contains(body, `<p data-testid="item-name">Widget 93</p>`) {
		t.Error(`the document does not name the item Go was asked for ("Widget 93")`)
	}
	if strings.Contains(body, "Widget 42") {
		t.Error("the document names an item nobody asked for")
	}
	item := remoteID(t, "getItem")
	if !strings.Contains(body, `"`+item+`/`) {
		t.Errorf("the boot payload carries no answer for %s", item)
	}
}

// TestTwoPagesRenderingAtOnceAreEachTheirOwn puts the pool under real HTTP.
// Two documents rendered at the same time on one runtime would each carry the
// other's module state, and the symptom is a page that is silently somebody
// else's.
func TestTwoPagesRenderingAtOnceAreEachTheirOwn(t *testing.T) {
	h := newProdHandler(t)

	paths := []string{"/items/11", "/items/22", "/items/33", "/items/44", "/", "/items/55", "/items/66", "/items/77"}
	bodies := make([]string, len(paths))

	start := make(chan struct{})
	var wg sync.WaitGroup
	for i, path := range paths {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			bodies[i] = rec.Body.String()
		}()
	}
	close(start)
	wg.Wait()

	for i, path := range paths {
		if path == "/" {
			if !strings.Contains(bodies[i], `<h1 data-testid="title">Home</h1>`) {
				t.Errorf("%s did not render the home page", path)
			}
			continue
		}
		id := strings.TrimPrefix(path, "/items/")
		if !strings.Contains(bodies[i], `<p data-testid="item-name">Widget `+id+`</p>`) {
			t.Errorf("%s did not render Widget %s", path, id)
		}
		for _, other := range paths {
			if other == path || other == "/" {
				continue
			}
			if strings.Contains(bodies[i], "Widget "+strings.TrimPrefix(other, "/items/")+"</p>") {
				t.Errorf("%s carries %s's item", path, other)
			}
		}
	}
}

// TestARenderedPageDoesNotRepeatItself renders the same page twice and requires
// the same bytes, because the document's ETag is a hash of itself: a map
// iterated in a different order would make every reload a fresh 200 and every
// conditional request a wasted round trip.
//
// The page is one whose data does not change between requests. `/account` is
// not: its layout hands out a fresh serial per load, on purpose, and two
// renders of it are supposed to differ.
func TestARenderedPageDoesNotRepeatItself(t *testing.T) {
	h := newProdHandler(t)

	first, firstETag := get(t, h, "/items/42").Body.String(), get(t, h, "/items/42").Header().Get("ETag")
	second := get(t, h, "/items/42").Body.String()
	if first != second {
		t.Error("the same page rendered twice produced different bytes")
	}
	if firstETag == "" {
		t.Fatal("a rendered page carried no ETag")
	}

	req := httptest.NewRequest(http.MethodGet, "/items/42", nil)
	req.Header.Set("If-None-Match", firstETag)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotModified {
		t.Errorf("a conditional request for an unchanged page returned %d, want 304", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Error("a 304 carried a body")
	}
}

// TestALayoutsDataAndItsPagesDataArriveTogether checks the hydration array a
// page under a layout boots from: one entry per node of the branch, each
// carrying what that node's Go load returned.
func TestALayoutsDataAndItsPagesDataArriveTogether(t *testing.T) {
	h := newProdHandler(t)
	session := businesslogic.Default.SignIn("grace")

	req := httptest.NewRequest(http.MethodGet, "/account", nil)
	req.AddCookie(&http.Cookie{Name: example.SessionCookie, Value: session})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	body := rec.Body.String()

	if !strings.Contains(body, `<p data-testid="account-user">Account of grace</p>`) {
		t.Error("the layout's data is not in the markup")
	}
	if !strings.Contains(body, `<p data-testid="parent-user">The layout loaded grace</p>`) {
		t.Error("the page's data is not in the markup")
	}
	if !strings.Contains(body, `data:{accountSerial:`) || !strings.Contains(body, `accountUser:"grace"`) {
		t.Error("the layout's data is not in the hydration array")
	}
	if !strings.Contains(body, `data:{parentUser:"grace"}`) {
		t.Error("the page's data is not in the hydration array")
	}
}

// TestATransportedValueReachesTheEngineWithItsType is the seam this file exists
// to pin: what the engine renders against is a value of the app's own class,
// not the object its fields travelled in.
//
// The fixture is 4500 cents, from `pageLoad` in
// src/routes/(marketing)/pricing/page.server.go, and it is the only place in
// the app that amount is written. "$45.00" is what `Money.format()` in
// src/hooks.ts makes of it — a method, on a class the browser declares — and
// nothing else on this page can produce that string: Go's own
// `businesslogic.Money.Format` is reached only by `quoteFor`, which is a
// command and is not called while a page renders.
//
// So the price standing in the markup is proof that Go's cents were decoded
// into a Money inside the engine and asked to format themselves. Before the
// SSR bundle carried the app's transport, this line threw mid-render and the
// visitor got the shell.
func TestATransportedValueReachesTheEngineWithItsType(t *testing.T) {
	h := newProdHandler(t)

	body := get(t, h, "/pricing").Body.String()

	if !strings.Contains(body, `<p data-testid="featured">Startup — $45.00</p>`) {
		t.Error(`the document does not carry the featured plan formatted by Money.format(); the render did not see a Money`)
	}

	// And the same value reaches the browser the way it always did: as cents
	// under the transport key, for kit's client to decode. A document that
	// server-rendered the price by flattening the type would carry the
	// formatted string here instead, and the client would hydrate a plain
	// object over markup that claims a method ran.
	if !strings.Contains(body, `price:app.decode("Money", {cents:4500})`) {
		t.Error("the hydration array does not carry the price as a Money for the client to decode")
	}
	if strings.Contains(body, `price:"$45.00"`) {
		t.Error("the hydration array carries a formatted string where the client expects a Money")
	}
}

// TestARemoteAnswersTransportedValueIsRenderedByItsOwnMethod is the other path
// the same type takes into a document.
//
// The one above is a server load's value: a load always runs, so it travels
// down inside the document by construction. This one is a remote function's,
// called back out to Go from inside the engine while the page was being
// rendered — decoded there by the app's own `transport` hook before the line
// that formats it ran. 750 cents is an amount no other function in this app
// returns, and `Money.format()` is a method, so a page handed a plain object
// could not have written "$7.50".
func TestARemoteAnswersTransportedValueIsRenderedByItsOwnMethod(t *testing.T) {
	h := newProdHandler(t)

	body := get(t, h, "/pricing").Body.String()

	if !strings.Contains(body, `<p data-testid="spotlight">Student — $7.50</p>`) {
		t.Error(`the document does not carry the spotlight plan formatted by Money.format(); the render did not see a Money`)
	}
	if !strings.Contains(body, `price:app.decode("Money", {cents:750})`) {
		t.Error("the remote answer travelling with the document does not carry the price as a Money")
	}
	if strings.Contains(body, "skgo: implemented in Go") {
		t.Error("the generated stub answered, which means Go did not")
	}

	// The plans beside it are in a boundary with a `pending` snippet, and
	// Svelte's server compiler emits that snippet instead of the boundary's
	// children — so this is a claim about one boundary rendering during SSR,
	// not about the page as a whole.
	if !strings.Contains(body, `data-testid="plans-pending"`) {
		t.Error("the document does not carry the plans list as still loading")
	}
}

// TestALoadThatFailsInTheRootLayoutIsKitsStaticErrorPage is the one load
// failure no `+error.svelte` can catch.
//
// Kit wraps the root error page inside the root layout rather than the other
// way round (`runtime/error-chain.js`), so nothing is declared above node 0.
// Kit's answer to a failure there is `static_error_page`
// (`runtime/server/errors.js`): the `error.html` the build carries, with the
// status and the message substituted in, carrying no app markup and no script
// so that nothing boots and nothing tries again.
//
// src/routes/layout.server.go refuses with 503 when the URL says
// `boom=root-layout`, and for no other reason — the status and the sentence
// below are that function's, and no other function in this app produces either.
func TestALoadThatFailsInTheRootLayoutIsKitsStaticErrorPage(t *testing.T) {
	h := newProdHandler(t)

	rec := get(t, h, "/?boom=root-layout")
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("GET /?boom=root-layout: status %d, want 503", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `<span class="status">503</span>`) {
		t.Error("the document is not kit's static error page")
	}
	if !strings.Contains(body, "<h1>The root layout could not reach the database</h1>") {
		t.Error("the static error page does not carry the message the load refused with")
	}
	if strings.Contains(body, "<script") {
		t.Error("the static error page carries a script, so the app would boot and try again")
	}
	if strings.Contains(body, `data-testid="app-nav"`) {
		t.Error("the static error page carries the app's own layout, so an error page rendered instead")
	}

	// And the same URL without the trigger is the page it always was, so the
	// refusal is the query parameter's doing and not the load's normal state.
	fine := get(t, h, "/")
	if fine.Code != http.StatusOK {
		t.Errorf("GET /: status %d, want 200", fine.Code)
	}
	if !strings.Contains(fine.Body.String(), `<footer data-testid="deployment">skgo example</footer>`) {
		t.Error("the home page does not carry the root layout load's value")
	}
}
