# Query-bearing prerender packaging — adversarial review, round 02

Outcome: **no findings**.

No material findings remain in the authorized packaging change. The round-one exposure of mapped crawl-query artifacts is closed. Broader existing encoded-path compatibility remains unresolved and is described below; this outcome does not claim a complete native request decoder or release qualification.

## Target and authority

Reviewed `/Users/tyler/Codex/2026-10-03/task-8/skgo-query-prerender` at **1e680d1d2f5e4c8400379a108c5bda564d0bf8fe**, against baseline **0e6d193ece885bb671dceaf7413eccfa92afe429** and immutable round one at f85b923. The runtime repair is **7ec3391191ea5b7f450bcc837eb6580d9357a962**. The final commit changes only its explanatory comment; I inspected that comparison and did not treat the commit itself as requiring another proof run.

Authority is `AGENTS.md`, the mandatory agent protocol, `/Users/tyler/.agents/skills/adversarial-review/SKILL.md`, `ephemeral/sveltekit-current/SKILL.md`, the authorized packaging goal and explicit request-decoder exclusion, `ephemeral/worklog/20261004-query-prerender-packaging.md`, and `ephemeral/reviews/20261004-query-prerender-round-01.md`. The capability is an ordinary embedded Go application containing Kit's query-bearing prerender artifacts, preserving native source bytes and inventory, compressed variants, logical identity, pathname-only selection, legacy builds and malformed-map refusal. Four-file ownership is an implementation constraint, not a defect exclusion. I considered surrounding HTTP interception and retained native counterexamples without adopting a requested verdict.

I performed this review without delegation, product edits, new servers, or duplicate full/native builds. I inspected the complete baseline-to-current change, the repair diff, and relevant adapter, static, endpoint, load/data/fetch, build fixture and Go embed context. Installed Kit remains **3.0.0**. Primary source inspected includes Kit's native redirect enqueue/output filename/recorded-path behavior, builder copying and compression, `decode_uri`, Go `net/url` decoding and embed-name validation, and the pinned [official adapter-node static middleware](https://raw.githubusercontent.com/sveltejs/kit/@sveltejs%2Fkit@3.0.0/packages/adapter-node/src/static.js). Native middleware retains inventory keys but splits the request search before URI decoding; reserved encoded delimiters do not acquire search-key authority.

## Round-one finding disposition

`static.go:366` now excludes raw-question-mark crawl entries from the handler's HTTP ownership set. It preserves `Manifest.Prerendered`, all logical-to-physical mappings, the logical file index, and the startup validation loop over the complete inventory (`static.go:426`). Thus an inaccessible crawl artifact must still exist and be valid; filtering its HTTP authority cannot conceal a missing build file.

The shared `isPrerenderedPath` predicate (`static.go:786`) also declines a decoded pathname containing `?`. This matters because endpoint interception receives the full manifest inventory. Its call at `endpoint.go:412` now lets the dynamic route run for a pathname the query-only crawl entry cannot own. Static method ownership and `ServedAsFile` use the same guard. Direct static data, remote, page and slash lookups (`static.go:654`, `:681`, `:702`, `:712`, `:1115`) use the filtered handler set. I found no remaining path from these branches to a query-only mapped artifact.

The new maintained test, `TestQueryOnlyPrerenderedInventoryDoesNotClaimHTTPPathnames` (`static_test.go:854`), checks the retained literal inventory/index, GET and HEAD misses for canonical and slash forms, `ServedAsFile` refusal, required-file startup failure, and a wildcard endpoint returning its supplied literal body with call count exactly one. Existing mapped tests still distinguish canonical, query and encoded-file bytes, exercise compressed representations, check physical namespace misses, and reject malformed metadata and collisions.

The independent controls below prove both repair parts are load-bearing. Raw files are under `/Users/tyler/Codex/2026-10-03/task-8/query-prerender-logs/`:

| Actual control | Observed result |
| --- | --- |
| `final-private-boundary-positive.log` | GET/HEAD `/only%3Fquery` and `/only%3Fquery/` all 404, no Location, no private bytes; maintained inventory/endpoint contract passes; exit 0 |
| `remove-private-inventory-http-filter.log` | Canonical GET/HEAD become 200 and GET exposes `private crawl artifact`; slash GET/HEAD become 308 `../only?query`; independent contract fails, exit 1 |
| `remove-shared-private-path-guard.log` | Wildcard positive control becomes 404 with zero calls; maintained contract fails, exit 1 |
| `final-scoped-handler-positive.log` | Canonical redirect ownership, mapped variants, exact logical identities, malformed-map refusal, required-file failure and query-only boundary contracts all pass, exit 0 |

These are actual positive and deliberately broken private controls, not inferred success from source. Round one's new mapped artifact exposure is closed without changing request decoding, manifest inventory or bytes.

## Packaging and current application proof

The adapter remains unchanged by this repair. It copies native prerender output, applies existing CSP handling, runs native compression, then relocates question-bearing files only in the adapter-owned tree (`internal/adapter/skgo-adapter.js:217`, `:254`, `:256`, `:349`). Hash-addressed physical storage and logical MIME/variant indexing stay separate. Go's map index (`static.go:444`) restricts the physical namespace, validates logical paths, refuses duplicate physical references and missing files, and refuses conflicts with ordinary prerendered entries. Ordinary builds without the optional mapping still work.

I retained and inspected round one's real native/build evidence: the baseline native build emits question-bearing names and ordinary Go compilation fails; the candidate native build and Go embed compile pass; untouched native identity bytes and decoded actual Brotli/gzip bytes match; native atlas/beacon inventory and all six mapped files remain present. I additionally inspected `captured-native-assertions-positive.log`, `drop-native-query-file.log`, `drop-native-query-record.log`, `remove-map-consumer-native-startup.log`, `drop-compressed-map-entry.log`, `missing-mapped-compression-file.log`, `remove-mapped-compression-encoding.log`, and `corrupted-mapped-brotli-content.log`. Their deliberately broken controls fail on the omitted inventory, file, consumer, encoding or actual bytes. The retained producer-removal controls also restore the actual Go embed failure. `question-normalization-collision-positive.log` preserves distinct question and literal-percent identities; `naive-question-percent-normalization.log` fails when those are deliberately collapsed.

`final-retained-app-build.log` records **GO_BUILD_EXIT=0** with the repaired runtime. `final-real-http-browser.log` records **BROWSER_PROBE_EXIT=0** and actual embedded Go HTTP behavior:

- `/old` GET 200 serves the exact 114-byte native script/meta artifact, with no Location; HEAD is 200 with zero body.
- `/old/?q=1` is 308 with Location `../old?q=1`; POST/OPTIONS on canonical and slash forms are 405 with `Allow: GET, HEAD`.
- Physical mapped storage is 404. GET/HEAD of both encoded private atlas/beacon paths and both slash forms are 404; they expose no private artifact and produce no private slash alias.
- Ordinary `/target?from=atlas` and `/target?from=beacon` return the canonical native identity. Actual browser navigation reaches both prerendered query destinations and the ordinary SSR destination with scripting enabled and disabled.

I opened `final-target-script.png` and `final-target-noscript.png`; both show the authored `Target route` page, with no blank page or error boundary. Native canonical/query target artifacts happen to share their bytes, so these real pages do not replace the independent distinct-literal selection tests.

Current coordinator `coordinator/test.{log,exit}` and `coordinator/vet.{log,exit}` were rerun after the runtime repair and record exit 0 at the final target. The uncached full ordinary suite includes gen and dev (44.551s and 39.250s); all packages pass. Earlier candidate checks are preserved separately. The sole source skip remains the Windows-guarded formatter fixture, inactive on this Darwin run. The prior canonical root build and fresh repaired embedded-app compile are evidence at their respective checkpoints; no future CI, exact-head both-mode full browser qualification, merge or release is represented as complete here.

## Existing encoded-path limitation and its disposition

The true native encoded-file oracles remain intact. In a fixture that also has the literal file `target%3Ffrom=atlas.html`, official middleware serves those encoded-file bytes for `/target%3Ffrom=atlas` and misses `/target%253Ffrom=atlas`. Go's current use of decoded `URL.Path` still does not implement that native URI contract. The filtering repair prevents selecting the private question-bearing file, but does not make the literal encoded filename reachable at the native URL or remove the existing double-encoding behavior.

This is independently established on **baseline 0e6d193**, before the mapping feature: `baseline-legacy-percent-native-gap.log` records wrong query-artifact bytes for `/target%3Ffrom=atlas`, wrong 200 for `/only%3Fquery`, and wrong encoded-file 200 for `/target%253Ffrom=atlas`, with **GO_TEST_EXIT=1**. Those controls use legacy unrelocated files and the original handler. `official-native-percent-probe.log` retains the real contrary native values. Their failures are not erased, converted into green expected values, or claimed fixed by this round.

The authorized request explicitly excludes a broad request-decoder change. I therefore treat the demonstrated legacy encoded-path defect as an unresolved compatibility limitation requiring separate authorized work, rather than a missing physical packaging capability or a newly introduced regression. The new query-artifact authority problem identified in round one is now separately proved closed. This review approves no broad native-URI equivalence claim; its **no findings** outcome applies to the requested packaging repair with that explicit limitation and the remaining coordinator qualification obligations.
