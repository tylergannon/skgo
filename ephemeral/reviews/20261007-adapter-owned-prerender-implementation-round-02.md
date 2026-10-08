# Adversarial review — adapter-owned prerender integration (round 02)

- **Date:** 2026-10-07 (local)
- **Reviewer model:** Claude Opus 4.8
- **Branch:** `codex/adapter-owned-prerender` (worktree `4230`)
- **Target:** The current working-tree implementation of
  `ephemeral/plans/adapter-owned-prerender.md` — generated application source carries plain
  typed throwing stubs for loads, remotes, endpoints, and the server hook; the adapter's
  build-only Vite integration re-injects the Go-forwarding bridges in Kit's Node (`ssr`) build
  environment only.
- **Spec authority:** Kit's installed pin `@sveltejs/kit@3.0.0` at
  `example/web/node_modules/@sveltejs/kit`.
- **Outcome:** **only nitpicks remain.**

## Relationship to round 01

Round 01 (`…-round-01.md`) found no material defects. Since then the change set was reworked
(diffstat moved 77→62 files): query-only `*.remote.ts` stubs no longer churn (the generated
comment now matches `HEAD` for non-prerender modules), and `prerender-modules.js` changed its
hook branch. I re-reviewed the whole change against the same authoritative sources rather than
trusting round 01, and re-ran the full proof on a now-quiescent worktree (round 01's single
whole-package adapter failure was concurrent-session contamination and does not recur — see
checks below).

## Caller instruction handling

The launch prompt imposed only valid operating constraints (read-only except the artifact, a
fixed artifact path under `ephemeral/reviews/`). It did not narrow the subject matter, predict
findings, or declare safe areas, so there was nothing to refuse. I reviewed broadly from the
plan, repository instructions, Kit's pin, and current artifacts.

## Evidence inspected

Implementation:
- `internal/adapter/skgo-adapter/prerender-modules.js` (re-read in full) — the build-only
  `load` hook; new hook branch inlining `${code}` and relaxing its export guard.
- `internal/gen/emit.go`, `loads.go`, `endpoints.go`, `prerender_command.go` (re-diffed) —
  unchanged in substance from round 01: throwing stubs, named TS type imports, empty
  `export {};` hook marker, `build` metadata carrier in `skgo.remotes.json`.
- `internal/adapter/skgo-adapter.js` (plugin wiring into `vite.plugins.pre`), `generated.js`
  (threads `build`), `adapter.go` (`//go:embed skgo-adapter.js skgo-adapter` embeds the whole
  directory, so `prerender-modules.js` ships automatically), `skgo-adapter/env.js:218,310`
  (goja resolves the build helper to a throwing build-only stub).

Generated output (committed `example/web`):
- `src/hooks.server.ts` = `export {};`; `src/routes/about/about.remote.ts` (throwing prerender,
  no bridge); `src/routes/actions/+page.server.ts` (load + actions coexistence with a named
  `ActionFailure` import, load body throwing, every action throwing); `skgo.remotes.json`
  `build` map (41 entries, each a single-key `source`/`remotes`/`endpoint`/`hook` shape).

Proof:
- Generation contracts: `javascript_loads_test.go`, `prerender_test.go`, `endpoints_test.go`,
  `language_application_test.go`, `adapter_test.go`, `application_fixtures_test.go` and the
  `assertThrowingSource` negative guard.
- Adapter unit + build tests: `generated_test.go`
  (`TestPrerenderModulesPreserveNativeDeclarationsAndRejectMissingMetadata`, now also covering a
  hook with an extra export), `package_test.go` (packaging/embedding equality),
  `prerender_inputs_identifier_build_test.go`, `prerender_inputs_build_test.go`
  (`TestPrerenderHelperIsBuildOnlyInGoja`).
- Production fixtures: `prerender_production_fixture_test.go` + `prerender-production`
  compiled `server_test.go` handler contracts.

Checks run (quiescent tree, all green, no `--- SKIP:`):
- `go test ./internal/gen -run 'TestProductionApplication|TestProductionOptionsApplication' -v`
  → PASS (11 subtests incl. `AdapterOwnedPrerenderModules`; options fixture included).
- `go test ./internal/adapter` (full package) → `ok … 212.910s`, no skips, no failures.
- (Round 01, still valid) `go test ./internal/adapter -run
  TestPrerenderModulesPreserveNativeDeclarationsAndRejectMissingMetadata` → PASS.

## Assessment

The developer outcome holds. Every generated application module is a plain throwing stub:
loads throw with their exact Go-derived shape and a named `RequestEvent`/`ActionFailure`
import; remote prerenders keep Kit's native `prerender(...)` / `prerender('unchecked', …)`
wrapper with a `unimplemented()` body and no options; endpoints bind each declared method to
`unimplemented`; the hook is `export {};`. `assertThrowingSource` fails the build on any
residual `building`/`buildLoad`/`skgoPrerender`/`event.platform`/`getRequestEvent`/
`@skgo/.../prerender`/`inputs:` token or inline TS `import(...)`, so these checks bite in both
directions (empty file fails the positive literals; stale bridge fails the negatives).

`prerender-modules.js` reconstructs bridges only under `apply: 'build'` +
`applyToEnvironment: ssr`, parsing with the app's own Vite parser (preserving module identity,
so Kit registers the original remote IDs/kinds), and splicing in the Go load/endpoint/remote
bridges and the hook's `requestHandle`/`requestFetch`. I re-walked the overlapping
body/inputs edit arithmetic and the arity checks; they produce the correct
`prerender(validate?, fn, { inputs })` shape for both call forms. Missing/malformed metadata, a
declaration disagreeing with the parsed exports, a build declaration on an authored module, or
a server module with no declaration all raise explicit build errors — exercised by the isolated
node test's three throw-paths and verified against Kit's file-existence hook discovery
(`write_server.js`, runtime `await import` of `{ handle, handleFetch, … }`).

The proof is load-bearing end to end: the real Kit build asserts bridge symbols present in the
Node SSR output and absent from the shipped client (`build/client`) and embedded goja engine
bundle (`build/ssr/bundle.js`); the packaging tests assert the npm tarball equals `packageFiles`
*and* the embedded bytes, so the new file cannot silently fail to ship; the
`skgoRemoteInputs` identifier build test (TS **and** JS) proves the Go producer and body each
run exactly once and the correct devalue-keyed artifact lands; `TestPrerenderHelperIsBuildOnlyInGoja`
renders `/prerender-helper` through the compiled goja engine and **requires** the
"declared prerender inputs are build-only" failure in the response body, satisfying the plan's
demand that denial be proven by an actual failure message, not by the mere absence of an import;
and the compiled handler contracts assert route-specific literal outputs, 404s for unbuilt
paths, and prerendered-route dispatch reduction — expectations sourced independently of the code
under test.

No material findings (incomplete requirement, incorrect implementation, verifiable bug,
critical antipattern, or race/crash) survived verification.

## Findings

### Nitpicks

1. **Hook branch carries speculative generality the generator never exercises**
   (`internal/adapter/skgo-adapter/prerender-modules.js:118-124`). The branch now inlines the
   original `${code}` and guards only against a pre-existing `handle`/`handleFetch`
   (`exports.has('handle') || exports.has('handleFetch')`), where round 01 rejected *any* export
   (`exports.size`). The generator, however, only ever writes `tsHeader + "export {};"`
   (`prerender_command.go`), and authored hooks are forbidden, so no hook with an extra export
   (e.g. a Go-owned `handleError`) can arise. The new unit case in `generated_test.go:211-221`
   fabricates such a `handleError` hook to cover the preservation path — i.e. it tests input the
   generator does not produce, and the relaxed guard is a strictly weaker invariant than the one
   it replaced. Harmless today, but it is unrequested generality; a tighter "the generated hook
   has no exports" invariant would match the generator and still fail loudly if that ever
   regressed. Classify: nitpick.

2. **Remote-stub comment is imprecise for prerender modules**
   (`internal/gen/emit.go:143-145`). The single comment now emitted for every `*.remote` module —
   "…skgo answers their endpoints itself, so anything that renders in the browser is proof the Go
   handler replied rather than this module" — is accurate for query/command/form remotes but not
   for prerender remotes, whose results are served from static build artifacts rather than a live
   Go handler at request time. Cosmetic; generated-comment wording only. Classify: nitpick.

## Outcome

only nitpicks remain
