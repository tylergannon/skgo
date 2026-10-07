# Reduce generator validation resource use

Status: implementation plan only. No implementation or execution is authorized
by this document. Prepared against `eab1b63`, whose implementation is still
`c6906b7`; refresh the source and installed dependency pins when work begins.

## Outcome

Normal generation and validation reuse Go's compiler export data for unchanged
dependencies. Tests generate an unchanged fixture once and retain all their
independent assertions. Changed source, changed configuration, and recovery
states continue to receive fresh analysis and actual generation. Resource
savings come from less work, with the same observable behavior and confidence.

The expensive dependency compilation should occur once per relevant Go
build-cache state, using Go's existing invalidation. This does not mean one
generation can cover every fixture or source transition in the suite.

Evidence:

- [Investigation and measurements](../gen-cpu-20261007/README.md).
- [Claims submitted for review](../gen-cpu-20261007/claims-for-review.md).
- [Claude Fable review](../reviews/202610070852-gen-cpu-claude-fable-round-01.md),
  outcome `material findings remain`.

The two demonstrated causes are matcher loading that requests dependency
syntax/type information, and fresh-file overlays that invalidate otherwise
usable dependency exports. The existing-example probe allocated about 1.5 GiB
per unchanged generation. Claude's fresh endpoint fixture, with no matcher,
still parsed 1,090 standard-library files and allocated 301 MiB. These are
different paths; fixing either alone is incomplete.

## Constraints

Use Go and the existing generator/test entrypoints. Keep `-count=1`, all
meaningful assertions, and the existing shared-build fixtures. Keep compiler
diagnostics and source locations, ownership/import restrictions, type identities,
foreign-type projection, stale-output recovery, and read-only checking.

Failed generation and formatter failure must retain previous output. Authored
files must remain protected. Generation currently buffers writes until its
publication stage; moving a write earlier is not an accepted way around overlay
invalidation. Preserve logical/physical path aliases and the external package
driver protocol already covered by tests.

Do not add a persistent SKGo analysis cache, a second fast suite, a completion
runner, or a fork of dependency code as the default solution. Do not apply the
recorded flag-only patch: it bypasses a guard and aborts a diagnostic test.
Do not remove source analysis actually required for application declarations or
named-type projection. Missing or unusable exports can legitimately require
source loading; distinguish that from blanket invalidation.

Kit remains the specification for generated application behavior. Any change
to that behavior requires mapping the installed Kit pin first. This work should
preserve the existing generated interface and renderer/runtime behavior.

## Work sequence

### 1. Establish one attributable baseline

Mission: identify how much suite work comes from each loading path, using one
captured execution rather than repeated expensive runs for separate questions.

Ownership: temporary Go profiling instrumentation and measurements under
`ephemeral/`; no change to generator semantics or default test behavior.

Acceptance:

- One generator-suite observation distinguishes load sites and whether export
  preservation fell back because an overlaid path was absent. Record parent
  CPU, allocations, wall time, available process-tree CPU/RSS, and concurrency
  settings. Keep the captured output for subsequent analysis.
- The successful existing-example and fresh endpoint shapes have independent
  parse counts. Record cache warmth and competing validation processes.
- Every expected test executes; skips, panic-aborted tests, and missing tools
  remain failures rather than favorable performance results.

The recorded profiles remain useful baseline evidence. This additional run
answers the attribution gap Claude identified; do not repeat it merely to
produce another summary. Temporary instrumentation must not become permanent
product telemetry or a new validation entrypoint.

### 2. Preserve matcher diagnostics while reusing dependency exports

Mission: a developer can generate and check typed matchers, including edited
types and transitive dependencies, without reparsing/type-checking dependency
source merely to establish import ownership.

Ownership: matcher/shared-parameter analysis and its diagnostic contracts. One
implementer owns the shared loading seam across this task and task 3; avoid
concurrent edits to that seam.

Acceptance:

- Every `TestSharedParamsSourceDiagnostics` case retains the expected refusal
  and `params.go` location, including transitive generated dependencies and
  the final compiler import-cycle case. Exercise the public generator/check
  path as well as the existing internal diagnostic coverage so the test's nil
  logger cannot substitute a panic for the expected refusal.
- Actual generated consumers and handlers retain typed values and parameter
  dependency tracking. Editing a matcher/domain type causes fresh analysis;
  stale generated declarations do not become an input to success.
- Successful existing-example generation with usable exports produces no
  accidental standard-library dependency parses. Repeated unchanged generation
  retains equivalent output and substantially lowers allocation and CPU
  against the baseline. The rejected experiment's 390 MiB observation is a
  comparison point, not a promised bound for the correct implementation.

The implementer must map pinned go/packages and Polytype behavior directly.
Import metadata independent of dependency syntax is a candidate supported by
the pinned API, not a predetermined implementation. It must preserve compiler
errors, package identities, aliases, and the complete source-import closure
needed by the existing guard. Validate each distinct input graph once within
the invocation where feasible; do not assume export-loaded
`go/types.Package.Imports()` is that closure.

Claude's possible non-type-helper import-cycle gap is unverified. Investigate
it when mapping the guard and distinguish an existing policy from any proposed
behavior extension; it is not an accepted extra feature in this plan.

### 3. Preserve export reuse when generated files do not yet exist

Mission: first generation, new routes, and read-only checking see current
authored and generated-in-memory declarations without invalidating the entire
dependency graph because a file has no on-disk copy.

Ownership: generation/loading integration and output-preservation contracts,
coordinated with task 2. The builder chooses the smallest supported mechanism
after mapping the pinned loader's handling of new files and parser callbacks.

Acceptance:

- The successful fresh endpoint fixture with usable dependency exports has
  zero standard-library source parse calls, replacing the observed 1,090.
  Actual first-time generated consumers still compile and run.
- New-file checking and generation, existing-file overlays, aliased roots,
  dependencies, and external drivers retain their existing behavior. Genuine
  source errors stay source-located and read-only checks write nothing.
- Repeated generation, stale syntax, removed runtime symbols, missing generated
  files, authored destination collisions, and formatter failure retain their
  existing output/recovery contracts. A failed attempt leaves the previous
  project output intact; successful publication remains complete.
- Resource savings appear in source parse counts and allocation/CPU, with
  temporary resources cleaned up. Moving work into child processes or
  increasing RSS does not count as eliminating the expense.

Claude's early-materialization suggestion is exploratory. Existing `write`
calls buffer their output; staging or path handling must preserve that fact's
observable guarantees. If a loader limitation requires an exception, identify
the exact case and its cost rather than retaining a blanket fallback silently.

### 4. Share identical fixture results

Mission: the endpoint-module and empty-action-registry tests read one generated
result while each retains its own fixture-anchored assertions.

Ownership: immutable test-fixture sharing, after the loading changes settle.
Use the package-lifetime sharing pattern already present in this suite.

Acceptance:

- The same fixture inputs/configuration are generated once when both tests
  execute. A read-only result remains valid until all consumers finish; mutable
  tests do not share it. All existing module, registration, manifest, and
  action-registry assertions still execute.
- Independently break the generated behavior each assertion claims to protect
  and confirm the assertion fails. Sharing must not replace independent
  expectations with values derived from the generated result.
- Keep separate executions for transitions: language selection/switching,
  edits, deletion, stale output, and idempotence. Consolidate another fixture
  only when its equivalence and lack of mutation are established.

This is a smaller saving. Do not build generic fixture-cache machinery or
memoize `validateSharedType` as a performance project; the profiles do not
support that as a material resource target.

## Validation and measurement budget

Develop against focused ordinary Go contracts. Run the complete generator
package after a candidate affects the shared loader; the original experiment
aborted before a selected second test ran and proves nothing about remaining
cases. Before delivery, `just test` must run the normal tests in both modules
without skips. Prefer capturing final generator resource measurements within
that normal invocation using temporary Go instrumentation, so one expensive
execution supports many comparisons. Add another full run only to answer an
unresolved question, a failure, or a subsequent change.

Compare before/after generation on the two demonstrated fixture shapes with
the same toolchain, build-cache conditions, and concurrency. Record allocation,
dependency source parses, parent and available child CPU, max RSS, and wall
time. Parent-only profiles cannot prove process-tree savings; concurrent-test
durations cannot be added as an estimate of total wall-time savings. Newly
required compilation belongs to Go's existing cache and must not recur merely
because an unchanged generation is repeated.

Independent validation owns the question “would these assertions fail if the
behavior broke?” and execution of the relevant contracts. Give that validator
the missions, repository/pinned-source authority, and acceptance above; it
chooses the fault checks. The validator also assesses measurement instrumentation
for omissions, altered load semantics, or confidence lost through sharing.

No new browser scenario is needed for an assertion entirely about generation
or Go HTTP behavior. Preserve the repository's existing browser qualification:
before merge/release, its normal suite must pass in production and dev modes on
the required main revision, and someone other than the changer opens the example
app. A changed generated interface must receive the existing client proof it
needs. This plan runs no tests and makes no release-readiness claim.

## Conditional tuning after the main savings

Only if meaningful contention remains, choose one justified comparison of
invocation-level parent `GOMAXPROCS`, test `-parallel`, and package `-p`.
Account for child limits and explicit caller settings. Retain the normal test
recipe; measure user/system CPU, RSS, and wall time, and state the tradeoff
before adopting a default. Do not tune by an exhaustive grid of suite runs.

Consider `GOGC` only after allocation and peak memory fall. One paired
measurement must show the CPU/RSS tradeoff on a machine shared with other
agents. Keep defaults if the benefit is not established. Request-time runtime
tuning remains outside the demonstrated problem.

## Completion

The implementation is complete when both demonstrated accidental dependency
source-loading paths are removed for the representative successful cases, all
existing diagnostics/output contracts and client qualification remain intact,
identical fixture sharing retains its independent assertions, and saved
measurements demonstrate reduced allocation and CPU. Report exceptions and
unmet claims explicitly. Checkpoint and delivery follow `agent-protocol` once
implementation and delivery are separately authorized; this plan itself is
only a checkpointed artifact.
