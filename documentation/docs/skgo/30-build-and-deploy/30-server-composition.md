---
title: Server composition
summary: How server.go assembles loads, remote functions, endpoints, actions and the renderer into one http.Handler.
proof:
  - TestEveryRouteInTheManifestIsServed
  - TestEveryDataURLIsAnsweredByGoAndNeverByTheDocument
  - TestAFatalServerRouteUsesTheAppsErrorTemplate
---

A Kit adapter hides its server inside the build output. In SKGo the server is Go code you own: `skgo new` writes `server.go` at the root of your module, and it builds one `http.Handler` from the embedded build and the registries `skgo generate` emits. `cmd/main.go` serves that handler, and so do your tests.

## The two constructors

The generated file exports a production constructor and a development one that share every line of setup:

```go
/// file: server.go
/// include: example/server.go#new-handler
```

`NewHandlerSized` is `NewHandler` with the number of page renders that may run at once; zero is the renderer's default, one per CPU. A native host, which shares a phone with the rest of the app, uses it to keep the pool small:

```go
/// file: server.go
/// include: example/server.go#new-handler-sized
```

`dist` is the adapter's output directory, usually `fs.Sub(web.Build, "build")`. `origin` is the public origin the browser uses. [`skgo dev`](development-server) calls `NewDevelopmentHandler` with a `*skgo.NativeDevelopment` that hands page rendering to Vite; a nil one makes `development.NewSSR` and `development.NewStaticHandler` return the production renderer and static handler. Keep both constructors on one `newHandler`, so your loads, actions, remote functions and hooks are the same code in both modes.

## Wiring

The example app's `server.go` is the scaffold's composition with the optional hooks filled in:

```go
/// file: server.go
/// include: example/server.go
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

`skgo.NewSSR` runs the compiled Svelte server code in a pool of goja runtimes, one render per runtime at a time. [`SSROptions`](skgo-server#SSROptions) is where you tune it. The example sets four of its fields:

```go
/// file: server.go
/// include: example/server.go#ssr-options
```

- `Init` runs `initServer` once before this application instance can serve any request. A failure stops construction; it also runs under native development and when page rendering is disabled.
- `Runtimes` bounds how many pages render at once; zero means one per CPU. Each runtime holds its own copy of the app's module state, so this trades memory for throughput and nothing else.
- `Fetch` is the in-process handler for a render-time `fetch` of the app's own routes. Leaving it nil refuses every such fetch.
- `OnError` hears about failures the visitor is never shown; nil logs them. See [Errors](errors).
- `HandleFetch` is Kit's `handleFetch` hook, `Environment` is the snapshot [`LoadEnvironment`](environment-variables#Validation) returns, and `FilterSerializedResponseHeaders` chooses which response headers a universal load may read.

Module-level state in a Svelte file lives per runtime. Shared state belongs in Go; see [State management](state-management).

## cmd/main.go

The example's `main` reads its flags, embeds the build, serves `NewHandler` through a mux and drains in-flight requests on `SIGTERM`. The scaffolded `main` is shorter: it serves `NewHandler` with `http.ListenAndServe` and installs no signal handling.

```go
/// file: cmd/main.go
/// include: example/cmd/main.go#main
```

In the scaffold, `origin` is a package variable set by `-ldflags "-X main.origin=…"` in the `build` recipe, so `-listen` defaults to the origin's host and port and `-origin` to the origin itself. The example derives the origin from `-listen` when `-origin` is empty. [Deploying](deploying#Shutdown) covers the shutdown, and [Performance](performance#Diagnosing-issues) the `-pprof` flag.

## Embedding in your own server

`NewHandler` returns an ordinary `http.Handler`. Mount it on a mux beside other Go handlers, or under your own `http.Server`:

```go
/// file: cmd/main.go
/// include: example/cmd/main.go#mux
```

`withMode` is the example's own middleware, which stamps every response with the mode that served it. A route the mux claims never reaches SKGo, so `handle` does not run for it, and `event.Fetch` cannot reach it either: a fetch of `/healthz` from a load is dispatched through `internal`, which is the Kit app, not your mux.

Mount the handler at the root. To serve the app under a prefix, set Kit's [`paths.base`](https://svelte.dev/docs/kit/configuration#paths) and rebuild; the handler expects request paths that still carry the base, and a request outside it is a 404, so do not wrap it in `http.StripPrefix`.

### Middleware and the request context

Middleware that observes requests, for logging or metrics, wraps the returned handler like any other. Middleware that stores values in the request context needs one more thought: a subrequest made with `event.Fetch` starts from a fresh context that keeps cancellation, the trace span and baggage, the client address and the fetch depth, and nothing else, as Kit's `fork_state_for_subrequest` keeps its request facilities and drops the rest. A value added outside the stack reaches the page's request but not a route the page fetches in process. To see it on both, add it inside the stack, around the handler `internal` dispatches to:

```go
/// file: server.go
/// include: example/server.go#compose
```

To add your own, wrap the right-hand side of that assignment in a `func(http.Handler) http.Handler`; it then runs inside `FetchConfig` and ahead of `handle`, for the page's request and for every subrequest. Per-visitor data belongs in `Locals`, which the `handle` hook fills afresh for every subrequest, as in Kit.
