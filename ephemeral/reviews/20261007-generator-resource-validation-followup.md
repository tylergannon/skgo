# Independent validation follow-up

Scope: latest staged parser location mapping and final measurement overlay.
Only review artifacts and temporary Go compiler-overlay probes were changed.
No full suite was run by this validator.

## Parser simplification

The initial standard directive `//line original:1` dropped columns. Both
permanent fresh parser/compiler diagnostic cases failed. Evidence:
`gen-resource-validation/line-directive-contracts.log`. Adding `:1:1` repairs
those cases; all `TestFreshPackages` cases pass in `line-column-fixed.log`.
`TestGenerationOwnsOneFileAndIgnoresPreviousDeclarations` passed with the new
directive approach, preserving the Polytype named-type path and stale/absent
generated-file transitions. Its execution is in `line-directive-contracts.log`;
that command failed overall solely because of the initial column loss.

One material mapping problem was then found: an authored later relative
directive `//line alternate.go:40:5` resolves against the staged token filename.
The resulting compiler diagnostic names `skgo-package-view-*/alternate.go`
instead of the original module's `alternate.go`. The same input under ordinary
`Config.Overlay` names the original module and passes. Evidence:
`later-line-directive.log` (introduced failure) and `later-line-ordinary.log`
(control). The prefix preserves line/column values but alone does not preserve
relative filename semantics. This was reported to the implementing agent for
repair and revalidation.

Focused commands:

```
go test -count=1 -v -run '^(TestFreshPackages.*|TestGenerationOwnsOneFileAndIgnoresPreviousDeclarations)$' ./internal/gen
go test -count=1 -v -run '^TestFreshPackages' ./internal/gen
go test -overlay=ephemeral/reviews/gen-resource-validation/probe.overlay.json -count=1 -v -run '^TestIndependentLaterLineDirective$' ./internal/gen
go test -overlay=ephemeral/reviews/gen-resource-validation/probe_later_ordinary.overlay.json -count=1 -v -run '^TestIndependentLaterLineDirective$' ./internal/gen
```

## Final measurement overlay

Read `final.overlay.json`, `final_package_overlay.go.txt`, `observer.go.txt`,
`final_main_test.go.txt`, and `final_probe_test.go.txt` under
`ephemeral/gen-cpu-20261007/implementation/`.

No changed loader mode, build flag, source publication, skipped contract, or
altered generator result was found in that instrumentation. The package loader
overlay adds only the observer invocation. The observer passes filename/source
to the underlying parser, keeps default parser modes, labels caller sites, and
maps staged filenames to logical module paths for counts. Mutexes protect
parser collection. The probe tests are sequential ordinary Go tests; parallel
suite tests resume after those probes. TestMain retains ordinary environment,
package temporary-directory lifetime, test execution, and exit status.

Interpret the measurements with these limits:

- Parse counts are for the observed `gen.test` process. Child CLI processes
  do not publish their counters into that parent's map. CPU profiles likewise
  exclude children; process-tree timing is needed for broader savings.
- Byte totals for staged files include the synthetic directive prefix. These
  are parser input byte totals, not exact authored file sizes.
- The extra unchanged/fresh probe tests belong in total suite CPU/allocation
  measurements. Both the attributable baseline and final run contain them.
- The final TestMain reads total allocation after stopping CPU profiling, then
  forces GC for the heap profile. That profiling-tail GC can appear in external
  time/RSS observations while being outside the CPU profile. Do not treat
  slightly different measurement windows as exact equivalence.
- Probe allocation deltas use process-global MemStats. Their sequential
  scheduling avoids attribution to parallel test bodies, while runtime and
  profiler background activity may still contribute small amounts.
- Parent `GOMAXPROCS` remains initialized before TestMain's existing child
  environment setting. The instrumentation does not change that concurrency.
- Final overlay references are local absolute paths. Preserve the files as
  evidence; relocating them requires updating their overlay paths.

The implementing agent owns the normal test run, prerequisite correction,
resource comparison, and final conclusion. This artifact does not claim that
the normal suite passed or establish process-tree savings.
