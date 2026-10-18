---
title: Page options
summary: Prerendering, SSR, CSR and trailing slashes stay in Kit's files; what each one means for your Go code.
proof:
  - example/e2e/features/actions-options.feature
  - example/e2e/features/ssr.feature
  - TestDynamicPrerenderEntriesAreServedFromFiles
  - TestPrerenderedGoRemoteIsAStaticAnswer
  - TestEveryPrerenderedPathIsServedFromItsFile
---

Kit's [page options](https://svelte.dev/docs/kit/page-options) are exported from `+page.ts`, `+page.server.ts`, `+layout.ts` or `+layout.server.ts`. In SKGo the server files are generated, so the options live in `+page.ts` and `+layout.ts`, as TypeScript, and apply to the whole route with Kit's usual inheritance. `skgo.Entries` supplies the server-side `entries` producer in Go; the other page options stay TypeScript. This page is about what each option means when the server is Go.

## prerender

A prerendered page is rendered once, at build time, and served as a file. `export const prerender = true` in `+page.ts` works as in Kit, and the Go load for the page runs during `vp build`: the SKGo adapter starts the generated prerender program in `internal/skgo/prerender`, which answers Kit's build-time requests with your Go loads and remote functions.

```ts
/// file: src/routes/prerender/[slug]/+page.ts
export const prerender = true;

```

```go
/// file: src/routes/prerender/[slug]/page.server.go
/// include: example/web/src/routes/prerender/[slug]/page.server.go
```

At request time the Go server serves `/prerender/atlas` from the file in the embedded build, and the load does not run again. A dynamic route whose pattern would also match a prerendered path loses to the file, as Kit's own servers behave.

### Prerendered remote functions

A `skgo.Prerender` is Kit's `prerender` remote function: a query whose answers are computed at build time and served as static files. `PrerenderOptions.Inputs` names a function that lists the arguments to render, which is Kit's `inputs` option:

```go
/// file: src/routes/about/about.remote.go
/// include: example/web/src/routes/about/about.remote.go
```

A load under a prerendered page may still read its parent, return a transported type and even a `Deferred`, as long as the value is settled when the page is built:

```go
/// file: src/routes/about/page.server.go
/// include: example/web/src/routes/about/page.server.go
```

### When not to prerender

Kit's rule is the rule: a page is prerenderable only if every visitor gets the same content from the server. A Go load that reads a cookie, a session or the request's headers cannot be prerendered, and one that writes a cookie or a header fails the build, as it would in Kit. Content that is personal belongs in a load that runs at request time, or in a remote function the page calls after it mounts.

## entries

Declare a named `func() ([]EntryParams, error)` with `skgo.Entries` in `page.server.go` or `server.go`, as in the Go catalog above. The generated `EntryParams` has exported fields named after the route parameters, with raw `string` values and `*string` for optional parameters. Leave an optional pointer nil to omit it. These are URL inputs; the request's `RouteParams` still contains the values your matchers convert them into.

Kit calls the producer during route analysis, before prerendering and without a request event. Read your catalog directly from Go. A returned error or panic fails the build with the route's module named. Kit then resolves the raw parameters into paths and rejects entries whose matcher selects a different route or no route.

An `entries` export in universal `+page.ts` takes precedence over Go page entries. Page entries take precedence over endpoint entries when both exist. Kit also discovers pages by crawling links from the prerendered pages it already has. `entries` lists routes; it does not enable `prerender` by itself.

## ssr

`export const ssr = false` turns off server rendering for the page: Go answers the document with an empty shell, and Kit's client renders the page after it boots. The Go load still runs, because Kit's client asks for `__data.json` as soon as it mounts, and the page's actions still work; a native form submission gets the shell back and the page draws the result after boot, while an enhanced submission shows it in place. If your application turns `ssr` off in the root layout, you have a single-page app whose Go server still answers every load, action, remote function and `server.go` route.

## csr

`export const csr = false` turns off Kit's client for the page: no JavaScript is sent, so `use:enhance` never runs and every form is a native submission. Go answers each `POST` with the complete document, including the `form` result of the action, which is the behaviour the site relies on when JavaScript is unavailable. An error from the action renders the nearest `+error.svelte` with that page's own options, so an error branch under a `csr = false` page may still have a client of its own.

## trailingSlash

Not yet per route. The Go server applies Kit's default of `never` to every route, and `always` to the root of an application with a `paths.base`; a request with the other form of a path gets Kit's redirect before anything is dispatched, and a prerendered page is found under either form. Per-route `trailingSlash` is [#163](https://github.com/tylergannon/skgo/issues/163).

## config

`config` is for adapters that read per-route settings. SKGo's adapter defines none, so there is nothing to put there.
