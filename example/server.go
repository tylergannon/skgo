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
	"errors"
	"io/fs"
	"net/http"
	"strings"

	"github.com/tylergannon/skgo"
	generated "github.com/tylergannon/skgo/example/internal/skgo"
	hooks "github.com/tylergannon/skgo/example/web/src"
)

// SessionCookie is the cookie the session id travels in. This is the one place
// the app spells it: everything downstream reads the session, not the cookie.
const SessionCookie = "skgo_session"

// supportID is the fixture value HandleError adds to every failure. It is a
// literal on purpose, the same way businesslogic.Default's fixtures are:
// something the Gherkin suite and the Go tests can both assert against
// without asking the app what it just rendered.
const supportID = "case-1121"

// docs: handle-error
// HandleError is the app's `handleError` hook — kit's own contract, mirrored:
// it runs for page and remote-function failures, expected or not
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
	if caught.Kind == "validation" && strings.HasSuffix(skgo.EventFrom(ctx).Request().URL.Path, "/validateCatalog") {
		return map[string]any{"message": "Choose a catalog name.", "reference": "catalog-validation-166"}
	}
	if caught.Kind == "unknown" {
		extra["message"] = "Something went wrong on our end."
	}
	return extra
}

// docs: end handle-error

// docs: new-handler
// NewHandler builds the app's server over the embedded production build in dist.
//
// The order is kit's own dispatch order, turned inside out into middleware.
// Handle is outermost because kit runs `handle` before it dispatches to
// anything, for every kind of request — that is not the loads registry's
// business even though this app also has one. Under it, the loads registry:
// `__data.json` must never reach the static handler, which would answer it
// with the boot document. Then remote functions, which live under the app
// directory and are not routes at all. Then the server routes, which own
// every path kit compiled a `+server.ts` for. Pages are last,
// because in kit they are what answers when nothing else did.
func NewHandler(dist fs.FS, origin string) (http.Handler, error) {
	return newHandler(dist, origin, 0, nil)
}

// NewDevelopmentHandler uses the same application stack with Kit-owned rendering.
func NewDevelopmentHandler(dist fs.FS, origin string, development *skgo.NativeDevelopment) (http.Handler, error) {
	return newHandler(dist, origin, 0, development)
}

// docs: end new-handler

// docs: new-handler-sized
// NewHandlerSized is NewHandler with the number of page renders that may run
// at once. Zero is the renderer's default, one per CPU.
func NewHandlerSized(dist fs.FS, origin string, runtimes int) (http.Handler, error) {
	return newHandler(dist, origin, runtimes, nil)
}

// docs: end new-handler-sized

func newHandler(dist fs.FS, origin string, runtimes int, development *skgo.NativeDevelopment) (http.Handler, error) {
	// The manifest defines the URL prefix remote calls arrive on.
	manifest, err := skgo.ReadManifest(dist)
	if err != nil {
		return nil, err
	}

	remoteCfg := manifest.RemoteConfig(origin)
	loadCfg := manifest.LoadConfig(origin)
	endpointCfg := manifest.EndpointConfig(origin)
	handleCfg := manifest.HandleConfig()
	// docs: reroute-config
	handleCfg.Reroute = hooks.Reroute
	// docs: end reroute-config
	loadCfg.Matchers = generated.Matchers()
	endpointCfg.Matchers = generated.Matchers()
	handleCfg.Matchers = generated.Matchers()
	handleCfg.Origin = origin

	var pages http.Handler
	// build makes the page handler once both registries exist: the renderer
	// needs the loads and the remote functions, and they need the manifest.
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
		return nil, err
	}
	build = func(loads *skgo.Loads, remotes *skgo.Remotes) (http.Handler, error) {
		// A render-time `event.fetch` of the app's own routes is answered
		// by the whole stack, pages included; see `app`.
		// docs: ssr-options
		ssr, err := development.NewSSR(dist, manifest, loads, remotes, skgo.SSROptions{
			Runtimes:    runtimes,
			Fetch:       internal,
			HandleFetch: handleFetch,
			Actions:     actions,
		})
		// docs: end ssr-options
		if err != nil {
			return nil, err
		}
		handleCfg.ErrorTemplate = ssr.ErrorTemplate()
		endpoints.SetErrorTemplate(ssr.ErrorTemplate())
		return development.NewStaticHandler(dist, skgo.WithSSR(ssr))
	}

	// The `transport` hook, declared in web/src/hooks.go beside the
	// web/src/hooks.ts that holds its browser half. Both registries get it: a
	// custom type reaches the browser through a remote function and through a
	// server load alike.
	//
	remoteCfg.Transport = generated.Transport()
	loadCfg.Transport = generated.Transport()
	remoteCfg.HandleError = HandleError
	loadCfg.HandleError = HandleError
	endpointCfg.HandleError = HandleError
	handleCfg.HandleError = HandleError

	remotes, err := skgo.NewRemotes(remoteCfg, generated.Remotes()...)
	if err != nil {
		return nil, err
	}
	loads, err := skgo.NewLoads(loadCfg, generated.Loads()...)
	if err != nil {
		return nil, err
	}
	endpoints, err = skgo.NewEndpoints(endpointCfg, generated.Endpoints()...)
	if err != nil {
		return nil, err
	}
	if pages, err = build(loads, remotes); err != nil {
		return nil, err
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
	// docs: compose
	app = generated.RequestBoundary(handleCfg,
		loads.Intercept(remotes.Intercept(endpoints.Intercept(pages))))
	// docs: end compose
	return skgo.FetchConfig{
		Origin:      origin,
		Base:        manifest.Base,
		Handler:     internal,
		HandleFetch: handleFetch,
		Prerendered: manifest.Prerendered,
	}.Intercept(app), nil
}

// docs: handle-fetch
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

// docs: end handle-fetch

// docs: client-address
// FlyClientAddress reads the client address from the header Fly's proxy sets.
// Install it with handleCfg.ClientAddress only when every request arrives
// through that proxy.
func FlyClientAddress(r *http.Request) (string, error) {
	if ip := r.Header.Get("Fly-Client-IP"); ip != "" {
		return ip, nil
	}
	return "", errors.New("no Fly-Client-IP header")
}

// docs: end client-address
