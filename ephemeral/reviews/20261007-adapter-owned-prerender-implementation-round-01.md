# Adversarial review — adapter-owned prerender integration (round 01)

- **Date:** 2026-10-07 (local)
- **Reviewer model:** Claude Opus 4.8
- **Branch:** `codex/adapter-owned-prerender` (worktree `4230`)
- **Target:** The working-tree implementation of `ephemeral/plans/adapter-owned-prerender.md`:
  move all Go-forwarding build bridges out of generated application source and into the
  adapter's build-only Vite integration, leaving generated loads / remotes / endpoints / the
  server hook as plain typed throwing stubs.
- **Spec authority:** Kit's installed pin `@sveltejs/kit@3.0.0` at
  `example/web/node_modules/@sveltejs/kit`.
- **Outcome:** **no findings** (only two genuine nitpicks and one environmental caveat below).

## Caller instruction handling

The launch prompt's phrases "requires Claude Opus consensus" and "no new proof machinery or
scope enhancements" were treated as context about the implementation's constraints, not as a
narrowing of review subject matter, and I confirmed they describe limits the implementer was
asked to honor rather than findings I was told to avoid. I reviewed the whole change broadly
from the plan, the repository instructions, Kit's pin, and the current artifacts. No caller
instruction improperly limited the review; the read-only boundary and the fixed artifact path
were honored.

## Evidence inspected

Generator side (`internal/gen`):
- `emit.go` — remote stub emission; removal of `prerenderFromGo`, `prerenderInputOptions`,
  the `getRequestEvent`/`remoteInputs` imports, and the `hasPrerender` branches; new
  `build` map carrier in `skgo.remotes.json` (`buildDeclaration`/`buildRemoteDeclaration`).
- `loads.go` — load stub now a typed throwing body; `building`/`buildLoad` bridge removed;
  named `RequestEvent`/`ActionFailure` type imports instead of inline `import(...)`.
- `endpoints.go` — method exports now bind to `unimplemented`; `fromGo`/`building` removed;
  `RequestHandler` type import in TS mode.
- `prerender_command.go` — generated `hooks.server.*` reduced to `export {};`.

Adapter side (`internal/adapter`):
- `skgo-adapter/prerender-modules.js` (new) — the build-only `load` hook that re-injects
  bridges; `skgo-adapter.js` wiring it into `vite.plugins.pre`; `generated.js` threading the
  `build` field; `prerender.d.ts`; `package.json` + `package_test.go` packaging.

Proof:
- Generated committed example under `example/web/src` (hooks, `about.remote.ts`,
  `about/+page.server.ts`, and a `grep` sweep of all committed `src`).
- Kit pin `src/core/sync/write_server.js` for hook discovery semantics.
- Tests: `javascript_loads_test.go`, `prerender_test.go`, `endpoints_test.go`,
  `language_application_test.go`, `adapter_test.go`, `prerender_inputs_identifier_build_test.go`,
  `server_only_build_test.go`, `generated_test.go`, and the production fixture
  (`prerender_production_fixture_test.go` + `application_fixtures_test.go`).

Checks run (all green, no skips):
- `go test ./internal/adapter -run TestPrerenderModulesPreserveNativeDeclarationsAndRejectMissingMetadata` — PASS.
- `go test ./internal/gen -run TestProductionApplication` — PASS, all 10 subtests incl.
  `AdapterOwnedPrerenderModules`, no `--- SKIP:`.
- `go test ./internal/gen -run '<changed generation contracts>'` — PASS.
- `go test ./internal/adapter -run TestPrerenderInputsOuterVPCLISignalDrainsBlockedProducer` — PASS in isolation (see caveat).

## Assessment of the developer outcome

The change achieves the plan's core boundary. Generated application source is now plain:
loads throw with their exact Go-derived return shape and a named `RequestEvent` import
(`example/web/src/routes/about/+page.server.ts`), remote prerenders keep Kit's native
`prerender(...)`/`prerender('unchecked', ...)` wrappers with a throwing body and no options
(`about.remote.ts`), endpoints bind each declared method to `unimplemented`, and the hook is
an empty `export {};` marker. A repository-wide grep of committed `src` finds no `building`,
`buildLoad`, `prerenderFromGo`, `skgoPrerender`, `event.platform`, `getRequestEvent`,
`@skgo/sveltekit-adapter/prerender`, or inline `import(...)` in any generated file (the two
hits are an authored `+layout.svelte` using `$app/env` and a Go comment — not generated code).

The bridges are reconstructed only in Kit's Node build environment by
`prerender-modules.js`, which is `apply: 'build'` and `applyToEnvironment: ssr`. Its `load`
hook parses the generated module with the application's own Vite parser (preserving module
identity and the original id, so Kit registers the original remote IDs/kinds), then splices
in: the Go load bridge with Kit's own `error`/`redirect`; the endpoint bridge per method; the
remote bridge via `getRequestEvent()` plus a lazy `remoteInputs` options callback for declared
inputs; and the hook's `requestHandle`/`requestFetch`. I traced the overlapping body/inputs
edit arithmetic (descending-start application with the inputs insertion at `callback.end`) and
it produces the correct `prerender(validate?, fn, { inputs })` arity for both the no-argument
and argument-taking shapes. Missing or malformed `build` metadata, a declaration that
disagrees with the parsed exports (wrong arity, renamed export, a `prerender` count mismatch,
a build declaration attached to an authored module, or a server module with no declaration) is
an explicit thrown build error rather than a fallthrough to a stub — matching the plan's
requirement and exercised by both the isolated node test and the mixed-kind fixture
(`query.live` + `command` + no-arg + argument/inputs prerender).

I verified the one mechanism most likely to be subtly wrong against the Kit pin: the empty
`export {};` hook. `src/core/sync/write_server.js` discovers the hook by file existence
(`resolve_entry`) and `get_hooks()` destructures `{ handle, handleFetch, ... }` from a runtime
`await import(server_hooks)`. Because discovery is existence-based and binding is at runtime,
the build-time injected exports are picked up correctly; the empty marker is sufficient, as the
plan claims.

## Load-bearing judgement of the tests

The tests are genuinely load-bearing, not green-by-construction:

- Generation contracts assert the literal typed throwing output **and** call
  `assertThrowingSource`, which fails on any residual bridge token
  (`building`, `buildLoad`, `skgoPrerender`, `event.platform`, `getRequestEvent`,
  `@skgo/.../prerender`, `inputs:`) and on inline TS `import(...)`. An empty file fails the
  positive literal checks; a file still carrying the old bridge fails the negatives. Both
  directions bite.
- `TestPrerenderModulesPreserveNativeDeclarationsAndRejectMissingMetadata` drives the real
  plugin over a mixed-kind module, asserts the native `query.live`/`command` wrappers survive
  untouched, asserts the injected remote/inputs bodies, asserts the source file on disk is
  byte-unchanged, and asserts the three disagreement/missing-metadata error paths throw.
- `testAdapterOwnedPrerenderModules` runs against a **real Kit build**: source bytes unchanged
  after build, stable across a second `go generate`, every generated `src` module passes
  `assertThrowingSource`, the Node SSR output (`.svelte-kit/output/server`) positively contains
  `skgoPrerenderLoad`/`skgoPrerenderEndpoint`/`requestHandle`, and the shipped client
  (`build/client`) and embedded goja engine bundle (`build/ssr/bundle.js`, written by the
  adapter's `adapt`) contain none of `skgoPrerenderLoad`/`skgoPrerenderEndpoint`/
  `skgoPrerenderRemote`/`skgoRequestHandle`. I confirmed `skgoRequestHandle` is a real
  build-only `event.platform` property in `prerender.js`, and that `build/ssr/bundle.js` is the
  adapted engine bundle (not the intermediate `build/.goja`), so the absence scan targets the
  correct shipped artifacts. `skgoPrerenderRemote` is deliberately not asserted *present* in
  SSR because Kit tree-shakes non-dynamic remote callbacks after crawling; remote forwarding is
  instead proven by the artifact/result contracts — consistent with the plan.
- The compiled production handler contracts (redirect artifact, prerendered endpoints/pages,
  no-argument null → artifact miss, stored error, transform failure, resolve-option defaults,
  shared locals) run against the one real build and are gated by `run()` requiring an explicit
  `--- PASS` and forbidding `--- SKIP:`.

## Findings

No material findings (no incomplete requirement, incorrect implementation, verifiable bug,
over-engineering, critical antipattern, or race/crash) survived verification.

### Nitpicks

1. **`requestHandle` SSR-presence check keys on a generic identifier.** In
   `prerender_production_fixture_test.go:202` the positive SSR assertion uses the bare name
   `requestHandle`. Unlike the `skgo*`-prefixed properties it is a plausible collision with
   unrelated code; it happens to be unambiguous in today's un-minified Node SSR output, but the
   `skgo`-prefixed hook property would be a more robust anchor. Cosmetic.

2. **Leak detection is coupled to literal property names.** Both the presence and absence scans
   match string literals (`skgoPrerenderLoad`, etc.). A future rename that moved the injection
   and the checks together would stay honest, but a partial rename could open a silent gap.
   Inherent to string-grep proofs and low risk given the build-env restriction also guards the
   boundary.

## Environmental caveat (not a finding about the change)

During the review the worktree was being modified concurrently: three adapter test files
(`package_test.go`, `prerender_inputs_identifier_build_test.go`, `server_only_build_test.go`)
appeared as modified between my first and later `git status`, and a whole-package
`go test ./internal/adapter` run failed once in
`TestPrerenderInputsOuterVPCLISignalDrainsBlockedProducer` with a transient
"removed stale link … server.go: no such file or directory". That test passes cleanly in
isolation, and the failure is consistent with another session mutating generated fixtures
mid-run rather than any defect in this change. If a clean consensus signal is needed, re-run
`just test` once the worktree is quiescent; the targeted, load-bearing suites above all pass.

## Outcome

no findings
