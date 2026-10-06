# Testing and validation: findings and continuation

## Can the original work finish?

Yes. Issue 247 is bounded fixture isolation and shared compatible setup. It does not require redesigning the entire testing system, repairing Gimbal, or settling every process-lifecycle question.

Part A implementation is present and its three changes have independent Opus acceptance: shared isolated production prerender fixture; separate authored failure fixture; shared minimal adapter fixture. Complete Part A integration validation remains outstanding. Part B requires typed-parameter APIs/tests to integrate into main, then migration of typed-load evolution and shared-parameter fixtures against that integrated base. Do not implement those APIs here or edit their active worktrees. Both parts are required to close 247.

A passing retry is evidence that a run passed, not an explanation or repair of a previous failure. An unresolved lifecycle defect must remain explicitly reported. Whether it blocks delivery of these fixture changes needs an explicit disposition based on relevance and preserved proof; neither silently waive it nor automatically expand 247 into a lifecycle rewrite.

## Original value and boundaries

A developer should be able to verify a narrow generator/prerender contract without unrelated demo consumers preventing the assertion from running. Compatible expensive preparation should run once; literal real-handler, generator, pinned Kit, adapter and production-composition contracts must remain. Whole-example and client behavior coverage remains.

Authoritative plan: `ephemeral/plans/20261006-issue-247-test-fixtures.md`.

No new proof framework, runner, persistent cache or wholesale suite rewrite. Large CI speedup is not the completion criterion. Browser-transcript replay against Go is a separate future performance investigation: https://github.com/tylergannon/skgo/issues/248 . The user declined prioritizing modest browser pruning.

## What actually went wrong

### 1. Fixture coupling makes failures difficult to interpret

Whole-example copies make a narrow test compile unrelated handlers, matchers and generated consumers. Changing a frontend package path or matcher type can break a contact consumer before the intended assertion. That is a demonstrated test architecture problem, not a prerender failure.

The new contract-only fixtures address this. Independent validators deliberately broke a real demo contact consumer: narrow fixtures passed while example compilation failed. They also broke relevant behavior and observed failures at the intended assertions, then restored mutations.

### 2. Task acceptance became outcome completion

Gimbal run `01M497K2S3BYMEB2T0YC0CZ3S7.implement` supplied one broad Part A outcome. Its planner produced four tasks. After the first task, Opus returned `validation_passed: true` and no substantial gaps, judging the shared production fixture. Gimbal's implementation loop treated that as completion of the entire outcome and exited. Three mandatory tasks were still pending.

This was not Claude going idle. Structured acceptance immediately preceded terminal success. The validation prompt conflates the selected task and current outcome in one verdict, and the loop trusts that verdict. This is a recurrence of https://github.com/tylergannon/gimbal/issues/424 ; this session's evidence is attached there. Installed Gimbal: `v0.12.2-0.20261001161540-4f2f025eef52`.

Recovery used explicit remaining outcomes, which advanced correctly. This is a workaround, not proof that the workflow is repaired. Separate task acceptance from whole-outcome acceptance without requiring execution of obsolete planner tasks merely because they remain on a list.

### 3. Validation discarded the evidence needed to diagnose failures

Opus ran the full shuffled adapter package through a grep filter selecting result lines. Two signal-drain tests failed, but assertion messages and child-process diagnostics were discarded. A later isolated run and same-seed full rerun passed. The first failure's cause is now unknown.

Reported: https://github.com/tylergannon/gimbal/issues/428 . This was an agent-authored command error, not evidence that Gimbal lost output supplied to its recorder. Pipelines also risk reporting the filter's status instead of the tested command's status. Preserve full output and producer status first; summarize afterward using ordinary tools.

An earlier mutation failed during setup. Filtered FAIL lines initially hid that distinction. After coordinator intervention, Opus discarded it and performed a narrower mutation that failed the intended HTTP assertion. A failing command alone does not prove an assertion is load-bearing.

### 4. Existing tests are not automatically valid requirements

Two tests failed in a full shuffled run:

- `TestPrerenderInputsVPOwnerSignalDrainsBlockedProducer`: 53.04s.
- `TestPrerenderInputsOuterVPCLISignalDrainsBlockedProducer`: 16.10s.

Seed: `1791313852756971000`. Full failed run: 294.501s. Isolated pair passed; unchanged same-seed full package passed in 194.497s. Report: https://github.com/tylergannon/skgo/issues/251 . No causal explanation is established.

A concrete test-contract concern was found afterward. The outer-CLI test sends SIGTERM to the outer vp process, waits for that process, then immediately requires the Go process group and private directory to be gone. SKGo's Vite owner detects parent loss on a 50ms polling interval and then starts asynchronous cleanup. Waiting for the launcher is not an explicit acknowledgment that the surviving owner completed cleanup. Inherited output pipes and Go's two-second WaitDelay may incidentally affect timing; they are not a deliberate cleanup protocol.

Installed vite-plus 1.0.0 delegates build execution through a native binding. Reading its JavaScript entry does not establish that native launcher's full signal-forwarding/reaping contract. Thus the synchronization concern is concrete, but it is NOT a proven explanation of the recorded failures. It also does not automatically explain the actual-Vite-owner signal test.

Distinguish these promises before changing code:

- A normal failure inside our live adapter should finish owned cleanup before its build promise settles.
- Graceful termination of the actual build owner should exercise cleanup owned by SKGo.
- External termination of an upstream launcher requires an explicitly established contract: immediate completion, bounded eventual cleanup, or unsupported behavior. Do not silently assume one.

If bounded eventual parent-loss cleanup is required, test that event explicitly. If it is not a supported requirement, reconsider both the test and the parent-watching machinery. Do not add machinery solely to satisfy an unexamined assertion; do not delete real leak assertions merely to get green.

### 5. There is separate evidence of a genuine cleanup failure

https://github.com/tylergannon/skgo/issues/250 records a native crawler failure where the build rejected while its Go process group and private directory remained, with `kill EPERM`. That report has original diagnostics. It is stronger evidence of an application cleanup problem than our filtered signal-test failures. A common cause between 250 and 251 is unproven.

The relevant adapter code owns detached Go process groups, signal handlers, parent polling, termination escalation and temporary directories. This complexity deserves review against required user behavior. Its existence alone does not prove every piece is necessary or incorrect.

## Principles for the rethinking

At the first failure, determine which promise the test protects and classify the failure: application defect, test defect, environment/setup defect, or unknown. Fix it or report it promptly with the original evidence.

Review tests as engineered software. Does the assertion measure the promised behavior? Does synchronization wait for the right event? Are expectations independent? Does the test fail for its contract rather than unrelated setup? Is its cost justified by confidence unavailable from a simpler test?

Use real Go handler tests for HTTP contracts and browsers for client behavior. Isolate unrelated application code. Share immutable compatible builds; retain intentionally incompatible states. Avoid turning verification into a second application.

A green command, a task-level QA pass, and a completed workflow are different facts. None alone establishes the whole requested result. Conversely, an unrelated failing test must not automatically force a universe-wide rewrite. Make the delivery consequence explicit.

Measurements here show reduced preparation counts and lower observed local times, but cache warmth and concurrent load were not controlled. Do not present them as a stable CI speedup.

## Continuation state

Worktree: `/Users/tyler/.codex/worktrees/e857/skgo`.
Branch: `codex/issue-247-test-fixtures`.

Accepted checkpoints:

- `b25585e`: shared production prerender fixtures.
- `f237642`: isolated failure diagnostic and shared bootstrap.
- `1f039db`: shared minimal adapter build.
- `fa09df9`: bug reports and causal-investigation correction.

Current Gimbal continuation: `01M498TZ4MSB3BXVYT81SYMWRX.implement`, instance `/Users/tyler/.codex/worktrees/5f2d/skgo/.gimbal/instance`. Last inspected in this chat: outcome 3, complete Part A integration, was active. Worker reported current-source build, generator/example/dev-render checks and visual interaction; full adapter Go run was still pending, with browser suites to follow. These are last observations, not final acceptance. Read fresh public results before asserting current state.

The scheduled monitor `monitor-issue-247-fixture-implementation` is PAUSED at the user's explicit request. Do not restart it automatically. The user subsequently questioned continued use of Gimbal; no new Gimbal runs should be launched. The existing run was not canceled by pausing the schedule. Resolve its actual state before any further execution or competing tests.

The user requires focused causal investigation and timely bug reports, not reassuring prose. They explicitly require asking whether a failing test should be rewritten/reconsidered/removed rather than assuming the implementation must grow to satisfy it. Do not relaunch duplicate reviews or build elaborate scheduled prompts. The user asked for this artifact to inform an overall rethinking; it does not itself authorize that larger redesign.

Issue records in `ephemeral/bugs/20261006-*` contain exact report text. Relevant independent results are in the two run directories under `.gimbal/runs/`; inspect public tool results and structured assessments, not private reasoning. Do not commit the runtime directories or raw reviewer logs.

## Follow-up: elapsed time and avoidable repetition

The combined-checkout checks subsequently completed: the worker's full Go JSON recorded 1,250 passing test/subtest events, no test failures/skips, and passing packages. Measured command wall time was 240.39s; adapter package time was 237.168s, almost the entire critical path. The Go producer's separate exit status was not captured. Both browser commands captured exit 0: production 47.47s between public command dispatch/result (runner 44.0s), development 258.25s (runner about 4.3m). These are one checkout/run's observations, not CI benchmarks.

Visual inspection was not separately timed. Public interaction calls show an approximately 49-second production burst and 12-second development burst, excluding server startup, later cleanup, and unrelated work between them. Do not report the intervening wall-clock span as visual-inspection labor or present this approximation as measured command time.

The final Opus reviewer repeated the contact-isolation exercise and launched another full Go run after the worker's broad checks. Earlier task reviewers had already validated the changed contracts and relevant mutations. Distinguish useful independent scrutiny from automatically repeating every command at every handoff.

Practical speed priorities: one broad validation execution at a stable source state; reuse its raw results across reviewers unless a concrete gap or change invalidates them. Use one independent validator rather than worker-spawned supplemental reviewers plus workflow QA. Make the assignment and acceptance boundary match so successful partial tasks do not trigger workflow recovery. Capture first-failure diagnostics and producer exit status before summarization. Verify installed prerequisites before launching an expensive suite. Investigate the actual critical paths: adapter build/lifecycle tests account for nearly all full-Go elapsed time; the 11 serial development source-edit scenarios take about 2.7 of 4.3 browser minutes. Optimize setup and legitimate contracts there before broad browser-pruning or new replay infrastructure. No tests should be removed solely for cost.

## Pause checkpoint after GitHub updates

The continuation run has now completed. Final Opus5.5 QA accepted Part A with no substantial gaps; it independently verified existing evidence and ran additional Go checks. Its restoration generation invalidated the example build-freshness timestamp, requiring a rebuild and example-only rerun; root and example checks ultimately passed. The lifecycle diagnosis remains unresolved. The recorded GOWORK concern is already addressed by internal/gen TestMain setting GOWORK=off; do not implement a redundant fix for that finding.

Fetched origin/main is now f0631f0 (#249), with typed-load APIs and internal/gen/load_params_test.go. That enables the typed-load portion of Part B after rebase/fresh baseline. internal/gen/shared_params_test.go remains absent from main: separately verify shared/remote-parameter integration before starting that portion. Neither rebase nor Part B was started.

User explicitly requested issue updates then pause. Updated SKGo247 (scope, failure triage, Part A acceptance, Part B readiness), SKGo251 (test-contract classification and bounded diagnosis), and Gimbal428 (redundant proof and validation-created failures). Exact comment bodies are in ephemeral/bugs/20261006-247-validation-followup.md, 20261006-251-failure-disposition.md, and 20261006-428-validation-amplification.md. Stop here. Scheduled monitor remains paused; do not restart it or initiate another Gimbal run.
