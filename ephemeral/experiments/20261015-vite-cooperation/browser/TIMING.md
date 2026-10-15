# Why the browser experiment reported roughly 0.8 seconds

The browser experiment’s `compile` field measured the **whole candidate preparation
pipeline**, not just the native compiler. It included fresh package loading and
type analysis on every edit. The prior native compiler result was not reproduced
by that label; the native compiler itself is still tens of milliseconds here.

This diagnostic is executable with:

```sh
go test -tags skgo_browser_experiment ./internal/hotreload -run '^TestNativeCompileStageTimingExperiment$' -count=1 -timeout=5m -v
```

`internal/hotreload/browser_timing_experiment_test.go` copies the exact current
`Toolchain.Compile` stages into an explicitly tagged diagnostic. Production code
was not modified. It creates the same generated native remote fixture as the
browser experiment, builds its host once, then times candidate preparation and
actual installation. No Vite/browser work occurs inside these measurements.

## First baseline: exact current stages

Raw output: [timing-baseline.log](timing-baseline.log).

| Stage | Revision 1 | Revision 2 | Revision 3 |
| --- | ---: | ---: | ---: |
| Whole candidate pipeline | 763.02 ms | 695.88 ms | 890.97 ms |
| `packages.Load` | 734.23 ms | 663.75 ms | 856.88 ms |
| `Lower` | 0.88 ms | 0.89 ms | 1.01 ms |
| `go tool compile` | 26.94 ms | 30.23 ms | 31.99 ms |
| Native installation | 1.40 ms | 1.80 ms | 1.94 ms |

Both classifications, candidate-file writes and numeric-retention analysis are
separately recorded in the log; each is under 0.4 ms. The package loader’s main
`go list` subprocess took 80–95 ms with `-export=false`. Its two auxiliary
release-tag/architecture probes took about 7–9 ms and overlap; their times must
not be summed as a serial wall-clock decomposition. Most of `packages.Load`
remains in-process analysis, not native object compilation or dependency export
compilation. Installation used the same host PID 19631 for all revisions.

## Bounded control: omit `NeedDeps`

Raw output: [timing.log](timing.log). This second run added graph/syntax counts
and compared unchanged full mode against the same overlay and load mode with
only `NeedDeps` removed. It also checked literal responses through the actual Go
HTTP handler after every installation.

| Mode | Whole pipeline, three revisions | `packages.Load` | Native compiler |
| --- | --- | --- | --- |
| Current full mode | 742.68 / 802.82 / 709.63 ms | 714.52 / 775.37 / 680.99 ms | 26.12 / 25.63 / 26.72 ms |
| Same overlay, omit `NeedDeps` | 621.66 / 564.86 / 591.54 ms | 592.60 / 535.08 / 562.78 ms | 27.24 / 27.89 / 26.87 ms |

**Both modes still loaded 260 packages containing 1,486 syntax files. The changed
root package contains only three syntax files.** Removing `NeedDeps` was not a
root-only analysis in this experiment.

The pinned source explains the result precisely. In
`golang.org/x/tools@v0.52.0/go/packages/packages.go:799–811`, the loader computes:

```go
// Overlays can invalidate export data.
// TODO(matloob): make this check fine-grained based on dependencies on overlaid files
exportDataInvalid := len(ld.Overlay) > 0 || pkg.ExportFile == "" && pkg.PkgPath != "unsafe"
```

Its subsequent `needsrc` condition loads source whenever requested types meet
that invalid-export condition. Any nonempty overlay consequently pushes the
dependency graph through source analysis, even without `NeedDeps`. The loader
then ignores dependency function bodies when `NeedDeps` is absent
(`packages.go:1247`), which is consistent with some reduction in work but does
not make it selective analysis. The no-`NeedDeps` control also requested
`go list -export=true` (106–134 ms), despite subsequently falling back to source.

All six control-run candidates installed into host PID **21152**. After each
installation the diagnostic requested the actual generated Go remote
`/_app/remote/1fvm1sx/order`, decoded its devalue result, and asserted independent
literal totals **1000, 2000, 3000** in each mode. Native installation took
1.33–1.79 ms. No host rebuild/relink occurred between these edits. The test exited
successfully and its owned host process was confirmed gone.

## What this establishes and does not

The extra latency is primarily repeated broad analysis in the current candidate
path. It is not a demonstrated regression of the native compiler from tens of
milliseconds to 0.8 seconds. The browser report’s ambiguous `compile` label hid
that distinction. The current fixture is generated and dependency-heavy; this
is not a controlled same-fixture comparison with every historical experiment.

Omitting one loader flag is **not** demonstrated as the fix. No retained-analysis
integration or optimization was implemented. The existing retained/selective
analysis work must be investigated and composed rather than replaced by a new
cache framework. These two bounded runs are diagnostics, not a statistical
benchmark, concurrency/load study, or proof of all edit compatibility rules.
The HTTP assertions establish actual native response changes; they do not
repeat the separate real-browser test or join its CSR fixture to Go SSR.
