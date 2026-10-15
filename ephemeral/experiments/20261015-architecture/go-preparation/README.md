# Go preparation comparison — 2026-10-15

The existing retained source-view machinery removes the repeated broad analysis
cost on this generated remote fixture. The warm retained-view samples prepared
native candidates in **138.261 and 137.409 ms**, then installed in **1.707 and
1.395 ms**. Actual HTTP responses independently asserted the changed values.
This is candidate preparation, not save-to-browser latency or a selected design.

Authority remains the cache's `research/dev-experience-20261015/GOAL.md`.
Research entered through `index/README.md`, then `compiler-retention.md`,
`selective-generation.md`, `dev-executed-experiments.md`, and the cited raw timing,
x/tools overlay implementation and current checkout sources. The old plan's
retention/porting tactics are not additional acceptance requirements.

## Comparison

Same generated order remote, same private native toolchain and imported archive
set, one host, and body edits returning literal 1000, 2000, 3000. All variants
retain current lowering, native compiler invocation, artifact identity creation
and host installation. Baseline is an explicitly instrumented copy of current
`Toolchain.compile` stages at checkpoint 13ea7b8, including full-overlay package
loading and artifact identity hashing. The root is the complete generated route
package; no whole-application caller-coverage claim follows.

- `baseline-full-overlay`: current full dependency syntax/type load with raw overlay.
- `exports-root`: existing `gen.preserveDependencyExports`, through a build-tagged
  experiment-only wrapper, with root syntax and dependency exports.
- `retained-root`: existing `goload.Session`, with that export-preserving fallback.
- `retained-view`: existing `goload.View` over the private captured fixture,
  retaining discovery across updates; this facility is already used by `gen.Project`.

All numbers below are milliseconds from the final coordinated quiet run. Other
agent compilation paused; light browser inspection continued. Three successive
body samples per mode are shown, not a statistical distribution. Modes ran in
fixed order. Host/setup costs precede the table; first view load is cold.

| Mode | Analysis, samples 1 / 2 / 3 | Compiler | Artifact identity | Total preparation | Install |
| --- | --- | --- | --- | --- | --- |
| baseline-full-overlay | 725.567 / 994.897 / 764.966 | 26.227 / 45.802 / 27.485 | 100.151 / 118.105 / 118.206 | 854.236 / 1163.512 / 913.244 | 2.012 / 1.480 / 1.803 |
| exports-root | 133.102 / 158.247 / 159.256 | 26.154 / 28.086 / 26.662 | 110.436 / 112.574 / 107.747 | 272.514 / 301.871 / 296.018 | 1.754 / 1.480 / 1.579 |
| retained-root | 125.427 / 128.214 / 131.018 | 25.803 / 26.003 / 25.194 | 110.092 / 109.807 / 111.424 | 263.724 / 266.820 / 270.262 | 1.380 / 1.617 / 2.180 |
| retained-view | 131.985 / 2.296 / 1.633 | 25.642 / 25.986 / 25.933 | 110.548 / 107.772 / 107.635 | 270.462 / 138.261 / 137.409 | 1.354 / 1.707 / 1.395 |

The root-only modes reduce dependency syntax to three root files. The view's
post-control counters were one metadata load, four metadata hits, six package
checks, sixteen parsed files, four export imports and **zero fallback loads**.
The extra package check/file is the changed dependency. Ordinary Session still
performed five metadata loads; retained parsing alone does not remove discovery.

## Correctness and scope

Final run: PASS, test body 11.41 seconds. Twelve body samples plus four repairs
and four dependency candidates made **20 successful installations**. Four
intentional malformed bodies were rejected before installation; each left the
literal 3000 response usable, followed by repair to literal 4000. There were no
unexpected execution failures. Each mode's imported constant control changed
`Offset` from 0 to 77: both the analyzed `go/types.Const` and the real generated
HTTP result (5077) were independently asserted. Dependency archive refresh is a
separate measured operation; it is not hidden in ordinary body-edit timings.

The native fixture uses the legacy `Lower` host setup retained from the earlier
browser experiment. Current compiler/platform/object artifact verification runs,
but this host does not pin `HostView`'s dependency identity. Consequently the
constant edit establishes analysis and compiler freshness; it **does not prove**
that production `HostView` would admit that dependency revision into the same
host. A complete-build fallback can remain correct. Arbitrary initialized-state
preservation and general unloading are not gates here.

Root-only modes do not reproduce `CompileApplication`'s complete executable
caller analysis. Their speed cannot justify silently dropping that check.
The private fixture inputs and dependency/module/toolchain immutability are
explicit assumptions; real project capture, event selection, stale admission,
structural edits, local replacements, full-app caller coverage, resource bounds,
Vite/browser signaling and SSR composition were not exercised.

## Remaining cost and implication

Current artifact creation rereads and hashes all dependency archives for every
candidate: roughly 108 ms in the warm retained-view samples. No identity check
was removed to improve these numbers. `artifact.go:dependencyIdentity` binds
actual archive bytes; `view.go:HostView` captures that identity in the host;
`native_runtime.go` checks it at installation. Go's compiler cache identities
and the existing retained source/view ownership provide prior evidence for
investigating retained identities, but no cheaper equivalent verification was
implemented or demonstrated here. Freshness is required; per-edit whole-archive
rehashing is the present tactic, not itself a GOAL requirement.

The result supports an integration experiment consuming existing retained inputs
for native preparation, while preserving caller coverage and compiler freshness.
It does not select the complete development architecture. A failure to preserve
those boundaries, or equivalent validation restoring the old cost, would defeat
this proposed advantage.

## Reproduction and provenance

Run `COMMAND.txt` from the checkout with the restored experiment sources. Pins,
source hashes and confirmed host cleanup are in `provenance.json`. The example's
frontend dependencies were installed through VitePlus before this work. Exact
Go toolchain: Go 1.27.2 darwin/arm64, CGO disabled; polytype 1.9.0, x/tools 0.52.0.
The native host PID 75988 was absent after the successful test joined its process.

`quiet-comparison.log` is the authoritative full raw final output;
`samples.json` extracts its twelve body samples. `attempt-1.log` and
`attempt-2.log` preserve successful exploratory runs under concurrent compilation,
with their corresponding source snapshots. Attempt 1 has three modes; attempt 2
adds the retained view. Final source additionally independently asserts analyzed
constant values. `source/` preserves the experiment, reused fixture and relevant
implementation sources. No production API or behavior was changed, no benchmark
was repeatedly rerun for a preferable number, and no files were committed by this lane.
