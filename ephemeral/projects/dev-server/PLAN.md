# Build a best-in-class Go development server

## Goal, priorities and methodology

Build a best-in-class development server for web applications written in Go. Aim for effectively immediate save-to-visible updates: an experience a developer would want to show to colleagues. For ordinary compatible Go implementation edits, the first application milestone is a few hundred milliseconds or better; establish the concrete target with the user from the baseline comparison and continue toward the practical latency floor. Frontend edits retain Vite's responsive HMR experience. Large structural edits may need a different path; they must recover automatically and keep development usable. Success is a fast, reliable, maintainable application, not a quantity of copied software.

- **Optimize the real edit experience.** Saves start useful work while the browser is idle. Unchanged work stays warm, rapid saves converge on the latest result, and compatible updates preserve the document and unsent input.
- **Choose the strongest applicable approach.** Vite is the primary model for the dev-server experience. gopls contributes Go-analysis expertise; Kit defines application semantics; Go's compiler/build tooling produces native code. These are starting points, not exclusive territories. Select by behavior, latency, correctness and maintenance fit. Translation effort or sharing Go's implementation language is not a reason to accept a weaker approach.
- **Borrow to improve work SKGO already performs.** Use established algorithms and patterns wherever applicable, through direct APIs or idiomatic Go semantic ports that preserve the relevant behavior and invariants. Upstream features do not create additional SKGO responsibilities merely by existing.
- **Always choose the next most impactful useful change.** Establish the current latency breakdown first. Select the largest remaining gain, including the prerequisites it actually needs, rather than proceeding through an adoption inventory. Explain when a foundational change enables a larger gain instead of delivering that gain itself.
- **Keep the sequence and complexity under control.** After each usable delivery, inspect the real result and maintenance cost with the user before expanding into another substantial assignment. Remove superseded strategies. Narrow or change an approach that adds complexity without the expected useful behavior.

This is the sole implementation plan. The [research index](/Users/tyler/src/skgo-token-cache/index/README.md) and its [adoption map](/Users/tyler/src/skgo-token-cache/index/adoption.md) provide source evidence and candidate mechanisms. This plan governs scope and sequence. Read the research's broader adoption directives as historical context, not execution instructions or extra assignments. Builders independently map the relevant pinned sources and choose the strongest implementation; the missions below specify capability, accepted behavior and ownership, not prescribed designs.

## Starting point and strategy

The source baseline is clean SKGO `53179fcdf0576f67e9dc13ad1df402cb9d18c863`, materialized at `repos/skgo-adoption-baseline-53179fc` in the token cache. It **already rebuilds in the background without HTTP traffic**, through whole-tree polling with default 150ms polling and 100ms quiet intervals. Requests additionally scan to catch edits before the poll notices them. Event-driven observation replaces that delay and repeated work; it does not introduce background rebuilding for the first time.

**First assignment: establish the real-app latency breakdown and select the first improvement with the user.** Use the existing example and ordinary logs/browser observation to locate save-to-observation, generation, native compilation/linking, process/renderer readiness and visible-update costs. Compare a frontend edit, a compatible Go body edit and a declaration edit against fixed expected values. Reuse captured output; no benchmark framework or new runner is needed. Historical measurements identify seconds of generation and linking as strong leads, but do not establish the current whole-application latency. Start implementation from this comparison, not an assumed ranking.

The intended steady-state path is:

`observe changed bytes → classify impact → generate only affected contracts → compile necessary code → publish compatible update → refresh affected values → paint`

For an ordinary compatible Go body edit, the target is **no generation, no repeated whole-project discovery, no full application link, no application restart and no document reload**. Selective generation and retained inputs remove analysis overhead; they are not expected to erase the link/startup floor. The strategic route through that floor is integrating native replacement into the real application, using the [native experiments](/Users/tyler/src/skgo-token-cache/index/native-engines.md), [compiler-input research](/Users/tyler/src/skgo-token-cache/index/compiler-retention.md) and [publication/lifetime evidence](/Users/tyler/src/skgo-token-cache/index/publication-lifetime.md) as candidates. This work competes for priority immediately; it does not wait for an adoption checklist to finish.

The existing [executed preflights](/Users/tyler/src/skgo-token-cache/index/executed-preflight.md) provide a manually bridged SKGO handler and native fixture results, not generated-app integration or whole-application timing. Extend the strongest applicable route to a real developer edit, preserving serving state and correct handler behavior. Keep the ordinary rebuild/restart path for incompatible changes. If a candidate cannot satisfy the actual application/runtime boundary, report that concrete limit and reconsider the route with the user; do not substitute an endless sequence of cache ports for the speed goal. Native lifetime safety is an application/runtime question, not something gopls analysis ownership or Vite HMR establishes by analogy.

The demonstrated native loader uses a private adapted Go toolchain, version-specific runtime internals and cgo/JIT facilities on darwin/arm64. These are experimental conditions, not accepted installation requirements for SKGO developers. Before substantial native integration, discuss the candidate's toolchain/setup burden, platform coverage and Go-version maintenance cost alongside the expected latency gain, and agree the development support envelope with the user. Preserve the ordinary supported development path and production's existing constraints; the builder cannot silently turn the prototype's requirements into product policy.

## Working missions

These missions describe the parts of the edit path we need to improve. Their numbers identify them; they do not prescribe execution order. Scope the selected mission to the next useful delivery. Dependencies are the behavior that delivery actually requires, not wholesale completion of another mission.

### 1. Saves promptly start useful work

**Capability.** Relevant saves, creates, deletes and renames drive the existing build path through events. Requests consume session state instead of discovering the filesystem. Initial startup still discovers the project once; generated outputs do not create rebuild loops.

**Accepted behavior.** A real filesystem event starts work while no HTTP requests arrive, and the resulting handler serves the supplied changed value. Compare observation/build-start delay and recurring scan work against the measured baseline. Existing selected module, route, control and embed inputs remain covered, including outside-root dependencies and membership changes. Partial saves, deletion and shutdown remain usable.

**Ownership and research.** Own Go input observation and supervisor handoff, coordinating with Vite's existing frontend observation. Use [development research](/Users/tyler/src/skgo-token-cache/index/adoption-development.md) D1–D2 and D15 to compare candidate mechanisms. Vite's responsive event behavior is the model; gopls's watcher is a candidate. Choose timing for this dev workload rather than inheriting editor-diagnostics quiet intervals.

### 2. Unchanged analysis and generation stay warm

**Capability.** Changed inputs update required declarations, codecs, stubs and registrations while unaffected producers avoid loading, lowering and emitting. Reuse unchanged discovery, parsing and analysis within the existing owners. Removing declarations retires their generated contributions without deleting authored files or other producers' output.

**Accepted behavior.** Supplied body, declaration and membership edits produce the correct executable values and generated bytes; consumed constants, tags and comments remain real inputs. A compatible body-only edit omits generation work. Show which repeated work disappeared and its effect on the real edit time. Error/repair preserves unrelated reusable work and old results still held by callers.

**Ownership and research.** Own generation and the necessary retained analysis. Loader improvements belong in polytype/goload and reach SKGO through its normal dependency update. Start from [generation/analysis research](/Users/tyler/src/skgo-token-cache/index/adoption-generation.md) and [selective-generation evidence](/Users/tyler/src/skgo-token-cache/index/selective-generation.md); choose the needed mechanisms rather than copying an entire analysis subsystem.

### 3. Rapid saves publish the current result

**Capability.** Generation, compilation and any required checks agree on the source they consume. Superseded work does not publish files, a serving application, stale diagnostics or readiness after a newer edit takes ownership. A current follow-up completes naturally.

**Accepted behavior.** Hold old work while a supplied second edit arrives; only the admissible result becomes visible. Correct values survive rapid saves, compilation failure and repair. Demonstrate that cancellation, coalescing or sharing removes wasted work without delaying a valid update for unrelated diagnostics.

**Ownership and research.** Own current-result admission across the existing supervisor, Worker/source materialization and publication seams. Consult [development](/Users/tyler/src/skgo-token-cache/index/adoption-development.md), [generation](/Users/tyler/src/skgo-token-cache/index/adoption-generation.md) and [project-revision](/Users/tyler/src/skgo-token-cache/index/project-revisions.md) research. Adapt command/check behavior only where this delivery requires it; no application-wide command-runner or analyzer rewrite is a prerequisite.

### 4. Compatible Go edits avoid relinking and restarting

**Capability.** A real Go handler implementation edit changes the running application's response while its process, application state and SSR runtimes survive. Compile only the necessary replacement code. Establish supported edit boundaries in the real application; incompatible changes automatically use the ordinary rebuild path.

**Accepted behavior.** Through ordinary development, a fixed handler value changes in the browser with the same application PID and initialized state, without a full application link or restart. Failed edits retain the working version and repair recovers. Measure the whole save-to-visible path, including compilation and publication. Active calls and returned values remain valid across replacement within the demonstrated boundary.

**Ownership and research.** Own real application integration and compatible publication, using [native-engine](/Users/tyler/src/skgo-token-cache/index/native-engines.md), [compiler-retention](/Users/tyler/src/skgo-token-cache/index/compiler-retention.md), [lifetime](/Users/tyler/src/skgo-token-cache/index/publication-lifetime.md) and [executed-preflight](/Users/tyler/src/skgo-token-cache/index/executed-preflight.md) evidence. The mechanism is an implementation choice requiring application proof, not a prescribed new loader platform. Broaden compatibility only after a useful first boundary works.

### 5. Development rendering follows Vite's updates

**Capability.** Frontend edits and Vite lifecycle changes update Go-owned development rendering using Vite's actual identities, transforms and update decisions. Retain unaffected rendering work. Requests and nested requests observe compatible route/configuration/module state.

**Accepted behavior.** The real example renders the supplied changed component and route values through Go, including membership changes, error/repair and Vite restart. In-flight requests remain usable and updates do not mix incompatible state. Show the effect on readiness/rendering time; remove custom update handling as its replacement becomes useful.

**Ownership and research.** Own the existing adapter/Go renderer integration. Consult [development](/Users/tyler/src/skgo-token-cache/index/adoption-development.md) and [runtime research](/Users/tyler/src/skgo-token-cache/index/adoption-runtime-tooling.md), using actual Vite APIs or the strongest applicable semantic ports. Direct ModuleRunner compatibility and concurrent old/new rendering remain implementation questions; neither is a prerequisite for every smaller improvement.

### 6. Backend readiness updates the page without a document reload

**Capability.** Compatible backend changes refresh affected Kit loads/remotes while preserving document identity, component state and unsent input. Vite continues to own frontend HMR. Structural changes and pages without scripting retain usable ordinary responses.

**Accepted behavior.** Kit's client shows fixed new load/query values while a supplied draft and initialized local counter survive. Navigation, backend failure/repair and incompatible changes recover correctly. Include browser refresh and paint in the measured edit latency.

**Ownership and research.** Own readiness-to-client integration through the existing successful-build seam, with current-result admission where required. Consult [Vite/Kit refresh evidence](/Users/tyler/src/skgo-token-cache/index/vite-kit-refresh.md), [executed preflights](/Users/tyler/src/skgo-token-cache/index/executed-preflight.md) and development mappings. Kit remains the authority for refresh semantics; no second query cache or navigation system is needed.

## Source and application boundaries

Read [source identities](/Users/tyler/src/skgo-token-cache/collection/adoption-scope.md): readable Vite 8.3.4 differs from installed Core 1.0.0 containing Vite 8.3.1; gopls source is x/tools v0.52.0; Kit is 3.0.0. Historical `repos/skgo-current` and the interrupted implementation are not the clean baseline. Porting a newer algorithm does not imply changing frontend pins.

Kit is the specification for its HTTP/application behavior. Read the installed pin and [current Kit guidance](../../sveltekit-current/SKILL.md) before changing mirrored behavior. VitePlus owns frontend commands. SKGO-owned implementation stays Go; necessary adapter/client integration stays in its existing JavaScript boundary. SSR stays in pooled goja, application I/O stays in Go, and production stays one binary without request-time Node.

Use direct public functionality when it serves the selected behavior; otherwise make focused semantic ports with upstream notices and source function/commit beside the code. Adapt product policies to SKGO's workload without discarding the relevant invariants. Keep existing public Go loading APIs and polytype backends; improve their owners instead of creating a private second loader.

The wider research also covers MCP, native SDK packaging, add-on staging and diagnostics. It remains available as evidence, but those are not independent assignments in this dev-server plan. Command/check or diagnostic improvements enter only when they materially serve a selected edit-path mission. No generic filesystem, scheduler, revision, compiler, loader or cache platform is prescribed.

## Delivery and ordinary validation

Before implementation, obtain the baseline comparison above. After each selected delivery, compare the same real-app edit and discuss the result with the user: where latency fell, what work disappeared, what custom code was removed and what maintenance cost remains. Keep the goal ambitious; a few hundred milliseconds removed from a seconds-long loop is an intermediate gain, not completion. Choose the next largest opportunity from the observed path and its necessary dependencies. Keep development usable throughout.

Handoffs provide mission, accepted behavior, ownership and candidate source routes. Builders map the relevant sources themselves and explain their selection. An independent validating agent checks that relevant tests would fail if the capability were broken or removed, then runs them as repository instructions require. This strategic plan does not need every tactical step or hypothetical failure mode spelled out in advance.

Use literal fixtures and Go tests at the real handler for HTTP/generated-byte behavior. Kit-client scenarios cover behavior that needs its client or a real no-script form. Use existing `just test`/`just e2e` recipes appropriately, with no silent skips and both E2E modes on main when integrating. Reuse expensive output, inspect the actual example after substantial changes, and distinguish source feasibility, fixture results and demonstrated application behavior. No new gate runner, proof directory, ledger, evidence manifest or expanded verification framework is part of this work.
