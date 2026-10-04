# Native prerendered page redirect review — round 03

Outcome: **no findings**.

Reviewed target: `8e35e35a4dbe75838a0d64848987012864842d9f`, in `/Users/tyler/Codex/2026-10-03/task-8/skgo-page-redirects`, against released `v0.16.1` (`eb401ec62c6e6a57a92ba19fb18f8fa83008376e`) and immutable rounds 01/02. This reviews the complete authorized page redirect implementation, its relevant surrounding code and proof, and the newly authorized qualification repair. It does not assert merge/release qualification of a future commit.

## Authority and sources

Read the repository `AGENTS.md`, mandatory agent protocol and adversarial review skills, `ephemeral/sveltekit-current/SKILL.md`, and the complete authoritative `ephemeral/worklog/20261004-prerendered-page-redirects.md`. Conversation acceptance requires an authored Go `/old` → `/target?from=atlas`, successful actual native build, canonical native HTML serving with GET 200/no Location and HEAD 200/zero body, slash GET 308 with `../old?q=1`, POST/OPTIONS 405 with `Allow: GET, HEAD` in both slash forms, static precedence over a genuinely matching wildcard dynamic endpoint with literal independent controls, missing listed artifact startup rejection, and stray unlisted file exclusion. Destination prerendering and other parts of the combined site prerendering bucket are not authorized capability requirements.

The parent additionally authorized `internal/dev/proc_unix_test.go` solely to repair readiness publication/observation without weakening cleanup proofs or changing production behavior. Ownership boundaries were not treated as exclusions of interacting defects. No verdict was assumed, no work delegated, and no implementation, generated state, dependency, control fixture, socket, or prior review artifact changed by this reviewer.

Inspected the complete baseline-to-target product/test diff and final fixture, retaining the earlier surrounding source review: adapter native copy/manifest/CSP handling, static startup/indexing/file resolution, compression and HTTP dispatch, endpoint configuration and matching, hook static exclusion, data/fetch handling, real example handler composition, embedding, and private build helpers. Re-read the full Unix process fixture and all its process cleanup and timing assertions. The only qualification change is the readiness writer and two failure-preservation tests; the redirect product and handler contract files are byte-identical to their previously controlled revision.

Installed Kit was independently reverified **3.0.0**. Primary native authority is its `src/core/postbuild/prerender.js:260–267, 542–599, 614–629`, plus `src/runtime/server/page/index.js:87–97`. The pinned official [adapter-node static source](https://raw.githubusercontent.com/sveltejs/kit/@sveltejs%2Fkit@3.0.0/packages/adapter-node/src/static.js) establishes recorded file/alias ownership before dynamic dispatch, GET/HEAD behavior, method rejection, and query-preserving 308 aliases. Its [build source](https://raw.githubusercontent.com/sveltejs/kit/@sveltejs%2Fkit@3.0.0/packages/adapter-node/src/index.js) maps builder-recorded paths to native files.

For the qualification repair, read actual Go 1.27.1 primary source: `/opt/homebrew/Cellar/go/1.27.1/libexec/src/os/file.go:431–439` and `src/os/file_unix.go:26–54`. The Unix implementation uses native `syscall.Rename`; Go's documentation explicitly warns that the same atomic property does not hold on non-Unix platforms. The fixture is constrained by `//go:build unix`, and publication uses a sibling temporary file, so it introduces no Windows portability claim or cross-filesystem rename assumption. Retained source/docs receipts are under `page-redirect-logs/readiness-proposal/`.

## Implementation and prior findings

The adapter retains Kit's native redirect files and path list rather than synthesizing status/location data. Canonical GET returns the HTML artifact. The existing slash response normalizes only the pathname, with a relative Location and the caller's query. `endpoint.go:409–415` passes recorded canonical/alias paths to static handling for every method, ahead of matching dynamic endpoint execution. `static.go:515–518` enforces native 405 ownership, and startup checks every manifest-recorded path for a file. The static predicate used by the hook also claims those requests. Go continues owning all HTTP and pooled Goja SSR; no dependency rework, Node runtime server, JavaScript harness, or native redirect artifact normalization was introduced.

**Round 01's runnable-build proof finding remains closed.** `internal/gen/prerender_redirect_test.go` leaves the destination ordinary/dynamic, preserves the exact authored Go redirect/query, requires successful native `vp build`, reads `/old` from the manifest and native script/meta refresh HTML, then requires ordinary `go build ./cmd`. Its private copied app imports the renamed frontend package containing `//go:embed all:build`. Setup, generation, native build, artifact, and binary compile failures are fatal; no stale source build tree is substituted.

The preserved native query-to-prerendered-destination case remains an explicit limitation: Kit can emit a `?`-bearing filename when that destination itself is prerendered, and Go's native embed rules reject the name. The accepted ordinary-destination path is proved without deleting or renaming native output. No broader support claim is made.

**The readiness-publication finding from rounds 01/02 is closed.** `publishFixtureInfo` at `internal/dev/proc_unix_test.go:286–309` creates a unique sibling file, writes all bytes, rejects write/short-write/close errors, closes it, then renames it into the readiness pathname. The final pathname is therefore absent until complete JSON is available to either polling reader. This fixes the causal empty/partial publication interval rather than accepting parse errors or repeatedly rerunning a flaky writer. Temporary-file cleanup is retained. The reader still immediately rejects malformed published JSON, and setup failure remains an explicit contextual error. Process group, listener, leader exit, inherited-pipe output, caller WaitDelay, grace-period, concurrent Stop, escalation, and final cleanup assertions are unchanged.

## Actual evidence inspected

Raw receipts are under `/Users/tyler/Codex/2026-10-03/task-8/page-redirect-logs/`. These runs were performed by the owner, parent, or independent validator. This reviewer inspected their actual outputs and matching source rather than claiming to have executed them.

- **Current source ordinary checks:** `repaired-canonical-test.log` / `.exit` contains actual canonical `go test -count=1 ./... ./example/...`, exit **0**, all packages passing, including `internal/dev` (37.710s), `internal/gen` (42.543s), and the ordinary example (16.411s). `repaired-canonical-vet.log` / `.exit` contains actual `go vet ./... ./example/...`, exit **0**.
- **Current qualification repair:** `readiness-proposal/product-focused-process-tests.log` / `.exit` records actual Stop-after-leader-exit, concurrent escalation, default/caller-supplied inherited-pipe WaitDelay, malformed JSON, and setup-error checks, exit **0**. It records live exact-owned descendants/listeners before Stop and absent PIDs/groups/listeners afterward. The publication patch and pinned Go rename source/docs are retained beside the log. Earlier proposal-only focused output is not mislabeled as product proof.
- **Skip audit:** the sole source `Skip` remains the existing `runtime.GOOS == "windows"` formatter-fixture guard at `internal/gen/format_test.go:13`, inactive on the current darwin/arm64 execution. No required tooling or new readiness/build test has a skip path. A nonverbose package exit alone was not used to infer this.
- **Original failures retained:** `canonical-test.log` / `.exit` preserves the actual pre-repair readiness parse failure and exit 1. The source fix closes it; the retained failure has not been erased or replaced by focused reruns. `independent-controls/go-app-build.log` preserves the earlier extra-destination-prerender embedding failure.
- **Native build control:** `independent-controls/baseline-build.log` fails the exact authored redirect under the released adapter despite a native `old.html` already existing. `candidate-build.log` succeeds under the redirect implementation. Thus the successful native build assertion distinguishes the broken adapter from merely finding a prewritten file.
- **Handler and wildcard controls:** `independent-controls/frozen-handler-positive.log` passes the native HTTP/static/startup/base contracts. `remove-static-method-guard.log`, `restore-old-endpoint-guard.log`, and `remove-endpoint-bypass.log` are actual exit-1 controls exposing wrong method responses, missing artifact bodies, and unwanted wildcard calls. The wildcard test anchors its invocation count at literal zero and requires `/other` to return literal dynamic bytes with exactly one matching call. `source-receipt.txt` records the controlled product files, independently confirmed unchanged by the current Git diff.
- **Actual embedded app and visual inspection:** `ssr-target-vp-build.log`, `go-embedded-ssr-target-build.log`, `embedded-app-server.log`, and `real-http-browser.log` prove successful native build and ordinary embedded Go compile, then actual HTTP serving and browser navigation. I inspected both `target-script.png` and `target-noscript.png`: both visibly show the ordinary example layout and **Target route**, without a blank screen or error boundary.

| Actual embedded application/control | Result |
| --- | --- |
| GET `/old` | 200 HTML, no Location; native script/meta refresh retains `/target?from=atlas` |
| HEAD `/old` | 200, no Location, zero body bytes |
| GET `/old/?q=1` | 308, `Location: ../old?q=1` |
| POST and OPTIONS at `/old` and `/old/` | All 405, `Allow: GET, HEAD` |
| Matching wildcard endpoint during recorded-path/alias requests | Literal zero calls; `/other` positive control invokes it exactly once |
| Missing listed artifact | Handler startup fails and names the literal missing path |
| Unlisted stray artifact | 404; a recorded positive-control page still serves its literal HTML |
| Browser scripting enabled and disabled | Both reach the query-bearing destination with status 200 and visible `Target route` |

## Findings and qualification boundary

No material findings or genuine nitpicks remain in the reviewed implementation and proof. Both earlier material findings have concrete source fixes and relevant actual validation. The authorized qualification repair preserves rather than relaxes the process cleanup proof.

The earlier full development/production qualification belongs to `d83043cb925c3616983ed6792c3b37f21431d3fd`, as recorded in `qualification-first-head.json` and `qualification-37191800156.log`; it is not evidence of a run at `8e35e35`. The parent separately inspected its native project records. Fresh CI/native qualification at the final review-document checkpoint and eventual `main` qualification remain delivery obligations. This **no findings** verdict neither fabricates those future results nor authorizes bypassing them. No publication or release action was performed by the reviewer.

Outcome: **no findings**.
