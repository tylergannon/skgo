# Investigation trail and reproduction

This records observable investigation work and its results: source reads, dependency inventory, primary-source checks, executable experiments, user-driven corrections and untested boundaries. It is more detailed than the assessment so a continuation can recover the evidence without repeating the investigation.

## 1. Establish the implementation being assessed

The root checkout was clean on main at `618f467b`, whose subject is the native-development/release alignment with Go 1.27.3. A managed worktree was created at that exact commit; the root checkout was used for orientation and source reading, with all new material confined to the worktree.

The research worktree became branch `codex/zig-dependency-analysis`. No product files were edited. The initial research commit was `1069dca9`; the subsequent handoff commit expands documentation.

Repository and skill instructions consulted:

- Root `AGENTS.md`: Kit as spec, plain Kit frontend, Go-owned application I/O, Vite development, `vp` frontend tooling, no unauthorized `docs/` writes.
- The session-start agent protocol and session-worklog skill: worktree isolation, tracked temporary material, and preserving consequential discoveries.
- `ephemeral/sveltekit-current/SKILL.md`: Kit 3 behavior that invalidates older SvelteKit assumptions.

A quick prior-context search looked for SKGO, Zig, goja, dependencies, native generation and route colocation. Historical notes helped identify the relevant implementation areas. They were not treated as current source authority: old route-colocation notes described a nested `go.mod`, while current code uses Go 1.27 `ignore` entries and directory symlinks.

The investigation used one agent. No delegated implementation or independent reviewer was involved.

## 2. Separate manifests from actual dependencies

Read these manifests:

- [`go.mod`](../../go.mod)
- [`go.work`](../../go.work)
- [`example/go.mod`](../../example/go.mod)
- [`example/web/package.json`](../../example/web/package.json)
- [`internal/adapter/package.json`](../../internal/adapter/package.json)
- [`plugins/cli/go.mod`](../../plugins/cli/go.mod)
- [`plugins/native-shell/go.mod`](../../plugins/native-shell/go.mod)

The shell commands in this document are from repository root unless stated otherwise.

The root module pins Go 1.27.3 and declares these direct dependencies:

| Module | Pinned version | Role |
| --- | --- | --- |
| github.com/dop251/goja | v0.0.0-20261016120441-313b6b662e53 | Production JS engine |
| github.com/dop251/goja_nodejs | v0.0.0-20260918173711-b481721df8a2 | URL web globals |
| github.com/fsnotify/fsnotify | v1.10.1 | Filesystem watching |
| github.com/tylergannon/devalue/v5 | v5.0.0 | Native Kit wire encoding |
| github.com/tylergannon/polytype | v1.10.0 | Native type grammar, projection/codecs |
| go.opentelemetry.io/otel | v1.47.0 | Tracing APIs/propagation |
| go.opentelemetry.io/otel/sdk | v1.47.0 | SDK used by tests/sample tracing |
| go.opentelemetry.io/otel/trace | v1.47.0 | Trace types |
| golang.org/x/mod | v0.42.0 | Go module/plugin/update tooling |
| golang.org/x/sys | v0.49.0 | Platform integration |
| golang.org/x/term | v0.47.0 | Terminal interaction |
| golang.org/x/tools | v0.52.0 | Go packages/analysis |
| gopkg.in/yaml.v3 | v3.0.1 | Lockfile/config/update processing |

There are 16 indirect root requirements. To distinguish actual runtime/CLI imports from module declarations, ran `go list -deps -json` separately for the library and CLI and grouped non-standard packages by their owning module.

Observed import graph, excluding the local SKGO module:

| External module | Library package count | CLI package count |
| --- | ---: | ---: |
| github.com/cespare/xxhash/v2 | 1 | 1 |
| github.com/dave/dst | 0 | 6 |
| github.com/dlclark/regexp2/v2 | 3 | 3 |
| github.com/dop251/goja | 8 | 8 |
| github.com/dop251/goja_nodejs | 3 | 3 |
| github.com/fsnotify/fsnotify | 0 | 2 |
| github.com/go-logr/logr | 2 | 2 |
| github.com/go-logr/stdr | 1 | 1 |
| github.com/go-sourcemap/sourcemap | 2 | 2 |
| github.com/google/pprof | 1 | 1 |
| github.com/tylergannon/devalue/v5 | 1 | 1 |
| github.com/tylergannon/polytype | 2 | 13 |
| github.com/tylergannon/structtag | 0 | 1 |
| go.opentelemetry.io/auto/sdk | 2 | 2 |
| go.opentelemetry.io/otel | 13 | 13 |
| go.opentelemetry.io/otel/log | 2 | 2 |
| go.opentelemetry.io/otel/metric | 2 | 2 |
| go.opentelemetry.io/otel/trace | 4 | 4 |
| golang.org/x/mod | 0 | 6 |
| golang.org/x/net | 1 | 1 |
| golang.org/x/sync | 0 | 1 |
| golang.org/x/sys | 0 | 1 |
| golang.org/x/term | 0 | 1 |
| golang.org/x/text | 13 | 13 |
| golang.org/x/tools | 0 | 30 |
| gopkg.in/yaml.v3 | 0 | 1 |

Library: 18 external modules, 220 standard-library packages, 7 local packages. CLI: 26 external modules, 251 standard-library packages, 29 local packages. A package being reachable here does not prove all of its code survives linking or consumes meaningful runtime resources.

The sample installs OpenTelemetry SDK tracing; the root production library graph did not include that SDK. Optional CLI/MCP dependencies include Cobra and mcp-go. Those plugins are separate modules rather than inherent dependencies of every application.

The native hot-reload source exposes another class of dependencies that root `go.mod` alone misses: it materializes a private goloader source copy and purego dependency for the native development host. Inspecting how tools construct private modules is necessary for a complete dependency analysis.

For scale, a Python source walk counted non-test `.go` lines, excluded `.git`, `ephemeral`, `node_modules`, `_links` and `testdata`, and skipped files with an early `Code generated` marker. It grouped root-package files separately from two-component directory prefixes. The relevant results were 18,558 root-runtime lines, 12,056 generation lines, 2,694 hot-reload lines, 1,601 dev-server lines and 1,436 SSR-engine lines. This counts comments and says nothing directly about migration effort.

## 3. Read the implementation boundaries

The following source areas were inspected because they answer different parts of the migration question:

| Current source | What it established |
| --- | --- |
| `internal/gen/links.go`, `example/go.mod` | Why Go cannot import Kit punctuation paths, current directory-symlink workaround and module ignore mechanism |
| `internal/gen/scan.go`, `types.go`, `clients.go`, `emit.go`, `transport.go` | Compiler-backed discovery, signature/type validation, Polytype projection, native bindings/codecs, TS stubs and transport handling |
| `internal/gen/shared_params.go`, `application_config.go` | Further native type/config validation beyond simple remote input/output |
| `internal/gen/retained.go`, `retained_watch.go`, `retained_future.go`, `snapshot.go` | Watching, retained analysis, coalescing/cancellation and package input reuse |
| `templateapi/generator.go`, `internal/gen/plugins.go` | Existing plugin model exposes Polytype grammar roots/definitions and remote operations |
| `internal/hotreload/toolchain.go`, `compile.go`, `lower.go`, `loader.go`, `cache.go` | Version-bound Go object-reader preparation, package analysis, function rewriting, candidate compilation, loader materialization and caches |
| `internal/devserver/session.go`, `router.go` | Generation/build orchestration and stable request routing to app/frontend targets |
| `internal/ssr/engine.go`, `async.go`, `globals.go`, `webglobals.go` | Runtime pools, nested render depth, bounded asynchronous host work, promise completion, web globals and URL binding |
| `internal/ssr/engine_development.go` | Native dev host omits goja and Vite renders development pages |
| `internal/adapter/skgo-adapter/env.js`, `entry.js`, `app-server.js`, `polyfill.js` | Kit/Vite environment integration, SSR bundle, host bridges, web API polyfills and goja parser lowering |
| `remote_live.go`, `data.go`, `deferred.go` | SSE framing, heartbeat and cancellation; streamed Kit data; deferred nested values |
| `fetch.go`, `document_fetch.go`, middleware/document origin handling | Internal versus external fetch, native HTTP client, credentials and external-origin semantics |
| `example/cmd/main.go` | Plain TCP listener handed to `http.Server.Serve`; graceful process shutdown |
| Optional CLI/native-shell plugin code | Other code generators and packaging that would remain separate migration scope |

The Go SSR engine is single-owner per runtime. Worker goroutines return native completion data; they do not manipulate JS values directly. Nested renders can require different pools to avoid a render waiting for another runtime while all runtimes are held. These are actual architecture constraints worth carrying into a new engine host, even if the dev hot-replacement requirement is dropped.

The URL module is loaded directly rather than exposing a general Node `require` loader to the JS engine. That preserves the intended application-I/O boundary. A JavaScriptCore port should make an equivalent conscious choice about which capabilities JavaScript receives.

The adapter's modern Svelte renderer preserves private fields. Its per-module lowering exists for syntax goja rejects, including async generators/for-await. Replacing the engine could remove this compatibility layer; removing all adapter code does not follow from that. Kit environment definitions, remote identity/build manifests, prerendering and host bridges remain responsibilities.

## 4. Correct the local Kit-version mismatch before concluding protocol details

The checked-in frontend manifest and current-Kit skill pin Kit 3.1.0. The root checkout's installed `node_modules/@sveltejs/kit/package.json` unexpectedly reported 3.0.0. Initial implementation reads were therefore not sufficient to claim exact pinned-version behavior.

Ran `vp install` from `example/web` in the isolated worktree. The lockfile verification passed; the output reported Kit 3.1.0, Svelte 5.57.2, TypeScript 6.0.3 and Vite Plus 1.3.0. No direct npm/pnpm invocation was used, and no root-checkout dependency installation was changed.

Then re-read the installed Kit 3.1.0 files:

- `src/runtime/client/remote-functions/query-live/iterator.js`: fetch, SSE Accept header, abort signal, body reader and SSE parser.
- `src/runtime/client/remote-functions/query-live/instance.svelte.js`: shared live-query state and reconnect behavior.
- `src/runtime/server/remote-functions.js`: 30-second heartbeat, SSE frames and cancellation.
- `src/runtime/server/data/index.js`: streamed response with `text/sveltekit-data`.
- Page serialization/runtime files for deferred document data.

This correction is important for continuation: ignored local dependencies can be stale even when the repository is clean. The report's pinned-Kit conclusions use the fresh worktree installation.

## 5. Check the current Zig toolchain and primary sources

The installed compiler reported Zig 0.17.0 and an aarch64 macOS target. Go reported 1.27.3. `zig env` identified the installed standard-library source; those local files were read rather than assuming older Zig APIs still applied.

Relevant installed Zig source:

- `std/http.zig`: HTTP version enum includes 1.0 and 1.1; transfer encoding includes chunked.
- `std/http/Server.zig`: streaming responses and HTTP/1.x parsing/framing.
- `std/http/Client.zig`: TLS, certificates and connection pools for outbound HTTPS.
- `std/Io.zig`: futures, groups, queues, async/concurrent calls and cancellation.
- `std/Build.zig`, `std/Build/Step/Run.zig`: executable/run steps, output capture and installation APIs.
- `std/lang.zig`: actual Zig 0.17 reflection layout, including struct field-name/type arrays and function parameter types.
- `std/Io/File.zig`: current I/O signatures while evaluating possible generator approaches.

Primary online references used:

- [Zig 0.17 language reference](https://ziglang.org/documentation/0.17.0/), particularly imports and comptime.
- [Zig official download index](https://ziglang.org/download/index.json), identifying the stable release and documentation.
- [Zig 0.17 release notes](https://ziglang.org/download/0.17.0/release-notes.html), especially incremental compilation, aarch64 and Mach-O status.
- [Bun runtime documentation](https://bun.com/docs/runtime), establishing JavaScriptCore as the engine.
- [Bun WebKit dependency configuration](https://github.com/oven-sh/bun/blob/main/scripts/build/deps/webkit.ts), establishing pinned prebuilts/local builds and target/ABI settings.
- [Bun contributor build documentation](https://github.com/oven-sh/bun/blob/main/docs/project/contributing.mdx), for the cost and separation of rebuilding WebKit.
- [Apple JavaScriptCore C API](https://developer.apple.com/documentation/javascriptcore/c-javascriptcore-api) and [WebKit API headers](https://github.com/WebKit/WebKit/tree/main/Source/JavaScriptCore/API).
- [QuickJS documentation](https://bellard.org/quickjs/quickjs.html) and [QuickJS-NG C API guide](https://quickjs-ng.github.io/quickjs/developer-guide/intro/), for the comparison embedding/lifetime model.
- [http.zig](https://github.com/karlseguin/http.zig), for an HTTP/1.1 server supporting the installed Zig release and its documented SSE helper.
- [Caddy reverse-proxy documentation](https://caddyserver.com/docs/caddyfile/directives/reverse_proxy), for upstream version selection, buffering, flushing and disconnect behavior.
- [WHATWG SSE specification](https://html.spec.whatwg.org/multipage/server-sent-events.html) and [MDN SSE guide](https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events/Using_server-sent_events), for application framing and browser connection limits.
- [nghttp2](https://github.com/nghttp2/nghttp2), as an example of a possible later native HTTP/2 dependency, not a selected integration.
- [zig-js](https://github.com/zig-utils/zig-js), a pure-Zig-engine alternative whose own API is pre-stabilization; no adoption claim was made.

Some browser fetches of the release notes failed, while directly retrieving the official page through the shell succeeded. The downloaded full text was used for targeted inspection and removed; the source URL and findings are retained. A few guessed source filenames and an uppercase http.zig README URL did not exist; actual file listings/source names were used instead. One malformed orchestration call executed nothing and was corrected. None of these failed lookups was interpreted as a product capability limitation.

## 6. Experiment: exact route-shaped imports

Created two fixture files under these real directory paths:

```text
ephemeral/zig-analysis/import-probe/src/routes/[foo]/[bar=nice]/data.remote.zig
ephemeral/zig-analysis/import-probe/src/routes/(marketing)/[[optional]]/[...rest]/data.remote.zig
```

The main probe imports them directly and checks literal values 42 and 17. The bracketed-path fixture was then extended with native `Input`/`Output` structs and a concrete lookup function. A second test checks the compiler-reported parameter/return types and actual result fields against literal expectations.

Executed from repository root:

```sh
zig test ephemeral/zig-analysis/import-probe/main.zig
```

Observed final output:

```text
1/2 main.test.imports Kit route paths directly...OK
2/2 main.test.compiler exposes remote function signature...OK
All 2 tests passed.
```

This does not scan directories, register routes, interpret a matcher, or invoke Kit. It settles the import-path question and concrete signature introspection only. No symlink exists in the probe.

## 7. Experiment: foreign-language generation inside the build program

The initial discussion suggested a separate host generator could inspect native types and write TypeScript. The user pushed further: put the generation in `build.zig` so it is part of the build and absent from the program.

The implemented probe does exactly that:

1. The build program directly imports the route fixture and the projection functions.
2. `comptime` constructs declaration strings using the actual native struct fields/types.
3. `b.addWriteFiles()` records physical generated files in the build graph.
4. Install steps publish the outputs under `zig-out/generated`.
5. A separate application executable imports the remote fixture but not the projection library.

Executed from the probe directory:

```sh
cd ephemeral/zig-analysis/import-probe
zig build --summary all
```

Observed output:

```text
Build Summary: 7/7 steps succeeded
install success
+- generate success
|  +- install generated to generated/types.ts success
|  |  +- WriteFile types.ts success
|  +- install generated to generated/Types.swift success
|     +- WriteFile types.ts (reused)
+- install import-probe success
   +- compile exe import-probe debug native success
```

The original output also reported roughly one second for the tiny application compilation and peak memory for that step. Those incidental numbers are deliberately not treated as a benchmark: the fixture is trivial, no representative SKGO dependency graph is present, and no matched Go baseline or repeated timing series was run.

Both generated files were read and compared with literal expected contents using Python assertions. The application executable was run successfully.

Exact TypeScript output:

```typescript
export interface Input {
  id: number;
}

export interface Output {
  id: number;
  label: string;
}
```

Exact Swift output:

```swift
struct Input: Codable {
    let id: UInt32
}

struct Output: Codable {
    let id: UInt32
    let label: String
}
```

The projection helper deliberately supports a small type vocabulary. Its TypeScript integer policy rejects widths above 53 bits rather than silently claiming every Zig integer fits JavaScript. Optional values map to a nullable type in this demonstration. That is an example policy, not an approved production omission/null contract. Strings, booleans and a narrow Swift integer mapping are present. Complex unions, custom transports, files, devalue references and nested promises are absent.

No TypeScript compiler or Swift compiler was invoked on the generated files during this research. Validation consisted of exact-content checks, a successful Zig build, and running the independent application target. The final application source graph does not import the generator; no binary-size comparison or symbol-disassembly audit was performed.

The phrase “comptime cannot write files” remains literally true but was initially misleading without explaining the build program. Compile-time computation supplies the content; the build program's runtime and WriteFile step supply the I/O. The whole operation is build-time from the application's perspective.

For a full application, importing declarations into the build runner may require dependencies that are awkward to configure there. A separately built host generator remains an alternative. Either design can keep generation absent from the shipped server.

## 8. Experiment: JavaScriptCore embedding

Located the installed Xcode macOS SDK's JavaScriptCore framework headers and checked the actual C signatures for context creation, evaluation and numeric conversion. The probe declares the narrow external API explicitly, links the system framework and creates/releases the context/string through the corresponding C functions.

Executed from repository root:

```sh
zig test ephemeral/zig-analysis/jsc-probe.zig -framework JavaScriptCore -lc
```

Observed output:

```text
1/1 jsc-probe.test.Zig embeds system JavaScriptCore through its C API...OK
All 1 tests passed.
```

The JavaScript creates a class with a private numeric field, invokes its method, and returns 42. The Zig side checks that no JS exception was produced and that numeric conversion yields exactly 42.

This establishes basic linking/evaluation on the current Mac. It does not establish that Bun's particular WebKit fork or prebuilt archives can be dropped into this build. It does not cover WebKit C++ APIs, scheduling promises, JIT restrictions, request isolation, engine pooling, interrupts, bundle syntax compatibility, or production distribution.

No JavaScriptCore source build was attempted. No QuickJS, pure-Zig engine, or Linux engine artifact was compiled or benchmarked.

## 9. Derive the HTTP conclusion with its limits

The pinned Kit source establishes the application-level formats and behavior. The installed Zig source establishes HTTP/1.1 and streaming primitives. Primary HTTP/SSE and reverse-proxy documentation establishes how these layers fit together.

The recommended split retains public HTTP/2 or HTTP/3 at an HTTP reverse proxy, with plaintext HTTP/1.1 between the proxy and Zig. A stream uses one upstream HTTP/1.1 connection, while the browser can multiplex public HTTP/2 streams. A raw TLS offloader without HTTP translation cannot simply send decrypted HTTP/2 to an HTTP/1.1 application.

The HTTP/1.1 browser connection budget remains relevant for direct local access. Pinned Kit uses fetch and its own SSE parser, so native EventSource reconnection behavior should not be assumed; Kit's live-query implementation itself contains retry/reconnect logic.

Heartbeat intervals, proxy buffering, output backpressure and disconnect propagation must be checked together in the real application. In particular, a generic low-latency proxy setting may change upstream cancellation on early browser disconnect. The assessment deliberately does not prescribe a copied Caddy configuration without testing those semantics.

No HTTP server, TLS listener, public proxy, SSE connection-load test or browser session was run as part of this research. The HTTP conclusion is source/protocol-based, unlike the executable import/generation/JSC probes.

## 10. Architecture implications of accepting development restarts

The user's clarification eliminates the need to carry over a major cluster of Go-specific machinery merely to preserve continuity during development. A Zig implementation can compile a new native process and restart it, keeping Vite and the stable public development endpoint.

The source already separates the development rendering path: goja is absent under the development build tag. That means the proposed development binary can omit production JSC as well. The largest external renderer dependency need not participate in ordinary native edit/link/start cycles.

What remains valuable is input discovery, sensible invalidation, file generation, failure reporting, readiness, routing, cleanup, and a client reload/reconnect experience. None requires retaining Go's patched object readers or candidate-object loader.

This simplification says little about production concurrency: production requests, nested fetch/render, load futures and live streams still need owned lifetimes, cancellation and bounded resources. Accepting dev restarts does not remove those runtime contracts.

## 11. What has not been proved or decided

- No application migration duration or staffing estimate.
- No demonstrated native build-speed advantage for a representative SKGO application.
- No production SSR throughput comparison, JSC-versus-goja measurement or memory comparison.
- No actual Kit bundle executed inside the new JSC host.
- No full Zig devalue implementation, schema grammar, optional/null policy or remote API.
- No generated `.remote.ts` module consumed by Kit and no generated client invoking a Zig HTTP handler.
- No watcher handling route additions/removals or signature edits.
- No restart/handoff supervisor, stable proxy integration or live-stream reconnect demonstration.
- No Windows/Linux portability proof, static-binary distribution proof or engine artifact selection.
- No broad repository Go test run: product implementation was not changed. The focused research probes and generated-content checks are the relevant executed validation.
- No PR, merge, release, installed-SKGO update or deployment is part of this branch.

## 12. Reproduction checklist

From a checkout of this branch, these are independent checks. Use Zig 0.17.0 for the exact saved source APIs.

```sh
# From repository root: direct imports and function signature reflection.
zig test ephemeral/zig-analysis/import-probe/main.zig

# From the probe directory: generate both client-language files and an app.
cd ephemeral/zig-analysis/import-probe
zig build --summary all
cat zig-out/generated/types.ts
cat zig-out/generated/Types.swift
./zig-out/bin/import-probe
```

Separately, from repository root on macOS with the framework available:

```sh
zig test ephemeral/zig-analysis/jsc-probe.zig -framework JavaScriptCore -lc
```

To inspect current native import graphs without changing product source:

```sh
go list -deps -json .
go list -deps -json ./cmd/skgo
```

To inspect the exact pinned Kit implementation on a fresh checkout, run `vp install` from `example/web`, confirm the installed package version, then read the runtime files identified above. Existing ignored `node_modules` from another task is not sufficient evidence of the pin.

The committed source is the durable reproduction material. `node_modules`, Zig caches, executables and generated `zig-out` outputs are intentionally not committed.

## Portable source index

These links resolve from this document to the repository source:

- [Root Go dependencies](../../go.mod)
- [Workspace modules](../../go.work)
- [Example Go dependencies](../../example/go.mod)
- [Frontend dependencies](../../example/web/package.json)
- [Adapter package](../../internal/adapter/package.json)
- [Current Kit reference](../sveltekit-current/SKILL.md)
- [Go route-link workaround](../../internal/gen/links.go)
- [Remote discovery/signatures](../../internal/gen/scan.go)
- [Native-to-client type projection](../../internal/gen/types.go)
- [Generated bindings](../../internal/gen/emit.go)
- [Plugin wire model](../../templateapi/generator.go)
- [Production renderer](../../internal/ssr/engine.go)
- [Renderer host scheduling](../../internal/ssr/async.go)
- [Development renderer boundary](../../internal/ssr/engine_development.go)
- [Web globals](../../internal/ssr/webglobals.go)
- [Adapter engine environment](../../internal/adapter/skgo-adapter/env.js)
- [Native hot-reload toolchain](../../internal/hotreload/toolchain.go)
- [Private native loader](../../internal/hotreload/loader.go)
- [Development routing](../../internal/devserver/router.go)
- [SSE handler](../../remote_live.go)
- [Deferred data](../../deferred.go)
- [Data responses](../../data.go)
- [Request fetch](../../fetch.go)
- [Example process/server](../../example/cmd/main.go)
