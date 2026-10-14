# Adversarial review: dev-server plan alignment, round 01

## Target

The whole of `ephemeral/projects/dev-server/PLAN.md` (235 lines) as it stands in the working tree, including the uncommitted TL;DR edits on top of `75c74ad`. I reviewed it against `CLAUDE.md` and the human requirements the caller restated.

The human's questions for this round are: does the plan reflect the goal, priorities and methodology they gave? Are the big alignment questions answered? Is it aimed at the right goal, and is the strategy in place, with tactics mostly left for later?

## Scope note

The caller says the human "accept[s] that the plan may not show every implementation step and do[es] not seek an expanded functionality or failure-mode checklist." I treated that as the human's requirement about how high-level the plan should be: strategy first, tactics later. I did not treat it as a limit on what this review may find. I ignored no narrowing. The review judges strategy and alignment, and it does not ask the plan to add detail.

A parallel review session's files in `ephemeral/reviews/dev-server-alignment-round-01/` were not read or relied on.

## Evidence inspected

- `CLAUDE.md`: mirroring rule, "Delegate missions, not methods" (sprint documents are mission/acceptance/ownership, with no prescribed designs), "Build the software", and the rule that no command means done.
- `ephemeral/projects/dev-server/PLAN.md`, all lines, plus `git diff` of the uncommitted change.
- Research entry `/Users/tyler/src/skgo-token-cache/index/README.md`, and from that index: `adoption.md`, `adoption-development.md` (head), `executed-preflight.md`, `landed-state.md`, `native-engines.md`, `compiler-retention.md`.
- `/Users/tyler/src/skgo-token-cache/collection/history-inventory.md`, which records the human's earlier latency complaints and the earlier ranked gain estimates.
- The baseline dev loop at `repos/skgo-adoption-baseline-53179fc/internal/dev/server.go` (loop, `awaitChange`, `build`, `start`, `ServeHTTP`, `noticeEdit`, Poll/Quiet defaults) and `internal/dev/watch.go` (`fingerprintPathsScan`).
- The prior round `ephemeral/reviews/20261014-dev-server-adoption-round-01.md`, which looked at source accuracy, not alignment.

## What the plan gets right

These are recorded so a later round does not reopen them.

- **Goal statement.** It names "best-in-class development server for web applications written in Go" (`PLAN.md:5`). It says success is a fast, reliable, maintainable real application, "not a quantity of copied software".
- **Vite as the primary model, other sources chosen by fit.** `PLAN.md:7` says this, and `:21` adds that "sharing Go's implementation language is not evidence that gopls is the better source".
- **Cross-language ports.** `PLAN.md:9` says "translation effort is not a reason to accept a weaker approach".
- **No adoption for its own sake.** `PLAN.md:9` says every adoption must name the existing work it improves. `PLAN.md:15` says the research's broader adoption language does not override the plan.
- **Impact-first, adaptive sequencing.** `PLAN.md:11`, `:31` and `:229` describe it. `PLAN.md:229` has a before/after check on the real app, a maintenance-cost check, and a discussion with the user before each substantial next assignment.
- **In-place rewrite.** The guidance is rewritten in place. There is no appendix.

## Findings

### 1. Critical — the one sequencing decision the plan commits to rests on a misdescribed baseline, with no measured latency breakdown

**Requirement missed:** "Sequence development by the next most impactful improvement, demonstrate real-app results."

The plan's first assignment is capability 1, justified as "Saving a relevant input starts preparation while the browser is idle" (`PLAN.md:13`). The outcome is stated as "starts development work without waiting for HTTP traffic… The application is ready or rebuilding when the next request arrives" (`PLAN.md:35`). Validation asks to "Demonstrate that an edit starts the existing build while no HTTP requests are arriving" (`PLAN.md:43`).

The baseline already does that:

- `internal/dev/server.go:321-350`: `loop` is a background goroutine that builds whenever `awaitChange` reports a change.
- `internal/dev/server.go:353-381`: `awaitChange` polls on a timer.
- `internal/dev/server.go:186-190`: the defaults are `Poll = 150ms` and `Quiet = 100ms`.
- `internal/dev/server.go:630-650`: `noticeEdit` only *holds* a request that arrives before the next poll. It does not start the build.

So builds already start with no HTTP traffic. What capability 1 actually removes is up to about 250 ms of poll and quiet delay, plus a whole-tree digest (`watch.go:150ff`) taken on each poll and in each request.

The recorded evidence points to much larger costs elsewhere:

- `history-inventory.md:15` and `:42` record a generation phase of about 3.2–6 s.
- `history-inventory.md:48` records "several seconds per Go edit" for `go build -o app-N`, link and child restart (`server.go:490-524`).
- `executed-preflight.md` records the Kit refresh after readiness at 6–14 ms.
- `landed-state.md` and `executed-preflight.md` both say whole-application edit latency has never been measured.

The plan has no step that measures where save-to-visible time goes in the real app before it ranks work. `PLAN.md:229` only measures *after* each delivery. "Next most impactful" therefore has no input, and the one ranking the plan makes points at what is probably the smallest term.

Capability 1 may still be a reasonable first step. It is cheap, and changed-file identities feed selective generation (3–6). But in that case it is a prerequisite, and the plan should say so. It should not be presented as the latency win.

**Impact:** The first delivery's before/after comparison will show a gain of a few hundred milliseconds against a multi-second loop. The plan has also told the builder to "demonstrate" a behavior that already exists. The human's main method, impact-ranked sequencing based on real-app data, is not applied at the only point where the plan makes a decision.

### 2. Issue — "effectively immediate" is not turned into a goal, and the dominant Go path has no strategy

**Requirement missed:** "aiming for effectively immediate save-to-visible-update behavior"; "are we aiming for the right goal? … do we have the strategy in place?"

The TL;DR makes the target softer: "fast enough that a developer would want to show it to colleagues", with "multi-second … waits" as the problem (`PLAN.md:5`). It never states what "effectively immediate" means for the two edit classes that matter:

- frontend edits, where Vite HMR is already close to immediate;
- Go edits, which today regenerate, run `go build`, link and restart a process.

It also never states which path is expected to get Go edits there.

The 13 capabilities are organized around the D/G/R upstream mapping IDs (`PLAN.md:193-213`). They mostly speed up analysis, generation, tooling and render lifecycle. The compile/link/restart floor, which the earlier ranking (`history-inventory.md:48`) put first, is not a capability. It appears only as the last bullet of "Genuine unresolved leads" (`PLAN.md:221`), framed as something to pick up "when linking, startup or repeated compiler discovery is the dominant remaining delay".

Vite feels immediate because it avoids bundling and restarting on each edit. For the Go side the plan does not say whether that implies avoiding the relink and restart, or whether it expects the ordinary rebuild path, made selective, to be fast enough. That is the big alignment question here. The plan leaves it open and shapes its body around what upstream offers rather than around the latency path.

**Impact:** Work could finish capabilities 1 and 3–10 faithfully and still leave a Go edit taking seconds. Nothing in the plan would show that the goal was unreachable on that route until the end. The plan needs a strategic position here: a stated target or an explicit decision to measure first. It does not need tactics.

### 3. Issue — off-goal adoptions carry full specifications in a dev-server plan

**Requirements missed:** "Do not adopt functionality merely because upstream has it"; "Build a best-in-class development server".

Four capabilities have no bearing on the save-to-visible loop:

- Capability 11 (`PLAN.md:157-167`) replaces the MCP protocol layer with the SDK gopls uses.
- Capability 12 (`PLAN.md:169-179`) is native SDK preparation for Swift/Zig/Xcode builds.
- Capability 13 (`PLAN.md:181-191`) is `sv` add-on preparation, which happens at project creation.
- Capability 2 (`PLAN.md:45-55`) is a general Go-command runner. Its own dependency line says it is "Independent of 1", and later capabilities need only "the applicable parts".

Each has full Adopt/Replace/Owner/Validation sections. The plan's sequencing rule picks the change that removes the largest remaining delay (`PLAN.md:31`), so these items can never be selected under it. `PLAN.md:31` calls them "independent improvements … select them when their benefit warrants the work". That leaves a second, upstream-driven selection route that never touches the stated goal. They are in the plan because the adoption survey found gopls or Vite counterparts (`adoption.md` rows R1–R4). Nothing in the dev-server goal asked for them.

**Impact:** The plan's stated scope is wider than its goal. A builder or reviewer reading it gets four specified workstreams that compete for attention while adding no dev-loop latency gain. They belong in a separate backlog or should be removed. The human's question about the right goal gets a mixed answer from the document itself.

### 4. Issue — the plan is mostly prescribed tactics, contrary to the human's strategy-first ask and to `CLAUDE.md`'s rule on delegating missions

**Requirements missed:** the human's "leave the tactical questions mostly for later", and `CLAUDE.md`: "Delegate missions, not methods… Sprint documents obey the same rule: mission, acceptance, ownership. No file lists, no prescribed designs, no step-by-step."

Strategy takes up about 25 lines (`PLAN.md:3-31`, `:225-235`). About 160 lines are capability bodies that prescribe designs with imperatives. Examples:

- "Port gopls content handles, cancel/clone/release and historical overlays" (`:61`).
- "Port gopls metadata change classification… ForFile/ImportedBy indexes… URI/hash/mode parse cache, including token-position ownership" (`:73`).
- "Use retryable keyed futures, shared typeCheckBatch, dependency waits before CPU-token acquisition" (`:85`).
- "Port x/tools's Runner/Invocation…" (`:49`).

They also carry line-anchored source links and implementation-level caveats, such as "gopls does not completely reconcile removed roots" (`:43`) and "Vite pending abort only evicts a cache entry" (`:67`).

This contradicts the plan's own framing that each owner "maps upstream independently and chooses the best-fitting implementation" and that the cited mechanisms "are source-backed candidates" (`PLAN.md:29`). A heading that says **Adopt G4, G6, G7** followed by "Port …" reads as a decision, not a candidate. The prescriptions were also written before any latency measurement (Finding 1). They lock in a gopls-centered retention design for 3–7 before anyone knows whether analysis retention is where the time goes.

**Impact:** The plan's detail caps builders at the planner's understanding, which is the failure `CLAUDE.md` describes. Because the detail sits in the plan, the human cannot quickly confirm the strategy. Per capability, the strategic content is the Outcome line, the Replace line, the owner, and the real-app proof. The adopt lists belong in the research index the plan already points to.

### 5. Nitpick — the research builders are sent to still states the directive the human overruled

**Requirement:** "Do not adopt functionality merely because upstream has it."

`PLAN.md:15` correctly says the research's broad adoption language does not override the plan. But the research index tells every reader the opposite:

- `index/README.md`: "Feasible **can implies should**."
- `index/adoption.md`: "Adopt upstream strategies everywhere applicable: feasible can implies should."
- `index/adoption-development.md`: "Feasible adoption is the requested target."

Handoffs point builders straight at those documents (`PLAN.md:231`). The plan wins on paper, but a builder reads both, and the research states its directive more strongly. Removing the directive from the research is a better fix than relying on a standing override sentence in the plan. This is outside the plan file and is noted for whoever owns the cache.

## Summary for the human's questions

- **Goal reflected?** Mostly in wording (`PLAN.md:5-9`), but "effectively immediate" is softened and never defined for Go edits (Finding 2). Off-goal items dilute it (Finding 3).
- **Priorities and methodology reflected?** The principles are there: Vite first, choose by fit, cross-language ports eligible, no adoption for its own sake, impact-ranked, measure and discuss (`PLAN.md:7-11`, `:229`). They are not applied at the plan's one committed decision (Finding 1).
- **Big alignment questions answered?** Two are open. Where does the time go today? What is the strategic route for Go edits to become near-immediate: no-relink replacement, or selective rebuild that is fast enough?
- **Strategy in place, tactics deferred?** The strategy paragraphs are sound. The document is mostly tactics (Finding 4).

## Outcome

material findings remain
