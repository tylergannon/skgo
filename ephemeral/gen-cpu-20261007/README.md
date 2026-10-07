# Why `gen.test` consumes CPU

Investigation on 2026-10-07, against `c6906b7b6029c27f5311e080fd0ec97587b2aca4`
(current main at the start). Go 1.27.1, darwin/arm64, 10 logical CPUs,
24 GiB RAM, installed Kit 3.0.0 and Polytype 1.4.0.

**The observed process is the generator's test executable. Its main expense is
repeated Go package parsing/type-checking and the resulting memory churn. There
are opportunities to reduce that work while retaining the assertions. Simply
removing dependency analysis loses an existing correctness check.**

This is an investigation artifact. No generator, server, or normal test-suite
source was changed. The experimental sources below are `.go.txt` files, used
only through Go compiler overlays; ordinary `go test ./...` does not compile
them. Serving-request CPU was not measured, so these results establish a
generation/validation cost, not the framework's request-time overhead.

## Full generator suite: one profiled execution

The compiled `./internal/gen` suite ran once, with the normal default test
parallelism. All 103 top-level tests passed; the captured output contains no
test skips. This is not a claim that the whole repository or browser suite was
validated.

| Measurement | Result | Evidence |
| --- | ---: | --- |
| Wall time, including profile finalization | 222.82 s | `default.resources.txt` |
| CPU sampled inside `gen.test` | 694.03 CPU-seconds, averaging 3.13 cores during the profile | `default.cpu-top.txt` |
| Timed execution user + system CPU, including children | 1,131.61 CPU-seconds | `default.resources.txt` |
| Maximum resident size reported by `time` | 8.24 GiB | `default.resources.txt` |
| Estimated cumulative allocation inside `gen.test` | 115.64 GiB | `default.alloc-cumulative.txt` |

Live process observations reached 531–572% CPU and roughly 7 GiB resident
memory. The allocation number is all allocations over time, not simultaneous
live memory or a demonstrated leak.

The cumulative CPU profile attributes 36.72% to `go/types.(*Checker).Files`
and 25.67% to `go/parser.ParseFile`. Background marking, allocation-assisted
marking, and sweeping are also substantial: 16.33%, 13.89%, and 18.83%
respectively. These are overlapping call-tree categories; do not add them to
the parsing/type-checking percentages. Parsing and type-checking account for
about 97.4% of sampled allocated bytes. This is much more than the cost of
printing generated Go/TypeScript declarations.

`default.test-durations.txt` lists elapsed test times. Tests run concurrently,
some wait on shared results or a shared fixture lock, and the measurements
include child processes. A long test duration is not an exclusive CPU attribution.

Other agents were validating this repository on the same machine during this
investigation. Wall times are therefore observations under contention, not
stable benchmark promises. CPU/allocation profiles cover only the instrumented
process; they exclude child compilers, frontend builds, and CLI executions.

## A repeated generation still reads dependency source

`probe_test.go.txt` calls the real `gen.Run` twice on one unchanged temporary
copy of the example, with its current generated files already present. It
observes each package parser callback, labels CPU samples by invocation, and
records allocation deltas. No frontend build runs here; the fixture links only
Kit's package, not the frontend formatter. Child `go list`/compiler work still
occurs. No persistent cache was deleted.

| Measurement per generation | Existing generated tree | Repeat with unchanged input |
| --- | ---: | ---: |
| Wall time | 12.14 s | 10.61 s |
| Allocated memory in process | 1,504.9 MiB | 1,504.1 MiB |
| Application source parse calls | 188 | 188 |
| SKGo checkout source parse calls | 57 | 57 |
| Standard-library source parse calls | 1,182 | 1,182 |
| Module-cache source parse calls | 212 | 212 |

The largest dependency source was `golang.org/x/text/collate/tables.go`, nearly
5 MB. Crypto and Unicode tables, the runtime, and regexp dependencies also
appear. A warm Go build cache does not prevent these in-process source parses.
The measured repetition here is between invocations; the largest dependency
files were parsed once within each of these particular invocations.

The direct cause in this existing-example probe is `internal/gen/load_params.go:249`: the matcher loader asks
for `NeedSyntax | NeedTypesInfo | NeedDeps`. In pinned
`golang.org/x/tools/go/packages` v0.50.0, `NeedDeps` applies the requested source
and type-information modes to dependency packages too (`packages.go:805-810`).

There is a second source-loading path worth investigating, independently of
that flag. `internal/gen/package_overlay.go:53` retains `Config.Overlay` when
an overlaid file does not yet exist. In go/packages v0.50.0, a nonempty overlay
invalidates dependency export data indiscriminately (`packages.go:802`),
forcing source loading even without `NeedDeps`. Fresh generation fixtures
create exactly this situation. The focused existing-tree probe above did not
quantify that fallback separately. Claude's independent review later reproduced
it on a fresh endpoint fixture: 1,090 standard-library source parses and
301.3 MiB allocated, with no `params.go` and no matcher `NeedDeps` load.

Reuse already exists within an invocation: `internal/gen/packages.go:11`
caches a loaded grammar package for type and codec generation. The probe still
shows some application files parsed two or four times across distinct loading
passes. That is another candidate for investigation, not proof that those
passes can safely be merged.

## Rejected shortcut: removing `NeedDeps`

A compiler-overlay experiment changed only the matcher loader's `NeedDeps`
flag, leaving the project's implementation untouched. The same two generation
probes then produced:

| Measurement | Existing generated tree | Repeat with unchanged input |
| --- | ---: | ---: |
| Wall time | 4.34 s | 3.62 s |
| Allocated memory in process | 389.9 MiB | 387.3 MiB |
| Standard-library/dependency source parse calls | 0 | 0 |

The experiment attributes approximately 1.1 GiB of source-analysis allocation
per invocation to the changed flag: about 74% of the original allocation. The two-probe
CPU profile fell from 11.19 to 2.87 sampled CPU-seconds. These are paired
observations, not an accepted optimization.

**The same experimental executable failed
`TestSharedParamsSourceDiagnostics/transitive-generated`.**
`validateSharedType` (`internal/gen/shared_params.go:225`) walks
`go/types.Package.Imports()` to reject domain dependencies reaching generated
bindings/params. Loading compiler export data alone did not preserve the
source-import information needed by that guard. The case proceeded past the
expected refusal and panicked while writing with the test's nil `Logf`.
See `no-deps.contracts.log` and `no-deps.contracts.resources.txt` for the exact
failure. The other selected test, `TestTypedLoadActualParameterDependencies`,
was paused when the process panicked and did not execute.

The unmodified full-suite execution passed this case. Do not apply
`no-deps.patch` as a fix. A viable optimization needs to preserve the complete
import-ownership and compiler-error checks while avoiding unnecessary syntax
and type-information construction for dependencies.

### Clarification: the diagnostic does not justify repeated full analysis

The `transitive-generated` regression case runs once per full generator-suite
execution. The repeated expensive work is production generator behavior:
`readGoParamMatchers` requests dependency syntax/type information for each
invocation. The failing experiment establishes that the unchanged validator
cannot rely on export-loaded `go/types.Package.Imports()` alone. It does not
establish that this guard requires full dependency source type-checking.

The concrete hazard is a matcher domain package importing generated params.
Generated params in turn imports that domain package to spell the matcher's
result type, creating an import cycle. A proof about one fixture cannot prove
that a different application, or a changed import graph, is free of that cycle.
For the same unchanged inputs, however, one validation result can be reused.

Pinned go/packages exposes a cheaper metadata mode:
`NeedName | NeedImports | NeedDeps`, with no `NeedSyntax`, `NeedTypes`, or
`NeedTypesInfo`. It supplies the dependency import graph without requesting
dependency parsing/type-checking. That provides a candidate way to retain the
fast type-loading experiment and separately validate the source-import graph.
This combination has not been implemented or measured here, and export/overlay
invalidation and compiler diagnostics still need verification.

There is also repeated validation within a single invocation:
`writeSharedParams` calls `validateSharedType` for each matched route parameter
before deduplicating equivalent type alternatives (`shared_params.go:366-375`).
The validator builds its forbidden-package list and starts a fresh graph walk
each time. Checking each distinct type/import graph once for that invocation
could preserve the same diagnostics while reusing its result across routes.

For tests, share a generated result among all assertions whose inputs and
starting state match. For changed inputs, retain a fresh input-dependent check,
which need not use the expensive dependency-syntax mode. Rejecting the exact
one-flag patch should not be read as rejecting either optimization.

## Test reuse without lowering confidence

The suite already implements the user's “expensive operation once, assert many
times” approach in several places:

- `main_test.go`: builds the real SKGo CLI once.
- `evolution_test.go`: shares the evolved, stale, and unchanged example
  generations across client and recovery assertions.
- `prerender_production_fixture_test.go`: shares the frontend/server/test-binary
  build across three independently executed production contracts.
- `check_test.go`: shares pristine CLI results and reuses a serialized mutable
  fixture for checks whose planted source errors differ.

A concrete remaining duplicate is
`TestAServerRouteBecomesTheModuleKitCompiles` and
`TestAppWithoutPageActionsStillHasAnActionRegistry`. Both call
`foreignFixture(t, "", {"app/web/src/routes/api/thing/server.go": endpointSource})`
and then `Run(cfg)` with the same configuration. Their independent module,
registration, manifest, and empty-action-registry assertions can read one
generated result. They took 5.93 s and 2.56 s respectively in this concurrent
run; those times are not an additive estimate of wall-time savings.

Another candidate is the no-argument remote projection assertion: its primitive
query is already present in the broader arity fixture. Review whether its
negative import/wiretype checks can read that result without losing the narrow
claim. Explicit-language and auto-detected-language tests, by contrast, exercise
different configuration paths even when their emitted bytes match.

Keep distinct executions when input or starting state is the behavior being
tested: language switching, changed matcher/declaration types, deleted files,
stale syntax, stale symbols, and generation idempotence. Repeating generation
in those cases is load-bearing. The existing failing diagnostic in the
experiment is direct evidence that removing expensive analysis indiscriminately
would reduce confidence.

One concurrency detail also amplifies the cost: `main_test.go:29` calls
`os.Setenv("GOMAXPROCS", "2")` inside `TestMain`. That limits future child Go
processes but does not change the running test process's scheduler. Both probes
confirmed `GOMAXPROCS=10`. Go's testing package also initializes its default
parallel-test limit before `TestMain`. A concurrency adjustment must explicitly
address the parent scheduler and test parallelism; two cores per child is not
a two-core budget for the entire suite. No concurrency settings were changed.

The first optimization target should be repeated package analysis, with its
transitive checks preserved. Sharing identical generation results is a separate,
smaller opportunity. Keep every existing assertion and the normal validation
entrypoint; do not introduce a reduced-confidence alternate suite.

## Independent Claude Fable review

The written claims are in `claims-for-review.md`. Claude Fable 5.1 reviewed them
in session `fbc0aa23-df12-4a1a-b9cd-e9a929eecdd1`; its complete report is
`ephemeral/reviews/202610070852-gen-cpu-claude-fable-round-01.md`. The outcome is
**material findings remain**, not implementation approval.

The review supports reuse once per identical fixture/input/starting state and
agrees that dependency import validation does not require full dependency
source type-checking. It identifies a more useful existing shared result than
new caching machinery: compiler export data, already maintained and invalidated
by Go's build cache. Both the matcher `NeedDeps` mode and fresh-file overlays
prevent that reuse. The reviewer's fresh-fixture probe independently establishes
the second path; a single full-suite observation tagged by load site and
overlay-fallback decision would quantify their respective contributions.

Do not read the earlier paired example probe as attribution of the entire
suite to matcher loading. Do not read the failed no-deps experiment as the only
failure it could cause: its panic aborted execution of the remaining selected
test. The metadata-based remedy is still a proposal, not demonstrated combined
behavior.

Claude ranks restoring compiler-export reuse above fixture consolidation and
finds no profiling evidence that memoizing `validateSharedType` would materially
save resources. It also recommends measuring invocation-level `GOMAXPROCS`,
test `-parallel`, and package `-p`; considering GC tuning only after allocation
and RSS drop; retaining `-count=1` because subprocess inputs are not safely
covered by the test result cache; and avoiding a separate fast suite or a
persistent SKGo analysis cache.

One implementation suggestion in the review needs qualification before use:
materializing generated files earlier must preserve failed-generation output
retention. Although `writeSharedParams` and `writeLoadParams` call `write`,
`emit.go:1134` routes those calls to the generation buffer when
`cfg.generation != nil`; publication occurs later in `output.go`.
`output_test.go` checks that source errors and formatter failures do not publish
changed output. The fresh-overlay problem is demonstrated; early publication
is not an accepted solution. An implementer must preserve those contracts and
read-only checks while restoring export reuse.

## Read or reproduce the profiles

From the repository root, `go tool pprof` can inspect the saved profiles without
the discarded executable binaries:

```console
go tool pprof -top ephemeral/gen-cpu-20261007/default.cpu.pprof
go tool pprof -top -alloc_space ephemeral/gen-cpu-20261007/default.heap.pprof
go tool pprof -tags ephemeral/gen-cpu-20261007/generation.cpu.pprof
```

To reproduce only the focused observation at this source revision:

```console
go test -overlay=ephemeral/gen-cpu-20261007/probe.overlay.json -c -o /tmp/skgo-gen-cpu-probe.test ./internal/gen
cd internal/gen
/usr/bin/time -l /tmp/skgo-gen-cpu-probe.test -test.run='^TestCPUProbe$' -test.count=1 -test.v
```

`no-deps.overlay.json` selects the rejected counterfactual. The copied
instrumented loader files are tied to the recorded commit; refresh their
observation hook against current source before using them on a changed revision.
The `.pprof`, logs, resource measurements, and text summaries beside this file
are the recorded evidence. The complete suite was profiled once; each paired
focused probe generated twice. The rejected experiment's diagnostic check ran
once and stopped at its failure. No browser or request-throughput benchmark ran.
