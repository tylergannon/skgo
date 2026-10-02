# Independent CPU-efficiency review, round 02

Outcome: **no findings**.

Target: complete current uncommitted performance work on `codex/cpu-efficiency`, base `69cf454`, in `/Users/tyler/Codex/2026-10-02/task-7/skgo-cpu-worktree`. Reviewed against the authorized user continuation, AGENTS.md, agent-protocol and adversarial-review instructions. The earlier finding did not limit this review's scope. No implementation files were edited, changes published, broad suite run, or frontend rebuilt by the reviewer. Validation ran sequentially with other SKGO work held.

## Evidence and implementation assessment

Re-read the full current adapter/retention diff and untracked transform regression. Also considered all evidence inspected in [round 01](202610021825-cpu-efficiency-round-01.md): relevant adapter build/dev paths, SSR pool/cancellation/fetch implementation and assertions, migrated pinned Kit renderer call, installed Svelte renderer/private fields, pinned Goja WeakMap implementation, measurement source, preserved bundles/binaries, build/test logs and CPU/allocation profiles, generator measurements and meaningful contract assertions.

The change stays within the ordinary adapter implementation. It preserves native private fields in modules containing merely misleading comments or quoted text, and lowers modules whose syntax tree contains an async generator or awaited for-of statement. It does not change request isolation, host scheduling, cancellation, pool bounds, or application I/O ownership.

Round 01's material finding is resolved. The old whitespace-sensitive regex is replaced by conservative `/\b(?:async|await)\b/` presence followed by the AST decision. All five independently reproduced comment-interposed fixtures now yield transformed code and a recorded lowering:

```js
async/*comment*/function* rows() { yield 1; }
async function/*comment*/* rows() { yield 1; }
const rows = { async/*comment*/*values() { yield 1; } };
async function rows() { for/*comment*/await (const row of source) consume(row); }
async function rows() { for await/*comment*/(const row of source) consume(row); }
```

Four of these forms are included in the expanded regression alongside ordinary declarations, expressions, methods, for-await, and template-expression syntax. The remaining function/star comment form was checked directly in this review. Earlier pinned-Goja compilation established that these constructs genuinely require lowering. The regression continues to reject lowering driven by JSDoc, comments, string literals, regular-expression literals, or template text. Reinstating the old prefilter fails the added cases; removing AST confirmation fails the previously demonstrated comment negative control; removing actual lowering fails unsupported-syntax cases.

## Final validation

- Independently hashed `ephemeral/cpu/bundle-measured-after.js`, `ephemeral/cpu/bundle-after.js`, and actual `example/web/build/ssr/bundle.js`. All are SHA-256 **a4355238a029137df31b72edea12d9e681ecf440fbf02a74d5c80acbc9761904**. The final prefilter correction did not alter measured runtime bytes.
- `go test ./internal/adapter -run '^TestEngineLoweringUsesSyntaxRatherThanComments$' -count=1 -v`: PASS, 0.571s package time, no skips. Initial sandboxed cache access failed explicitly; approved compiler-cache access succeeded.
- Compiled final identity and actual rebuilt frontend with `go test -c -o /tmp/skgo-cpu-review-final-example.test ./example`: PASS.
- Ran the same 24 focused production-handler tests selected in round 01 with `-test.count=1 -test.v`: PASS, no skips. They cover startup and matching adapter identity, rendered Go values, argument identity, runes fields, concurrent visitors, repeated render isolation, transported instances/methods, retention, request/session/data fetch, endpoints and destination kinds, nested pool capacity/visitor isolation, hydration-fetch serialization, hook rewrites, CORS/no-cors, header filtering, body/header request identity, and fetched cookies. The 500-document retention test passed in 0.44s.
- Round 01's full `internal/ssr` validation remains applicable: every test passed without skips, including late cancelled hosts, runtime replacement/reuse, pool waits, interrupted JavaScript, host budget/lifetime, fetch scheduling, and discarded pending work. These files were unchanged in the final correction; they were not redundantly rerun.

The retention regression remains load-bearing: positively asserted successful documents and independently fixed 32 MiB live-heap ceiling with handler reachability preserved. In round 01 the original preserved server at 500 successful home requests retained **72,596,848 bytes** after three GCs, versus the test ceiling **33,554,432 bytes**. Final candidate passes. This directly substantiates the test comment's original >60 MiB retention claim.

## Performance evidence and limits

No accounting defect was found invalidating the demonstrated improvement. Matched home samples measured process user+system CPU **4.1757965 -> 2.0270475 ms/request**, allocations **2,598,624 -> 2,028,266 bytes/request**, and post-GC live heap **270,113,592 -> 6,649,552 bytes** after 2,000 renders. Matched 1,000 nested-page samples measured CPU **5.03299 -> 2.281026 ms/request**, allocations **2,508,470 -> 1,799,330 bytes/request**, and live heap **194,644,488 -> 7,956,248 bytes**. Final byte identity preserves the measured runtime change. The causal explanation is supported by source and negative-control behavior: misleading JSDoc caused private-field downleveling; Goja WeakMap value/key cycles retained render trees, increasing GC work. Actual final bundle execution confirms native field support.

These are local sequential handler measurements, including equal recorder/request/fixture-check overhead. They establish the avoidable waste and its removal, not service throughput, an architectural ceiling, or statistical repeated-run confidence. Background FileProvider contention contaminates wall/p95 results. Startup/idle improvement is not established by short fluctuating samples. Final cumulative allocation profiles include startup/warmup/post-loop work; per-request deltas separately cover the measured warm loop.

Generator CPU is separate from SSR. Existing logs identify slow elapsed tests and inspected assertions show meaningful diagnostics, wire/type projection, stale recovery and regeneration contracts. Their elapsed times include shared fixture/lane waits, while parent-only CPU profiling excludes children; exact per-test CPU attribution is still not demonstrated. The GOMAXPROCS comparison is resource policy, not this algorithmic improvement. No test was weakened to obtain the result.

No material findings or genuine nitpicks remain in the current implementation and constrained proof. This review does not claim all residual Goja/SSR cost is solved. Broad `just test`, both e2e modes on main, browser inspection, and release/merge readiness were outside this validation dispatch. Nothing was published.
