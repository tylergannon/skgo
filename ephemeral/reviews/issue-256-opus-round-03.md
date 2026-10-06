# Adversarial review — issue #256, round 03 (Opus 5.5)

## Target

The uncommitted working tree on `codex/issue-256-prerender-service` (base `10ad4de`), in
`/Users/tyler/.codex/worktrees/595c/skgo`. It is reviewed against
<https://github.com/tylergannon/skgo/issues/256> and `CLAUDE.md`. Rounds 01 and 02 are the
baseline and have not been modified.

The caller's constraints were treated as operating limits only: write only this artifact,
and reuse the captured runs. Nothing narrowed the review's subject matter, so nothing had to
be refused.

## Evidence inspected

- **Issue and instructions**: issue #256 (plan, exclusions, lifecycle-test table,
  acceptance) and `CLAUDE.md`.
- **Implementation**:
  - All of the current `internal/adapter/skgo-adapter/prerender.js`. In detail: the revised
    `alive` (:149-162), `signalTree` (:163-188), `stopJob` (:203-227), `startJob`'s
    `hasExited` marker (:290-296), `fail` (:337-352) and `cleanup` (:454-484).
  - `prerender_service.go`, unchanged since round 02.
- **Tests**: the rewritten `internal/adapter/prerender_service_timeout_test.go`
  (`TestPrerenderCallbackTimeoutFailsBuildDespiteApplicationCatch`).
- **Worklog**: every entry in `ephemeral/worklog/prerender-service.md` after round 02.
- **Captured runs**:
  - `ephemeral/test-output-validated.log`: complete
    `go test -count=1 ./... ./example/...`, every package `ok`, `just test exit status: 0`,
    finished 15:33:08. The last source edit was the timeout test at 15:30:32, and the run's
    adapter package took 138 s, so this run covers the current tree.
  - `build-output-validated.log`: one `service started/stopped (pid 43028)`,
    `✔ done`, exit 0.
  - `vet-output-validated.log`: exit 0.
  - `server-output-validated.log`: prod listening.
  - `adapter-real-timeout.log`: the new test passes in 32.67 s.
  - `adapter-declared-drain.log` and `adapter-cleanup-debug.log`.
  - `browser-output-stable.log`: 5/5 engine-build scenarios, run at 15:21, before the
    cleanup changes.
- **Platform checks** (read-only scratch Python on this Darwin 25.6 host): for a process
  group whose only member has exited but has not been reaped, `killpg` returns **EPERM**
  for signal 0, **and also for SIGTERM and SIGKILL**. After reaping it returns ESRCH.

## Disposition of round-02 findings

1. **EPERM failed a successful build and leaked the private directory.**
   - `alive` now treats EPERM as "exists".
   - `stopJob` joins `job.closed` (exit and stdio close, i.e. reaped) within the grace before
     probing.
   - The full sequential run is green, and the declared-inputs build and producer-failure
     drain checks that failed now pass.
   - Fixed, except for the narrower window described in finding 1 below.
2. **Timeout-fails-the-build was not tested as a composition.** Fixed.
   `TestPrerenderCallbackTimeoutFailsBuildDespiteApplicationCatch` runs a real native build:
   - The application catches the real 30 s timeout and renders a fallback.
   - The test asserts the exact `BUILD_APP_REJECTED:skgo prerender service exited
     unexpectedly` cause, that Go's handler context was cancelled (via a file the Go body
     writes), and the full group and private-directory drain.
   - It would fail if any of these were removed: the helper abandonment, the owner's
     exit-to-failure handling, or the boundary's rethrow.
   - It replaces the socket-only 30 s test, so the suite's wall-clock cost did not grow.
3. **Cleanup failure was not reported alongside the primary error.** Fixed. `fail` appends
   `cleanup failed: …` to the primary error's message and stack, and skips the
   self-reference case.
4. **Windows unexercised.** Still true; it carries forward as finding 2.

## Findings

### 1. issue — `signalTree` can still throw EPERM when the helper exits as the 150 ms grace expires, failing a successful build

*Race condition with a causal explanation.* Issue #256: "kill and reap the process group,
then remove temporary files". The corrected intent, as the worklog states it, is that "a
post-leader-exit denied termination remains a cleanup failure **unless the existing bounded
final group check observes ESRCH**".

`signalTree` (`prerender.js:180-185`) tolerates EPERM only when `job.hasExited` is already
true. `hasExited` is set in the child's `exit` listener (`:293`), which Node runs only after
libuv has handled SIGCHLD and reaped the child.

**Sequence:**

1. On success cleanup, `stopJob(service, true)` closes the control channel. It then waits on
   `bounded(job.closed, SHUTDOWN_GRACE)` (`:208-213`), a 150 ms timer.
2. The Go helper finishes `Shutdown` and exits close to the 150 ms mark. It is now a zombie
   leader of a group with no other members.
3. libuv's loop runs the timer phase before it services the SIGCHLD pipe again. This happens
   when the timer expiry woke the loop, or when the Node thread was descheduled across the
   exit, which is plausible under the loaded full suite that exposed round 02's bug. So the
   grace rejection runs first, and its continuation runs in the same timer phase:
   - `alive(job)` (`:215`) gets EPERM and returns `true`.
   - `signalTree(job, false)` (`:216`) calls `kill(-pgid, SIGTERM)` and gets **EPERM**. The
     platform check above confirms this for SIGTERM.
   - `job.hasExited` is still `false`, because the exit listener has not run, so `:185`
     rethrows.
4. `stopJob` rejects, and `cleanup` records the error and skips `rmSync` (`:476`). It throws
   `AggregateError("skgo could not drain owned prerender processes")`. The build that had
   already written `build/` fails, exactly as in round 02's `test-output-stable.log:387-412`.

The bounded final check (`:221-226`) would have seen ESRCH a few milliseconds later, once
libuv reaped the leader. But it is never reached, because the throw happens first.

**Scope.** The window is far narrower than in round 02:

- the leader's exit must fall between the loop leaving its poll phase and the grace timer's
  callback;
- the helper normally exits well inside 150 ms when idle.

The validated full run did not hit it, and no capture shows it. Still, the outcome is
identical to the critical round-02 bug: a nondeterministic spurious failure of a successful
macOS build, plus a leaked private directory. It is also the precise case the new comment at
`:181-182` says is handled.

The same reasoning applies to the SIGKILL call at `:218`. For a group this owner created,
EPERM from `kill` already means "something is still there" in `alive`. Treating it the same
way in `signalTree`, and letting the existing bounded final check decide, would match the
stated intent without adding a retry.

### 2. nitpick — Windows remains implemented but never run

`prerender_control_windows.go` and `windowsControl()` (`prerender.js:230-271`) are
cross-compiled only. The worklog says "runtime remains unverified here". The process-tree
path also differs on Windows:

- `alive` probes a single pid;
- teardown goes through `taskkill /T`;
- the lifecycle tests `t.Fatal` on Windows.

None of that has executed. CLAUDE.md: "never claim anything that hasn't been demonstrated
running." The PR should say that Windows is unexercised rather than present it as
supported.

## Notes (not findings)

- **The new timeout test costs about 33 s of real time.** That is the hard-coded
  `CALLBACK_TIMEOUT`. The worklog's decision not to add test-only timeout controls follows
  the issue's exclusions. The test runs once and asserts several outcomes from one build,
  which is the pattern CLAUDE.md asks for, so it is not reported.
- **The browser scenarios ran before the round-03 cleanup edits.** They last ran at 15:21,
  and the worklog says so plainly. The later changes touch only owner cleanup, not served
  bytes. The final build was reopened in Chrome and inspected (worklog, final
  `observation`).

## Outcome

material findings remain
