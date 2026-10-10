# skgo

**A Go backend for SvelteKit.**

skgo lets you write the server side of a SvelteKit app in Go. Remote functions,
server loads and API routes are Go code that sits next to the Svelte files that
use them. skgo generates TypeScript types and bindings from that code, so you
write a function in Go and await it in Svelte, the same way you would call any
SvelteKit remote function. Pages are rendered by SvelteKit's own renderer
running inside the Go process, and the app deploys as a single Go binary. Node
is only needed to build it.

> **Status:** pre-1.0. skgo uses SvelteKit 3.0.0, reviewed in the
> [compatibility journal](SVELTEKIT_COMPATIBILITY.md). All server code is written in Go: TypeScript remote functions,
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

The SvelteKit transport runtime is `github.com/tylergannon/devalue/v5`
(v5.0.0, parity with JavaScript devalue 5.9.4). Polytype v1.5.0 generates
its typed codecs. Applications that imported `github.com/tylergannon/polytype/devalue`
directly must move those imports to `github.com/tylergannon/devalue/v5` and
regenerate their skgo bindings.

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

You need Go, [VitePlus](https://viteplus.dev) **1.0.0** (`vp`) and
[just](https://just.systems). This release qualifies that exact VitePlus toolchain;
`skgo new` refuses other bootstrap versions before project creation. It uses
vp’s managed environment with this release’s pnpm 12.9.1 companion and records
that selection in the generated frontend, regardless of ambient pnpm settings.

```sh
npm install --global vite-plus@1.0.0
go install github.com/tylergannon/skgo/cmd/skgo@latest
skgo new myapp
cd myapp
just dev
```

`skgo new` runs `sv create` through VitePlus and asks `sv`'s usual questions.
Anything after `--` is passed to `sv create`.

## Updating an application

Run `skgo update` with Go installed to install the latest released CLI and let
that new executable align global `vp`, the project's Go requirement, adapter,
SvelteKit patch and frontend toolchain. The release selects an exact vp version
and its qualified pnpm companion; commands run through vp's managed environment.
Use `--vp /path/to/vp` to select the global installation explicitly.

Inside an application, update stages frontend migration, rejects changes to
authored sources or configuration, then installs, regenerates and builds using
the verified project Go tool when declared. Native applications also regenerate
Swift bindings and build macOS by default; select another build with
`--native-platform simulator --native-preset synthetic`. A successful native
build does not establish device installation or runtime behavior. Failures report
an incomplete update; earlier CLI or dependency changes are not rolled back.
Outside an application only the CLI and global vp are updated. Templates are
never replayed, and native plugin binaries must be rebuilt for the updated host.

## Application add-ons and templates

A native Go plugin exports `func SKGoPluginV1() templateapi.Plugin` from
[`templateapi`](templateapi/template.go). `Describe` supplies help, options and
ordered recipes; `Apply` performs one application change. A template applies
those same add-ons after ordinary minimal TypeScript creation:

```sh
skgo new --template example --set message=Hello myapp
cd myapp
skgo add --set text=Hello receipt
```

The names above must be supplied by an installed plugin. See the
[small external-plugin fixture](cmd/skgo/testdata/template-plugin/plugin.go)
for an implementation that preserves existing files. Recorder template qualification remains part of
[the application composition work](https://github.com/tylergannon/skgo/issues/293);
this interface alone does not establish Voice Notes template delivery.

`skgo new --help`, `skgo add --help`, `skgo add NAME --help` and
`skgo new --template NAME --help` list compatible exports and their options.
`--set NAME=VALUE` is repeatable; put flags before the destination/add-on name.
New projects store application identity in `skgo.json`. Older projects must
supply `skgo add --name NAME` explicitly; existing native identity requires
verification before creating that file. Add-ons must report partial writes and
conflicts; skgo stops at the failed step without rolling back third-party work.

Plugins are trusted code, loaded only by the development CLI. Loading help can
run their Go initializers. They must match the executing binary's Go toolchain,
shared dependency sources and build settings, on a cgo-enabled macOS or Linux
host. A same-version standalone CLI and `go tool skgo` can have different module
graphs. Inspect the actual host using `skgo buildinfo --json` or
`go tool skgo buildinfo --json`; `skgo env` remains application configuration
resolution.

Use the source-build helper to stage the plugin module, align its shared graph
and settings, compile with the host's recorded Go toolchain, and check that the
result loads into that host before installing it:

```sh
# Standalone CLI; plugin main declares var skgoVersion string and uses it in Describe.
go run github.com/tylergannon/skgo/cmd/skgo-plugin-build@latest \
  --host "$(command -v skgo)" --source ./plugin-source --package ./template \
  --out "$HOME/.skgo/plugins/standalone/example.so"

# A project's go-tool host. Keep its build in its own directory.
go run github.com/tylergannon/skgo/cmd/skgo-plugin-build@latest \
  --project ./myapp --source ./plugin-source --package ./template \
  --out "$HOME/.skgo/plugins/myapp/example.so"
```

The helper does not change the source or consumer `go.mod`. `--version-symbol`
defaults to `main.skgoVersion`; `--skgo-source` supplies an explicit checkout
when qualifying an unreleased host. The source directory needs its own Go module;
symlinks and unreproducible relative replacements are rejected. Staging excludes
`.git`, `node_modules`, `ephemeral`, `.local`, `.cache`, `build`, `.build` and
existing `.so` output. Plugin-only dependencies must also agree between plugins;
the helper's one-plugin qualification does not prove coexistence.

Unset/empty `SKGO_PLUGIN_DIRS` searches `~/.skgo/plugins`. A nonempty
comma-separated value replaces it; directories are scanned immediately, without
recursion. For the examples, set `SKGO_PLUGIN_DIRS=~/.skgo/plugins/standalone`
for standalone commands or `SKGO_PLUGIN_DIRS=~/.skgo/plugins/myapp` for the
project tool. Missing explicit directories are errors. Failed plugins warn and
are excluded; their exports are unknown. Duplicate IDs invalidate those loaded
plugins; duplicate names among loaded plugins are ambiguous and fail commands
that select them. Diagnostics identify the files to remove or isolate.
Ordinary untemplated creation does not load plugins. Generated apps do not need
plugins installed to build or run.

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


### Native application foundation

`skgo add --set bundle-id=dev.example.myapp native-app` adds an embedded Go
host and a SwiftUI shell for macOS and iOS. Build on macOS with Xcode and the
native tool versions declared in skgo's `native/mise.toml`:

```sh
go tool skgo native build --platform macos
go tool skgo native build --platform simulator
go tool skgo native build --platform iphone
```

`native/skgo-native.json` is the application configuration (`nativeapp.Config`).
The builder compiles `native/Sources` and, when `swiftRemotes` is nonempty,
generates and compiles `native/Generated.swift`. Additional local Swift packages,
sources, resources, platform deployment targets and Info.plist properties come
from that configuration. Named presets select Go tags, bundle suffixes, resource
sets and additional plist properties with `--preset NAME`; output goes to
`native/build/<platform>[-<preset>]`. A null/omitted preset resources list inherits
the application resources; an explicit empty list clears them. Unknown configuration
fields fail explicitly. Native compilation does not install or launch the result.

The default `native/Sources/ApplicationView.swift` is an application-owned slot.
Its exact initial bytes are exposed as `nativeapp.DefaultApplicationView` so an
add-on can replace only the recognized default. It defines one retained
`ApplicationState`, the content view and macOS settings view. `NativeHost` offers
shutdown, background-change and availability callbacks. Listener recovery retains
the Go application's state and loopback origin; recording and WebView-specific
readiness remain the application's responsibility.

Builds are unsigned unless `SKGO_DEVELOPMENT_TEAM` is explicitly configured;
`SKGO_DEVICE_ID` optionally chooses a signing destination. An archive or successful
build is not evidence of distribution or physical-device behavior.
