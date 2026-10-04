# Core matrix follow-ups — adversarial review, round 04

## Target and authority

Reviewed the complete relevant implementation from v0.16.0 (`6b74b0719c7f7fa542b76d36d4378fc891925179`) through `a7c2e1fae7d9eb83929de0a0dc14e45c876d4d47` in `skgo-core-followups`. Target reads began after the caller's implementation-ready message. The tree was clean and remained at that implementation throughout review. Earlier review rounds remain immutable; the reviewer changed no implementation, integration dependency/generated state or integration listener.

Authority remains repository `AGENTS.md`, `/agent-protocol`, `/adversarial-review`, `ephemeral/sveltekit-current/SKILL.md`, the issue bodies/comments independently read in round one for [skgo #223](https://github.com/tylergannon/skgo/issues/223) and [skgo-project #105](https://github.com/tylergannon/skgo-project/issues/105)/[#106](https://github.com/tylergannon/skgo-project/issues/106)/[#107](https://github.com/tylergannon/skgo-project/issues/107), and caller-restated original requirements. These preserve Go-owned HTTP and Goja SSR/native private fields; require literal universal-load correctness with a separately retained, independently called query; independent source-edit execution after regular failure with a complete native JSON report and aggregate failure status; a real failed prerender build naming the authored Go load; and nonzero unexpected-child exits with clean cancellation and owned-group cleanup. The authorized scaffold addition requires project-local native `vp check --fix` after writes and propagation of residual failure. No expected verdict or excluded defect area limited review.

## Evidence inspected and executed

- Read the full new runner/process repair diff and surrounding implementation/tests, and rechecked unchanged generator, adapter, query/component, native source and proof from the earlier rounds. Installed Kit remains **3.0.0**; installed Playwright is **1.63.0**. There is no dependency-pin change or replacement of Go HTTP, the Goja pool or native private fields.
- Read Playwright's native blob serialization, `createMergedReport`, `mergeReports`, `createReporters`, config reporter composition and output-file resolution. The new Go runner delegates serialization/merging to Playwright, passes the caller config to the final merge, and retains blob resources needed by native attachment paths. No skgo JavaScript harness was added.
- Independently ran the entire current `cmd/skgo-e2e` Go package: all tests passed, including native env destinations, regular/source/merge error aggregation, cancellation, missing blobs and config-option forms. These tests mock native invocations, so the real fixtures below remain essential.
- Independently compiled the current runner and reproduced the original round-three configured-reporter trigger in `/var/folders/lt/09rsy64x65s_0fp2b8zq3n7m0000gn/T/skgo-core-review-round04-json-stoyno4_`. With a config-owned JSON `outputFile` and no env/CLI reporter override, both regular failures and the passing source-edit result are retained: **unexpected=2, expected=1, skipped=0**, aggregate exit **1**. The fixture owns copied Playwright packages and runs literal assertions without a browser/server.
- Inspected actual new native receipts under `/tmp/core-followup-logs/native-report-merge/`, also retained in `task-8/core-followup-logs/native-report-merge/`. Config-owned JSON failure preserves both regular failures plus source-edit; positive FILE, DIR+NAME, absolute NAME, attached-config NAME and configured HTML+JSON cases preserve all **3** results with zero skips/unexpected results. The actual `success-fixture/logs/cli-stdout-json.stdout` parses as one JSON document with 3 expected results. For the configured failure report, both native error-context resource paths exist, and literal attachments remain inline. Earlier combined diagnostic logs are not treated as standalone JSON proof.
- Independently reran the original exited-leader reproduction using private copies of the repaired `proc.go`/`proc_unix.go` in `/var/folders/lt/09rsy64x65s_0fp2b8zq3n7m0000gn/T/skgo-core-review-round04-groups-3nfkmcph`. Before `Stop`, descendant **24260**, PPID 1, PGID **24259**, was sleeping after leader exit 7. After `Stop`, exact-PID inspection found it gone; the reproduction now passes. Its actual output is `actual-result.log`.
- Independently ran all three new targeted process tests with approved random loopback binds and OS inspection. All pass without skips. Exited-leader cleanup removes descendant **26722** and closes **127.0.0.1:63646**; the TERM-ignoring case remains live during grace then removes **26728** and closes **63649**, with concurrent Stops returning after approximately 814 ms. Inherited-pipe cases preserve exit 7 and the output marker, honor default **250 ms** and caller **600 ms** WaitDelay, then remove descendants **26749/26760**, their listeners **63653/63656**, and their groups. Each test owns and cleans only its recorded group.
- Current raw canonical `just test` and `just vet` logs are `task-8/core-followup-logs/final-local/round04-just-test.log` and `round04-just-vet.log`; they pass with no failure/skip lines. Current full tests include the real failed-prerender build and scaffold orchestration cases. Their earlier independent execution remains valid for unchanged code: renamed `ui/` failed-build fixture asserts `/about`, authored root-layout source and literal error; intentional HTTPError/redirect payloads are preserved. Scaffold before/after, authored negative and restored native checks remain unchanged and were rechecked in their retained receipts.
- Full normal dev/prod browser reports remain **174 passed each, zero skipped/unexpected/flaky/errors**, with 134 chromium, 29 noscript and 11 source-edit results. Stable-native query receipts still show the controlled identical double-GET counterexample with literal `Widget 42` in both consumers; they establish neither natural GC frequency nor the original CI cause. The component still independently calls `getItem(params.id)` while only the retained route keeps its separate proxy. The caller's prior production UI inspection is recorded; this reviewer did not start another live application. Full browser reports predate the newest reporting/process repairs; current native/unit executions cover those changed paths.

## Previous finding dispositions

The original startup-return leak, env FILE/DIR/NAME loss and attached-short-config destination defect remain repaired. Both round-three findings are also repaired for their exact reported triggers: config-owned JSON now receives the combined native report, and the owned process group is stopped after leader exit, including TERM-resistant descendants and inherited pipes. These dispositions are based on real executions above. Two additional native reporter input combinations remain incorrect.

## Findings

### 1. Issue — `--add-reporter=json` never produces the combined JSON document

**Locations:** `cmd/skgo-e2e/main.go:126-131`, `cmd/skgo-e2e/main.go:146-150`, `reporterSelection` at lines 314-339 and `withoutReporterSelection` at lines 342-361.

Playwright 1.63.0 declares `--add-reporter <reporter>` in `lib/program.js:189` and appends it to the selected/configured reporters in `lib/common/index.js:522`. The runner recognizes only replacement `--reporter` forms. Consequently, it leaves an added JSON reporter on each stage beside its owned blob reporter, then omits that addition from the final merge. Each stage writes its own JSON to stderr because stage stdout is redirected there; the final stdout contains only the config's list report. The requested complete native JSON document is absent. Regular failure aggregation and source-edit execution still work, but the report contract fails.

**Independent reproduction:** the private JSON fixture above uses three projects, one literal assertion per project and `reporter: 'list'`. All JSON destination env variables are unset. Ran the native CLI and final runner, respectively:

```text
FAIL_REGULAR=1 ./node_modules/.bin/playwright test \
  -c<fixture>/external-config/playwright.config.js --add-reporter=json
FAIL_REGULAR=1 ./skgo-e2e \
  -c<fixture>/external-config/playwright.config.js --add-reporter=json
```

Native Playwright exits **1**; its stdout contains list output and **one complete JSON document** with **expected=1, unexpected=2, skipped=0**, representing both failed regular projects and the passing source-edit project. Its stderr contains no JSON document. Actual outputs: `logs/native-add-json-failure.stdout`, `.stderr`, `.exit`.

The runner also exits **1**, but stdout contains **no JSON document**. Stderr contains **two separate documents**, one with **unexpected=2, expected=0** and one with **expected=1, unexpected=0**, interleaved with stage diagnostics. The final merge never emits their combined JSON. Actual outputs: `logs/runner-add-json-failure.stdout`, `.stderr`, `.exit`. The passing variant also returns 0 without the requested combined JSON (`native-add-json.*` / `runner-add-json.*`). Existing tests do not exercise Playwright's added-reporter option.

### 2. Issue — env NAME overrides config `outputFile` contrary to native precedence

**Locations:** `cmd/skgo-e2e/main.go:72-93` and final reporter override at lines 146-150; `resolveJSONOutputPath` at lines 208-249 considers only environment values.

The installed native `resolveOutputFile` at `lib/runner/index.js:1534-1552` uses this order: nonempty **OUTPUT_FILE** resolved from cwd; then reporter **options.outputFile** resolved from configDir; only when neither supplies a file does it resolve **OUTPUT_DIR** (then configured/default directory, then configDir) and **OUTPUT_NAME** (then configured/default filename). Thus a JSON reporter's configured `outputFile` takes priority over env OUTPUT_NAME and OUTPUT_DIR. Native merge preserves that same reporter option when it loads the caller config.

The runner instead treats any env NAME as a reason to force `--reporter json` on the merge, even when the caller config already declares JSON. That CLI replacement discards configured reporter options, so the output moves to the env-derived filename. This loses the native destination contract and can leave the configured report from an older run in place while returning success. The existing FILE/DIR tests do not cover a configured file combined with lower-priority env NAME.

**Independent reproduction:** in the same private fixture, the config declares:

```text
reporter: [['json', { outputFile: 'configured-native.json' }]]
```

With OUTPUT_FILE and OUTPUT_DIR unset, no CLI reporter override, and ordinary passing assertions, ran:

```text
PLAYWRIGHT_JSON_OUTPUT_NAME=env-name.json \
  ./node_modules/.bin/playwright test -c<fixture>/external-config/playwright.config.js
PLAYWRIGHT_JSON_OUTPUT_NAME=env-name.json \
  ./skgo-e2e -c<fixture>/external-config/playwright.config.js
```

Both return **0** with **3 expected, 0 skipped/unexpected/flaky** results. Native writes **`external-config/configured-native.json`**, honoring its configured option; its actual document/output/exit are retained in `logs/native-env-name-options.json`, `.stdout`, `.stderr`, `.exit`. The runner writes **`external-config/env-name.json`**, having discarded that option; its actual document/output/exit are `logs/runner-env-name-options.json`, `.stdout`, `.stderr`, `.exit`. Result completeness is now correct, but the native destination is still wrong for this supported combination. An explicit caller `--reporter=json` would intentionally replace config options; this reproduction provides no such override.

## Assessment and limits

No additional material flaw was found in query literal correctness, retained independent-consumer sharing, authored failed-build diagnostics, scaffold finishing, unexpected-child status handling or repaired process-group cleanup. The new native blob merge fixes the original configured-report loss and preserves attachment resources. Passing full Go/browser evidence and the targeted repair proof do not exercise the two remaining reporter input combinations above. The review makes no release, main-CI or publication claim. No product repair or tag was performed by the reviewer.

**Outcome: material findings remain.**
