# Stage 4 review: RequestEvent prerender request lifecycle

**Target:** uncommitted working tree over `996a65e` on `codex/request-event-plan`
(stage 4, "Prerender request lifecycle", of `ephemeral/plans/20261006-request-event.md`).
**Reviewer:** Claude Opus 5.5, independent; no delegated reviewers, no implementation edits.
**Kit pin:** `example/web/node_modules/@sveltejs/kit` `3.0.0` (verified from `package.json`).

**Outcome: material findings remain**

## Scope and launch-prompt notes

I derived scope from AGENTS.md/CLAUDE.md and the plan's stage-4 row: "A real Kit build runs the Go hook once
per logical request, shares locals across its load/remote callbacks, and carries the actual response through hook
before/after logic. Go-backed fetches work, refusals preserve their meaning, and completion/cancellation releases
request state". I also used the plan's §2 prerender bridge contracts 1–3 and 5, plus the "Prerender has real request
ownership" row of the definition of done, insofar as they apply to stage 4. Contract 4 (resolve options and the
predicate relay) is stage 5 in the plan's own table, so I did not count its absence as a defect.

The caller said Sol's latest logs all passed. I checked that claim and found the logs were incomplete:
`stage4-cookie-build.log` shows a 12-second `internal/gen` run, which matches a filtered run. The full `internal/gen`
package has not passed since the lifecycle endpoints were added to the shared production fixture (finding 1).

## Evidence inspected

- The full diff over `996a65e` and the new untracked files: `prerender_request.go` and its test, `middleware.go`,
  `prerender_service.go`, `prerender_load.go`, `prerender_remote.go`,
  `internal/adapter/skgo-adapter/prerender.js`, `env.js` and `skgo-adapter.js`, `internal/gen/prerender_command.go`,
  `endpoints.go` and `gen.go`, the generated example stubs and `example/web/src/hooks.server.ts`, the fixture hook
  and the `lifecycle*` routes, and the adapter/gen test changes.
- Pinned Kit source:
  - `runtime/server/respond.js`: the per-request emulator `platform`, `setHeaders` duplicate rule, handle
    Redirect/HttpError handling and `x-sveltekit-routeid`.
  - `runtime/server/endpoint.js:27,71`: endpoints inherit `state.prerender_default` and emit `x-sveltekit-prerender`.
  - `core/postbuild/prerender.js:455-463,749-750`: fetched dependencies promote their route to `prerender=true`.
  - `core/postbuild/fallback.js`: `generateFallback` runs `respond` with `building: true`, so the hook runs.
  - `exports/internal/shared.js` (`HttpError(error)`) and `exports/index.js` `error()` (the body includes `status`).
  - `runtime/server/page/index.js` (concurrent server-load promises).
- Worklog `ephemeral/worklog/20261007-request-event-implementation.md` and the Sol logs under `ephemeral/tmp/stage4-*`.

## Checks run

All runs used `PATH="/opt/homebrew/bin:$PATH"` (pnpm 12.9.1). The worktree's `git status --porcelain` hash was
identical before and after every run in the worktree. Mutating probes ran in a scratch copy at `/tmp/skgo-probe`,
synced from the worktree. I restored that copy afterwards; `diff -rq` showed it identical apart from its symlinked
`node_modules`.

| # | Where | Command | Result |
| --- | --- | --- | --- |
| 1 | worktree | `go test -count=1 -run 'TestRealKitBuildSharesPrerenderLocals\|TestPrerenderHookRefusesBothAuthored' ./internal/gen/` | ok (12.1s, real `vp build`) |
| 2 | copy | `go test -count=1 ./... ./example/...` (the `just test` body) | `internal/gen` FAIL (4 tests, below); `internal/adapter` and `example/devrender` failed only because of the copy's symlinked `node_modules` (see 3 and 4); every other package ok |
| 3 | worktree | `go test -count=1 -run 'TestCompiledProductionStoredPrerenderError\|TestGoPagePrerenderRedirectBuildsKitsNativeArtifact' ./internal/gen/` | **FAIL**: "this binary answers, but the frontend has no +server.ts export for: GET /lifecycle-api, GET /lifecycle-cookies/[operation]" |
| 4 | worktree | `go test -count=1 ./example/devrender/` then `go test -count=1 ./internal/adapter/` | ok (18.7s), ok (196.6s) |
| 5 | copy | real-build probe: a prerendered page whose universal load fetches `/lifecycle-api?probe=error` (the hook returns `skgo.Errorf(418,"teapot literal")`) and `?probe=redirect` (the hook returns `Redirect{307}`) | Rendered `err:418:{"status":418,"message":"teapot literal"}` and `red:307`; this matches Kit 3's `error()` body |
| 6 | copy | real-build probe: `prerenderRequests.close()` logs leaked entries, with an extra unread-body subrequest added | `close: 0 entries`, so JS `/end` releases every logical request in a real build |
| 7 | copy | real-build probe: colocated layout and page Go loads, each calling `SetHeader` | Page passed. Logging showed the page `/load` started after the layout finished, because the bridge awaits `parent()`. My suspected header-duplication race therefore does not occur, and I withdrew it. |
| 8 | copy | Go probe: during prerender, a Go load calls `event.Fetch("/other")`, where `/other` is a manifest page route | `"404 Not Found 404 page not found\n"` |
| 9 | copy | real-build probe: the hook redirects `/probe-redirect`, before resolve and after resolve | Kit `prerender_unseen_routes`. Pinned Kit behaves the same way (the route ID is recorded only from the resolved response's `x-sveltekit-routeid`), so this is not a defect. |

## What holds up

Each item below was checked by running it, not only by reading the diff:

- **Single hook, shared locals.** The literal `layout-11`/`page-12`/`remote-13` values in the real build would fail
  if the hook ran per callback or callbacks lost locals. The after-hook appends a comment to Kit's actual rendered
  body, and its status/Content-Type guard rejects a synthesized response.
- **Fetch isolation.** A Go fetch and a Kit universal fetch to a Go endpoint each get fresh locals (`own locals 10`)
  and `IsSubRequest`.
- **Address and deferred work.** The client address is unavailable at build, and deferred data completes.
- **Refusals through Kit.** Refusals cross into Kit with Kit's own classes and meaning (check 5).
- **Release.** Logical requests are released in a real build (check 6). The Go-level cancellation test drains deferred
  callbacks without running after-logic on a fabricated response.
- **Isolation.** Two suspended begins with different literal values stay isolated.
- **Generated hook.** It refuses authored `hooks.server.{ts,js}` and is inert outside `building`. The goja
  environments replace it with `export {}`.
- **Requestless inputs.** `/inputs` remains requestless.

## Findings

### 1. Critical: a Go endpoint fetched during prerender makes the production binary refuse to start, and `just test` is red

**Category:** verifiable bug / incomplete requirement. The plan requires that Go-backed fetches work, and AGENTS.md
requires `just test` green.

**Mechanism:** Stage 4 newly lets a Kit build execute Go `+server` routes: `endpoints.go` changes the stubs to
`fromGo`, and `remoteEndpoint` was added to `prerender.js`. Pinned Kit then treats such a route as follows:

- An endpoint fetched by a prerendered page inherits `state.prerender_default` (`runtime/server/endpoint.js:27`).
- The endpoint answers with `x-sveltekit-prerender: true` (`:71`).
- The prerenderer promotes the route to `prerender_map = true` (`core/postbuild/prerender.js:455-463`) and writes
  `prerendered/lifecycle-api` and `prerendered/lifecycle-cookies/*`.
- Kit drops fully prerendered routes from `kit._.routes`. The adapter writes `skgo.manifest.json` routes from that
  list (`internal/adapter/skgo-adapter.js:202-212`), so the routes are absent there.

The runtime `NewEndpoints` then runs `checkDrift` (`endpoint.go:348-390`) against `generated.Endpoints()`, which still
registers those routes, and refuses construction.

**Reproduction (check 3, in the worktree):**
`go test -count=1 -run 'TestCompiledProductionStoredPrerenderError|TestGoPagePrerenderRedirectBuildsKitsNativeArtifact' ./internal/gen/`

The output is "skgo: the built frontend and this binary disagree about the server routes. this binary answers, but
the frontend has no +server.ts export for: GET /lifecycle-api, GET /lifecycle-cookies/[operation]". The full package
run (check 2) fails the same way in `TestGoPagePrerenderRedirectBuildsKitsNativeArtifact`,
`TestCompiledProductionPrerenderTransformFailure`, `TestCompiledProductionStoredPrerenderError` and
`TestCompiledProductionNoArgumentNullReachesTheArtifactLookup`. I confirmed (`TestProbeManifest` in the copy) that
`skgo.manifest.json` has no `/lifecycle-api` or `/lifecycle-cookies` route and that `build/prerendered/lifecycle-api`
exists.

**Impact:**
- Any application whose prerendered page (or its loads) fetches one of its Go endpoints during build gets a binary
  whose handler construction fails.
- Kit's specification is that such a route is prerendered: its fetched paths are served as files and it is removed
  from the dynamic route table. The adapter manifest and the Go drift check need to agree with that.
- The candidate's `just test` is red. The "all passed" proof covered only a filtered `internal/gen` run.

### 2. Issue: a Go `event.Fetch` of the app's own page, data or remote route during prerender silently returns a plain 404

**Category:** incorrect implementation. Plan §2 contract 2 says bridge/internal failures "never look like successful
empty data", and §2 says resources unavailable at build must "fail visibly … not be silently skipped". The stage-4
criterion "Go-backed fetches work" also applies.

**Mechanism:** `prerender_request.go:172-178` builds the build-time fetch handler as
`BindRequest(endpoints.Intercept(http.NotFoundHandler()))`. Only Go endpoints are reachable. Any other own-origin
path, such as an existing page, `__data.json` or `/_app/remote/...`, gets a 404 that reads like an ordinary missing
resource. At runtime the same fetch goes through the full bound app stack, and in Kit a server-side fetch of a page
renders it.

**Reproduction (check 8):** I began a logical request with a manifest containing page routes `/page` and `/other`,
then ran a `/load` whose Go load calls `EventFrom(ctx).Fetch(ctx, GET /other)`. The result was
`"404 Not Found 404 page not found\n"`, with no error. A load that branches on `StatusCode == 404` would bake wrong
content into prerendered HTML.

**Impact:** Prerendered output can silently diverge from what the served app returns.
- **Minimum fix:** return an explicit build-time error (5xx or a Go error naming the unsupported path) so Kit's
  prerender policy surfaces it.
- **Full fix:** route the fetch through Kit's own `fetch` for the logical request.

This is not blocking if it is filed, but it is a concrete bug.

### 3. Issue: no committed real-build check covers hook refusals through the bridge

**Category:** incomplete requirement / test coverage. The stage-4 criterion says "refusals preserve their meaning".
The definition of done requires the real build to prove "redirects/errors before load execution". The plan says tests
accompany the stage that introduces the behavior.

**Mechanism:** `TestPrerenderRefusalsSkipResolutionAndPreserveMeaning` (`prerender_request_test.go`) asserts only
Go's JSON answer to `/begin`. The JavaScript half (`requestHandle` mapping `answer.redirect`/`answer.error` to
`internal.Redirect`/`internal.HttpError`, `prerender.js:794-811`) has no committed test. Deleting those two `throw`
lines, or building the wrong class, would leave the suite green while the build fell through to an empty
`final` and failed with an unrelated message.

**Evidence that the behavior currently works (check 5):** In a real build, a hook refusal on a Kit subrequest gave
`418 {"status":418,"message":"teapot literal"}` and `307`. That matches the pinned `error()` body shape. The gap is
the missing load-bearing test, not the behavior. A fixture route like the probe in check 5, asserting literal
status and body in the prerendered HTML, would close it cheaply inside the existing production-fixture build.

### 4. Nitpick: a generated `hooks.server.*` is never pruned when prerender work disappears

`writePrerenderHook` runs only when `hasPrerenderWork()` is true (`prerender_command.go:13-15,38`).
`generatedModuleName` in `prune.go` does not include `hooks.server`. If an app loses its last Go load, endpoint,
prerender remote and `HookPackage` selection, the old generated hook stays. It still calls `requestHandle` at
build time, including from Kit's `generateFallback`, which runs `respond` with `building: true`, against a service
that is no longer generated.

I derived this from the code and did not reproduce it. The case is rare, and the build would fail visibly.

### 5. Nitpick: per-request overhead and style in the bridge

- **Repeated manifest work:** `requestHandle` (`prerender.js:760-769`) re-imports `manifest-full.js` and calls
  `await route.endpoint()` for every route on every prerendered request and subrequest. It could compute the route
  table once per worker.
- **Header delta:** `runPrerenderLoad` and `runPrerenderEndpoint` compute response-header deltas by diffing a
  snapshot of the shared logical-request headers. This is correct only because the generated load bridge serializes
  server loads through `parent()` (check 7). A short comment would keep a future change from introducing duplicate
  `setHeaders` (Kit `header_already_set`).
- **Indentation:** `skgo-adapter.js:21,102` mixes space indentation into tab-indented code.

## Verdict

The core lifecycle works in a real build: one hook per logical request, shared locals, Kit's actual response
reaching the after-hook, Kit-faithful refusals, and release on completion. However, finding 1 means the candidate's
`just test` is red, and a newly enabled path (a prerendered page fetching a Go endpoint) produces a production
binary that refuses to start. That must be fixed before stage 4 is marked done. Findings 2 and 3 are concrete and
can be filed as nonblocking per the stated policy. Findings 4 and 5 are nitpicks.
