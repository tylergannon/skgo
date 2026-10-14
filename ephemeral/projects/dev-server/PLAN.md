# Build the new SKGO development server

## The plan in brief

Start from **`dev` at `b2f2c00483139b821072a94d22875674909e9eaf`**, where [PR #322](https://github.com/tylergannon/skgo/pull/322) completed the deletion requested in #321. Build one new Go development system from the indexed upstream implementations. The old server and failed branch supply no implementation foundation or compatibility requirements.

`dev` is the temporary integration branch for deletion and the entire replacement. Worktree branches and replacement PRs target `dev`; incomplete checkpoints are allowed there without publishing releases. Production checks still mean what they say, and unavailable development qualification stays explicitly unavailable until restored. Promote the finished replacement to `main` only after the complete validation below. This plan does not require incomplete development functionality to remain releaseable at each checkpoint.

The engineering order is:

1. Establish generated native execution through real SKGO handlers, including stable function identity, ordinary bodies, host state and lifetime.
2. Establish retained, event-fed source and build inputs shared by generation and compilation.
3. Restore development rendering using Vite's actual environment/ModuleRunner lifecycle in pooled goja.
4. Connect those capabilities into ordinary `skgo dev`, from save through application installation to Kit refresh.
5. Complete the structural-edit, error, dependency, stream and process-lifecycle behavior.
6. Remove remaining unnecessary work and qualify the complete developer experience.

These are implementation missions with accepted behaviors and dependency boundaries, not optional product modes. Native integration is the first priority and risk to resolve. Steps 2 and 3 are independent of its implementation and can proceed alongside it under separate ownership; step 4 requires all three. Steps 1–3 can be built and tested without advertising a usable development server. The integrated replacement is delivered when steps 4–6 satisfy the complete behavior below. No old server remains available during reconstruction.

[Product and process](#product-and-process) · [Upstream mechanisms](#upstream-mechanisms) · [Execution sequence](#execution-sequence) · [Behavior matrix](#behavior-matrix) · [Validation and finish](#validation-and-finish)

## Product and process

### One owner of the save-to-browser transition

The session is a long-running Go program owning the public socket, project state, build work, diagnostics and every process it starts. It starts Vite through the project's VitePlus environment and a Go application host. The application host owns its initialized application state, native code installation and the mandatory goja renderer pool. Vite owns frontend compilation, its environment graphs and frontend HMR. Kit's own client owns query/load invalidation and navigation.

Keeping the application host separate from the session lets a structural application rebuild replace the host without losing the public listener, project state or Vite. Native edits replace code inside the host without restarting it. These are two installation operations owned by the same new session. There is no server-selection switch or compatibility path.

**Browser/Vite topology:** The browser uses only the Go session's public origin, matching Kit's configured `paths.origin`. Page documents (including application 404s), server loads/data requests, remote functions, actions and matched endpoints always go to the Go application host. Remaining tool/module/asset requests and a dedicated HMR WebSocket path reach the private loopback Vite listener through Go's standard HTTP reverse-proxy/upgrade facilities. Follow Vite's middleware recognition/precedence rather than trying to enumerate its dynamic URL space in a manifest allowlist: Vite handles optimized dependencies, `/@id`, `/@fs`, query-qualified CSS/imports, maps and bootstrap/ping paths itself. Its private listener keeps Kit's non-post asset middleware (`dev/index.js:272`) but suppresses Kit's post-installed page/data/remote handler (`:311`), so forwarding can never fall through to Node application execution. Vite advertises the public origin/WebSocket endpoint through configuration supported by the actual installed pin. Step 3 owns this adapter routing boundary; step 4 owns public Go routing and the origin/control handshake. Tests address both listeners directly and exercise actual dynamic module/CSS/HMR requests through the public listener.

Production continues to use the existing production generator, Go handlers, adapter build and pooled goja renderer. Application I/O executes in Go. The development loader and its platform requirements do not enter a production build.

### What a save does

The desired source revision, installed application revision and Vite environment lifecycle are distinct facts. They must not be represented by one ambiguous “ready” flag.

1. A watcher reports paths. The project owner reads changed bytes/membership and advances desired source state. Equal-content noise does not advance it. Configuration or workspace changes invalidate their corresponding view.
2. gopls-derived metadata and dependency rules determine affected analysis and generators. The native compiler has its own dependency/artifact identity; a generation cache hit does not establish executable freshness.
3. Work consumes captured inputs associated with that revision. Compatible work is shared; obsolete work is canceled where cancellation is supported. Later completion of canceled work cannot admit outputs, errors or code.
4. Generation prepares owned outputs. Compilation produces either a compatible native candidate or a complete application candidate. Compatibility is explicit: source-shape changes, unsupported lifetimes and missing integration are different outcomes.
5. The session verifies the candidate still serves desired source and the relevant Vite lifecycle. The host prepares installation; generated frontend outputs and Kit graph changes are coordinated before announcing readiness. Publication never means “the compile process exited.”
6. After installation, the session sends an ordered readiness event through Vite. Kit refreshes active Go queries/loads. Frontend-only edits continue through Vite's ordinary HMR.

The publication boundary must account for externally visible generation writes. An epoch check after files have already escaped into Vite is too late. Existing generation staging/output ownership is adapted to prepare/cancel/commit semantics; it is not replaced with a new project-copying framework.

Requests use an installed application. They do not walk source, hash the project or wait behind ordinary compile work. Initial failure has a diagnostic response; a later compiler failure keeps the successful application installed and makes current failure visible in the terminal/browser. Repair installs the corrected revision and clears that diagnostic.

### Native execution and state

Use the selected `pkujhd/goloader` over stock released Go compiler output. Step 1's native primitive must pass on **Go 1.27.2 darwin/arm64 and linux/amd64** before its exit condition is met. Start local development on this Mac, and qualify the same primitive on Ubuntu as part of that task, not only after integration. Both loader profiles require cgo (Linux also uses its cgo `libdl`/`-ldl` code). Step 1 owns aligning CI's selected toolchain and the automated private development preparation to Go 1.27.2: current `go-version-file` declarations select 1.27.1 and are not sufficient. Preparing importable object packages uses a private source view of that selected stock toolchain; installed Go remains untouched. Other unsupported configurations produce a startup diagnostic instead of silently losing native functionality; production's platform boundary remains independent.

Adopt the stable named-function dispatch pattern found in got-reload, adapted to compiler-produced native code rather than its interpreter. A generated host entry keeps the original Go function identity; installation changes the implementation reached through it. Generated bindings and function references used by `skgo.Refresh` must agree on that identity. Normal production generation keeps its direct Go calls.

**Build isolation:** Step 1 owns producing the development host as well as its dispatch. Development-transformed Go source, registrations, loader dependencies and compiler/package-access preparation live in a private development build view. They never replace the authored tree's ordinary generated Go files or modify its module declarations. The session automatically builds that profile; the developer does not choose native versus non-native development. The CLI and normal SDK test binary do not statically depend on the loader. After any development session, ordinary `go generate` and `CGO_ENABLED=0 go build` still consume the direct production bindings without a cleanup/regeneration ritual. The native fixture tests explicitly exercise that transition.

The host owns package initialization, existing types and mutable state. Body replacement must not rerun initializers, instantiate a second copy of a package's mutable globals, or give values crossing the boundary incompatible Go type identity. Calls through helpers must execute the intended revision too; replacing only the registered entry while silently calling an outdated helper fails the capability.

An edited helper invalidates its complete executable caller closure, including possible inlined copies. Admit native installation only when every affected caller can be replaced consistently through the native boundary. A helper with an uninstrumented host/background caller, or a closure the implementation cannot establish, requires a complete application rebuild. No partial native update may leave two callers observing different revisions of the edited helper. Analysis typerefs are not this executable caller proof.

This fixes the integration contract, not a new public API. The implementer maps got-reload's rewrite/registration code, GoLoader's symbol/data/type/initialization handling and SKGO's generated typed calls before selecting the smallest implementation. Blind whole-package loading is not justified: GoLoader loads initialization tasks, and its symbol map distinguishes host data and locally loaded text. The manual preflight's host anchor is evidence, not the generated implementation.

The initial native contract is useful synchronous body edits with unchanged declarations, using arguments, existing structs, a helper and initialized state. Cover a remote with arguments, a server load returning an existing struct, and a normal endpoint/action body. Function/signature/type/package-state/initialization changes use complete rebuilding until separately supported. Changes involving new native callbacks, goroutines, deferred values, runtime types **or data that can escape into retained host state** are not admitted merely because the source still has the same signature. Primitive host-state mutation such as the fixture counter is allowed; persisting strings, slices, interfaces/errors or composite values is classified for rebuilding unless the generated boundary establishes host ownership by a demonstrated copy/encoding before retirement. Calls into APIs that may retain candidate-backed values are included in that rule. Do not invent a generic deep copier or assume GoLoader makes every literal safe. These edits remain functional through complete rebuilding; broader native compatibility requires its corresponding lifetime proof.

Native publication must keep executing code and native-backed data alive until the actual consumer finishes. A lease ending when a load returns is insufficient if SSR/serialization still uses its result. The first implementation uses the POC's bounded current-plus-candidate publication discipline for admitted synchronous work, extended through real handler/serialization ownership. It must reject unsupported escaping native lifetime before load. A partially failed runtime load is not a normal compile error: preserve the last desired source, replace the uncertain host through the same session, and explain that recovery. Do not call `Unload` on code or metadata that may remain reachable.

Nested generated calls/internal fetches borrow their enclosing native lifetime; they must not recursively acquire a writer-preferring lock and deadlock a pending publication. Unrelated host-only live streams do not hold a blanket native-publication lease. Coalesce pending edits while retirement is blocked, and use explicit bounded drain/cancellation or application-host replacement when execution cannot retire. Repeated saves never justify unmapping reachable code or accumulating unlimited native revisions.

## Upstream mechanisms

The [semantic index](/Users/tyler/src/skgo-token-cache/index/README.md) is a source router. Its historical instructions to extend the old supervisor are superseded. Source-backed mappings are not application execution proof.

Implementation baseline is post-deletion `dev` at `b2f2c00483139b821072a94d22875674909e9eaf`; `53179fc` remains only the historical SKGO research baseline. The deletion removed `generate.Worker` as well as the old server/runner. Surviving production facilities include `generate.Prepare`/`Run`, the generation source-view materializer, typed registries and the pooled goja renderer; build the new retained development state without resurrecting Worker. The older native/refresh fixtures must be adapted to these surviving APIs, not copied with deleted development flags.

Frontend/source pins: installed Kit `3.0.0` with the repository's `skgo-kit-3.0.0-prerender-queue-v1` compatibility patch (installed queue SHA-256 `400bf34544e2b3c5512e489eb5a0ca99a7483f0b6d733f70cab9e81e28f3a0f5`); VitePlus Core `1.0.0` containing Vite `8.3.1`; readable Vite `8.3.4` at `c2c9ef4`; x/tools `v0.52.0` at `16517d9`; GoLoader `2695cfc`. The refresh preflight used patched Kit too. Implementers check the current `dev` tree and actual installed frontend pin/required patch before mapping behavior. VitePlus owns every frontend invocation.

| Mechanism to adopt | Source and important behavior | Required SKGO adaptation |
|---|---|---|
| Ordered filesystem events and recursive watches | gopls [`fsnotify_watcher.go`](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/filewatcher/fsnotify_watcher.go:26), especially `run`, `process`, `convertEvent`, `WatchDir` | Include routes, embeds and local replacement modules. Carry ordering and receiver/registration separation; replace the 500 ms diagnostics batching policy with the low-latency scheduling required here. |
| Shared immutable source state | gopls [`invalidateViewLocked`](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/cache/view.go:785), [`Snapshot.clone`](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/cache/snapshot.go:1504), `ReadFile`, `Acquire` | Use changed-file handles and retained state. Adapt the production generation source-view materializer to these inputs rather than inventing another recursive capture/tree reconstruction. |
| Metadata, reverse dependencies, staged validity | gopls [`metadata.Graph`](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/cache/metadata/graph.go:20), [`metadataChanges`](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/cache/snapshot.go:2032), [`packageHandle`](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/cache/check.go:817), `evaluatePackageHandle` | Extend existing shared Go analysis owners. Generator keys also include tags, consumed comments, constants, markers and membership; gopls typerefs alone omits inputs SKGO needs. |
| Shared parse/computation | gopls [`parseCache`](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/cache/parse_cache.go:149), [`future.go`](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/cache/future.go:12), `memoize.Promise` | Preserve file-position ownership, key equivalence and cancellation ownership; avoid a second type-loader or global serialization of unrelated work. Internal upstream packages require attributed Go ports. |
| Valid pending work and stale admission | Vite [`transformRequest`](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/node/server/transformRequest.ts:78), result admission at line 438; optimizer prepare/cancel/commit | Apply to generation writes and application installation, not just cache insertion. Vite's pending `abort` evicts a map entry; actual cancellation needs context ownership. |
| Stable named dispatch and symbol registration | got-reload [`rewriteFunc`](/Users/tyler/src/skgo-token-cache/repos/reload-interpreters-watchers/got-reload/pkg/gotreload/gotreload.go:461), `rewritePkg` and registration generation; [SKGO boundary fixture](/Users/tyler/src/skgo-token-cache/experiments/dev-preflight-20261014/skgo/ephemeral/dev-preflight/skgo-boundary/boundary_test.go:29) | Use compiled native implementations, preserve typed SKGO calls and refresh identity, and handle host state/types rather than importing Yaegi or assuming the fixture covers them. |
| Native object loading and lifecycle | GoLoader [`ReadObjs`](/Users/tyler/src/skgo-token-cache/repos/reload-native/pkujhd-goloader-current/link/readobj.go:69), [`Load`](/Users/tyler/src/skgo-token-cache/repos/reload-native/pkujhd-goloader-current/link/ld.go:561), `addSymbolMap`, [`doInitialize`](/Users/tyler/src/skgo-token-cache/repos/reload-native/pkujhd-goloader-current/link/init.1.21.go:125); [POC publication](/Users/tyler/src/skgo-token-cache/experiments/history-native/poc/reload.go:223) | Supply matching compiler artifacts and explicit state/type/lifetime integration. The loader supplies neither call draining nor rollback of partially registered runtime state. |
| Vite event/environment lifecycle | Vite `onFileChange`, environment `hotUpdate`, [`DevEnvironment`](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/node/server/environment.ts:40), HotChannel | Use Vite directly. Installed 8.3.1 dispatches custom-environment hooks after graph invalidation; no raw-watcher bypass or `watcher.emit` patch. |
| Complete module-runner semantics | Vite [`ModuleRunner`](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/module-runner/runner.ts:39), [`ModuleEvaluator`](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/module-runner/types.ts:49), [`EvaluatedModules`](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/module-runner/evaluatedModules.ts:11), `cachedRequest`, `getModuleInformation`, [`HMR handler`](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/module-runner/hmrHandler.ts:7), payload queue and transport lifecycle | First test direct upstream library execution in goja with Go-owned evaluator/transport. If its requirements cannot fit the engine/repository constraints, make attributed Go semantic ports of the necessary algorithms with the concrete incompatibility recorded. In either case, preserve aliases, pending evaluation, cycles, edges, cache/invalidate replies and ordering; reuse source maps. |
| Browser refresh and recovery | Vite custom HMR delivery; pinned Kit `refreshAll` and `_invalidate`; [executed refresh preflight](/Users/tyler/src/skgo-token-cache/experiments/dev-preflight-20261014/skgo/ephemeral/dev-preflight/kit-refresh/README.md) | Notify after readiness; call Kit's public refresh API. Healthy refresh preserves state. Failed query-boundary recovery and CSR-disabled documents have different behavior, tested explicitly. |

Ports retain source revision/function provenance and upstream notices. Reuse public facilities directly where available. Adaptation is for a named SKGO seam, not permission to replace a known algorithm with a custom design.

## Execution sequence

### 0. Completed starting point: deletion on `dev`

**Verified source state:** PR #322 merged to `dev` as `b2f2c00483139b821072a94d22875674909e9eaf` on 2026-10-14. The old dev packages, runner, Worker, command, templates and dev qualification were removed. Production facilities remain, and development qualification is explicitly unavailable. The failed worktree and research cache remain intact. This is a completed prerequisite, not another deletion assignment or a claim that this planning session reran the deletion's runtime validation.

Replacement implementation starts from current `dev` in its own worktree and targets `dev`. Do not cherry-pick the failed string-query branch. Reconcile the actual surviving production interfaces before assigning work; its private file layout is not presumed by this plan.

### 1. Generated native execution at the real application boundary

**Mission:** A freshly generated application's ordinary Go handlers can use compiled replacement bodies while its host identity/state and Kit behavior remain correct.

**Ownership:** Native compilation/loading, development-only generated dispatch and its integration with the surviving typed production registries. This owner also owns the compatibility result and native lifetime boundary. It does not own a file watcher, CLI server, browser notification system or new compiler.

This ownership includes the private dev build view and development dependency isolation described above. The Go test driver builds/launches a separate real application host with the prepared stock Go 1.27.2 environment and symbols explicitly retained (`-ldflags=-s=false` at that pin), then drives that host's actual HTTP handler. It does not load native objects into a normally stripped `go test` executable. Preparation is automated by the tested build primitive on both darwin/arm64 and linux/amd64, including cgo/libdl prerequisites and private object-package access. Missing prerequisites fail rather than skip. Align CI's selected Go version here; root `just test` stays an ordinary Go test invocation and exercises native code through the real host on Ubuntu. Development-host-only code is isolated so the ordinary SDK/CLI build does not require the private GOROOT. Incomplete work can remain on `dev`, but step 1 is not marked passed by a Mac-only run.

**Accepted proof, in Go tests at the composed handler:**

- Given argument 7, counter initialized to 40 and helper `2*x`, the first `helper(x)+counter.Add(1)` call returns literal 55. Add 10 in the registered body: the next call returns 66. Change the helper to `3*x`: the next call returns 74. PID and initializer count remain fixed. The generated boundary must also handle an existing application struct and private package state; a new manually supplied counter argument cannot stand in for ordinary application state.
- Two generated handlers share that helper: both observe its changed calculation after a native update, including when one caller's source bytes are unchanged. Add a caller outside the native replacement boundary and assert the helper edit is classified for a complete rebuild before any partial native installation; step 5 proves that rebuild reaches every caller.
- A load returns an existing application struct; its rendered HTTP document contains the new field value. An endpoint/action body changes a real response. These use generated integration, not fixture-specific manual dispatch.
- A command using `skgo.Refresh` or `RefreshNoArg` resolves the original named query after multiple swaps. Typed errors, cookies and generated codecs still cross the real boundary correctly.
- Hold a call/serialization across publication. Its old revision stays valid through consumption; the next call reaches the new one. Repeat swaps with forced GC and verify retirement independently, including data actually backed by native mappings where copying is claimed.
- Hold a nested internal request/helper while a writer waits; it completes without reentrant-lock deadlock. An unrelated live stream does not prevent a compatible synchronous handler update.
- A compiler failure leaves the previous result and state intact; repair installs the new result. A type/global-initializer/new escaping-lifetime change is classified before an unsafe load. The test distinguishes unsupported edit from missing dispatch or loader setup.
- A body that persists a candidate-derived string/slice/error/composite into host state is either rejected for native installation before load, or its proved host-owned representation survives retirement and GC. Test genuinely candidate-backed data where that is the claimed hazard; a literal GoLoader already copied to heap cannot establish the general claim.
- In the same generated project, run the dev profile, stop it, and build/run production with cgo disabled without restoring or regenerating source. Production returns its supplied fixture value and contains no development-loader dependency.

Use the selected loader/preflight rather than repeating engine selection. The first-priority engineering task is mapping and implementing this generated bridge, including private symbols, host state/types and initializer behavior. Steps 2–3 have independent owners and may progress alongside it; integrated steps 4–6 depend on this contract passing. If a source incompatibility prevents the useful native contract, stop dependent work and bring the exact failing case and smallest proposed adaptation to the user; stop all replacement work if the overall goal is shown impossible, as requested. The deliverable does not silently become restart-only, literal-only, a different loader, or restored old code. Only a new user decision can change the goal, in which case revise steps 4–6 and their acceptance before executing that changed scope. There is no pre-authorized weaker contingent product.

**Exit condition:** The generated bridge and its load-bearing real-host tests pass with stock Go 1.27.2 on darwin/arm64 and linux/amd64, with production build isolation demonstrated. There is still no advertised development server. This is an application primitive, not product completion.

### 2. Retained project inputs, selective generation and compiler inputs

**Mission:** Each observed source change updates retained state; affected generation and native/full compilation consume the same selected bytes.

**Ownership:** Project/file state and shared analysis/generation. Adapt existing production source-view/output staging in its current owner; coordinate necessary polytype work in its own worktree. The native owner consumes compilation inputs from this boundary and retains the host's actual dependency artifacts.

**Accepted proof:**

- Watch a real temporary application, including a local replacement and embed directory. Atomic editor saves, preserved-mtime writes, populated-directory creation and rename/removal lead to fixture-defined generated/served values without request-triggered discovery. A reported event forces acquisition of fresh changed-file content; gopls's disk-cache mtime shortcut is not evidence that an observed equal-mtime save is unchanged.
- Hold generation and compiler work, save another fixture, and release the held work in either order. Only current source may publish its generated output or candidate. Diagnostic locations refer to the authored file/revision that caused them.
- A body-only edit reuses generation but compiles the new body. Struct/tag/constant/marker changes regenerate exactly the affected outputs; generated output writes do not trigger an endless self-build loop. Removed producers remove only their own artifacts.
- Compiler imports come from archives associated with the host build and current dependency identities. Editing a local dependency or adding an embed match invalidates executable eligibility even when a type projection stays the same.
- A routine edit reads the changed/affected closure rather than recapturing all authored source. Unchanged analyses remain available across edits and recover after an invalid intermediate source.

**Exit condition:** The event-to-candidate path produces real current native/full-build inputs with bounded retained state. Avoid a separate persistent Go compiler or generic revision platform. Compilation remains stock Go's responsibility.

### 3. Vite-driven development documents in Go

**Mission:** A Go handler renders live Kit pages from a Vite environment, and both SSR and frontend HMR follow Vite's real module lifecycle.

**Ownership:** Necessary adapter/client glue, upstream ModuleRunner/goja integration, private Vite routing guard and live Kit metadata. This owner does not implement application compilation or a second filesystem watcher for Vite-owned modules.

Begin with one bounded direct-use compatibility test of Vite's actual runner in a pooled goja runtime, using its custom evaluator/transport seams. Bundle the upstream dependency through the existing adapter build, with source-map interception/debug/transport timers disabled or owned in Go, external module execution rejected, and every I/O operation in Go. This is reuse of an upstream library, not authorization to write an SKGO JavaScript runner or add application I/O/timers to the engine. If direct use needs unsupported engine semantics or prohibited custom JavaScript/runtime behavior, record that exact blocker and implement the necessary algorithms in Go. Run the same alias/cycle/cache/update tests on the selected integration. Do not assume direct use works or prescribe a maintained port before this check.

**Accepted proof:** Module alias/cycle/shared-evaluation tests follow upstream cases. A changed dependency with unchanged importer bytes updates the next SSR document; deletion/recreation and changed import edges recover. Vite cache/invalidate replies and reconnect/reset behavior work. Real Kit branch/node/template/style data, Go I/O callbacks and renderer-pool reentrancy work through the handler.

**Render consistency decision:** A request holds its selected manifest/registry and exclusive ownership of its checked-out runtime. Ordered environment updates apply at runtime admission, not by polling a private file journal during requests. A render whose required module fetch crosses an invalidated environment must not emit a mixed document: discard/retry before commitment. A post-commit stream that cannot continue consistently is canceled and recovered through the update lifecycle. Already evaluated old code can finish while held; an absent historical module cannot be fetched from current Vite and passed off as old. This policy is tested with an update while a Go-backed async render and a dynamic import are held. A manifest refcount by itself does not solve this problem.

The retry applies to module preparation/rendering, not replaying a submitted command, action or native form. Retain the request's resolved Go answers/action result through render preparation. If a consistent response cannot be completed after application effects, surface the controlled failure without automatically executing those effects again. Hold an action across an environment edit and assert its fixture-owned mutation happened exactly once.

**Exit condition:** Live Go-rendered documents and Vite HMR work together without the deleted runner, old HTTP journal, old SSR refresh locks or raw watcher interception. A browser reaches Vite module/assets and the HMR socket only through the public Go origin, while documents, loads, remotes, actions and endpoints are answered by Go. Direct requests to the private Vite listener cannot invoke Node application handlers. Any new synchronization is justified by this tested consistency boundary. The production pool/renderer remains production infrastructure.

### 4. Connect ordinary `skgo dev` from startup to paint

**Mission:** One command in the example and a fresh generated application runs the single replacement end to end.

**Ownership:** Go session integration, command/template/recipe wiring, application control/readiness and narrow adapter-to-Kit readiness delivery. Steps 1–3 own their respective mechanisms; this owner connects them and owns a complete save, not an additional implementation of them.

**Accepted proof:** A Go test launches the real command, makes a save without issuing requests, observes work through its process/control boundary, and then verifies the fixture's new response. A browser scenario edits a meaningful compatible body and sees its new value with the same application PID, document identity, local counter, shallow state and unsent input. No `--native`, app-level opt-in annotation or manual native registration exists. A fresh generated app passes the same behavior, not only the repository example.

Publication is exercised under a save during generation, compilation and installation, plus a navigation during refresh. The stale result, stale diagnostic and stale browser event must not win. Structural frontend generation cannot get ahead of the application it describes. Browser success is asserted from visible values/state, not from a resolved refresh promise.

**Exit condition:** The new command is wired once through every restored development entry point. Its representative compatible Go edit performs no full application link/restart and no document reload. Further functionality in step 5 remains required before claiming complete development support.

### 5. Complete edit and recovery behavior

**Mission:** The replacement covers the complete developer surface in the matrix below, including edits that require new application state.

**Ownership:** Session installation/lifecycle and the domain owner named in each row. This is completion of the same implementation, not a new restart mode.

Build a structural candidate from selected inputs while the old host continues serving. Readiness includes its Go handler/manifest/renderer initialization. Switch the stable listener only after successful preparation. Drain finite requests with explicit bounds; terminate/cancel obsolete streams according to their lifecycle. Where an application has exclusive startup resources, drain/stop its prior host before acquiring the resource and show a startup diagnostic during that necessary gap. A failed startup can relaunch the last successful artifact when resource ownership permits; it must not pretend the new source is serving.

Exercise watcher failure/overflow, Vite restart, application crash, port conflicts and SIGINT/SIGTERM. Recover/reconcile an uncertain watch scope outside requests; report failures explicitly. Stop every process the session owns, including descendants, and leave no stale control connection or public listener. Use x/tools [`gocommand.Runner`/`Invocation`/`runCmdContext`](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/internal/gocommand/invoke.go:31) as the concrete command-ownership source, not helpers from the deleted dev package. Step 1 has already qualified native generated-host behavior on Linux and Mac; this step restores complete Ubuntu development qualification through the integrated command, not a platform skip or a restart-only substitute.

**Exit condition:** Every matrix row has its appropriate real-handler or Kit-client proof. Restored CLI help, examples, generated docs/recipes and CI describe this one system. Temporary absence of qualification from #321 is replaced by real development qualification, not skipped checks.

### 6. Reach the latency goal and qualify the result

**Mission:** Make the completed replacement fast in the real application and keep it stable through an editing session.

Use observed save-to-visible time, including watch, classification, generation, compilation, native/full installation and Kit paint. The initial engineering target is a few hundred milliseconds or better for the representative warm compatible body edit. Measure a dependency change and a structural change separately; no claim that every Go edit avoids rebuilding. Use ordinary process observations/existing logs and small Go benchmarks where useful, not a proof framework or timing ledger.

Remove measured work from the critical path: repeated dependency fences, global projection misses, all-package discovery, repeat parses and unnecessary host links. Use the existing upstream invalidation and compiler identities. The old POC's roughly 250 ms fencing floor and constant-string branch's timings are neither targets nor replacement performance evidence.

**Exit condition:** Report actual example/generated-app save-to-visible observations, preserved state, full-build behavior and repeated-edit resource behavior. The supported path cannot accumulate unloaded-code hazards or unbounded native revisions. The relevant Go suite, both browser modes, and actual visual inspection establish the result together.

## Behavior matrix

Exact statuses, headers, cookies, wire bodies and rendered HTML belong in Go tests against the real composed handler. Use independent supplied fixtures. Browser scenarios are reserved for Kit-client action or a real no-script form.

| Edit/situation | Intended behavior | Proof that distinguishes success |
|---|---|---|
| Svelte/CSS edit; imported frontend dependency edit | Vite HMR; new Go SSR sees the current graph | Client-visible fixture update with state retained where Vite promises it; literal SSR document assertion separately |
| Compatible Go remote/load/action/endpoint/helper body | Native installation; unchanged contracts/state | Literal new calculation/result, unchanged PID and initializer count; open client retains document/draft/local state |
| Signature, declared type, package/global initializer change | Relevant generation and a new application host | New typed wire/document result; observed host replacement with same public address; initializers run once in that host |
| Go import/local replacement/build-selection change | Recompute affected metadata/compiler inputs; rebuild where native closure is incompatible | Cross-package fixture result changes; no use of a stale archive or old inlined implementation |
| Imported explicit constant, tag, marker or consumed comment | Correct affected generation and executable closure | Literal generated declaration/codec and actual handler result, including the indexed explicit-constant regression case |
| New/removed/renamed route, layout, endpoint or matcher | New Kit graph plus matching Go registrations | Literal 404→200→404 and correct rendered data; Kit navigation actually reaches the new route |
| App template, error template or page options | Pinned Kit behavior in the next document/client | Actual template text and markup; `ssr=false` SPA behavior; `csr=false` no script; no forced promise of HMR without scripting |
| Changed embed bytes/membership | Invalidate the real build/registration inputs | Exact supplied content and file count, including newly added matches |
| Syntax/type error then repair | Current diagnostic; working application retained; repair visible | Authored source location plus positively working previous value, then literal repaired value; no blank-page negative assertion |
| Query error boundary or failed load then repair | Kit-supported refresh/recovery; document reload only where required | Recovered value and actual boundary state, not merely resolved `refreshAll`; state reset reported where recovery remounts/reloads |
| Batch/command/form refresh, live query, deferred load | Preserve Kit semantics and lifetime; full build for non-admitted native constructs | Real coalescing/enhanced submit/reconnect/streamed value and native form behavior; correct named-query refresh after swaps |
| Fast saves or save during any build/publication stage | Latest desired source wins | Independently ordered source fixtures; hold/release operations in adversarial order and assert actual current bytes/results |
| Concurrent render/dynamic import during environment update | Coherent document or controlled retry/stream recovery | Paired fixture values cannot mix; genuine async client recovery still displays complete content |
| Vite/host restart, watcher uncertainty, cancellation | Reconcile or diagnose; correct ownership and cleanup | Real process/listener/control observations and successful subsequent use; no orphan process or silent skipped lane |

## Validation and finish

Use one authoritative plan: this document. The semantic index and preflight packet provide source/evidence routes, not competing engineering orders. Each implementation handoff contains the mission, accepted behavior, owner and source routes above; the implementer performs its own upstream mapping. Do not prescribe a tactical file list or transmit an unverified source interpretation as fact.

An independent validation agent checks whether the selected tests are load-bearing, then runs them. Removal/rejection of native installation, generation invalidation or readiness must cause the relevant positive assertions to fail. Reuse captured expensive output rather than rerunning a full build for every assertion. No new gate command, proof directory, evidence manifest or acceptance framework.

Run `just test` with no silent skips. On `dev`, restore and run `just e2e prod` and `just e2e dev` through the ordinary entry points before promoting the completed replacement. After promotion, run the required main qualification through the release process; do not publish interim `dev` checkpoints as releases. Run the built production example and a fresh generated application, with the production cgo-free/no-runtime-Node boundary intact. Inspect the real rendered development app after substantial changes; do not hand over uninspected screenshots or infer visible correctness from a process exit code.

The work finishes when #321's deleted implementation remains absent, the single replacement covers the matrix, native updates do meaningful work through generated applications, Kit preserves healthy browser state, and measured edit latency supports the claim being made. Source provenance and test results support those behaviors; neither substitutes for running the application.
