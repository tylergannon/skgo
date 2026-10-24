# Laptop continuation: SKGO in Zig

This handoff preserves the investigation, evidence, user corrections, and unfinished questions from the October 24, 2026 conversation. The user wants to continue discussing the findings by voice from a laptop; the earlier voice connection ran through a remote computer and had disruptive latency.

The work is exploratory. No port, release, deployment, or wholesale rewrite has been authorized or implemented. The user authorized documenting the investigation in depth and pushing this research branch. Everything here is under `ephemeral/`; no `docs/` files or product implementation were changed.

## Start here

- [Assessment](README.md): the dependency inventory and architectural conclusions.
- [Investigation trail](investigation.md): what was examined, how it was checked, exact results, limitations, source links, and reproducible commands.
- [Build program](import-probe/build.zig): the smallest working example of the user's build-time foreign-language generation idea.
- [Projection functions](import-probe/types.zig): deliberately limited TypeScript and Swift projection using compiler reflection.
- [Import/signature checks](import-probe/main.zig): direct imports through Kit-shaped directory names and actual function-signature reflection.
- [JSC embedding check](jsc-probe.zig): Zig calling the system JavaScriptCore C API on macOS.
- [Session worklog](../worklog/20261024-zig-dependency-analysis.md): corrections and durable findings.

The source baseline is SKGO `618f467b`. The initial research checkpoint is `1069dca9`. These identify the inspected implementation and the first saved probes; later documentation commits on this branch extend the handoff without changing the baseline.

## What prompted the research

The user asked for a deep dependency analysis of SKGO and what a Zig implementation would require. The motivation is a potentially much faster native build loop, especially after Zig's aarch64 backend matures, and the possibility of deleting complexity accumulated around Go development.

Specific initial questions were whether Zig can import colocated handlers beneath route paths such as `src/routes/[foo]/[bar=nice]`, whether a symlink workaround would be needed, and whether SSE and SvelteKit protocols require HTTP/2. The user is willing to defer inbound TLS and put the application behind another server's TLS termination.

The user then identified Bun's JavaScript engine as a plausible production renderer and proposed using `comptime` for code generation. They correctly challenged an overly narrow initial framing of that idea: compile-time reflection can generate TypeScript and other language source as part of `build.zig`, with the generator absent from the application executable.

## User constraints and corrections that matter

1. **Development is single-user. Restarts are expected sometimes.** Do not make preservation of application globals, process state, or open streams across edits a prerequisite. This materially changes the architecture assessment: rebuild/restart can replace the native hot-replacement machinery.
2. **Keep useful development behavior.** Vite SSR/HMR, a stable browser address, readable compile errors, process cleanup and coherent generated files remain valuable. Restarts being acceptable does not mean leaving a broken proxy, leaked processes, or stale frontend declarations.
3. **Comptime is allowed to generate foreign-language code in the build process.** The proven simple design uses `b.addWriteFiles()` with strings produced at compile time. It does not require a separate generator executable or shipping the generator in the server.
4. **Bun's engine is a candidate, not a settled dependency.** The engine is JavaScriptCore. Embedding it does not require using the Bun runtime as the application host.
5. **Inbound TLS can be deferred.** Outbound HTTPS for application API calls is a separate requirement. Public HTTP/2 can be provided by an HTTP reverse proxy even when Zig serves HTTP/1.1 upstream.
6. **Kit remains the spec and the frontend remains ordinary Kit.** The opportunity is to change the native host and implementation language, not invent a custom browser protocol that merely resembles Kit.

## Findings and their evidence levels

| Finding | Evidence level | Important limit |
| --- | --- | --- |
| Zig imports files beneath bracketed, matcher, group, optional and rest directory names without symlinks | Executed with installed Zig 0.17.0 | Proves filesystem imports, not Kit routing or route discovery |
| The compiler exposes a concrete imported remote function's input/output types | Executed assertions against the exact fixture types | Does not define a production remote declaration API or cover generic functions |
| TypeScript and Swift source can be constructed at comptime and emitted by `build.zig` | Executed build, exact output comparison, separate application executed | Only a small set of primitive field types; not full Polytype/devalue parity |
| Zig can embed macOS JavaScriptCore through the public C API | Executed evaluation of a private-field JavaScript class returning 42 | No Kit bundle, promises, asynchronous host calls, Linux artifact, or performance proof |
| Current development host excludes goja and lets Vite render | Current source inspection | Must retain that separation deliberately in any port |
| Current Go route colocation uses directory symlinks plus module ignore entries | Current source inspection | Historical notes about a generated nested route module are outdated |
| Kit live queries use SSE; deferred data and document streams use other formats | Fresh pinned Kit 3.1.0 source inspection | No new streaming server or proxy was implemented during this research |
| HTTP/1.1 can carry these application streams; public HTTP/2 can terminate at a reverse proxy | Protocol/library source and primary documentation | Upstream connection usage, buffering and cancellation still need integration testing |
| Fast incremental native builds may simplify development | Architectural hypothesis supported by deleting Go-specific constraints | No representative SKGO-versus-Zig timing comparison exists |

## Where the voice discussion stopped

The user wanted the findings explained one at a time, at a comfortable pace while doing dishes and making coffee. They had not read the long written report.

The first explained finding was direct route-file imports: Zig can name the actual handler file, while Go needs the symlink tree to obtain a legal package import path.

The discussion then focused on the next consequence: once Zig imports a concrete function, its compiler can expose the actual argument and return types. The user asked to expand that statement. The explanation used a customer lookup accepting a number and returning a record with name/email fields.

An important precision was made in that explanation: **SKGO already uses Go's compiler libraries for reliable type information.** It does not maintain a hand-written Go type checker. The potential Zig win is making native binding/codec specialization ordinary generic code inside the compiler, and letting the build program use the same types for client artifacts, reducing the external package-analysis and generated-native-source pipeline.

The user then paused the technical discussion to request this complete written handoff and branch push, so they could pick it up on the laptop. No subsequent architectural decision should be assumed.

## A useful order for continuing the conversation

This is a reading/discussion map, not an approved implementation backlog:

1. Finish compiler reflection and build-time generation: which native source disappears, which external files still need emission, and how one wire model can serve runtime codecs and client declarations.
2. Explain the remaining serialization work: native types do not automatically become Kit-compatible devalue, and null/omission/numeric/custom-type policies still matter.
3. Explain JavaScriptCore: engine versus host APIs, retained adapter code, modern syntax, and the distribution/build consequences of a prebuilt C++ engine.
4. Explain what the simpler dev server keeps and drops, especially that production JSC can stay entirely outside the native development link.
5. Explain HTTP/1.1 streaming and the proxy split, including browser connection limits and upstream connection usage.
6. Explain the real concurrency/lifetime work: request and stream cancellation, data that outlives request parsing, nested SSR/fetch, and bounded work.
7. Discuss a small complete feasibility application and the measurements needed before choosing a rewrite.

## Open questions

- Which production targets are required initially: macOS arm64 for local development, Linux amd64 for deployment, Linux arm64, Windows, or something narrower?
- Should production use a pinned standalone JSC everywhere, or a system framework on macOS and a separately packaged engine on Linux? That affects reproducibility and artifact management.
- Which JavaScriptCore API boundary is sufficient for promise scheduling, async host callbacks, interrupt/cancellation and request isolation? The current probe only exercises synchronous evaluation through public C functions.
- How much existing adapter/polyfill JavaScript can run unchanged with a JSC host? Goja-specific bindings and syntax transforms are distinct from Kit-specific adapter responsibilities.
- Can the build program inspect the real application's native declarations directly, or is a separately configured host metadata executable easier once dependencies are involved? Both keep generation out of the final application.
- How should native declarations identify query, command, form, live, batch, loads, actions and endpoint kinds? File naming alone does not express those semantics.
- What is the minimal native wire grammar, and how are nullability, absent fields, numeric bounds, bytes/files, errors, transported types and nested deferred values represented?
- What server/I/O combination meets the expected concurrent request and long-lived stream count? The documented thread-per-SSE helper in http.zig is a reason to examine the API, not a reason to assume the whole library is unsuitable.
- How are native file additions/removals discovered and frontend outputs published without leaving Vite with half-updated declarations? The simple probe has an explicit import and does not solve watching.
- What is the actual end-to-end edit latency on the user's laptop, including signatures and added routes, rather than a trivial compiler microbenchmark?
- Which optional product features must accompany a first native port: full tracing, CLI/MCP generation, Swift clients/native shell, update tooling and plugin installation? No scope decision has been made.

## Suggested feasibility boundary

A technically useful next experiment would preserve ordinary Kit and exercise one real colocated remote, one deferred load, one cancellable live query, Vite development with native rebuild/restart, and a production JSC-rendered document adopted by Kit's browser client. It should use the actual native schema/codecs and test direct HTTP/1.1 plus a proxy serving public HTTP/2.

This experiment has not been implemented or authorized by this handoff request. Its purpose would be to evaluate both integrated behavior and full edit-to-response latency before committing to migration scope.

## Scope of the saved artifacts

The checked-in code is self-contained research material. There is no production Zig server, no `.remote.zig` integration with Kit, no native Zig devalue codec, and no replacement of the Go SDK. The generated TypeScript and Swift outputs are build artifacts and are recreated by the commands in the investigation trail. The repository's generated caches and installed dependencies are not needed in Git.

The branch can be read on any machine. The import/generation probe needs a matching Zig toolchain. The JSC probe specifically needs macOS and its JavaScriptCore framework. Recheck current upstream APIs if using another Zig release.
