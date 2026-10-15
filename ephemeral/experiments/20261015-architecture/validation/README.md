# Independent load-bearing experiment validation — 2026-10-15

Both native preparation and renderer experiment claims survived an independent
positive execution. Three deliberate mutation controls failed at their intended
assertions. This validates these bounded fixture claims; it does not select an
architecture or establish integrated save-to-browser latency.

Research began at the cache's `index/README.md` and authoritative
`research/dev-experience-20261015/GOAL.md`, followed `compiler-retention.md` and
`dev-framework-runtime.md`, then inspected the actual native experiment source
and `go-preparation/quiet-comparison.log`. Installed Kit was checked as 3.0.0.
The existing native/root coverage and legacy host admission limitations were
preserved, rather than promoted into additional passing requirements.

## Executed controls

| Run | Expected result | Actual result |
| --- | --- | --- |
| Native second install omitted | Literal HTTP expectation must catch an old response | FAIL, wanted 2000 but actual HTTP total was 1000; 3.45s test body |
| Retained analysis capture keeps dependency Offset=0 while disk/compiler archive changes to 77 | Independent typed constant expectation must catch stale analysis | FAIL, `dependency analysis wanted literal 77`; 3.60s test body |
| Unmodified native experiment, all four modes | Body edits, syntax rejection/repair and dependency edits work through actual host | PASS, 11.59s test body; 20 installations, 24 literal HTTP assertions, four syntax rejections and four analyzed-77 assertions |
| Retained renderer consumes event cursor but does not deliver events to ModuleRunner | At least a stale pooled renderer must fail literal response expectations | FAIL, concurrent HTTP returned `Component A: Dependency A` while `Component A: Dependency 1` was required; 4.34s test body |
| Unmodified renderer experiment copied into this owned directory | All compared paths render changes, import addition and repair through pooled goja | PASS, 6.58s test body |

The native negative controls narrow modes to retained-view only, keeping the
literal body, syntax/repair and constant expectations. The stale-analysis
mutation changes only the captured dependency source presented to the retained
view; the disk source and refreshed native archive remain 77. The omitted-install
mutation leaves the second candidate compiled but checks HTTP without publishing
it. Neither mutation changes the expected value.

The renderer control replaces `await receive(payload)` with `void payload` in
the retained runner bootstrap. A serial render initially happened to show the new
value, but concurrent requests exposed a stale pooled runtime. Consequently the
concurrent fixture is materially stronger than a single successful render. It
also emits its routine `HTTP ... literal=...` diagnostic after collecting errors;
that line by itself is not a passing assertion. The test failed as required.

## Assertion boundary

Native `architecture_preparation_test.go` lines 148–174 decode the real generated
HTTP envelope and compare total against the supplied literal. Lines 177–195
check host install response and unchanged PID before fetching. Lines 330–343
independently inspect the imported `go/types.Const`, rather than deriving an
expectation from the compiler result. The positive run exercised four fresh
constant analyses and four real 5077 responses.

Renderer `renderer_test.go` lines 187–198 require positive page text, and lines
200–247 require every one of eight concurrent HTTP results to contain the
literal. Lines 291–292 apply both to each retained dependency change. The repair
check at lines 349–353 requires `Component B: Repaired 901`. Blank output cannot
satisfy those checks. Expected malformed source failures are separate assertions
inside the passing positive experiment.

The renderer's `Created()` is lifetime creations, not live pool occupancy. The
fixture independently permits two created runtimes normally and three after its
one deliberately failed render discards a runtime. This is consistent with
`internal/ssr/ssr.go` render cleanup; no observed baseline is used to manufacture
the limit. It does not prove long-running bounded retention.

Root-only native modes still omit complete application caller coverage. The host
uses legacy Lower and does not pin production HostView dependency identity;
constant freshness does not establish production admission. Renderer topology,
metadata, application I/O, browser behavior and general source-map coverage are
not established by this fixture. Timing above identifies executions and is not a
new performance comparison; use the original coordinated comparison logs.

## Reproduction and preservation

`COMMANDS.txt` records commands run from the checkout root. Native controls use
Go overlays; the actual native files were never edited. The renderer was copied
into this directory to avoid modifying its owner's files and fixture logs.
`source/*.go.txt` retains exact mutation sources without making deliberately
failing controls discoverable by ordinary Go package tests. Overlay paths are
absolute to this checkout; relocate their JSON paths when reproducing elsewhere.
The `.txt` suffix does not affect Go's overlay source reading.

`native-*.log`, `renderer-*.log` and corresponding Vite logs preserve all output,
including intentional failure. There were no unexpected execution failures and
no skips. `renderer-positive-vite.log` is the final positive Vite output;
`vite-last.log` is its same local capture. All temporary application/renderer
fixtures were cleaned by their tests. No production behavior was modified.
