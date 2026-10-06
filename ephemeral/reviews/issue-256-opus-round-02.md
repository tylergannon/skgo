# Adversarial review — issue #256, round 02 (Opus 5.5)

## Target

The uncommitted working tree on `codex/issue-256-prerender-service` (base `10ad4de`), in
`/Users/tyler/.codex/worktrees/595c/skgo`. It is reviewed against
<https://github.com/tylergannon/skgo/issues/256> ("prerendering through one Go service") and
`CLAUDE.md`. Round 01 (`issue-256-opus-round-01.md`) is the baseline and has not been
modified.

The caller's constraints were treated as operating limits only: write only this artifact,
and reuse the captured runs. Nothing narrowed the review's subject matter, so nothing had to
be refused.

## Evidence inspected

- **Issue #256 body**: the plan, its exclusions, the lifecycle-test table and the acceptance
  paragraph. Also `CLAUDE.md`.
- **Implementation**:
  - `prerender_service.go` (all of it), including the new `failed` channel, the `abandoned`
    callback and the BaseContext guard.
  - `prerender_control_{unix,windows}.go`, `remote_execution.go`, `remote.go`,
    `prerender_remote.go` and `prerender_load.go`.
  - `internal/adapter/skgo-adapter/prerender.js`, all 716 lines: `alive`, `signalTree`,
    `stopJob`, `windowsControl`, `startJob`, `prepare`, `fail`, `cleanup`, the signal and
    parent observers, `installPrerenderFailureBoundary`, `invokeService`, `remoteInputs`,
    `remoteLoad` and `remoteFunction`.
- **Tests**:
  - `prerender_service_test.go`, including the new
    `TestPrerenderServiceAbandonedCallbackFailsHelper`.
  - `internal/adapter/prerender_service_timeout_test.go`.
  - `internal/adapter/prerender_inputs_lifecycle_test.go`: the fixture, the `observation()`
    probe at :309 and the drain assertion at :499-505.
  - The `prerender_inputs_build_test.go` edit.
- **Captured runs**:
  - `ephemeral/test-output-stable.log`: the complete `go test -count=1 ./... ./example/...`
    run started 15:20:12, ending in **FAIL**.
  - `test-output-final.log`, `build-output-stable.log`, `server-output-stable.log`,
    `vet-output-stable.log` and `browser-output-stable.log`. The last shows 5/5
    `engine-build.feature` scenarios passing: the prerendered Go layout and page loads, the
    dynamic prerender entry, the reused remote artifact, and a Go value under Kit's id.
  - `adapter-lifecycle-debug.log` and `generator-fixture-output.log`.
- **Worklog**: `ephemeral/worklog/prerender-service.md`, every entry after round 01.
- **Platform check** (read-only, a scratch Python process): on this Darwin 25.6 host,
  `killpg(pgid, 0)` against a process group whose only member has exited but has not been
  reaped returns **EPERM**. Once the member is reaped it returns ESRCH.

## Disposition of round-01 findings

1. **Five adapter regressions.**
   - The Malformed-source test and the declared-inputs string rewrite now match the tree.
   - The four lifecycle failures were fixed by the 150 ms grace.
   - The worklog's adjudication of the late-writer assertion is accepted. The descendant wrote
     while Kit's crawler was still running, so the file's existence did not separate
     post-failure work. The issue also excludes timing acceptance. The retained checks
     (whole group gone, private directory gone, finite interrupt deadlines) are what the
     issue asks for.
   - **There is still no passing full run**; see finding 1.
2. **A timeout did not fail the build.** The Go side is fixed: abandoning an active
   callback now fails the helper, which is tested at the real socket. The owner keeps nonzero
   exits. The composition is untested end to end; see finding 2.
3. **60 s compiler deadline.** Removed. The startup deadline now covers only the
   control-channel and readiness waits. Resolved.
4. **Residue and mixed style.** The no-op `try/catch` rethrows, `makeError`, the duplicate
   `inputs` type check and the tabs/single-quote half are all gone. Resolved.
5. **Unbounded stderr buffer.** The service's stderr is now relayed and not buffered. Only
   the compiler's stderr, which its failure message needs, is kept. Resolved.

## Findings

### 1. critical — On macOS, owner cleanup treats its own exited-but-unreaped helper as a drain failure. It failed a successful build and left the private directory behind in the stable full run.

*Verifiable bug / race.* Issue #256: "kill and reap the process group, then remove
temporary files." Issue #256 also asks for "Run the repository checks … once at the stable
implementation." CLAUDE.md: "`just test` green."

**Mechanism.**

- `alive()` (`internal/adapter/skgo-adapter/prerender.js:149-158`) probes the group with
  `process.kill(-pid, 0)`. It treats only ESRCH as "gone" and rethrows everything else.
- On Darwin, a group whose leader has exited but has not yet been reaped answers **EPERM**.
  The platform check above reproduces this.
- `stopJob` (`:196-209`) does the following:
  - It closes the control channel and waits only `SHUTDOWN_GRACE` (150 ms) for `job.exited`.
  - It then calls `alive(job)` at `:203`, and again at `:206` after SIGTERM plus 150 ms, and
    in the poll loop at `:208`.
  - If the Go helper is between `exit` and libuv's SIGCHLD reap when any of those probes
    runs, `alive` throws EPERM.
- That moment is likely whenever the helper's `Shutdown` and process teardown take longer
  than 150 ms, for example under a loaded full suite.
- `cleanup()` (`:426-455`) records the throw as a drain failure. It therefore **skips
  `rmSync(owner.dir)`** (`:446`) and throws `AggregateError("skgo could not drain owned
  prerender processes")`.

**Reproduction in the captured proof.** `ephemeral/test-output-stable.log`:

- `:7` and `:387-412`: `TestDeclaredPrerenderInputsBuildAndProduceNativeArtifacts` fails.
  - The build had completed (`skgo: wrote build/`).
  - Then `vp build` exits 1 with `AggregateError: skgo could not drain owned prerender
    processes` → `Error: kill EPERM at alive (prerender.js:152) at stopJob (prerender.js:203)`.
  - **A successful build was failed by its own cleanup.**
- `:417-418`: `TestPrerenderInputsBuildAppWaitsForProducerFailureDrain` fails with
  `observation=map[… groupAlive:false … privateDirExists:true]`.
  - The group was already gone (ESRCH) when the probe ran, yet the private directory remained.
  - `cleanup` keeps the directory only when `errors.length > 0`, so cleanup recorded an error
    here as well.
  - No line in the output names that error (see finding 3). The same EPERM race is the
    consistent explanation.
- `:433`: `FAIL github.com/tylergannon/skgo/internal/adapter`.

Both tests passed in the targeted `adapter-lifecycle-debug.log` run and in `test-output-final.log`
for the lifecycle case. That they fail only in the loaded full run is what this race predicts.
The worklog does not record the stable run's result.

**Impact.**

- On macOS, the primary developer platform here, `vp build` / `just build` fails
  nondeterministically after a successful prerender.
- Temporary helper directories leak on the failure path.
- The issue's acceptance criterion, a stable implementation with repository checks run once
  green, is unmet.
- The probe cannot tell "group still running" from "group exited, reap pending". The leader's
  own `exit` event (`job.exited`) and the reaped state are the authority for the leader. A
  group-probe EPERM for a group this process created and owns is not a permission problem.

### 2. issue — The timeout-fails-the-build guarantee is assembled from three links, and no test exercises the composition

*Incomplete requirement (proof).* Issue #256: "If it expires, abort that HTTP request and
**fail the build** with the operation identity and timeout in the error." The user
explicitly asked that "a timeout should cause the build to fail".

The guarantee now depends on all three of the following:

1. `invokeService` aborts at 30 s (`prerender.js:536-560`).
2. Go's handler sees `r.Context()` cancelled. It reports through `abandoned`, and
   `servePrerenderService` returns an error, so the process exits nonzero
   (`prerender_service.go:116-143, 74-93`).
3. The `service.exited` handler records any nonzero or signalled exit as `owner.failed`, even
   during closing (`prerender.js:396-406`). `installPrerenderFailureBoundary` then rethrows
   `owner.failed` after `buildApp` resolves (`:511-512`).

The tests cover links 1 and 2 separately:

- `prerender_service_timeout_test.go` covers link 1, against an `httptest` stand-in.
- `TestPrerenderServiceAbandonedCallbackFailsHelper` covers link 2, and only for
  `servePrerenderService`'s return value.

Nothing shows the one outcome the user asked for: a build whose application swallows the
timed-out call, so Kit writes the page anyway, still exits nonzero. Link 3 is the one that
was changed to fix round-01 finding 2, and no test fails if it reverts. Specifically:

- `!owner.closing || code !== 0` could go back to `!owner.closing`;
- the boundary's `if (owner.failed) throw owner.failed` could be dropped;
- the generated `main` could swallow `RunPrerenderService`'s error.

None of these reverts would turn any test red.

CLAUDE.md: contracts are tests at the real handler, and each must "actually fail if the
feature were broken or removed".

**Impact.** The user's one terminal-failure policy is held only by inspection, and a later
cleanup refactor can silently restore the round-01 behaviour.

### 3. nitpick — A cleanup failure on the failure path does not appear in the reported error

Issue #256: "Report cleanup failure alongside the primary error."

`fail()` (`prerender.js:319-326`) attaches the cleanup error as an
`owner.failed.cleanupError` property and rethrows the primary error unchanged. Its message
and stack say nothing about cleanup. In the producer-failure run (`test-output-stable.log:417-431`):

- cleanup demonstrably failed (the private directory was retained);
- yet the only text emitted is the primary `declared Inputs producer lifecycle failure`
  error;
- a reader cannot tell that a drain failed, or why.

When the primary error *is* the cleanup error, the property points to itself
(`cleanupError: [Circular *1]`, `:390`).

Vite+'s CLI happens to `inspect` the thrown object, so a real `vp build` may show the
property. Anything that reports `message` or `stack` (Kit, a harness, CI annotations) loses
it. Putting both in the thrown error itself, for example an `AggregateError` of primary plus
cleanup or a message suffix, would satisfy the requirement wherever the error is printed.

### 4. nitpick — Windows is implemented but has never run

- `prerender_control_windows.go` and `windowsControl()` (`prerender.js:212-255`) pass a
  duplex named pipe as the child's stdin.
- The worklog records only cross-compilation ("runtime remains unverified here").
- The issue names the Windows inherited-handle mechanism as part of the plan.
- CLAUDE.md: "never claim anything that hasn't been demonstrated running."

The code is a reasonable reading of the plan. The PR must state that Windows is unexercised
and must not present it as supported.

## Outcome

material findings remain
