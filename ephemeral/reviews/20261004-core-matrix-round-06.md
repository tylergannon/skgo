# Core matrix follow-ups — adversarial review, round 06

## Target and authority

Reviewed the complete relevant work from released v0.16.0 (`6b74b0719c7f7fa542b76d36d4378fc891925179`) through clean implementation-ready target **`bf01f28f46a12a84dcaa6ca60d25b02a59e217e2`** in `/Users/tyler/Codex/2026-10-03/task-8/skgo-core-followups`. Earlier reviews remain immutable. The reviewer changed only this new artifact in the target; independent executions used owned private source/package copies and no application sockets. No target implementation, dependencies, generated state or integration listener was changed.

Authority remains `AGENTS.md`, `/agent-protocol`, `/adversarial-review`, `ephemeral/sveltekit-current/SKILL.md`, the original issue bodies/comments read in the first review for [skgo #223](https://github.com/tylergannon/skgo/issues/223) and [skgo-project #105](https://github.com/tylergannon/skgo-project/issues/105)/[#106](https://github.com/tylergannon/skgo-project/issues/106)/[#107](https://github.com/tylergannon/skgo-project/issues/107), and the caller-restated original requirements. These preserve Go HTTP ownership, pooled Goja SSR and native private fields; require ordinary universal-load literal correctness and separately retained independent-call slow single-flight; require source-edit after regular failure with complete native reporting and aggregate failure; require authored Go prerender diagnostics proved by a real failed build; and require nonzero unexpected Vite exits plus clean external cancellation and owned-child cleanup. The authorized stock-scaffold addition requires project-local native `vp check --fix` after all writes and propagation of residual failure.

The complete pinned reporter survey and explicitly selected single-multiplexer compatibility bounds also apply. They constrain implementation without excluding a defect area from review. The lifecycle correction specifically requires persistent native merge resources independent of removable stage scratch and final report folders, plus a byte-identical post-callback archive for the existing artifact collector. No expected verdict, excluded file or assumed-safe implementation area limited this review.

## Evidence inspected and executed

- Read the complete new retention implementation/tests and baseline-to-current relevant implementation, including the root Just/VP recipe, stage argument/parser/reporter composition, supervisor/process cleanup, authored prerender bridge and generated stubs, scaffold finishing, route/component and browser assertions. Rechecked unchanged source/proof from earlier rounds. Installed Kit remains **3.0.0** and Playwright **1.63.0**. The work retains Go-owned HTTP, pooled Goja and native private fields without broad dependency rework or an owned JavaScript tooling harness.
- The ordinary query route returns its resolved literal value; its retained variant keeps a separate proxy and the component independently calls `getItem(params.id)`. Slow single-flight and ordinary literal correctness remain separate assertions. Stable native receipts in `task-8/native-query-stable/receipts/` retain the controlled identical double-request cache counterexample and literal `Widget 42` in both consumers, without proving natural GC frequency or the original CI cause. Current issue-state worklog attribution remains accurate: site #105/#107 recovered on v0.16.0, rather than through this unreleased follow-up.
- Rechecked the real failed-prerender build test, including renamed `ui/`, literal authored error, `/about` and authored Go layout source, and unchanged successful/intentional HTTPError/redirect behavior. Rechecked scaffold command ordering and actual native before/after/negative/restored receipts under `task-8/core-followup-logs/scaffold-checks/newapp-viteplus-finishing-20261004/`. Missing required tools fail; these proofs do not silently skip.
- Rechecked unexpected exit 0/7, failure-before-cancellation ordering, cleanup on startup errors, clean external SIGINT, process-group cleanup after leader exit, TERM-resistant descendants and inherited-pipe WaitDelay. Their implementation is unchanged by the retention repair. Earlier independent PID/group/listener executions remain applicable; current canonical Go tests exercise them again. This reviewer did not bind ports or signal a different owner's process.
- Read installed native Playwright output resolution, one-multiplexer reporter composition, callback error/status handling, HTML/blob folder cleanup, merge resource extraction and attachment patching. Existing CLI/config/options/env corrections and explicit unsupported-input boundaries remain intact. Stage blobs stay isolated from final reporter env destinations. The post-report archive now copies native ZIPs, extracted JSONL and resource files without rewriting native JSON paths or reconstructing report contents.
- Independently compiled exact current source in **`/tmp/skgo-core-round06-review-_cyi9m8y/`**. Target and copied `main.go` both have SHA-256 **`05b516f73bc3f43cb3bd49edcbe8b2b03f1e04ef8c8544dc2048f7a3c2128da7`**. The entire copied runner Go test package passes with `-count=1 -v`, no skips; actual output is `runner-tests.log` with exit `0`. These orchestration tests mock native calls; the real controls below are independently necessary.
- Independently reran installed native and compiled-wrapper HTML+JSON success/failure and final-blob controls using privately copied Playwright packages and authored literal assertions/attachments, without a browser or server. Actual commands, observations, stdout, stderr, exits, JSON, final ZIPs and archived input copies are in `fixture/logs/` under that private root. Default-temp HTML+JSON success returns **0**, **3 passed**, zero skips/unexpected/flaky/global errors, and all three native JSON paths contain exact `file:Widget 42:<project>` bytes. Their archive is byte-identical to the persistent native input across **7 files**. The deliberate regular failure returns **1**, **unexpected=2, expected=1, skipped=0**, with source-edit passing; all three authored file bytes survive, and its **9-file** input/archive comparison passes. Inline bodies remain intact.
- Independently verified final blob now returns **0** with three actual file resources containing exact chromium/noscript/source-edit byte strings. `logs/wrapper-blob.zip` includes `resources/364e8f67a4323ba09241527e7bcfeea9a1f95248.txt` (chromium), `resources/729a6f796777c03c3608b650bc84c168ea0a82ce.txt` (noscript), `resources/5996a60e0b905ebc380e061b16b37297aed369e2.txt` (source-edit), and `report.jsonl`. The native control likewise has three resources. Thus both exact default-temp round-five triggers are repaired.
- Inspected producer native controls and retained HTML embedded ZIPs under `task-8/core-followup-logs/round06-retention/receipts/`, including `resource-check.txt`, `blob-resource-check.txt`, `html-failure-check.txt`, actual JSON/ZIPs and archive trees. They corroborate intact HTML/JSON/blob file and inline bytes, combined failure status and byte-identical persistent/archive trees. Read existing actual finite reporter controls in `reporter-contract-final/receipts/`; source worklog and N09/N10 controls were still pending at this checkpoint and are not counted as completed evidence.
- Read current canonical **`task-8/core-followup-logs/final-local/round06-just-test.{log,exit}`** and **`round06-just-vet.{log,exit}`**: both exit **0**. Full Go package results include `cmd/skgo`, `cmd/skgo-e2e`, `internal/dev`, `internal/gen`, `internal/newapp` and the example. No failure/skip lines appear; the sole source skip identified by the caller is Windows-only and inactive on this darwin/arm64 run. Existing application dev/prod reports retain **174 passed each**, zero skipped/unexpected/flaky/errors, but predate the newest reporter repairs. This reviewer did not rerun the live app or claim exact-head full browser/main qualification, merge readiness, publication or release.

## Previous finding disposition

The earlier supervisor, report destination/config/composition and exited-leader findings remain repaired for their reported triggers. The latest repair also fixes both round-five collisions under the ordinary OS temporary directory: HTML cleanup of cwd `test-results` no longer removes native input, final blob contains all three resource files, and both success and ordinary failure preserve native attachment paths and archive bytes. A remaining supported environment combination puts the supposedly independent input back inside that same removable hierarchy.

## Finding

### 1. Issue — a valid `TMPDIR` inside the artifact hierarchy defeats persistent input separation

**Locations:** `cmd/skgo-e2e/main.go:656` (`os.MkdirTemp("", "skgo-e2e-merge-input-")`), comment/requirement at **670-674**, and final archive handling at **159-164 / 675-679**.

The repair assumes the default temporary directory is outside cwd `test-results`, but `os.MkdirTemp` with an empty parent uses `os.TempDir`. On Unix that honors nonempty **`TMPDIR`**. The installed Go source used for the independent runner confirms this at `/opt/homebrew/Cellar/go/1.27.1/libexec/src/os/tempfile.go:86-89` and `os/file_unix.go:390-399`. There is no check or independent location selection in the current code when that directory lies beneath the final report/artifact folder.

A caller can legitimately isolate temporary work under its workspace's `test-results/temporary`. With the already-supported HTML `outputFolder: 'test-results'`, native HTML's `onEnd` (`Playwright lib/runner/index.js:3471-3473`) removes that parent. Native merge has already patched JSON file paths into the newly created input (`8019`, `8340-8360`), so all those files disappear. The post-callback archive detects the missing input and returns nonzero, which prevents false green, but cannot preserve the required native resources or archive. A valid native invocation that passes now fails solely through the wrapper's transport lifecycle and leaves a damaged selected report.

**Independent actual reproduction:** fixture `html-delete.config.js` uses separate test `outputDir: 'outside-test-results'` and selects:

```js
reporter: [
  ['html', { outputFolder: 'test-results', open: 'never' }],
  ['json', { outputFile: 'html-delete.json' }]
]
```

The three projects each assert literal `Widget 42`, attach exact file bytes `file:Widget 42:<project>` and inline bytes `inline:Widget 42:<project>`. In `/tmp/skgo-core-round06-review-_cyi9m8y/fixture/`, the actual controls set **`TMPDIR=/tmp/skgo-core-round06-review-_cyi9m8y/fixture/test-results/temporary`**, `CI=1`, `FORCE_COLOR=0`. That directory was recreated before each invocation; no JSON/blob destination or internal reporter hook override was present:

```text
node ./node_modules/playwright/cli.js test -c html-delete.config.js
../skgo-e2e -c html-delete.config.js
```

Native returns **0**, emits **3 expected / 0 unexpected/skipped/flaky**, three `passed` results, `errors: []`, and all three file paths under the separate test output directory contain the exact authored bytes. The wrapper emits the same counts/status/global errors but returns **1**, with no artifact archive. Its exact missing JSON file paths are:

| Project | Missing path | Required bytes |
| --- | --- | --- |
| chromium | `/tmp/skgo-core-round06-review-_cyi9m8y/fixture/test-results/temporary/skgo-e2e-merge-input-2678270074/resources/ccbe0f595205d56f5ca646773262f3b565b41180.txt` | `file:Widget 42:chromium` |
| noscript | `/tmp/skgo-core-round06-review-_cyi9m8y/fixture/test-results/temporary/skgo-e2e-merge-input-2678270074/resources/5c7cb8b1b440a2acd394f5e9fbca639ec04dfaa7.txt` | `file:Widget 42:noscript` |
| source-edit | `/tmp/skgo-core-round06-review-_cyi9m8y/fixture/test-results/temporary/skgo-e2e-merge-input-2678270074/resources/86047db7609d5eeadba8fc412f9a7135c5f75ff5.txt` | `file:Widget 42:source-edit` |

The actual wrapper stderr ends:

```text
archive Playwright blob reports: inspect native merge input: lstat /tmp/skgo-core-round06-review-_cyi9m8y/fixture/test-results/temporary/skgo-e2e-merge-input-2678270074: no such file or directory
```

Receipts are `logs/native-html-temp-parent.{command.json,json,stdout,stderr,exit}` and `logs/wrapper-html-temp-parent.{command.json,json,stdout,stderr,exit}`. `.command.json` records immediate return-time path existence/bytes and the absent archive. The environment is ordinary Go temporary-directory selection, not an unsupported CLI spelling or a reporter that intentionally destroys its test attachments: native's separate `outputDir` avoids that overlap and its control succeeds intact. Randomizing only the final input directory name does not establish independence from a removable ancestor.

## Assessment and limits

No additional material flaw was found in the original query, prerender, scaffold, supervisor/process requirements or the selected bounded native reporter/parser behavior. The current nonzero archive failure correctly exposes the remaining resource loss rather than masking it, but persistent native resources and the post-report archive remain unsatisfied for the reproduced environment. Pending finite controls and exact-head full application/main qualification are recorded as limits, not assumed passes. The verdict applies to `bf01f28`; a future bounded repair is not part of this immutable review.

**Outcome: material findings remain.**
