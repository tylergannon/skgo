# Make SKGO development updates feel immediate

## TL;DR

Build a dev server whose save-to-visible-update experience is fast enough that a developer would want to show it to colleagues. Saving files starts useful work immediately, before an HTTP request arrives; unchanged work stays warm; rapid edits converge on the latest result; compatible updates preserve the document and unsent input. Multi-second rebuild/reload waits are the problem to eliminate. Keep the ambition high while demonstrating gains in the real application.

Where SKGO already performs work that Vite or gopls does better, adopt their established functionality, patterns and algorithms wherever feasible. Use upstream APIs directly or carry the applicable algorithms into the existing Go owner. Every adoption must identify the existing work it improves or replaces; upstream features do not create new SKGO responsibilities merely by existing. Existing implementation-specific tests can change with the strategy; correct application behavior remains required.

Always select the next most impactful useful change toward immediate development updates, accounting for the prerequisites it actually needs. After each usable delivery, check the real edit experience, remaining delay and maintenance cost before selecting the next assignment. The mappings below support those choices; completing every mapping is not the goal.

**First assignment: capability 1. A running developer session reacts to filesystem events without project scans in HTTP requests or polling.** Saving a relevant input starts preparation while the browser is idle. Port the applicable gopls watcher algorithms using SKGO's discovered input scope, and finish that useful change against the current generation/build path. A new project, revision or filesystem framework is not a prerequisite.

This is the sole implementation plan. The external [adoption index](/Users/tyler/src/skgo-token-cache/index/adoption.md), [development mappings](/Users/tyler/src/skgo-token-cache/collection/adoption-development.md), [generation/analysis mappings](/Users/tyler/src/skgo-token-cache/collection/adoption-generation.md) and [runtime/tooling mappings](/Users/tyler/src/skgo-token-cache/collection/adoption-runtime-tooling.md) are drill-down evidence. Their D/G/R IDs below identify source mappings, not competing plans. This plan governs scope and sequence; broad adoption language in the research does not override the goal or create additional assignments.

## Governing boundaries

Start from clean SKGO `53179fcdf0576f67e9dc13ad1df402cb9d18c863`, materialized in the cache's `repos/skgo-adoption-baseline-53179fc`. Historical `repos/skgo-current` and the interrupted implementation are not the baseline. [Source identities](/Users/tyler/src/skgo-token-cache/collection/adoption-scope.md) distinguish readable Vite 8.3.4 from installed Core 1.0.0 containing Vite 8.3.1. Porting an algorithm from the newer source does not imply changing the frontend pin. gopls source is x/tools v0.52.0; installed Kit is 3.0.0.

Use public dependencies and existing Vite APIs before copying code. Internal gopls/x-tools packages need focused Go ports with upstream notices and source function/commit beside the code. Preserve applicable upstream algorithms, but adapt product policy such as file filters, timing and retention to SKGO's actual workload. Import only the mechanisms needed to improve existing work; a port must not grow into a copy of the surrounding language-server or bundler subsystem. Keep public Go loading/analysis APIs and polytype backends. Changes to polytype's retained loader belong there and reach SKGO through a normal dependency update; do not build a private second loader.

Kit remains the specification for its HTTP/application semantics. Read the installed pin and [current Kit guidance](../../sveltekit-current/SKILL.md) before changing mirrored behavior. VitePlus owns frontend commands. SKGO-owned implementation stays Go; necessary adapter/client integration stays in its existing JavaScript boundary. SSR stays in pooled goja, application I/O stays in Go, and production stays one binary without request-time Node.

The user's scope is authoritative for implementation and review: assess whether a change advances the dev experience, replaces existing complexity and preserves correct behavior. Adoption without a concrete benefit, unnecessary prerequisites and code growth without useful capability are scope problems even when the code is correct. Do not expand the task into new requirements, theoretical completeness, mathematical certainty, generic hardening, formal models, correctness/proof machinery, gate runners, evidence manifests, architectural frameworks or an expanded testing campaign. Existing meaningful Go tests and running the real app are sufficient. Bring an actual unfilled gap back before inventing beyond necessary glue.

## Working capabilities and their dependencies

Each item describes an improvement to work SKGO already performs. Select and scope the next assignment by its expected effect on the current save-to-visible-update delay, then check that effect in the real app. Its owner maps upstream independently and chooses the implementation. Ownership below names a responsibility, not a prescribed file list. Finish usable behavior and remove the replaced strategy before expanding. Preserve the capability numbers as source-mapping references; they are not an execution sequence or an obligation to implement every listed mechanism.

Capability 1 is the first assignment. After that, choose the work that removes the largest remaining delay: retained source/generation work in 3–6, renderer work in 8–9, browser refresh in 10, or the native compilation/replacement leads below. Capabilities 2 and 7 support that work where command ownership or checks actually need improvement. Backend refresh can use current successful-build readiness; adopt the relevant source/publication ownership from 3 where needed rather than requiring the entire analysis chain first. The source-map part of 8 and capabilities 11–13 are independent improvements to existing behavior; select them when their benefit warrants the work, without delaying a larger dev-loop gain. Dependencies describe required behavior and existing seams, not mandatory wholesale completion of earlier capabilities.

### 1. Actual watches drive development

**Outcome.** Saving, creating, deleting or renaming a relevant input starts development work without waiting for HTTP traffic. The application is ready or rebuilding when the next request arrives. HTTP requests consume current session state; they do not discover the filesystem. Startup still loads the initial project once.

**Adopt D1, D2, D15.** Port gopls's [fsnotify watcher](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/filewatcher/fsnotify_watcher.go:26): fast event draining, separate recursive registration, ordered batches, populated-directory expansion, rename/delete handling, retry and joined shutdown. Use its [workspace-derived scope](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/cache/session.go:1131) with SKGO's discovered module, route, control and embed inputs. Vite retains its frontend watcher and [watch-change/invalidation/HMR ordering](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/node/server/index.ts:965); use its existing registration APIs for adapter dependencies.

**Replace.** Polling, repeated whole-project digests, request-time `noticeEdit`, and the request-before-poll test. An observed preserved-mtime edit reads changed bytes; an HTTP request is no longer an independent observation.

**Dependencies/owner.** No later capability is needed. Own Go input observation, selected watch scope and supervisor handoff. Retain current generation/build/replacement for this slice. Avoid competing Go invalidation paths through both Go and Vite watchers. Adopt gopls's event handling, not its fixed 500ms editor-diagnostics delay. Batch related save events only as needed for usable partial-save behavior; do not impose a blanket quiet interval before every rebuild.

**Validation.** Ordinary Go tests use real watcher events and served-handler old/new literals, including an outside-root dependency, populated directory, deletion and shutdown. Demonstrate that an edit starts the existing build while no HTTP requests are arriving, then serve the changed literal. Run the example and compare save-to-build-start and save-to-visible-update time with the baseline. Substitute SKGO input policy for gopls's Go-only filters and exclude generated output loops. Upstream does not completely reconcile removed roots or guarantee missed events; do not claim either or restore HTTP scans. Retired inputs must stop invalidating the current selected project even if an OS watch remains until close.

### 2. Go commands share execution and cancellation ownership

**Outcome.** Overlapping Go operations have consistent working-directory, cancellation, pipe and module-update behavior across dev, generation, checks and plugin tooling.

**Adopt R2.** Port x/tools's [Runner/Invocation](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/internal/gocommand/invoke.go:31): bounded concurrency, the specific concurrent go.mod-update retry, serialized streaming, logical `PWD`, and [interrupt/kill/pipe cleanup](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/internal/gocommand/invoke.go:298). Retain SKGO's selected Go executable and existing process-group protections where adaptation is needed.

**Replace.** Duplicated Go launch/cleanup strategies. cmd/go still owns resolution, compilation and caches. VitePlus, Zig, Xcode and Git retain native invocation contracts; this is not an all-command engine.

**Dependencies/owner.** Independent of 1. Own Go child execution; callers own flags, environment policy, diagnostics and operation lifetime. Later shared analysis/checking uses this subprocess cancellation seam.

**Validation.** Go tests cover logical paths, overlap, the precise retry condition and cancellation during execution/output copying. Exercise a real build/check with the selected toolchain. Exact redundant subprocess sequences can change; tool failures and host/plugin compatibility remain meaningful.

### 3. Generation, compilation and checks consume one owned source view

**Outcome.** Overlapping edits cannot publish obsolete generated code, mismatched executable bodies or stale diagnostics. A current follow-up completes after an interrupted build. Warm development retains unchanged inputs; one-shot commands may acquire a fresh view once.

**Adopt D3, D4, G1, G9, G14.** Port gopls [content handles](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/file/file.go:16), [cancel/clone/release](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/cache/view.go:785) and [historical overlays](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/cache/snapshot.go:493). Use Vite's [pending-result validity](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/node/server/transformRequest.ts:78), [coalesced follow-up](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/node/server/index.ts:856) and [prepare/cancel/commit](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/node/optimizer/index.ts:523). Connect these to the existing Worker, private materialization, module remapping and generated-output staging.

**Replace.** Independent digest/generator/compiler reads, complete warm recaptures, recapture-then-live-source fallback and checker overlays reopening live authored paths. Tagged/generated phases can have distinct build selections. Preserve the supported ordinary path for inputs outside materialization; extend the existing seam rather than silently narrowing support.

**Dependencies/owner.** 1 provides observed changes. Use existing command ownership and the applicable parts of 2 needed for cancellation; completing command-wrapper adoption throughout the application is not a prerequisite. Own retained inputs through generation/build/check handoff and current-result admission. Keep backing files alive until consumers finish, including after cancellation. This does not replace package analysis, the supervisor or compiler.

**Validation.** Real-handler Go fixtures supply changed bodies/contracts, pause obsolete work while another edit arrives, and cover route startup/membership, local module/workspace controls and embeds. Old work cannot install files, a serving child, diagnostics or readiness after losing ownership. Preserve authored/foreign-file protection and error/repair. gopls lazily reads unknown files; it is not an atomic filesystem snapshot. Vite pending abort only evicts a cache entry, and its cache guard does not make mixed project paths atomic. Apply ownership at actual publication boundaries without a new transaction system.

### 4. Discovery and parsing retain unaffected work

**Outcome.** Structural edits reload affected Go scopes; file edits reuse unchanged parses. New/moved files update membership without discarding the known project.

**Adopt G2, G3, G5.** Port gopls [metadata change classification](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/cache/snapshot.go:1964), scoped loading/replacement, [ForFile/ImportedBy indexes and reverse closure](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/cache/metadata/graph.go:20), and [URI/hash/mode parse cache](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/cache/parse_cache.go:128), including token-position ownership. Keep public `go/packages`, Go's selection rules and goload View.

**Replace.** Clearing all metadata groups for one structural change, fixed-point graph sweeps and repeated header/projection/typechecking parses. Shared ASTs remain immutable; projections must not erase their bodies in place.

**Dependencies/owner.** 3 supplies content and logical build identity. Own polytype/goload metadata/parse reuse and SKGO integration. Workspace/configuration changes may recreate a view; test variants and tagged/generated phases remain distinct.

**Validation.** Go fixtures change imports, membership and build selection, then exercise syntax error/repair. Check selected packages and fixed authored diagnostic locations; observe unchanged-file reuse and affected-scope discovery. Former layer-specific counters are replaceable. Include supported compiler directives/cgo/embed inputs rather than assuming gopls's narrower comparison covers SKGO compilation.

### 5. Type analysis shares work and useful lifetimes

**Outcome.** Equivalent consumers share analysis. A local error or cancelled consumer does not discard unrelated successful work or invalidate graphs still held by callers.

**Adopt G4, G6, G7.** Port gopls [staged package validity/key revalidation](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/cache/check.go:817), preserving imports when dependency keys agree. Use [retryable keyed futures](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/cache/future.go:12), shared [typeCheckBatch](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/cache/check.go:235), dependency waits before CPU-token acquisition, and snapshot/batch/parse-cache lifetimes. Use the distinct [memoize.Promise](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/util/memoize/memoize.go:43) for lazy snapshot work where its semantics fit.

**Replace.** Whole-universe invalidation, global analysis exclusion, whole-map projection eviction on error and the arbitrary three-universe policy. Pure shared analysis does not authorize parallel publication.

**Dependencies/owner.** Use the source/metadata identities from 3–4 and cancellation behavior from 2 where the selected analysis work needs them. Improve existing goload reuse incrementally; sharing every computation mechanism is not a prerequisite for retaining an unaffected package. Own retained goload concurrency/lifetime and Worker integration, not a scheduler. Honor caller-owned Session lifetime and old returned graphs.

**Validation.** Go tests cover shared consumers, cancelled computation with a remaining waiter, independent packages, error/repair, unchanged imports and old graphs consumed after edits/Close. Assert actual types/positions. Compiler artifact validity remains separate from in-memory types; generator inputs in 6 cannot be narrowed to typerefs.

### 6. Generation reruns affected producers and retires obsolete output

**Outcome.** Required type/codec/stub/registration output changes while unrelated producers stay warm. Deleting a declaration removes its producer's contributions and orphaned generated outputs while preserving other producers' and authored files. Reusable producer results survive restart with bounded storage.

**Adopt G8, G10, G13.** Apply gopls [package dependency keys](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/cache/check.go:1275) and [predecessor summaries](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/cache/analysis.go:476) to existing producers. Port Vite [edge replacement/orphan identification](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/node/server/moduleGraph.ts:244) into existing output ownership. Extend current cache with gopls [input-key/content values, decoded LRU and bounded disk retention](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/filecache/filecache.go:72), preserving checksums and rename publication.

**Replace.** App-wide projection misses, routine frontend prune sweeps and latest-whole-plan retention. Startup repair may discover existing artifacts. Aggregated Go files lose a producer's contribution, not a whole still-owned file.

**Dependencies/owner.** Use the retained source and analysis needed from 3–5 for the selected producers. Deliver selective reuse incrementally rather than requiring a complete replacement of the analysis infrastructure first. Own producer dependencies/results and existing emitters/backends. No task language or cache platform.

**Validation.** Literal constants/enums, tags, consumed comments, markers, route/configuration and embed fixtures produce correct bytes/handler values. A body-only edit executes new code while declarations stay warm. Cover deletion, shared outputs, authored/foreign files, corruption fallback and cold/warm reuse. Raw typerefs omits generator inputs: use metadata reverse closure plus actual local inputs and summary equality. Keep caching optional rather than copying gopls's fatal initialization policy.

### 7. Checks share actions and typed diagnostic locations

**Outcome.** SKGO-owned generation/advice checks reuse retained analysis and report authored locations in the bytes they checked. Compiler/external checker failures remain visible.

**Adopt G11, G12 and G14's check consumer.** Use public `go/analysis` with gopls [keyed actions/facts/summaries](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/cache/analysis.go:451), fed by the owned source view. Port [content-bound mapping](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/protocol/mapper.go:7) and [token-span diagnostics](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/cache/analysis.go:1245). Retain public prepared packages, structured vet output and actual source-path mapping.

**Replace.** Redundant SKGO analysis and internal string-only errors followed by regex reconstruction. Keep text adapters for genuinely unstructured external tools; convert byte/line and UTF-16 coordinates explicitly.

**Dependencies/owner.** 2–6. Own SKGO analyzers/diagnostic/check integration. Actual `go build` still proves compilation/linking. External vet/Staticcheck/frontend contracts remain unless equivalent analyzer behavior has been mapped.

**Validation.** Ordinary Go fixtures assert fixed paths/spans, multibyte source, tagged/generated phases, read-only checking and cancellation. Real checks still detect compiler/external failures. Redundant subprocess sequence assertions can change; checks cannot silently disappear. Do not promise exact dependency positions where upstream only has approximate export information.

### 8. The renderer consumes Vite's actual module lifecycle

**Outcome.** Module edits/deletions/errors and Vite restarts update pooled goja runtimes using upstream identities and payloads. Errors can name original components.

**Adopt D5–D9, investigate D14, use R6.** Use the running Vite [environment/hot channel](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/node/server/environment.ts:109), normal `hotUpdate`, server graph and acceptance propagation. Before enlarging the Go runner, check upstream [evaluator/transport options](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/module-runner/types.ts:50) and direct `transport.invoke`. Direct portable ModuleRunner is a source-backed candidate, not demonstrated goja compatibility or permission to expand the JavaScript boundary.

If direct execution cannot satisfy that boundary, independently feasible Go ports are the path: [canonical ID/URL/file indexes and evaluation state](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/module-runner/evaluatedModules.ts:11), pending evaluation versus genuine cycles, complete [cached/invalidate protocol](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/node/ssr/fetchModule.ts:81) and [typed serial HMR handling](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/module-runner/hmrHandler.ts:7) with connection/environment lifecycle. Keep richer server graph maintenance in Vite; evaluated-runner edge retention differs.

**Replace.** Raw watcher bypass, append-only path journal, request-time cursor polling/count restart guesses, URL-only exports caching, pending-as-complete evaluation and bespoke importer invalidation. Cross-process transport is glue, not a second graph or replay-log project.

**Dependencies/owner.** Independent of Go analysis. Own adapter environment, module protocol/evaluation and safe per-runtime update application. Coordinate with 9; never mutate a checked-out goja runtime concurrently.

**Validation.** Go runner/handler tests cover aliases/query modules, pending imports/cycles, changed imports/deletion, cache/invalidate replies, ordered payloads and restart. Edit the real example and inspect SSR/browser behavior. Do not claim perfect upstream evaluated-graph pruning; its source documents limitations.

**Independent small delivery: R6 mapped errors.** Stop disabling Vite inline maps, pass actual evaluator prefix through [startOffset](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/node/ssr/fetchModule.ts:101), and use [goja's inline-map/stack support](/Users/tyler/src/skgo-token-cache/repos/goja-e9f698900731/parser/statement.go:916). Trigger a component exception with a fixed original location. No decoder/Node stack hook is needed. Keep external map loading under Go policy. This does not wait for direct-runner feasibility or journal replacement.

### 9. Requests acquire a published manifest view

**Outcome.** Requests/nested subrequests own a manifest/configuration view. Successor publication no longer requires holding mutable-current ownership throughout a render, and old environments cannot publish after replacement.

**Adopt R5, D12.** Port gopls [short-lock acquisition](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/cache/view.go:585), [lease lifetime](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/cache/snapshot.go:209) and successor publication. Use Vite's actual [restart](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/node/server/index.ts:1408), [environment identity guard](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/node/server/hmr.ts:419) and [close ownership](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/node/server/environment.ts:369) where applicable.

**Replace.** Request-driven manifest polling, mutable request-held ownership and journal lifecycle guesses. Keep SKGO process-group cleanup, active HTTP draining, exclusive-resource transfer and the required pool/Go I/O bridge; upstream environment teardown does not supply them.

**Dependencies/owner.** Use module/environment updates from 8 and matching Go readiness through the existing seam or relevant ownership from 3. Own render-view publication/runtime coordination. Nested requests share the ancestor's acquired view.

**Validation.** Go handler tests hold a render/subrequest across route/template publication and Vite restart, asserting supplied values remain paired. Cover shutdown/active streamed responses. A manifest lease does not pin historical Vite transforms; an HMR queue does not freeze future fetches. Remove serialization only after an upstream-compatible coordination path demonstrates pairing. Lease adoption can ship with necessary render exclusion; concurrent old/new rendering remains a narrow unresolved lead.

### 10. Backend readiness refreshes Kit values; Vite owns stabilization/warmup

**Outcome.** Compatible Go edits update loads/remotes without replacing the document; frontend HMR and partial-save recovery work; renderer startup can reuse warmed transforms.

**Adopt D10, D11, D13.** Deliver readiness through Vite [custom events](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/client/client.ts:273), then call Kit [refreshAll](/Users/tyler/src/skgo-token-cache/repos/sveltekit-pinned/src/runtime/client/client.js:2711), whose own invalidation/navigation code refreshes queries and preserves page state. Use supported [watch options/output exclusions](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/node/watch.ts:61), per-file read stabilization and [environment warmup](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/node/server/warmup.ts:10). Go no-op events use content identity at the owned consumer.

**Replace.** Reload after every healthy build, `watcher.emit` monkey-patching, fixed global-delay assertions and custom preloading. Full reload remains valid when Vite acceptance or generated contracts require it. Warmup caches transforms, not every runtime's exports.

**Dependencies/owner.** Refresh needs successful readiness for matching backend/frontend contracts. Use the existing readiness seam and the applicable current-result admission from 3; it does not require completing 4–9. Watcher cleanup/warmup coordinates with 8 where those paths change. Own readiness/client integration and supported Vite options; Kit owns query/navigation behavior.

**Validation.** Kit-client scenarios show fixed new load/query values while a draft, explicitly initialized counter and document identity survive. Exercise navigation racing refresh, error/repair, structural replacement and usable no-script responses. Keep partial-save/edit/revert outcomes and inspect the example. No mapped drop-in replaces interception of Kit's synchronous partial-template exception: reproduce any remaining failure and bring that concrete gap back before inventing interception. Use actual warmup/cache behavior without claiming runtime evaluation disappears.

### 11. MCP uses the public SDK gopls uses

**Outcome.** Existing tools use standard initialization, schemas, framing and request cancellation during long checks.

**Adopt R1.** Directly use the public SDK as [gopls does](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/mcp/mcp.go:91), with [typed registration](/Users/tyler/src/skgo-token-cache/repos/mcp-go-sdk-v1.8.0/mcp/server.go:365) and stdio/IO transport. Keep tool bodies/results; connect request contexts and an explicit service-lifetime cancellation relationship to checks.

**Replace.** Handwritten JSON-RPC/MCP structs, serial dispatch, initialization/framing and schemas. Add no tools, gopls service or new transport.

**Dependencies/owner.** Independent; 2/7 improve check execution but do not block SDK substitution. Own CLI protocol/handler context integration.

**Validation.** Go tests with the SDK client exercise initialization, tool validation/output, a long check cancelled by request ID and shutdown. [SDK session close is graceful](/Users/tyler/src/skgo-token-cache/repos/mcp-go-sdk-v1.8.0/mcp/server.go:2131) and waits for calls: do not claim root cancellation forcibly stops arbitrary checks. Show actual context propagation.

### 12. Native SDK preparation retains valid builder state

**Outcome.** Repeat builds retain valid SDK preparation and native builder caches; failed preparation does not first delete the usable tree.

**Adopt R3.** Port Vite [identity-valid reuse](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/node/optimizer/index.ts:384) and [unique prepare/cancel/commit](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/node/optimizer/index.ts:523). Include actual selected source, mutable module replacements and relevant target/configuration in preparation identity.

**Replace.** Unconditional `.sdk` deletion. Zig/Go/Xcode/VitePlus keep their compilation/cache ownership; no SKGO compiler cache.

**Dependencies/owner.** Independent. Own SDK source/preparation/active-tree lifetime, not native code replacement. Inspect actual cache paths and source-selection inputs when implementing.

**Validation.** Go tests cover unchanged/changed selection, failure and concurrent ownership. Run two real native builds where supported and inspect the actual surviving cache and correct output. Missing toolchains cannot count as passing. Vite rename does not establish a cross-process transaction; respect active build ownership.

### 13. Add-on preparation publishes immutable link targets

**Outcome.** Failed/new/concurrent preparation cannot erase a package still linked by an earlier `sv` invocation.

**Adopt R4.** Port Vite [run-owned preparation/commit](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/node/optimizer/index.ts:523) and gopls [immutable content before reference publication](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/filecache/filecache.go:228). Keep upstream `sv` and VP-qualified installation.

**Replace.** Destructive reuse of the fixed per-user stage. Finish pack/extract/install before publishing a target; do not mutate a previously handed-out target.

**Dependencies/owner.** Independent. Own preparation/downstream target lifetime, not templates or another package manager. Sharing the upstream strategy with 12 does not require an artifact framework.

**Validation.** Go tests hold a prior literal package through its retained link during failure/concurrent preparation. Run real VP project creation and inspect its link and usable app. Unlike Vite's old dependency directory, `sv` targets outlive preparation. Keep published targets until link ownership is established; no invented expiry cleanup. Reclamation is a follow-up gap, not permission to retain destructive staging.

## Source mapping coverage

This table locates research for selected work. Coverage is not a completion checklist or a priority ranking.

| Mappings | Capability | Relevant dependencies / integration seam |
|---|---|---|
| D1, D2, D15 | 1 | First assignment, current build path |
| R2 | 2 | Independent command adoption |
| D3, D4, G1, G9, G14 | 3 | 1; applicable command ownership from 2 |
| G2, G3, G5 | 4 | 3, metadata/parsing |
| G4, G6, G7 | 5 | Relevant source/metadata/cancellation from 2–4 |
| G8, G10, G13 | 6 | Selected producers' source/analysis from 3–5 |
| G11, G12; G14 consumer | 7 | 2–6, checks/diagnostics |
| D5–D9; D14 candidate | 8 | Separate Vite/runtime chain |
| R6 | 8, independent small delivery | Current evaluator/Vite/goja |
| R5, D12 | 9 | 3/8, manifest/runtime lifecycle |
| D10 | 10 | Current readiness; relevant ownership from 3 |
| D11, D13 | 10 | Supported Vite APIs, coordinate 1/8 |
| R1 | 11 | Independent SDK adoption |
| R3 | 12 | Independent SDK preparation |
| R4 | 13 | Independent add-on preparation |

## Genuine unresolved leads

- **D14 direct runner:** check exact installed syntax/lowering, goja promises/globals, external-module refusal, pool ownership and the Go implementation rule. If direct use fits, adopt it. Otherwise report the mismatch and deliver independently applicable D7–D9 ports; no silent JavaScript rule relaxation or runtime fork.
- **R5 concurrent renders:** manifest leases and HMR queues do not establish historical module retention. Bring back the smallest actual pairing failure before adding beyond necessary coordination. This does not block watching, module protocol, source maps or leases.
- **D11 partial templates:** supported APIs replace broad watcher intervention, but no upstream substitute for synchronous Kit exception interception was established. Preserve the recovery case and identify any remaining real failure.
- **R4 reclamation:** establish downstream link lifetime before deleting immutable published targets. Do not invent age-based cleanup.
- **No-link/no-restart Go edits and the compilation floor:** [native-engine](/Users/tyler/src/skgo-token-cache/index/native-engines.md) and [compiler-retention](/Users/tyler/src/skgo-token-cache/index/compiler-retention.md) research support the larger speed goal. When linking, startup or repeated compiler discovery is the dominant remaining delay, select the smallest useful application integration or experiment from these leads; do not postpone it until all thirteen capabilities are complete. Reuse the existing native experiments, distinguish fixture results from real application integration, and preserve the ordinary rebuild path for incompatible edits. Vite/gopls do not establish native unload, active-frame or returned-value safety. This plan does not prescribe a new loader/lifetime system or claim analysis leases solve executable ownership.

No mapped counterpart replaces Kit wire/domain semantics, the required goja pool/Go I/O bridge, target-specific Swift/native compilation, typed environment schema or source-preserving project/template behavior. Retain existing direct upstream reuse there; adopt a later concrete counterpart if found, not an analogy.

## Delivery and ordinary validation

Keep development usable after each selected assignment through existing supervisor, Worker/source view, staging, adapter and compiler seams. Remove the superseded path as its replacement lands. Do not accumulate permanent polling/watching alternatives or postpone useful work for an idealized platform.

After each usable delivery, compare the same representative real-app edit before and after: save to work starting, backend readiness and visible browser update. Include rapid saves and error/repair when affected. Check what work disappeared, what custom code was removed and what ongoing complexity the change introduced. Use ordinary logs, the existing tests, the diff and the actual app; no new benchmark or reporting framework. Discuss those observations with the user before expanding into another substantial assignment. If the expected gain is absent or complexity is growing without useful behavior, narrow or change the approach. Choose the next largest remaining opportunity from what was observed, rather than continuing down the numbered list automatically. Necessary analysis/check improvements serve that path; unrelated diagnostics need not hold up a successful visible update.

Handoffs provide mission, upstream source, behavior and ownership. Builders do their own mapping. An independent validating agent checks that relevant tests would fail if the capability were broken/removed, then runs them as repository instructions require; do not turn that into a mutation-testing framework.

Use literal fixtures and Go tests at the real handler for HTTP/generated-byte/ownership contracts. Kit-client scenarios cover what needs its client or a real no-script form. Run appropriate existing `just test`/`just e2e` recipes without silent skips, including both E2E modes on main when integrating. Reuse expensive output instead of rebuilding per assertion. Open the actual example for substantial behavior changes. A green command alone does not establish an inspected usable page or an unexecuted guarantee.

No new gate runner, proof directory, ledger, scheduling estimate, generalized revision/scheduler/filesystem/compiler/loader framework or additional plan is part of this work. Bring an actual consequential gap back before inventing a new mechanism.
