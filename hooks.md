---
title: Hooks
description: Write request hooks and reroute in Go, and pair Go and TypeScript transport hooks.
division: skgo
section: Advanced
order: 4020
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

`src/hooks.server.ts` is generated, and contains only `export {};`. `skgo generate` refuses to overwrite one you wrote, so move its logic to Go; see [Project structure](/docs/skgo/project-structure). SKGo does not generate or read `src/hooks.client.ts`; Kit's client hooks run in the browser as Kit documents them.

## handle

`handle` runs for every request that reaches the app, during prerendering too, before any load, action, endpoint or remote function. `skgo new` declares it in `internal/serverhooks/handle.go`:

```go internal/newapp/gofiles/internal/serverhooks/handle.go.tmpl
```

A nil `Handle` passes every request through. `internal/skgo/config.go` names the package with `--hook-package`, and `--hook-symbol` names the variable when it is not `Handle`; see [skgo generate](/docs/cli/generate). The generated `RequestBoundary` in `internal/skgo` wraps your app in it.

`params.Middleware` is Kit's `handle` with the event and `resolve` as arguments: it takes a `context.Context`, a `params.RequestEvent` and a `params.Resolve`, and returns an `*http.Response` and an `error`.

The event is the one loads see: the URL, the matched route and its typed parameters, cookies, locals, and the request-kind flags `IsDataRequest`, `IsRemoteRequest` and `IsSubRequest`. Because a hook runs for every route, its parameters are the app-wide `params.Params`; see [Advanced routing](/docs/skgo/advanced-routing#Matching-Typed-parameter-values). The example application's hook package is below, and its `visitSlug` reads the `slug` parameter that way:

```go example/internal/serverhooks/handle.go
```

Return without calling `resolve` to answer the request yourself, with `skgo.NewResponse` or a response you built. Return a `*skgo.Redirect` or an `*skgo.HTTPError` to refuse it, as Kit's `redirect()` and `error()` do. SKGo shapes the refusal for the request: a real redirect or error page for a document, the JSON Kit's client expects for a data request or a remote call. Headers the hook set are added only to a response `resolve` produced; its cookies are also added to a returned `*skgo.Redirect`, but not to a response the hook builds itself. See [Cookies](/docs/skgo/cookies#Where-cookies-can-be-written).

Call `resolve` at most once; a second call returns an error. It returns before the body is complete, so wrapping the response does not delay streaming.

Static files and prerendered pages are answered before `handle`, so the hook never sees them. A panic in the hook is recovered: the visitor gets a 500, and `HandleConfig.OnPanic` is told, or the panic is logged.

### Before-only hooks

Most hooks only need to run before the app. `params.Handle` takes the event and returns the event to forward, and its `Middleware` method turns it into a middleware. `params.Sequence` chains middleware as Kit's [`sequence`](https://svelte.dev/docs/kit/@sveltejs-kit-hooks#sequence) does. The example's `Handle`, in the file above, chains its before-only hook with three middleware. Its `initialize` reads the session cookie once and refuses two requests on purpose.

### locals

Kit's `event.locals` is a Go struct you own. `skgo new` declares it empty in `internal/app/locals.go`, and `--locals-package` points `skgo generate` at it:

```go internal/newapp/gofiles/internal/app/locals.go.tmpl
```

Every request gets a fresh `Locals`, and `event.Locals` is a pointer to it. Hooks set fields through it. Loads read the same pointer from their typed event, and any Go code holding the request's context can call the generated `app.LocalsFrom(ctx)`, as the example's account guard does:

```go example/web/src/routes/account/layout.server.go
```

A hook must not replace `event.Locals` with `nil`; `resolve` refuses that. A fetch from Go back into the app gets new locals, as a Kit subrequest does. [Auth](/docs/skgo/auth) builds sessions this way.

### Resolve options

Pass a `skgo.ResolveOptions` as `resolve`'s last argument to change how this request's page is rendered. The fields are Kit's [resolve options](https://svelte.dev/docs/kit/hooks#Server-hooks-handle):

- `TransformPageChunk` rewrites the HTML document. It runs once, on the whole document, with `done` set to `true`; values the page streams afterwards are sent untransformed. It applies to pages, their error pages and the `ssr = false` shell, not to data, endpoint or remote responses.
- `FilterSerializedResponseHeaders` decides which headers of a universal load's `fetch` responses the hydration data carries. Without it, the renderer's `SSROptions.FilterSerializedResponseHeaders` applies, and without that, none.
- `Preload` decides which files the document preloads. By default JavaScript and CSS are preloaded and fonts are not; stylesheets are always linked.

In the example's hook package above, `VisitMiddleware`, a middleware that wraps the response, passes `markTransformed`, a transform that marks the root element, and `SerializedHeaders` passes a filter that lets a few headers through. `VisitMiddleware` also sets a cookie before it calls `resolve`, the way [Cookies](/docs/skgo/cookies) describes.

Under `params.Sequence`, options combine as Kit combines them: an inner hook's transform runs before an outer one's, and the outermost `FilterSerializedResponseHeaders` and `Preload` win.

## handleFetch

`handleFetch` changes or replaces a fetch the server makes on the page's behalf: `fetch` in a universal load during server rendering, and `event.Fetch` from Go. Its signature is `func(ctx context.Context, request *http.Request, next skgo.Fetch) (*http.Response, error)`, and `skgo.EventFrom(ctx)` is the originating request's event. The example's `handleFetch`, in its application configuration, rewrites a query parameter before dispatch and marks the response:

```go example/internal/app/config.go
```

The hook sees the request before SKGo adds the page's cookies and `Authorization` header to it. Calling `next` dispatches without re-entering the hook. Pass the function in both places in `server.go`: as `HandleFetch` on the `skgo.SSROptions` given to `NewSSR`, for render-time fetches, and on the `skgo.FetchConfig` that wraps the app, for `event.Fetch` from Go. Credentials follow Kit's [rules](https://svelte.dev/docs/kit/hooks#Server-hooks-handleFetch); [Cookies](/docs/skgo/cookies#Cookies-and-server-fetch) covers them. Select the same function through [shared application configuration](/docs/skgo/prerendering#Shared-application-configuration) to run it during prerendering too.

## handleError

`handleError` decides what the error page and `page.error` receive. It is a Go function on the load, remote, endpoint and handle configurations in `server.go`; [Errors](/docs/skgo/errors#Where-handleError-runs) shows it and lists where it runs. In the browser, Kit's own `handleError` from `src/hooks.client.ts` applies.

## init

Kit's `init` hook is not supported. Initialise database clients and other shared state in `newHandler` in `server.go`, which both `NewHandler` and `NewDevelopmentHandler` call. The [development server](/docs/skgo/development-server) builds the app from `NewDevelopmentHandler` and never runs `main`, so `main` is not a place for application setup.

## reroute

Export the adapter's universal `reroute` from `src/hooks.ts`, as the last line of the example's does:

```ts example/web/src/hooks.ts
```

Put the Go hook in `src/hooks.go` and configure the same function on your application's `HandleConfig`:

```go example/web/src/hooks.go
```

The example's `newHandler` in `server.go` assigns `hooks.Reroute` to `handleCfg.Reroute`.

Return a pointer to the pathname Kit should match, or `nil` to keep the incoming pathname. The selected route and parameters change; the original `event.url`, request URL and browser address stay intact. The example fetches its destination from a Go endpoint, then reaches a typed Go load or another Go endpoint through the alias. The generator includes an exported `Reroute` from `src/hooks.go` in the build service as well.

The hook receives a URL copy and a request-aware `Fetch`. It runs before `handle`, so its fetch hook has no selected route, parameters or initialized application locals. You can wait for Go work and use the request context for cancellation. A returned error or panic answers with Kit's opaque `Internal Server Error` response before `handle` runs.

Keep the hook pure and idempotent. Kit caches each browser result by full URL and owns that cache; server requests run the Go function independently. The universal TypeScript export only transports calls to Go, so the application logic and data access stay in Go in development and production.

## transport

Kit's [`transport`](https://svelte.dev/docs/kit/hooks#Universal-hooks-transport) hook stays in `src/hooks.ts`. Each key also needs a `skgo.Transported` marker in `src/hooks.go` naming the Go type that travels under it; [Transport](/docs/skgo/transport#Transport) shows both halves.

## Further reading

- [Auth](/docs/skgo/auth), for sessions built on `handle`, locals and cookies
- [skgo reference](/docs/reference/skgo), for `Middleware`, `ResolveOptions`, `HandleError` and `HandleFetch`
- [Server composition](/docs/skgo/server-composition), for where each configuration in `server.go` is built
