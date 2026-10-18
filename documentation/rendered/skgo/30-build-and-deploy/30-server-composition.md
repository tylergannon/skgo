---
title: "Server composition"
summary: "How server.go assembles loads, remote functions, endpoints, actions and the renderer into one http.Handler."
---

A Kit adapter hides its server inside the build output. In SKGo the server is Go code you own: `skgo new` writes `server.go` at the root of your module, and it builds one `http.Handler` from the embedded build and the registries `skgo generate` emits. `cmd/main.go` serves that handler, and so do your tests.

## The two constructors

The generated file exports a production constructor and a development one that share every line of setup:

```go
// NewHandler builds the Go-owned application stack over the embedded build.
func NewHandler(dist fs.FS, origin string) (http.Handler, error) {
	return newHandler(dist, origin, nil)
}

// NewDevelopmentHandler keeps application behavior in Go while Kit renders pages.
func NewDevelopmentHandler(dist fs.FS, origin string, development *skgo.NativeDevelopment) (http.Handler, error) {
	return newHandler(dist, origin, development)
}
```

`dist` is the adapter's output directory, usually `fs.Sub(web.Build, "build")`. `origin` is the public origin the browser uses. [`skgo dev`](development-server) calls `NewDevelopmentHandler` with a `*skgo.NativeDevelopment` that hands page rendering to Vite; a nil one makes `development.NewSSR` and `development.NewStaticHandler` return the production renderer and static handler. Keep both constructors on one `newHandler`, so your loads, actions, remote functions and hooks are the same code in both modes.

## Wiring

The example app's `server.go` is the scaffold's composition with the optional hooks filled in:

```go
/// file: server.go
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
	"net/http"
	"strings"

	"github.com/tylergannon/skgo"
	generated "github.com/tylergannon/skgo/example/internal/skgo"
)

// SessionCookie is the cookie the session id travels in. This is the one place
// the app spells it: everything downstream reads the session, not the cookie.
const SessionCookie = "skgo_session"

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

// NewHandlerSized is NewHandler with the number of page renders that may run
// at once. Zero is the renderer's default, one per CPU.
func NewHandlerSized(dist fs.FS, origin string, runtimes int) (http.Handler, error) {
	return newHandler(dist, origin, runtimes, nil)
}
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
		ssr, err := development.NewSSR(dist, manifest, loads, remotes, skgo.SSROptions{
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
		return development.NewStaticHandler(dist, skgo.WithSSR(ssr))
	}

	// The `transport` hook, declared in web/src/hooks.go beside the
	// web/src/hooks.ts that holds its browser half. Both registries get it: a
	// custom type reaches the browser through a remote function and through a
	// server load alike.
	//
	remoteCfg.Transport = generated.Transport()
	loadCfg.Transport = generated.Transport()
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
	app = generated.RequestBoundary(handleCfg,
		loads.Intercept(remotes.Intercept(endpoints.Intercept(pages))))
	return skgo.FetchConfig{
		Origin:      origin,
		Base:        manifest.Base,
		Handler:     internal,
		HandleFetch: handleFetch,
		Prerendered: manifest.Prerendered,
	}.Intercept(app), nil
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
```

Reading it from the top:

- `skgo.ReadManifest` parses `skgo.manifest.json` from the build, and the `...Config` methods copy what each registry needs out of it. Until `vp build` has written a manifest, this is where startup fails.
- `generated` is your bindings package, `internal/skgo`. `Matchers`, `Actions`, `Remotes`, `Loads` and `Endpoints` return what `skgo generate` built from your route files; `Transport` exists only when `src/hooks.go` declares [`Transported`](transport#Transport) types.
- `internal` is a forward reference to the finished stack. Render-time `fetch` and `event.Fetch` from Go dispatch through it, in process, with no socket.
- `build` makes the renderer once the loads and remote functions exist, and hands its `error.html` to the endpoints and the `handle` hook so a fatal error outside a page still gets Kit's error page.
- The registries get their `HandleError`, the two that carry values to the browser get the `Transport`, and the renderer and the fetcher get the same `HandleFetch`.

## Request order

A request passes through the handlers from the outside in:

1. `skgo.FetchConfig.Intercept` installs `event.Fetch` on the request. It is outermost so that the `handle` hook's own event can fetch too.
2. `generated.RequestBoundary` runs your `handle` hook from `internal/serverhooks` and creates the typed `Locals`. `handleCfg.Static` names the requests served straight from files, which Kit's adapters answer before `handle` and which the hook therefore never sees. See [Hooks](hooks).
3. `loads.Intercept` answers `__data.json`.
4. `remotes.Intercept` answers remote function calls under `_app/remote/`.
5. `endpoints.Intercept` answers the `server.go` routes.
6. The page handler serves client files and prerendered pages, and renders everything else. A `POST` to a page runs its form action, which comes from `SSROptions.Actions`.

[`NewStaticHandler`](skgo-server#NewStaticHandler) lists the order in which the last step decides what a path is.

## The renderer

`skgo.NewSSR` runs the compiled Svelte server code in a pool of goja runtimes, one render per runtime at a time. [`SSROptions`](skgo-server#SSROptions) is where you tune it:

- `Runtimes` bounds how many pages render at once; zero means one per CPU. Each runtime holds its own copy of the app's module state, so this trades memory for throughput and nothing else.
- `Fetch` is the in-process handler for a render-time `fetch` of the app's own routes. Leaving it nil refuses every such fetch.
- `OnError` hears about failures the visitor is never shown; nil logs them. See [Errors](errors).
- `HandleFetch` is Kit's `handleFetch` hook, `Environment` is the snapshot [`LoadEnvironment`](environment-variables#Validation) returns, and `FilterSerializedResponseHeaders` chooses which response headers a universal load may read.

Module-level state in a Svelte file lives per runtime. Shared state belongs in Go; see [State management](state-management).

## cmd/main.go

The generated `main` is short. It reads two flags, embeds the build and serves `NewHandler` with `http.ListenAndServe`:

```go
var origin = "http://127.0.0.1:8080"

func main() {
	listen := flag.String("listen", hostPort(origin), "address to listen on")
	trusted := flag.String("origin", origin, "public browser origin")
	flag.Parse()

	dist, err := fs.Sub(web.Build, "build")
	if err != nil {
		log.Fatal(err)
	}
	handler, err := app.NewHandler(dist, *trusted)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("myapp: listening on http://%s", *listen)
	if err := http.ListenAndServe(*listen, handler); err != nil {
		log.Fatal(err)
	}
}
```

`origin` is set by `-ldflags "-X main.origin=…"` in the `build` recipe, so `-listen` defaults to the origin's host and port and `-origin` to the origin itself. There is no signal handling; [Deploying](deploying#Shutdown) adds it.

## Embedding in your own server

`NewHandler` returns an ordinary `http.Handler`. Mount it on a mux beside other Go handlers, or under your own `http.Server`:

```go
handler, err := app.NewHandler(dist, *trusted)
if err != nil {
	log.Fatal(err)
}

mux := http.NewServeMux()
mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
})
mux.Handle("/", handler)

log.Fatal(http.ListenAndServe(*listen, mux))
```

A route the mux claims never reaches SKGo, so `handle` does not run for it, and `event.Fetch` cannot reach it either: a fetch of `/healthz` from a load is dispatched through `internal`, which is the Kit app, not your mux.

Mount the handler at the root. To serve the app under a prefix, set Kit's [`paths.base`](https://svelte.dev/docs/kit/configuration#paths) and rebuild; the handler expects request paths that still carry the base, and a request outside it is a 404, so do not wrap it in `http.StripPrefix`.

### Middleware and the request context

Middleware that observes requests, for logging or metrics, wraps the returned handler like any other. Middleware that stores values in the request context needs one more thought: a subrequest made with `event.Fetch` starts from a fresh context that keeps cancellation, the trace span and baggage, the client address and the fetch depth, and nothing else, as Kit's `fork_state_for_subrequest` keeps its request facilities and drops the rest. A value added outside the stack reaches the page's request but not a route the page fetches in process. To see it on both, add it inside the stack, around the handler `internal` dispatches to:

```go
app = withStore(store, generated.RequestBoundary(handleCfg,
	loads.Intercept(remotes.Intercept(endpoints.Intercept(pages)))))
```

`withStore` is your own `func(*Store, http.Handler) http.Handler`. Per-visitor data belongs in `Locals`, which the `handle` hook fills afresh for every subrequest, as in Kit.
