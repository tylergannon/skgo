# SKGO in Zig: dependency and architecture assessment

Assessment: 2026-10-24. SKGO source: `618f467b`; Go 1.27.3; freshly installed pinned Kit 3.1.0 and Svelte 5.57.2; installed Zig 0.17.0 on macOS arm64. This is research and small capability probes, not an implemented port or a build-performance comparison.

**A Zig implementation is technically credible. The main work is preserving SKGO's contracts while replacing its Go runtime and language tooling. Bracketed route imports and HTTP/2 are not blockers.** The strongest simplification comes from combining compiler reflection with an ordinary restart-based development server. Production JavaScript embedding is the largest external dependency decision.

The intended meaning of “in Zig” here is that Zig owns the process, application handlers, and native framework. Svelte, Kit, browser JavaScript, and build-time TypeScript remain. JavaScriptCore itself is C++, so that option preserves a native application architecture but not a literally all-Zig implementation.

**What SKGO currently depends on**

The root `go.mod` has 13 direct module requirements and 16 indirect ones. Import-graph inspection with `go list -deps -json .` reaches 18 external modules and 220 standard-library packages; `./cmd/skgo` reaches 26 external modules and 251 standard-library packages. These are import dependencies, not measurements of linked bytes, startup time, or compile cost. In particular, manifest requirements include dependencies used by tests or examples rather than the production library itself.

| Dependency or subsystem | What SKGO gets from it | Zig disposition |
| --- | --- | --- |
| Go standard library: `net/http`, `httputil`, `net/url`, multipart, cookies, filesystem, embedding, process control, signals, crypto, compression | The HTTP server/client, internal and external fetch, proxy, static responses, request bodies, shutdown, and packaging | Use Zig standard-library facilities and/or a server library; recreate framework integration and establish the required behavior. This is much more than replacing a socket listener. |
| Go goroutines, channels, `context`, synchronization, GC | Concurrent requests and loads, deferred data, live producers, cancellation, bounded renderer jobs, lifetime safety | Use explicit task ownership, cancellation, allocation, and a chosen I/O implementation. Zig 0.17 has `std.Io` futures, groups, queues, and concurrency; the primitives exist, but Go's ownership model does not transfer automatically. |
| `goja` and its parser/regexp/sourcemap/profiling dependencies | Production Kit renderer and universal JavaScript execution | Replace with JavaScriptCore or another embedded engine. Most of this dependency subtree follows the engine rather than needing separate Zig ports. |
| `goja_nodejs` URL module | `URL` and `URLSearchParams`; SKGO installs it directly without enabling a general Node module loader | Replace the web API binding. An ECMAScript engine does not supply browser URL/fetch/stream APIs by itself. |
| `github.com/tylergannon/devalue/v5` v5.0.0 | Native Kit-compatible value encoding/decoding | Implement a Zig codec or adapt an independently qualified implementation. Ordinary JSON is insufficient. |
| Polytype v1.10.0 | Go type analysis, TypeScript declarations, generated devalue codecs, optional/nullable wire semantics, plugin type grammar | Replace the Go frontend and native-code backend. Compiler reflection can provide Zig type information; the language-neutral wire model remains useful, but existing implementation and plugins are Go packages. |
| `golang.org/x/tools`, Go AST/types packages, `dave/dst` transitively | Package loading, type checking, generation, advice, retained compiler views and native replacement analysis | Zig's compiler can own type resolution and reflection. Remove Go-specific package/source machinery. Filesystem discovery and emitting artifacts for Kit still need a build tool. |
| `fsnotify` v1.10.1 and SKGO retained-generation machinery | Watching, coalescing, invalidation, reuse of analyzed inputs | Replace or substantially shrink. Zig build watching may cover native inputs; Vite still needs coherent generated frontend files and route-add/remove discovery. |
| Private `goloader` and `purego` dependencies materialized by `internal/hotreload` | Native Go replacement objects and no-cgo dynamic loading; these are not obvious from the root manifest | Drop for a restart-based Zig dev server. Current code also prepares a version-bound view of Go's internal object reader and rewrites application functions. |
| OpenTelemetry v1.47.0 and its support modules | Trace propagation and framework spans; the sample installs the SDK | Keep a small tracing boundary; implement native propagation and choose an exporter/SDK strategy. Feature parity requires a replacement; a reduced first release could explicitly defer full instrumentation. |
| `x/mod` v0.42.0 | Module resolution, module-file editing, version comparison, plugin/update operations | Go module management goes away for a Zig-only application. Zig package/plugin distribution and updates still need a coherent interface. |
| YAML v3, `x/sys`, `x/term` | Lockfile/update tooling, Kit patching, OS integration, CLI interaction | Use suitable Zig libraries/platform APIs or retain narrow subprocess helpers where appropriate. These are peripheral to the runtime. |
| Optional CLI plugin: Cobra v1.10.2, mcp-go v1.3.0 | Generated CLI and MCP server/client functionality | Port separately or retain a separate Go tool. They are not baseline dependencies of every SKGO server. |
| Optional native-shell plugin and SDK | Swift client generation and Apple application packaging | Reuse conceptual remote metadata and the shell/UI; replace the Go plugin/build integration. Swift/Xcode work does not disappear by changing the server language. |

The frontend remains Kit 3.1.0, Svelte 5.57.2, Vite Plus 1.3.0, TypeScript 6.0.3, the Svelte Vite plugin, and the existing adapter responsibilities. Kit's devalue/cookie/MIME/etc. dependencies remain with Kit. The sample's bits-ui and YAML packages are application choices. Formatting, svelte-check, and the Lightpanda client-handoff verification are development tools, not a native server runtime.

For scale, a non-test, non-generated source-line inventory found about 18,558 lines in the root Go package, 12,056 under `internal/gen`, 2,694 under `internal/hotreload`, 1,601 under `internal/devserver`, and 1,436 under `internal/ssr`. These counts include comments and are not effort estimates. Much generation/hotreload code would be replaced rather than translated, while the root runtime encodes substantial Kit behavior that remains relevant.

**Route imports: directly demonstrated**

`zig test ephemeral/zig-analysis/import-probe/main.zig` passes two tests. The source imports:

```zig
const nested = @import("src/routes/[foo]/[bar=nice]/data.remote.zig");
const grouped = @import("src/routes/(marketing)/[[optional]]/[...rest]/data.remote.zig");
```

The second test obtains the actual argument and return types of `nested.lookup` with `@typeInfo(@TypeOf(nested.lookup))`, calls the function, and checks literal results. There are no symlinks.

Directory names in an import string are filesystem path components, not Zig identifiers. The remaining rule is the module root: ordinary file imports must be within the importing module's permitted source tree. Put the generated application root at a suitable common ancestor or use explicit build-system modules/imports. Do not place a generated root in a deeply nested cache directory and assume arbitrary `../../` imports may escape it.

Current SKGO's `internal/gen/links.go` instead creates Go-nameable directory symlinks and Go 1.27 module `ignore` entries. That entire workaround is avoidable in Zig.

**What comptime can replace, and what must still be generated**

| Responsibility | Good home in a Zig implementation |
| --- | --- |
| Check native handler signatures and supported wire types | Compiler reflection and compile errors |
| Specialize request decoding, result encoding, validators, and dispatch wrappers | Generic Zig code specialized by comptime |
| Construct typed handler tables from an explicit registry | Comptime |
| Traverse reachable native types into one wire schema model | Comptime-assisted library; emit through a host executable when external files are needed |
| Discover added, renamed, or removed `.remote.zig`, server-load and endpoint files | Build program/generator doing filesystem I/O |
| Produce physical `.remote.ts`, `+page.server.ts`, endpoint stubs, declarations, hooks/config bridges | `build.zig` WriteFile steps fed by comptime-generated contents, or a host generator executable for a more complex application graph |
| Compile Svelte/TypeScript, evaluate Vite configuration, discover Kit manifests/identities and prerender | Kit/Vite plus the adapter |
| Emit optional Swift/CLI/MCP clients or other plugin files | Build-time comptime projection plus file-output steps, or a generator executable consuming the same resolved model |
| Publish changed files coherently, remove obsolete owned files, refresh Vite | Build/dev orchestration |

The user correctly proposed putting foreign-language generation in the build program, so it never becomes part of the application binary. This is directly demonstrated in `import-probe/build.zig`: it imports the bracketed-path remote, uses `comptime` to construct both TypeScript interfaces and Swift `Codable` structs from the native `Input` and `Output`, and feeds those strings to `b.addWriteFiles()`. `zig build --summary all` passed 7/7 build steps, producing `zig-out/generated/types.ts`, `zig-out/generated/Types.swift`, and a separate application executable whose source does not import the generator. A separate emitter executable is not necessary for this simple case.

Comptime itself cannot perform arbitrary filesystem writes or execute Vite. The ordinary runtime of the build program and its generated WriteFile steps provide the file-output side. A practical larger split is a build-time directory scan that produces a small import registry, then either direct build-program reflection or a host generator configured with the application's module dependencies. Define the wire semantics once and share them with runtime codecs; do not independently infer a second schema in a source-text parser.

There is a staging concern: compiling a metadata emitter that imports full application modules can accidentally require application libraries, target-specific code, or frontend build artifacts. Keep the metadata/native-handler dependency graph independent of embedded production assets; arrange host-versus-target compilation explicitly. Avoid a cycle in which generating frontend types requires compiling a server that embeds the frontend output being generated.

Comptime removes generated native source and external type-checker machinery where appropriate. It does not remove the semantics of omitted versus null values, safe integer transport, strings versus byte arrays, error responses, custom transported types, promise placeholders, or devalue references. Those still require implementation.

**JavaScriptCore is a credible first candidate**

[Bun uses JavaScriptCore](https://bun.com/docs/runtime), Apple's WebKit JavaScript engine. Embedding that engine does not require adopting Bun's complete runtime, package manager, HTTP stack, or Node compatibility layer.

`zig test ephemeral/zig-analysis/jsc-probe.zig -framework JavaScriptCore -lc` passes on this Mac. It creates a JavaScriptCore context through the public C API and evaluates a private-field class to the literal number 42. This proves basic Zig embedding against the system framework; it does not prove Kit rendering, asynchronous host callbacks, production portability, or performance.

The architectural fit is promising: keep Zig-owned application I/O and give JavaScript only the renderer/universal-code host calls it needs. Current adapter logic, component bundles, data envelopes, and web API polyfills provide reusable concepts and some reusable JavaScript. Native goja values and callback bindings need replacement. Modern syntax support may let us delete goja-specific syntax lowering, subject to running the actual Kit/Svelte bundle.

The remaining renderer work includes URL/URLSearchParams, text codecs, Request/Response/Headers/FormData/streams as required, console and timers, promise/microtask and host-completion integration, request isolation, cancellation, nested rendering/fetch, and runtime pooling. JavaScriptCore does not turn into Bun simply by linking its library.

For portability and repeatability, choose between system JSC on macOS and a pinned standalone JSC build distributed for each supported production target. [Bun itself uses a pinned WebKit dependency and prebuilt artifacts or an explicit local build](https://github.com/oven-sh/bun/blob/main/scripts/build/deps/webkit.ts). We would own the corresponding headers, ABI/settings, target support, release artifacts and link cost. Normal application edits should never rebuild WebKit. A single distributable executable can include an engine, but truly static deployment and cross-compilation need per-target proof.

[QuickJS/QuickJS-NG](https://quickjs-ng.github.io/quickjs/developer-guide/intro/) is the useful comparison: a smaller embedding/build surface and explicit value lifetime management, with execution throughput and startup to measure on SKGO's renderer. It also needs host web APIs and an event loop. A pure-Zig engine such as [zig-js](https://github.com/zig-utils/zig-js) is an option to evaluate if language purity matters, but its own API is pre-stabilization and its published test claims are not SKGO qualification.

**HTTP/1.1 supports the required protocols**

Pinned Kit 3.1.0 source confirms:

- `query.live` uses fetch with `Accept: text/event-stream`, a response-body reader, and Kit's SSE parser; it is not the browser EventSource API.
- The server emits a 30-second keep-alive comment, result/error/redirect frames, and cancellation. Live-query client code has reconnection behavior.
- Deferred navigation data uses `text/sveltekit-data` and streamed chunks.
- Initial document responses can stream HTML and deferred promise-resolution data.

These are application-level streaming contracts, not requirements for HTTP/2. HTTP/1.1 can carry each using a streaming response body, generally with chunked transfer encoding when the final length is unknown. SSE message boundaries and HTTP chunk boundaries are independent.

Recommended deployment:

```text
browser -- HTTPS + HTTP/2 or HTTP/3 --> reverse proxy
        -- HTTP/1.1 --> Zig application
```

The proxy must terminate HTTP at the application layer and translate versions; a raw TCP TLS terminator alone does not translate HTTP/2. Each open response stream consumes an upstream HTTP/1.1 connection. Size the proxy upstream connection pool and application resources for that. Browser multiplexing is retained on the public connection.

[Caddy documents upstream HTTP version selection and streaming behavior](https://caddyserver.com/docs/caddyfile/directives/reverse_proxy). Flush behavior, intermediary buffering, idle timeouts/heartbeats, bounded output and disconnect propagation matter more than adding native HTTP/2 for this first architecture. Check cancellation before copying a configuration recipe: some explicit low-latency proxy modes alter early-disconnect behavior.

With direct browser-to-origin HTTP/1.1, multiple long-lived live queries compete with the browser's small per-origin connection budget, often six across tabs ([MDN](https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events/Using_server-sent_events)). This is an acceptable local-development limitation if understood; retaining public HTTP/2 via a reverse proxy avoids making it the production default.

Installed Zig 0.17's `std.http.Version` has HTTP/1.0 and HTTP/1.1, and its HTTP server supports streaming/chunked output. A candidate higher-level server is [http.zig](https://github.com/karlseguin/http.zig), whose current branch targets Zig 0.17. Its documented SSE helper starts a thread per stream, so that helper is not an automatic replacement for Go's many lightweight goroutines. Choose and exercise the concurrency model against expected live-stream counts before committing to that API. Native HTTP/2 could be added later with a library such as nghttp2 if a real requirement emerges.

Inbound TLS can be deferred. Outbound HTTPS remains necessary for ordinary `event.fetch("https://...")` and many application APIs; Zig's standard HTTP client has TLS/certificate support. Preserve the external HTTPS origin for cookie/CSRF/redirect semantics behind the proxy. The current example already serves a plain socket using `http.Server.Serve`; it does not explicitly enable cleartext HTTP/2.

**A simpler development server is consistent with the requirement**

The user explicitly accepts that this is single-user development and restarts happen. Do not make preservation of in-memory application state or open streams a prerequisite for the port.

Keep Vite's normal SSR/HMR, a stable development address and proxy, coherent frontend/native generation, readable build diagnostics, clean process cleanup, and a simple readiness handoff. Replace the native application after a successful build. In-flight development requests may fail and streams may reconnect or require a refresh. An old healthy server can remain available during compilation, without preserving process state once the replacement takes over.

Drop Go's replacement-object compiler/loader, internal runtime/object-reader coupling, host function rewriting and old-code retirement. Keep only the build scheduling/invalidation needed for actual inputs. A clean restart also means ordinary startup initialization runs again; database migrations or other application startup effects must be appropriate for development restarts.

Importantly, current `internal/ssr/engine_development.go` already excludes goja from the development host: Vite renders the page. Preserve this split so ordinary Zig edits need not link JavaScriptCore at all. Production engine choice and fast native development iteration can be evaluated independently.

**Build speed is a hypothesis to measure on the actual target**

[Zig 0.17's release notes](https://ziglang.org/download/0.17.0/release-notes.html) describe the improved incremental path for most x86_64-linux projects using `zig build -fincremental --watch`. They still identify the new Mach-O linker and self-hosted aarch64 backend as future work; the aarch64 section says linker enhancements block progress. Do not assume that the same near-instant path is available on this Mac today.

The latency that matters is edit to the new response appearing: native analysis/codegen, compilation, linking, startup, handoff and client refresh; signature edits also involve regenerated frontend files and Kit/Vite. A broad comptime registry and a large static engine can move costs rather than erase them. Keep JavaScriptCore absent from native dev builds and prebuilt/cached in production builds.

**The useful next experiment is a narrow complete application, not a wholesale rewrite**

One colocated remote in `[foo]/[bar=nice]`, typed input/output and physical generated `.remote.ts`; a plain Kit page that invokes it; one server load with a deferred value; one cancellable live query; Vite development with native rebuild/restart; and a production JavaScriptCore-rendered document adopted by Kit's browser client. Run direct HTTP/1.1 and through a proxy serving public HTTP/2. Compare body-only edits, signature edits and route additions on macOS arm64 and the Linux development/deployment target.

Acceptance is the running page, exact wire behavior, disconnect cleanup, and measured end-to-end edit latency with representative application dependencies. The current probes settle route imports, native function-signature reflection, direct build-program TypeScript/Swift generation, and basic macOS JSC embedding only. No estimate of migration duration, throughput win, SSR speedup, or compile-time speedup is justified by these probes.
