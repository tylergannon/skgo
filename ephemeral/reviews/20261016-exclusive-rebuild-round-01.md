# Adversarial review: exclusive rebuild (#340), round 01

Date: 2026-10-16 11:34 local
Worktree: `/Users/tyler/.codex/worktrees/skgo-340-exclusive-rebuild`, branch `codex/issue-340-exclusive-rebuild`, uncommitted working tree on top of `c3f111c`.

## Review target

Uncommitted changes to:

- `internal/hotreload/host.go` (+86/-31): `BuildHost` split into `CompileHost` + `(*Host).Start`; new `(*Host).Stop`; `Close` delegates to `Stop`.
- `internal/devserver/session.go` (+48/-?): full-rebuild path now compiles the replacement while the old host serves, then inside `Commit` stops the old host, starts the replacement, publishes it; on failure after the stop it restarts the old executable.
- `internal/devserver/session_integration_test.go` (+52): fixture app acquires a real `flock` on `owner.lock` in its constructor and reads a durable `catalog` file; adds a rejected-initialization step asserting the "restored previous application" log line and continued service.
- `ephemeral/worklog/exclusive-rebuild.md` (new).

Authoritative requirements used: issue tylergannon/skgo#340 and its source-confirmation comment; `CLAUDE.md`; the caller's restatement that the user authorized "compile the replacement while the old application serves, then stop and wait for the old process before replacement initialization, with live proof in the existing Gimbal upgrade worktree." No caller instruction narrowed the subject matter; the read-only boundary and artifact path were honored.

## Evidence inspected

- Full `git diff` of the three Go files, plus `internal/devserver/session.go:100-520` (whole `Run` loop), `internal/hotreload/host.go` (struct, `CompileHost`, `Start`, `Stop`, `Close`, `Prepare`, `Install`, generated `main` template at lines ~620-650).
- `internal/gen/retained.go:570-593` (`ProjectBuild.Commit` holds `p.mu` across the admit callback), `internal/gen/retained_watch.go:200,239,290` (watcher calls `Observe`, which takes `p.mu`).
- `internal/devserver/router.go` (`setApp`, proxy transport), `cmd/skgo/dev.go:32-36` (`context.Canceled` handling), `internal/adapter/skgo-adapter/dev.js:160-200` (dist snapshots are content-hashed directories that are never removed).
- `Justfile` `test` recipe and `.github/workflows/ci.yml` (neither runs `-tags skgo_dev_integration`).
- `go build ./... && go vet ./internal/hotreload ./internal/devserver`: clean.
- Not run here: the tagged integration test and the Gimbal consumer comparison. The caller states the parent session is running both; this review did not see their output. The worklog records only the v0.34.4 reproduction in Gimbal, not a run of the fixed binary, so the "live proof" element of the authorization is unverified from this worktree.

## What the change gets right

The ordering defect named in #340 is fixed in the primary path: `compileHost` runs while the old host serves (`session.go:408`), and inside `current.Commit` the old process is killed and waited (`old.Stop()`, `session.go:426`) before `replacement.Start` runs the application constructor (`session.go:428`). A compile failure never reaches `Stop`, so the healthy host keeps serving. The fixture's `flock` is a real exclusive OS lease held by the constructor, and under the old ordering the replacement constructor would fail with `EWOULDBLOCK`, so the existing "valid structural repair did not replace host" assertion is load-bearing for the fix.

## Findings

### 1. Issue: recovery after a failed handoff restarts a stale binary while the session's bookkeeping says otherwise

**Where:** `internal/devserver/session.go:435-446`; `internal/hotreload/host.go:291-343` (`Start`), `host.go:430-440` (`Install` sets `h.installed`), `host.go:117` (`installed = files` only at compile time).

**What happens:** `old.Start(ctx)` re-executes the executable compiled at the old host's last full rebuild, with `SKGO_DEV_DIST` set to `h.options.DistDir`, the dist snapshot captured at that compile. Everything the old process received since then is gone: every fast-path `Install` (goloader objects are loaded at runtime only; the generated `main` does not replay them) and every `Reload(dist)`. But the `Host` struct still carries `h.installed = <last installed files>` and the session still carries `goodInputs/goodRevision/goodDist` for the newer revision. Consequences:

- The visitor is served an application older than the one that was "preserved". The log line "restored previous application" is not true in the sense a developer will read it.
- On the next edit, `Prepare` diffs `next.Files()` against `h.installed` (`host.go:486`). A file that the restarted process never saw but that equals `installed[name]` is skipped as already present. The reverted package is never reinstalled until something forces a full rebuild. The dev server reports `published revision N` while the process serves revision N-k.
- The restarted process also serves the manifest/env from the older dist snapshot. Snapshots are content-hashed and never deleted (`dev.js:175-180`), so this does not crash, but route membership and `env.json` are stale, and a later fast path only calls `Reload` when `snapshot.Dist != goodDist`, which is false, so the stale dist persists.

**Reproduction (extends the existing tagged test):** after the "return 10" repair rebuild, write `sessionRoute("9609", 9)` and wait for `literal-9609:9` (fast path, same PID). Then write the `rejectStartup` source. After "restored previous application", `GET /orders` returns `literal-8508:9`, not `literal-9609:9`. Then write `sessionRoute("9609", 9)` to another route file or re-save it unchanged: `Prepare` sees no delta and the body stays at 8508. The current test cannot observe this because no fast-path install occurs between the last full rebuild and the rejected one, and the value it asserts after recovery (`literal-8508:9`) is the one baked into the binary.

**Impact:** The issue asked for "an explicit recovery outcome" for initialization failure after handoff. The delivered outcome silently diverges the served application from the reported revision, which is the class of failure #340 complains about (a "successful" publication that is not what it claims). The existing crash-recovery path (`case <-active.Done()`, `session.go:239-258`) already does this correctly by calling `makeHost(goodInputs, goodDist)`. The handoff-failure path should either do the same, or reset `installed` and `DistDir` and replay, and the integration test needs a fast-path install between the last rebuild and the rejected candidate so the assertion can tell a restarted binary from a restored application.

### 2. Issue: `Stop` is `SIGKILL`; the consumer's shutdown path still never runs

**Where:** `internal/hotreload/host.go:367-372`; generated `main` defers (`nativeClose`, `listener.Close`, `server.Close`) at `host.go:641-643`.

The issue comment states that `Host.Close` "bypasses the consumer's graceful shutdown path", and the issue body names "a fixed socket, a database/process lock" as the exclusive resources in scope. `Stop` keeps `Process.Kill()`. A kernel-held `flock` is released on kill, which is what the fixture proves, but a lock implemented as a lock file with PID content, a database that needs a clean close, or an app-owned fixed Unix socket leaves stale artifacts that the replacement constructor must now handle on its own. The authorized change ("stop and wait") is satisfied by kill-and-wait, so this may be a deliberate scope choice, but the `Stop` doc comment ("waits for process exit, releasing application-owned OS resources") overstates what it does, and no test or worklog note records that graceful shutdown was considered and declined.

### 3. Issue: the only behavioral proof is a build-tag-gated test that `just test` and CI never run

**Where:** `internal/devserver/session_integration_test.go:1` (`//go:build skgo_dev_integration`); `Justfile:64` (`vp exec go ... test ... ./...` without tags); `.github/workflows/ci.yml:84-85`.

`CompileHost`, `Start`, `Stop`, and restart-after-stop are new public surface in `internal/hotreload` with no package-level test. All regression coverage for the ordering fix and the recovery path lives in one opt-in integration test. This is the pre-existing pattern for this file, but the new path is exactly the one CLAUDE.md warns about: a green `just test` says nothing about it, and a future refactor of `Start`/`Stop` (for example, someone making `Stop` idempotent by reusing `closeOnce`) would break restart silently. At minimum, a hotreload-level test that compiles a trivial host, stops it, restarts it and observes the second PID would run under `just test`.

### 4. Nitpick: `Commit` now holds the project mutex across process teardown and constructor startup

**Where:** `internal/gen/retained.go:577-585` (`p.mu` held across `admit`); `internal/devserver/session.go:424-433`.

Before this change the admit callback was a symlink rename plus `setApp`. Now it spans `Kill`, `Wait`, `exec`, and the application constructor (for Gimbal, a catalog restore). The watcher goroutine's `Observe` calls (`retained_watch.go:200,239,290`) and `project.Revision()` block for that duration. fsnotify queues events so nothing is lost, and the session loop is single-threaded anyway, but a slow constructor now stalls filesystem observation and any other `p.mu` reader. Worth a comment at the call site, or moving `Stop`/`Start` outside the lock with a revision re-check, if this ever shows up as latency.

### 5. Nitpick: the restored-host test assertion cannot distinguish outcomes

**Where:** `internal/devserver/session_integration_test.go:396-403`.

After "restored previous application" the test checks `literal-8508:9`, which is both the compiled-in value and the last installed value, and the log substring, which is written unconditionally on the success branch. It would pass against a recovery that restarted any binary serving that route. See finding 1 for the state it should anchor on instead (a fast-path value installed after the last full rebuild, plus the PID having changed from the pre-stop PID).

## Outcome

`material findings remain`
