# Independent generator resource validation

Scope: the uncommitted implementation on `codex/gen-cpu-investigation`, based
on `2315164`, assessed against the implementation plan. This validator changed
only files under `ephemeral/reviews/`. The implementing agent owns the complete
normal suite, profiling, and final delivery.

Outcome: no remaining material regression found in the focused contracts after
repair. This is not a complete-suite or release-readiness verdict.

## Problems found and repaired

1. The first staging implementation remapped AST filenames away from
   `packages.GoFiles`, breaking Polytype named-type resolution. The actual
   typed-handler, edited generated-consumer, and stale-output contracts failed
   (`gen-resource-validation/focused.log`). The repaired implementation keeps
   token file names aligned and maps adjusted positions; the same contracts
   pass (`focused-repaired.log`).
2. A new overlaid Go file beside authored `//go:embed value.txt` failed with
   `cannot embed irregular file value.txt`: staged assets were symlinks. An
   independent fixture passed under ordinary `Config.Overlay` and failed under
   staging (`embed-ordinary.log`, `embed-local.log`). The repaired stage uses
   regular files and materializes embedded asset directories. The independent
   fixture and permanent nested-asset/workspace contracts now pass
   (`independent-repaired.log`, `new-permanent-contracts.log`).
3. Fresh-file parser errors still named the temporary module after AST position
   remapping because parser errors captured their positions before that change.
   The independent parser case failed while compiler diagnostics passed
   (`fresh-diagnostics.log`). Explicit scanner-error remapping repairs both
   cases (`independent-repaired.log`, `new-permanent-contracts.log`).

## Load-bearing assertions

The shared endpoint/action fixture retains separate literal assertions. A
compiler overlay changed only the test helper to remove one generated output
fragment after successful generation. All eight removals caused the intended
assertion to fail, while the other consumer passed:

| Removed fragment | Failing assertion |
| --- | --- |
| GET export | endpoint module GET export |
| POST export | endpoint module POST export |
| throwing stub | endpoint module Go-only stub |
| GET registration | endpoint GET registration |
| POST registration | endpoint POST registration |
| manifest route | endpoint manifest route |
| manifest GET | endpoint manifest method |
| empty Actions registry | action-registry consumer |

Evidence: `gen-resource-validation/fault-*.log` and the passing clean pair in
`endpoint-clean.log`. The faults use fixture literals, rather than a baseline
computed from generated output. They do not modify product sources or ordinary
tests. The package-lifetime helper is protected by `sync.Once`; each test reads
the result, and transitions retain their separate generators.

Removing only recursive source-import metadata traversal made exactly the
transitive-generated diagnostic fail with `<nil>`; the other source-diagnostic
cases still passed (`fault-import-guard.log`). The temporary test overlay
initialized `Logf`, so an incidental nil-logger panic cannot stand in for the
intended assertion. Bypassing the internal transitive assertion then exercising
public Run independently failed on the late compiler-cycle diagnostic instead
of the expected source-located ownership refusal
(`fault-public-import-guard.log`). Reversing public-path order independently
checks Check as well (`fault-public-check-import-guard.log`). The unmodified
public paths and output-preservation assertions pass
(`public-diagnostics-rechecked.log`).

Replacing fresh staging with the original ordinary-overlay fallback made the
new source-analysis assertion fail with 1,090 dependency parses
(`fault-fresh-exports.log`). Thus the added resource guard detects precisely
the accidental analysis path being eliminated.

## Focused executions

`focused-repaired.log` records all eleven selected top-level tests and their
subcases passing, without skips:

```
go test -count=1 -v -run '^(TestReadOnlyPackages.*|TestSharedParamsSourceDiagnostics|TestTypedLoadActualParameterDependencies|TestSharedParamsGeneratedConsumers|TestGenerationOwnsOneFileAndIgnoresPreviousDeclarations|TestFormatterFailureDoesNotPublishGeneratedOutput|TestAServerRouteBecomesTheModuleKitCompiles|TestAppWithoutPageActionsStillHasAnActionRegistry)$' ./internal/gen
```

This includes external-driver overlays, current and changed dependencies,
aliases, actual typed-load requests and dependency tracking, edited generated
consumers, stale syntax/runtime declarations, absent output, authored collision,
formatter failure, and the shared fixture's independent consumers. Actual
consumer test output is retained in that log. Package execution took 25.306s
under concurrent machine activity; this timing is not a performance bound.

Permanent fresh staging additions were run after they were added:

```
go test -count=1 -v -run '^TestFreshPackages' ./internal/gen
go test -count=1 -v -run '^TestSharedParamsSourceDiagnostics$' ./internal/gen
```

Both passed. Independent embed/parser/compiler fixtures and fault checks use
ordinary Go tests, injected with `go test -overlay`; the saved JSON and `.go.txt`
inputs make each experiment inspectable and reproducible. Expected fault
executions exit nonzero and are not reported as passing software.

## Measurement assessment and limits

The parser instrumentation wraps the existing callback and retains its source,
filename, parser mode, and error. It does not change requested loader flags or
publish output. The baseline's mode-based matcher label needed correction:
after removing `NeedDeps`, optimized matcher syntax loads would otherwise be
labeled as scanner loads. The saved after-probe instead identifies the calling
load site. Metadata-only loads are correctly not counted as syntax parses.

The observed after probes show zero standard-library/module-cache syntax
parses on unchanged example generation, about 395/394 MiB allocated per Run,
and zero standard-library parses with 6.3 MiB allocated on the fresh endpoint.
These support less parent source analysis. They do not alone establish
process-tree CPU/RSS or suite-wide savings; the implementing agent's captured
normal invocation must establish those and account for child work, cache
conditions, skips, and contention. Instrumented allocation comparisons use the
same parser wrapper on each side. A parent CPU profile excludes children.

One staging limitation remains: `packages.Load(cfg, "./...")` omits untouched
directories exposed through symlinks. An independent fixture returns only its
overlaid route package instead of that route and its untouched sibling
(`wildcard.log`). The current matcher, scanner, and Polytype paths request exact
import paths, so no current application failure was found. Keep the helper's
exact-package assumption explicit; do not claim generic wildcard equivalence.
Existing real-path alias and workspace exact-package contracts pass.

Cross-filesystem hardlink fallback was read but not exercised: this worktree
and its temporary directory are on the same filesystem. External-driver
fallback is deliberately preserved, and those drivers may choose source
loading. Required named-type projection source loading remains legitimate.
No Kit-facing interface changes were found; browser qualification and opening
the example application remain owned by the delivery workflow if merge/release
is requested.
