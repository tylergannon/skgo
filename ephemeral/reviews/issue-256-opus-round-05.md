# Adversarial review — issue #256, round 05 (Opus 5.5)

## Target

Branch `codex/issue-256-prerender-service` at `a6e2366` (PR #258), reviewed against
<https://github.com/tylergannon/skgo/issues/256> and `CLAUDE.md`. The branch has two
commits on `10ad4de`:

- `5535d9a feat: use one Go HTTP service per prerender build`, which is the tree reviewed
  in rounds 01–04, now committed;
- `a6e2366 test: check build helper termination without a timed writer`.

Two other sources were applied:

- **The user correction** in `ephemeral/worklog/prerender-service.md`, final `correction`
  entry. The user rejected keeping the successful declared-input build's one-second
  late-writer probe, and authorized three things:
  - removing that probe;
  - checking helper shutdown in the existing successful build instead;
  - relying on the dedicated descendant, failure and interruption lifecycle tests for
    descendant coverage.

  This is a user decision. It is applied as a requirement, not reopened.
- **The caller's constraints**, treated as operating limits only: write only this artifact,
  and reuse the captured runs. Nothing narrowed the review's subject matter, so nothing had
  to be refused.

## Evidence inspected

- **Issue and instructions**: issue #256 (plan, exclusions, lifecycle-test table,
  acceptance), `CLAUDE.md`, and every worklog entry after round 04.
- **The diff of `a6e2366`**
  (`internal/adapter/prerender_inputs_build_test.go`, +16/−15):
  - It removes the `SKGO_LATE_RECEIPT` `sh -c 'sleep 1; printf late'` descendant from the
    fixture's `siteInputs`, along with the `time.Sleep(1200ms)` / `os.Stat(lateReceipt)`
    assertion.
  - It adds three checks:
    - the build output contains exactly one `skgo prerender service started (pid N)` and one
      `stopped (pid N)` line, for the same pid;
    - on Unix, `assertInputsProcessGroupGone(t, N)` passes. That helper
      (`prerender_inputs_lifecycle_test.go:551-557`) requires `kill(-N, 0)` to return
      exactly ESRCH.
  - Every declared-input and artifact assertion is retained.
- **`5535d9a`**: the same implementation that rounds 01–04 inspected. `prerender.js` is 747
  lines; `alive`, `signalTree` and `stopJob` (:149-230) have the round-04 EPERM handling.
  It also commits the captured logs and review artifacts under `ephemeral/`. That matches
  existing practice: 53 `ephemeral/*.log` files are already tracked at `10ad4de`.
- **The CI failure that prompted `a6e2366`** (run `37535874629` on `5535d9a`). Exactly one
  `--- FAIL`: `TestDeclaredPrerenderInputsBuildAndProduceNativeArtifacts`,
  `prerender_inputs_build_test.go:286: owned Go producer descendant performed late I/O
  after build completion`.
  - Under the new design the helper, and so any descendant it spawns, legitimately lives
    for the whole build.
  - A file written one second after `siteInputs` ran therefore does not show work after
    completion.
  - This is the same invalid premise the worklog had already corrected in the lifecycle
    fixture. The user's correction is consistent with the code.
- **Local proof of the final state**:
  - `ephemeral/declared-input-ci-fix.log`: the rewritten test passes (25.89 s).
  - `generator-output-drain-fix.log`: `internal/gen` `ok`.
  - `adapter-output-drain-fix.log`: the full adapter package, zero skips.
  - `test-output-validated.log`, `build-output-drain-fix.log` and
    `browser-output-stable.log`, as in round 04.
- **PR #258 CI on `a6e2366`**: run `37537696155` was `in_progress` when this review was
  written.

## Is the replacement check load-bearing?

- **Cleanup not running on success.** The `stopped (pid N)` line is printed only by
  `cleanup()` after a drain that reported no errors (`prerender.js`, cleanup). If the
  success path skipped cleanup, or cleanup recorded a drain failure, the exactly-one-stop
  assertion would fail.
  - This is not a self-derived baseline. The independent part of the check is the OS probe:
    the pid named in the `started` line must have no process group left when `vp build`
    returns.
  - Cleanup awaits group absence before the build resolves, so the probe has no timing
    race.
  - A helper or descendant still in the group would leave it present and fail the probe.
- **The issue's "one helper lifetime visible" acceptance** is now asserted on every run of
  this test, not only seen once in a captured log.
- **What the test no longer covers.** The successful build no longer spawns a descendant,
  so success-path descendant cleanup is no longer exercised here. That is exactly the
  coverage the user chose to reuse from the lifecycle tests. All of those run the same
  `stopJob(service, true)` group teardown with a TERM-ignoring descendant.

## Findings

### 1. nitpick — Windows remains implemented but never run

The status is unchanged from rounds 02–04:

- `prerender_control_windows.go` and `windowsControl()` in `prerender.js` are only
  cross-compiled;
- `alive` probes a single pid on Windows, and teardown goes through `taskkill /T`;
- the new group-absence check is skipped on Windows, and the lifecycle tests `t.Fatal`
  there.

The worklog states this plainly. CLAUDE.md: "never claim anything that hasn't been
demonstrated running." The PR body should say that Windows is unexercised.

No other findings. The user's correction is implemented as recorded, it introduces no
timed child, sleep or extra fixture, and it leaves the runtime unchanged.

## Notes (not findings)

- The PR's CI run on `a6e2366` had not finished. The only failure on the previous head was
  the removed probe, and the rewritten test passes locally. Merge should wait for that run
  to go green, per the repository protocol.

## Outcome

only nitpicks remain
