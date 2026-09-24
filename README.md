# skgo

**A Go backend for SvelteKit.**

skgo lets you write the server side of a SvelteKit app in Go. Remote functions,
server loads and API routes are Go code that sits next to the Svelte files that
use them. skgo generates TypeScript types and bindings from that code, so you
write a function in Go and await it in Svelte, the same way you would call any
SvelteKit remote function. Pages are rendered by SvelteKit's own renderer
running inside the Go process, and the app deploys as a single Go binary. Node
is only needed to build it.

> **Status:** pre-1.0. skgo tracks the SvelteKit 3 prerelease pinned in this
> repository. All server code is written in Go: TypeScript remote functions,
> loads and API routes are not supported. The [example app](example/) shows what
> works today.

## Agent skills

skgo publishes skills that teach a coding agent how to build with it: the
[skgo skill](skills/skgo/SKILL.md) and the
[polytype skill](https://github.com/tylergannon/polytype/tree/main/skills/polytype)
for the Go types that cross the wire. Paste this prompt into Claude Code, Codex
or any other agent that reads skills:

```text
Install the agent skills for skgo (a Go backend for SvelteKit) and polytype (which generates its Go-to-TypeScript types):

vp dlx skills add tylergannon/skgo -y
vp dlx skills add tylergannon/polytype -y

Without vp (VitePlus), use pnpm dlx instead, not npx: skgo projects use pnpm. Add -g to install them for all projects. Then read the skgo skill before writing skgo code.
```

## Example

A query, written in Go:

```go
// web/src/routes/todos/todos.remote.go
package todos

import (
	"context"

	"github.com/tylergannon/skgo"
)

type Todo struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

func todos(ctx context.Context) ([]Todo, error) {
	return store.List(ctx)
}

var _ = skgo.Query(todos)
```

Called from a Svelte component:

```svelte
<!-- web/src/routes/todos/+page.svelte -->
<script lang="ts">
	import { todos } from './todos.remote';
</script>

{#each await todos() as todo}
	<p>{todo.text}</p>
{/each}
```

`skgo generate` writes `todos.remote.ts` next to the Go file. The query runs in
Go, and `todo` has the type of the `Todo` struct.

## What you write and what skgo generates

| You write, in Go | skgo generates | SvelteKit sees |
| --- | --- | --- |
| `todos.remote.go`, using `skgo.Query`, `Command`, `Form`, `LiveQuery` or `BatchQuery` | `todos.remote.ts`, its TypeScript types, the wire codecs and the Go registration | A remote function, imported from `./todos.remote` |
| `page.server.go` or `layout.server.go` | A typed `+page.server.ts` or `+layout.server.ts` | A server load. `data` is typed from the Go result. |
| `server.go`, a `net/http` handler | A `+server.ts` | An API route |

The generated TypeScript function bodies throw an error. They exist so SvelteKit
can build its client and manifests, which means every response comes from Go.

## For Go developers

- The build embeds the frontend, its assets and the server-rendering bundle in
  one binary. Node runs at build time only.
- A remote function is an ordinary Go function that takes a `context.Context`
  and returns `(T, error)`. A one-line marker such as `skgo.Query(todos)`
  registers it.
- API routes are `http.Handler`s. You assemble loads, remote functions, API
  routes and page rendering into your own `net/http` server (see
  [`example/server.go`](example/server.go)).
- The request and its cookies are available from `skgo.EventFrom(ctx)`.
- The JSON shape of your Go types becomes the TypeScript types. After you change
  a type, regenerate, and the TypeScript checker reports every component that
  still uses the old shape.
- There is no cgo. The JavaScript engine, [goja](https://github.com/dop251/goja),
  is written in Go.
- Generated files are ordinary Go and TypeScript, marked `DO NOT EDIT`.

## For SvelteKit developers

- Components are ordinary Svelte 5.
- Routing, layouts, error pages, navigation and the remote-function client are
  SvelteKit's own code, not a reimplementation.
- Queries, commands, forms, live queries and batch queries are imported and
  called as in any SvelteKit app. Only the implementation is Go.
- A Go load's result type becomes the page's `data` type.
- Pages are rendered on the server. Streamed promises, `ssr = false` and
  `csr = false` work.
- Remote forms still work when JavaScript is disabled.
- In development, `just dev` runs `vp dev` behind the Go server. The Go server
  renders pages and forwards modules and hot reloading.
- `skgo new` creates the project with `sv create`, then adds Vitest and
  Storybook.

## SvelteKit feature support

This matrix covers SvelteKit's server-facing features and the tooling needed to
build a skgo app. **Available** means the current Go implementation has a
working example or focused test; it does not promise every edge case of the
upstream API. **Limited** names a narrower supported case. **WIP** is active
work, not a released feature. **Not supported** means skgo has no compatible
implementation. **Not verified** means this repository does not yet establish
the behavior. The upstream links describe SvelteKit generally; skgo follows
the [pinned Kit 3 prerelease](example/web/package.json), which may differ from
the public documentation.

### Pages and routing

| SvelteKit feature | skgo | Boundary or example |
| --- | --- | --- |
| [Svelte 5 pages, server rendering and hydration](https://svelte.dev/docs/kit/routing) | Available | Kit's renderer runs in Go; [cold HTML and hydration](example/e2e/features/ssr.feature) are exercised. |
| [Client-side navigation](https://svelte.dev/docs/kit/routing) | Available | Kit's client navigates without a document reload; [browser scenario](example/e2e/features/app.feature). |
| [Layouts, route groups and nested error pages](https://svelte.dev/docs/kit/routing) | Available | [Grouped routes](example/e2e/features/colocation.feature), [nested loads](example/e2e/features/loads.feature) and [error boundaries](example/e2e/features/showcase.feature). |
| [Dynamic and rest route parameters](https://svelte.dev/docs/kit/advanced-routing) | Available | Parameters reach colocated Go handlers; [browser scenarios](example/e2e/features/colocation.feature). |
| [Optional parameters and parameter matchers](https://svelte.dev/docs/kit/advanced-routing) | Limited | Optional captures are decoded; JavaScript parameter matcher functions are not run by Go. |
| [Page and layout server loads](https://svelte.dev/docs/kit/load) | Available | Go loads produce typed `data` in documents and `__data.json`; [browser scenarios](example/e2e/features/loads.feature). |
| [Universal JavaScript loads](https://svelte.dev/docs/kit/load#Universal-vs-server) | Not verified | Page options in `+page.ts` work, but there is no demonstrated universal `load`; application I/O in JavaScript is outside skgo's contract. |
| [Parent load data, URL, params and route ID](https://svelte.dev/docs/kit/load) | Available | Go exposes `Parent` and request data through `EventFrom`; [nested example](example/web/src/routes/account/page.server.go). |
| [Load dependencies, invalidation and untrack](https://svelte.dev/docs/kit/load#Rerunning-load-functions) | Limited | Go has `Depends` and `Untrack`; the [browser suite](example/e2e/features/loads.feature) proves reuse of a layout load, not every invalidation path. |
| [Streaming load promises](https://svelte.dev/docs/kit/load#Streaming-with-promises) | Available | Deferred Go values stream into HTML and client navigation; [browser scenarios](example/e2e/features/stream.feature). |
| [Expected errors, redirects and error pages](https://svelte.dev/docs/kit/errors) | Available | Go load and remote outcomes use Kit's page/error behavior; [browser scenarios](example/e2e/features/ssr.feature). |
| [`ssr = false` and `csr = false`](https://svelte.dev/docs/kit/page-options) | Available | SPA fallback and script-free HTML are [browser exercised](example/e2e/features/ssr.feature). |
| [Prerendering](https://svelte.dev/docs/kit/page-options#prerender) | Limited | Kit-built static files are served; a branch with a Go server load cannot be prerendered, and prerendered redirects are refused. |
| [Dynamic prerender entries](https://svelte.dev/docs/kit/page-options#entries) | Not verified | There is no demonstrated `entries` generator in the example. |
| [Trailing slash and route config options](https://svelte.dev/docs/kit/page-options) | Limited | Default trailing-slash redirect is [exercised](example/e2e/features/routes.feature); other per-route options are not established. |

### Requests and server behavior

| SvelteKit feature | skgo | Boundary or example |
| --- | --- | --- |
| [`+server` endpoints and HTTP methods](https://svelte.dev/docs/kit/routing#+server) | Available | `net/http` handlers serve Kit routes, including `QUERY`; [browser scenarios](example/e2e/features/routes.feature). |
| [Page, endpoint and action on one route](https://svelte.dev/docs/kit/routing#+server) | WIP | Shared dispatch with a classic action is in the form-actions work. |
| [Cookies and request-local state](https://svelte.dev/docs/kit/load#Cookies) | Available | `EventFrom`, `SetCookie` and typed locals support the [sign-in example](example/e2e/features/auth.feature). |
| [Response headers from loads](https://svelte.dev/docs/kit/load#Headers) | Available | Go `Event.SetHeader` follows Kit's duplicate-header and cookie rules; [implementation](loadevent.go). |
| [`handle` and `handleError` hooks](https://svelte.dev/docs/kit/hooks) | Available | Go hooks cover requests and public errors; [example](example/server.go). |
| [`handleFetch`, `handleValidationError`, `reroute` and server `init` hooks](https://svelte.dev/docs/kit/hooks) | Not supported | No Go equivalents are exposed. |
| [Server-side `fetch`](https://svelte.dev/docs/kit/load#Making-fetch-requests) | Limited | Rendering can call back to Go for same-origin endpoints; general application network I/O from JavaScript is not supported. |
| [Custom transport types](https://svelte.dev/docs/kit/hooks#Universal-hooks) | Available | Paired Go and Kit transport hooks preserve methods; [browser scenarios](example/e2e/features/transport.feature). |
| [Authentication and authorization](https://svelte.dev/docs/kit/auth) | Available | A Go `handle` hook supplies session locals and a layout load guards routes; [browser scenarios](example/e2e/features/auth.feature). |
| [Content Security Policy](https://svelte.dev/docs/kit/configuration#csp) | Available | Nonces cover rendered and streamed documents; [browser scenarios](example/e2e/features/csp.feature). |
| [CSRF and origin checks](https://svelte.dev/docs/kit/configuration#csrf) | Limited | Remote mutations check the build-time origin; [remote implementation](remote.go). Classic action rules are WIP. |
| [Environment variables and platform context](https://svelte.dev/docs/kit/environment-variables) | Limited | Go server code uses Go environment and request context; Kit's JavaScript server env modules and host-specific `platform` adapters are not skgo APIs. |
| [Server-only module isolation](https://svelte.dev/docs/kit/server-only-modules) | Limited | Kit's frontend build keeps its client/server module rules; application server logic belongs in Go, not JavaScript modules. |
| [Server asset `read`](https://svelte.dev/docs/kit/$app-server#read) | Not verified | The Kit export is present in the SSR bundle, but there is no demonstrated asset read through skgo's Go renderer. Application I/O belongs in Go. |
| [Server instrumentation and tracing](https://svelte.dev/docs/kit/observability) | Not supported | Kit's `instrumentation.server` JavaScript hook is not a Go hook. Use Go instrumentation in the host application. |

### Mutations and remote functions

| SvelteKit feature | skgo | Boundary or example |
| --- | --- | --- |
| [Remote queries and arguments](https://svelte.dev/docs/kit/remote-functions#query) | Available | Typed Go queries render on the server and answer client requests; [browser scenarios](example/e2e/features/remote.feature). |
| [Batch queries](https://svelte.dev/docs/kit/remote-functions#query.batch) | Available | Calls combine into one Go batch; [browser scenarios](example/e2e/features/batch.feature). |
| [Live queries](https://svelte.dev/docs/kit/remote-functions#query.live) | Available | Initial SSR value and subsequent pushes are [browser exercised](example/e2e/features/live.feature). |
| [Commands and single-flight refresh](https://svelte.dev/docs/kit/remote-functions#command) | Available | Go commands refresh requested queries in the same response; [browser scenarios](example/e2e/features/refresh.feature). |
| [Remote forms, validation and file uploads](https://svelte.dev/docs/kit/remote-functions#form) | Available | Typed fields, issues, dirty state and upload bytes are [browser exercised](example/e2e/features/form.feature). |
| [Remote forms without JavaScript](https://svelte.dev/docs/kit/remote-functions#form) | Available | Native POST returns a rendered page and survives hydration; [browser scenarios](example/e2e/features/form-noscript.feature). |
| [Classic page form actions](https://svelte.dev/docs/kit/form-actions) | WIP | Go `+page.server` default/named actions, `use:enhance` and their route behavior are being built in a separate task. |
| [Remote prerender functions](https://svelte.dev/docs/kit/remote-functions#prerender) | Not supported | There is no Go build-time remote-function execution path. |
| [TypeScript server implementations](https://svelte.dev/docs/kit/routing) | Not supported | Remote bodies, server loads, actions and endpoints must be Go; generated TypeScript stubs throw. |

### Build and developer tooling

| Capability | skgo | Boundary or example |
| --- | --- | --- |
| [Create a project with `sv`](https://svelte.dev/docs/kit/creating-a-project) | Available | `skgo new` delegates to `sv create` and adds the Go adapter, Vitest and Storybook. |
| Generate Go registrations and TypeScript types | Available | `skgo generate` produces typed bindings and throwing Kit stubs; [generator](internal/gen/gen.go). |
| Development server and hot updates | Available | Go serves documents while Vite serves modules and HMR; [browser scenarios](example/e2e/features/zz-source-update.feature). |
| Single-binary build and static assets | Available | The generated project's [build recipe](internal/newapp/gofiles/Justfile.tmpl) embeds the client, assets and SSR bundle in a Go binary. Node is a build-time dependency. |
| Go tests and browser scenarios | Available | `go test` and the [Gherkin/Playwright suite](example/e2e/features) cover the example; run both development and built-server modes. |
| Svelte and TypeScript checking | Available | The example has `svelte-kit sync && svelte-check`; [package script](example/web/package.json). This is not yet an integrated skgo command. |
| Vitest component tests and Storybook | Available | `skgo new` provisions both through upstream tooling; [scaffolder](internal/newapp/newapp.go). |
| Unified checking, linting and formatting | WIP | The separate agent-tooling task is developing a single skgo command; no `skgo check --fix` is released. |
| skgo MCP tools and versioned documentation server | WIP | The separate agent-tooling task is designing and implementing these; they are not shipped here. |
| Host-specific adapters and Node runtime deployment | Not supported | skgo uses its own adapter and a Go process; Kit's Node, serverless and edge adapters are different deployment targets. |

## Current limitations

- TypeScript server code. Remote functions, loads and API routes must be
  written in Go.
- Classic form actions are WIP. Use a remote `form` in released skgo versions.
- Prerendering a page whose route has a Go server load.
- Pointer, map and interface types in remote function signatures. Use value
  types, named structs or `polytype.Nullable`.
- `output.bundleStrategy: 'inline'` and `router.resolution: 'server'`.
- `sv` library templates, and add-ons that write JavaScript server code.
  `skgo new` refuses them.

## Start a project

You need Go, [VitePlus](https://viteplus.dev) (`vp`) and
[just](https://just.systems).

```sh
go install github.com/tylergannon/skgo/cmd/skgo@latest
skgo new myapp
cd myapp
just dev
```

`skgo new` runs `sv create` through VitePlus and asks `sv`'s usual questions.
Anything after `--` is passed to `sv create`.

## Build and deploy

```sh
just build      # go generate, vp build, go build → bin/myapp
./bin/myapp
```

The build compiles in the app's public origin, and skgo checks it on non-GET
remote calls. If the origin changes, rebuild. Behind a reverse proxy the listen
address can differ from the origin. Put your usual TLS proxy or load balancer in
front.

## Example app

| Route | Shows |
| --- | --- |
| `/todos` | Queries, commands, forms, live updates and refresh |
| `/account` | Nested Go server loads |
| `/api` | An API route answered by a `net/http` handler |
| `/stream` | Load values streamed into the page |
| `/plain`, `/spa` | `csr = false` and `ssr = false` |

From this repository:

```sh
just install && just build && just serve    # http://127.0.0.1:8080
```

## Links

- [Overview](https://tylergannon.github.io/skgo/)
- [Go package documentation](https://pkg.go.dev/github.com/tylergannon/skgo)
- [Agent skills](#agent-skills)

skgo is pronounced "skay-go". MIT licensed; see [LICENSE](LICENSE).
