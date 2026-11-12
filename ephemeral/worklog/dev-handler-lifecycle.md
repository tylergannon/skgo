# Development handler lifecycle

friction: Installed Gimbal implement run 01M78DWAVCE2VKJ098K0BGSSZE marked its single outcome completed after QA accepted only the baseline reproduction task. Confirmed production code remained unchanged and the retirement regression still failed. Continued in the same worktree with a cohesive whole-repair outcome instead of treating selected-task QA as feature completion.

decision: Repair upstream skgo #452 from current main a0da95ecd54cb92aa98592a25110b98e911db3de; original report is skgo-project #237. Gimbal owns coding and independent validation, coordinated in the requesting chat.
correction: Source inspection confirms missing application cleanup on current main. The earlier Teamwork constructor-failure experiment is not proof of successful reload behavior or the cause of the macOS memory warning.
friction: The desktop create_worktree tool failed to return on two attempts, with no checkout created. Used an isolated Git worktree on codex/dev-handler-lifecycle after terminating the unresponsive calls; root checkouts and unrelated worktrees remain intact.
decision: Use installed implement workflow role defaults and one cohesive lifecycle outcome. Preserve the complete acceptance boundary rather than treating a partial QA pass as completion.
friction: First Gimbal admission 01M78DTHE3RC08SBS8ZD7KQ6W9 stopped before any coding because Codex daemon version failed against an absent control socket. Started the existing local Codex daemon through its supported CLI; no provider/model substitution or implementation replay.

## Task 1: Reproduce the leak through Host.Reload (2026-11-12)

decision: This task establishes the runtime baseline only. Production lifecycle code is unchanged at a0da95ecd54cb92aa98592a25110b98e911db3de. The reusable regression deliberately expects correct retirement and fails on the confirmed defect; subsequent tasks own the repair and independent validation.

Worker model: `gpt-6.1-sol`, coding role, high effort, as recorded for `outcome.1/implementation.1/task.1/coding.1` in the active Gimbal run's session/model-call metadata. This worker personally ran the commands and inspected the HTTP/resource observations below. No independent validation is claimed by this entry.

Added `internal/hotreload/host_lifecycle_test.go`, tagged `skgo_qualification`, using the existing captured-host fixture pattern. It prepares a small Go application with generated registries, compiles the actual generated host with `NativeKit: true`, starts its child, and calls the public `Host.Reload` control-pipe path. The real `NativeDevelopment.Handler` bridge wraps the returned application handler. There is no constructor-only harness or `Install` substitution.

Each application handler implements `io.Closer` and owns a raw OS descriptor with an exclusive file lock. Raw descriptors avoid an `os.File` finalizer masking the missing application cleanup. The parent probes that lock using an independently opened file description; the `Close` method also appends a generation receipt. `/state` reports completed/active application requests. Literal expected generations and retirement values are chosen in the test, independently of framework implementation. Temporary fixture files live in Go's test directory and are removed by test cleanup; no proof scripts or run-output files were added.

Baseline rerun command:

`GOTOOLCHAIN=go1.27.3 go test -tags=skgo_qualification ./internal/hotreload -run '^TestHostReloadRetiresIdleApplicationResource$' -count=1 -v`

Exit status: **1**, at `idle application retirement`, after the generated child successfully acknowledged reload. Final raw-descriptor fixture run took 2.061s overall. Personally observed:

| Observation | Expected | Observed |
| --- | --- | --- |
| Initial HTTP response | 200, `handler-101` | 200, `handler-101` |
| Initial request finished before reload | `completed=1 active=0` | `completed=1 active=0` |
| Initial resource held | true | true (external lock attempt blocked) |
| `Host.Reload` | nil error | nil error, success acknowledged |
| Replacement HTTP response | 200, `handler-202` | 200, `handler-202` |
| Child identity on both responses | same PID as started host | `99959` on both generations and both state responses |
| Replacement request finished | `completed=1 active=0` | `completed=1 active=0` |
| Idle predecessor resource still open | false | **true** (external lock attempt still blocked) |
| Replacement resource still open | true | true |
| Application Close receipts | `handler-101\n` exactly once | empty; receipt file absent |

The assertion is retained at the end of `TestHostReloadRetiresIdleApplicationResource` and reruns with the command above. Both the HTTP replacement and missing retirement were observed in one child before its deferred `Host.Close` reaped it. The test contains no sleeps, polling, skipped checks, or empty test selection.

friction: The ambient Go tool selected go1.27.4, while native development explicitly requires go1.27.3. The initial unpinned focused command exited 1 at that gate; using `GOTOOLCHAIN=go1.27.3` reaches the actual host. An intermediate fixture run exited 1 because an empty manifest used a nil remote list; the fixture now declares an explicit empty list and successfully initializes the real bridge.

Setup/checks: `vp install --frozen-lockfile` in `example/web` exited 0, installing this worktree's pinned Kit 3.1.1 from the local package cache with no lockfile change. Read the required current-Kit guidance and inspected the installed package/version and Vite dev reload source; this task changes no mirrored Kit semantics and runs no Vite/browser session. The first `GOTOOLCHAIN=go1.27.3 just test` exited 1 before tests because mise had not trusted this worktree's `example/mise.toml`. Inspected that configuration (Node 24.22.0, vite-plus 1.3.0, project-local tool path), then `mise trust example/mise.toml` exited 0 and reran the check.

friction: The plain-shell `GOTOOLCHAIN=go1.27.3 just test` then ran the ordinary suite but exited 1 in the existing adapter test `TestPrerenderOwnerCompilesOncePublishesAndDrainsOnEveryFailure`: expected `/invalid readiness/`, observed `Error: skgo prerender service startup timed out` (adapter package 64.204s). Ordinary hotreload tests passed (0.388s). The failure reports Node v26.13.1, while `mise -C example exec -- node --version` reports the configured v24.22.0. Retrying the repository recipe under the pinned mise environment to distinguish environment drift from a repository failure; no adapter code changes.

Final ordinary check: `GOTOOLCHAIN=go1.27.3 mise -C example exec -- just test` exited **0**. The adapter test/package passed (3.377s); ordinary hotreload tests passed (0.400s), and the recipe completed across the listed workspace modules. The daemon restarted during the first pinned-environment attempt, losing its process handle and result; after confirming no check process remained, reran that command to obtain the observed exit status above. `gofmt -l internal/hotreload/host_lifecycle_test.go` printed nothing; `git diff --check` exited 0. The new qualification regression remains deliberately red as recorded above; ordinary `just test` does not select `skgo_qualification`.

Limitations: This baseline proves idle predecessor application-resource retention across one successful handler reload, with the replacement still serving. It does not establish a safe cleanup contract, failed-candidate preservation, in-flight native bridge retirement, repeated replacements, or graceful final shutdown. Those remain outcome requirements for later tasks. It does not diagnose Teamwork or explain its reported memory warning. No outcome/workflow edits, docs edits, commits, pushes, merges, deployments, or releases were performed.
