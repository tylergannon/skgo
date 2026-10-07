# Stage 4 review, round 2: RequestEvent prerender request lifecycle

**Target:** uncommitted working tree over `996a65e` on `codex/request-event-plan`, after the repair of round 1's
blocking finding ([round 1](20261007-request-event-stage4-opus.md), finding 1).
**Reviewer:** Claude Opus 5.5, independent. I made no implementation edits and used no delegated reviewers.
**Kit pin:** `3.0.0`.

**Outcome: only nitpicks remain**

## Scope

The review covers the whole stage-4 candidate against the plan row "Prerender request lifecycle". I focused on the
new repair:

- the adapter's `prerenderedEndpoints`;
- `Manifest.PrerenderedEndpoints` / `EndpointConfig.PrerenderedEndpoints`;
- the merge in `Endpoints.checkDrift`;
- the fixture's startup and HTTP contracts.

I reused my round-1 checks of the lifecycle itself, because the files involved have not changed since round 1.
`prerender.js`, `prerender_request.go`, `middleware.go` and the prerender service all have modification times earlier
than the round-1 review.

Round-1 findings 2 and 3 are filed as #272 and #273. Stage 5 (resolve options) and stage 6 (whole integration) remain
scheduled in the plan, so this round does not count them as defects.

## Kit mapping of the repaired behavior

- **Inheritance:** `runtime/server/endpoint.js:27` gives an endpoint `state.prerender_default` when it does not
  export its own `prerender`. A Go endpoint fetched while a `prerender = true` page renders therefore answers
  `x-sveltekit-prerender: true` (`:71`).
- **Promotion:** `core/postbuild/prerender.js:455-463` promotes that route to `prerender_map = true`.
- **Builder output:** `core/adapt/builder.js:95` exposes `prerender: prerender_map.get(route.id)` on
  `builder.routes`. Line `:126` drops routes whose value is `true` from the generated server manifest.

As a result, Kit serves only the prerendered files for such a route, and other parameter values do not match any
dynamic route.

The repair mirrors this:

- **Adapter** (`skgo-adapter.js`): records `builder.routes` entries with `prerender === true` that have compiled Go
  endpoint methods. It writes them into `prerenderedEndpoints` in `skgo.manifest.json`.
- **Go** (`endpoint.go:355-370`): counts those declarations in the strict drift comparison, both for missing and for
  extra methods.
- **Dispatch:** `endpointRouting` still builds dispatch only from `Routes`, so these registrations get no dynamic
  dispatch.
- **Overlap guard:** a route listed in both sets is an explicit error.

Routes with `prerender = 'auto'` stay in the dynamic table, as in Kit.

## Checks run

All checks used `PATH="/opt/homebrew/bin:$PATH"`. The worktree's `git status --porcelain` hash was the same before
and after the worktree runs. Probes and mutations ran in a scratch copy at `/tmp/skgo-probe2`, which I deleted
afterwards.

| # | Where | Command / probe | Result |
| --- | --- | --- | --- |
| 1 | — | Compared Sol's log timestamps with source modification times | `stage4-endpoint-drift-generator.log` (full `internal/gen`, ok, 224.624s) finished at 13:06:27. The last source edit was at 13:02:41. The root log (13:03:40), example-build log (13:03:42) and example-tests log (13:05:42) are also later than the last edits to `endpoint.go`, `static.go` and the adapter (12:58:58). I reused these runs instead of rerunning the full suites. |
| 2 | worktree | `go test -count=1 -run 'TestGoPagePrerenderRedirectBuildsKitsNativeArtifact\|TestCompiledProductionPrerenderTransformFailure\|TestCompiledProductionStoredPrerenderError\|TestCompiledProductionNoArgumentNullReachesTheArtifactLookup\|TestCompiledProductionPrerenderedEndpointsAndPages\|TestRealProductionStartsWithPrerenderedEndpoints\|TestRealKitBuildSharesPrerenderLocals' -v ./internal/gen/` | All 7 pass, including the 4 that failed in round 1. This is one real `vp build`, 9.96s. |
| 3 | worktree | `go test -count=1 -run 'TestFullyPrerenderedEndpoints\|TestPrerender' .` | ok |
| 4 | copy | Built the fixture and read `skgo.manifest.json` | `prerenderedEndpoints = {"/lifecycle-api":["GET"],"/lifecycle-cookies/[operation]":["GET"]}`. No `lifecycle*` route is left in `routes`. |
| 5 | copy | Started the compiled production `server` (`LISTEN_ADDRESS=127.0.0.1:0`) and used curl | See the next table. |
| 6 | copy | Mutation A: `checkDrift` ignores `PrerenderedEndpoints` | `TestFullyPrerenderedEndpointsCheckDriftWithoutDynamicDispatch` FAIL. `TestRealProductionStartsWithPrerenderedEndpoints` FAIL ("production failed before listening … disagree about the server routes"). `TestCompiledProductionPrerenderedEndpointsAndPages` and `TestCompiledProductionStoredPrerenderError` FAIL. |
| 7 | copy | Mutation B: the adapter emits an empty `prerenderedEndpoints` | `TestCompiledProductionPrerenderedEndpointsAndPages` FAIL ("fully prerendered endpoint declarations: map[]"). The real-startup and stored-error contracts also FAIL with the drift refusal. |

Requests to the running production server (check 5):

| Request | Response |
| --- | --- |
| `GET /lifecycle-api` | 200 `Go fetch: own locals 10` (the prerendered file) |
| `GET /lifecycle-api/` | 308 |
| `HEAD /lifecycle-api` | 200 |
| `GET /lifecycle-cookies/beta-set` | 200 `cookies updated` |
| `GET /lifecycle-cookies/gamma-set` | 404 HTML document; the parameter value was never built, and the Go handler did not run |
| `GET /lifecycle/alpha` | 200 |
| `GET /lifecycle/gamma` | 404 |
| `GET /ordinary` | 200 (a dynamic page still renders) |
| `POST /lifecycle-api` | 405 |

The 405 comes from the static handler's existing policy for prerendered files (`static.go:632-757`). This change did
not modify that policy.

The mutations show the new contracts are load-bearing. If either half of the repair is removed, the production
binary refuses to start and the suite reports it. The fixture's assertions on unbuilt paths and on `called == 0` also
cover the dynamic-dispatch side: they would fail if these routes were dispatched dynamically again.

## Findings

Round 1's critical finding is resolved.

The repair is narrow. It follows Kit's own route promotion, keeps checking strict in both directions, and adds no
compatibility machinery. Two notes from round 1 remain open; neither blocks:

### Nitpicks

1. **Stale generated hook (unchanged from round 1, finding 4).** A generated `hooks.server.{ts,js}` is not in
   `prune.go`'s `generatedModuleName` set. It survives when an app loses all prerender work, and then fails the next
   build visibly. I derived this from the code and did not reproduce it.
2. **Per-request overhead (unchanged from round 1, finding 5).** `requestHandle` re-imports `manifest-full.js` and
   imports every endpoint module on each prerendered request. The header-delta snapshotting relies on load callbacks
   being serialized through `parent()`, which a comment could note. The adapter has mixed indentation at
   `skgo-adapter.js:21,102`.

## Verdict

The stage-4 lifecycle passes in a real build. The production consequence of that build is now correct: the binary
starts, serves Kit's prerendered endpoint files, and does not run the endpoints for parameter values that were never
built. Sol's full `internal/gen`, root and example runs were made after the final edits, so they are valid evidence
for the current candidate. Only nitpicks remain.
