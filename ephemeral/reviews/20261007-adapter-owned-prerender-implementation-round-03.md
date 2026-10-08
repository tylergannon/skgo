# Adversarial review — adapter-owned prerender integration (round 03)

- **Date:** 2026-10-07 (local)
- **Reviewer model:** Claude Opus 4.8
- **Branch:** `codex/adapter-owned-prerender` (worktree `4230`)
- **Target:** The current working-tree implementation of
  `ephemeral/plans/adapter-owned-prerender.md` — generated application source carries plain
  typed throwing stubs for loads, remotes, endpoints, and the server hook; the adapter's
  build-only Vite integration (`internal/adapter/skgo-adapter/prerender-modules.js`) re-injects
  the Go-forwarding bridges in Kit's Node (`ssr`) build environment only.
- **Spec authority:** Kit's installed pin `@sveltejs/kit@3.0.0` at
  `example/web/node_modules/@sveltejs/kit`.
- **Outcome:** **only nitpicks remain.**

This round (a) adjudicates round-02 nitpick 1, which the caller contested, and (b) re-reviews
the entire current work against the same authoritative sources. The core implementation is
byte-identical to what round 02 reviewed (`prerender-modules.js` md5 `b5ae6cc…`, `emit.go`
`40e5f8c…` unchanged), so this round concentrates on the contested finding and re-runs the
proof that bears on it.

## Caller instruction handling

The launch argument supplied specific facts (the `installInputsErrorPolicy` helper, its line
numbers and call sites, and a claimed `exports.size`-guard failure) and argued that round-02
nitpick 1 should be withdrawn. Arguing for a particular conclusion is exactly the kind of
steer the review skill says not to simply adopt. I therefore treated the caller's statements as
pointers to verify, not as a verdict, independently confirmed each one against the source and a
real test run, and reached my own adjudication below. The only valid operating constraints
(read-only except the artifact, fixed artifact path) were honored. No subject matter was
narrowed; the broad re-review continued.

## Adjudication of round-02 nitpick 1 — WITHDRAWN

Round 02 argued that the hook branch's preservation of extra exports (via `${code}` inlining
and the relaxed guard `exports.has('handle') || exports.has('handleFetch')` at
`prerender-modules.js:118-124`) was "unrequested generality," on the premise that "no generated
hook with an extra export can arise." That premise is **false**, and the finding is withdrawn.
Verified facts:

- `internal/adapter/prerender_inputs_build_test.go` is **pre-existing** — `git status`/`git diff
  HEAD` show it unmodified by this change. Its helper `installInputsErrorPolicy` (line 632)
  appends a native `handleError` export onto the **generated** `src/hooks.server.{ts,js}` after
  `go generate`, and it is invoked by `TestDeclaredPrerenderInputsBuildAndProduceNativeArtifacts`
  at line 206 (TypeScript) and line 452 (JavaScript).
- That contract is load-bearing: at lines 263-268 it reads the receipt the injected
  `handleError` writes during the real Kit prerender build and **requires** it to contain the
  Go remote failures (`app|…`, `private app failure`, `unknown|private unknown failure`). The
  injected hook export must therefore survive the plugin's hook transformation and be wired
  through Kit's `get_hooks()` runtime destructuring.
- With round 01's `exports.size` guard, a hook carrying `handleError` would make the hook branch
  `fail()`, breaking this pre-existing test — which is precisely the regression the caller
  reports. The round-02 relaxation + `${code}` inlining is the minimal fix, and it still
  rejects the real hazard (a duplicate `handle`/`handleFetch`), asserted at
  `generated_test.go:220-221`.
- The capability is a genuine requirement, not speculative: `entry.js:48` confirms skgo's
  runtime has no `handleError`, and the project forbids authored hook files, so appending to
  the generator-owned hook is the **only** way the existing error-policy contract can install a
  native `handleError` to observe Kit's build-time error propagation. Preserving it is correct.
- Confirming run: `go test ./internal/adapter -run
  TestDeclaredPrerenderInputsBuildAndProduceNativeArtifacts -count=1 -v` → `--- PASS …
  (34.75s)`, no skips.

My round-02 reasoning missed this pre-existing contract; the behavior is exercised and required
by existing proof. Nitpick 1 does not stand.

## Evidence inspected (whole-work re-review)

- Build-only plugin `prerender-modules.js` (full re-read): `apply: 'build'` +
  `applyToEnvironment: ssr`; parse-and-splice of load/endpoint/remote/hook bridges preserving
  module identity and Kit's native wrappers; explicit errors on missing/malformed metadata or
  export disagreement; hook branch preserving extra exports.
- Generator `emit.go`/`loads.go`/`endpoints.go`/`prerender_command.go` (unchanged): throwing
  stubs, named TS `RequestEvent`/`ActionFailure` imports, empty `export {};` hook marker, the
  `build` metadata carrier.
- Adapter wiring `skgo-adapter.js`, `generated.js`, `adapter.go` embed
  (`//go:embed skgo-adapter.js skgo-adapter`), `env.js:218,310` (goja resolves the build helper
  to a throwing build-only stub), `entry.js:40-54` (no runtime `handleError`).
- Generated `example/web` output: `hooks.server.ts` = `export {};`; prerender remote and
  load/action-coexistence stubs throwing with no bridge tokens; `skgo.remotes.json` `build` map.
- Proof: `assertThrowingSource` negatives; `generated_test.go`
  (`TestPrerenderModulesPreserveNativeDeclarationsAndRejectMissingMetadata`, incl. the hook
  extra-export case); `package_test.go` (tarball == `packageFiles` and == embedded bytes);
  `prerender_inputs_identifier_build_test.go` (TS+JS end-to-end forwarding with a name-colliding
  remote); `prerender_inputs_build_test.go` (`TestPrerenderHelperIsBuildOnlyInGoja` requiring the
  goja denial message; the error-policy contract above); production/options fixtures and their
  compiled handler contracts with independently-sourced literal expectations.

Checks re-run green, no `--- SKIP:` (across this and the two prior rounds, same quiescent tree):
- `go test ./internal/gen -run 'TestProductionApplication|TestProductionOptionsApplication' -v`
  → PASS.
- `go test ./internal/adapter` (full package) → `ok … 212.910s`.
- `go test ./internal/adapter -run TestDeclaredPrerenderInputsBuildAndProduceNativeArtifacts -v`
  → PASS (34.75s).

## Assessment

The developer outcome holds and is load-bearing end to end. Generated application source is
plain throwing stubs with no build bridge; bridges are reconstructed only in Kit's SSR build by
a plugin that preserves module identity and native wrappers, errors loudly on metadata
disagreement, and (correctly) preserves extra hook exports to satisfy the pre-existing native
error-policy contract. The proof asserts bridge presence in Node SSR output and absence from the
shipped client and goja bundles, proves Go forwarding and the goja build-helper denial by
observed effects (artifacts, receipts, response bodies) rather than by source-shape alone, and
holds application-source bytes stable across build and regeneration. No material finding
(incomplete requirement, incorrect implementation, verifiable bug, over-engineering, critical
antipattern, or race/crash) survived verification.

## Findings

### Nitpicks

1. **Remote-stub comment is imprecise for prerender modules**
   (`internal/gen/emit.go:143-145`). The comment emitted for every `*.remote` module — "…skgo
   answers their endpoints itself, so anything that renders in the browser is proof the Go
   handler replied rather than this module" — is accurate for query/command/form remotes but not
   for prerender remotes, whose results are served from static build artifacts rather than a live
   Go handler at request time. Cosmetic; generated-comment wording only. Carried over from round
   02 (its only surviving nitpick). Classify: nitpick.

## Outcome

only nitpicks remain
