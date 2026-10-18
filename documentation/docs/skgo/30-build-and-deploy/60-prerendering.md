---
title: Prerendering
summary: How Kit's build calls your Go loads, endpoints and remote functions, and how the binary serves what it wrote.
proof:
  - TestAPrerenderedPageCanHaveItsOwnGoLoad
  - TestAPrerenderedPageCanHaveAGoLayoutLoadInItsBranch
  - TestEveryPrerenderedPathIsServedFromItsFile
  - TestDynamicPrerenderEntriesAreServedFromFiles
  - TestPrerenderedGoRemoteIsAStaticAnswer
  - TestAPrerenderedRedirectUsesKitsCanonicalHTMLArtifact
  - TestAPrerenderedFetchCarriesNothingOfThePageAndKeepsTheClientAddress
  - TestPrerenderFetchRefusesUnavailableDestinationsAndKeepsEndpointIsolation
  - TestPrerenderResolveOptionDefaultsAndFailures
---

Prerendering works as it does in [Kit](https://svelte.dev/docs/kit/page-options#prerender): mark a page with `export const prerender = true`, and `vp build` writes it to a file. What differs is where the data comes from. Server loads, `server.go` routes and remote functions are Go, so the build calls Go. [Page options](page-options#prerender) covers the option itself; this page covers the machinery and its edges.

## How prerendering runs

During `vp build` the adapter compiles the Go package named by its [`prerenderPackage`](adapter#Options-prerenderPackage) option, by default the entry `skgo generate` writes, and starts it as a service. Kit's prerenderer still drives the build: it crawls from the non-dynamic routes, follows links, honours `entries`, `handleHttpError`, `handleMissingId` and `handleUnseenRoutes` as [Kit documents](https://svelte.dev/docs/kit/page-options#prerender-Troubleshooting), and asks the service whenever a page needs a Go load, an endpoint or a remote function.

The generated service is a `package main` of its own:

```go
/// file: internal/skgo/prerender/skgo_gen.go
/// include: example/internal/skgo/prerender/skgo_gen.go
```

It imports your route packages, so package-level `var` and `init` code runs at build time, but it never calls `main` or `NewHandler`; a database you open there is not open during the build. `BindRequest` is your `handle` hook, so each build-time request has typed `Locals` as a live one would, and `event.ClientAddress()` returns an error because there is no client. Go code has no `building` flag; universal code can still read [`building`](https://svelte.dev/docs/kit/$app-env#building) from `$app/env`.

The output lands in the build's `prerendered/` tree, and the binary serves it from there.

## Pages with Go loads

The page option stays in TypeScript, beside the Go load:

```ts
/// file: src/routes/about/+page.ts
export const prerender = true;
```

```go
/// file: src/routes/about/page.server.go
/// include: example/web/src/routes/about/page.server.go
```

The load runs once, at build time, and may read its parent, return a transported type and a `Deferred`, as long as the value is settled when the page is built. Its result is in the page's HTML and in the page's `__data.json` file, so a client-side navigation to the page reads the same data.

Kit's rules for [when not to prerender](page-options#prerender-When-not-to-prerender) apply unchanged, and `skgo generate` enforces one of them early: a page with Go form actions whose `prerender` is `true` or `'auto'` fails generation. A route Kit prerenders leaves the server's dynamic route table, as it does in Kit's own adapters, so a path the build did not write is a 404 at runtime.

## Dynamic entries

For a dynamic route no crawled link reaches, export [`entries`](https://svelte.dev/docs/kit/page-options#entries) from `+page.ts`, or register a Go producer with [`skgo.Entries`](skgo-loads#Entries) in `page.server.go` or `server.go`. The Go load runs once per entry:

```ts
/// file: src/routes/prerender/[slug]/+page.ts
export const prerender = true;

// Kit asks for these parameters during route analysis, before prerendering.
export function entries() {
  return [{ slug: "atlas" }, { slug: "beacon" }];
}
```

```go
/// file: src/routes/prerender/[slug]/page.server.go
/// include: example/web/src/routes/prerender/[slug]/page.server.go
```

A Go producer takes no request event and returns the route's generated `EntryParams` values. Kit calls it during route analysis, before prerendering. An `entries` export in universal `+page.ts` takes precedence over Go page entries; page entries take precedence over endpoint entries.

## Prerendering server routes

A `server.go` route cannot export `prerender`, because its generated `+server.ts` is a stub. As in [Kit](https://svelte.dev/docs/kit/page-options#prerender-Prerendering-server-routes), a server route that a prerendered page fetches is prerendered with it, whether the page fetches it from a universal load or a Go load fetches it with `event.Fetch`. The service runs the Go handler, the build writes the response, and the binary serves it as a file. The route leaves the dynamic table, so a path the build did not fetch is a 404.

## Remote prerender functions

A [prerender remote function](remote-functions#prerender) is a Go function registered with `skgo.Prerender`. Calls Kit meets while prerendering pages are saved automatically; `Inputs` names a function in the same file that lists further arguments:

```go
/// file: src/routes/about/about.remote.go
/// include: example/web/src/routes/about/about.remote.go
```

`PrerenderOptions.Dynamic` and `Validate` are not implemented, and `skgo generate` reports either as unsupported. A prerender function is therefore never run at request time: an argument the build did not render has no answer. The browser caches the results as Kit [describes](https://svelte.dev/docs/kit/remote-functions#prerender).

### The Kit queue correction

Kit %kit%'s prerender queue can finish before declared `Inputs` are queued. A build that declares `Inputs` checks that the installed Kit carries SKGo's correction, and fails with `SKGO_KIT_PRERENDER_QUEUE` when it does not. `skgo new` configures the project-local patch; for an existing project run [`skgo kit-patch`](/docs/cli/kit-patch):

```sh
go tool skgo kit-patch --web web --apply
cd web && vp install --no-frozen-lockfile && cd ..
go tool skgo kit-patch --web web --check
```

## Redirects

A Go load that returns a `*skgo.Redirect` while prerendering produces Kit's redirect artifact, an HTML file with a script and a `<meta http-equiv="refresh">` pointing at the destination. The binary serves that file with status 200 and no `Location` header, as Kit's static output is served, and answers `POST` to it with a 405. The crawler follows the redirect, so a prerenderable destination is prerendered too; a dynamic destination stays dynamic.

## Shared application configuration

Keep `HandleFetch`, `HandleError` and the default serialized-header filter in one
[`ApplicationConfig`](skgo-hooks#ApplicationConfig) value:

```go
/// file: internal/app/config.go
/// include: example/internal/app/config.go#application-config
```

Select that value in the generation declaration:

```go
/// file: internal/skgo/config.go
/// include: example/internal/skgo/config.go
```

`--config-package` names the Go package; `--config-symbol` selects an exported
`ApplicationConfig` variable and defaults to `Config`. Keep its package loadable
before generation: use the runtime's `skgo.RequestLocals` when a hook needs typed
locals, rather than depending on a generated helper.

The default generated prerender command reads the selected value. It still owns
the generated loads, remotes, endpoints, matchers, transport and request boundary.
No custom entry point or adapter setting is needed. The same value configures the
runtime stack in `server.go`: `HandleError` on the load, remote, endpoint and
request configs, `HandleFetch` on both `FetchConfig` and `SSROptions`, and the
header filter on `SSROptions`. `skgo new` wires these assignments for you.

At build time the fetch hook surrounds Kit's supplied fetch continuation for a
universal load, and Go's dispatcher for `event.Fetch`. The error hook runs for
build-time loads, remotes, endpoints and render errors; an already-handled remote
error reuses its public body. An explicit per-request
`ResolveOptions.FilterSerializedResponseHeaders` replaces the shared default in
both modes. Without a selected config, the generated service keeps its existing
defaults: no fetch hook or application error hook, and no serialized headers.

### Custom prerender service

[`prerenderPackage`](adapter#Options-prerenderPackage) remains available when the
build needs an application-owned entry point. Such an entry calls
[`RunPrerenderService`](skgo-server#RunPrerenderService) and supplies its generated
registrations itself. [`PrerenderServiceOptions`](skgo-server#PrerenderServiceOptions)
lists the available settings.

## Static-only output

SKGo has no static export. Even when every page is prerendered, the build is shaped for the binary: client assets and prerendered pages are separate trees, prerendered files whose paths carry a query string are relocated and mapped in the manifest, and there is no `fallback` or `strict` option. Deploy a fully prerendered app as the binary; Kit's [static adapter](https://svelte.dev/docs/kit/adapter-static) is not supported.

## Limitations

- At build time, `event.Fetch` from Go reaches `server.go` routes. A known page, its `__data.json` path or the remote-function service needs Kit's request context, which the Go build callback cannot enter. These fetches return a Go error naming the unavailable destination; they do not return a misleading 404. A genuinely missing route still returns 404. Fetch page/data/remote destinations from a universal load when Kit must handle them during prerendering.
