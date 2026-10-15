# Build the SKGO dev server

## Direction and next build

Build one `skgo dev` command that serves Go application responses, renders SvelteKit pages in pooled goja runtimes, and uses Vite for frontend development. A compatible Go body edit replaces code in the running application while preserving initialized state and browser state. A structural edit builds and installs a new application host behind the same public listener.

Continue from the working generated native replacement implementation. Its retained-revision checkpoint is qualified on Go 1.27.2 Mac ARM64 and Linux AMD64. The next build removes cgo from that implementation. The following build adds selective reclamation alongside broader compatible retained cases. Then complete the generated-handler contract and connect it to retained project inputs, live rendering and the development session.

The lifetime policy is straightforward: **replace compatible code, reclaim revisions in the demonstrated safe class, and retain other compatible revisions within a finite budget.** Exhausting the budget replaces the host and visibly resets its application state. This separates which edits can replace code correctly from which old revisions can be reclaimed.

| Stage | Result | Dependency |
|---|---|---|
| 1. Generated native replacement | Cgo-free replacement, coherent requests, preserved host state/types, selective reclamation and bounded retention | Qualified retained checkpoint already supplies the foundation |
| 2. Retained project inputs | File events feed selective analysis, generation and compiler inputs from the same captured source | Can proceed alongside remaining stage 1 work |
| 3. Live SvelteKit rendering | Vite’s module lifecycle drives Go-rendered documents and frontend HMR | Can proceed alongside stages 1 and 2 |
| 4. Integrated `skgo dev` | One command owns the complete save-to-browser transition | Stages 1–3 |
| 5. Complete edit and recovery behavior | Structural changes, diagnostics, streams and process recovery work through that command | Stage 4 |
| 6. Performance and qualification | Measured useful edit latency, bounded resources and the complete developer experience | Stage 5 |

Each bounded build has a definition of done. Once independent validation establishes it, stop that build. An improvement that can be omitted while still meeting the definition belongs to later work. Implementation handoffs state the capability, accepted proof, ownership and source routes; builders map upstream code and choose the detailed implementation.

[Working foundation](#working-foundation) · [System behavior](#system-behavior) · [Build sequence](#build-sequence) · [Behavior matrix](#behavior-matrix) · [Upstream adoption](#upstream-adoption) · [Validation and delivery](#validation-and-delivery)

## Working foundation

Replacement work integrates on `dev`, whose post-deletion foundation is `b2f2c00483139b821072a94d22875674909e9eaf` ([PR #322](https://github.com/tylergannon/skgo/pull/322)). The implementation uses the surviving production generator, `generate.Prepare`/`Run`, source-view materializer, typed registries and pooled goja renderer. The replacement is a new development implementation built on those facilities and the upstream mechanisms below.

The generated order application already proves captured Go source → compiled object → native installation → real typed HTTP response:

- A command changes quantity from `2` to `3`; the query returns literal `4200`. After a 10% helper replacement, a new request returns `3780` while a held old request remains active. Releasing it returns coherent old values `4200`/`4200`.
- A 20% helper revision produces `3360` through both generated queries, including the caller whose source did not change. PID, initializer count `1`, quantity `3`, compiled result types and named-query refresh survive the swaps and forced GC.
- A syntax failure leaves the installed application usable; repaired source compiles and installs in that same host. Method-dispatch and missing-host-storage incompatibilities are identified before loading.
- A fixture budget of two loaded revisions refuses the third before object access, while the installed `3360` response remains usable. This establishes bounded admission; the integrated session will perform host recycling.

The same test passed on native Go 1.27.2 [Ubuntu AMD64](https://github.com/tylergannon/skgo/actions/runs/38658130587/job/116172867201) and [Mac ARM64](https://github.com/tylergannon/skgo/actions/runs/38658130587/job/116172867544). Independent validation removed revision-local helper binding and obtained the expected mixed-revision failure. This is the qualified application primitive on which the remaining stages build the usable development server.

Backend research supports continuing the native implementation and adapting its OS wrappers to remove cgo. The entire checkpoint passed experimentally on Mac with `CGO_ENABLED=0`; the maintained build and Linux execution are the next qualification work. iXGo also demonstrated a useful method-free GC-owned lifetime class, which remains recorded as an alternative opportunity. The [research index](/Users/tyler/src/skgo-token-cache/index/reload-applicability.md) and [Fable review with evidence qualifications](/Users/tyler/src/skgo-token-cache/collection/fable-reload-recommendations-20261014-context.md) preserve the comparisons and their limits. This plan proceeds with native loading on that evidence; a later concrete backend blocker can be assessed against the same application contract.

## System behavior

### Ownership and routing

A long-running Go session owns the public socket, project state, build work, diagnostics and every process it starts. It launches Vite through the project’s VitePlus environment and a Go application host. The host owns initialized application state, code replacement and the mandatory goja runtime pool. Vite owns frontend compilation, its module graphs and HMR; Kit’s client owns navigation and query/load invalidation.

The public listener and browser use one origin matching Kit’s `paths.origin`. A private loopback Vite listener supplies tooling, transformed modules, assets and HMR. Go’s standard HTTP proxy and upgrade facilities compose that boundary with Go application responses. Go answers page documents, loads, remotes, actions and endpoints; Node serves development tooling.

The routing boundary follows the installed Kit/Vite middleware precedence, including Kit assets, Vite transforms and filesystem access rules, removal of early static/public middleware, and Kit’s saved static responder for existing allowed files and after application 404s. Preserve configured base/assets paths, service-worker entry handling and supported Chrome-devtools requests. For example, an allowed existing `/package.json` retains its file response even with an HTML Accept header; a genuinely missing application path receives Go’s rendered 404.

Stage 3 preserves Kit’s frontend matcher-module/export/name validation while establishing tooling fallthrough to Go. Stage 2 owns paired Go matcher declarations and typed-registry validation. A route/registry revision publishes only after both validations succeed.

Production continues through its existing generator, adapter, handlers and pooled goja renderer. Application I/O executes in Go. JavaScript in the embedded engine executes Kit/Svelte rendering and application components; Go owns file, network and timer operations. The production binary remains cgo-free with Node needed only at build time.

### A save through publication

The session tracks desired source, installed application and Vite environment lifecycle separately.

1. File events update captured bytes and membership. Equal-content noise leaves the source revision unchanged; configuration and workspace changes invalidate their affected view.
2. Shared gopls-derived state identifies affected analysis and generation. Stock Go’s artifact identities determine compilation freshness independently of generation cache hits.
3. Generation and compilation consume the selected captured inputs. Compatible work is shared, obsolete work is canceled where supported, and admission checks the desired revision again.
4. Generation stages its owned outputs. Compilation prepares either compatible replacement objects or a full application candidate. Publication coordinates generated frontend outputs, the Kit graph and the application before they become visible.
5. The host or new application prepares successfully, then the session commits the installation and emits ordered readiness through Vite. Kit refreshes active Go queries/loads. Frontend-only changes follow Vite’s normal HMR lifecycle.

Stage 2 extends the existing generation source-view/output owner with prepare/cancel/commit semantics. Cancellation also governs diagnostics and browser events, so only the current revision can publish them. Requests use installed code while compilation proceeds. Initial failure yields a diagnostic response; later failure keeps the last successful application serving and displays the current terminal/browser diagnostic. Repair installs the corrected revision and clears it.

### Compatible code and bounded lifetime

The host owns initialization, existing Go types and mutable package storage. Stable named entries preserve function identity for generated bindings and `skgo.Refresh`; installation atomically publishes a complete implementation table. Each admitted request and its nested execution observe one revision, including changed helpers and affected callers with unchanged source bytes.

Initial code replacement covers synchronous body/helper edits with unchanged declarations, using arguments, existing structs and initialized state. Compatible closures, sorting comparators, boxed formatting arguments, errors and values backed by a loaded revision can use retention. Changes to signatures, declared types, package storage or initialization select a full application build. Generic function replacement and new asynchronous execution also take the full-build path at this stage. Existing live/deferred features retain their Kit semantics; admitting their execution for replacement requires the lifetime proof below.

Before loading, establish host storage/type compatibility and coherent executable caller coverage, including inlined copies. An edited helper with callers outside the replaceable boundary selects a full build. Candidate initialization must preserve host ownership: executable initialization requires rebuilding, while a bare initialization-task record alone is not executable initialization.

A revision’s lifetime covers execution and consumption, including serialization, SSR and any admitted deferred producer after its HTTP handler returns. Nested calls/internal fetches borrow the enclosing revision lifetime; unrelated host-only streams remain independent. Reclamation eligibility requires both a supported revision class and completed consumers. Compatible revisions with longer-lived backing or slow consumers remain retained and charged to explicit mapping/count budgets. LRU ordering can choose among eligible victims or release cache ownership while live consumers pin their revisions.

Budget exhaustion and uncertain partial runtime loading cause host replacement through the session. Structural edits select a full build before installation. Both paths keep the public listener and Vite alive, and report the application-state reset. Shutdown has bounded drain and cancellation.

### Build profile and toolchain

Stage 1 owns host/object compilation, `importcfg`, dependency archives, loader bindings and compatibility/lifetime results. Development-transformed source and loader dependencies live in a private build view; authored source/module declarations and ordinary generated production bindings retain their production form. After a development session, ordinary `go generate` and `CGO_ENABLED=0 go build` work directly. The SDK/CLI build remains independent of the development loader and private GOROOT.

The host, objects, dependency archives and compiler-source view share the selected toolchain identity and compatible settings. Preparation uses a private source view while the installed toolchain stays intact. Artifact mismatches are diagnosed before loading. Unsupported platforms/Go series and missing actual prerequisites produce startup diagnostics before readiness.

Select Go 1.27.2 explicitly for initial native Mac ARM64/Linux AMD64 qualification; keep those host tests in both CI lanes. Later patch support is established by the same tests, and a new Go series is qualified before advancing its development pin. This is independent of the library’s minimum Go directive and the scaffold’s policy of recording its running Go version.

## Build sequence

### 1. Complete generated native replacement

**Capability:** A freshly generated application’s ordinary Go handlers use compiled replacement bodies with stable identity/state and bounded code lifetime.

**Owner:** Compilation/loading and development dispatch, including the private host build view, compiler artifacts, compatibility decisions and code lifetime. Stage 2 supplies captured source/analysis/generated inputs. The existing retained checkpoint is the starting implementation.

Build the following checkpoints in order, completing each at its own definition of done.

#### 1a. Cgo-free native replacement — next build

A developer builds and uses the existing replacement primitive with `CGO_ENABLED=0` and no C compiler requirement. Adapt the loader’s OS boundary using the [native no-cgo experiment and upstream sources](/Users/tyler/src/skgo-token-cache/index/reload-native-nocgo-applicability.md), preserving compilation, bindings, dispatch and retained lifetime behavior.

**Definition of done:** The full existing generated-host test passes without skips on native Go 1.27.2 Mac ARM64 and Linux AMD64 with `CGO_ENABLED=0`. It preserves both queries, held old/new requests, mutated state, named refresh, syntax repair and binding/budget outcomes. The maintained preparation/build path works without a C compiler, production build isolation holds, and any exposed process-symbol/library lookup preserves the original process-handle semantics, including `dlopen(NULL)` behavior. An independent validator checks the assertions are load-bearing and runs the real-host tests.

This build changes platform integration only. Its definition is complete when those behaviors pass; reclamation, expanded edit classes and server/browser integration have their own checkpoints below.

#### 1b. Selective reclamation with compatible retention

A useful synchronous body/helper class becomes reclaimable, while a compatible value backed by another revision can stay in application state across replacements. Choose the bounded class and retained counterpart from representative calculation/helper, validation/error, formatting or sorting edits.

**Definition of done:** Hold consumption across publication, verify coherent old/new results, then observe the eligible revision’s native mappings disappear only after release using OS address ranges. In the same host, positively assert a supplied candidate-backed value’s behavior and continued backing across later swaps and GC. Both cases preserve binding checks; retained revisions consume the finite budget and exhaustion refuses another load. These paired behaviors complete this build. Stage 4 owns actual host recycling.

Reclamation uses a small explicitly supported mechanism. Compatible cases beyond it use retention. General value-flow analysis, generic deep copying and speculative loader changes lie beyond this checkpoint. The [prior consultation](/Users/tyler/.codex/worktrees/30de/skgo/ephemeral/projects/dev-server/fable-diminishing-returns-raw.json) supplies object-check ideas whose generic-dictionary and symbol assumptions still require verification; it is research input for choosing the mechanism.

#### 1c. Complete the generated-handler contract

Extend the same primitive through the remaining handler, recovery and consumption cases:

- A Go server load returning an existing struct changes its rendered HTTP document; endpoint/action edits change supplied responses. Typed errors, cookies, codecs and command-driven named refresh retain their behavior through swaps.
- Two generated callers see the edited helper, including an unchanged caller. An external uninstrumented caller selects a full build before partial installation; stage 5 verifies the resulting application reaches every caller.
- Held execution/serialization stays coherent through publication and GC. Slow consumers select retention within budget. Any admitted live/deferred producer remains backed after handler return through actual consumption.
- A nested internal request/helper completes while publication waits, and an unrelated live stream permits a compatible synchronous update.
- Compiler failure preserves the working result/state and repair installs. Structural or host-binding changes select rebuilding. Compatible synchronous closures and boxed/retained values can select retention.
- A replacement body/helper panic reports the supplied panic at its authored file/line, releases lifetime ownership, and leaves the same host usable. Healthy and repaired requests return literal values with unchanged PID and initializer count.
- Startup diagnostics cover unsupported platforms/Go series and missing actual prerequisites. Mismatched host/object artifacts are rejected before loading.
- In one generated project, run and stop development, then build/run production with cgo disabled using the existing authored/generated tree. Assert the supplied production result and isolated development dependencies.

The Go tests build a separate real host with symbols retained (`-ldflags=-s=false` at the initial pin), drive its actual composed HTTP handler, and share expensive preparation across assertions. Reuse preparation under a key covering toolchain and preparation inputs; rebuild hosts for changed source/dependencies. Measure suite cost before adding cache machinery.

**Stage completion:** The generated-handler contract passes on both native target platforms with the cgo-free profile, selective reclaim/retain behavior, production isolation and toolchain checks. Stages 2 and 3 may proceed under separate ownership; stage 4 connects the completed capabilities. A concrete contract blocker is isolated at its owning seam and reported with the failing case and smallest proposed adaptation.

### 2. Retained project inputs and selective generation

**Capability:** Each observed source change updates retained state, and affected generation and compilation consume the same selected bytes.

**Owner:** Project/file state and shared analysis/generation, extending existing production source-view/output staging. Compilation artifacts remain with stage 1; any polytype changes use their own worktree.

**Accepted proof:**

- A real temporary application with a local replacement and embed directory responds to atomic saves, preserved-mtime writes, populated-directory creation and rename/removal with fixture-defined outputs and compiler inputs. File events acquire fresh changed content; equal content suppresses redundant work.
- Hold generation/input consumption, save another fixture, and release work in either order. Only the current revision publishes outputs, inputs or diagnostics; diagnostic locations identify the authored file/revision.
- A body edit reuses generation and supplies its new body to compilation. Struct/tag/constant/marker changes regenerate the affected outputs. Generated writes settle without self-build loops, and removed producers remove their owned artifacts.
- A local dependency change or new embed match invalidates actual compiler inputs even with unchanged type projection. Stage 1 verifies the resulting imports/archives against host and dependency identities.
- A route `[n=Order]`, metadata naming `Order` and paired Go `Order(string) (OrderNumber, bool)` produce the named value at the handler. Missing or wrongly typed declarations give diagnostics and block publication; repair resumes matching for pages and endpoints. Stage 3 validates frontend exports/names; stage 4 checks client navigation agrees with Go.
- Routine edits read the changed/affected closure, retain unchanged analyses and recover after invalid intermediate source.

**Completion:** Event-fed captured inputs drive either replacement compilation or a full application build with bounded retained state. Stock Go owns compilation and its cache.

### 3. Live SvelteKit rendering

**Capability:** Go renders live Kit pages from a Vite environment, and SSR/frontend HMR follow Vite’s real module lifecycle.

**Owner:** Adapter/client glue, ModuleRunner/goja integration, private tooling fallthrough and live Kit metadata. Stage 2 owns Go project inputs; Vite owns frontend watching.

First run a bounded compatibility test of Vite’s actual ModuleRunner in pooled goja using its evaluator/transport seams. Bundle the upstream library through the adapter build; Go owns I/O, external map loading and required timer/transport work. Configure Node-specific interception/debug facilities off and reject external module execution. If a concrete engine or ownership incompatibility prevents direct reuse, port the required named runner algorithms into Go. The same tests qualify either integration. The result records the exact reuse/port boundary.

Establish the Kit tooling/application seam through supported integration or an owned pinned Kit correction. Preserve tooling/static precedence, metadata validation, configured base/assets behavior and fallthrough to Go application handling. This stage owns the private boundary; stage 4 composes the public origin.

**Accepted proof:** Upstream-style alias, cycle, pending/shared evaluation, cache and update cases pass. A changed dependency with unchanged importer bytes updates the next SSR document; deletion/recreation and import-edge changes recover. Vite cache/invalidate replies and environment reconnect/reset work. Actual Kit branch/node/template/style data, Go callbacks and renderer-pool reentrancy work through the handler. An authored component exception reports its supplied file/line through Vite maps and goja stack support.

Each render holds its selected manifest/registry and a checked-out runtime. Apply ordered environment updates at runtime admission. Already evaluated old code can finish; if preparation needs a module from an invalidated environment, discard/retry rendering before response commitment. A committed stream that loses consistency is canceled and recovered through the update lifecycle. Resolve a submitted command, action or form once and retain its Go result across render preparation. Rendering retries reuse that result; a terminal rendering failure surfaces a controlled error after those application effects.

Exercise an update during a held Go-backed async render, dynamic import and action. Assert coherent supplied document values, controlled stream recovery and one action mutation. At the private listener, test real module/CSS/asset/HMR responses, allowed `/package.json` bytes, static/route collisions, denied-file rules, missing matcher exports/names and repair. Application requests explicitly fall through to Go.

**Completion:** Live Go-rendered documents and Vite HMR share the tested lifecycle, with authored source positions, consistent renders and the complete private tooling boundary. The existing pooled renderer remains the production infrastructure.

### 4. Integrated `skgo dev`

**Capability:** One command in the example and a fresh generated application owns startup and every save through installation and browser refresh.

**Owner:** Session integration, command/template/recipe wiring, process control/readiness and adapter-to-Kit readiness delivery. Restore `just e2e dev`, Playwright’s development selection and its server-mode assertion here. Shared scenarios run in both modes; edit scenarios run in development.

**Accepted proof:** A Go test launches the real command, saves without issuing requests, observes build work through process/control state, then gets the new fixture response. A browser edits a meaningful compatible body and sees the new value with the same application PID, document identity, local counter, shallow state and unsent input. Generated applications receive dispatch and registration automatically through the ordinary command.

During unpublished `dev` work, qualify a fresh app using existing checkout overrides: fixture `--skgo-version`, `--skgo-replace <checkout>`, `--adapter file:<checkout>/internal/adapter` and `--sv-addon file:<checkout>/internal/sv`. Corresponding `newapp.Options` are `SkgoVersion`, `SkgoReplace`, `AdapterSpec` and `SVAddonSpec`. Use isolated add-on packaging and VitePlus. After release, repeat with the published CLI and normal registry selection.

Through the public Go origin, prove actual browser module/CSS/asset loading and HMR upgrade, Kit’s allowed-file precedence, a real Go-rendered 404, and Go page/data/remote/action/endpoint responses. Independent private-listener probes verify application fallthrough. Save during generation, compilation and installation, and navigate during refresh: only current source outputs, diagnostics and readiness events can win. Assert visible values/state and coordinated frontend/application revisions.

Budget exhaustion and uncertain-load recovery replace the application host through this session, report state reset and preserve the public listener/Vite. **Completion:** All restored development entry points invoke the same integrated command; its representative compatible edit updates code and the browser while preserving the live host and healthy page state.

### 5. Complete edit and recovery behavior

**Capability:** The integrated server covers the behavior matrix, including edits that require new application state.

**Owner:** Session installation/lifecycle together with the domain owner for each matrix row.

Build a structural candidate while the installed host serves. Readiness includes handlers, manifest and renderer initialization; switch the stable listener after successful preparation. Drain finite requests within explicit bounds and cancel obsolete streams according to their lifecycle. Exclusive startup resources require draining/stopping the prior host before acquisition, with a visible startup diagnostic during the gap. Failed startup can relaunch the last successful artifact when resource ownership permits, accurately reporting which revision serves.

Exercise watcher failure/overflow, Vite restart, application crash, port conflicts and SIGINT/SIGTERM. Reconcile uncertain watch scopes outside requests, diagnose failures and join every owned process/descendant. Process/listener/control observations establish complete cleanup and successful subsequent use.

**Completion:** Every matrix row has its real-handler or Kit-client proof. CLI help, examples, generated recipes and CI describe the integrated command. Restore production/development browser qualification in the existing Ubuntu workflow with its manual dispatch entry point, and pass both modes against the candidate `dev` revision before promotion.

### 6. Performance and qualification

**Capability:** The completed server provides useful edit latency and stays stable through repeated saves.

Measure save-to-visible time across watching, classification, generation, compilation, installation and Kit paint. The initial target is a few hundred milliseconds or better for a representative warm compatible body edit. Measure dependency and structural changes separately. Use ordinary process observations, existing logs and small Go benchmarks to find repeated input acquisition, dependency fences, global projection misses, repeated parsing and unnecessary host links. Remove work justified by those measurements.

Measure root Go suite wall time and keep it in the repository’s seconds tier by sharing expensive preparation. Exercise repeated edits to observe mappings, retained resources and eventual host recycling. **Completion:** Actual example/fresh-app results establish preserved state, full-build behavior, bounded resources and the latency claimed. Complete Go tests, both browser modes and visual inspection qualify the finished server.

## Behavior matrix

Use literal fixtures in Go tests at the composed handler for statuses, headers, cookies, wire bodies and rendered documents. Use browser scenarios for behavior requiring Kit’s client or a real no-script form.

| Edit/situation | Intended behavior | Accepted proof |
|---|---|---|
| Svelte/CSS or imported frontend dependency | Vite HMR; subsequent Go SSR uses the current graph | Visible fixture update with Vite-promised state preservation; literal SSR document |
| Compatible Go remote/load/action/endpoint/helper body | Replace code, preserving contracts and initialized state | New literal result; fixed PID/initializer count; document/draft/local state preserved |
| Compatible synchronous closure, boxed value or retained candidate-backed value | Replace and retain when reclamation is outside the supported class | Supplied behavior survives swaps/GC; backing stays mapped and counts against budget |
| Eligible retired revision | Reclaim after actual consumers finish | Coherent held request/consumption, then native mapping disappearance |
| Budget exhaustion or uncertain runtime load | Replace the host through the session | Last usable response until transition where possible; visible state reset; stable public address and recovery |
| Signature, declared type, package/global initializer change | Regenerate as needed and build a new host | New typed wire/document result; host change; initialization once in that host |
| Go import, local replacement or build selection | Recompute compiler metadata/artifacts; rebuild for incompatible caller coverage | Cross-package supplied result from fresh archives and updated inlined callers |
| Imported constant, tag, marker or consumed comment | Update affected generation and executable callers | Exact generated declaration/codec and handler result, including the indexed constant regression |
| Added/removed/renamed route, layout, endpoint or matcher | Validate the new Kit graph and Go registrations | Matcher diagnostics/repair; literal 404→200→404; client navigation; Kit’s route-change reload behavior |
| App/error template or page options | Apply pinned Kit document/client behavior | Supplied markup; `ssr=false` SPA fallback; `csr=false` script-free document |
| Embed bytes/membership | Invalidate build and registration inputs | Supplied bytes and exact file count including new matches |
| Syntax/type error and repair | Serve the working revision with current diagnostics; install repair | Authored location, positive old value and literal repaired value |
| Replacement panic | Recover at the request boundary and preserve the host | Supplied panic/authored stack, fixed PID/init count, healthy and repaired responses |
| Query error boundary/failed load and repair | Kit-supported refresh/recovery | Recovered value and boundary state; explicit reset when Kit remounts/reloads |
| Batch/command/form refresh, live query, deferred load | Preserve Kit semantics and actual-consumer lifetime | Coalescing, enhanced submit, reconnect, streamed value, named refresh and usable no-script form |
| Rapid saves or save during build/publication | Current desired source wins | Ordered supplied revisions released adversarially; current bytes/results/diagnostics |
| Render/dynamic import during update | Coherent document or controlled retry/stream recovery | Paired fixture values and complete recovered content; action effects occur once |
| Vite/host restart, watch uncertainty, cancellation | Reconcile or diagnose and clean up owned resources | Process/listener/control observations and successful subsequent use |

## Upstream adoption

Use the [semantic index](/Users/tyler/src/skgo-token-cache/index/README.md) for pinned source and experiment retrieval. Direct reuse executes the upstream library or running facility. An attributed semantic port copies the named algorithms into Go, adapting types and ownership while preserving behavior and notices. Internal upstream Go packages use that port approach. SKGO integration joins these facilities to production registries, build ownership and the renderer pool.

The mapping pins are Kit `3.0.0` with `skgo-kit-3.0.0-prerender-queue-v1` (installed queue SHA-256 `400bf34544e2b3c5512e489eb5a0ca99a7483f0b6d733f70cab9e81e28f3a0f5`), VitePlus Core `1.0.0` containing Vite `8.3.1`, readable Vite `8.3.4` at `c2c9ef4`, x/tools `v0.52.0` at `16517d9`, and GoLoader `2695cfc`. At each handoff verify the actual installed pins and map behavior from the installed Kit. Check readable Vite against the installed version before transferring algorithms. Record copied functions, revision and provenance in the working material. VitePlus owns frontend invocations.

| Mechanism and adoption | Specific upstream code | SKGO integration |
|---|---|---|
| Filesystem event delivery — **direct reuse + attributed Go semantic port** | Reuse `github.com/fsnotify/fsnotify`. Port gopls [`NewFSNotifyWatcher`, `run`, `process`, `convertEvent`, `WatchDir`](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/filewatcher/fsnotify_watcher.go:26). | Use fsnotify and port ordered event delivery and recursive directory registration into the Go project owner. Adapt protocol/configuration types and use low-latency scheduling for development. |
| Watch scope and directory expansion — **attributed Go semantic port; direct Vite reuse** | gopls [`Session.FileWatchingGlobPatterns`](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/cache/session.go:1164), [`Snapshot.fileWatchingGlobPatterns`](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/cache/snapshot.go:815), [`ExpandModificationsToDirectories`](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/cache/session.go:969); Vite [`addWatchFile`](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/node/server/pluginContainer.ts:867). | Port workspace-derived watch selection and directory-event expansion for Go packages, local replacements and embeds. Vite owns module watching through its plugin registrations; SKGO supplies project membership. |
| Shared immutable source state — **attributed Go semantic port** | gopls [`invalidateViewLocked`](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/cache/view.go:785), [`Snapshot.clone`](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/cache/snapshot.go:1504), `ReadFile`, `Acquire`. | Port changed-file handles, snapshot invalidation and acquire/release ownership into retained inputs feeding `cacheSources`/`generationSourceView` in `internal/gen/cache_source.go`. Extend the existing materializer. |
| Metadata, reverse dependencies and staged validity — **attributed Go semantic port** | gopls [`metadata.Graph`](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/cache/metadata/graph.go:20), [`metadataChanges`](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/cache/snapshot.go:2032), [`packageHandle`, `evaluatePackageHandle`](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/cache/check.go:817). | Port graph updates and staged validity/key rules into the shared analysis owner feeding `prepareLoadParams` and `runCached`/`generationPlan`. Include tags, consumed comments, constants, markers and membership. Step 1 separately establishes executable caller coverage. |
| Shared parse/computation — **attributed Go semantic port** | gopls [`parseCache`](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/cache/parse_cache.go:149), [`future.go`](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/cache/future.go:12), `memoize.Promise`. | Port shared-computation/key equivalence, waiter cancellation and file-position ownership into that same analysis owner. |
| Valid pending work and stale admission — **attributed Go semantic port** | Vite [`transformRequest`](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/node/server/transformRequest.ts:78), result admission at 438; [`runOptimizeDeps`, `successfulResult.cancel`/`commit`](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/node/optimizer/index.ts:523). | Port pending-work reuse/invalidation and prepared-output cancel/commit into generation publication and installation. Go contexts own cancellation; leased application artifacts retain their own cleanup lifecycle. |
| Go command ownership — **attributed Go semantic port; direct compiler reuse** | x/tools [`gocommand.Runner`, `Invocation`, `runCmdContext`](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/internal/gocommand/invoke.go:33). | Port command sharing/serialization for module updates, cancellation, stdio cleanup and logical PWD handling. Stock `cmd/go` owns compilation and compiler caching. |
| Stable named dispatch — **attributed Go semantic port + SKGO integration** | got-reload [`rewriteFunc`](/Users/tyler/src/skgo-token-cache/repos/reload-interpreters-watchers/got-reload/pkg/gotreload/gotreload.go:460), `rewritePkg` and registration generation; [SKGO boundary fixture](/Users/tyler/src/skgo-token-cache/experiments/dev-preflight-20261014/skgo/ephemeral/dev-preflight/skgo-boundary/boundary_test.go:29). | Continue the implemented stable named-wrapper transformation with compiler-produced targets and typed registries. Step 1 owns state/type binding, coherent helper calls and lifetime. |
| Go object loading — **direct pinned dependency + SKGO integration** | GoLoader `2695cfc`: [`ReadObjs`](/Users/tyler/src/skgo-token-cache/repos/reload-native/pkujhd-goloader-current/link/readobj.go:69), [`Load`](/Users/tyler/src/skgo-token-cache/repos/reload-native/pkujhd-goloader-current/link/ld.go:561), `addSymbolMap`, [`doInitialize`](/Users/tyler/src/skgo-token-cache/repos/reload-native/pkujhd-goloader-current/link/init.1.21.go:125); [POC publication](/Users/tyler/src/skgo-token-cache/experiments/history-native/poc/reload.go:223). | Use the revision-locked loader in the private development host. Continue its generated integration and add the bounded cgo-free platform adaptation. Step 1 owns matching compiler artifacts, storage/type binding, draining and uncertain-load recovery. |
| Vite environment lifecycle — **direct reuse + SKGO integration** | Vite `onFileChange`, environment `hotUpdate`, [`DevEnvironment`](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/node/server/environment.ts:40), `HotChannel`. | Use the actual Vite environment and custom-environment hooks. Installed 8.3.1 dispatches them after graph invalidation; connect Go readiness through that lifecycle. |
| Module graph and HMR boundaries — **direct Vite reuse** | [`EnvironmentModuleGraph.updateModuleInfo`, `invalidateModule`, `onFileChange`, `onFileDelete`](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/node/server/moduleGraph.ts:145); [`updateModules`](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/node/server/hmr.ts:683), [`propagateUpdate`, `isNodeWithinCircularImports`](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/node/server/hmr.ts:808). | Run Vite as the owner of its server graph, reverse edges, invalidation, HMR acceptance boundaries and cycle-to-reload decisions. The evaluator owns its separate evaluated-module cache with upstream runner semantics. |
| Per-file read stabilization — **direct Vite reuse; attributed Go semantic port where Go reads** | Vite [`resolveChokidarOptions`](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/node/watch.ts:61), [`readModifiedFile`](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/node/server/hmr.ts:1135); gopls equal-content check in [`metadataChanges`](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/cache/snapshot.go:2032). | Keep Vite watch/read behavior. Where Go reads need stabilization, port bounded per-file retry tied to fresh bytes and equal-content suppression. Test preserved-mtime writes, transient empty reads and genuinely empty files. |
| Close, restart and warmup — **direct Vite reuse + SKGO process integration** | [`DevEnvironment.close`](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/node/server/environment.ts:369); server [`close`, queued `restart`](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/node/server/index.ts:835), [`restartServer`](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/node/server/index.ts:1408); [`warmupFiles`, `warmupFile`](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/node/server/warmup.ts:10). | Invoke Vite-owned close, queued restart and configured warmup. SKGO joins its Go work/children and resets the evaluator when Vite replaces an environment. |
| Complete evaluated-module runner — **slice-owned investigation: direct reuse first, attributed Go semantic port if incompatible** | [`ModuleRunner`](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/module-runner/runner.ts:39), [`ModuleEvaluator`](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/module-runner/types.ts:49), [`EvaluatedModules`](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/module-runner/evaluatedModules.ts:11), `cachedRequest`, `getModuleInformation`; [`HMR handler`](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/module-runner/hmrHandler.ts:7), shared payload queue and transport lifecycle. | Stage 3 first tests the actual upstream library in goja with Go evaluator/transport ownership. A concrete incompatibility selects an attributed Go port of the required named algorithms. Both paths preserve aliases, pending evaluation, cycles, dependency edges, cache/invalidate replies and ordered updates. |
| Rendered exception source positions — **direct Vite/goja reuse + SKGO integration** | Vite [`fetchModule` inline maps/`startOffset`, `inlineSourceMap`](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/node/ssr/fetchModule.ts:101); goja [`parseSourceMap`](/Users/tyler/src/skgo-token-cache/repos/goja-e9f698900731/parser/statement.go:945), [`File.Position`](/Users/tyler/src/skgo-token-cache/repos/goja-e9f698900731/file/file.go:158). | Pass inline maps and the evaluator wrapper offset into goja’s existing map/stack support. Go owns external map loading. Stage 3 determines the offset by raising an exception at a supplied authored component line. |
| Kit tooling/static/application split — **direct Kit reuse; slice-owned seam investigation** | Each worktree's verified installed Kit: `example/web/node_modules/@sveltejs/kit/src/exports/vite/dev/index.js:301`, saved static responder at 325–338 and 451–455, `update_manifest` at 101; `src/core/params.js:46` within that package. | Stage 3 establishes the integration seam, using supported hooks or an owned pinned Kit correction. Preserve tooling, saved-static-responder precedence and frontend matcher validation while application responses fall through to Go. |
| Browser refresh and recovery — **direct Vite/Kit reuse + SKGO readiness integration** | Vite custom HMR delivery; pinned Kit `refreshAll` and `_invalidate`; [executed refresh preflight](/Users/tyler/src/skgo-token-cache/experiments/dev-preflight-20261014/skgo/ephemeral/dev-preflight/kit-refresh/README.md). | After installation, emit readiness through Vite and call Kit’s public refresh API. Kit owns invalidation and its distinct healthy-refresh, error-boundary and CSR-disabled recovery behavior. |

## Validation and delivery

This document defines the implementation order and accepted behavior. Source indexes, historical experiments and review records supply supporting evidence. Each stage resolves its detailed interfaces inside its stated ownership; the explicit runner compatibility test and Kit seam work are engineering tasks within this plan.

An independent validation agent checks that selected assertions are load-bearing and then runs them. Removing replacement, invalidation or readiness must fail the corresponding positive assertion. Expectations come from supplied fixtures and literal values. Share expensive setup and reuse captured output across assertions. Verification uses ordinary Go tests and the real program.

For the complete server, pass `just test`, `just e2e prod` and `just e2e dev` with all required cases executed. Run the production example and a fresh generated application with the cgo-free, build-time-only Node boundary. Open and inspect the rendered development application after substantial changes. Before promotion, dispatch the restored Ubuntu workflow against the candidate branch with `gh workflow run qualification.yml --ref dev` and verify both browser modes. Repeat affected qualification when changes invalidate its result.

Use worktrees based on current `origin/dev` and PRs targeting `dev` (`just worktree` accepts an explicit `dev` base). Integrate relevant `main` advances into shared `dev` with a Conventional Commit such as `chore: sync main into dev`; preserve shared history and reconcile against this application contract. Rebase task branches onto current `dev` before squash merging them. Checkpoints can integrate there while development qualification is being restored; releases follow completed qualification.

Promote the fully qualified combined state in one squash PR from `dev` to current `main`, titled `feat: replace the dev server`, using the normal feature release semantics. Compute the version from the actual main tag and intervening commits at promotion. The deletion’s breaking marker remains in integration history rather than the promotion’s squash message. The main release workflow then publishes the completed result, followed by the published fresh-app pairing check.

Delivery is complete when the integrated command covers the matrix, meaningful generated-application edits preserve healthy state, structural changes and recovery work, repeated edits stay bounded, and measured latency supports the result. Each completed bounded build remains complete while work advances to the next capability.
