# Native prerendered page redirect review — round 02

Outcome: **material findings remain**.

Reviewed target: `ba7963ecf53234bc938fc1dca226a2da740c8eb5`, in `/Users/tyler/Codex/2026-10-03/task-8/skgo-page-redirects`, against released `v0.16.1` (`eb401ec62c6e6a57a92ba19fb18f8fa83008376e`) and immutable round 01. This is a complete assessment of the authorized page redirect capability and its relevant surrounding implementation and proof, not merely a review of the last test edit.

## Authority, sources, and scope

The governing sources are `AGENTS.md`, the mandatory agent protocol and adversarial review skills, `ephemeral/sveltekit-current/SKILL.md`, and the complete `ephemeral/worklog/20261004-prerendered-page-redirects.md`. The accepted capability is a prerendered authored Go page load redirect `/old` → `/target?from=atlas`, with native HTML artifact serving, native HTTP method/slash behavior, static ownership over matching dynamic endpoints, missing listed artifact rejection, and unlisted file exclusion. The destination page is not required to be prerendered. Completing other parts of the combined site prerendering bucket is not authorized by this assignment.

Read the complete released-baseline diff, the complete final fixture, and the relevant surrounding adapter copy/manifest/CSP code, static startup and file resolution, endpoint routing/configuration, static predicates used by hooks, data dispatch, fetch handling, ordinary example handler composition, embedding, and build fixture helpers. Prior round source mapping and actual receipts were reassessed alongside the new final ordinary-suite output. The readiness-file source from round 01 was read again. Original implementation file ownership constrains edits, not whether a concrete interacting verification defect can be reported. No verdict was assumed, no review work delegated, and no implementation, generated state, dependency, or external control fixture changed by this reviewer.

Installed Kit is verified **3.0.0**. Its `src/core/postbuild/prerender.js:260–267, 542–599, 614–629` writes and records the native redirect script/meta refresh artifact. `src/runtime/server/page/index.js:87–97` returns 204 for an ordinary dynamic destination during the crawl. The pinned official [adapter-node static source](https://raw.githubusercontent.com/sveltejs/kit/@sveltejs%2Fkit@3.0.0/packages/adapter-node/src/static.js) and [build source](https://raw.githubusercontent.com/sveltejs/kit/@sveltejs%2Fkit@3.0.0/packages/adapter-node/src/index.js) establish canonical file selection, method rejection, HEAD suppression, and query-preserving slash aliases ahead of dynamic dispatch. Go's local primary embedding source, `/opt/homebrew/Cellar/go/1.27.1/libexec/src/embed/embed.go:93–99`, explains why native query-bearing filenames cannot be embedded directly.

## Current implementation and the closed finding

The adapter preserves Kit's files and recorded path list while removing the old redirect rejection. It does not synthesize an HTTP redirect status map. `endpoint.go:409–415` bypasses matching dynamic endpoints for every method and either slash form. `static.go:515–518` gives recorded files and aliases native 405 ownership; existing canonical serving and relative 308 handling send the native bytes and slash response. Startup still requires every recorded path to have an artifact. The hook's static predicate claims those files ahead of application middleware. Go still owns HTTP and pooled Goja SSR; no runtime Node process, dependency rework, JavaScript test harness, handwritten redirect format, or native artifact normalization is introduced.

**Round 01 finding 1 is closed for the accepted capability.** The final `internal/gen/prerender_redirect_test.go` leaves the target ordinary/dynamic, preserves the authored query-bearing Go redirect and successful native `vp build` assertion, requires `/old` and its native script/meta refresh artifact, then executes ordinary `go build -o <temporary binary> ./cmd` at lines 110–115. Generation, native build, artifact reads, and embedded app compile failures are all fatal. The copied example's import is adjusted for its frontend rename at line 37, so the compiled application actually imports the generated `ui` package containing `//go:embed all:build`; it does not compile a different placeholder frontend. `copyExample` excludes the source checkout's frontend build tree, and generation points back to the reviewed skgo module.

This repair removes an extra fixture constraint and verifies the actual binary. It does not delete or rename Kit output to conceal the earlier failure. The preserved query-to-prerendered-destination control remains a limitation: Kit can emit `target?from=atlas.html` when the target itself is prerendered, and ordinary Go embedding rejects that name. The accepted ordinary-destination path is proved separately; this review makes no broader support claim.

## Actual evidence and its limits

Receipts are under `/Users/tyler/Codex/2026-10-03/task-8/page-redirect-logs/`. I inspected actual output and corresponding source; these runs were performed by the owner, parent, and independent validator, not executed by this reviewer.

- `final-canonical-test.log` / `.exit`: final ordinary `just test` (`go test -count=1 ./... ./example/...`) exits **0**, including root handlers, `internal/gen` (43.108s), `internal/dev` (37.872s), and the ordinary example. The final new build test has no skip or early-success path. The only source `Skip` is the existing Windows-guarded formatter fixture at `internal/gen/format_test.go:13`, inactive on this darwin/arm64 run.
- `canonical-build.log` / `.exit` and `canonical-vet.log` / `.exit` contain successful canonical build and vet receipts for the preceding implementation checkpoint. The only subsequent source change is the reviewed build-test repair. These older receipts are not mislabeled as new final-head executions.
- `independent-controls/baseline-build.log` fails the authored redirect with the old adapter's literal rejection and exit 1, despite Kit already writing `old.html`. `candidate-build.log` passes with the redirect implementation, making successful native build completion a load-bearing distinction. The original failed ordinary embed compile is retained in `go-app-build.log` and is not erased.
- `independent-controls/frozen-handler-positive.log` passes all six selected handler/startup/base contracts. The matching wildcard route uses Kit's actual rest-pattern shape, a literal zero invocation expectation over canonical/alias GET/HEAD/POST/OPTIONS, and an unrecorded `/other` positive control requiring literal dynamic bytes and exactly one call.
- `remove-static-method-guard.log`, `restore-old-endpoint-guard.log`, and `remove-endpoint-bypass.log` are actual failing controls: they expose wrong method responses, missing native bodies, or nonzero wildcard calls and exit 1. `source-receipt.txt` records byte-identical frozen adapter/static/endpoint implementation. Those product files are unchanged at this target, so the controls apply to the current product behavior; they are not claimed to exercise the newly added Go compile assertion.
- `ssr-target-vp-build.log` and `go-embedded-ssr-target-build.log` prove a separate native dynamic-destination build and actual embedded Go application compile, both exit 0, without changing generated artifacts. `embedded-app-server.log` and `real-http-browser.log` prove actual serving by that binary. The final fixture now matches this successfully embeddable destination shape.

| Actual embedded application check | Observed outcome |
| --- | --- |
| GET `/old` | 200 HTML, no `Location`; native script/meta refresh preserves `/target?from=atlas` |
| HEAD `/old` | 200, no `Location`, zero body bytes |
| GET `/old/?q=1` | 308, `Location: ../old?q=1` |
| POST and OPTIONS, `/old` and `/old/` | All 405, `Allow: GET, HEAD` |
| Browser, scripting enabled | Reaches `/target?from=atlas`, destination 200, visible `Target route` |
| Browser, scripting disabled | Native meta refresh reaches the same destination/query, 200, visible `Target route` |

I personally inspected both retained `target-script.png` and `target-noscript.png`: both show the ordinary example layout and the destination heading, without a blank page or error boundary. The source-backed missing-file test requires a startup error naming a literal deleted recorded path; the stray-file test requires 404 while a recorded positive-control page still serves literal HTML. These assertions remain meaningful and passed.

No future CI result, full final branch development/production native suite, or `main` native browser qualification is claimed. The actual scoped browser check above is evidence of the redirect behavior, not a replacement claim for those later runs.

## Finding

### 1. Issue — the observed ordinary-suite readiness-publication race remains unfixed

**Source:** `internal/dev/proc_unix_test.go:52` publishes the worker's readiness pathname using `os.WriteFile`. The local primary Go implementation (`/opt/homebrew/Cellar/go/1.27.1/libexec/src/os/file.go:939–949`) opens/creates/truncates the file and then writes its bytes. `awaitFixtureInfo`, at `proc_unix_test.go:286–290`, treats any successful read as complete publication and returns immediately on JSON unmarshalling error. Both the launching fixture and parent test poll this file.

**Observed reproduction:** The unchanged test failed during the prior complete canonical run, retained as `canonical-test.log` / `.exit` (**1**):

```text
--- FAIL: TestStartProcessBoundsInheritedPipesAndPreservesWaitDelay/caller-supplied
    proc_unix_test.go:198: unexpected end of JSON input; leader output: ""
FAIL github.com/tylergannon/skgo/internal/dev
```

A reader can observe the pathname after creation and before the JSON write, read an empty/partial file, and abort the process cleanup/WaitDelay proof before its real assertions run. This is a concrete source race matching the actual observed failure. It is not a hypothetical tooling warning or an inference that the page redirect handler is wrong.

**Impact and authority:** The final ordinary suite's exit 0 is valid evidence of that successful run and removes the claim that the latest complete run is red. It does not change the writer/reader interleaving or repair the intermittent test failure. Likewise, the retained twenty successful focused repetitions in `dev-readiness-race-diagnostic.log` do not fix it. The mandatory [agent protocol](/Users/tyler/.codex/skills/agent-protocol/SKILL.md) explicitly requires: “Fix broken tests or CI regardless of origin.” Its allowance for a materially distracting pre-existing failure requires HITL disposition; none was supplied as authority to treat this race as fixed. The original six-area implementation ownership is not such a disposition.

Make readiness publication complete before readers accept it, preserve meaningful malformed/setup failures, and exercise the actual ordinary test after repair. This need not broaden the page redirect implementation or alter its native static contract. The reviewer remains read-only; any source repair must be assigned/authorized by the parent.

## Conclusion

The accepted page redirect implementation and its bounded embedded-build proof now satisfy the inspected native contract and have actual positive and failing controls. Round 01's build-proof issue is closed. One material repository verification defect remains: the recorded readiness-publication race is unchanged, even though the latest full ordinary run passed.

Outcome: **material findings remain**.
