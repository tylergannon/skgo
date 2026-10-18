---
title: "Hooks"
summary: "Write request hooks and reroute in Go, and pair Go and TypeScript transport hooks."
---

Kit's [hooks](https://svelte.dev/docs/kit/hooks) are app-wide functions the framework calls at fixed points. In SKGo the server hooks are Go, because Go answers every request. The universal hooks stay in `src/hooks.ts`, and Kit runs them as usual; SKGo adds a Go half for `transport` and a Go-backed `reroute`.

| Kit hook      | In SKGo                                                       |
| ------------- | ------------------------------------------------------------- |
| `handle`      | a Go function in your hook package                            |
| `handleFetch` | a Go function you pass to `server.go`                         |
| `handleError` | a Go function you pass to `server.go`; the client half is Kit |
| `init`        | not supported; initialise in `newHandler`                     |
| `reroute`     | a Go function on `HandleConfig`, with Kit's universal forwarding hook |
| `transport`   | `src/hooks.ts` plus a Go marker in `src/hooks.go`             |

`src/hooks.server.ts` is generated, and contains only `export {};`. `skgo generate` refuses to overwrite one you wrote, so move its logic to Go; see [Project structure](project-structure). SKGo does not generate or read `src/hooks.client.ts`; Kit's client hooks run in the browser as Kit documents them.

## handle

`handle` runs for every request that reaches the app, during prerendering too, before any load, action, endpoint or remote function. `skgo new` declares it in `internal/serverhooks/handle.go`:

```go
package serverhooks

import "example.com/myapp/internal/skgo/params"

var Handle params.Middleware
```

A nil `Handle` passes every request through. `internal/skgo/config.go` names the package with `--hook-package`, and `--hook-symbol` names the variable when it is not `Handle`; see [skgo generate](/docs/cli/generate). The generated `RequestBoundary` in `internal/skgo` wraps your app in it.

`params.Middleware` is Kit's `handle` with the event and `resolve` as arguments:

```go
func(ctx context.Context, event params.RequestEvent, resolve params.Resolve) (*http.Response, error)
```

The event is the one loads see: the URL, the matched route and its typed parameters, cookies, locals, and the request-kind flags `IsDataRequest`, `IsRemoteRequest` and `IsSubRequest`. Because a hook runs for every route, its parameters are the app-wide `params.Params`; see [Advanced routing](advanced-routing#Matching-Typed-parameter-values). The example's `visitSlug` reads the `slug` parameter that way:

```go
/// file: internal/serverhooks/handle.go
func visitSlug(p params.Params) string {
	if value, ok := p.Slug().(params.SlugParam_String); ok {
		return value.Value
	}
	return ""
}
```

Return without calling `resolve` to answer the request yourself, with `skgo.NewResponse` or a response you built. Return a `*skgo.Redirect` or an `*skgo.HTTPError` to refuse it, as Kit's `redirect()` and `error()` do. SKGo shapes the refusal for the request: a real redirect or error page for a document, the JSON Kit's client expects for a data request or a remote call. Headers the hook set are added only to a response `resolve` produced; its cookies are also added to a returned `*skgo.Redirect`, but not to a response the hook builds itself. See [Cookies](cookies#Where-cookies-can-be-written).

Call `resolve` at most once; a second call returns an error. It returns before the body is complete, so wrapping the response does not delay streaming.

Static files and prerendered pages are answered before `handle`, so the hook never sees them. A panic in the hook is recovered: the visitor gets a 500, and `HandleConfig.OnPanic` is told, or the panic is logged.

### Before-only hooks

Most hooks only need to run before the app. `params.Handle` takes the event and returns the event to forward, and its `Middleware` method turns it into a middleware. `params.Sequence` chains middleware as Kit's [`sequence`](https://svelte.dev/docs/kit/@sveltejs-kit-hooks#sequence) does. The example chains its before-only hook with three middleware:

```go
/// file: internal/serverhooks/handle.go
var Handle = params.Sequence(params.Handle(initialize).Middleware(), SerializedHeaders, PreloadFonts, VisitMiddleware)
```

The example application's `initialize` reads the session cookie once and refuses two requests on purpose:

```go
/// file: internal/serverhooks/handle.go
// Handle is the app's `handle` hook — the one place it decides who the caller
// is. It runs once per request, before any load or remote function, and puts
// the answer where all of them can read it with app.LocalsFrom.
//
// A guard is then a load that reads the session; see
// web/src/routes/account/layout.server.go, which turns a signed-out visitor
// away from every page under /account without any of those pages knowing.
func initialize(ctx context.Context, event params.RequestEvent) (params.RequestEvent, error) {
	request := event.Request()
	event.Locals.RootLayoutFailure = request.URL.Query().Get("boom") == "root-layout"
	event.Locals.ScriptSafeProof = request.URL.Query().Get("proof") == "script-safe"
	if request.Method == http.MethodPost && request.URL.Path == "/actions" {
		switch request.URL.Query().Get("hook") {
		case "sign-in":
			return event, &skgo.Redirect{Status: http.StatusSeeOther, Location: "/actions/signed-in?required=1"}
		case "forbidden":
			return event, skgo.Errorf(http.StatusForbidden, "Hook denied this edit")
		}
	}
	if run := request.URL.Query().Get("run"); run != "" && request.URL.Path != "/api/replay-count" {
		businesslogic.Replays.Record(run, request.Method+" "+request.URL.Path)
	}
	id, _ := event.Cookie(SessionCookie)
	event.Locals.Session = businesslogic.Default.Session(id)
	return event, nil
}
```

### locals

Kit's `event.locals` is a Go struct you own. `skgo new` declares it empty in `internal/app/locals.go`, and `--locals-package` points `skgo generate` at it:

```go
package app

type Locals struct{}
```

Every request gets a fresh `Locals`, and `event.Locals` is a pointer to it. Hooks set fields through it. Loads read the same pointer from their typed event, and any Go code holding the request's context can call the generated `app.LocalsFrom(ctx)`:

```go
/// file: src/routes/account/layout.server.go
// layoutLoad is the section's guard and its shared data.
//
// The session comes from the request's locals, which the app's one `handle`
// hook filled in; nothing here reads a cookie or knows how a session is
// spelled. Turning a signed-out visitor away here turns them away from every
// page below, so adding another one needs no new guard.
func layoutLoad(event LayoutRequestEvent) (LayoutData, error) {
	ctx := event.Context()
	session := app.LocalsFrom(ctx).Session
	if session.User == "" {
		return LayoutData{}, &skgo.Redirect{Status: 307, Location: "/"}
	}

	// The visitor can ask for this section's data again without navigating;
	// `invalidate('app:account')` in the browser is what makes kit's client ask
	// for this node and no other.
	event.Depends("app:account")

	return LayoutData{
		AccountUser:   session.User,
		AccountSerial: nextSerial(ctx),
	}, nil
}

var _ = skgo.Load(layoutLoad)
```

A hook must not replace `event.Locals` with `nil`; `resolve` refuses that. A fetch from Go back into the app gets new locals, as a Kit subrequest does. [Auth](auth) builds sessions this way.

### Resolve options

Pass a `skgo.ResolveOptions` as `resolve`'s last argument to change how this request's page is rendered. The fields are Kit's [resolve options](https://svelte.dev/docs/kit/hooks#Server-hooks-handle):

- `TransformPageChunk` rewrites the HTML document. It runs once, on the whole document, with `done` set to `true`; values the page streams afterwards are sent untransformed. It applies to pages, their error pages and the `ssr = false` shell, not to data, endpoint or remote responses.
- `FilterSerializedResponseHeaders` decides which headers of a universal load's `fetch` responses the hydration data carries. Without it, the renderer's `SSROptions.FilterSerializedResponseHeaders` applies, and without that, none.
- `Preload` decides which files the document preloads. By default JavaScript and CSS are preloaded and fonts are not; stylesheets are always linked.

The example passes a transform that marks the root element, and a filter that lets a few headers through, from a middleware that wraps the response:

```go
/// file: internal/serverhooks/handle.go
// markTransformed is the document transform of the /middleware pages and of
// /stream, whose deferred values must still reach kit's client behind a
// transformed shell: the one place a middleware chooses to rewrite the
// assembled document, here by marking the root element so a scenario can see
// kit's client hydrate a transformed page.
func markTransformed(_ context.Context, html string, _ bool) (string, error) {
	return strings.Replace(html, `<html lang="en">`, `<html lang="en" data-middleware-transformed="yes">`, 1), nil
}
```

```go
/// file: internal/serverhooks/handle.go
// SerializedHeaders is the app's choice of which headers a universal load's
// hydration data carries. It is made here, per request, rather than on the
// renderer, so the whole universal-fetch suite runs through the request-local
// path.
func SerializedHeaders(ctx context.Context, event params.RequestEvent, resolve params.Resolve) (*http.Response, error) {
	return resolve(ctx, event, skgo.ResolveOptions{FilterSerializedResponseHeaders: app.SerializedHeader})
}
```

```go
/// file: internal/serverhooks/handle.go
// VisitMiddleware is the part of the app's `handle` hook that wraps the
// response. For the /middleware pages it establishes a visit before anything
// answers — refreshing the visit cookie when it is missing or stale, so the
// load that runs next reads the token from the same cookie jar the visitor
// receives it from — and marks the response it gets back with what it
// observed on the matched event.
func VisitMiddleware(ctx context.Context, event params.RequestEvent, resolve params.Resolve) (*http.Response, error) {
	route := event.RouteID()
	if route == "/stream" {
		response, err := resolve(ctx, event, skgo.ResolveOptions{TransformPageChunk: markTransformed})
		if err != nil {
			return nil, err
		}
		response.Header.Set("X-Skgo-Middleware", route+" data="+strconv.FormatBool(event.IsDataRequest()))
		return response, nil
	}
	if route != "/middleware" && !strings.HasPrefix(route, "/middleware/") {
		return resolve(ctx, event)
	}
	token, _ := event.Cookie(businesslogic.VisitCookie)
	if !strings.HasPrefix(token, "visit-") {
		token = businesslogic.FreshVisitToken
		if err := event.SetCookie(businesslogic.VisitCookie, token, skgo.CookieOptions{}); err != nil {
			return nil, err
		}
	}
	event.Locals.Visit = businesslogic.Visit{
		Token: token, Route: route, Slug: visitSlug(event.Params), Data: event.IsDataRequest(),
	}
	response, err := resolve(ctx, event, skgo.ResolveOptions{TransformPageChunk: markTransformed})
	if err != nil {
		return nil, err
	}
	response.Header.Set("X-Skgo-Middleware", route+" data="+strconv.FormatBool(event.IsDataRequest()))
	return response, nil
}
```

`VisitMiddleware` also sets a cookie before it calls `resolve`, the way [Cookies](cookies) describes.

Under `params.Sequence`, options combine as Kit combines them: an inner hook's transform runs before an outer one's, and the outermost `FilterSerializedResponseHeaders` and `Preload` win.

## handleFetch

`handleFetch` changes or replaces a fetch the server makes on the page's behalf: `fetch` in a universal load during server rendering, and `event.Fetch` from Go. Its signature is `func(ctx context.Context, request *http.Request, next skgo.Fetch) (*http.Response, error)`, and `skgo.EventFrom(ctx)` is the originating request's event. The example rewrites a query parameter before dispatch and marks the response:

```go
/// file: server.go
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

The hook sees the request before SKGo adds the page's cookies and `Authorization` header to it. Calling `next` dispatches without re-entering the hook. Pass the function in both places in `server.go`: as `HandleFetch` on the `skgo.SSROptions` given to `NewSSR`, for render-time fetches, and on the `skgo.FetchConfig` that wraps the app, for `event.Fetch` from Go. Credentials follow Kit's [rules](https://svelte.dev/docs/kit/hooks#Server-hooks-handleFetch); [Cookies](cookies#Cookies-and-server-fetch) covers them. `handleFetch` does not run during prerendering ([#278](https://github.com/tylergannon/skgo/issues/278)).

## handleError


`handleError` decides what the error page and `page.error` receive. It is a Go function on the load, remote, endpoint and handle configurations in `server.go`; [Errors](errors#Where-handleError-runs) shows it and lists where it runs. In the browser, Kit's own `handleError` from `src/hooks.client.ts` applies.


## init

Kit's `init` hook is not supported. Initialise database clients and other shared state in `newHandler` in `server.go`, which both `NewHandler` and `NewDevelopmentHandler` call. The [development server](development-server) builds the app from `NewDevelopmentHandler` and never runs `main`, so `main` is not a place for application setup.

## reroute

```ts
// src/hooks.ts
export { reroute } from '@skgo/sveltekit-adapter/reroute';
```

Put the Go hook in `src/hooks.go` and configure the same function on your application's `HandleConfig`:

```go
/// file: src/hooks.go
// Reroute selects a route from Go while preserving the visitor's alias URL.
var Reroute skgo.Reroute = func(ctx context.Context, event skgo.RerouteEvent) (*string, error) {
	if !strings.HasPrefix(event.URL.Path, "/go-alias/") {
		return nil, nil
	}
	name := strings.TrimPrefix(event.URL.Path, "/go-alias/")
	request, err := http.NewRequestWithContext(ctx, "GET", "/api/reroute-target?name="+name, nil)
	if err != nil {
		return nil, err
	}
	response, err := event.Fetch(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	path := string(body)
	return &path, nil
}
```

```go
/// file: server.go
handleCfg.Reroute = hooks.Reroute
```

Return a pointer to the pathname Kit should match, or `nil` to keep the incoming pathname. The selected route and parameters change; the original `event.url`, request URL and browser address stay intact. The example fetches its destination from a Go endpoint, then reaches a typed Go load or another Go endpoint through the alias. The generator includes an exported `Reroute` from `src/hooks.go` in the build service as well.

The hook receives a URL copy and a request-aware `Fetch`. It runs before `handle`, so its fetch hook has no selected route, parameters or initialized application locals. You can wait for Go work and use the request context for cancellation. A returned error or panic answers with Kit's opaque `Internal Server Error` response before `handle` runs.

Keep the hook pure and idempotent. Kit caches each browser result by full URL and owns that cache; server requests run the Go function independently. The universal TypeScript export only transports calls to Go, so the application logic and data access stay in Go in development and production.

## transport

Kit's [`transport`](https://svelte.dev/docs/kit/hooks#Universal-hooks-transport) hook stays in `src/hooks.ts`. Each key also needs a `skgo.Transported` marker in `src/hooks.go` naming the Go type that travels under it; [Transport](transport#Transport) shows both halves.

## Further reading

- [Auth](auth), for sessions built on `handle`, locals and cookies
- [skgo hooks](skgo-hooks), the reference for `Middleware`, `ResolveOptions`, `HandleError` and `HandleFetch`
- [Server composition](server-composition), for where each configuration in `server.go` is built
