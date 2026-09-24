---
name: skgo
description: Build and maintain SvelteKit 3 applications served by Go with skgo. Use for Go remote functions, server loads, route handlers, server composition, generation, development, and deployment. Do not use for ordinary SvelteKit apps whose server runs in JavaScript.
---

# skgo

Build the app as SvelteKit engineers and Go engineers expect: SvelteKit owns the
frontend contract; Go owns all server behavior and the production process.

## Orient first

Read the app's `README.md`, selected build file (`mise.toml`, `Justfile`, or
`scripts/env.sh`), `web/vite.config.ts`, root `server.go`, and the nearest
working route before changing it. Check the exact skgo and
SvelteKit versions in `go.mod` and `web/package.json`; pre-1.0 skgo follows a
pinned SvelteKit 3 prerelease, so do not substitute behavior remembered from
Kit 2 or a different Kit 3 build.

## Keep the ownership boundary clear

- Write pages, layouts, components, navigation, and browser interaction as
  ordinary SvelteKit and Svelte.
- Write every query, command, form, live query, batch query, server load, and
  `+server.ts` handler in Go beside the route or library that uses it.
- Do not implement application I/O in TypeScript. Generated TypeScript server
  bodies throw intentionally; they give Kit the declarations and manifests its
  own client needs.
- Use `net/http`, `context.Context`, and normal Go packages for server work.
  Reach request state through `skgo.EventFrom(ctx)`.

## Author through Go, consume through Kit

Declare remote functions with `skgo.Query`, `skgo.Command`, `skgo.Form`,
`skgo.LiveQuery`, or `skgo.BatchQuery` in a `*.remote.go` file. Keep exported
wire types JSON-shaped and give fields explicit JSON tags. Let generation
project those types and codecs; do not hand-maintain a parallel TypeScript type.

Import the generated sibling `.remote` module from Svelte and use Kit's native
client API, including `.updates(...)`. When refresh policy belongs to the
server, use the typed Go refresh and reconnect helpers instead of string IDs.

## Call a Form from Go

For a Form whose fields are flat scalars (including `polytype.Optional[T]`
scalars) and whose result is made of scalars, generation also writes an
importable Go package at `<bindings>/client` with one typed method per
supported Form. Forms with files or nested data keep their browser and server
bindings but get no Go client method.

```go
forms := client.Client{FormClient: skgo.FormClient{BaseURL: "http://127.0.0.1:8080"}}
result, err := forms.Submit(ctx, optional.Input{Name: "Ada"})
```

To reach the server over a Unix socket, give `FormClient.HTTPClient` a
transport that dials the socket. `BaseURL` can then name any `http://` host,
such as `http://skgo`:

```go
transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
}}
forms := client.Client{FormClient: skgo.FormClient{
	BaseURL: "http://skgo", HTTPClient: &http.Client{Transport: transport},
}}
```

A call returns the declared result, `*skgo.Invalid` for field issues,
`*skgo.HTTPError` for a server error envelope, or the transport error. The
caller's context cancels the request. Each submission is one POST: redirects,
including 307 and 308, come back as errors and are never replayed. The
example's `cmd/form-client` is a working caller.

## Loads and routes

Server loads live in `page.server.go` and `layout.server.go`. HTTP routes live
in `server.go` and are ordinary `http.Handler` functions. Keep the generated
Kit stubs beside them; their failure is what prevents JavaScript from silently
becoming a second server implementation.

## Generate and compose

Use the build gesture in the project's README. It installs the frontend, runs
`go generate`, builds through Vite+, and links the binary. Files
marked generated are outputs, not authoring surfaces.

The root server composition should mount one production stack in Kit's dispatch
order: request hook, loads, remotes, endpoints, then pages/static assets. Build
SSR with the generated manifest and the same load and remote registries that
answer browser requests. Reuse that composition from tests and the binary.

In development, Vite+ may answer page assets through the Go proxy, but Go must
still intercept every server load, remote call, and endpoint before the proxy.

## Preserve Kit's invariants

- The app's public origin is fixed at frontend build time and checked again by
  Go for non-GET remote calls. Change the single `ORIGIN` in the selected build
  file and rebuild both halves together.
- Keep SvelteKit route syntax and path identity intact, including `[params]`,
  `[...rest]`, route groups, and `#lib` imports.
- A query may read request state but not mutate cookies. Commands and forms may
  mutate it.
- Respect per-argument query identity. Refresh exactly the typed query instances
  the command is allowed to refresh, and bound client-requested refreshes.
- Preserve `ssr = false`, `csr = false`, prerender, errors, redirects, and
  transport hooks as Kit defines them; skgo mirrors Kit rather than inventing
  parallel semantics. A page whose branch has a Go server load cannot be
  prerendered, and generation refuses one that asks to be.
- There are no classic `+page.server.ts` form actions; use a remote `form`.
  Remote function signatures cannot use pointer, map or interface types; use
  value types, named structs, or `polytype.Nullable`.

## Verify the real boundary

Run the project's frontend build/check and `go test ./...`. Then exercise the
running binary in the relevant mode. For server-rendered behavior, inspect the
initial document or run with JavaScript disabled so a client-side fallback
cannot impersonate success. For an endpoint or remote function, confirm the Go
handler answered and that the generated throwing stub could not have done so.

Use the repository's [example app](https://github.com/tylergannon/skgo/tree/main/example)
as the working reference. Its front page maps each supported capability to a
route, and its Go and browser tests state the currently demonstrated boundary.
