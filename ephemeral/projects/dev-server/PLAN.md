# Build the SKGO development experience

## Goal and decision state

[GOAL.md](GOAL.md) is authoritative. Run `skgo dev`, open the app, edit Svelte or Go, and see the change quickly and automatically. Preserve healthy browser state where the change permits; report useful errors and recover after repair. Vite owns frontend development; Go owns every application endpoint and pooled goja rendering; Go development machinery prepares native patches and supervises rebuild/restart fallback. Arbitrary initialized Go state continuity is not required.

**No listener architecture has been selected.** This plan replaces the older prescribed build sequence. Coordinator ownership, renderer delivery, IPC, watcher details and caching mechanisms are candidates to test, not requirements inherited from that sequence. Existing native patching is the fast-path foundation; rebuilding is fallback. We choose an architecture from working compositions and their observed costs, then build the command. Research and continuation documents are not the deliverable.

Current worktree is based on `origin/dev` at `868c5dba9bf1670d2252996b058ecf19bca659ca`, on `codex/dev-architecture-experiments`. Retained qualification is checkpointed at `ce877ff`. Inspect current Git state before resuming; these identifiers are provenance, not reset targets.

## What is already demonstrated

| Capability | Evidence and boundary |
|---|---|
| Native patching through generated Go handlers and pooled SSR | The real generated HostView contract exercises remotes, loads, endpoints, actions, refresh, deferred consumption, cookies, panic mapping and internal fetch. The new retained path preserves the existing classifier, lowering, compiler and artifact identity checks. |
| Fast repeated preparation with whole-app checks | Same-app full preparation measured832–880ms; retained preparation measured150–168ms warm plus about4ms installation. First retained run548ms. Independent caller, dependency and real archive-byte mismatch controls reject unsafe candidates. |
| Retained preparation joined to a real browser | Actual source4101→5202→6303, malformed source retaining6303, repair7407. Warm preparation161–177ms plus about4ms installation; first cold793ms. Draft/count/hydration/document/PID preserved. This fixture uses production Kit assets and a Vite event connection; it is not the complete frontend development loop or save-to-paint timing. |
| Vite-public development composition | Actual Svelte HMR and Go SSR updates; two isolated instances; actual native edits and a Go child crash recovered on a new private port while preserving the observed Vite connection and healthy browser state. This older fixture used full preparation, separately from the retained timing proof. |
| Coordinator proxy primitive | Standard Go ReverseProxy preserves a synthetic upgraded stream across app replacement. Actual coordinator-fronted Kit/browser behavior is the next experiment, not established by that synthetic test. |

Sources: [retained qualification](../../experiments/20261015-retained-qualification/REPORT.md), [independent validation](../../experiments/20261015-retained-validation/REPORT.md), [browser observation](../../experiments/20261015-retained-browser/OBSERVATIONS.md), [Vite composition](../../experiments/20261015-vite-listener/REPORT.md), [listener comparison](../../experiments/20261015-listener-ownership/README.md).

The historical137–138ms root-only result was a promising narrower experiment. Current whole-app and browser measurements support the speedup without borrowing that narrower figure or claiming an end-to-end bound.

## Current experiments: remove the uncertainties that decide architecture

These missions run in parallel under separate ownership. Start discovery at `/Users/tyler/src/skgo-token-cache/index/README.md`, follow its current leaves, then inspect cited source. Kit's installed pin is the specification. Builders map it themselves; a dispatcher's preferred mechanism is not evidence.

### Listener ownership and lifecycle

**Capability:** A real Kit browser uses Go application responses through a stable public address, while tooling and app processes change independently.

**Owner:** Coordinator composition experiment. Compare against the existing Vite-first fixture; do not replace it with another synthetic transport test.

**Accepted proof:** Supplied literal values in fresh Go SSR and hydrated pages; correct public HMR URL without private-port fallback; coherent Kit origin after public-port collision; multiple isolated instances; actual Go child replacement/crash; explicit Vite failure and reconnection behavior. Observe browser drafts/document identity and useful unavailable/recovery behavior. Preserve failures as well as successes.

**Decision:** Compare coordinator-owned target updates and public error handling against Vite-owned listener/HMR simplicity. Count actual lifecycle and routing responsibilities. The replaceable Go application owning the public socket remains a candidate if evidence supports its restart behavior. A coordinator need not be another executable. Vite does not need the private Go address for the already tested artifact/goja or ModuleRunner rendering paths when a coordinator routes application traffic directly.

### Current captured inputs through native publication

**Capability:** Fast preparation consumes one coherent source revision, and only a still-current revision can install or publish diagnostics/browser events.

**Owner:** Retained capture/publication integration. Preserve exercised whole-app, reverse-caller, dependency and HostView admission.

**Accepted proof:** Controlled overlapping edits hold obsolete work across a newer desired revision; independent literals show no stale installation or notification. Multi-file changes yield a coherent selected capture or an explicit rejection, followed by working repair. The current browser watcher's reread-before-install has a race; the separately passing Project stale-generation test is not joined native-publication proof. After that boundary works, demonstrate automatic rebuild/restart for a rejected structural/dependency change, rather than merely returning CompleteBuild.

### Structural Kit metadata

**Capability:** Route and remote additions, changes and removals keep Go routing/registries and Kit's client node identities coherent.

**Owner:** Structural metadata investigation and bounded running prototype.

**Accepted proof:** Newly added route works by direct Go response and actual client navigation; existing routes keep the correct node identity; removal behaves correctly; remote membership/types follow Kit's constraints. Map Kit's own lifecycle from pinned source before selecting a synchronization mechanism. Existing evidence of Go404 plus stale client node imports establishes the problem; a renderer-only rebuild or reload notification does not solve it.

## Architecture selection and implementation

Bring the results together in one comparison with topology diagrams, measured intervals, correctness/recovery observations and concrete remaining costs. Recommend the composition supported by those results and present the architecture choice to the user. Continue independent work while a choice is pending; do not invent a permission gate for already-authorized experiments or fixes.

After selection, integrate the demonstrated pieces into the actual Go-owned development command and Vite-required adapter glue. Carry the source/currentness boundary, origin initialization, metadata publication and process ownership into that implementation. Preserve production generation/build isolation. Use the existing generator, typed registries and pooled renderer rather than reimplementing Kit or its frontend graph.

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
