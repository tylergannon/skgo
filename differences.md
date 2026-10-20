---
title: Differences from SvelteKit
description: What SKGo does not support, supports differently, or defaults differently from SvelteKit, with the open issue for each.
division: skgo
section: Appendix
order: 6040
---

SKGo runs SvelteKit's own router, client and renderer, so anything not listed here behaves as the [Kit docs](https://svelte.dev/docs/kit) describe. Where a difference is a gap rather than a design, the skgo issue is linked.

## Server code is Go

- **No JavaScript server code.** Server loads, form actions, remote functions, `+server` routes and the `handle` hook are Go. `skgo generate` writes the `+page.server.ts`, `+layout.server.ts`, `+server.ts` and `*.remote.ts` modules Kit compiles, and their bodies throw `skgo: implemented in Go`. See [Loading data](/docs/skgo/load), [Form actions](/docs/skgo/form-actions), [Remote functions](/docs/skgo/remote-functions) and [Routing](/docs/skgo/routing).
- **Authored server modules are refused.** A server module Kit compiled that SKGo did not generate fails the production build. An authored `hooks.server.ts` stops `skgo generate`, and so does an authored `src/env.ts` once `src/env.go` declares an environment.
- **Universal `setHeaders`** throws during a render.

## Configuration

- **Adapter.** There is one adapter, `@skgo/sveltekit-adapter`, with options `out`, `precompress` and `prerenderPackage`. See [Adapter](/docs/skgo/adapter).
- **Refused options.** `output.bundleStrategy: 'inline'`, `router.resolution: 'server'` and a non-empty route `config` fail the build with a message saying so.
- **Hash routing.** Under `router.type: 'hash'`, Go answers a document request for any path with Kit's client shell, and the browser renders the page the hash names. Kit's own server answers a path other than the root with `404`. Static assets are still served from their paths.
- **Origin.** Kit 3 fixes `paths.origin` at build time; `skgo new` reads it from `ORIGIN` in `vite.config.ts`, and `just build` bakes the same value into the binary. `paths.base` is honoured by every Go handler.
- **Toolchain.** Kit is pinned to one exact release with a project-local patch to its prerender queue, applied by [`skgo kit-patch`](/docs/cli/kit-patch), and frontend tooling runs through a pinned VitePlus. See [Prerequisites](/docs/skgo/creating-a-project#Prerequisites).

## Routing

- **Trailing slashes.** `trailingSlash` in `+page.ts` and `+layout.ts` works as in Kit. A route with only a `server.go` always takes Kit's default of `never`: its `+server.ts` is generated, so `skgo generate` refuses one that exports `trailingSlash` or `config`. See [Page options](/docs/skgo/page-options#trailingSlash).
- **Param matchers.** The server runs Go matchers declared in `src/params.go`. Every matcher `src/params.ts` declares needs a Go matcher of the same name, or `skgo generate` fails with `requires a Go matcher in src/params.go`. See [Matching](/docs/skgo/routing#Route-parameters-Matching).
- **`reroute`.** Write the hook body in Go and export the adapter's universal forwarding hook from `src/hooks.ts`; see [Hooks](/docs/skgo/hooks#reroute). An arbitrary JavaScript reroute body is not executed by the Go server.

## Hooks

- **`handle`** is `var Handle params.Middleware` in `internal/serverhooks`, with Kit's `resolve` and `ResolveOptions` (`TransformPageChunk`, `FilterSerializedResponseHeaders`, `Preload`), composed with `params.Sequence`. See [Hooks](/docs/skgo/hooks).
- **`handleFetch`** is a Go `HandleFetch` on `FetchConfig` for Go-side fetches and on `SSROptions` for render-time fetches.
- **`handleError`** is a Go `HandleError`, set on the load, remote, endpoint and handle configurations in `server.go`. See [Error handling](#Error-handling).
- **`init`** has no Go equivalent; startup work goes in `cmd/main.go` before the server listens ([#168](https://github.com/tylergannon/skgo/issues/168)).
- **Remote argument validation** uses `HandleError` with `CaughtError.Kind == "validation"`, as in Kit. The hook receives private decoder issues and chooses the public error fields; see [Validation errors](/docs/skgo/errors#Validation-errors).
- **`transport`** works: declare the Go types with `skgo.Transported` in `src/hooks.go` beside the encoders and decoders in `src/hooks.ts`. See [Types and transport](/docs/skgo/transport#Transport).
- **`event.platform`** does not exist.

## Error handling

- **Error kinds.** `CaughtError.Kind` is `"app"` for any `*skgo.HTTPError`, including the ones SKGo makes itself for a 404, `"validation"` for a rejected generated remote argument, and `"unknown"` for other Go errors, whose original value is in `caught.Err`. Kit's `framework` kind is represented as `"app"`.
- **Server routes** write their own error responses, so `HandleError` sees only a panic.
- **The `handle` hook.** An error the hook returns reaches `HandleError` for page requests; a `__data.json` or remote request gets the JSON refusal Kit's client expects without it.

See [Errors](/docs/skgo/errors).

## Cookies

- **API.** `Event.Cookie`, `SetCookie` and `DeleteCookie` replace `cookies.get`, `set` and `delete`. There is no `getAll`, `serialize` or `parse`, and `CookieOptions` has no `partitioned` or `priority`.
- **Defaults.** Path `/`, `HttpOnly` and `SameSite=Lax` as in Kit, but `Secure` is decided from the application's configured origin rather than the request URL: on unless the origin is `http://localhost`.
- **Writes.** A query cannot write a cookie, as in Kit, but a query called directly from a command's Go code still can ([#271](https://github.com/tylergannon/skgo/issues/271)). A `server.go` handler uses `http.SetCookie`.

See [Cookies](/docs/skgo/cookies).

## Requests and responses

- **`setHeaders`** is `Event.SetHeader`, allowed in server loads, page actions and the `handle` hook, and refused in remote functions; a `server.go` handler sets headers on its `http.ResponseWriter`.
- **`event.fetch`** is `Event.Fetch`, which answers the application's own routes in-process with Kit's credential rules.
- **`getRequestEvent`** is `skgo.EventFrom(ctx)`.
- **Body size.** A remote `form` body over 32 MiB is refused with `413`; the cap is not configurable.
- **Client address.** `Event.ClientAddress()` is the peer address unless `HandleConfig.ClientAddress` is set; no proxy header is trusted by default.

## Remote functions

- **Kinds.** `query`, `query.batch`, `query.live`, `command`, `form` and `prerender` are all supported. `prerender`'s `dynamic` and `validate` options are reported as unsupported by the generator; only `inputs` works.
- **Types.** An argument or result cannot be a pointer, a map or an interface, and a function cannot be variadic. Use value types, named structs and `polytype.Nullable`. See [Types and transport](/docs/skgo/transport).
- **Route parameters.** Raw `Param` and `Params` panic in every remote function; use the typed getters on the generated event.

## Environment

- **Declaration.** Variables are a Go struct in `src/env.go` marked with `skgo.Environment`, and `skgo generate` writes Kit's `src/env.ts` from it. The server refuses to start without the validated snapshot `skgo.LoadEnvironment` returns. See [Environment variables](/docs/skgo/environment-variables).

## Rendering

- **Production renderer.** Pages render on goja: no Node built-ins, no `crypto`, `FormData` or global `fetch`, nothing loaded at run time, and one render per runtime at a time. See [UI libraries](/docs/skgo/integrations#UI-libraries).
- **Source maps.** The production bundle has none, so render stack traces point at `ssr/bundle.js`.
- **`read` from `$app/server`** is not resolved in the Go renderer ([#172](https://github.com/tylergannon/skgo/issues/172)). Read files in Go.

## Prerendering

- **Errors.** A `TransformPageChunk` error fails its page, and Kit's `prerender.handleHttpError` policy decides whether the build stops, as for any page error. A malformed reply from the Go prerender service stops the build under every policy.
- **Fetch.** During prerendering, Go `Event.Fetch` reaches server routes. An existing page, its data route or the remote-function service returns an explicit capability error because it requires Kit's request context; fetch those destinations from a universal load.

See [Prerendering](/docs/skgo/prerendering).

## Security

- **`csrf.trustedOrigins`** is an exact-match list. `'*'` is compared literally and does not turn the check off as it does in Kit; an empty configured origin does.

## Observability

- **Tracing.** With Kit's `tracing.server` on, Go emits Kit's spans. Configure the OpenTelemetry provider in Go before serving; there is no `instrumentation.server.ts`. See [Observability](/docs/skgo/observability).

## Deployment

- **Configuration.** adapter-node's environment variables are not read. The origin is compiled in by `just build` and overridden with `-origin`; the address is `-listen`. See [Deploying](/docs/skgo/deploying).
- **Shutdown.** The generated `cmd/main.go` drains active HTTP requests for up to ten seconds on `SIGTERM` or interrupt, then closes remaining connections and runs application cleanup. No per-request server timeouts are configured. See [Deploying](/docs/skgo/deploying#Shutdown).
- **Hosting.** Serverless platforms are not supported; see [Project types](/docs/skgo/project-types#Serverless-app).

## Development

- **Toolchain.** `skgo dev` requires exactly the Go release SKGo pins, on `darwin/arm64` or `linux/amd64`, builds the host without cgo, and rejects a non-empty `GOEXPERIMENT` or custom `GOFLAGS`. See [`skgo dev`](/docs/cli/dev).
- **Go edits.** A compatible Go edit installs into the running application; any other Go change, or an edit the native loader fails to install, replaces the application and resets its in-memory state. See [Hot updates](/docs/skgo/development-server#Hot-updates).
- **Edit latency.** Save-to-visible time for Go edits and new Go routes has no recorded measurement ([#311](https://github.com/tylergannon/skgo/issues/311)).
- **Cookies.** A cookie set by Go on a render-time subrequest is not relayed into the page response. The default development origin is `http://127.0.0.1:PORT`, so Go-written cookies get `Secure`, unlike under Kit's own dev server.

See [Development server](/docs/skgo/development-server).

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
