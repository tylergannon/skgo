# Adversarial review: fix plan for #265 — round 02

Written 2026-10-06, local time. Read-only review; nothing outside this file
was changed. Round 01 is `watcher-265-plan-round-01.md`.

## Target

The revised `ephemeral/plans/watcher-265-fix.md` (still untracked, worktree
`watcher-265` at `eec9fd4`). No implementation has landed: `git diff HEAD`
outside `ephemeral/` is empty, so "current implementation and proof" means
the code and tests the plan builds on, unchanged since round 01, plus the
research logs.

Authoritative sources are the same as round 01: issue #265, the research
directory, `@sveltejs/kit` 3.0.0 and the chokidar 3.6.0 bundled in
`@voidzero-dev/vite-plus-core@1.0.0` as installed under `example/web`, and
`CLAUDE.md`. The caller added one conversation-only requirement — the user
prioritises stability and build success over modest added latency — which is
a goal, not a narrowing, and is applied below. No scope narrowing was
requested; none was ignored.

## Evidence inspected

Everything listed in round 01, re-read where the plan changed, plus:

- `internal/vite/runner.go:71-110` — Go's `Boot`/`Refresh` consume the
  change cursor and drop only the modules the changed files reach.
- `example/devrender/devrender_test.go:548-562` — `settleChanges`: five
  consecutive equal readings at 200 ms, i.e. about one second of quiet, is
  what every change-log-based contract treats as "final".
- `example/devrender/devrender_test.go:430-470` — the build-emit test's
  700 ms / 500 ms spacing between writes and touches.
- `example/e2e/features/zz-source-update.feature` and
  `example/e2e/steps/dev.ts:94-104` — the HMR scenario's 30 s dev budget.
- `example/web/node_modules/vite` (vite-plus shim 1.0.0) exists, so a
  node-driven `resolveConfig` from `example/web` is available to step 1.
- Bundled chokidar `_awaitWriteFinish` (`node.js:16756-16787`): first poll
  at `pollInterval`, emit when `now - lastChange >= stabilityThreshold`; so
  the plan's "roughly 200–225 ms" for 200/25 is accurate. `_emit`
  (`:16649-16652`) refreshes `lastChange` on every fs event that arrives
  while the path is pending, so a threshold is a *quiet* window, not a
  fixed delay.

## Round 01 findings, status

1. `server.watch: null` / preservation mechanism — **addressed.** The plan
   now reads `config.server?.watch?.awaitWriteFinish` in the hook,
   contributes only on `undefined`, and drops the `null` contract with the
   correct reason (kit's hook, `vite/index.js:398-404`, runs first).
2. Regression not load-bearing on macOS / no injection mechanism —
   **addressed in design.** Step 2 names the mechanism (fixture-written
   wrapper config over the renamed copy) and makes the decision (package-wide
   `useFsEvents:false,usePolling:false`). A consequence of that decision is
   finding 3 below.
3. Partial-save conditional — **addressed.** Step 5 now mandates a positive
   observation (settled template served with its marker).
4. Latency unbounded — **partly addressed.** A number is stated for the
   proposed default; the escalation clause introduced with it is finding 2.

## Findings

### 1. Issue — Steps 3 and 4 contradict each other on the baseline's timing

Step 3 fixes the order: "First observe that the edit is served, then
immediately restore the original." Step 4 forbids "relying on a fixed sleep
or an assumed fast HTTP request" to land the restore inside the vulnerable
window. With the default removed, that window is the 50 ms after chokidar
*emits* the first `change` (`node.js:16685`, `_throttle(EV_CHANGE, path,
50)`); it does not open before emission, because until then Vite still
serves `probe = 1` from its cache and there is nothing to observe. Observing
the served intermediate therefore necessarily puts a module GET — Vite
invalidation, a transform, an HTTP round trip — between the barrier and the
restore write, which is exactly the "assumed fast HTTP request" step 4
rules out. The research landed its restore at 14 ms after emission on a
quiet laptop; a loaded Linux CI runner can miss 50 ms, and a miss makes the
baseline *pass* (both edits delivered), so the negative proof the validator
depends on is a coin the plan has not called.

The plan already permits the resolution — "recorded event timing" — but does
not connect it: the trial should be judged valid only when the fixture's
recorded timing shows the restore's raw fs event arrived within the throttle
window of the first emission, and otherwise retried a bounded number of
times before being reported as inconclusive (which is a failure of the
baseline run, not a pass). Alternatively the intermediate observation can be
taken from the recorded event/transform trace rather than from a live GET.
Either way, the plan should say which; as written an implementer who follows
step 3 literally violates step 4.

Impact: the one-off failing baseline and the permanent regression's
load-bearing-ness on CI both rest on an unspecified race.

### 2. Issue — The escalation clause has no ceiling, and the fixture's own settle logic breaks silently above one

"Increase the window further if partial-save or platform qualification shows
that it improves reliability; do not select the smallest passing value merely
for speed." Under the user's stability priority this is reasonable, but no
upper bound is given and no contract would catch one that is too large. Two
concrete interactions in the existing fixture make that dangerous:

- `settleChanges` (`devrender_test.go:548-562`) declares a file settled after
  roughly one second of an unchanged change-log count. A threshold at or
  above ~800 ms means a deferred `change` can land *after* settle returns and
  after the test has taken its cursor — the metadata-touch, invalid-UTF-8 and
  rapid-restore contracts then count a late event from the previous phase as
  if it belonged to the next, or miss one. That is the "baseline read off the
  thing under test" trap `CLAUDE.md` names, and nothing in `just test` would
  flag it; the suite would merely become flaky.
- The build-emit test (`:430-470`) spaces its writes 700 ms and touches
  500 ms apart on the assumption that each produces its own observation. A
  threshold near those numbers coalesces them. It still passes, because its
  assertions are "the authored file was mentioned at least once" and "build/
  never was", so coverage degrades without a failure.

The plan should state a ceiling tied to the fixture (well under the settle
window; the plan can also require `settleChanges` and the barrier to be
expressed in terms of the configured threshold) and require that the chosen
value be recorded with its justification, so a later "increase further" is
a decision with evidence rather than drift.

### 3. Issue — After step 2, the backend every macOS `just dev` uses has no standing automated coverage of the new default, and the modes meant to cover it are opt-in runs nothing executes

Forcing `useFsEvents:false` package-wide is the right call for a load-bearing
regression (round 01, finding 2), and Linux CI keeps covering inotify. The
cost is that FSEvents — the path every macOS developer runs under
`just dev`, and the only one on which the defect never reproduced
(`macos-fsevents.log`) — goes from being exercised on every developer's
`just test` to being exercised once, by hand, in a "test-only mode that
leaves native watcher options untouched". The plan's own words for the new
default under FSEvents are "only the forced `fs.watch` backend has candidate
evidence so far". An opt-in mode is a check that can silently not run;
`CLAUDE.md` treats a check that did not run as not passed, and no CI job is
macOS.

Given the user's stability priority this needs a stated decision, not an
omission: either the native and polling modes are one-time qualification
runs whose output (command, SHA, backend options as printed by
`watcher.options`, pass/fail) is attached to #265 and the PR and explicitly
described as not recurring; or the validator's macOS run executes the
native mode as a second focused invocation of the restored-content and
preservation contracts. The plan should also say what happens if the native
run disagrees with the `fs.watch` run.

### 4. Nitpick — The real-server partial-save assertion is timing-ambiguous

Step 5's "incomplete template write followed by the completed save" takes
one of two paths depending on the gap between the writes relative to the
threshold: inside it, the truncated bytes are coalesced and kit's validator
never sees them; outside it, kit throws, the adapter's guard
(`env.js:1086-1098`) sends the overlay, and the completed save recovers.
Both end with the marker served, so one assertion written without a stated
gap exercises whichever path the machine's timing picks. The plan should
require both gaps, written into the test as literal durations relative to
the configured threshold, so the recovery path the guard exists for is
provably still reached.

### 5. Nitpick — Wrapper-config mechanics worth stating

Step 2's wrapper must keep the discovery name (`vite.config.ts`) and the
renamed original must not match one (`vite.config.*` with a recognised
extension), or `vp dev` picks the wrong file. Vite also adds the imported
original to `configFileDependencies` and restarts the server on a change to
either; no present test writes them, but the fixture's `copyWebRoot` and any
future test that rewrites the example config must know the restart is the
consequence. One sentence in the plan avoids a confusing failure later.

## Outcome

`material findings remain`
