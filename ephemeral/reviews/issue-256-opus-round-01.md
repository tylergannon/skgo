# Adversarial review — issue #256, round 01 (Opus 5.5)

## Target

Uncommitted working tree on `codex/issue-256-prerender-service` (base `10ad4de`)
in `/Users/tyler/.codex/worktrees/595c/skgo`, against
<https://github.com/tylergannon/skgo/issues/256> ("prerendering through one Go
service") and the repository instructions in `CLAUDE.md`.

**The target changed while this review ran.** When the review started, `git status`
did not list `internal/adapter/prerender_inputs_build_test.go` or
`internal/gen/prerender_test.go`. Both were later modified, the lifecycle test diff
grew from 219 to 241 lines, `SHUTDOWN_GRACE` in `prerender.js` dropped from 2 500 ms
to 150 ms, and `ephemeral/adapter-lifecycle-debug.log` and
`ephemeral/generator-fixture-output.log` appeared. The findings below cite the
completed `ephemeral/test-output-fixed.log` (15:13) and note where later edits
change the picture.

The caller's constraints were treated as operating limits only: write only this
artifact, reuse captured builds, run no extra full suite. Nothing narrowed the
review's subject matter.

## Evidence inspected

- Issue #256 body: the ratified plan, its exclusions, the lifecycle-test disposition table and the acceptance paragraph. The issue has no comments.
- `CLAUDE.md` (contracts are Go tests at the real handler; never derive expectations from the thing tested; a passing command is not evidence; skips are failures).
- The full diff: `prerender_service.go`, `prerender_control_{unix,windows}.go`, `remote_execution.go`, `prerender_remote.go`, `prerender_load.go`, `remote.go`, `internal/gen/prerender_command.go`, `internal/adapter/skgo-adapter.js`, `internal/adapter/skgo-adapter/prerender.js` (all 633 lines), the regenerated example files, and the changed or new tests (`prerender_service_test.go`, `prerender_service_timeout_test.go`, `prerender_inputs_lifecycle_test.go`, `prerender_*_test.go`, `kit_queue_compatibility_test.go`, `ownership_test.go`).
- Pinned Kit 3.0.0: `src/exports/vite/build/index.js:131` (`loadEnv` in the `config` hook), `src/utils/fork.js:56` (the worker gets `{...process.env}` when it is constructed), and `plugins/env-vars.js:68`. The secret is published after Kit's env snapshot and before workers start, as the plan requires.
- `ephemeral/build-output{,-fixed}.log`: one `skgo prerender service started (pid 4687)` / `stopped (pid 4687)` pair per build. `ephemeral/server-output.log`, `vet-output{,-fixed}.log`, `test-output.log` (interrupted) and `test-output-fixed.log` (complete, **FAIL**).
- `ephemeral/worklog/prerender-service.md` and `ephemeral/reviews/issue-256-test-validation.md`.
- Main's CI history (`gh run list --branch main`): CI on `10ad4de` fails, but only in `internal/gen`, with the same `skgo.Load` signature failures shown in `test-output-fixed.log`. Those failures are pre-existing. `internal/adapter` is green on main.
- `git grep`: none of `RunPrerenderBuild`, `RunPrerenderLoad`, `workerRequest`, `joinFailedPrerenderOwner` or `ensureCompiled` remain. The old invocation machinery is gone.

## Findings

### 1. critical — The full repository run regresses five `internal/adapter` tests that pass on main, and the current tree has no passing full run

*Requirement not met.* Issue: "Run the repository checks appropriate to the affected behavior once at the stable implementation… Reuse existing tests where they assert a valid promise." CLAUDE.md: "`just test` green in seconds… A passing command is not evidence on its own."

`ephemeral/test-output-fixed.log` ends with `FAIL github.com/tylergannon/skgo/internal/adapter`. These failures are not in main's CI failure list:

- **`TestPrerenderInputsMalformedGoSourceFailsAndCleansPrivateDirectory` (0.00 s), `config.go: file exists`.** The rewritten test (diff hunk at `prerender_inputs_lifecycle_test.go:133`) calls `prepareMinimalInputsApp`, which already writes `internal/skgo/config.go` and `web/src/routes/+page.svelte` (`prerender_inputs_minimal_build_test.go:70-71`). It then runs `os.CopyFS` over the same tree, and `CopyFS` refuses to overwrite. The test never reached the compiler. It was rewritten by this change and committed without ever passing. A later in-flight edit stages with `stageMinimalInputsBootstrap` instead.
- **`TestDeclaredPrerenderInputsBuildAndProduceNativeArtifacts`, "could not add a declared input for the duplicate page remote" (`prerender_inputs_build_test.go:183`).** The change rewrote `example/web/src/routes/about/about.remote.go` to `skgo.Prerender(buildReceipt, skgo.PrerenderOptions{Inputs: receiptNames})`. That test string-rewrites the old `var _ = skgo.Prerender(buildReceipt)`. The in-flight edit now searches for `…{Inputs: buildInputs}`, which still does not match `receiptNames`, so as of this review the test still fails the same way. Editing the shared example app to serve as proof, without updating its consumers, is the coupling #252 and #254 were written to remove.
- **`TestPrerenderInputsBuildAppWaitsForUnrelatedCrawlerFailureDrain`, `TestPrerenderInputsVPOwnerSignalDrainsBlockedProducer` and `TestPrerenderInputsOuterVPCLISignalDrainsBlockedProducer` failed with late descendant I/O and a retained private directory.** All three are tests the plan says to keep. The cause is in the implementation, not the tests:
  - On failure, `stopJob(service, true)` closed the control channel and waited `SHUTDOWN_GRACE` (then 2 500 ms).
  - Go's `servePrerenderService` waits up to 2 s in `server.Shutdown` for a handler that ignores cancellation (`prerender_service.go:71-75`).
  - So an uncooperative callback and its process group lived about 2 s past the build failure. That is long enough for the fixture's `sleep 2; printf late` descendant to write.
  - In the outer-launcher case, the harness's 2 s `WaitDelay` closed the owner's stdio pipes before cleanup reached `rmSync`. This is consistent with the private directory found remaining.

  The plan asked for "a short fixed grace period" before killing. A 2 s+ wait on the failure and interrupt paths is what the retained checks rejected.

**What the in-flight fix does.** It sets `SHUTDOWN_GRACE = 150`, which addresses the cause. In the same edit it deletes the `late-descendant-write` assertion and replaces the fixture's late writer with `while :; do sleep 1; done` (lifecycle test diff around old lines 218–227 and 320–321). That removes the one check that bounds how long owned work may run after failure. The remaining group-gone check passes for any eventual kill, including the 2 s one that just failed. Restoring the grace while weakening the test that caught it makes the test unable to catch the same regression next time.

`ephemeral/adapter-lifecycle-debug.log` shows only four targeted tests passing. No full run exists for the tree as it now stands.

**Impact.** The implementation has no passing evidence. The issue's acceptance criterion, "cancellation ends the owned work within its bound", is currently backed by a test that no longer measures any bound.

### 2. issue — A callback timeout throws inside Kit's worker but does not, by itself, fail the build

*Requirement misread.* Issue: "Every callback has a finite timeout. If it expires, abort that HTTP request and **fail the build** with the operation identity and timeout in the error… a timeout should cause the build to fail; no fancy recovery is needed."

`invokeService` (`prerender.js:475-502`) aborts the fetch and throws `skgo prerender … timed out after 30000ms` inside Kit's worker, and that is all. The build owner on the main thread is never told. Whether the build fails depends on how Kit and the app handle the error:

- `remoteFunction`'s timeout is an ordinary `Error` inside the application's render. A universal `+page.ts` that does `try { receipt = await buildReceipt('atlas') } catch { receipt = 'unavailable' }`, or an app with `prerender.handleHttpError: 'warn'`, writes the page and lets `buildApp` resolve.
- Meanwhile the stuck Go handler keeps running.
- At success cleanup, `stopJob` closes the control channel. Go's `Shutdown` times out and the helper exits through `log.Fatal` with status 1.
- `stopJob` and `cleanup` never inspect `job.exited`'s code (`prerender.js:198-214, 373-399`). The build then reports `skgo prerender service stopped` and `✔ done`.

The only timeout test, `internal/adapter/prerender_service_timeout_test.go`, calls `remoteInputs` against an `httptest` stand-in. It proves the error message and that the request context is cancelled. It never shows a build failing.

That test also waits the full hard-coded `CALLBACK_TIMEOUT` (30 s) of wall-clock time, because the constant cannot be overridden. A single assertion costs the adapter package 30 s, against the project's stated seconds-tier contract budget.

**Impact.** The one terminal-failure policy the user specified can be silently swallowed, and the evidence suite cannot detect that.

### 3. issue — The helper's `go build` now has a 60 s deadline, a new way for legitimate builds to fail

`prepare()` races the compiler against `STARTUP_TIMEOUT` (`prerender.js:291-298`) and fails with "skgo prerender compiler startup timed out". The old `ensureCompiled` had no deadline (`git show HEAD:…/prerender.js` around lines 360–370).

The plan's startup deadline exists "so a helper blocked **before readiness** cannot hang the build". It was about the service's readiness announcement, and the readiness wait (`prerender.js:340-344`) already covers that. The helper package depends on 307 packages, including goja, goja_nodejs, polytype and x/text (`go list -deps ./example/internal/skgo/prerender`). In a fresh CI container or a new worktree, `go build` may download modules and compile all of them cold. That can exceed 60 s, and it now runs serially before the frontend build (the `buildApp` pre hook).

**Impact.** A build that succeeded before can now fail nondeterministically depending on cache state. That is the opposite of the "reliable builds with predictable process lifetimes" thesis. Bounding a hung helper is correct; bounding the toolchain's legitimate cold compile is not what was asked for.

### 4. nitpick — Leftover no-op code and mixed style in `prerender.js`

- About ten `try { … } catch (error) { throw error; }` wrappers survive from the deleted relay, at `prerender.js:530-531, 540-542, 548, 593-603, 611, 614-615, 625-632`.
- `remoteInputs` re-checks `typeof answer.inputs` after `validateEnvelope` has already checked it (`:103-105` vs `:535`).
- `makeError` is a bare wrapper around `new Error`.
- The file switches from 2-space indentation with double quotes (lines 1–502) to tabs with single quotes (lines 504–633).

CLAUDE.md asks for code that reads like its surroundings. This is the replacement's core bridge file and should not carry the deleted design's residue.

### 5. nitpick — The helper's whole stderr is buffered in memory for the life of the build

`startJob` appends every stderr chunk to `job.stderr` (`prerender.js:244-247`) for every job, including the long-lived service, while also relaying it to `process.stderr`. Only the compiler's failure message reads `job.stderr`. The service's buffer grows without bound for the whole build if application code logs heavily during prerender.

## Outcome

material findings remain
