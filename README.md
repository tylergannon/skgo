# skgo

**A Go backend for SvelteKit.**

skgo lets you write the server side of a SvelteKit app in Go. Remote functions,
server loads and API routes are Go code that sits next to the Svelte files that
use them. skgo generates TypeScript types and bindings from that code, so you
write a function in Go and await it in Svelte, the same way you would call any
SvelteKit remote function. In production, pages are rendered by SvelteKit's own
renderer running inside the Go process, and the app deploys as a single Go
binary. Node is needed for builds and for the Vite/Kit development server; it
is not needed to run the production binary.

> **Status:** pre-1.0. skgo uses SvelteKit 3.1.0, reviewed in the
> [compatibility journal](SVELTEKIT_COMPATIBILITY.md). All server code is written in Go: TypeScript remote functions,
> loads and API routes are not supported. The [example app](example/) shows what
> works today.

## Install

On Apple Silicon macOS, install the signed and notarized CLI with Homebrew:

```sh
brew install --cask tylergannon/skgo/skgo
```

Linux AMD64 and ARM64 archives are available from
[GitHub Releases](https://github.com/tylergannon/skgo/releases), alongside
`checksums.txt`. Extract the archive's `skgo` executable onto your PATH.

`skgo update` downloads the latest published release, verifies its checksum,
and installs that binary before aligning your toolchain and project. macOS
updates retain the release's Developer ID signature.

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

Use VitePlus (`vp`) for frontend tooling. SKGO declares the pins; vp manages the underlying package-manager version. Do not install or select pnpm separately. Add -g to install the skills for all projects. Then read the skgo skill before writing skgo code.
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
can build its client and manifests. In development, the adapter replaces those
bodies with callbacks to Go; application server logic stays in Go in both modes.

## For Go developers

- The production build embeds the frontend, its assets and server-rendering
  bundle in one binary. That binary needs no Node at request time.
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
- `skgo dev` runs Vite for frontend HMR and a Go application with native updates
  for compatible Go edits. Structural changes rebuild the Go host.
- `skgo new` creates the project with `sv create`, then adds Vitest and
  Storybook.

## Development

`skgo dev` currently requires **Go 1.27.2 exactly**, Node 22.17 or newer and the
project's VitePlus 1.3.0 environment. Its startup gate accepts only macOS ARM64
(`darwin/arm64`) and Linux AMD64 (`linux/amd64`) on the running platform;
Windows, Intel Macs, Linux ARM64 and other Go versions are rejected. These are
development-host restrictions, separate from production deployment. The native
host builds with `CGO_ENABLED=0`; applications requiring cgo cannot use it.
A nonempty `GOEXPERIMENT` and custom `GOFLAGS` for build tags, compiler flags
or race/memory sanitizers are rejected. Accepted platforms are not a claim of
complete platform qualification.

From an application root with its existing generation declaration, run:

```sh
go tool skgo dev
```

In this checkout, run `go run ./cmd/skgo dev --root example`. Install frontend
packages with `vp install` in the frontend directory first. Use `--web` and
`--out` for non-default frontend and generated-binding directories.

Open the address printed by the command. The Go coordinator keeps that public
listener while Vite/Kit owns development rendering and HMR, and Go supplies
application loads, actions, endpoints, remotes and request hooks. `--listen 127.0.0.1:8080` selects the requested address;
when the port is occupied, the next available port is selected before Kit is
initialized. Add `--strict-port` to require that exact port.

Compatible Go edits install into the running host and refresh Kit queries and
loads. Changes classified as requiring a full rebuild, including declaration,
initialization and import changes, replace the Go application; arbitrary
in-memory Go state need not survive. Native-loader failures do not always take
that rebuild path: an observed edit to a function with deferred
`sync.Mutex.Unlock` emitted raw `R_ADDROFF` relocation stderr and kept serving the
old code without a normal revision/Vite error notification, publication or
rebuild during a 75-second check. Recovery through another edit or a restart
has not been verified; see [issue #330](https://github.com/tylergannon/skgo/issues/330).
Svelte edits use Vite HMR. Compile errors show the authored location and
keep the last working application; repairing the source clears the error.
Browser draft preservation is not established: a recorded Svelte title edit
updated without manual reload but lost an unsent form draft. Vite restarts can
also reload the document. In-flight requests are not guaranteed to survive a
child-process crash.

The application provides `NewDevelopmentHandler(build fs.FS, origin string,
development *skgo.NativeDevelopment) (http.Handler, error)`. Share the ordinary
production constructor's setup and use `development.NewSSR(build, ...)` and
`development.NewStaticHandler(build, ...)` in that setup. Passing a nil
development value selects the production implementations. The filesystem stays
ordinary `fs.FS`, including decorators and subdirectories. Newly generated apps
include both entry points; existing apps must add the development entry point.

The production build still produces one Go binary with Go/goja rendering and
needs no Node at request time. Native Kit rendering is development-only.

Development has focused browser checks and a real two-session integration test,
but a complete browser-suite pass on this topology is not established. Direct
SSR subfetches over the private Go socket expose response cookies to server code
but do not automatically relay them into Kit's pending cookies or the outer page
response. This accepted limitation does not apply to every cookie path; qualify
your application's authentication flow in development as well as production.
The new topology also has no recorded latency benchmark or complete platform
qualification. The [development handoff](ephemeral/projects/dev-server/CONTINUE.md)
records the existing evidence, including an unexplained intermittent native
lifetime assertion; later passes do not establish its cause or repair.

## SvelteKit feature support

This matrix covers SvelteKit's server-facing features and the tooling needed to
build a skgo app. **Available** means the current Go implementation has a
working example or focused test; it does not promise every edge case of the
upstream API. **Limited** names a narrower supported case. **WIP** is active
work, not a released feature. **Not supported** means skgo has no compatible
implementation. **Not verified** means this repository does not yet establish
the behavior. The upstream links describe SvelteKit generally; skgo follows
the [pinned Kit 3 version](example/web/package.json), which may differ from
the public documentation. Browser evidence below is production evidence unless
explicitly identified as development evidence; it does not establish full parity
for the new development server.

### Pages and routing

| SvelteKit feature | skgo | Boundary or example |
| --- | --- | --- |
| [Svelte 5 pages, server rendering and hydration](https://svelte.dev/docs/kit/routing) | Available | In production, Kit's renderer runs in Go; [cold HTML and hydration](example/e2e/features/ssr.feature) are exercised. |
| [Client-side navigation](https://svelte.dev/docs/kit/routing) | Available | Kit's client navigates without a document reload; [browser scenario](example/e2e/features/app.feature). |
| [Layouts, route groups and nested error pages](https://svelte.dev/docs/kit/routing) | Available | [Grouped routes](example/e2e/features/colocation.feature), [nested loads](example/e2e/features/loads.feature) and [error boundaries](example/e2e/features/showcase.feature). |
| [Dynamic and rest route parameters](https://svelte.dev/docs/kit/advanced-routing) | Available | Parameters reach colocated Go handlers; [browser scenarios](example/e2e/features/colocation.feature). |
| [Optional parameters and parameter matchers](https://svelte.dev/docs/kit/advanced-routing) | Available | Optional captures and paired Go matchers select loads and endpoints; matcher rejection tries the next route before 404. [Client navigation scenarios](example/e2e/features/routing-options.feature) run in development and production. |
| [Page and layout server loads](https://svelte.dev/docs/kit/load) | Available | Go loads produce typed `data` in documents and `__data.json`; [browser scenarios](example/e2e/features/loads.feature). |
| [Universal JavaScript loads](https://svelte.dev/docs/kit/load#Universal-vs-server) | Not verified | Page options in `+page.ts` work, but there is no demonstrated universal `load`; application I/O in JavaScript is outside skgo's contract. |
| [Parent load data, URL, params and route ID](https://svelte.dev/docs/kit/load) | Available | Go exposes `Parent` and request data through `EventFrom`; [nested example](example/web/src/routes/account/page.server.go). |
| [Load dependencies, invalidation and untrack](https://svelte.dev/docs/kit/load#Rerunning-load-functions) | Limited | Go has `Depends` and `Untrack`; the [browser suite](example/e2e/features/loads.feature) proves reuse of a layout load, not every invalidation path. |
| [Streaming load promises](https://svelte.dev/docs/kit/load#Streaming-with-promises) | Available | Deferred Go values stream into HTML and client navigation; [browser scenarios](example/e2e/features/stream.feature). |
| [Expected errors, redirects and error pages](https://svelte.dev/docs/kit/errors) | Available | Go load and remote outcomes use Kit's page/error behavior; [browser scenarios](example/e2e/features/ssr.feature). |
| [`ssr = false` and `csr = false`](https://svelte.dev/docs/kit/page-options) | Available | SPA fallback and script-free HTML are [browser exercised](example/e2e/features/ssr.feature). |
| [Prerendering](https://svelte.dev/docs/kit/page-options#prerender) | Available | Kit renders Go loads at build time; Go serves the emitted static files and redirects. |
| [Dynamic prerender entries](https://svelte.dev/docs/kit/page-options#entries) | Available | `skgo.Entries` enumerates raw route parameters from Go during Kit analysis; [static serving contract](example/server_test.go). |
| [Trailing slash and route config options](https://svelte.dev/docs/kit/page-options) | Limited | Page `trailingSlash` supports `never`, `always` and `ignore`, including layout inheritance and overrides; [routing contracts](example/routing_options_test.go) and [client navigation](example/e2e/features/routing-options.feature) cover development and production. Endpoint-only routes use Kit's default `never`; authored server-only slash overrides and nonempty adapter `config` are refused at generation/build with a source or route diagnostic. |

### Requests and server behavior

| SvelteKit feature | skgo | Boundary or example |
| --- | --- | --- |
| [`+server` endpoints and HTTP methods](https://svelte.dev/docs/kit/routing#+server) | Available | `net/http` handlers serve Kit routes, including `QUERY`; [browser scenarios](example/e2e/features/routes.feature). |
| [Page, endpoint and action on one route](https://svelte.dev/docs/kit/routing#+server) | Available | Go endpoints and classic actions share a route; [handler contracts](example/action_protocol_test.go) and [browser scenarios](example/e2e/features/actions-shared.feature). |
| [Cookies and request-local state](https://svelte.dev/docs/kit/load#Cookies) | Available | `EventFrom`, `SetCookie` and typed locals support the [sign-in example](example/e2e/features/auth.feature). |
| [Response headers from loads](https://svelte.dev/docs/kit/load#Headers) | Available | Go `Event.SetHeader` follows Kit's duplicate-header and cookie rules; [implementation](loadevent.go). |
| [`handle` and `handleError` hooks](https://svelte.dev/docs/kit/hooks) | Available | Go hooks cover requests and public errors; [example](example/server.go). |
| [`reroute`](https://svelte.dev/docs/kit/hooks#Universal-hooks-reroute) | Available | A Go hook selects routes and params while preserving the alias URL; Kit's universal forwarding hook keeps client navigation consistent. [Handler contracts](example/reroute_test.go) and [browser navigation](example/e2e/features/reroute.feature) cover development and production. |
| [Remote validation errors](https://svelte.dev/docs/kit/hooks#Server-hooks-handleError) | Available | Kit 3 uses `handleError` with kind `validation`; Go hooks choose public message and extra fields before the remote body runs. |
| [Server `init`](https://svelte.dev/docs/kit/hooks#Server-hooks-init) | Not supported | Initialize shared Go state in application setup. |
| [Server-side `fetch`](https://svelte.dev/docs/kit/load#Making-fetch-requests) | Limited | Go exposes request-aware fetch and `HandleFetch`; [implementation](fetch.go). Direct SSR socket subfetch cookie relay is limited in development; see [Development](#development). Application network I/O belongs in Go. |
| [Custom transport types](https://svelte.dev/docs/kit/hooks#Universal-hooks) | Available | Paired Go and Kit transport hooks preserve methods; [browser scenarios](example/e2e/features/transport.feature). |
| [Authentication and authorization](https://svelte.dev/docs/kit/auth) | Available | A Go `handle` hook supplies session locals and a layout load guards routes; [browser scenarios](example/e2e/features/auth.feature). |
| [Content Security Policy](https://svelte.dev/docs/kit/configuration#csp) | Available | Nonces cover rendered and streamed documents; [browser scenarios](example/e2e/features/csp.feature). |
| [CSRF and origin checks](https://svelte.dev/docs/kit/configuration#csrf) | Limited | Remote mutations check the build-time origin; [remote implementation](remote.go). Classic action origin and content-type rules have [handler contracts](example/action_protocol_test.go). |
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
| [Classic page form actions](https://svelte.dev/docs/kit/form-actions) | Available | Go default/named actions, `use:enhance` and script-free posts have [browser scenarios](example/e2e/features/actions.feature) and [handler contracts](example/action_protocol_test.go). |
| [Remote prerender functions](https://svelte.dev/docs/kit/remote-functions#prerender) | Not supported | There is no Go build-time remote-function execution path. |
| [TypeScript server implementations](https://svelte.dev/docs/kit/routing) | Not supported | Remote bodies, server loads, actions and endpoints must be Go; generated stubs throw in production and delegate to Go in development. |

### Build and developer tooling

| Capability | skgo | Boundary or example |
| --- | --- | --- |
| [Create a project with `sv`](https://svelte.dev/docs/kit/creating-a-project) | Available | `skgo new` delegates to `sv create` and adds the Go adapter, Vitest and Storybook. |
| Generate Go registrations and TypeScript types | Available | `skgo generate` produces typed bindings and throwing Kit stubs; [generator](internal/gen/gen.go). |
| Development server and hot updates | Limited | `skgo dev` combines Vite HMR, native Go body updates, rebuild fallback and process recovery; see [Development](#development). |
| Single-binary build and static assets | Available | The generated project's [build recipe](internal/newapp/gofiles/Justfile.tmpl) embeds the client, assets and SSR bundle in a Go binary. Node is a build-time dependency. |
| Go tests and browser scenarios | Available | `go test` and the [Gherkin/Playwright suite](example/e2e/features) cover the example; run `just e2e prod` against the built server. The development session has a real Go integration test (`go test -tags skgo_dev_integration ./internal/devserver`); full browser qualification of the new development topology remains incomplete. |
| Svelte and TypeScript checking | Available | The example has `svelte-kit sync && svelte-check`; [package script](example/web/package.json). This is not yet an integrated skgo command. |
| Vitest component tests and Storybook | Available | `skgo new` provisions both through upstream tooling; [scaffolder](internal/newapp/newapp.go). |
| Unified checking, linting and formatting | WIP | The separate agent-tooling task is developing a single skgo command; no `skgo check --fix` is released. |
| skgo MCP tools and versioned documentation server | WIP | The separate agent-tooling task is designing and implementing these; they are not shipped here. |
| Host-specific adapters and Node runtime deployment | Not supported | skgo uses its own adapter and a Go process; Kit's Node, serverless and edge adapters are different deployment targets. |

The SvelteKit transport runtime is `github.com/tylergannon/devalue/v5`
(v5.0.0, parity with JavaScript devalue 5.9.4). Polytype v1.6.0 generates
its typed codecs. Applications that imported `github.com/tylergannon/polytype/devalue`
directly must move those imports to `github.com/tylergannon/devalue/v5` and
regenerate their skgo bindings.

## Current limitations

- TypeScript server code. Remote functions, loads and API routes must be
  written in Go.
- Development requires the exact toolchain and host platforms listed above.
  Full development browser qualification and draft preservation remain open;
  direct SSR socket subfetch response cookies are not automatically relayed.
- Pointer, map and interface types in remote function signatures. Use value
  types, named structs or `polytype.Nullable`.
- `output.bundleStrategy: 'inline'` and `router.resolution: 'server'`.
- `sv` library templates, and add-ons that write JavaScript server code.
  `skgo new` refuses them.

## Start a project

You need Go, [VitePlus](https://viteplus.dev) **1.3.0** (`vp`) and
[just](https://just.systems). This release qualifies that exact VitePlus toolchain;
`skgo new` refuses other bootstrap versions before project creation. It uses
vp’s managed environment with this release’s pnpm 12.14.0 companion and records
that selection in the generated frontend, regardless of ambient pnpm settings.

```sh
curl -fsSL https://vite.plus | VP_VERSION=1.3.0 bash
go install github.com/tylergannon/skgo/cmd/skgo@latest
skgo new myapp
cd myapp
just install
just serve
```

`skgo new` runs `sv create` through VitePlus and asks `sv`'s usual questions.
Anything after `--` is passed to `sv create`.

## Updating an application

Run `skgo update` with Go installed to install the latest released CLI and let
that new executable align global `vp`, the project's Go requirement, adapter,
SvelteKit patch and frontend toolchain. The release selects an exact vp version
and its qualified pnpm companion; commands run through vp's managed environment.
Use `--vp /path/to/vp` to select the global installation explicitly.

Inside an existing VitePlus application, update stages dependency alignment
without running whole-project migration. It rejects changes to authored sources
or configuration, then installs, regenerates and builds using
the verified project Go tool when declared. Failures report an incomplete update;
earlier CLI or dependency changes are not rolled back. Outside an application
only the CLI and global vp are updated. Templates are never replayed. Native
plugin updates and builds are explicit: install a prebuilt native-shell release
matching the updated official host, then run `skgo plugin native-shell build`.

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
supply `skgo add --name NAME` explicitly. Core no longer checks native-specific
identity before creating that file; choose a name matching existing native
configuration. Add-ons must report partial writes and
conflicts; skgo stops at the failed step without rolling back third-party work.

Plugins are trusted code, loaded only by the development CLI. Loading help can
run their Go initializers. They must match the executing binary's Go toolchain,
shared dependency sources and build settings, on a cgo-enabled macOS or Linux
host. A same-version standalone CLI and `go tool skgo` can have different module
graphs. Inspect the actual host using `skgo buildinfo --json` or
`go tool skgo buildinfo --json`; `skgo env` remains application configuration
resolution.

Install a source module for the executable you are using:

```sh
skgo plugin install github.com/tylergannon/skgo-template-voice-recorder
skgo new --help
CI=1 skgo new --name FieldNotes --module example.com/field-notes \
  --template voice-notes --set bundle-id=com.example.fieldnotes field-notes

# The project tool may have a different executable and dependency graph.
cd field-notes
go tool skgo plugin install github.com/tylergannon/skgo-template-voice-recorder
go tool skgo add --help
```

`MODULE@VERSION`, branch and commit queries are supported; an omitted query means
Go's `latest`. Output identifies the resolved version, exact Git commit, actual
host executable/Go version/SHA256 and installed exports. The installer requires
Git, the host's exact Go toolchain and cgo. It resolves through canonical Git
using `GOPROXY=direct`, preserving your checksum/private-module and GOVCS policy,
and reports whether checksum-database validation is enabled or exempted. Before
running source, it compares the checkout's module zip and go.mod hashes with Go's
resolution. Nested module inputs come from that same canonical Git commit.
Unavailable provenance, checksums or source access are errors; archives and
precompiled binary installation are not supported.

Managed files live at `~/.skgo/plugins/hosts/<executable-sha256>/<module-sha256>.so`.
Ordinary help/new/add discover only this host's managed directory automatically.
An updated or differently built host needs another install; help lists retained
other-host source references without loading those binaries or choosing a version
for you. Old host directories remain for deliberate manual cleanup. Remove a
module's selection for the current host with `skgo plugin remove MODULE`; this
also removes its source reference and does not edit an application or other host.
Installation qualifies the candidate alongside currently discoverable plugins in
a fresh host process before replacing the module's prior selection. Unrelated
stale plugins warn; conflicts involving the candidate prevent publication.

Plugin authors put `{"package":"./template"}` in module-root `skgo-plugin.json`
(`.` is also valid). Unknown fields and paths outside that module are rejected.
Export `SKGoPluginV1` and declare `var skgoVersion string` in the main package,
using it in `Describe`; the builder injects the actual host's version. Preparation
is ordinary `go generate <package>`, with no JSON hooks. The builder writes host
shared dependency pins, runs `go mod tidy -e` to prepare generator sums while
preserving declared requirements, generates, runs strict `go mod tidy`, verifies the shared graph,
and compiles, all under the host's exact Go toolchain with `GOWORK=off`.
Declare generator dependencies and generated-source requirements in go.mod.
Module mode (`-mod=mod`) ignores committed vendor trees. Direct source resolution
uses a temporary cache; compilation uses your ordinary cache and proxy settings.
An unreproducible development host without recorded source replacement is rejected.
The writable stage has no `.git`, `node_modules`, `ephemeral`, `.local`, `.cache`,
`build`, `.build` or `.so` files, and rejects source symlinks; generators must not
rely on `git describe`, ignored assets or excluded directories. Generation cannot
require a newer Go toolchain than the host. Source and consumer modules remain
untouched. Initialization and generation execute trusted code without sandboxing.

For an existing local source checkout, the manual helper remains available. It
builds already-prepared source and qualifies that plugin in isolation:

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
comma-separated value replaces that manual search; it does not disable managed
installations. Manual directories load first, then the current managed directory,
with sorted immediate `.so` files and no recursion. For the examples, set `SKGO_PLUGIN_DIRS=~/.skgo/plugins/standalone`
for standalone commands or `SKGO_PLUGIN_DIRS=~/.skgo/plugins/myapp` for the
project tool. Missing explicit directories are errors. Failed plugins warn and
are excluded; their exports are unknown. Duplicate IDs invalidate those loaded
plugins; duplicate names among loaded plugins are ambiguous and fail commands
that select them. Go may reject a second copy before its descriptor can be read;
diagnostics identify the first loaded file and the rejected copy. Avoid keeping a
manual and managed copy of the same plugin. Remove the manual file or use
`skgo plugin remove MODULE`; changing `SKGO_PLUGIN_DIRS` cannot hide a managed copy.
Ordinary untemplated creation does not load plugins. Generated apps do not need
plugins installed to build or run.

## Build and deploy

Go build tools can embed the same generator as the CLI through
[`github.com/tylergannon/skgo/generate`](generate/generate.go). Call `Run` for
ordinary generation, or `Prepare`, inspect `Prepared.Packages()` for your own
analysis, then call `Prepared.Run` and `Prepared.Close`. The prepared graph
shares the parsed remote/route source and its type information with the caller.

`Run` and `skgo generate` cache successful, formatted generation plans in the
user cache directory. An unchanged invocation checks current source contents and
live Kit route/matcher metadata, then restores its generated files without Go
analysis or formatting. Owned generated output is disposable: missing or broken
previous output does not become an analysis input. Set `Config.CacheDir` (or
`--cache-dir`) to a custom directory, or `off` to disable caching.
`SKGO_CACHE_DIR` overrides the default when no directory is supplied. `Prepare` and
`Check` always analyze current source.

Cache misses analyze a captured copy of local modules, including workspace and
local replacement dependencies. Private analysis enables Go's `-trimpath` to
reuse compiler artifacts across temporary roots; explicit caller flags still
take precedence. Additions, deletions, dependency source,
configuration, tool identity and observed Kit metadata invalidate the plan.
Installed Go toolchains, versioned module-cache content and installed Node
packages follow their tools' immutability convention; reinstall or upgrade them
rather than editing their installed source in place. External package drivers,
caller package snapshots/loaders, vendoring, external Go overlays/modfiles, and
reached local cgo packages use ordinary generation without a persistent result.
An unavailable or corrupt cache is optional state and does not prevent generation.

Build tools can share a caller-owned `polytype/goload.Session` through
`Config.Session`. It reuses compatible analysis within an invocation on cache misses and remains
unused on persistent hits. Captured Go source bytes go directly to its parser;
private captured modules and original source paths remain separate views.

Callers that already loaded plain domain packages can supply
`NewPackageSnapshot` results through `Config.Packages` to reuse their type
projections. For Polytype projections, load with
`BuildFlags: []string{"-tags=jsonschema"}` and `grammar.LoadMode`, adding
`packages.NeedDeps` when imported types also need source. Matching is
conservative: source, load configuration, and flags must match exactly.
Incompatible snapshots fall back to normal loading.

Disk-loaded packages containing `skgo_gen.go`, including most route and locals
packages, generally cannot be reused: generation replaces previously generated
Go with intermediate overlays before analyzing current source. Use `Prepare`
and its discovery graph when your tool needs to share route/remote analysis.
`Config.OnPackageLoad` reports the fallback package patterns and configuration,
so callers can see which loads remain; it does not report a mismatch reason.

Keep snapshots and their source immutable for the invocation, and obtain fresh
snapshots after edits. Installed toolchain and module-cache source must remain
unchanged. Reuse is useful when the embedding tool already needs compatible
packages; preloading solely for generation adds work, and the API does not
guarantee a reduction in total generation time.

```sh
just build      # go generate, vp build, go build → bin/myapp
./bin/myapp
```

The build compiles in the app's public origin, and skgo checks it on non-GET
remote calls. If the origin changes, rebuild. Behind a reverse proxy the listen
address can differ from the origin. Put your usual TLS proxy or load balancer in
front.

Generated servers handle `SIGTERM` and interrupt by closing the listener and
draining active HTTP requests for up to ten seconds. At the deadline, remaining
connections close, including unfinished live-query or deferred-response streams.
Put application initialization and deferred cleanup, such as flushing a tracing
provider, in `run`; its defers finish before `main` reports an error and exits.
Keep your platform's termination grace period longer than the shutdown timeout.

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


### Native application plugin

Swift generation and the existing Apple shell live in the independently versioned
[`plugins/native-shell`](plugins/native-shell) module. Install its prebuilt `.so`
for the exact official skgo release; run its commands under
`skgo plugin native-shell`. Core web generation, build and development do not
load the native implementation. A core release alone does not publish a native
companion; native artifacts require their own qualification and release.

The plugin carries its Swift/Zig runtime and shell resources. Client generation
does not require shell setup. See the [plugin contract guide](templateapi/plugin-capabilities.md)
and [native-shell guide](plugins/native-shell/native-shell.md) for installation,
configuration, build limitations and the extraction's deliberate subtractions.
