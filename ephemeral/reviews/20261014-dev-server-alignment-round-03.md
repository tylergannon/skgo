# Adversarial review: dev-server plan alignment, round 03

## Target

The whole of `ephemeral/projects/dev-server/PLAN.md` as it stands in the working tree: 99 lines, uncommitted on top of `75c74ad`. I reviewed it against `CLAUDE.md` and the same human requirements as rounds 01–02:

- Build a best-in-class Go development server aiming for effectively immediate save-to-visible updates.
- Vite is the primary model. gopls, Kit and Go tooling are selected by fit.
- Cross-language ports are fully eligible, with no bias toward Go sources.
- Improve work SKGO already performs; nothing is adopted merely because upstream has it.
- Sequence by impact, demonstrate results on the real app, and periodically discuss latency and maintenance cost with the user.
- Rewrite the plan in place.
- Settle the big alignment questions and the strategy now, and leave tactics for later.

## Scope note

The caller gave only the artifact path and a read-only constraint. Both are valid operating constraints, and I ignored no narrowing. As before, I read the human's acceptance of a strategy-level plan as a requirement on how much detail the plan should carry, not as a limit on this review.

## Evidence inspected

- `CLAUDE.md`, in particular:
  - "Delegate missions, not methods"
  - "This is mirroring, not design"
  - "Build the software"
  - "There is no command that means done"
  - the one-binary, goja-pool and Go-only I/O constraints
- `ephemeral/projects/dev-server/PLAN.md`, all lines.
- Rounds `20261014-dev-server-alignment-round-01.md` and `-round-02.md`.
- Baseline `repos/skgo-adoption-baseline-53179fc/internal/dev/server.go`. I re-checked it against the baseline description at `PLAN.md:17`: Poll/Quiet defaults at `:186-190`, the background loop at `:321-381`, and the request-time `noticeEdit` at `:630-650`.
- Research under `/Users/tyler/src/skgo-token-cache`:
  - `index/README.md`, `adoption.md`, `adoption-development.md`, `executed-preflight.md`, `landed-state.md`, `native-engines.md`, `compiler-retention.md`, `publication-lifetime.md`
  - `collection/history-inventory.md`, `collection/history-native.md`

## Disposition of round-02 findings

1. **Resolved.** Round 02 found that the native-replacement route left its developer-facing cost to the builder. `PLAN.md:29` now addresses this:
   - It names the demonstrated loader's private adapted toolchain, version-specific runtime internals, and cgo/JIT use on darwin/arm64.
   - It says these are experimental conditions, not accepted installation requirements.
   - Before substantial native integration, toolchain/setup burden, platform coverage and Go-version maintenance cost must be discussed with the user against the expected latency gain, and a development support envelope agreed.
   - It forbids the builder from turning prototype requirements into product policy.

   This matches the facts recorded in `native-engines.md` and `collection/history-native.md:18-33`. It also meets the human's requirement to discuss latency against maintenance cost with them. The plan does not claim a decision the human has not made.
2. **Resolved.** Round 02 found the latency milestone loose. `PLAN.md:5` now sets "a few hundred milliseconds or better" and says the concrete target is set with the user from the baseline comparison.

## Alignment assessment

These are the human's questions, answered from the current text.

- **Right goal.** `PLAN.md:5` states the effectively-immediate aim, a first milestone for compatible Go edits, and a target agreed with the user. It covers frontend edits (Vite HMR kept) and structural edits (automatic recovery). It defines success as a fast, reliable, maintainable application rather than adopted code.
- **Priorities and methodology.** `PLAN.md:7-11` restates each of the human's directions:
  - Vite primary, with other sources chosen by fit.
  - Translation effort and shared language give no bias.
  - Borrowing only improves existing work.
  - Next most impactful change, with the prerequisites it actually needs.
  - Maintenance cost reviewed with the user after each delivery.

  `PLAN.md:13` and `:91` keep the research's broader "can implies should" language from widening scope.
- **Big alignment questions answered.**
  - *Where does the time go?* That is the first assignment, measured on the real app before any ranking (`PLAN.md:19`, `:95`). The plan correctly says historical numbers are leads, not the current breakdown.
  - *What route reaches near-immediate Go edits?* `PLAN.md:23-27` states the steady-state path and the body-edit target. It says plainly that selective generation does not remove the link/restart floor, and names native replacement as the route through it. A rebuild fallback is kept for incompatible edits, and the route is reconsidered with the user if the runtime boundary cannot be met.
  - *What will we ask developers to accept?* That is a decision for the user before integration (`PLAN.md:29`).
- **Strategy in place, tactics deferred.**
  - The six missions are stated as capability, accepted behavior and ownership/research routes (`PLAN.md:33-81`), with no prescribed designs. That follows `CLAUDE.md`'s mission-delegation rule.
  - Mission numbering is explicitly not an execution order (`PLAN.md:33`).
  - Off-goal adoptions (MCP, SDK packaging, add-on staging, the general command runner) are not assignments (`PLAN.md:91`).
  - Validation uses the repository's ordinary means: Go tests at the real handler, Kit-client scenarios only where kit's client is needed, existing `just` recipes with no silent skips, and looking at the real app. No new proof machinery is added (`PLAN.md:97-99`).
- **Factual accuracy of the strategic premises.**
  - The baseline description (`PLAN.md:17`) matches `server.go`.
  - The native-loader conditions (`PLAN.md:29`) match the research.
  - The statement that preflights do not establish whole-application timing (`PLAN.md:19`, `:27`) matches `executed-preflight.md` and `landed-state.md`.

I looked for remaining problems and found none worth raising:

- **Contradictions:** none. The native route at `:25` and the support envelope agreed with the user at `:29` fit together: the route is strategy, and what developers must accept is the human's decision.
- **Re-added inventory scope:** none.
- **Prescriptive tactics:** none.
- **Missing requirements:** none.
- **Over-built infrastructure:** none.
- **Absolute research links:** the links to the local token cache use absolute paths. That matches the layout convention in `CLAUDE.md`, which also cites an absolute local path. The research lives outside the repository, and builders are told to map the pinned sources themselves, so this is not a defect.

## Findings

No findings.

## Outcome

no findings
