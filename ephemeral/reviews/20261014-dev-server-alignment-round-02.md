# Adversarial review: dev-server plan alignment, round 02

## Target

The whole of `ephemeral/projects/dev-server/PLAN.md` as it stands in the working tree: 97 lines, rewritten in place since round 01 and still uncommitted on top of `75c74ad`. I reviewed it against `CLAUDE.md` and the same human requirements as round 01:

- Build a best-in-class Go development server aiming for effectively immediate save-to-visible updates.
- Vite is the primary model. gopls, Kit and Go tooling are selected by fit.
- Cross-language ports are fully eligible, with no bias toward Go sources.
- Improve work SKGO already performs; nothing is adopted merely because upstream has it.
- Sequence by impact, demonstrate results on the real app, and periodically discuss latency and maintenance cost with the user.
- Rewrite the plan in place. Settle strategy and alignment now; leave tactics for later.

## Scope note

The caller restated no narrowing beyond the artifact path and the instruction not to edit other files. Both are valid operating constraints. As in round 01, I read the human's acceptance of a strategy-level plan as a requirement on the plan's level of detail, not as a limit on this review.

## Evidence inspected

- `CLAUDE.md`: mirroring rule, "Delegate missions, not methods", "Build the software", the rule that no command means done, and the one-binary/no-cgo SSR constraints.
- `ephemeral/projects/dev-server/PLAN.md`, all lines.
- Round 01: `ephemeral/reviews/20261014-dev-server-alignment-round-01.md`.
- Baseline dev loop `repos/skgo-adoption-baseline-53179fc/internal/dev/server.go` (Poll/Quiet defaults at `:186-190`, `loop`/`awaitChange` at `:321-381`, `build` at `:445-524`, `noticeEdit` at `:630-650`). This checks the plan's new description of the baseline.
- Research in `/Users/tyler/src/skgo-token-cache`:
  - `index/README.md`, `index/adoption.md`, `index/adoption-development.md`
  - `index/executed-preflight.md`, `index/landed-state.md`
  - `index/native-engines.md`, `index/compiler-retention.md`, `index/publication-lifetime.md`
  - `collection/history-inventory.md`, `collection/history-native.md`

## Disposition of round-01 findings

1. **Resolved.** Finding 1 said the first assignment was justified by a misdescribed baseline and nothing was measured. `PLAN.md:17` now describes the baseline correctly: background rebuild through 150 ms/100 ms whole-tree polling, plus scans inside requests. That matches `server.go:186-190`, `:353-381` and `:630-650`. `PLAN.md:19` makes the first assignment a real-app latency breakdown across frontend, body and declaration edits, chosen with the user. `PLAN.md:10` says to establish the breakdown first.
2. **Resolved.** Finding 2 said "effectively immediate" had no target and the Go path had no route. `PLAN.md:5` keeps "effectively immediate" and sets a first milestone for compatible Go edits. `PLAN.md:23-27` states the steady-state path and names the target for body edits: no generation, no rediscovery, no link, no restart, no reload. It says plainly that selective generation does not remove the link/startup floor, and it makes native replacement the route through that floor. That route is now mission 4 (`PLAN.md:57-63`) and is no longer a trailing lead.
3. **Resolved.** Finding 3 said MCP, native SDK, add-on staging and the command runner had full specifications. `PLAN.md:89` removes them as assignments and lets command/check work in only when it serves a selected edit-path mission (`PLAN.md:55`).
4. **Resolved.** Finding 4 said the plan was mostly prescribed tactics. The missions are now Capability / Accepted behavior / Ownership and research (`PLAN.md:31-79`). `PLAN.md:13` and `:95` state that designs are not prescribed. This matches `CLAUDE.md`'s rule to delegate missions, not methods.
5. **Resolved within the plan.** Finding 5 said the research still told builders that feasible adoption should happen. `PLAN.md:13` tells builders to read the research's broader adoption directives as historical context, not execution instructions. The research files still contain those directives, but that is outside this file and is not reported again.

The goal, priorities and methodology now match the human's wording point by point:

| Human requirement | Where the plan meets it |
|---|---|
| Effectively immediate goal | `:5` |
| Vite primary; choose by fit, not by language | `:8` |
| Improve existing work only | `:9`, `:89` |
| Impact-first with the prerequisites actually needed | `:10`, `:93` |
| Discuss latency and maintenance with the user | `:11`, `:93` |
| Real-app demonstration | `:19`, `:61`, `:93`, `:97` |
| Rewritten in place | the whole document |

## Findings

### 1. Issue — the native-replacement route leaves its known developer-facing cost to the builder

**Requirements missed:** "periodically discuss latency and **maintenance cost** with the user", and the human's request that "the big alignment questions [be] properly answered."

`PLAN.md:25` makes native replacement the strategic route through the link/restart floor. `PLAN.md:63` calls the mechanism "an implementation choice requiring application proof". `PLAN.md:27` asks the builder to come back only "if a candidate cannot satisfy the actual application/runtime boundary."

The research already records what the only demonstrated candidate costs a developer who uses the dev server, not just SKGO:

- `native-engines.md` (Toolchain intrusion): goloader requires exposing `cmd/internal` as `cmd/objfile`. The POC used a private, adapted clone of GOROOT.
- `native-engines.md` (Runtime coupling): on darwin/arm64 it needs cgo plus MAP_JIT/pthread JIT calls, and symbols must survive linking (`-ldflags=-s=false`).
- `native-engines.md`: its module, type and itab layouts and its runtime linknames are tied to one Go version (1.27), "rather than a stable application API".
- `collection/history-native.md:18-20` and `:33`: the POC ran on one platform with a private 1.27.2 toolchain. On macOS the loader requires cgo and private compiler/runtime internals.
- `publication-lifetime.md:23`: the standard-plugin alternative needs cgo `dlopen`, an exact toolchain match and Linux/FreeBSD/macOS, and it cannot unload.

These are not hypothetical failure modes. They are the known price of the route the plan has chosen. They determine:

- whether `skgo dev` would need a modified or private Go toolchain on the developer's machine;
- which platforms get the fast path;
- whether each Go release can break dev mode.

Those are exactly the latency-versus-maintenance trade-offs the human asked to discuss. The plan does not record that these costs are accepted, bounded or still undecided. It frames the mechanism as the builder's choice, and the only reason it gives for escalating is failure to meet the runtime boundary, not the cost of meeting it.

The history shows the human raised goloader themselves and "approved more aggressive reload techniques" (`history-inventory.md`, root lines 5555 and 6129). So this may already be decided. If so, the plan should say so and say where the limits are, such as a private toolchain, specific platforms or cgo in dev. Otherwise one builder may reject goloader on pure-Go grounds and another may quietly ship a patched-toolchain requirement.

**Impact:** The most consequential product decision in the plan — what a developer must install, and on which platforms, to get the headline speed — is left to whoever builds mission 4. It surfaces only after integration work has been spent, rather than as a decision the human makes at the start. Raising it is a strategy question, not a tactical one: one sentence recording the accepted toolchain/platform/cgo envelope, or naming it as the first question to put to the user before mission 4 starts, would close it.

### 2. Nitpick — "hundreds of milliseconds or better" is a loose milestone for "effectively immediate"

`PLAN.md:5` sets the first milestone for compatible Go edits at "hundreds of milliseconds or better". Read literally, a 900 ms update meets it, and that is not what the human described. The recorded POC reached about 254 ms with most of that time spent rehashing inputs (`history-inventory.md:38`). `PLAN.md:93` does guard against stopping early ("an intermediate gain, not completion"), so this matters only if someone reads the milestone as an exit criterion. A concrete first number, for example a few hundred milliseconds for a body edit, with the floor still the aim, would remove the ambiguity. Choosing that number can reasonably wait for the baseline comparison in `PLAN.md:19`.

## Outcome

material findings remain
