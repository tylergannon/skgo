# Independent implementation review, round 3: SKGO development generation

Outcome: **no findings**.

## Target and scope

Reviewed the entire current implementation and proof in `/Users/tyler/.codex/worktrees/skgo-dev-generation/skgo` against the complete accepted plan `/Users/tyler/.codex/worktrees/dd88/skgo-project/ephemeral/plans/dev-generation-callback.md`, the user-authorized SKGO-first implementation scope with Gimbal deferred, repository AGENTS.md, agent-protocol and adversarial-review. No scope narrowing was applied. The code/test snapshot remained frozen throughout review. I changed only this artifact.

Inspected all tracked changes and all new implementation/test files: the development CLI/configuration, public tooling-only devgen protocol, retained helper compilation/discovery/identity and process lifecycle, startup/update/rebuild coordination, raw watcher acquisition and epoch admission, focused tests, complete real-session fixture and worklog. I retained the prior source-backed production boundary assessment: application runtime entrypoints do not import the new tooling integration. Deferred Gimbal generation decisions are not represented as delivered.

## Findings and adjudication

No material correctness, performance or production-isolation finding remains in the reviewed SKGO slice. No useful nitpick to report.

The round-2 cache-starvation defect is resolved in `internal/gen/raw_events.go` and the watcher event boundary. Known per-module `.cache/node-compile` roots and explicitly configured NODE_COMPILE_CACHE roots are excluded as tool-owned outputs. The watcher also avoids feeding those paths into ordinary source acquisition when the optional generator integration is selected. Explicit raw control files remain selected. This is narrower than excluding `.cache`, arbitrary dot directories or all generated files.

The regression independently checks both sides: events under conventional and configured Node cache roots do not advance the generation epoch, while adjacent `.cache/producer-input.json` remains selected. The real fixture continuously writes both cache forms every 10ms during startup, updates and recovery. I ran that fixture while the ordinary full suite ran concurrently; it now completes. This resolves the actual operational failure from round 2, rather than merely passing in an otherwise quiet tree.

Earlier round-1 repairs remain correct and covered:

- Every initial obsolete retry reruns current application generation before preparing a guarded build; the fixture injects a raw non-Go event after the first generation pass and before initial Prepare/Commit and successfully converges.
- Helper discovery fingerprints Go-selected native/assembly/embed/module/workspace inputs and build settings; literal assembly behavior changes from 101 to 202 in a fresh helper.
- Helper/prerequisite processes own their groups; real descendants are stopped on replacement, close and canceled prerequisite.
- Raw collection is optional; ordinary sessions retain no raw history. The watcher opt-out regression observes a Go barrier after creating/deleting 64 distinct paths and asserts no map/channel/epoch backlog.
- The private route-link mirror is excluded by its ownership boundary while authored originals and application-owned generated Go remain selected.

## Independent executed checks

Passed:

1. `go test ./devgen ./internal/gen ./internal/devserver -run 'Test(SerializedCallback|RejectNonMonotonic|Raw|WatcherDeliversNonGo|WatcherWithoutGenerator|Generator)' -count=1` — devgen 0.104s, gen 2.630s, devserver 10.183s.
2. `go test -tags=skgo_dev_integration ./internal/devserver -run '^TestRealSessionApplicationGeneration$' -count=1 -v` — **PASS**, 154.956s. Its controlled continuous cache writers ran throughout; ordinary `just test` ran in a separate process concurrently.
3. `just test` — completed with all tested packages reporting success, including gen 217.602s, hotreload 205.818s and the example 109.344s. The full captured log has no failure, error, termination or skip marker. The implementer's separately executed exact-snapshot full suite also completed successfully, but this review has its own full-suite execution.
4. `git diff --check` — no diagnostics.

My diagnostic logs are `/tmp/skgo-generation-review-round3-live.log` and `/tmp/skgo-generation-review-round3-suite.log`; these are local execution outputs, not new repository proof infrastructure. The broad check completed naturally before a cleanup request found any live descendants; no process was stopped and all my owned check executions are complete.

## What the running fixture demonstrated

The runtime test reaches the actual Go/Vite development application and asserts literal endpoint output plus saved project state. It reports application PID sequence:

`54686 → 56271 → 57817 → 58338 → 58774 → 59141 → 59918`

These correspond to initial startup, generated-input change, recovery after partial-write error, recovery after helper death/lost reply, missing-output repair, fresh helper implementation, and deleted/recreated non-Go input. Each recovery checks the expected literal generated value through the served handler. Partial-write and helper failure keep the previous process answering correctly until repair; retrying unchanged generated bytes still requires the full replacement. Equal-byte output retains output mtime and the current application PID. The original helper survives ordinary application replacement; dead/edited helper logic gets a new helper process. The durable project literal remains `saved-project-340`, exercising exclusive-lock handoff through the existing application constructor.

The injected startup event is visible in the helper's cumulative request paths and startup completes. Continuous known-cache writes no longer fill those selections and prevent admission. A few unrelated bounded dependency-tool events can still cause conservative passes; the measured integration converged under the full real suite, and no further operational failure or unrequested performance claim is inferred from those events.

## Why the tests are load-bearing

- Protocol tests compare literal request fields and exact changed/error replies, and reject corrupt/non-monotonic requests.
- Raw-event tests keep compiler revisions unchanged for non-Go input, preserve obsolete selections, and make a stale admission callback fail if invoked. Removing epoch guarding would fail them.
- Cache tests assert positive selection of adjacent application input, so an over-broad ignore cannot satisfy them by selecting nothing. The live cache writer demonstrates the boundary through the real watcher/coordinator.
- Helper tests exercise actual prerequisite-produced compilation input, parent retention, broken-code rejection, recovery, selected Go membership and actual assembly output. They do not rely only on internal cache statistics.
- Descendant tests use real child processes and require their disappearance after lifecycle transitions.
- The real handler's literal values, process IDs, mtimes and durable project state would expose removing generation, dropping the rebuild latch, prematurely retiring healthy application state, or reusing stale helper code.

These are ordinary contracts and running-software observations, not an exit-code-only product verdict. The no-findings outcome applies to this generic SKGO implementation and its stated proof. It does not claim Gimbal producer integration, save-to-visible performance leadership, browser-suite parity for all development features, or publication/installation of a release; those are separate work and remain accurately bounded.
