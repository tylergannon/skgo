# Issue 247: isolate build-test fixtures and share compatible preparation

This is the concrete plan proposed in chat, recorded for design review before
implementation. Authority: https://github.com/tylergannon/skgo/issues/247 and the
repository instructions.

## Goal and scope

A developer can verify a generator or prerender change without unrelated demo
consumers preventing the test from reaching its assertions. Preserve the real
generator, pinned Kit, adapter, production rendering and named contracts while
eliminating unnecessary preparation. Whole-example coverage must still catch
actual application integration failures.

The user requests correctness and feasibility review and agreement that the
result eliminates the major testing problem in the issue. They explicitly
exclude complicated proof machinery and significant scope additions. Use
ordinary Go tests and existing setup. No new framework, persistent artifact
cache, bespoke proof runner or command claiming the software is done. Adapter
shutdown and cleanup failures remain separate diagnoses, not promised fixes.

Implementation has two ordered parts, not a claim that either partial result
finishes issue 247. Part A can start on current main: the prerender failure,
shared successful production fixture, explicit `ui` coverage and adapter
minimal-app consolidation. Part B starts after the typed-parameter APIs and
tests integrate into main: typed-load evolution and the shared-parameter
fixture. Do not implement missing parameter APIs here or edit the active
parameter worktrees. Rebase the isolated fixture work onto the integrated
API before Part B. Capture a fresh comparable baseline at that point; do not
compare Part B against a base on which its APIs do not exist. Completion of
the issue requires both parts.

## 1. Purpose-built applications

Replace copied-demo fixtures with checked-in applications under `testdata`,
staged into temporary directories when tests require mutation.

Check in the contract-bearing sources, not independently maintained dependency
pins. Staging uses an explicit allowlist from this checkout's example:
`go.mod`, `go.sum`, and `web/package.json`, `web/vite.config.ts`,
`web/tsconfig.json`, `web/src/app.html`. These remain the common build bootstrap;
rewrite the local SKGo replace/adapter link and frontend-root paths as needed,
while preserving the Go package/type identities the contracts assert. Link the
installed frozen frontend dependencies and current adapter using the existing
helpers; absence is a test failure. Do not install floating dependencies or
copy demo handlers, generated registries, routes or application hooks.
Contract-specific hooks and server wiring belong to the fixture. This is
small staging code in ordinary Go tests, not a fixture framework.

- Prerender failure: a Go layout load returning the literal
  `prerender fixture literal failure`, plus prerendered `/about`. The real build
  must fail and identify `/about`, `src/routes/layout.server.go`, `GET /about`
  and that literal error. An arbitrary compilation failure cannot satisfy it.
- Successful production prerender: redirect routes and their destinations, one
  no-argument remote and pages consuming its artifact. Prove native redirect
  artifacts survive the adapter and production rendering reads built remote
  results/errors without executing the Go body.
- Typed-load evolution: the `Order` matcher and both routes consuming its named
  Go type. Prove declarations refresh before stale handlers fail, then repaired
  handlers produce literal HTTP results.
- Shared parameter types: preserve the existing type-identity and route-key
  matrix, including aliases, distinct package types, methods, composite types
  and encoded route names, and its generation/compilation guarantees.

Contact forms, authentication and unrelated demo features must not enter these
narrow fixtures' compilation.

## 2. Share specific successful builds safely

Use one successful production fixture containing the redirect and remote
artifact cases. They currently use the same frontend root, origin and
production build configuration. Generate/build once per package invocation;
separately selectable tests inspect its immutable output.

Redirect contracts retain `/old` to `/target?from=atlas`, `/second` to
`/target?from=beacon`, and `/ordinary-old` to `/ordinary?from=legacy`. Retain
native redirect markup, distinct query artifacts, compressed-file mappings,
and preservation of the dynamic destination. Mere HTML-file existence is not
sufficient.

Remote assertions use isolated artifact overlays and fresh handler state:

- Missing null-key artifact: HTTP 500 and zero Go-body calls.
- Stored error: HTTP 409 with `built no-argument failure` and its marker.
- Page-transform failure: fallback retains `built transform value` with the
  expected two transform calls.

The fixture's handler constructor receives transform middleware per instance;
it must not retain the current process-global `PrerenderTransformFixture`
slot. This is fixture-local wiring, not a new SKGo public API. Each handler's
transform counter is local. A remote body-call counter may be shared only
with an independent absolute expectation of zero for every runtime case;
never reset it or derive a baseline from it while other cases execute.

Retain compiled production rendering and real handler composition. Keep the
deliberately failing prerender fixture separate. Reuse the existing
package-lifetime temporary storage and once-only setup mechanisms; do not
introduce a general cache. Shared artifacts cannot be mutated by consumers.

Keep the failure fixture's authored source small and separate, with `/about`
as its only prerender target and its own failing layout. It shares the build
bootstrap above, not the successful fixture's route tree. This avoids making
the diagnostic depend on which of several routes Kit crawls first and avoids
weakening the literal `/about` expectation.

## 3. Preserve type-change lifecycle

The typed-load test retains three generator attempts: initial generation;
generation after replacing `OrderNumber.Label()` with `RevisedOrder.Text()`,
which refreshes declarations before failing on stale handlers; and successful
generation after repairing those handlers. Actual requests must return
`Revised order #42` and `Revised order #7` as independently specified literals.
Checking generated declarations alone cannot substitute for serving proof.

The shared-parameter fixture keeps its type matrix and intentional
failure/recovery stages, removing unrelated demo consumers rather than
difficult cases. Mutable lifecycle states cannot contaminate other tests.

## 4. Explicit nondefault frontend root

Author the successful production fixture directly under `ui`. Explicitly
verify generation, Kit build, generated Go imports and production serving
from that root. Stop renaming the entire demo from `web` to `ui` in every
unrelated test.

## 5. Adapter build grouping

The large declared-inputs test already shares its successful build. Its failed
producer, malformed bridge response and JavaScript configuration require
separate builds and retain their contracts.

Combine the two compatible minimal adapter applications into one fixture with
no Go loads. One generation/build should prove the empty-input receipt
`inputs:empty\nbody:empty\n` and distinct same-named exports producing
`alpha:atlas` and `25`. Keep exact artifact assertions. This targets actual
duplication rather than claiming every existing build is redundant.

This work does not eliminate every whole-example copy. Retain the large
declared-inputs fixture's existing producer-error, transport, artifact and JS
mode coverage; its grouping is inspected, not rewritten wholesale. Retain the
identifier, undeclared-remote and lifecycle build tests as adjacent contracts
outside this issue's named migration. Retain `evolution_test.go` and example
typedrift sandboxes because they assert on the demo's actual generated output.
The separate typed-load prerender/dependency tests and frontend-only params
test remain as-is in Part B; replacing `sharedFixture` benefits its existing
callers but does not silently rewrite every neighboring test. Remaining narrow
demo copies are a known residual dependency, not covered by the isolation
claim. The issue's four named failing tests and the `sharedFixture` callers
are the required isolation boundary.

## 6. Acceptance using ordinary tests and bounded experiments

Deliberately break an unrelated demo contact consumer: the migrated narrow
fixtures should still pass while whole-example validation fails. Part A uses
an ordinary contact-consumer compile error available on main. Part B also
demonstrates independence from the integrated named-variant consumer that
motivated the issue. Separately break
representative covered behavior (artifact lookup, redirect mapping, type
refresh, with type refresh tested in Part B) and confirm its test fails. An independent validator checks that the
retained assertions are load-bearing and runs them.

Run individual tests and shuffled groups to expose shared-state dependence.
Compare actual generation/build/server-start counts and elapsed time under
matching cache conditions, with relevant CPU/memory measurements. Include
child `go test` compilations and frontend sync/type-check runs rather than
counting only explicit `go build` and `vp build` commands. Count intentional
failures and configuration transitions. Report measured results;
do not invent a speed target. Run affected groups during iteration; retain
whole-example production/development coverage and required browser scenarios
at stabilized checkpoints. Skips are not passes. Use the real app for visual
inspection where the repository instructions require it.

## Review source locations and evidence limits

This checkout is `/Users/tyler/.codex/worktrees/e857/skgo`, initially at
`b3984d8`. Tests here include:

- `internal/gen/main_test.go`, `evolution_test.go`, `prerender_test.go`,
  `prerender_redirect_test.go`, `prerender_remote_ssr_test.go`;
- `internal/adapter/prerender_inputs_build_test.go` and
  `prerender_inputs_minimal_build_test.go`;
- `example/server.go`, `main_test.go`, `typedrift_test.go`, and browser suite.

Two issue-named files are not on this checkout's base revision. Read them
without modifying the active worktrees:

- `/Users/tyler/.codex/worktrees/remote-function-params/skgo/internal/gen/load_params_test.go`
- `/Users/tyler/.codex/worktrees/remote-function-params/skgo/internal/gen/shared_params_test.go`

The typed-load test is also available under
`/Users/tyler/.codex/worktrees/5f2d/skgo/internal/gen/load_params_test.go`.
These are active development sources and are reference material only until
Part B begins against their integrated state, as specified above.

No builds or benchmarks have been run for this plan. This checkout lacks the
installed Kit dependencies. The fixture proposal is grounded in existing
test source; verify installed pinned Kit per
`ephemeral/sveltekit-current/SKILL.md` before implementation or claiming
compatibility demonstrated by execution.
