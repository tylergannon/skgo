# Independent CPU-efficiency review, round 01

Outcome: **material findings remain**.

Target: uncommitted adapter lowering fix and Go regressions on `codex/cpu-efficiency`, base `69cf454`, in `/Users/tyler/Codex/2026-10-02/task-7/skgo-cpu-worktree`. No implementation files were edited, frontend rebuilt, broad suite run, or changes published by this review. One controlled process ran at a time. Parent held other SKGO work.

## Finding

**Issue — the detector still lets valid unsupported syntax bypass lowering.** `internal/adapter/skgo-adapter/env.js:522` returns before parsing unless the old regular expression at line 173 matches adjacent whitespace-separated tokens. Comments are legal between these tokens. The user's requirement, restated in the authorized continuation, is to preserve genuinely unsupported syntax's lowering path while fixing ordinary SSR CPU waste.

Directly importing the current `engineTransform` from the example app directory returned `null` for all of these valid JavaScript fixtures:

```js
async/*comment*/function* rows() { yield 1; }
async function/*comment*/* rows() { yield 1; }
const rows = { async/*comment*/*values() { yield 1; } };
async function rows() { for/*comment*/await (const row of source) consume(row); }
async function rows() { for await/*comment*/(const row of source) consume(row); }
```

A temporary Go program invoking the actual pinned `goja.Compile` rejected every fixture: the first two with “Async generators are not supported yet”, the method and loops with syntax errors. In dev, a module using such a construct can reach Goja unlowered; a native production bundler output preserving it can do the same. The issue existed in the old regex, rather than being newly introduced by AST confirmation, but remains in the touched detector and the regression's unsupported-syntax coverage. Use a conservative prefilter that cannot exclude these constructs before inspecting their AST, and add comment-interposed fixtures. Do not fix it by lowering renderer modules merely because comments contain these words.

## Validation and regression sensitivity

- `go test ./internal/adapter -run '^TestEngineLoweringUsesSyntaxRatherThanComments$' -count=1 -v`: PASS, 0.700s package time, no skips.
- `go test ./internal/ssr -count=1 -v`: PASS, 0.968s package time, no skips. Includes cancellation with noncooperative late hosts, cancelled pool waits, JavaScript interruption/replacement, worker budget/lifetime, simultaneous host calls, fetch scheduling, runtime reuse, pending work isolation, and class-field execution.
- Compiled the candidate example once with `go test -c -o /tmp/skgo-cpu-review-example.test ./example`. Initial sandboxed compilation could not read a Go cache file; approved compiler-cache access succeeded. The binary embeds the actual rebuilt candidate frontend, not the preserved baseline.
- Ran 24 selected example tests in that binary with `-test.count=1 -test.v`: PASS, no skips. These covered fresh bundle startup, Go-rendered home/query values, runes class fields, simultaneous pages, repeated render isolation, transported values/methods, 500-document retention, request/session fetch, data navigation, endpoint response, all destination kinds, nested rendering with pool capacity one and two under 12 visitors, fetched-response hydration serialization, hook rewrites, CORS/no-cors, header filtering, request-body/header identity, and fetched cookies.
- The new memory regression executes the same production-handler constructor the real command uses, positively asserts HTTP 200 and the Go-produced site-name markup in each of 500 responses, forces three GCs, and keeps the handler reachable after `ReadMemStats`. Its 32 MiB absolute ceiling is independent of the renderer's current counter/measurement. It passed against the candidate.
- Ran preserved `./ephemeral/cpu/measure-before.bin -n 500 -path /` independently. It positively checked each home document and retained its handler through three GCs. Live heap was **72,596,848 bytes**, exceeding the regression's **33,554,432-byte** ceiling by over 2x. Thus the original bundle would fail the memory contract, and the test's “500 home documents kept over 60 MiB” comment is now directly supported. This is a matched-path negative control, not a recompiled exact test against old assets.
- Executed the old `HEAD` transform in memory alongside the candidate on the regression's JSDoc fixture. Old transform lowered it; candidate returned `null`. The added comment regression fails if AST confirmation is removed. The ordinary unsupported fixtures in the new test also require a transformed result and recorded lowering; deleting lowering makes them fail. The comment-interposed cases above remain uncovered.

## Measurement review

Inspected `ephemeral/cpu/measure/main.go`, original and rebuilt bundles/binaries, build logs, before/after home and nested logs, CPU/allocation profiles, generator logs, worklog, relevant adapter/pool/fetch code, installed Svelte renderer, pinned Goja WeakMap implementation, and the migrated pinned Kit `page/render.js`.

CPU accounting uses `getrusage(RUSAGE_SELF)` user plus system time, rather than runtime capacity/idle metrics. It measures the actual production-handler constructor and in-process requests, including request/recorder allocation and fixture checks identically before/after. First request is separate; measured warm loops exclude the following forced GCs and pauses. Default runtime capacity is 10 in both compared logs. CPU sampling is enabled during the warm loop in both profile comparisons. Allocations/request use `TotalAlloc` and `Mallocs` deltas around that loop. The allocation profile written at process end is cumulative, including startup, warmup, and post-loop GC; it must not be described as a warm-only allocation profile.

Matched existing 2,000-home logs: CPU **4.1757965 -> 2.0270475 ms/request**; allocations **2,598,624 -> 2,028,266 bytes/request**; live heap after three GCs **270,113,592 -> 6,649,552 bytes**. Matched existing 1,000 nested-page logs: CPU **5.03299 -> 2.281026 ms/request**; allocations **2,508,470 -> 1,799,330 bytes/request**; live heap **194,644,488 -> 7,956,248 bytes**. These are controlled local sequential samples, not throughput/capacity claims or a statistical distribution of repeats. Background FileProvider contention contaminates wall times and p95; it does not belong to this process's CPU total. Startup and idle values fluctuate between these short samples, so there is no demonstrated startup/idle improvement claim here.

The mechanism is credible and independently inspected: Svelte's `render_async` JSDoc immediately precedes `*/`, triggering the old `async\s*\*` regex; lowering to ES2017 rewrites private fields to WeakMaps. Pinned Goja holds WeakMap values strongly while keys are weak, so a value closing over/referencing its key creates the observed retention path. Preserving the renderer's native private fields removes that avoidable path. Actual candidate bundle startup/render tests demonstrate Goja support, rather than relying on its advertised target. CPU profiles show the large GC scan reduction; residual scheduling/GC cost remains. This does not establish that all SSR/fetch overhead is solved or that Goja is intrinsically cheap enough for every application.

The generator evidence is separate: baseline process tree used 133.49 CPU seconds over 26.21 wall seconds, while its parent-only profile sampled 18.93 seconds dominated by GC/package loading. The slowest recorded test elapsed times include shared fixture/lane waits: nullable projection 16.66s, named deferred names 16.52s, real authored-location advice 15.75s, read-only advice/stale-link check 13.58s, form-client regeneration 13.31s. Inspected assertions protect real contracts (authored diagnostics and failures, generated nullable/deferred projections, stale recovery, byte-exact repeat generation, decoder evolution and compilation), rather than purposeless stress loops. Per-test CPU attribution is not established by those elapsed rankings. The resource-policy GOMAXPROCS comparison is not an algorithmic improvement and should remain described separately.

No benchmark-accounting defect invalidating the measured retention/CPU improvement was found. No visitor-isolation or cancellation failure was found in focused validation. Broad `just test`, both main e2e modes, and human/browser inspection were not performed by this constrained review; no merge/release-readiness verdict is implied.
