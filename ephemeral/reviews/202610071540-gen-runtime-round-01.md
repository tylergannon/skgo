# Adversarial review — PR #280 `test(gen): reuse applications and isolate declaration rules` (round 01)

## Target

- PR: https://github.com/tylergannon/skgo/pull/280, branch `codex/gen-test-runtime` at `25a591e`, base `main` (`c867b6f`).
- Requirement under review: bring isolated `internal/gen` tests from ~170 s (PR measures 132 s on the same base) to **20 s or less without reducing useful confidence**, obeying repository instructions (Go tests at the real handler, no skips reporting success, never derive the expectation from the thing under test).
- Operating constraints honoured: read-only except this file. No scope-narrowing instruction was present in the request; none was ignored.

## Evidence inspected

- Full `git diff main...HEAD` (37 files, +1171/−744): every `internal/gen/*_test.go` change, the one production change (`internal/gen/locals.go`), new testdata (`prerender-options/server_test.go`, `prerender-production/ui/src/params.{go,ts}`, `typed-load/[number=Order]/*`), and the worklog.
- Surrounding production code the new direct-call tests exercise: `scan.go` (`readMarker`, `remoteSignature`, `isRemoteRequestEvent`, `isContext`), `prerender.go` (`checkPrerenderedActions`), `locals.go` (`validateLocalsType`, `validateHookSymbol`), root `request_handle.go` (real `RequestResolve`/`RequestMiddleware`).
- Pre-change versions of the removed/rewritten tests (`git show main:...`) for `check_test.go`, `prune_test.go`, `javascript_test.go`, `javascript_loads_test.go`, `layout_params_test.go`, `load_params_test.go`, `shared_params_test.go`, `prerender_test.go`, `prerender_resolve_options_test.go`, `output_test.go`.
- `go vet ./internal/gen`: clean. `gofmt -l internal/gen`: only two pre-existing testdata files not touched by this PR.
- Two independent isolated runs on this machine (10 cores), `go test -count=1 -json ./internal/gen`, nothing else heavy running:
  - run 1: **28.02 s** wall, 260 pass / 0 fail / 0 skip
  - run 2: **20.79 s** wall, 260 pass / 0 fail / 0 skip
  - Slowest top-level tests (run 1): `TestSharedParamsGeneratedConsumers` 16.4 s, `TestProductionApplication` 15.7 s, `TestFrontendRebuildWithoutCallerGeneration` 13.2 s, `TestStage2LayoutMetadataAcrossLanguageSwitches` 12.7 s, `TestPrerenderGoLoadFailureNamesAuthoredRoute` 12.6 s, `TestCheckApplication` 11.1 s, `TestStage2GeneratedLayoutDomainsAndTracking` 10.4 s.
- Confirmed no silent skips: zero `skip` events in both JSON runs; the child-process consumers (`stage2GoTest`, `runSharedConsumer`, `TestRepairedTypedLoad`, `TestGeneratedParameterDependencies`, `TestMatcherOnlyHandler`, the external-driver child) now assert `=== RUN`/`--- PASS` for each selected test and fail on `--- SKIP:`.

## Assessment of preserved contracts

The claim "declaration rules are proven through Go's type checker against the real validators" holds in the cases I traced:

- `TestEventDomainDeclarations`, `TestCommandFormEventDiagnostics`, `TestMarkerRefusalsNameTheAuthoredDeclaration`, `TestActionDeclarationsMatchKit` (most cases) call the real `readMarker`/`scanFile`/`checkDuplicates`/`checkPrerenderedActions` and assert specific diagnostics plus the authored file position. Removing the corresponding rule would fail them.
- `TestRemoteSignaturesRejectUnsupportedFunctions` / `TestABatchQueryThatIsNotABatchIsRefused` call `remoteSignature` directly; the by-name claim they used to carry (the old test was literally named `...IsRefusedByName`) now lives in `TestMarkerRefusalsNameTheAuthoredDeclaration` ("broken is declared as a query"). Acceptable.
- `TestPrerenderInheritanceMatchesKit` is now *more* load-bearing than before: the old version only asserted `Run` succeeded on six layouts and never reached the inheritance decision; the new one plants an action and asserts accept/reject per Kit's universal ?? server ?? inherited rule.
- `TestPrerenderOptionsFailuresArePositioned` grew from 5 to 7 cases with exact messages.
- `TestGenerateRemovesWhatTheDeletedGoProduced` became a unit test of `pruneStaleArtifacts`; the end-to-end removal-through-`Run` claim moved into `TestGenerationOwnsOneFileAndIgnoresPreviousDeclarations` (removes `page.server.go` + `save.remote.go`, asserts the stubs existed before and are gone after, authored `+page.svelte` survives).
- Language switching: the merged `TestGeneratedLanguageApplications` still asserts `.ts` counterparts are gone after the switch (`javascript_test.go:141`, run after the Auto/JS regeneration), the authored file survives, jsconfig.json auto-detection, the prerendered JS load's build bridge, JSDoc loads/actions/transported class, and the TS bridge.
- Stage 2 event/parameter negatives: the eleven removed `Run`+`Check` cases are covered by `TestEventDomainDeclarations` (event-domain spellings), the exact reflected method-set table in the stage-2 consumer (`layout_params_test.go:322–324`, which is what proves a page cannot read a descendant param etc.), and the new parsed-scope check that `IDParam_Bool` is absent from the sealed union.
- Shared-params private-type cases moved to `TestSharedParamsTypeAccessibility` calling `validateSharedType` directly, including the public-alias-over-private-type acceptance.
- Real Kit builds, real `vp build`, compiled consumers, the started production binary, the CLI JSON report with exit status and exact line, Check-without-writes, and the external-driver overlays all remain and all ran.

The production change in `locals.go` is a pure extraction; the only behavioural delta is error text using `pkg.Path()` instead of `cfg.LocalsPackage`/`cfg.HookPackage`, which are equal for a loaded package.

## Findings (most severe first)

### 1. issue — incomplete requirement: the ≤ 20 s isolated target is not reproducibly met

- Evidence: two isolated runs here measured **28.02 s** and **20.79 s**; the PR's own figures (19.700 s, 19.856 s) sit within 1.5 % of the ceiling, and the PR concedes 31.4 s under repo-wide contention. The suite's wall time is set by CPU oversubscription across ~8 tests that each take 10–16 s (`internal/gen/main_test.go:29` pins children to `GOMAXPROCS=2`, and the top-level parallelism is `ncpu`), so run-to-run variance of several seconds is inherent and there is no headroom.
- Impact: the user's acceptance is "20 seconds or less"; on the same class of machine it passes some runs and fails others. Either the number has to be met with margin, or the acceptance should be restated with the measured variance so a reviewer can tell a regression from noise.
- Pointers, not prescriptions: `TestSharedParamsGeneratedConsumers` (16.4 s) and `TestProductionApplication` (15.7 s) define the floor; `TestFrontendRebuildWithoutCallerGeneration` and `TestPrerenderGoLoadFailureNamesAuthoredRoute` each still run their own Vite/Kit build.

### 2. nitpick — the hand-written `skgo` stand-in does not mirror the real `RequestResolve`

- `internal/gen/declarations_test.go:70` declares `type RequestResolve[P,L any] func(context.Context,RequestEvent[P,L])(*http.Response,error)`; the real type at `request_handle.go:66` is `func(context.Context, RequestEvent[P, L], ...ResolveOptions) (*http.Response, error)`.
- `TestSelectedHookConfigurationAndAliases/function` (`locals_test.go:55–60`) now proves convertibility against this stand-in, not the real shape. `validateHookSymbol` is shape-agnostic and the real shape is still exercised by the production fixture's `--hook-package` generation, so confidence is not actually lost — but a stand-in that silently diverges from the package it imitates is exactly the "expectation copied from memory" trap; copy the real declaration or say in the comment why the shape is irrelevant.

### 3. nitpick — dead helpers left behind

- `sandboxExample` (`stalegen_test.go:161`), `sharedSandbox` (`main_test.go:74`) and `requireSkgoBinary` (`main_test.go:63`) have no remaining callers after this PR removed the example-copy pattern. `go vet` does not flag unused functions, so they will linger and invite someone to resurrect the slow pattern.

### 4. nitpick — previous-output state matrix collapsed to a single mixed regeneration

- `output_test.go:86–110` now puts each of the eight owned files into exactly one of {syntax-broken, undefined-symbol, absent} by `i % 3` and regenerates once, where before every file was put in every state (three regenerations). The initial `Run` already covers all-absent, so the trade is two regenerations for losing "every file × every state". A generator that read one specific stale file only when it was syntactically valid would now be caught only if sort order happens to assign that file to the undefined-symbol slot. Low probability; worth knowing it was traded.

### 5. nitpick — grouped subtests keep a `Test` prefix

- `application_fixtures_test.go` registers `t.Run("TestFormClientInputContractEvolution", ...)` etc. `go test -run TestFormClientInputContractEvolution` now matches nothing at top level and exits 0 with "no tests to run" — the failure mode the repository instructions warn about for anything that "passes" by not running. Dropping the prefix (`TestEvolvedApplication/FormClientInputContractEvolution`) removes the false affordance.

## Outcome

**material findings remain** — one issue (the ≤ 20 s acceptance is met only at the margin on the author's machine and was not reproduced here: 28.02 s and 20.79 s), plus four nitpicks. The confidence half of the requirement is satisfied: the preserved tests exercise the real validators and real builds, nothing skips, and the rewritten declaration tests are at least as load-bearing as what they replaced.
