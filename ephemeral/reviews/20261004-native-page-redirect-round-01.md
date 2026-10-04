# Native prerendered page redirect review — round 01

Outcome: **material findings remain**.

Reviewed target: `ad62f94227a26293c66d6466092368b8a9dd3b7a`, against released `v0.16.1` (`eb401ec62c6e6a57a92ba19fb18f8fa83008376e`), in `/Users/tyler/Codex/2026-10-03/task-8/skgo-page-redirects`. This artifact assesses that frozen revision. The later validation repair at `ba7963e` is not part of this verdict.

## Authority and scope

Read `AGENTS.md`, the mandatory agent protocol and adversarial review skills, `ephemeral/sveltekit-current/SKILL.md`, and the complete `ephemeral/worklog/20261004-prerendered-page-redirects.md`. The authorized capability is a prerendered Go page load redirect, retained as Kit's native HTML and served by Go with native static ownership. Conversation acceptance supplies the literal `/old` → `/target?from=atlas`, canonical GET/HEAD, query-preserving slash normalization, rejection of POST/OPTIONS, matching wildcard endpoint exclusion with an independent positive control, missing listed artifact rejection, and unlisted file exclusion.

The authorization covers the page redirect subset. It does not authorize completion of the combined site prerendering bucket or demand that the destination page itself be prerendered. This is the capability boundary, not an exclusion of interacting defects: endpoint dispatch, static predicates used by hooks, embedding, generated build bridges, existing ordinary tests, and actual browser behavior were inspected. No caller verdict was assumed. No implementation, generated state, dependency, or control fixture was changed by this reviewer, and no work was delegated.

## Sources and implementation inspected

The complete relevant diff includes `internal/adapter/skgo-adapter.js`, `static.go`, `endpoint.go`, both corresponding handler test files, and `internal/gen/prerender_redirect_test.go`. Surrounding code read includes adapter build/copy/manifest/CSP handling, static startup indexing and missing-file validation, compression and file resolution, `ServedAsFile`, middleware static exclusion, load-data dispatch, internal fetch handling, endpoint configuration/routing, example handler composition, embedding, and the real build fixture helpers.

Installed Kit was independently verified as **3.0.0**. Its `src/core/postbuild/prerender.js:260–267, 360–367, 542–599, 614–629` defines redirect file naming, native script/meta refresh output, recorded paths, and redirected crawl entries. `src/runtime/server/page/index.js:87–97` returns 204 for an ordinary dynamic destination during prerendering. No redirect status map is needed in Go.

The pinned official [adapter-node static source](https://raw.githubusercontent.com/sveltejs/kit/@sveltejs%2Fkit@3.0.0/packages/adapter-node/src/static.js), particularly `create_file_map` and `serve_static`, selects a recorded canonical file or slash alias before the dynamic server. Canonical files answer 200, HEAD ends without streaming the file, other methods answer 405 with `Allow: GET, HEAD`, and slash aliases answer 308 with the query retained. The pinned [adapter-node build source](https://raw.githubusercontent.com/sveltejs/kit/@sveltejs%2Fkit@3.0.0/packages/adapter-node/src/index.js), including `create_prerendered_table`, ties static routes to builder-recorded paths and native files.

Local primary Go source was also inspected: `/opt/homebrew/Cellar/go/1.27.1/libexec/src/embed/embed.go:93–99` forbids matched names containing `?` and rejects invalid matches. The ordinary example uses `//go:embed all:build` at `example/web/dist.go:15`.

The product changes follow the native static contract: the adapter removes its redirect rejection while keeping the native copy and recorded path list; `endpoint.go:409–415` bypasses matching endpoints for every method and both slash forms; `static.go:515–518` rejects unsupported methods before dynamic page dispatch; the existing canonical file and relative slash responses remain responsible for the bytes and statuses. Go continues owning HTTP and pooled Goja SSR. No dependency rework, Node runtime server, JavaScript proof harness, redirect schema, or native artifact reconstruction was introduced.

## Actual proof inspected

Raw receipts are under `/Users/tyler/Codex/2026-10-03/task-8/page-redirect-logs/`. These runs were performed by the implementation owner, parent, or independent validator; this reviewer read their actual output and the corresponding source, rather than claiming to have executed them.

- `canonical-build.log` / `.exit`: actual root `just build`, exit 0. `canonical-vet.log` / `.exit`: actual root `just vet`, exit 0.
- `current-real-build.log` and `scoped-sourcechecks.log`: the new real native `vp build` test runs and passes. Missing tooling/setup is fatal in its dependency helper; there is no new skip path.
- `independent-controls/baseline-build.log`: the same authored redirect on the released adapter exits 1 with `/old -> 307 /target?from=atlas`. Native `old.html` already exists before the adapter rejects it. This makes the successful-build assertion load-bearing, unlike checking only for the file.
- `independent-controls/candidate-build.log`: the frozen candidate test passes with `vp build` exit 0. Its complete output and fixture are retained; the subsequent ordinary app compile fails as described in finding 1.
- `independent-controls/frozen-handler-positive.log`: all six selected native HTTP/static/wildcard/startup/base tests pass. The wildcard test uses the actual Kit rest-pattern shape, asserts its invocation count against literal zero, then sends `/other` and requires the literal dynamic response and exactly one call to `/other`.
- `independent-controls/remove-static-method-guard.log`: removing that guard causes POST canonical/alias failures and exit 1. `restore-old-endpoint-guard.log`: restoring the old GET/HEAD-only exact-path bypass fails POST/OPTIONS behavior and the zero-call assertion. `remove-endpoint-bypass.log`: removing the bypass fails canonical bodies, method responses, and the invocation counter, exit 1. These are actual negative controls, not predictions from the diff.
- `independent-controls/source-receipt.txt` records frozen source identity for the disposable control. Its static, endpoint, and adapter source is byte-identical to `ad62f94`. The dynamic-destination variant removes the destination's extra prerender option and adjusts only the copied example's frontend import for its `web` → `ui` fixture rename; it does not normalize or remove any generated artifact.
- `independent-controls/ssr-target-vp-build.log`: that separate dynamic-destination variant has native `vp build` exit 0. `go-embedded-ssr-target-build.log`: its actual ordinary embedded Go app compile exits 0. The built manifest retains `/old`, the built `old.html` retains the literal query redirect, and no query-bearing destination artifact is generated.
- `independent-controls/embedded-app-server.log` and `real-http-browser.log`: the actual embedded Go application answers on the validator's owned port 18088. The observed HTTP and browser results are summarized below. I inspected both `target-script.png` and `target-noscript.png`; each visibly shows the ordinary example layout and **Target route**, without an error boundary or blank page.

| Actual request/control | Observed result |
| --- | --- |
| GET `/old` | 200, HTML, no `Location`; the decompressed 114-byte body is the native script/meta refresh with `/target?from=atlas` |
| HEAD `/old` | 200, no `Location`, zero body bytes |
| GET `/old/?q=1` | 308, `Location: ../old?q=1`, zero body bytes |
| POST `/old` and `/old/` | Both 405, `Allow: GET, HEAD` |
| OPTIONS `/old` and `/old/` | Both 405, `Allow: GET, HEAD` |
| Browser with scripting enabled | Native artifact navigates to `/target?from=atlas`, destination 200, visible `Target route` |
| Browser with scripting disabled | Native meta refresh navigates to the same query-bearing destination, destination 200, visible `Target route` |

The missing-listed-file test deletes a literal recorded file and requires startup error naming `/about`. The unlisted-file test requires `/stray` to miss while `/about` still serves literal fixture HTML. The startup implementation checks every manifest entry; it therefore covers redirect entries as well. Those assertions are genuine handler contracts, not a manufactured manifest count or self-derived baseline.

## Findings

### 1. Issue — the committed successful build proof produces an app that cannot compile

**Evidence:** Frozen `internal/gen/prerender_redirect_test.go:67` marks the destination page itself `prerender = true`. Lines 73–106 then require only native `vp build` success, inclusion of `/old`, and redirect HTML fragments, and never compile the resulting Go application. `example/web/dist.go:15` embeds the complete native build tree.

The retained exact fixture at `skgo-page-redirect-baseline/ephemeral/independent-candidate-fixture` records both `/target` and `/target?from=atlas` in `ui/build/skgo.manifest.json`, and contains `ui/build/prerendered/target?from=atlas.html` plus its native compressed variants. Actual `independent-controls/go-app-build.log` reports:

```text
ui/dist.go:15:12: pattern all:build: cannot embed file build/prerendered/target?from=atlas.html: invalid name target?from=atlas.html
GO_BUILD_EXIT=1
```

**Cause and impact:** Kit's redirect branch enqueues the resolved destination including its query (`prerender.js:566–568`), and native naming retains that string (`260–267`). With the fixture's extra destination prerender option, the crawler writes the query-bearing name. Go's documented embedding rules reject it. Thus the test's success is not proof of the repository's required runnable, single embedded binary; it can remain green when its generated app cannot be built.

This is a bounded test/proof defect, not evidence that the accepted redirect/query capability always fails. The independent ordinary-destination variant successfully builds, embeds, serves all required HTTP responses, and navigates in both browser scripting modes. The acceptance request does not require the destination's extra prerender mode. Use a fixture for the authorized capability and assert its actual ordinary Go application compile, while preserving Kit's output. The query-to-prerendered-destination filename/embedding boundary remains an explicit unsupported limitation unless separately addressed; deleting or renaming native files to make this fixture pass would not prove native compatibility.

### 2. Issue — the ordinary suite has an observed readiness-publication race

**Evidence:** `canonical-test.log` / `.exit` preserves root `just test` exit 1:

```text
--- FAIL: TestStartProcessBoundsInheritedPipesAndPreservesWaitDelay/caller-supplied
    proc_unix_test.go:198: unexpected end of JSON input; leader output: ""
FAIL github.com/tylergannon/skgo/internal/dev
```

At frozen `internal/dev/proc_unix_test.go:52`, the worker writes readiness with `os.WriteFile`. That operation creates the pathname before the full JSON content has been written. `awaitFixtureInfo` at lines 286–290 treats the first successful `os.ReadFile` as complete publication and immediately returns an unmarshalling error. Both the parent test and the fixture's launcher read this file, so either can observe the empty/partial publication interval. The recorded failure matches that interleaving.

**Impact and requirement:** This is pre-existing and outside the implementation owner's original file assignment, but it is a concrete reliability failure in the current repository's required ordinary suite. The agent protocol requires fixing broken tests regardless of origin, and `AGENTS.md` requires a green ordinary suite with real runnable checks. `dev-readiness-race-diagnostic.log` shows twenty focused repetitions passing; those do not eliminate this source race or supersede the retained full-suite failure. Make readiness publication complete before readers can accept it, preserve meaningful malformed/setup failures, then run the required ordinary suite. No current full-suite pass can be asserted at this reviewed revision.

## Remaining qualification and conclusion

The accepted native page redirect HTTP behavior has actual positive evidence, and the new build/method/endpoint assertions have actual failing controls. The implementation remains small and follows the pinned source. The two findings above concern the committed runnable-build proof and a real ordinary-suite blocker; neither is suppressed by its origin or the narrow feature authorization.

No current page-redirect branch or `main` full native development/production suite qualification was supplied. The specific actual embedded-browser check above is a scoped proof, not a claim of those broader runs. Their eventual delivery status is separate from this frozen review and must not be inferred from canonical build/vet success or from repaired code not reviewed here.

Outcome: **material findings remain**.
