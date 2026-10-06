# Adversarial review — typed-load foundation, round 01

Date: 2026-10-06 (local). Worktree: `/Users/tyler/.codex/worktrees/e9c3/skgo`,
branch `codex/typed-load-foundation`, HEAD `68e8db3` on top of `origin/main`
`b3984d8`, plus staged/working-tree changes. Reviewed as `git diff origin/main`
(working tree), read between ~11:50 and ~12:00.

**Caveat on the target.** The worktree was being modified by another process
while this review ran: `data.go`/`document.go`/`endpoint.go` moved from
`MM` to staged between my first two commands, and every
`example/internal/skgo/links/*/skgo_remotes_gen.go` acquired an unstaged change
(stripping the `Skgo_*` published-name block) at 12:00:16, after my test runs
had finished. Findings below are against the source as read; nothing in this
review wrote to the project other than this file.

## Scope

Derived from the authoritative sources, not the launch prompt: CLAUDE.md
("Kit is the specification", contracts are Go tests at the real handler,
skips are failures, never derive expectations from the thing under test),
Kit 3.0.0 installed at `example/web/node_modules/@sveltejs/kit` (verified
`package.json` version `3.0.0`), and the user contract restated by the caller
(route-local `RouteParams` with private typed storage and tracking getters,
explicit `RequestEvent[RouteParams]`, conversion before reads without
dependencies, named matcher types/methods, zero/false/empty vs. optional
absence incl. nil interface, per-load conditional dependencies and `Untrack`,
Kit route precedence / rejection fallback / optional+rest semantics, params
regenerated before stale handlers compile; minimal fixes for document/data
double decoding, encoded slashes, malformed paths before loads, captured
optional-matcher rejection with demonstrated matcher invocation).

The caller's list of excluded implementation areas (descendant layout typing,
app-wide union params, remote API changes, route-ID/locals work, lifecycle
retry) was treated as a constraint on the solution, not on what I may report.
No caller instruction narrowed the defects I could consider; nothing was
ignored.

## Evidence inspected

Kit source read: `src/utils/routing.js` (`parse_route_id`, `exec`,
`run_matcher`, `find_route`), `src/utils/url.js` (`decode_pathname`),
`src/runtime/server/respond.js` (decode failure path → `handle()` →
`respond_with_error(400, 'Malformed URI')`; `x-sveltekit-pathname`),
`src/runtime/server/page/load_data.js` (`load_server_data`: params Proxy
tracking, `untrack` `finally { is_tracking = true }`, returned `uses`),
`src/exports/params/index.js` (`defineParams`).

skgo diff read in full: `load.go`, `loadevent.go`, `request_event.go` (new),
`routing_path.go` (new), `data.go`, `document.go`, `endpoint.go`,
`middleware.go`, `proxy.go`, `static.go`, `prerender_load.go`,
`internal/gen/{load_params.go,emit.go,scan.go,check.go,gen.go,prune.go}`,
`internal/adapter/skgo-adapter/prerender.js`, `internal/advice/{catalog.go,
origin.go}` and testdata stub, example `src/params.{go,ts}`, both typed-load
routes with their generated `skgo_params_gen.go` / `+page.server.ts`, e2e
feature and steps, and surrounding unchanged code (`normalizePath`,
`kitPattern`, `matchRoute`, `Endpoints.match`, `NewLoads`/`loadRouting`).

Tests read for load-bearing-ness: `typed_load_test.go`,
`routing_path_test.go`, `prerender_load_test.go`, `example/server_test.go`,
`example/routing_path_test.go`, `example/devrender/routing_path_test.go`,
`example/web/src/params_test.go`, `internal/gen/load_params_test.go`, and the
generator test diffs. They are load-bearing: expectations are literal
fixtures (route patterns/params copied from Kit 3.0.0's `parse_route_id`,
literal receipts, literal `uses.params` lists, matcher call counts), the
gen tests compile real generator output and hit the real handler, and
`TestLoadRoutingPathContracts` forces the optional matcher to actually run
(`matcherCalls == 1`, `Lang` asserts it saw `abc`) rather than only asserting
absence.

Tests executed by me (PATH prefixed with `.tools/bin` as the Justfile does):

- `go test -count=1 .` → `ok 2.203s` (plus a filtered run of
  `TestTypedLoad*|TestLoadRoutingPathContracts|TestPrerender*` → PASS; the one
  `panic(...)` line in `-v` output is the recovered panic a test expects).
- `go test -count=1 ./internal/gen -run 'LoadParams|Params|Stale|Ownership|Evolution|PrerenderRedirect|JavaScriptLoads|Prune'`
  → `ok 85.194s` (includes `TestTypedLoadParamsRefreshBeforeStaleHandlerCompilation`,
  `TestTypedLoadActualParameterDependencies`, `TestTypedLoadMatchersPrerenderNamedGoValues`,
  `TestTypedLoadJavaScriptMatcherContract`).
- `go test -count=1 ./example/ ./example/devrender ./example/web/src -run 'TestSharedPathDecoding|TestTypedDependencySerialsBelongToEachCaller|TestEveryPrerenderedPathIsServedFromItsFile|TestOrderMatcherBoundaries|...'`
  → all `ok`.
- `go test -count=1 ./cmd/skgo -run 'TestCheckReportsQueryAdviceAtAuthoredLocations|TestCheckReportsRefreshAndRouteAdviceAtAuthoredLocations|TestCLIAndInitializedStdioMCPExposeSameAdviceAndFailedCheck'`
  → `ok 14.405s`.

Captures: `ephemeral/tmp/test.log` is **red** — `FAIL github.com/tylergannon/skgo/cmd/skgo`
(three tests, lines 1650–1656), every other package `ok`, zero `SKIP`.
The failures do not reproduce when I run them; the worklog's own friction note
says the capture was taken with `GOFLAGS=-v`, which makes `skgo check`'s child
`go build` print package names that it reports as unrecognized output, and one
failure also recorded a `skgo` status of "typed loads require installed
SvelteKit route metadata" that the current source does not produce for that
fixture. Either way the capture is not evidence of a green suite; my rerun is.
`vet.log` holds only the command line (no output = clean). `build.log` shows
a successful adapter build. No browser suite was run (per constraints), so the
two new Gherkin scenarios are unverified here.

Out-of-tree probe (scratchpad module with `replace` to this worktree; not
written into the repo): registry with routes `/orders/[n=Order]` and
`/plain/[n]`, **no loads registered** — see finding 1.

## Findings

### 1. critical — incorrect implementation: a matcher route is unreachable unless some Go load registers that matcher

`load.go:433-437` (`execMatchedParams`): when `matchers != nil` and
`matchers[param.Matcher] == nil`, the route is silently rejected. `NewLoads`
(`load.go:212`) always creates a non-nil `ls.matchers` and fills it only from
`load.matchers` (`load.go:234-236`); the generator only emits
`SkgoParamMatchers()` into packages that contain a `skgo.Load`
(`emit.go:664`, `load_params.go:130-145`), and only requires a Go matcher for
routes that have a Go load (`load_params.go:150-157`).

Consequences, both reachable in an ordinary SvelteKit app:

- An app with matcher routes and **no Go server load anywhere** (pure
  `+page.svelte`, or `+page.ts` universal loads): every `[x=matcher]` route
  404s for both the document (`SSR.serve` → `matchValues`) and `__data.json`.
- A matcher declared in `src/params.ts` without a Go twin, used by a route
  that has no Go load: generation succeeds and the route 404s at runtime.

Kit's `find_route`/`exec` runs the JS matcher and matches. This is the
opposite of "Kit route precedence and rejection fallback": a valid route
falls through to a 404, and nothing in generation or `NewLoads` reports it.

Reproduction (run against this worktree):

```
cfg := skgo.LoadConfig{Origin:"http://127.0.0.1:8080", Nodes:[]string{""}, Routes:[]skgo.ManifestRoute{
  {ID:"/orders/[n=Order]", Pattern:"^/orders/([^/]+?)/?$", Params:[]skgo.ManifestParam{{Name:"n",Matcher:"Order"}}, Page:&skgo.ManifestPage{Leaf:0}},
  {ID:"/plain/[n]",        Pattern:"^/plain/([^/]+?)/?$",  Params:[]skgo.ManifestParam{{Name:"n"}},                 Page:&skgo.ManifestPage{Leaf:0}},
}}
ls, _ := skgo.NewLoads(cfg)
// GET /orders/42/__data.json -> 404 Not Found
// GET /plain/42/__data.json  -> 200 {"type":"data","nodes":[null]}
```

Expected: `/orders/42` matches (Kit does). Note also that Kit throws (500) on
a matcher name with no definition; skgo's silent skip is a third behaviour.
The fix belongs with the matcher table, not the loads: every declared matcher
must reach the registry regardless of which routes carry loads (and a
TS-only matcher on any route must be a generation error, as it already is for
routes with loads).

### 2. issue — incomplete requirement: Go's own `+server.ts` routes still match without matchers

`endpoint.go:603-616` (`Endpoints.match`) calls `execParams`, i.e.
`execMatchedParams(..., nil)`, so a `/api/[id=Order]/+server.ts` answers
`/api/banana` where Kit's `find_route` would reject the candidate and continue
to the next route or 404. Before this diff that was a documented gap for the
whole server ("matchers are deliberately absent"); this diff removes the
documentation (`load.go:368-369`) and adds Go matchers for loads only, leaving
the server's route table matcher-aware for pages and matcher-blind for
endpoints and for the `cfg.routes` fallback in `handle.go:289-297`. CLAUDE.md:
"Every server endpoint is Go's" and "a mirrored feature must obey kit's
rules". No test covers an endpoint with a matcher param.

### 3. nitpick — Malformed URI is answered before `handle` and as plain text

`middleware.go:171-174`, `endpoint.go:400-403`, `data.go:63-66`,
`document.go:504-509 / 563-568`, `proxy.go:102-106` return
`http.Error(w, "Bad Request", 400)`. Kit (`respond.js` ~299 and ~599-608)
sets `resolved_path = null`, still runs the app's `handle` hook with the raw
`event.url`, and renders a 400 via `respond_with_error` (root layout +
`+error.svelte`). The user's stated requirement ("malformed paths before
loads") is met; the deviation is that a `handle` hook which sets security
headers or logs will not see these requests, and the body is not Kit's error
page. Covered by `TestLoadRoutingPathContracts` and `TestSharedPathDecoding`
only as "status 400 and the load/matcher did not run".

### 4. nitpick — prerender bridge has a silent-zero fallback and a misleading comment

`prerender.js:757-766` now sends `params: {}` and only sends
`routePattern/routeParams/routePath` when the route is found in
`manifest-full.js`; if it is not, `RunPrerenderLoad` (`prerender_load.go:93-108`)
skips matching and every typed accessor yields its zero value with no error.
`request_event.go:35-39` says "Prerender bridges and legacy registrations can
supply string params", but a string reaching `value.(T)` for a non-string `T`
panics (which `runLoad` turns into a 500), so the fallback is only safe for
`string` params. Prefer failing loudly when the route is missing and dropping
the string fallback rather than keeping two half-paths.

### 5. nitpick — capture is not green

`ephemeral/tmp/test.log` records `FAIL github.com/tylergannon/skgo/cmd/skgo`.
The three tests pass on rerun, but a capture produced with `GOFLAGS=-v` (per
the worklog) is not a usable proof artifact for `just test`; whichever
process produces these logs should run the Justfile recipe unmodified.

## Notes (not findings)

- Layout loads receive only their own directory's params as typed accessors
  (`prepareLoadParams` derives the id from the directory); Kit's layout
  `params` include the leaf's. Excluded by the user; `event.Param(name)` on the
  embedded `*Event` still reads and tracks descendant params, so there is a
  working path.
- `Untrack` now matches Kit's `finally { is_tracking = true }` (nested
  untrack re-enables tracking on exit of the inner callback) — verified against
  `load_data.js` and asserted by the `read=nested` case in
  `TestTypedLoadActualParameterDependencies`.
- `decodePathname` reproduces `decode_pathname` (split on `%25`, `decodeURI`
  semantics: reserved escapes survive, invalid UTF-8 rejected) and
  `execMatchedParams` decodes captures with `url.PathUnescape` ≈
  `decodeURIComponent`; the rest-param-with-matcher `''` case and the
  chained-optional buffering follow `exec` line for line.
- Generation order is right for the "refresh before stale handlers compile"
  requirement: `prepareLoadParams` runs before `loadApp` in both `Run` and
  `Check`, and `TestTypedLoadParamsRefreshBeforeStaleHandlerCompilation`
  proves the new params file lands even when the old body fails to compile.
- `prepareLoadParams` shells out to `node` at generate/check time to run Kit's
  own `parse_route_id`; that is build-time only and consistent with "Node is a
  build-time dependency".

## Outcome

material findings remain
