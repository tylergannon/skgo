# Adversarial review — RequestEvent plan, round 02

**Target:** `ephemeral/plans/20261006-request-event.md` (untracked, worktree `bad1`, baseline
`916ccdc`), revised after round 01.
**Kind:** design plan; implementation deferred until #262 lands. No code under review.
**Operating constraints honoured:** read-only except this artifact. No caller narrowing detected.

## Evidence inspected

Everything listed in round 01 (repository instructions, pinned Kit 3.0.0 sources, current skgo
event/hook/load/remote/fetch/document code, generator, scaffold, issue #262), re-read where the
plan changed, plus for this round: `fetch.go:415–470` (subrequest context construction),
`document_fetch.go:70–100` (SSR render-time fetch host), `example/server.go:197,230,247,300–308`
(which handler answers each fetch kind), `internal/newapp/gofiles/server.go.tmpl:45,69,96`
(scaffold handler assembly), `example/internal/skgo/prerender/main_gen.go:13` and
`prerender_service.go:48–120` (prerender service assembly), `middleware.go:151–160` (bypasses),
`handle.go:262–300` (`refuse` serialization), and Kit `src/core/postbuild/prerender.js:396–406`
(prerender goes through `respond`, so Kit's `handle` runs during prerender).

## Round-01 findings — status

1. Public hook surface unspecified → **resolved** (§2 declares `RequestMiddleware`,
   `RequestHandle`, `RequestSequence`, `Intercept(cfg, makeParams, next)`, `RequestLocals`,
   `HandleConfig.ClientAddress`, `--locals-package/--locals-type`; generated `params` wrappers).
2. Pointer replacement unacknowledged / binding instant ambiguous → **resolved** (Kit's readonly
   `locals` named as the pin; one binding instant at final resolve; nil → `Errorf(500)` with the
   refusal shapes that `handle.go:262–300` actually produces; late write a documented no-op).
3. Migration scope vs #262 → **resolved** (in-tree update only; #262 acceptance referenced, not
   restated).
4. Unrunnable DoD rows → **resolved** (hydration row now a load-data claim; race-detector row
   removed).
5. Nitpicks → incorporated (matcher-package cycle, Kit's name-dedupe stated as a deliberate
   extension, file-kind-keyed scan, lazy address result).

## Findings

### 1. Issue — the "single application binding boundary" is not on every stack that runs application handlers

§2 makes the typed `Intercept` mandatory and removes every other source of locals: "Loads/remotes/
endpoints do not independently create locals", `RequestLocals` "outside a bound request … returns
nil", and the scaffold/example "always mount `params.Middleware(nil).Intercept(handleCfg,
handlers)`". The plan names only the main request stack. Two other stacks execute application
handlers today and do not pass through any hook:

- **SSR render-time fetch.** A universal load's `fetch` of an own route is answered by
  `SSROptions.Fetch` (`document_fetch.go:79`). The scaffold sets it to
  `endpoints.Intercept(http.NotFoundHandler())` (`server.go.tmpl:45,69`) and mounts no
  `FetchConfig` at all (zero occurrences in the template). Today that is harmless because the
  library itself allocates fresh locals for every subrequest (`fetch.go:424–426`) and `LocalOf`
  is nil-safe. Under the plan an endpoint in a scaffolded app that does
  `app.LocalsFrom(r.Context()).Session` dereferences nil when reached from an SSR fetch.
  The example routes both fetch kinds through the full `app` stack (`example/server.go:197,
  230, 247, 305`), so only the example would be covered.
- **Prerender.** The generated prerender command is `RunPrerenderService(transport, loads,
  remotes)` (`main_gen.go:13`); `prerenderHandler` (`prerender_service.go:96`) runs loads and
  remotes with no `HandleConfig`, no hook and no locals store. Kit prerenders through `respond`
  (`prerender.js:396–406`), so its `handle` hook runs and populates `locals` at build time. The
  plan's DoD already assumes a boundary exists here ("build/prerender address failure") but §2
  never says the prerender service binds locals, and a prerendered load that reads locals goes
  from "absent" today to a nil dereference.

Impact: the plan's own acceptance ("a generated typed application must install this boundary";
"No-hook path supplies an empty object") is unmet for the default scaffold and for every
prerendered load, and the failure is a crash rather than a diagnostic.
Fix: enumerate the stacks — main app, `SSROptions.Fetch`, `FetchConfig.Handler`, the generated
prerender command — and require each to pass through the typed boundary (nil hook at minimum, with
`ClientAddress` erroring under prerender). Decide and state whether the application's hook runs
during prerender (Kit: yes); if not, record it as a mapped deviation rather than leaving the
prerender boundary implicit.

### Nitpicks

- §4 says a subrequest uses the parent's lazy address result. A subrequest's context is
  `valuelessContext{ctx}` (`fetch.go:422`) and only `RemoteAddr` is copied (`fetch.go:438–440`),
  so the result must be carried explicitly, as `ssr.CarryDepth` is. The DoD's "internal fetch"
  metadata row should state the observable that proves it: with a provider that reads a forwarded
  header the subrequest (which lacks the header) still reports the parent's value. Kit's basis:
  `state.js:53` spreads the *function*, which closes over the platform request.
- §2 `Intercept(cfg, makeParams func(*Event) (P, error), next)`: the error path is unshaped. For
  remotes the analogous generated-constructor failure is a 503 `callerManifestDrift`
  (`remote_caller.go:23–25`); say whether the hook boundary refuses the same way or treats it as
  a 500 configuration error.
- §2 "requesting a different locals type inside a bound request is … handled by the existing
  panic/error boundary": name it as a panic with a message that names both types, so the
  `OnPanic` record (`middleware.go` `runGuarded`) is actionable.

## Outcome

`material findings remain`
