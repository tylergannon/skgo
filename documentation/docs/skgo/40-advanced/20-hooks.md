---
title: Hooks
summary: Write handle, handleFetch and handleError in Go, and pair Go and TypeScript transport hooks.
proof:
  - example/e2e/features/middleware.feature
  - TestMiddlewareRunsBeforeResolveAndAfterInOrder
  - TestMiddlewareRefusalsKeepKitsShapesPerRequestKind
  - TestMiddlewareStaysOutOfStaticFilesAndTheBundle
  - TestSequenceOptionsComposeInnerTransformBeforeOuter
  - TestAPanickingHandleHookAnswersInsteadOfResettingTheConnection
---

Kit's [hooks](https://svelte.dev/docs/kit/hooks) are app-wide functions the framework calls at fixed points. In SKGo the server hooks are Go, because Go answers every request. The universal hooks stay in `src/hooks.ts`, and Kit runs them as usual; SKGo adds a Go half for `transport`.

| Kit hook      | In SKGo                                                       |
| ------------- | ------------------------------------------------------------- |
| `handle`      | a Go function in your hook package                            |
| `handleFetch` | a Go function you pass to `server.go`                         |
| `handleError` | a Go function you pass to `server.go`; the client half is Kit |
| `init`        | not supported; initialise in `newHandler`                     |
| `reroute`     | not supported                                                 |
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

The event is the one loads see: the URL, the matched route and its typed parameters, cookies, locals, and the request-kind flags `IsDataRequest`, `IsRemoteRequest` and `IsSubRequest`. Because a hook runs for every route, its parameters are the app-wide `params.Params`; see [Advanced routing](advanced-routing#Matching-Typed-parameter-values).

Return without calling `resolve` to answer the request yourself, with `skgo.NewResponse` or a response you built. Return a `*skgo.Redirect` or an `*skgo.HTTPError` to refuse it, as Kit's `redirect()` and `error()` do. SKGo shapes the refusal for the request: a real redirect or error page for a document, the JSON Kit's client expects for a data request or a remote call. Headers the hook set are added only to a response `resolve` produced; its cookies are also added to a returned `*skgo.Redirect`, but not to a response the hook builds itself. See [Cookies](cookies#Where-cookies-can-be-written).

Call `resolve` at most once; a second call returns an error. It returns before the body is complete, so wrapping the response does not delay streaming.

Static files and prerendered pages are answered before `handle`, so the hook never sees them. A panic in the hook is recovered: the visitor gets a 500, and `HandleConfig.OnPanic` is told, or the panic is logged.

### Before-only hooks

Most hooks only need to run before the app. `params.Handle` takes the event and returns the event to forward, and its `Middleware` method turns it into a middleware. `params.Sequence` chains middleware as Kit's [`sequence`](https://svelte.dev/docs/kit/@sveltejs-kit-hooks#sequence) does:

```go
var Handle = params.Sequence(params.Handle(initialize).Middleware(), VisitMiddleware)
```

The example application's `initialize` reads the session cookie once and refuses two requests on purpose:

```go
/// file: internal/serverhooks/handle.go
/// include: example/internal/serverhooks/handle.go#initialize
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
/// include: example/web/src/routes/account/layout.server.go#guard
```

A hook must not replace `event.Locals` with `nil`; `resolve` refuses that. A fetch from Go back into the app gets new locals, as a Kit subrequest does. [Auth](auth) builds sessions this way.

### Resolve options

Pass a `skgo.ResolveOptions` as `resolve`'s last argument to change how this request's page is rendered. The fields are Kit's [resolve options](https://svelte.dev/docs/kit/hooks#Server-hooks-handle):

- `TransformPageChunk` rewrites the HTML document. It runs once, on the whole document, with `done` set to `true`; values the page streams afterwards are sent untransformed. It applies to pages, their error pages and the `ssr = false` shell, not to data, endpoint or remote responses.
- `FilterSerializedResponseHeaders` decides which headers of a universal load's `fetch` responses the hydration data carries. Without it, the renderer's `SSROptions.FilterSerializedResponseHeaders` applies, and without that, none.
- `Preload` decides which files the document preloads. By default JavaScript and CSS are preloaded and fonts are not; stylesheets are always linked.

```go
response, err := resolve(ctx, event, skgo.ResolveOptions{
	TransformPageChunk: func(ctx context.Context, html string, done bool) (string, error) {
		return strings.Replace(html, "old", "new", 1), nil
	},
})
```

Under `params.Sequence`, options combine as Kit combines them: an inner hook's transform runs before an outer one's, and the outermost `FilterSerializedResponseHeaders` and `Preload` win.

## handleFetch

`handleFetch` changes or replaces a fetch the server makes on the page's behalf: `fetch` in a universal load during server rendering, and `event.Fetch` from Go. Its signature is `func(ctx context.Context, request *http.Request, next skgo.Fetch) (*http.Response, error)`, and `skgo.EventFrom(ctx)` is the originating request's event:

```go
func handleFetch(ctx context.Context, request *http.Request, next skgo.Fetch) (*http.Response, error) {
	if request.URL.Host == "api.yourapp.com" {
		request = request.Clone(ctx)
		request.URL.Scheme, request.URL.Host = "http", "localhost:9999"
	}
	return next(request)
}
```

The hook sees the request before SKGo adds the page's cookies and `Authorization` header to it. Calling `next` dispatches without re-entering the hook. Pass the function in both places in `server.go`: as `HandleFetch` on the `skgo.SSROptions` given to `NewSSR`, for render-time fetches, and on the `skgo.FetchConfig` that wraps the app, for `event.Fetch` from Go. Credentials follow Kit's [rules](https://svelte.dev/docs/kit/hooks#Server-hooks-handleFetch); [Cookies](cookies#Cookies-and-server-fetch) covers them. `handleFetch` does not run during prerendering ([#278](https://github.com/tylergannon/skgo/issues/278)).

## handleError

`handleError` decides what the error page and `page.error` receive. It is a Go function on the load, endpoint and handle configurations in `server.go`; [Errors](errors#Where-handleError-runs) shows it and lists where it runs. In the browser, Kit's own `handleError` from `src/hooks.client.ts` applies.

## init

Kit's `init` hook is not supported. Initialise database clients and other shared state in `newHandler` in `server.go`, which both `NewHandler` and `NewDevelopmentHandler` call. The [development server](development-server) builds the app from `NewDevelopmentHandler` and never runs `main`, so `main` is not a place for application setup.

## reroute

`reroute` is not supported. Go matches routes on the server without it, so a `reroute` export in `src/hooks.ts` would make the browser and the server disagree about which route a URL belongs to ([#167](https://github.com/tylergannon/skgo/issues/167)).

## transport

Kit's [`transport`](https://svelte.dev/docs/kit/hooks#Universal-hooks-transport) hook stays in `src/hooks.ts`. Each key also needs a `skgo.Transported` marker in `src/hooks.go` naming the Go type that travels under it; [Transport](transport#Transport) shows both halves.

## Further reading

- [Auth](auth), for sessions built on `handle`, locals and cookies
- [skgo hooks](skgo-hooks), the reference for `Middleware`, `ResolveOptions`, `HandleError` and `HandleFetch`
- [Server composition](server-composition), for where each configuration in `server.go` is built
