---
title: Differences from SvelteKit
summary: What SKGo does not support, supports differently, or defaults differently from SvelteKit, with the open issue for each.
---

SKGo runs SvelteKit %kit%'s own router, client and renderer, so anything not listed here behaves as the [Kit docs](https://svelte.dev/docs/kit) describe. Where a difference is a gap rather than a design, the skgo issue is linked.

## Server code is Go

- **No JavaScript server code.** Server loads, form actions, remote functions, `+server` routes and the `handle` hook are Go. `skgo generate` writes the `+page.server.ts`, `+layout.server.ts`, `+server.ts` and `*.remote.ts` modules Kit compiles, and their bodies throw `skgo: implemented in Go`. See [Loading data](load), [Form actions](form-actions), [Remote functions](remote-functions) and [Routing](routing).
- **Authored server modules are refused.** A server module Kit compiled that SKGo did not generate fails the production build. An authored `hooks.server.ts` stops `skgo generate`, and so does an authored `src/env.ts` once `src/env.go` declares an environment.
- **Universal `setHeaders`** throws during a render.

## Configuration

- **Adapter.** There is one adapter, `@skgo/sveltekit-adapter`, with options `out`, `precompress` and `prerenderPackage`. See [Adapter](adapter).
- **Refused options.** `output.bundleStrategy: 'inline'` and `router.resolution: 'server'` fail the build with a message saying so.
- **Hash routing.** Under `router.type: 'hash'` Go renders the requested path instead of the client shell ([#245](https://github.com/tylergannon/skgo/issues/245)).
- **Origin.** Kit 3 fixes `paths.origin` at build time; `skgo new` reads it from `ORIGIN` in `vite.config.ts`, and `just build` bakes the same value into the binary. `paths.base` is honoured by every Go handler.
- **Toolchain.** Kit is pinned to %kit% with a project-local patch to its prerender queue, applied by [`skgo kit-patch`](/docs/cli/kit-patch), and frontend tooling runs through VitePlus %vp%. See [Toolchain](toolchain).

## Routing

- **Trailing slashes.** Go has no per-route `trailingSlash` option: every route takes Kit's default of `never`, except the root of an application with a `paths.base`, which Kit forces to `always`. A mismatched URL gets Kit's redirect ([#163](https://github.com/tylergannon/skgo/issues/163)).
- **Param matchers.** The server runs Go matchers declared in `src/params.go`. Every matcher `src/params.ts` declares needs a Go matcher of the same name, or `skgo generate` fails with `requires a Go matcher in src/params.go` ([#159](https://github.com/tylergannon/skgo/issues/159)). See [Matching](routing#Route-parameters-Matching).
- **`reroute`.** The Go server does not apply a `reroute` hook from `src/hooks.ts` ([#167](https://github.com/tylergannon/skgo/issues/167)).

## Hooks

- **`handle`** is `var Handle params.Middleware` in `internal/serverhooks`, with Kit's `resolve` and `ResolveOptions` (`TransformPageChunk`, `FilterSerializedResponseHeaders`, `Preload`), composed with `params.Sequence`. See [Hooks](hooks).
- **`handleFetch`** is a Go `HandleFetch` on `FetchConfig` for Go-side fetches and on `SSROptions` for render-time fetches.
- **`handleError`** is a Go `HandleError`, set on the load, endpoint and handle configurations in `server.go`. See [Error handling](#Error-handling).
- **`init`** has no Go equivalent; startup work goes in `cmd/main.go` before the server listens ([#168](https://github.com/tylergannon/skgo/issues/168)).
- **`handleValidationError`** has no Go equivalent. An argument that does not decode into the Go type gets `400 Bad Request` ([#166](https://github.com/tylergannon/skgo/issues/166)).
- **`transport`** works: declare the Go types with `skgo.Transported` in `src/hooks.go` beside the encoders and decoders in `src/hooks.ts`. See [Types and transport](transport#Transport).
- **`event.platform`** does not exist ([#170](https://github.com/tylergannon/skgo/issues/170)).

## Error handling

- **Error kinds.** `CaughtError.Kind` is `"app"` for any `*skgo.HTTPError`, including the ones SKGo makes itself for a 404, and `"unknown"` for any other Go error, whose original value is in `caught.Err`. Kit's `framework` and `validation` kinds do not exist.
- **Remote functions skip `HandleError`.** `RemoteConfig` has no error hook: an `HTTPError` is returned as is, and any other error or a panic becomes an opaque `500` ([#354](https://github.com/tylergannon/skgo/issues/354)).
- **Server routes** write their own error responses, so `HandleError` sees only a panic.
- **The `handle` hook.** An error the hook returns reaches `HandleError` for page requests; a `__data.json` or remote request gets the JSON refusal Kit's client expects without it.
- **Svelte boundaries.** An error a `<svelte:boundary>` catches during a production render skips `HandleError` ([#353](https://github.com/tylergannon/skgo/issues/353)).
- **Prerendering.** `HandleError` and `HandleFetch` are not carried into the prerender ([#278](https://github.com/tylergannon/skgo/issues/278)).

See [Errors](errors).

## Cookies

- **API.** `Event.Cookie`, `SetCookie` and `DeleteCookie` replace `cookies.get`, `set` and `delete`. There is no `getAll`, `serialize` or `parse`, and `CookieOptions` has no `partitioned` or `priority`.
- **Defaults.** Path `/`, `HttpOnly` and `SameSite=Lax` as in Kit, but `Secure` is decided from the application's configured origin rather than the request URL: on unless the origin is `http://localhost`.
- **Writes.** A query cannot write a cookie, as in Kit, but a query called directly from a command's Go code still can ([#271](https://github.com/tylergannon/skgo/issues/271)). A `server.go` handler uses `http.SetCookie`.

See [Cookies](cookies).

## Requests and responses

- **`setHeaders`** is `Event.SetHeader`, allowed in server loads, page actions and the `handle` hook, and refused in remote functions; a `server.go` handler sets headers on its `http.ResponseWriter`.
- **`HEAD`.** A `+server` route answered from its `GET` handler may carry a `Content-Length` Kit omits ([#201](https://github.com/tylergannon/skgo/issues/201)).
- **`event.fetch`** is `Event.Fetch`, which answers the application's own routes in-process with Kit's credential rules. Parts of Kit's server-side fetch semantics are still open ([#169](https://github.com/tylergannon/skgo/issues/169)).
- **`getRequestEvent`** is `skgo.EventFrom(ctx)`.
- **Body size.** A remote `form` body over 32 MiB is refused with `413`; the cap is not configurable.
- **Client address.** `Event.ClientAddress()` is the peer address unless `HandleConfig.ClientAddress` is set; no proxy header is trusted by default.

## Remote functions

- **Kinds.** `query`, `query.batch`, `query.live`, `command`, `form` and `prerender` are all supported. `prerender`'s `dynamic` and `validate` options are reported as unsupported by the generator; only `inputs` works.
- **Types.** An argument or result cannot be a pointer, a map or an interface, and a function cannot be variadic. Use value types, named structs and `polytype.Nullable`. See [Types and transport](transport).
- **Route parameters.** Raw `Param` and `Params` panic in every remote function; use the typed getters on the generated event.

## Environment

- **Declaration.** Variables are a Go struct in `src/env.go` marked with `skgo.Environment`, and `skgo generate` writes Kit's `src/env.ts` from it. The server refuses to start without the validated snapshot `skgo.LoadEnvironment` returns. See [Environment variables](environment-variables).

## Rendering

- **Production renderer.** Pages render on goja: no Node built-ins, no `crypto`, `FormData` or global `fetch`, nothing loaded at run time, and one render per runtime at a time. See [UI libraries](integrations#UI-libraries).
- **Source maps.** The production bundle has none, so render stack traces point at `ssr/bundle.js`.
- **`read` from `$app/server`** is not resolved in the Go renderer ([#172](https://github.com/tylergannon/skgo/issues/172)). Read files in Go.

## Prerendering

- **`entries`** must be exported from `+page.ts` or `+layout.ts`; a Go load cannot supply them ([#352](https://github.com/tylergannon/skgo/issues/352)).
- **Errors.** A `TransformPageChunk` failure during prerendering is always terminal ([#276](https://github.com/tylergannon/skgo/issues/276)).
- **Fetch.** During prerendering, `Event.Fetch` of an existing page or data route returns a 404 ([#272](https://github.com/tylergannon/skgo/issues/272)).
- **Header filter.** The renderer's `FilterSerializedResponseHeaders` is not applied while prerendering ([#275](https://github.com/tylergannon/skgo/issues/275)).

See [Prerendering](prerendering).

## Security

- **`csrf.trustedOrigins`** is an exact-match list. `'*'` is compared literally and does not turn the check off as it does in Kit; an empty configured origin does.

## Observability

- **Tracing.** With Kit's `tracing.server` on, Go emits Kit's spans. Configure the OpenTelemetry provider in Go before serving; there is no `instrumentation.server.ts`. See [Observability](observability).

## Deployment

- **Configuration.** adapter-node's environment variables are not read. The origin is compiled in by `just build` and overridden with `-origin`; the address is `-listen`. See [Deploying](deploying).
- **Shutdown.** The generated `cmd/main.go` has no graceful shutdown and no server timeouts ([#361](https://github.com/tylergannon/skgo/issues/361)).
- **Hosting.** Serverless platforms are not supported; see [Project types](project-types#Serverless-app).

## Development

- **Toolchain.** `skgo dev` requires exactly Go %go% on `darwin/arm64` or `linux/amd64`, builds the host without cgo, and rejects a non-empty `GOEXPERIMENT` or custom `GOFLAGS`. See [`skgo dev`](/docs/cli/dev).
- **Stale code.** A native relocation failure can leave old code serving without an error ([#330](https://github.com/tylergannon/skgo/issues/330)).
- **Load errors.** A failing Go load returns an opaque 500 with no message or source frame ([#243](https://github.com/tylergannon/skgo/issues/243)).
- **New routes.** A newly added Go route can take close to a minute to become ready ([#277](https://github.com/tylergannon/skgo/issues/277)).
- **Cookies.** A cookie set by Go on a render-time subrequest is not relayed into the page response. The default development origin is `http://127.0.0.1:PORT`, so Go-written cookies get `Secure`, unlike under Kit's own dev server.

See [Development server](development-server).

## Unchanged from Kit

| Kit page | In SKGo |
|---|---|
| [Link options](https://svelte.dev/docs/kit/link-options) | Unchanged |
| [Server-only modules](https://svelte.dev/docs/kit/server-only-modules) | The rules apply to frontend code; server-only code is Go packages |
| [Snapshots](https://svelte.dev/docs/kit/snapshots) | Unchanged |
| [Shallow routing](https://svelte.dev/docs/kit/shallow-routing) | Unchanged |
| [Images](https://svelte.dev/docs/kit/images) | Unchanged |
| [Icons](https://svelte.dev/docs/kit/icons) | Unchanged |
| [Accessibility](https://svelte.dev/docs/kit/accessibility) | Unchanged |
| [SEO](https://svelte.dev/docs/kit/seo) | Unchanged; a sitemap is a Go `+server` route |
| [`$app/state`](https://svelte.dev/docs/kit/$app-state) | Unchanged |
