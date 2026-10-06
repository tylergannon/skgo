# Adversarial review — issue #256, round 04 (Opus 5.5)

## Target

The uncommitted working tree on `codex/issue-256-prerender-service` (base `10ad4de`), in
`/Users/tyler/.codex/worktrees/595c/skgo`. It is reviewed against
<https://github.com/tylergannon/skgo/issues/256> and `CLAUDE.md`. Rounds 01–03 are the
baseline and have not been modified.

The caller's constraints were treated as operating limits only: write only this artifact,
and reuse the captured runs. Nothing narrowed the review's subject matter, so nothing had to
be refused.

## Evidence inspected

- **Issue and instructions**: issue #256 (plan, exclusions, lifecycle-test table,
  acceptance) and `CLAUDE.md`.
- **Change since round 03**: the source mtimes show one edit,
  `internal/adapter/skgo-adapter/prerender.js` at 15:35:39. I re-read `alive` (:149-162),
  `signalTree` (:163-188) and `stopJob` (:203-230) in full:
  - `signalTree` now records EPERM as `job.signalError` unconditionally. It no longer depends
    on `job.hasExited`.
  - Bounded reaping and group absence then decide the outcome. These are the `job.closed`
    wait (`CLEANUP_TIMEOUT`) and the 5 s group-absence poll.
  - The final error message and `cause` carry the permission diagnostic.
  - `hasExited` now only selects whether to join `closed` within the grace (`:208`).
- **Worklog**: every entry in `ephemeral/worklog/prerender-service.md` after round 03.
- **Captured runs after the edit**:
  - `ephemeral/build-output-drain-fix.log`: one `service started` / `stopped (pid 58248)`,
    `✔ done`, exit 0.
  - `ephemeral/adapter-output-drain-fix.log`: the full `internal/adapter` package, uncached
    and verbose. `PASS`, `ok … 109.509s`, zero `--- SKIP` lines.
- **Captured runs before the edit**: `test-output-validated.log` (full run, every package
  `ok`), `vet-output-validated.log` and `browser-output-stable.log`.
- **Which tests drive `prerender.js` outside `internal/adapter`**:
  - `internal/gen/prerender_fixture_test.go:29` and `typed_load_fixture_test.go:22` link the
    worktree's `internal/adapter` into their fixtures.
  - `internal/gen/prerender_test.go:37` and `prerender_production_fixture_test.go:41` run a
    real `vp build` through it.
- **Platform checks from round 03 still apply.** On this Darwin host, signals 0, TERM and
  KILL sent to a group of exited-but-unreaped processes all return EPERM. POSIX group `kill`
  reports EPERM only when no member could be signalled. So EPERM from a group this owner
  created means that only zombies or unsignalable members remain. The bounded checks that
  follow correctly decide whether that state clears.

## Disposition of round-03 findings

1. **EPERM in `signalTree` before Node observes the exit.** Fixed.
   - A transient EPERM no longer throws. The existing bounded `closed` and group-absence
     checks see ESRCH once libuv reaps the leader.
   - A persistent EPERM still fails cleanup within the bound and names the permission error.
   - No retry or extra signalling was added.
   - The private directory is kept only when a real drain failure is recorded.
2. **Windows unexercised.** Still true; it carries forward as finding 2.

Nothing else in the implementation changed after round 03, so its earlier dispositions
stand. In summary:

- one Go HTTP service per build with `/load`, `/remote` and `/inputs`;
- 127.0.0.1:0 with a bearer secret announced over fd 3, and over an inherited duplex pipe on
  Windows;
- env published in the `buildApp` pre hook after Kit's snapshot and restored on cleanup;
- a 30 s callback timeout that fails the build even when the application catches it, proved
  by a real build;
- a finite readiness deadline and no compiler deadline;
- one owner that cancels, closes, waits a 150 ms grace, signals and reaps the group, then
  removes temp files, with cleanup failure appended to the primary error;
- control-channel EOF stops the helper, and the vp parent watcher is kept;
- remote execution and encoding are shared through `remote_execution.go`;
- the one-shot protocol tests and the retry test are deleted; the kept lifecycle tests
  pass;
- the example About page exercises parent/child loads, transported Money, a deferred value
  and declared-input prerender remotes. It was served by Go and opened in Chrome.

## Findings

### 1. nitpick — No single full run exists at the final tree. The worklog's reason for reusing the earlier run is inaccurate.

Issue #256: "Run the repository checks appropriate to the affected behavior once at the
stable implementation."

The worklog's last observation reused the 15:33 full `just test` "for unchanged
Go/generator code". The final edit, though, is in `prerender.js`, and `internal/gen`'s
prerender tests run real `vp build`s through that same file (`prerender_fixture_test.go:29`,
`prerender_test.go:37`, `prerender_production_fixture_test.go:41`). Those builds include a
failing-load build whose owner cleanup takes the changed path. They were last run before
the edit.

The risk is small:

- the edit only stops an exception for EPERM, an errno that has no other cause for a
  self-owned group;
- every lifecycle contract that the edit could break is in `internal/adapter`, and that
  package was re-run in full with zero skips.

Still, the statement that the reused run covers unchanged code is not accurate, and the
acceptance wording asks for one run at the stable state. Re-running `internal/gen` once (about
60 s) closes this. A full `just test` would also do it, and costs about one more minute.

### 2. nitpick — Windows remains implemented but never run

`prerender_control_windows.go` and `windowsControl()` (`prerender.js:230-271`) are
cross-compiled only ("runtime remains unverified here"). The Windows process-tree path has
never executed:

- `alive` probes a single pid;
- teardown goes through `taskkill /T`;
- the lifecycle tests `t.Fatal` on Windows.

CLAUDE.md: "never claim anything that hasn't been demonstrated running." The PR should say
that Windows is unexercised.

## Outcome

only nitpicks remain
