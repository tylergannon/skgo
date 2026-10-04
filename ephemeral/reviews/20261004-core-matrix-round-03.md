# Core matrix follow-ups — adversarial review, round 03

## Target and authority

Reviewed the complete relevant implementation from v0.16.0 (`6b74b0719c7f7fa542b76d36d4378fc891925179`) through `8f8df7cfe6b8751ecd8760a285dad236393901da` in `skgo-core-followups`, including the attached-config repair and newly authorized stock-scaffold finishing step. The caller's later freshness worklog edit changes no implementation. Earlier reviews remain immutable. The reviewer changed no implementation, integration dependencies, generated application state or integration listener.

Authority remains repository `AGENTS.md`, `/agent-protocol`, `/adversarial-review`, `ephemeral/sveltekit-current/SKILL.md`, issue bodies/comments independently read in round one for [skgo #223](https://github.com/tylergannon/skgo/issues/223) and [skgo-project #105](https://github.com/tylergannon/skgo-project/issues/105)/[#106](https://github.com/tylergannon/skgo-project/issues/106)/[#107](https://github.com/tylergannon/skgo-project/issues/107), and the caller-restated original requirements. These require Go-owned HTTP and Goja SSR with native private fields; literal universal-load correctness plus a separately retained, independently called query; independent source-edit execution after regular failure with one complete native JSON document and aggregate failure status; a real failed prerender build naming the authored Go load; and unexpected-child failure with clean external cancellation and owned-process cleanup. The additional scaffold authorization requires project-local native `vp check --fix` after generator writes, without suppressing residual failures. No expected verdict or excluded defect area limited review.

## Evidence inspected and executed

- Read the final change and surrounding generator, adapter, supervisor, process-group, runner/report, example query and browser-step code. Rechecked installed Kit **3.0.0** cache/proxy and prerender sources and Playwright **1.63.0** config/output resolution and CLI declarations. The diff changes no dependency pin, Go HTTP ownership, Goja engine or native private-field handling.
- Stable-native receipts in `task-8/native-query-stable/receipts/` still establish the controlled counterexample: ordinary navigation makes one GET; forced post-load collection makes two identical GETs with both consumers showing literal `Widget 42`, in dev and production preview. This does not identify the original CI trigger or establish natural GC frequency. The final component independently calls `getItem(params.id)` while the retained route returns its own proxy, preserving the distinct cache-sharing claim.
- Independently ran the entire final `cmd/skgo-e2e` Go test package: all tests passed. This includes the attached short config case, native env destinations, stale-output protection, regular/source failure aggregation, interruption and stage isolation.
- Independently compiled the final runner and compared it with native Playwright in the private fixture `/var/folders/lt/09rsy64x65s_0fp2b8zq3n7m0000gn/T/skgo-core-review-round03-json-vic0crcs`. With attached `-c<external-config>/playwright.config.js` and a relative JSON NAME, native Playwright returned 0 with one result at the config-relative destination; the runner returned 0 with all three projects there, zero skipped/unexpected results and no misplaced working-directory report. This independently confirms the round-two repair. The fixture owns copied Playwright dependencies and uses literal assertions without browser/server sockets.
- Read the caller's attached-config native comparison receipts under `/tmp/skgo-json-attached-short-c-20261004/`, and the earlier native FILE/DIR/absolute-NAME positive and regular-failure reports under `/tmp/skgo-json-native-proof-20261004/`. The latter failure report preserves two failed regular results and a passing source-edit result, with aggregate exit 1. The distinct configured-reporter failure reproduced below is not exercised by these checks.
- Read the unmodified v0.16.0 stock scaffold receipts at `task-7/skgo-build-proof/ephemeral/receipts/v0.16.0-fresh-build-lint/`: 11 format failures and four native `prefer-vite-plus-imports` errors. Read the new raw create/check/format/lint/build and client/server Vitest outputs, generated config/package files, authored negative fixtures and restored checks under `task-8/core-followup-logs/scaffold-checks/newapp-viteplus-finishing-20261004/`. Creation invokes the repair after Go generation and completes its initial build; standalone checks report all 18 matched files formatted and zero lint/type errors, retaining two warnings. The generated config still enables the original native import rule and type-aware/type checks. The controlled format and `no-debugger` fixtures produce their intended diagnostics; restored checks pass. The denied combined browser startup is preserved separately, and the client/server project runs actually pass.
- Read installed VitePlus **1.0.0** documentation for `check --fix`, including its format/autofix behavior and preservation of configured checks. Independently ran `TestCreateWithoutATerminalSettlesTheMinimalTypeScriptApplication` and `TestCreateSurfacesVitePlusFrontendCheckFailure`: both pass. Their assertions cover project-local command placement after Go generation and failure propagation before the initial build. The initial sandbox run explicitly failed on the httptest registry's denied random loopback bind; the approved run completed without skips. No application listener was started.
- Prior independent execution of the named real failed-prerender test and root prerender protocol tests remains valid for unchanged code. The test actually fails a frontend build, checks authored `src/routes/layout.server.go`, `/about` and the literal error, and renames the frontend to `ui/`. Intentional HTTPError and redirect payloads remain distinct from unexpected diagnostic failures.
- Rechecked full production/development JSON reports: each contains **174 passed, 0 skipped, 0 unexpected, 0 flaky, 0 top-level errors**, with 134 chromium, 29 noscript and 11 source-edit results. The preserved deliberate regular browser failure still runs source-edit and retains its failure. These full reports predate the final CLI/scaffold repairs; targeted native/unit proof covers those changes. The caller's recorded production UI inspection showed both item consumers rendering `Widget 42`; this reviewer did not open a new live application.
- Read final canonical logs `/tmp/skgo-core-final-repairs-restored-just-test.log`, `/tmp/skgo-core-final-repairs-build.log` and `/tmp/skgo-core-final-repairs-vet.log`: the intended fresh build, full Go suite and vet pass. The preceding retained failure names a frontend stub newer than its embedded build; the caller rebuilt instead of weakening that freshness assertion. Prior round-two independent supervisor tests and no-Stop negative control remain valid for unchanged cleanup code. The separate descendant fixture below tests an uncovered process-group case.

## Previous finding dispositions

1. **Startup return skips cleanup — resolved for the reported trigger.** The deferred cancellation and `Stop` cover every later return after Vite starts. The independent startup test and no-Stop negative control in round two detect the original leak. This does not establish descendant cleanup after a leader has already exited.
2. **Native env destination loss — resolved for the reported FILE/DIR/NAME triggers.** Absolute per-stage FILE values isolate reports, and the combiner preserves both stages. The configured-reporter case below is a separate unsupported native input.
3. **Attached short config destination — resolved.** The final helper handles `-c<path>` consistently with native Playwright. Both the new Go test and this round's independent native comparison pass.

## Findings

### 1. Issue — a JSON reporter configured in Playwright still loses regular results

**Locations:** `cmd/skgo-e2e/main.go:62-79`, `cmd/skgo-e2e/main.go:99-108`, and `hasJSONReporter` at line 296.

The runner discovers JSON only from environment destinations or an explicit reporter CLI argument. A normal Playwright config can itself declare `reporter: [['json', { outputFile: 'configured-native.json' }]]`. Without an env/CLI override, `jsonPath` stays empty: neither stage receives an isolated FILE, and no combination occurs. Native Playwright resolves the configured file under the config directory, so source-edit overwrites the completed regular report. Aggregate process failure remains correct, but the requested native document omits every regular failure and misleadingly contains only a passing source-edit run. This violates report completeness; it also occurs with successful regular results.

**Independent reproduction:** the private fixture above declares the three normal project names and one literal assertion. `FAIL_REGULAR=1` makes only chromium/noscript expect `Widget 42` but receive `wrong`. Its config uses the JSON reporter declaration above. From the fixture directory, with all three JSON destination environment variables unset:

```text
FAIL_REGULAR=1 ./node_modules/.bin/playwright test \
  -c<fixture>/external-config/playwright.config.js \
  --project=chromium --project=noscript
```

Native Playwright returns **1** and writes a document containing both failed regular results: **unexpected=2, expected=0, skipped=0**. The actual document is copied to `logs/native-configured.json`; command output and exit are retained beside it.

```text
FAIL_REGULAR=1 ./skgo-e2e -c<fixture>/external-config/playwright.config.js
```

The final runner returns **1**. `logs/runner-configured.log` proves both regular assertions failed, then source-edit ran and passed, then the runner reported `regular Playwright stage failed: exit status 1`. However, the requested `external-config/configured-native.json`, copied to `logs/runner-configured.json`, contains only **source-edit passed**: **expected=1, unexpected=0, skipped=0**. The regular failures were overwritten. Existing Go tests and env-based native proof do not cover a config-owned reporter destination. The normal matrix using explicit JSON env destinations is unaffected by this trigger.

### 2. Issue — an exited process leader causes owned descendants to escape cleanup

**Locations:** `internal/dev/proc.go:64-76`, used by the new deferred cleanup at `cmd/skgo/dev.go:136-143` after the unexpected-exit watcher at lines 123-134.

`Process.Stop` treats the leader's `Done` state as proof that its whole process group is gone. If a launcher spawns a worker and exits while that worker continues, `p.exited()` is true and `Stop` returns before signalling the group. The worker remains orphaned in the group that skgo explicitly created and owns. The new supervisor correctly returns nonzero for that leader exit, but its deferred cleanup calls this no-op `Stop`, so it does not fulfill the process-group ownership contract. The same assumption at the later `case <-p.done` can also end the grace period based only on the leader. This helper predates the patch; the finding is material to the requested supervisor cleanup behavior.

**Independent reproduction:** `/var/folders/lt/09rsy64x65s_0fp2b8zq3n7m0000gn/T/skgo-core-review-round03-groups-3gtgdjgv` contains byte copies of the final `proc.go`/`proc_unix.go` and a disposable Go test. It invokes the real `StartProcess` on its own shell launcher, which starts `sleep 600` with detached stdio, records the worker PID, and exits 7. After `Done`, the test verifies the worker's PGID matches the exact owned leader, invokes `Stop`, and checks the live process again. It binds no port. Actual approved output in `actual-result-approved.log` contains:

```text
before Stop: leader=68591 leaderExit=exit status 7 descendant=68592  1 68591 S  sleep 600
after Stop: 68592  1 68591 S  sleep 600
owned descendant 68592 remains alive after Stop
```

The test fails on a sleeping live process, not a zombie or a broken-output artifact. Its deferred cleanup killed only recorded group 68591; subsequent exact-PID inspection found both 68591 and 68592 gone. The earlier sandbox attempt failed on denied `ps` execution and is retained separately; it is not the defect evidence. Existing supervisor tests exercise a single live leader and therefore do not expose this surviving-descendant trigger. This round directly reproduces the cleanup helper behavior rather than launching an additional full supervisor fixture.

## Assessment and limits

No additional material flaw was found in the literal query correction, retained independent-consumer assertion, authored failed-build diagnostic, attached-config repair, or stock-scaffold finishing change. The scaffold repair preserves native rules and propagates residual command failure. Normal source-edit sequencing and aggregate exit behavior are demonstrated, and the original startup leak is repaired. The two uncovered cases above still lose the caller's failure report or an owned process-group member. Full passing suites do not exercise them. No implementation repair, publication or tag was performed by the reviewer.

**Outcome: material findings remain.**
