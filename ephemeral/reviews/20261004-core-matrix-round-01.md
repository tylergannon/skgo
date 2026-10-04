# Core matrix follow-ups — adversarial review, round 01

## Target and authority

Reviewed `skgo-core-followups` at `a97102ad573adc2600b9ff668c01040b9168db71`, including the full relevant diff from v0.16.0 (`6b74b0719c7f7fa542b76d36d4378fc891925179`). The working tree was clean when inspected. Review is independent and read-only except for this artifact and temporary execution fixtures. No integration listener or generated application state was changed by the reviewer.

Authority: repository `AGENTS.md`; `/agent-protocol`; `/adversarial-review`; `ephemeral/sveltekit-current/SKILL.md`; live issue bodies and comments from [skgo #223](https://github.com/tylergannon/skgo/issues/223), [skgo-project #105](https://github.com/tylergannon/skgo-project/issues/105), [#106](https://github.com/tylergannon/skgo-project/issues/106), and [#107](https://github.com/tylergannon/skgo-project/issues/107). Caller-restated requirements preserve Go-owned HTTP, Goja SSR and native private fields; require literal universal-load correctness plus a separate retained-proxy independent-consumer check; require source-edit execution after ordinary Playwright failure with a complete combined native JSON report and aggregate failure status; require an actual failed prerender build; and require nonzero unexpected Vite exits with clean external cancellation and owned-child shutdown. The dependency and read-only restrictions were treated as implementation/operating constraints, not restrictions on review scope. No caller-provided verdict was assumed.

## Evidence inspected

- Installed `example/web/node_modules/@sveltejs/kit/package.json` confirms **3.0.0**. Read its query proxy, cache controller, promise/effect pinning, prerender error handling and page-load failure handling. Also read the installed Playwright JSON reporter's serialization and output-path resolution.
- Read `ephemeral/worklog/20261004*` and `core-matrix-followups.md`, changed runner/generator/adapter/supervisor code and tests, all regenerated load-stub changes, and surrounding load execution, process-group cleanup, example query component, browser fixtures, and CI/qualification entrypoints.
- Read stable-native receipts under `/Users/tyler/Codex/2026-10-03/task-8/native-query-stable/receipts/`: ordinary navigation has one GET; controlled post-load GC has two identical GETs with literal `Widget 42` in both consumers in dev and production preview. These receipts support replacing the unconditional request-count oracle; they do not establish the original CI trigger.
- Independently ran `go test -count=1 ./cmd/skgo-e2e -v`, the root `TestPrerenderLoad*` tests, and `go test -count=1 ./internal/gen -run '^TestPrerenderGoLoadFailureNamesAuthoredRoute$' -v`. All passed. The named generator test actually invokes the frontend build, requires failure, and asserts `/about`, `src/routes/layout.server.go`, and the literal authored error in its output; its renamed `ui/` fixture also checks the non-default frontend root.
- Inspected `/tmp/skgo-core-prod-playwright.json` and `/tmp/skgo-core-dev-playwright.json`: **174 passed, 0 skipped, 0 unexpected, 0 flaky in each mode**, including 134 chromium, 29 noscript and 11 source-edit scenarios each. Both revised query-navigation scenarios and all source-edit scenarios passed. Inspected the independent real-failure report at `skgo-source-edit/example/e2e/playwright-report/real-failure-prod.json`: one regular scenario failed, both source-edit scenarios ran and passed, no skips, and the failure remains in the combined native document.
- Inspected the earlier and current `just test` logs, `vet` log, and the restored full test log `/tmp/skgo-core-final-just-test-restored.log`. The intervening failure had fixture-route types (`/bad/[x=invalid]`, `/text/[x=text]`, etc.) in the example's generated type universe. The preserved `/tmp/skgo-core-native-param-shared-dependency.txt` identifies an external native-param fixture whose whole `node_modules` symlink pointed into this checkout; the caller isolated that fixture and rebuilt the example. The restored full run passes. This is not attributed to the changed prerender helper, which excludes `$app` when linking dependencies.

## Findings

### 1. Issue — startup failure leaves the owned Vite process alive

**Locations:** `cmd/skgo/dev.go:117`, `cmd/skgo/dev.go:164-167`, `cmd/skgo/dev.go:181-184`.

Vite starts before the public Go listener is opened. If `server.Listen()` fails, the function returns at line 167 and never reaches `viteProc.Stop`. Deferred context cancellation does not stop this child: it was created with `exec.Command` and is owned through `dev.Process`, whose shutdown requires `Stop`. An occupied or malformed public listen address can therefore leave a Vite process and its workers running after `skgo dev` has already failed. This defect predates the reviewed diff, but violates the repository's explicit process-ownership contract and is material to the supervisor follow-up.

**Independent reproduction:** built the current CLI and a silent fake Vite executable that sleeps indefinitely, using fixture `/var/folders/lt/09rsy64x65s_0fp2b8zq3n7m0000gn/T/skgo-core-review-startup-cxs02v5a`. Ran:

```text
<fixture>/skgo dev --root <fixture> --web web \
  --listen invalid-listen-address --origin http://127.0.0.1:8080 \
  --vite <fixture>/silent-fake-vite --vite-port 18399
```

No listener was bound by this fixture. `silent-supervisor.log` contains:

```text
skgo dev: started vite pid 7973 on 127.0.0.1:18399
listen tcp: address invalid-listen-address: missing port in address
```

The supervisor returned **1**. Subsequent OS process inspection, saved as `silent-child-status.txt`, showed PID **7973**, parent PID **1**, process group **7973**, and state **S** (sleeping), with the exact `silent-fake-vite --host 127.0.0.1 --port 18399 --strictPort` command. Thus the child was running and orphaned, rather than merely an exited process visible to `kill(pid, 0)`. The reviewer then terminated only process group 7973. Preserve the fixture logs for the repair review.

Cleanup must cover every return after a successful Vite start, with a test that independently checks the recorded child is gone after a startup failure. The current external-cancellation test checks the supervisor's exit and logging but does not assert child disappearance; do not rely on that test as the shutdown proof.

### 2. Issue — native JSON destination settings break the combined report

**Locations:** `cmd/skgo-e2e/main.go:58-67`, `cmd/skgo-e2e/main.go:89-94`, `cmd/skgo-e2e/main.go:110-112`.

The runner recognizes only `PLAYWRIGHT_JSON_OUTPUT_NAME` and passes other reporter environment variables through unchanged. Installed Playwright 1.63.0's `resolveOutputFile` in `lib/runner/index.js:1534` gives `PLAYWRIGHT_JSON_OUTPUT_FILE` priority over `OUTPUT_NAME`, and resolves a relative `OUTPUT_NAME` against `PLAYWRIGHT_JSON_OUTPUT_DIR`. Consequently, two ordinary supported ways to request a native JSON report violate the caller-restated requirement to produce one complete document at the caller's path:

- With `PLAYWRIGHT_JSON_OUTPUT_FILE=/path/report.json` and `--reporter=json`, both stages write the same file. The second stage overwrites the regular report; the runner then fails to find its temporary reports. The caller's file contains only source-edit results, so ordinary failures and successes are lost.
- With `PLAYWRIGHT_JSON_OUTPUT_DIR=/path/reports`, `PLAYWRIGHT_JSON_OUTPUT_NAME=report.json`, and `--reporter=json`, Playwright writes the stage files under `/path/reports`, while the combiner reads relative paths in the working directory. The command returns failure and never writes the requested combined `/path/reports/report.json`.

**Independent reproduction:** built the current `cmd/skgo-e2e` and ran it against real installed Playwright in `/var/folders/lt/09rsy64x65s_0fp2b8zq3n7m0000gn/T/skgo-core-review-json-env-eo0lplq9`. The isolated config has the three project names the runner selects and one synchronous passing literal assertion per project; it uses no browser or server, modifies no application source and binds no ports.

For `OUTPUT_FILE=<fixture>/native-file.json`, both stages completed, but the runner exited **1** with:

```text
combine Playwright JSON reports: read regular report: open playwright-report/combined-4499.4499.regular.json: no such file or directory
```

`native-file.json` reports **expected=1**, and its sole represented project is `source-edit`; the two passing regular-project results were overwritten. Captured output: `output-file.log`.

For `OUTPUT_DIR=<fixture>/native-dir` and `OUTPUT_NAME=native-name.json`, the runner exited **1** with:

```text
combine Playwright JSON reports: read regular report: open native-name.4584.regular.json: no such file or directory
```

`native-dir/` contains `native-name.4584.regular.json` and `native-name.4584.source-edit.json`, but no `native-name.json`. Captured output: `output-dir-name.log`. Native output-path precedence must be resolved consistently before assigning separate stage destinations and combining their reports; passing only the `OUTPUT_NAME` tests does not cover this behavior.

## Assessment and limits

The reviewed query changes preserve an independent component `getItem(params.id)` call while the retained route keeps its separate proxy alive in route data. The ordinary navigation asserts both literal results and one total document request, which proves Kit client navigation. The failed-prerender diagnostic is attached separately from intentional HTTPError/redirect payloads; Go-owned request handling and native private-field source are not replaced by this diff. The runner's normal `PLAYWRIGHT_JSON_OUTPUT_NAME` path retains ordinary failure results and executes source edits independently.

Full production and development suites pass through the normal `OUTPUT_NAME` path, and the independent failed-build test passes. Neither passing result exercises the two reproductions above. No product-visible regression is inferred from the deliberately injected JavaScript-disabled regular-stage failure or from the isolated native-param fixture contamination. The startup finding predates this patch; the report-destination finding is introduced by the new runner. Both have concrete reproductions and remain present in the reviewed implementation. No implementation repair was made by the reviewer.

**Outcome: material findings remain.**
