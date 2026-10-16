# Adversarial review: exclusive rebuild (#340), round 02

Date: 2026-10-16 ~11:50 local
Worktree: `/Users/tyler/.codex/worktrees/skgo-340-exclusive-rebuild`, branch `codex/issue-340-exclusive-rebuild`.
Target: commit `0293cb8` "fix(dev): compile replacements before retiring application owners". Working tree clean at review time.

Prior round: `ephemeral/reviews/20261016-exclusive-rebuild-round-01.md` (reviewed the uncommitted snapshot that reused the old executable on handoff failure).

No caller instruction narrowed subject matter. Read-only boundary and artifact path honored. The caller's request to run `TestCapturedNativeHTTPHost` and to validate that coverage is load-bearing was treated as additional work, not as scope.

## Evidence inspected

- `git show 0293cb8` for `internal/hotreload/host.go`, `internal/devserver/session.go`, `internal/hotreload/host_test.go`, `internal/devserver/session_integration_test.go`; surrounding code in `session.go` `Run` loop (crash path at ~239-258, rebuild path at ~405-470), `host.go` `Close`/`receive`/`exchange` (364-400), `internal/gen/retained.go:570-593` (`Commit` holds `p.mu` across `admit`).
- Issue #340 body and source-confirmation comment; `CLAUDE.md`.
- `ephemeral/worklog/exclusive-rebuild.md` (decision to recover from last admitted inputs; graceful shutdown declared out of the authorized repair).
- Proof under `/Users/tyler/.codex/worktrees/dd88/skgo-project/.gimbal/issue340-proof/`: `baseline.log` (v0.34.4: "skipped saved project ... already owned by another instance", then `404 GET .../conversations/new`), `fixed.log` (rebuild publishes PID 15235 with no skip, project URL 200; compile failure keeps 200 on PID 14869; repair publishes 16624), `final-gimbal.log` (candidate panics in package init, "restored previous application (Go pid 24662)", project page 200, repair publishes 24847), `final-tests.log` (hotreload 58.2s ok, devserver with tag 54.1s ok), `regression-fixed.log` tail (tagged test PASS 42.2s).
- Ran here: `cd example/web && vp exec go -C ../.. test -count=1 -run '^TestCapturedNativeHTTPHost$' ./internal/hotreload` → `ok 11.532s` (build cache warm). Output at `/private/tmp/claude-501/-Users-tyler--codex-worktrees-skgo-340-exclusive-rebuild/bcd19645-f4d0-4679-9e4b-b3d18c043cf2/scratchpad/hotreload-test.log`.

## Disposition of round-01 findings

1. **Stale-binary recovery (issue):** repaired. On failure after the old host is retired, `session.go` now calls `makeHost(goodInputs, goodDist)` (the same recipe as the crash path), publishes it, and sets `hostInputs = goodInputs`. `Host.Stop`/restart-from-executable was removed entirely, which also removes the `installed`/`DistDir` bookkeeping hazard. The integration test now installs a fast-path edit (`literal-8754:9`) after the last full rebuild and asserts that value after "restored previous application"; a restart of the old executable would serve `8508` and fail. Load-bearing.
2. **SIGKILL vs graceful stop:** unchanged, but the worklog records it as outside the user-authorized repair and the `Start` doc comment no longer overstates. Demoted to a nitpick below.
3. **Coverage gated behind the integration tag:** repaired. `TestCapturedNativeHTTPHost` (default `go test`, runs under `just test`) now has the fixture constructor take a real `flock` on `owner.lock`, compiles the replacement while the first host serves, asserts `replacement.PID == 0` after `CompileHost`, checks the old host still answers, closes it, then `Start`s the replacement. Under the previous `BuildHost` ordering the replacement constructor would hit `EWOULDBLOCK` and the test would fail at build time. Load-bearing. Verified passing here.
4. **`p.mu` held across kill + constructor:** still present (nitpick below).
5. **Post-recovery assertion indistinguishable:** repaired by the `8754` anchor (see 1).

## Findings

No critical findings and no issues remain. Nitpicks:

### N1. Cancellation during handoff can end `skgo dev` with a spurious failure message

**Where:** `internal/devserver/session.go` recovery block (`if err != nil && stopped { ... makeHost(goodInputs, goodDist) ... return fmt.Errorf("replacement failed: %v; restore previous application: %w", err, recoveryErr) }`); compare the crash path at `session.go:~245` which checks `ctx.Err() != nil` before reporting.

If Ctrl-C arrives after `closeHost(old)` and before `publishHost(replacement)`, `replacement.Start(ctx)` fails with the context error, `stopped` is true, and recovery runs `CompileHost` on a cancelled context. Whether `recoveryErr` wraps `context.Canceled` depends on which step fails first (`MkdirAll`, `prepareToolchain`, `packages.Load`, or the `go build` exec), so `cmd/skgo/dev.go:33` `errors.Is(err, context.Canceled)` may not match and the process exits non-zero with "replacement failed: context canceled; restore previous application: ...". Before this commit, cancellation mid-rebuild returned nil via the loop's `ctx.Done()` case. A `ctx.Err() != nil` guard before attempting recovery restores the old exit behavior. Not reproduced here (timing-dependent); causal chain is direct from the code.

### N2. `Commit` now holds the project mutex across kill, wait, and the replacement constructor

**Where:** `internal/gen/retained.go:577-585`; `internal/devserver/session.go` admit callback.

Previously the admit callback was a symlink rename. Now it spans `Process.Kill`, `Wait`, `exec`, and the application constructor (Gimbal's catalog restore). `Observe` from the watcher goroutine and `project.Revision()` block for that duration. Nothing is lost (fsnotify queues), and the loop is single-threaded anyway, but it is the one place a slow consumer constructor now stalls filesystem observation. A comment at the call site would save the next reader a trip.

### N3. Stop remains `SIGKILL`; consumer shutdown never runs

**Where:** `internal/hotreload/host.go:366-371`; generated `main` defers at `host.go:~641`.

Recorded in the worklog as a deliberate boundary of this repair. The fixture's kernel `flock` is released on kill, which is what both tests and the Gimbal proof demonstrate. A consumer whose lease is a PID file, an app-owned fixed socket path, or a store needing a clean close still has to tolerate a hard kill on every full rebuild. Fine to leave, but the issue thread should say so when this closes.

### N4. The handoff gap reaches the browser as a Vite SSR 500, not a deliberate unavailable response

**Where:** `final-gimbal.log` and `regression-fixed.log`: `Error: connect ENOENT /private/tmp/skgo-*/go.sock` followed by `500 GET ...`.

Those lines in the proof are from initial startup (pre-existing: Vite SSR dials `go.sock` before the first publish). The same path is now also open during every full rebuild, from `closeHost(old)` (which unlinks the socket) until `publishHost(replacement)`, and for the whole recompile on the recovery path. The issue accepts a restart interval, so this is not a defect of the fix, but a developer who reloads during that window sees a Vite 500 page rather than a "restarting" state. Worth noting in the issue closure as the expected shape of the interval.

### N5. `BuildHost` is now only a test convenience

**Where:** `internal/hotreload/host.go:85-95`; only non-test caller removed, `session.go` carries its own `makeHost` closure with the same body; `host_test.go:119` is the sole user.

Harmless duplication. Either `makeHost` should call `BuildHost`, or `BuildHost` should go.

## Outcome

`only nitpicks remain`
