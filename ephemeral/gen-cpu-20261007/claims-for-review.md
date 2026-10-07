# Generator resource claims for independent review

The user requests Claude Fable adversarial review of whether we can and should
do intensive analysis once and reuse its result, plus any other advice for
reducing test resource usage. The objective is to retain confidence in the
software, all meaningful assertions, and proof of genuinely different input
states. This is research and review, not authorization to implement changes.

The investigation and saved measurements are in `README.md` beside this file.
The profiled baseline is implementation commit
`c6906b7b6029c27f5311e080fd0ec97587b2aca4`. The current branch contains only
investigation artifacts, including copied experimental Go sources used through
compiler overlays. No proposed optimization has been accepted or implemented.

## Claims and their current evidence

1. **Most observed generator-test cost is package analysis and memory churn.**
   One profiled generator-suite execution passed 103 top-level tests without
   skips, took 222.82 seconds wall time, sampled 694.03 CPU-seconds inside the
   test process, and allocated an estimated 115.64 GiB cumulatively. Its
   parsing/type-checking stacks account for about 97.4% of sampled allocations.
   Concurrent validation elsewhere on the machine limits timing comparisons.
   This does not measure serving-request cost.

2. **Unchanged generation repeats substantial dependency-source analysis.**
   Two `gen.Run` invocations on the same example copy each parsed 1,182
   standard-library and 212 module-cache files and allocated about 1.5 GiB.
   Matcher loading requests dependency syntax and type information through
   `NeedDeps`; the probe retained the current generation behavior.

3. **The flag-only shortcut is unsafe with the existing validator, but its
   failure does not establish that full dependency source analysis is needed.**
   Removing only matcher `NeedDeps` through a compiler overlay reduced the
   paired allocation measurements to about 390 MiB and source dependency parses
   to zero. `TestSharedParamsSourceDiagnostics/transitive-generated` then
   proceeded past its expected refusal and panicked at the test's nil `Logf`.
   The working explanation is that export-loaded `go/types.Package.Imports()`
   does not retain every source dependency the ownership guard needs. The
   regression case runs once per full suite; the repeated analysis occurs in
   the generator itself across many calls.

4. **Import metadata might preserve the guard while retaining fast type
   loading.** This is an unimplemented, unmeasured candidate. Pinned
   go/packages v0.50.0 supports `NeedName | NeedImports | NeedDeps` without
   syntax/type-information requests. We infer that source-import ownership
   validation could use that metadata graph separately from export-based type
   loading. Compiler errors, cycles, overlay behavior, package identities, and
   the complete diagnostic contract have not been demonstrated with this
   combination. Whether the present guard's full policy is justified is also
   open to review.

5. **For identical test inputs and starting states, one generation result can
   serve multiple independent assertions without removing coverage.** The
   endpoint-module and empty-action-registry tests independently create the
   same endpointSource fixture and generate with the same configuration.
   Existing shared CLI, evolved-example, and production-build fixtures are
   precedents. Language switching, source edits, deletion, stale generated
   syntax/symbols, and idempotence may require separate invocations because the
   transition is what is being tested. Possible hidden process-global state or
   mutation makes sharing a claim to assess, not an automatic transformation.

6. **Within an invocation, repeated validation of an unchanged type/import
   graph could be reused.** `writeSharedParams` validates each matched route
   parameter before deduplicating equivalent alternatives. Each validation
   builds a forbidden-package list and starts its own graph walk. The reuse
   boundary and preservation of source-located errors have not been proved.

7. **A result about one graph cannot certify different or changed graphs.**
   “Once” could mean once per immutable fixture, once per generation's distinct
   graph/type, or reuse across invocations with justified invalidation. It has
   not been established that one intensive run for the entire suite suffices,
   or that a persistent cache would be worthwhile. Any distinction necessary
   to retain confidence should be made explicit by the review.

8. **The intended child-process CPU cap does not cap the parent test process.**
   `TestMain` sets the GOMAXPROCS environment after startup. The probes measured
   `runtime.GOMAXPROCS(0)==10`; future children inherit 2. Testing initializes
   default parallelism before TestMain. Changing parent/test/child concurrency
   may reduce contention, but resource and wall-time effects are unmeasured.

## Questions for Claude

- Which claims above survive independent scrutiny, and which are unsupported,
  misleading, or wrong? What evidence is needed to decide whether intensive
  work can and should happen once with its result shared?
- What does “once” safely mean for these tests and for real generation? Are we
  preserving checks worth preserving, or paying for an unnecessary policy?
- What other resource reductions would retain or improve confidence? Please
  distinguish demonstrated opportunities from hypotheses, and weigh the cost
  and complexity of any proposed machinery against the savings.

Source pointers are orientation rather than review-scope limits:
`internal/gen/{main_test.go,load_params.go,shared_params.go,shared_params_test.go,package_overlay.go,packages.go,evolution_test.go,check_test.go,prerender_production_fixture_test.go,endpoints_test.go,actions_test.go}`.
