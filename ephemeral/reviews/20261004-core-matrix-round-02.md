# Core matrix follow-ups — adversarial review, round 02

## Target and authority

Reviewed implementation at `fea66b23aa5dee4b621640177b57e0a6ac81525a` in `skgo-core-followups`, including the complete relevant change from v0.16.0 (`6b74b0719c7f7fa542b76d36d4378fc891925179`) and both round-one repairs. The only later working-tree change observed was the caller's worklog edit. The reviewer changed no implementation, generated application state, integration listener or earlier review artifact.

Authority remains repository `AGENTS.md`, `/agent-protocol`, `/adversarial-review`, the installed-source instruction in `ephemeral/sveltekit-current/SKILL.md`, live issue content read in round one for [skgo #223](https://github.com/tylergannon/skgo/issues/223) and [skgo-project #105](https://github.com/tylergannon/skgo-project/issues/105)/[#106](https://github.com/tylergannon/skgo-project/issues/106)/[#107](https://github.com/tylergannon/skgo-project/issues/107), and the caller-restated requirements. These require Go-owned HTTP and Goja SSR/native private fields; literal universal-load correctness with a separate retained-proxy independent query call; independent source-edit execution after regular failure and a complete native JSON report at the caller's destination; an actual failed prerender build naming the authored Go load; and correct unexpected-exit, cancellation and owned-child cleanup behavior. No assumed verdict or safe area limited the review.

## Evidence inspected and executed

- Read the full repair diff and surrounding supervisor/process-group code, runner/report combination logic and tests; rechecked the remaining generator, adapter, generated stubs, query fixtures, browser steps and canonical entrypoints. The installed Kit package remains **3.0.0**. The native cache and prerender sources and stable-query receipts inspected in round one remain the specification and evidence for unchanged code.
- Read installed Playwright **1.63.0** `resolveConfigLocation`, `resolveOutputFile` and CLI config-option declarations. Its relative JSON `OUTPUT_NAME` default is the resolved config directory; its native CLI accepts an attached short `-c<path>` value, as independently exercised below.
- Independently ran the entire current `cmd/skgo-e2e` Go test package: all tests passed, including FILE precedence, DIR+NAME, config-directory relative NAME, stale-output protection, aggregate failure, cancellation and independent-stage checks.
- Independently ran the three current targeted supervisor tests with the pinned Staticcheck on PATH: unexpected Vite exit 0/7, clean external cancellation, and public-listener startup failure. All passed. Initial sandboxed execution failed explicitly on denied loopback binds; the coordinated run with approved escalation completed without skips. Cancellation observed supervisor exit 0, PID/group **37356** absent and listener **54689** closed; startup failure observed PID/group **37371** absent and listener **54692** closed.
- Independently ran the retained disposable `/tmp/skgo-vite-negative-control` startup test with `Stop` disabled. It failed as intended on PID/group **38998 alive=true** and listener **54775 open=true**, before test cleanup. Subsequent inspection found neither that exact process nor its listener. This establishes that the new cleanup assertion detects the removed behavior rather than passing from a broken-stdout child exit.
- Built the current runner and ran both original JSON reproductions against privately copied Playwright packages in `/var/folders/lt/09rsy64x65s_0fp2b8zq3n7m0000gn/T/skgo-core-review-round02-json-rw4f5h_y`. FILE and DIR+NAME each returned **0** with a combined document containing all **3** literal-fixture results, with no skips or unexpected results. These fixtures use no browser/server and bind no ports.
- Inspected `/tmp/skgo-json-native-proof-20261004/` actual native positive reports for FILE, DIR+NAME and absolute NAME: 3 passed each. Its `regular-failure.json` preserves both failing regular projects and the passing source-edit project (**unexpected=2, expected=1, skipped=0**); the captured failure log ends with `regular Playwright stage failed: exit status 1` after source-edit passed. These are independent native reporter documents, not reconstructed claims.
- Retained full normal browser proof: `/tmp/skgo-core-prod-playwright.json` and `/tmp/skgo-core-dev-playwright.json` each contain **174 passed, 0 skipped, 0 unexpected, 0 flaky** (134 chromium, 29 noscript, 11 source-edit). The named real failed-prerender test and root prerender protocol tests were independently executed in round one and remain unchanged. The restored full `just test` and `vet` logs remain available, as does the authored prerender before/after build diagnostic. The full normal suites predate the final CLI-only repairs; the targeted executions above validate those repairs.

## Round-one finding dispositions

1. **Startup child leak — resolved.** `cmd/skgo/dev.go:136-143` defers context cancellation and owned-group `Stop` immediately after successful Vite startup. It now covers the public-listener failure return as well as ordinary shutdown. Current tests check PID, group and listener disappearance; the independent no-Stop negative control fails on all three observations.
2. **FILE and DIR+NAME JSON loss — resolved for the reported triggers.** `resolveJSONOutputPath` selects the native destination and each stage receives a distinct absolute `PLAYWRIGHT_JSON_OUTPUT_FILE` with inherited DIR/NAME removed. Both original reproductions now preserve the complete report and return success. The remaining config-option variant below is a distinct defect in that new resolution helper.

## Findings

### 1. Issue — an attached short config option silently changes the JSON destination

**Locations:** `cmd/skgo-e2e/main.go:231-249`, especially option parsing at lines 235-245; its result is used by `resolveJSONOutputPath` for a relative `PLAYWRIGHT_JSON_OUTPUT_NAME`.

Native Playwright accepts both `-c /path/playwright.config.js` and `-c/path/playwright.config.js`. The helper recognizes the separated form and `-c=...`, but ignores the attached native form. Playwright still loads the requested external config because the runner forwards that argument unchanged. Meanwhile, the helper falls back to the working directory and forces both stage reports and the final combined report there. The runner returns **0**, but the report is absent from the destination that the same native invocation selects. A caller collecting the config-relative report loses its artifact despite a successful command.

**Independent reproduction:** fixture `/var/folders/lt/09rsy64x65s_0fp2b8zq3n7m0000gn/T/skgo-core-review-round02-json-rw4f5h_y` owns copied `@playwright/test`, `playwright` and `playwright-core` packages and an external config under `external-config/playwright.config.js`. From the fixture working directory, ran the native CLI:

```text
PLAYWRIGHT_JSON_OUTPUT_NAME=shorthand-native.json \
  ./node_modules/.bin/playwright test \
  -c<fixture>/external-config/playwright.config.js \
  --project=chromium --reporter=json
```

It returned **0**, reported one passing literal fixture, and wrote `external-config/shorthand-native.json`; no `shorthand-native.json` appeared in the working directory. Captured output: `shorthand-native.log`.

Then ran the current compiled Go runner:

```text
PLAYWRIGHT_JSON_OUTPUT_NAME=shorthand-runner.json \
  ./skgo-e2e -c<fixture>/external-config/playwright.config.js --reporter=json
```

It returned **0** and preserved all three passing project results, but wrote `<fixture>/shorthand-runner.json`. The requested native destination `<fixture>/external-config/shorthand-runner.json` does not exist. Captured output: `shorthand-runner.log`. The existing Go test uses only a separated `--config` value, so it does not detect this mismatch. This is a report-path compatibility issue; no data loss occurs when reading the incorrectly placed document, and the normal matrix commands using absolute report paths are unaffected.

## Assessment and limits

No additional material flaw was found in the literal-value query correction, retained independent-consumer single-flight proof, real failed-build authored diagnostic, unexpected-Vite status handling, deferred child cleanup, or independent source-edit execution. The unchanged normal browser and Go evidence remains valid for those unchanged paths. The new config-path helper has the reproducible destination bug above. This review is not a claim that the branch has been released or that an unrelated pending scaffold change was reviewed. No implementation repairs were made by the reviewer.

**Outcome: material findings remain.**
