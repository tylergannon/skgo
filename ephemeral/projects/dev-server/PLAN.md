# Build the SKGO development experience

Current consolidated architecture and honest evidence appraisal: [DESIGN.md](DESIGN.md).

## Scope correction from the user — 2026-10-15

The authorized task is to prove the concept through subagents and present architecture evidence, measurements, diagrams and unresolved questions for selection. The user permitted choosing the obvious listener recommendation; that did NOT authorize implementing the product. The parent incorrectly expanded the task into product implementation and incorrectly described that expansion below as authorization.

Further implementation and new verification runs are stopped. Already-running agents/checks are allowed to finish and preserve their results; do not cancel them. Existing code and evidence are preserved unmerged. Resume only the bounded proof-of-concept assessment unless the user explicitly authorizes implementation. The historical implementation missions below record work undertaken, not current instructions or user requirements. Later coordinator changes are incompletely qualified; do not claim the final combined implementation passes full regression.


## Goal and decision state

[GOAL.md](GOAL.md) is authoritative. Run `skgo dev`, open the app, edit Svelte or Go, and see the change quickly and automatically. Preserve healthy browser state where the change permits; report useful errors and recover after repair. Vite owns frontend development; Go owns every application endpoint and pooled goja rendering; Go development machinery prepares native patches and supervises rebuild/restart fallback. Arbitrary initialized Go state continuity is not required.

**Selected: the `skgo dev` Go coordinator owns the public listener and proxies application traffic to the replaceable Go host and frontend tooling/HMR to Vite.** The user authorized the recommended choice after the real coordinator experiment. The coordinator binds before initializing Kit origin and owns process readiness, target updates and unavailable responses. Renderer delivery, IPC and watcher details remain implementation choices informed by the experiments, not inherited requirements. Existing native patching is the fast-path foundation; rebuilding is fallback. Research and continuation documents are not the deliverable.

Current worktree is based on `origin/dev` at `868c5dba9bf1670d2252996b058ecf19bca659ca`, on `codex/dev-architecture-experiments`. Retained qualification is checkpointed at `ce877ff`. Inspect current Git state before resuming; these identifiers are provenance, not reset targets.

## What is already demonstrated

| Capability | Evidence and boundary |
|---|---|
| Native patching through generated Go handlers and pooled SSR | The real generated HostView contract exercises remotes, loads, endpoints, actions, refresh, deferred consumption, cookies, panic mapping and internal fetch. The new retained path preserves the existing classifier, lowering, compiler and artifact identity checks. |
| Fast repeated preparation with whole-app checks | Same-app full preparation measured832–880ms; retained preparation measured150–168ms warm plus about4ms installation. First retained run548ms. Independent caller, dependency and real archive-byte mismatch controls reject unsafe candidates. |
| Retained preparation joined to a real browser | Actual source4101→5202→6303, malformed source retaining6303, repair7407. Warm preparation161–177ms plus about4ms installation; first cold793ms. Draft/count/hydration/document/PID preserved. This fixture uses production Kit assets and a Vite event connection; it is not the complete frontend development loop or save-to-paint timing. |
| Vite-public development composition | Actual Svelte HMR and Go SSR updates; two isolated instances; actual native edits and a Go child crash recovered on a new private port while preserving the observed Vite connection and healthy browser state. This older fixture used full preparation, separately from the retained timing proof. |
| Coordinator-public Kit composition | Two actual instances, collision-resolved public origin, real native edits, child crash recovery and Svelte HMR now work through a Go coordinator. Go crash preserved browser state and HMR connection; Vite outage retained app HTTP, but Vite recovery reloaded the document and lost its draft. Renderer reinitialization after Vite restart was explicitly invoked in the harness. |

Sources: [retained qualification](../../experiments/20261015-retained-qualification/REPORT.md), [independent validation](../../experiments/20261015-retained-validation/REPORT.md), [browser observation](../../experiments/20261015-retained-browser/OBSERVATIONS.md), [Vite composition](../../experiments/20261015-vite-listener/REPORT.md), [listener comparison](../../experiments/20261015-listener-ownership/README.md).

Coordinator browser observations: [independent browser and HTTP controls](../../experiments/20261015-coordinator-integration/browser/OBSERVATIONS.md). The comparison used the older full compilation fixture; its timing does not measure the retained path.

The historical137–138ms root-only result was a promising narrower experiment. Current whole-app and browser measurements support the speedup without borrowing that narrower figure or claiming an end-to-end bound.

## Current integration findings

The actual command now serves the real example with clean hydration, public Vite HMR, Go-only native updates and automatic Kit query/load refresh. Actual edits preserved the Go PID, document and seeded draft; malformed Go showed authored diagnostics and retained last-good output; repair dismissed the overlay. Svelte HMR changed the browser and fresh Go SSR on the same host. The actual route/remote addition rendered7719 through Go and Kit client navigation; existing Item42 remained correct after the graph shift, and removal returned404.

The maintained real-session Go test exercises two instances, native edits, structural fallback, Go crash and Vite restart/republication. Independent private mutation kept the old app target after replacement and failed the new literal assertion. Native tests cross six compatible edits, replacement cleanup and embedded-resource/test-file admission. No overall completion claim follows from these individual checks.

Remaining concrete checks: verify the new metadata admission fix eliminates transient mixed generated Params/manifest failures; verify development prerender behavior on About; qualify static assets and configured environment values; run occupied-port/public-origin and combined-change tests, then the normal Go and production browser suites. Go prerender handler/SSR tests already pass, including production recorded-only guards. See [continuation](CONTINUE.md) and [actual structural browser observations](../../experiments/20261015-product-structure/OBSERVATIONS.md).

## Remaining implementation missions

Implement the selected composition using the qualified pieces; remaining integration work runs in parallel under separate ownership. Start discovery at `/Users/tyler/src/skgo-token-cache/index/README.md`, follow its current leaves, then inspect cited source. Kit's installed pin is the specification. Builders map it themselves; a dispatcher's preferred mechanism is not evidence.

### Listener ownership and lifecycle

**Capability:** A real Kit browser uses Go application responses through a stable public address, while tooling and app processes change independently.

**Owner:** CLI/coordinator implementation. Reuse the completed composition experiment; implement the real command, private process lifecycle and routing with meaningful tests.

**Accepted proof:** Supplied literal values in fresh Go SSR and hydrated pages; correct public HMR URL without private-port fallback; coherent Kit origin after public-port collision; multiple isolated instances; actual Go child replacement/crash; explicit Vite failure and reconnection behavior. Observe browser drafts/document identity and useful unavailable/recovery behavior. Preserve failures as well as successes.

**Decision resolved:** Use coordinator-owned targeting and public error handling. The actual experiment demonstrated origin initialization, both instances, Go crash recovery, Vite outage and natural recovery, and subsequent Svelte HMR. Integrate this into the command with automatic renderer-tooling reinitialization after Vite restart. The coordinator is the existing CLI/session process, not another required executable; Vite needs the public origin but not the private Go target.

### Current captured inputs through native publication

**Capability:** Fast preparation consumes one coherent source revision, and only a still-current revision can install or publish diagnostics/browser events.

**Owner:** Native host and retained compiler integration, coordinated with the session owner at ProjectBuild.Commit. Preserve exercised whole-app, reverse-caller, dependency and HostView admission. The product must build and serve the real generated app, not a hardcoded fixture.

**Accepted proof:** Controlled overlapping edits hold obsolete work across a newer desired revision; independent literals show no stale installation or notification. Multi-file changes yield a coherent selected capture or an explicit rejection, followed by working repair. The new ProjectBuild experiment and independent mutation control now establish currentness through real native installation and actual Vite frames. Removing the revision guard admitted the obsolete callback and failed the test. Structural initializer fallback rebuilt a new HostView in2.36s and resumed native edits. Carry this working boundary into the command; Observe batching still needs a deliberate filesystem capture policy. See [publication](../../experiments/20261015-retained-publication/REPORT.md) and [independent validation](../../experiments/20261015-publication-validation/REPORT.md).

### Structural Kit metadata

**Capability:** Route and remote additions, changes and removals keep Go routing/registries and Kit's client node identities coherent.

**Owner:** Vite adapter and renderer integration. Prove the live metadata seam, then use it in the real command; the frontend remains Kit/Vite-owned.

**Accepted proof:** Newly added route works by direct Go response and actual client navigation; existing routes keep the correct node identity; removal behaves correctly; remote membership/types follow Kit's constraints. Map Kit's own lifecycle from pinned source before selecting a synchronization mechanism. Existing evidence of Go404 plus stale client node imports establishes the problem; a renderer-only rebuild or reload notification does not solve it.

## Architecture selection and implementation

The selected composition is coordinator → Go application / Vite. Build it into the actual Go-owned development command and Vite-required adapter glue. Carry the demonstrated source/currentness boundary, origin initialization and process ownership into that implementation. Finish live route/remote metadata publication and automatic renderer reinitialization after Vite restart. Preserve production generation/build isolation. Use the existing generator, typed registries and pooled renderer rather than reimplementing Kit or its frontend graph.

The full-build route addition/removal experiment establishes a fallback boundary, including real Kit client navigation; it does not establish live dev graph identity. Its raw paired manifests were removed during cleanup and no account-route node-index equivalence is claimed. The live-metadata mission must preserve exact artifacts and show the actual shared graph before claiming this gap closed.

A representative automatic loop must cover:

- Svelte edits through Vite and compatible Go edits through native patching, including simultaneous edits.
- Fresh SSR plus hydrated queries/loads/navigation; healthy browser state where refresh permits.
- Compile/render errors with useful authored diagnostics, last-good service and recovery.
- Structural/dependency changes through actual rebuilding and replacement; coherent route/remote metadata.
- Multiple instances, port collisions, app crashes, Vite loss/reconnection, owned shutdown and bounded resources across repeated edits.

Measure save-to-visible latency separately from preparation and installation. Record cold and warm samples and the actual edit class. A single successful fixture does not establish universal application, platform or lifetime coverage.

## Verification and completion

Go tests exercise the real handler with independent fixture literals. Kit-client scenarios cover what only its client can prove. Independent validation checks assertions fail when the relevant behavior is broken, then runs them. Open the actual app and inspect what a visitor sees. Use the minimum expensive runs needed, preserve their output, and never count skipped prerequisites or cached live-server results as passing evidence.

Use Go for SKGO-owned tooling and VitePlus for frontend tools. Application I/O and SSR stay in Go, SSR uses the existing goja pool, and production remains one cgo-free binary with Node needed only for build tooling. No Node application/SSR sidecar, custom JS engine or Vite graph port.

Completion means the real command delivers the goal, supported by meaningful real-handler and browser checks and inspection of the example. `just test` and `just e2e prod` are relevant facts, not standalone verdicts. Checkpoint concrete work and keep the token cache/index and [continuation](CONTINUE.md) aligned with new evidence; do not stop merely because another report has been written.
