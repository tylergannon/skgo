# Ratify the SKGO development architecture

[GOAL.md](GOAL.md) defines the desired experience. [DESIGN.md](DESIGN.md) holds the design and evidence appraisal. The user's latest instruction is explicit: **land a valid architecture that smaller models can execute; do not implement the product.**

## Scope and selected foundation

Run `skgo dev`, open the app, edit Go or Svelte, and see changes automatically and quickly. Preserve healthy browser state where the edit permits; expose useful errors and recover. Vite owns frontend development. Go owns every application endpoint and pooled goja rendering. Native replacement is the ordinary fast path; rebuild is a fallback. Arbitrary initialized Go state preservation is not required.

The coordinator-owned public listener is selected on the existing two-instance, collision, child-replacement and Vite-failure evidence. It binds before Kit origin initialization and learns actual private targets through readiness. Vite needs the public origin, not the private Go target in this composition. Do not reopen this choice without contrary evidence.

The worktree is `codex/dev-architecture-experiments`, based on `dev` at `868c5dba9bf1670d2252996b058ecf19bca659ca`. Existing product edits exceeded the original POC scope. Preserve them unmerged as experimental inputs; this plan does not authorize finishing, refactoring or qualifying that implementation. Historical implementation missions were removed from the active plan because a warning above contradictory instructions was insufficient.

## Remaining architecture experiments

### Publication during overlapping edits — Sol 6.1

**Developer capability:** compatible Go changes refresh the live app; structural frontend/Go changes never silently resolve a route or remote to the wrong identity, and repair recovers predictably.

**Ownership:** the private overlap experiment under `ephemeral/experiments/20261015-overlap-answer/`. Product source remains frozen; small isolated experimental variants are allowed.

**Question:** what publication policy is sufficient when Vite's client graph can advance independently of Go preparation, renderer publication and browser notification?

**Accepted proof:** controlled obsolete work held across a demonstrably observed newer revision; actual Go values, Vite frames and Kit browser behavior; combined frontend/Go changes; route/remote membership changes affecting existing client identities. Record harmless temporary old/new values separately from stale publication, wrong node identity and failed recovery. A material failure requires one bounded proposed-policy experiment and a control showing the assertion detects the failure. Preserve exact ordering and paired artifacts. Do not invent arbitrary-filesystem atomicity as a requirement.

**Output:** an evidence-backed publication policy, its user-visible transition behavior, failure/recovery boundaries, and invariants an implementer can follow without further architecture research.

### Actual visible latency and state retention — Sol 6.1

**Developer capability:** ordinary saves produce visibly updated data or components quickly enough to improve on the reported six-to-ten-second rebuild loop.

**Ownership:** private copied app, measurement tools and artifacts under `ephemeral/experiments/20261015-visible-latency/`.

**Question:** what does a developer actually wait for, beyond compiler preparation, and which state survives each edit class?

**Accepted proof:** a bounded set of cold/warm Go, Svelte and structural-fallback saves; independent expected values; pre-save observations and timestamped old/new DOM bounds; clock and polling error; Go PID/document/draft/count identity. Preserve contaminated, late-observer and failed attempts separately. Report sample counts, range and median, not a manufactured p95 or paint claim. Serialize expensive work against the overlap experiment. Explain any material delay or state reset using pinned source or a narrow comparison; do not optimize the product.

**Output:** measured save-to-DOM intervals, a stage-cost explanation where evidenced, and explicit state-preservation promises and limitations.

### Independent validation — Luna

**Ownership:** independent assessment under `ephemeral/experiments/20261015-open-questions-validation/`; read-only access to other agents' artifacts.

**Accepted proof:** independently cross-check literals, timestamps, ordering, source versions and negative controls. A newer source write is not proof the observer admitted that revision; native installation is not proof the browser updated; a rendered page is not proof hydration succeeded. Identify any result that cannot support the proposed architecture claim.

## Bounded endpoint

Finish when the two experiment lanes and their independent checks allow DESIGN to state:

1. The selected process topology, ownership and address-discovery contracts.
2. A demonstrated policy for compatible, structural, overlapping, obsolete and failed updates, including client metadata and ready-event ordering.
3. Actual measured visible latency and honest state-preservation behavior for the bounded edit classes.
4. Implementer missions with clear ownership, inputs/outputs, failure semantics, pinned-source references and accepted proof. Small-model implementers should not have to invent the missing architecture.
5. A short separation of unresolved architecture blockers from later implementation qualification and optional optimization.

If an experiment disproves a policy, test the smallest plausible alternative sufficient to answer that question. If no bounded alternative works, report the blocker and concrete evidence; do not silently move the unsolved design into an implementation task. This is not an open-ended benchmark campaign, full regression run, release or production implementation.

## Evidence and working rules

Start retrieval at `/Users/tyler/src/skgo-token-cache/index/README.md`, then currentness/structure, retained-preparation and product-observation routes. All subagents use the same cache and semantic index. Cache material is evidence, not authority over GOAL or the current user instruction.

Kit's installed pin is the specification; read `ephemeral/sveltekit-current/SKILL.md` and verify versions. Use Go for SKGO-owned experiments and `vp` for frontend tools. Keep application I/O and pooled SSR in Go, no cgo or Node SSR sidecar. Share missions, not prescribed implementation methods. Keep private ownership boundaries and do not revert others' edits. Do not cancel ongoing agents.

The parent synthesizes evidence and resolves design decisions; subagents run experiments. Answer user questions literally before taking action. Update the design, continuation and actionable worklog as evidence changes; avoid counting an additional document as experimental progress.
