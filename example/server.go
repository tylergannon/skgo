// Package example assembles the demo app's server. It exists so that there is
// exactly one production stack: the binary in cmd and the tests beside this
// file both call NewHandler, and neither can be green over a composition the
// other does not use.
//
// The composition is load-bearing and was got wrong once: a test that built
// only `remotes.Intercept(static)` kept asserting that every `__data.json` is
// refused 404, which is what the static handler does on its own and the
// opposite of what the app does.
package example

import (
	"context"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/businesslogic"
	generated "github.com/tylergannon/skgo/example/internal/skgo"
)

// SessionCookie is the cookie the session id travels in. This is the one place
// the app spells it: everything downstream reads the session, not the cookie.
const SessionCookie = "skgo_session"

// Handle is the app's `handle` hook — the one place it decides who the caller
// is. It runs once per request, before any load or remote function, and puts
// the answer where all of them can read it with skgo.LocalOf.
//
// A guard is then a load that reads the session; see
// web/src/routes/account/layout.server.go, which turns a signed-out visitor
// away from every page under /account without any of those pages knowing.
func Handle(ctx context.Context) error {
	event := skgo.EventFrom(ctx)
	request := event.Request()
	if request.Method == http.MethodPost && request.URL.Path == "/actions" {
		switch request.URL.Query().Get("hook") {
		case "sign-in":
			return &skgo.Redirect{Status: http.StatusSeeOther, Location: "/actions/signed-in?required=1"}
		case "forbidden":
			return skgo.Errorf(http.StatusForbidden, "Hook denied this edit")
		}
	}
	if run := request.URL.Query().Get("run"); run != "" && request.URL.Path != "/api/replay-count" {
		businesslogic.Replays.Record(run, request.Method+" "+request.URL.Path)
	}
	id, _ := event.Cookie(SessionCookie)
	return skgo.SetLocal(ctx, businesslogic.Default.Session(id))
}

// VisitMiddleware is the part of the app's `handle` hook that wraps the
// response. For the /middleware pages it establishes a visit before anything
// answers — refreshing the visit cookie when it is missing or stale, so the
// load that runs next reads the token from the same cookie jar the visitor
// receives it from — and marks the response it gets back with what it
// observed on the matched event.
func VisitMiddleware(ctx context.Context, event *skgo.Event, resolve skgo.Resolve) (*http.Response, error) {
	route := event.RouteID()
	if route == "/stream" {
		response, err := resolve(ctx, skgo.ResolveOptions{TransformPageChunk: markTransformed})
		if err != nil {
			return nil, err
		}
		response.Header.Set("X-Skgo-Middleware", route+" data="+strconv.FormatBool(event.IsDataRequest()))
		return response, nil
	}
	if route != "/middleware" && !strings.HasPrefix(route, "/middleware/") {
		return resolve(ctx)
	}
	token, _ := event.Cookie(businesslogic.VisitCookie)
	if !strings.HasPrefix(token, "visit-") {
		token = businesslogic.FreshVisitToken
		if err := event.SetCookie(businesslogic.VisitCookie, token, skgo.CookieOptions{}); err != nil {
			return nil, err
		}
	}
	err := skgo.SetLocal(ctx, businesslogic.Visit{
		Token: token, Route: route, Slug: event.Params()["slug"], Data: event.IsDataRequest(),
	})
	if err != nil {
		return nil, err
	}
	response, err := resolve(ctx, skgo.ResolveOptions{TransformPageChunk: markTransformed})
	if err != nil {
		return nil, err
	}
	response.Header.Set("X-Skgo-Middleware", route+" data="+strconv.FormatBool(event.IsDataRequest()))
	return response, nil
}

// markTransformed is the document transform of the /middleware pages and of
// /stream, whose deferred values must still reach kit's client behind a
// transformed shell: the one place a middleware chooses to rewrite the
// assembled document, here by marking the root element so a scenario can see
// kit's client hydrate a transformed page.
func markTransformed(_ context.Context, html string, _ bool) (string, error) {
	return strings.Replace(html, `<html lang="en">`, `<html lang="en" data-middleware-transformed="yes">`, 1), nil
}

// SerializedHeaders is the app's choice of which headers a universal load's
// hydration data carries. It is made here, per request, rather than on the
// renderer, so the whole universal-fetch suite runs through the request-local
// path.
func SerializedHeaders(ctx context.Context, _ *skgo.Event, resolve skgo.Resolve) (*http.Response, error) {
	return resolve(ctx, skgo.ResolveOptions{FilterSerializedResponseHeaders: filterSerializedResponseHeaders})
}

// supportID is the fixture value HandleError adds to every failure. It is a
// literal on purpose, the same way businesslogic.Default's fixtures are:
// something the Gherkin suite and the Go tests can both assert against
// without asking the app what it just rendered.
const supportID = "case-1121"

// HandleError is the app's `handleError` hook — kit's own contract, mirrored:
// it runs for every error a page render raises, expected or not
// (`exports/hooks/public.d.ts`: "runs for every error thrown ... except
// redirects"), and whatever it returns is merged over what the error already
// carries.
//
// Every failure gets a support id, which is the one thing a real error page
// almost always adds and kit's own default has no room for. An error nobody
// meant to happen also loses its own message here: this app makes no promise
// about what an arbitrary Go error's text might contain, so the visitor is
// told something a person wrote instead of it. caught.Err still carries the
// real one, for a hook that wants to log it before replacing it.
func HandleError(ctx context.Context, caught skgo.CaughtError) map[string]any {
	extra := map[string]any{"supportId": supportID}
	if caught.Kind == "unknown" {
		extra["message"] = "Something went wrong on our end."
	}
	return extra
}

// NewHandler builds the app's server over the build in dist. With a non-empty
// proxy it forwards pages to a running `vp dev` server; otherwise it serves the
// embedded build.
//
// The order is kit's own dispatch order, turned inside out into middleware.
// Handle is outermost because kit runs `handle` before it dispatches to
// anything, for every kind of request — that is not the loads registry's
// business even though this app also has one. Under it, the loads registry:
// `__data.json` must never reach the static handler, which would answer it
// with the boot document. Then remote functions, which live under the app
// directory and are not routes at all. Then the server routes, which own
// every path kit compiled a `+server.ts` for — in dev too, where kit's own
// server would otherwise run the generated stub and throw. Pages are last,
// because in kit they are what answers when nothing else did.
func NewHandler(dist fs.FS, proxy, origin string) (http.Handler, string, error) {
	return NewHandlerSized(dist, proxy, origin, 0)
}

// NewHandlerSized is NewHandler with the number of page renders that may run
// at once. Zero is the renderer's default, one per CPU.
func NewHandlerSized(dist fs.FS, proxy, origin string, runtimes int) (http.Handler, string, error) {
	// The manifest is read in both modes: it is where appDir and base come
	// from, and those decide the URL prefix remote calls arrive on.
	manifest, err := skgo.ReadManifest(dist)
	if err != nil {
		return nil, "", err
	}
	if proxy != "" {
		manifest, err = skgo.ReadDevManifest(dist, proxy)
		if err != nil {
			return nil, "", err
		}
	}

	remoteCfg := manifest.RemoteConfig(origin)
	loadCfg := manifest.LoadConfig(origin)
	endpointCfg := manifest.EndpointConfig(origin)
	handleCfg := manifest.HandleConfig()
	handleCfg.Origin = origin

	mode := "prod"
	var pages http.Handler
	// build makes the page handler once both registries exist: the renderer
	// needs the loads and the remote functions, and they need the manifest.
	// Both modes render, so both go through it.
	var build func(*skgo.Loads, *skgo.Remotes) (http.Handler, error)
	// Declared here, ahead of the closure below that reaches into it: the
	// closure runs after NewEndpoints has filled this in, but a closure
	// captures the variable itself and Go resolves the name when the literal
	// is written, not when it is called.
	var endpoints *skgo.Endpoints
	// app is the whole stack below FetchConfig, and what `event.fetch` answers
	// from: a subrequest reaches every page, data route, remote function,
	// server route, asset and prerendered file the visitor's own request would.
	// It is assigned last, after the renderer that fetches through it exists.
	var app http.Handler
	internal := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { app.ServeHTTP(w, r) })
	actions, err := skgo.NewActions(generated.Actions()...)
	if err != nil {
		return nil, "", err
	}
	if proxy != "" {
		target, err := url.Parse(proxy)
		if err != nil {
			return nil, "", err
		}
		// In dev the client is served by vite, not by this build, so its
		// baked version differs from the manifest's — sending
		// x-sveltekit-version would make it reload in a loop. Kit skips the
		// remote CSRF check in dev too.
		remoteCfg = manifest.RemoteConfig("")
		remoteCfg.Version = ""
		remoteCfg.Dev = true
		remoteCfg.CookieOrigin = origin
		loadCfg.Version = ""
		loadCfg.Dev = true
		endpointCfg.Dev = true
		handleCfg.Version = ""
		handleCfg.Dev = true
		mode = "dev"
		// Go renders the document in dev too. `vp dev` never runs an adapter,
		// so there is no bundle here; the engine pulls one module at a time
		// out of the same `goja` environment the build compiles, which the
		// adapter also declares in the dev server. Everything that is not a
		// document — modules, their CSS, the files in static/, the HMR socket —
		// still goes through to vite, so the browser only ever talks to Go.
		build = func(loads *skgo.Loads, remotes *skgo.Remotes) (http.Handler, error) {
			ssr, err := skgo.NewDevSSR(dist, manifest, loads, remotes, proxy, skgo.SSROptions{
				Runtimes:    runtimes,
				Fetch:       internal,
				HandleFetch: handleFetch,
				Actions:     actions,
			})
			if err != nil {
				return nil, err
			}
			handleCfg.ErrorTemplate = ssr.ErrorTemplate()
			endpoints.SetErrorTemplate(ssr.ErrorTemplate())
			return skgo.NewDevPages(target, manifest, ssr, log.Printf, endpoints), nil
		}
	} else {
		build = func(loads *skgo.Loads, remotes *skgo.Remotes) (http.Handler, error) {
			// A render-time `event.fetch` of the app's own routes is answered
			// by the whole stack, pages included; see `app`.
			ssr, err := skgo.NewSSR(dist, manifest, loads, remotes, skgo.SSROptions{
				Runtimes:    runtimes,
				Fetch:       internal,
				HandleFetch: handleFetch,
				Actions:     actions,
			})
			if err != nil {
				return nil, err
			}
			handleCfg.ErrorTemplate = ssr.ErrorTemplate()
			endpoints.SetErrorTemplate(ssr.ErrorTemplate())
			return skgo.NewStaticHandler(dist, skgo.WithSSR(ssr))
		}
	}

	// The `transport` hook, declared in web/src/hooks.go beside the
	// web/src/hooks.ts that holds its browser half. Both registries get it: a
	// custom type reaches the browser through a remote function and through a
	// server load alike.
	//
	// After the branch above, not before it: the dev arm replaces remoteCfg
	// wholesale with a fresh manifest.RemoteConfig, so anything set earlier is
	// dropped. Setting it above cost a run — the suite was green in prod and
	// the pricing page threw `price.format is not a function` in dev.
	remoteCfg.Transport = generated.Transport()
	loadCfg.Transport = generated.Transport()
	loadCfg.HandleError = HandleError
	endpointCfg.HandleError = HandleError
	handleCfg.HandleError = HandleError

	remotes, err := skgo.NewRemotes(remoteCfg, generated.Remotes()...)
	if err != nil {
		return nil, "", err
	}
	loads, err := skgo.NewLoads(loadCfg, generated.Loads()...)
	if err != nil {
		return nil, "", err
	}
	endpoints, err = skgo.NewEndpoints(endpointCfg, generated.Endpoints()...)
	if err != nil {
		return nil, "", err
	}
	if pages, err = build(loads, remotes); err != nil {
		return nil, "", err
	}
	handleCfg.Loads = loads
	handleCfg.Static = skgo.ServedAsFile(pages)
	// Handle mounts outermost: kit runs `handle` before it dispatches to
	// anything, and that is true of every registry below, not just the loads
	// one that happens to also answer `__data.json`.
	//
	// Event.Fetch — what a Go load or endpoint calls to reach this app's own
	// routes — is mounted outside Handle so the hook's own event can fetch too.
	// It answers in-process through the same stack, and each subrequest runs
	// Handle again with fresh locals.
	app = skgo.Sequence(skgo.Handle(Handle).Middleware(), SerializedHeaders, VisitMiddleware).Intercept(handleCfg,
		loads.Intercept(remotes.Intercept(endpoints.Intercept(pages))))
	return skgo.FetchConfig{
		Origin:      origin,
		Base:        manifest.Base,
		Handler:     internal,
		HandleFetch: handleFetch,
		Prerendered: manifest.Prerendered,
	}.Intercept(app), mode, nil
}

// The example hook rewrites an API alias query before dispatch and marks the
// response. Fetching this API in a universal load still reuses its response
// during hydration; the hook itself only runs in Go.
func handleFetch(ctx context.Context, request *http.Request, next skgo.Fetch) (*http.Response, error) {
	request = request.Clone(ctx)
	if request.URL.Path == "/api/todos" {
		query := request.URL.Query()
		query.Del("via")
		request.URL.RawQuery = query.Encode()
	}
	if strings.HasPrefix(request.URL.Path, "/api/replay/") && request.URL.Query().Has("alias") {
		query := request.URL.Query()
		query.Del("alias")
		query.Set("step", "rewritten")
		request.URL.RawQuery = query.Encode()
	}
	response, err := next(request)
	if err == nil && strings.HasPrefix(request.URL.Path, "/api/") {
		response.Header.Set("X-Fetch-Hook", "Go")
	}
	return response, err
}

// The headers a universal fetch's replay may carry into the document. The
// answer depends on the value as well as the name, as Kit's own contract
// allows: of the cookies an answer sets, only the one named lamp is replayed.
func filterSerializedResponseHeaders(name, value string) bool {
	switch name {
	case "x-fetch-hook", "x-replay-allowed":
		return true
	case "set-cookie":
		return strings.HasPrefix(value, "lamp=")
	}
	return false
}
