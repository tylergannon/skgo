# Core matrix follow-ups — adversarial review, round 05

## Target and authority

Reviewed the whole relevant implementation from v0.16.0 (`6b74b0719c7f7fa542b76d36d4378fc891925179`) through `61328bbf38c007d044b7d0fdccdc58d837e06034` in `/Users/tyler/Codex/2026-10-03/task-8/skgo-core-followups`. The target was clean at the implementation-ready checkpoint and remained at that code during review. Previous review artifacts are immutable. This reviewer changed no implementation, generated files, integration dependencies or integration listener; independent executions used copied packages/source in an owned private fixture. This artifact is the reviewer's only worktree edit.

Authority is repository `AGENTS.md`, `/agent-protocol`, `/adversarial-review`, `ephemeral/sveltekit-current/SKILL.md`, the original issue bodies/comments read in the first review for [skgo #223](https://github.com/tylergannon/skgo/issues/223) and [skgo-project #105](https://github.com/tylergannon/skgo-project/issues/105)/[#106](https://github.com/tylergannon/skgo-project/issues/106)/[#107](https://github.com/tylergannon/skgo-project/issues/107), and the caller's restatement of the original user requirements. These require Go-owned HTTP and pooled Goja SSR with native private fields; ordinary universal-load literal correctness plus a separate retained, independently called slow-query single-flight case; source-edit execution after regular Playwright failure with one complete native selected report and aggregate failure; authored Go failed-prerender diagnostics proved by a real failed build; and nonzero unexpected Vite exits with clean external cancellation and cleanup of the owned child group. The authorized stock-scaffold repair runs project-local native `vp check --fix` after all writes and propagates remaining failure.

The source-backed reporter survey and caller's explicitly selected compatibility bounds also apply. They permit one native final multiplexer, explicit CLI replacement plus CSV addition, or configured/default base plus one added built-in/existing local reporter path. Unsupported configured-base multiple additions/package additions and a required operand equal to literal `--` must fail explicitly; they must not silently succeed. No instruction excluded a defect area or prescribed a verdict. The pending retention repair was not reviewed as though it already existed.

## Evidence inspected and executed

- Read the baseline-to-target implementation, surrounding tests, repository instructions and applicable worklogs. Rechecked the query/load/component, authored-error bridge/generator, scaffold finishing, development supervisor, process-group cleanup and complete ordered runner. Installed Kit is **3.0.0** and Playwright is **1.63.0**. No broad dependency changes, Node request-time sidecar, replacement HTTP ownership, removal of the Goja pool or rewriting of native private fields appeared in this work.
- Read installed native Kit query proxy/cache ownership and prerender error handling. The ordinary route returns resolved literal values. Its retained variant holds its own proxy while the component still independently calls `getItem(params.id)`. The retained slow assertion is separate from ordinary literal correctness. Actual stable-native receipts in `task-8/native-query-stable/receipts/` show both consumers rendering `Widget 42`, one ordinary request, and two identical requests after controlled post-await finalization/eviction. They demonstrate the cache counterexample, not natural GC frequency or the original CI cause. The worklog correctly avoids claiming that this core follow-up originally fixed already-closed site issues #105/#107.
- Rechecked real failed-build proof in `internal/gen/prerender_test.go`, including the renamed `ui/` fixture, literal authored Go error, `/about`, authored root-layout source and the required nonzero native build. A missing frontend toolchain fails rather than skips. Intentional HTTPError and redirect payloads remain distinct from ordinary failed loads. The bridge adds the ordinary authored error to the native build diagnostic without changing successful payloads.
- Rechecked supervisor failure-before-cancellation ordering, cleanup on every post-start return, unexpected exit 0/7 handling, clean external SIGINT and owned process-group cleanup after the leader exits. The unchanged code retains the earlier real PID/group/listener controls, TERM-resistant descendant control and inherited-pipe WaitDelay controls described in round four. Current integrated tests exercise these paths; this round did not open new sockets or signal another owner's process.
- Read scaffold finishing order and remaining-error propagation. Actual receipts under `task-8/core-followup-logs/scaffold-checks/newapp-viteplus-finishing-20261004/` show native creation/build, native clean formatting/lint checks, the authored formatting/lint failures and restored checks. The orchestration tests alone are not substituted for those native controls.
- Read the complete current Go runner and its pinned native option-role table against Playwright's `program.js`, bundled Commander, config/reporter loaders, `resolveOutputFile`, reporter multiplexer, JSON/HTML/blob serializers and native merge/attachment patching. The runner preserves native scalar repetition, CSV ordering/duplicates, config options and module contexts within the explicitly selected bounds. Stage-only blobs are isolated from the caller's final blob env destinations; the reserved internal reporter hook is rejected on input. Required operands, flag-looking operands, short clusters, project variadic arity and the standalone positional separator receive explicit handling. Unsupported aliases still reach a native failure.
- Independently copied and compiled the exact current runner in `/tmp/skgo-core-round05-review-4a8gqg77/`. The copied `main.go` and target both have SHA-256 `ebf1c8e3860fd2dae0087733c89d07972c184700cf2becc62dc11a527495b583`. Ran the entire copied runner Go test package with `-count=1 -v`: all tests passed, no skips; actual output is `runner-tests.log`. These tests mock Playwright invocations and do not establish native reporter/resource correctness.
- Independently ran installed native Playwright and that compiled runner using privately copied Playwright/core packages, three named projects, one literal assertion per project, an inline attachment and an authored file attachment. No browser or server was needed. Actual commands, stdout, stderr, exits, JSON and ZIPs are retained in `/tmp/skgo-core-round05-review-4a8gqg77/fixture/logs/`. Config-option/env-NAME precedence now matches native output: `native-config-name.*` and `wrapper-config-name.*` both write the config-owned report with three passing results. Configured list plus added JSON now emits the combined native document: `native-add-json.*`, `wrapper-add-json.*`; `wrapper-add-json-failure.*` returns **1** with **expected=1, unexpected=2, skipped=0**, retaining the passing source-edit result after both regular failures.
- Read actual producer receipts under `task-8/core-followup-logs/reporter-contract-final/receipts/`, including `r1-{native,wrapper}-{success,failure}.*`, `r2-{native,wrapper}.*`/configured JSON, `r3-*`, `reporter-csv-*`, `project-*`, `p19-*` and `p15-just-env-json.*`. The copied-root Just/VP control actually passes `--add-reporter=json` and preserves three results. CLI replacement plus CSV addition preserves duplicate native reporter output. These are actual scoped controls, not a claim that the producer's still-running finite partitions had all completed. The copied-root fixture establishes the argument/convenience contract, not complete application qualification.
- Inspected current canonical `task-8/core-followup-logs/final-local/round05-just-test.log` and `round05-just-vet.log`, both reported exit **0**. The full Go suite has no failure or skip lines and includes the real prerender/scaffold/process tests. Existing full application dev/prod native reports have **174 passed each**, **0 skipped/unexpected/flaky/errors**, with 134 chromium, 29 noscript and 11 source-edit results. They predate the latest reporter correction; no exact-head full browser, main-CI, release or publication claim follows from them. The earlier caller production UI inspection is recorded; this reviewer did not start another live application.

## Previous finding dispositions

The earlier startup orphan, env FILE/DIR/NAME report loss, attached-short-config handling, configured reporter overwrite and exited-leader descendant leak remain repaired for their reported triggers. Both round-four findings are repaired for their exact triggers by the current single native merge: added JSON receives the complete report, and env NAME no longer replaces the config-owned JSON reporter options. Independent native controls above support those dispositions. Report completeness by counts does not establish file attachment preservation; the following supported native combinations expose a remaining loss.

## Finding

### 1. Issue — final reporters can delete the runner's native merge resources and return success

**Locations:** `cmd/skgo-e2e/main.go:134-145` and `retainBlobReports` at **lines 650-671**, especially the fixed cwd `test-results` parent at **lines 654/657**.

The runner copies its stage ZIPs into `cwd/test-results/skgo-e2e-blobs-*`, then passes that directory to native `merge-reports`. Native merge extracts resources there and rewrites file attachments to absolute paths there. A caller's valid final HTML or blob reporter can also own `cwd/test-results`. Its normal output cleanup then deletes the runner's merge-input/resource tree during final reporting. The wrapper returns **0**, while requested JSON attachment paths point to missing files or the requested final blob omits those files. This violates the complete native report/attachment contract and also destroys the retained transport receipts intended for artifact collection.

**Native source authority:** installed Playwright `lib/runner/index.js:8019` installs `AttachmentPathPatcher(dir)`; **8340-8360** resolves attachment paths under that merge input. HTML `onEnd` at **3471-3473** removes its configured output folder before building its report. Blob `onEnd` at **3115-3130** first prepares its output, then skips an attachment whose file no longer exists; `_prepareOutputFile` at **3138-3148** removes its resolved output directory. Native merge's **8035** reporter-error status does not detect an attachment silently skipped because its file disappeared. Native JSON serialization records the patched paths; it does not restore missing bytes.

**Reproduction and actual policy:** the private fixture owns its dependencies. Both configs use `outputDir: 'outside-test-results'`, so ordinary native test attachments do not overlap the final reporter output directory. Each of `chromium`, `noscript` and `source-edit` asserts literal `Widget 42` and attaches a file with exact UTF-8 bytes `file:Widget 42:<project>`, plus inline bytes `inline:Widget 42:<project>`. The title also contains literal flag-looking text and a pipe; no CLI interpretation supplies the expectation.

For HTML+JSON, `html-delete.config.js` selects:

```js
reporter: [
  ['html', { outputFolder: 'test-results', open: 'never' }],
  ['json', { outputFile: 'html-delete.json' }]
]
```

From `/tmp/skgo-core-round05-review-4a8gqg77/fixture/`, with `CI=1 FORCE_COLOR=0`, the actual commands were:

```text
node ./node_modules/playwright/cli.js test -c html-delete.config.js
../skgo-e2e -c html-delete.config.js
```

Both exit **0**; both JSON documents contain **expected=3, unexpected=0, skipped=0, flaky=0**, three `passed` results and `errors: []`. Native file paths under `outside-test-results/.../attachments/` exist at return and contain the three exact authored byte strings. The wrapper instead emits these paths, all missing immediately at its return:

| Project | Exact wrapper JSON attachment path | Authored bytes lost |
| --- | --- | --- |
| chromium | `/private/tmp/skgo-core-round05-review-4a8gqg77/fixture/test-results/skgo-e2e-blobs-2877772565/resources/342f2522ff5e6734594fa9adfe7d58715793ad0e.txt` | `file:Widget 42:chromium` |
| noscript | `/private/tmp/skgo-core-round05-review-4a8gqg77/fixture/test-results/skgo-e2e-blobs-2877772565/resources/e0322f712aa01a58a14b82d85e312f03ed5b95c8.txt` | `file:Widget 42:noscript` |
| source-edit | `/private/tmp/skgo-core-round05-review-4a8gqg77/fixture/test-results/skgo-e2e-blobs-2877772565/resources/180d080bf5341252d52efc2f62f562a591d577cf.txt` | `file:Widget 42:source-edit` |

Actual reports and observations are `logs/native-html-parent.json` and `logs/wrapper-html-parent.json`; corresponding `.command.json` files record the exact command, return-time existence and observed bytes, with `.stdout`, `.stderr` and `.exit` retained. The inline attachments retain their correct base64 bodies in both reports. The loss is specifically the referenced file bytes, despite otherwise complete results and a green exit. Later independent runs also clean their own `test-results`; current existence after those runs must not replace the captured return-time observations.

For final blob, `blob.config.js` selects native `reporter: 'blob'`, with the same separate test `outputDir`. The actual commands were:

```text
PLAYWRIGHT_BLOB_OUTPUT_DIR=test-results \
  node ./node_modules/playwright/cli.js test -c blob.config.js
PLAYWRIGHT_BLOB_OUTPUT_DIR=test-results \
  ../skgo-e2e -c blob.config.js
```

Both exit **0**. Saved `logs/native-blob-parent.zip` contains `report.jsonl` and these exact resource entries/bytes:

| Native ZIP entry | Exact bytes |
| --- | --- |
| `resources/fa51a37a221382836f85d2245e84dba5a9e45e43.txt` | `file:Widget 42:source-edit` |
| `resources/8be3541269e147bd62e400e694fcdc33438e3f52.txt` | `file:Widget 42:chromium` |
| `resources/dbd65b969bea11443d6b2016bdf24f9cd1a4a2d8.txt` | `file:Widget 42:noscript` |

Saved `logs/wrapper-blob-parent.zip` contains **only `report.jsonl`, zero resource entries**. Its three `onTestEnd` events and final `onEnd` retain `passed` status. Its `onAttach` events still reference `resources/fa2e978fcc797ea2dbd58e68e2576621537e89df.txt` (chromium), `resources/99f63db7c27e26d9baec4e6ee1978bf6d7825c90.txt` (noscript) and `resources/69f0e0524ffa0a6349e2fd7b1817b2a7058e08b3.txt` (source-edit), but none is present in the ZIP. Inline attachments retain the correct base64 byte bodies. `.command.json`, `.stdout`, `.stderr`, `.exit` and `.entries.json` retain both observations. Native's valid destination succeeds with intact resources; the wrapper introduces the collision through its retained-directory location. This is the same root defect as the HTML/JSON case, not a separate finding.

The retained native input/resources must survive the selected final reporters' legitimate output cleanup and remain available at the paths emitted by native JSON after return. Passing result counts, successful ZIP creation and aggregate status checks alone do not prove that obligation.

## Assessment and limits

No additional material flaw was found in the other original requirements, authorized scaffold finishing, selected bounded reporter composition/parser behavior, repaired native output precedence, or owned child status/cleanup. Current Go tests and native count/status controls pass, but the two independent resource controls above demonstrate a false-green reporting loss. Finite producer controls and exact-head canonical branch/main browser qualification were still outstanding at this review checkpoint; this artifact does not turn those pending runs into evidence. It makes no merge-ready, release, tag or publication claim. The caller's subsequently proposed repair is outside this immutable target and has no effect on this verdict.

**Outcome: material findings remain.**
