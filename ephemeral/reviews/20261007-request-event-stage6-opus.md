# Adversarial review: RequestEvent feature, stage-6 integration (Opus)

## Target

- Worktree `/Users/tyler/.codex/worktrees/bad1/skgo`, branch `codex/request-event-plan`.
- `origin/main..HEAD` is 23 commits, ending at HEAD `f71941a`, plus uncommitted stage-6 changes:
  - the example's `/event-layout/[section]` route, its homepage entry and its contract;
  - the `loads.feature` scenario;
  - the source-edit timeout;
  - the split between the strict generated-entry fixture and the permissive options fixture;
  - the real-build transform-failure mode.
- `origin/main` is an ancestor of HEAD (`085456e`, #269).
- The specification is `ephemeral/plans/20261006-request-event.md`, including its definition of done, plus the predicate appendix it references and `ephemeral/plans/20261007-request-event-handoff.md`. The stage missions in `ephemeral/plans/20261007-request-event-outcomes.json` and the repository's `CLAUDE.md` also apply.
- Kit authority: `example/web/node_modules/@sveltejs/kit/package.json` reports version 3.0.0. This is a patched pnpm install.

### Scope note

The launch prompt says known limits #272/#275/#276 "remain explicit". It also says the source-edit timeout change "is not a performance fix". I treated both as context, not as narrowing.

I measured the dev latency question independently against `main`'s CI and report it below. I did not treat accepted limits as safe areas. Where I found an adjacent gap that is not covered by those limits, I report it.

## Evidence inspected

### Plan, handoff and earlier reviews

I read the plan, the handoff, the stage missions and the stage-5 Opus review. I read the full core implementation:

- `request_handle.go`, `request_metadata.go`, the `middleware.go`/`handle.go`/`event.go` diffs;
- `prerender_request.go`;
- the `prerender.js` diff;
- `internal/gen/layout_params.go`;
- the generated `hooks.server.ts`, the prerender command and `RequestBoundary`;
- `example/server.go` and `example/internal/serverhooks/handle.go`.

### Kit mapping, checked independently

`respond.js:204-209`, `state.js:32,49-55` and `fetch.js:199-207` confirm three things:

- With no adapter `emulate`, `state.platform` is `undefined`.
- Each Kit event, including subrequests forked by `fork_state_for_subrequest`, therefore gets its own `event.platform ??= {}` object in `requestHandle`.
- Subrequest handles cannot overwrite the parent's `skgoRequestHandle`.

The layout data envelope from the running server matches Kit's `uses` format:

- untracked reads: `{"search_params":["route"],"params":["section"]}`;
- tracked reads: `{"search_params":["route"],"params":["section","item"],"route":1}`.

### Running example at http://127.0.0.1:8080, looked at as a person would

The page `/event-layout/red/first/one?route=untracked` rendered `red|first|/event-layout/[section]/[item]/one||1` when signed out. After signing in as `grace` and reloading, it rendered `…|grace|2`. I then followed each link, and each step rendered the expected values:

| Link | Result |
| --- | --- |
| Unread sibling | Layout reused (`…/one|grace|2`, page "Second sibling page") |
| Unread item | Layout reused; URL `/red/second/two` |
| Tracked section | `blue|second|…/two|grace|3` |
| Track route | Serial 4 |
| Tracked sibling | `…/one|grace|5`, page "First sibling page" |
| Tracked item | `blue|third|…/one|grace|6` |

I looked at the screenshot myself. It shows "Signed in as grace", the values line, the six links and the child heading, with no error boundary. The homepage lists "Typed locals and layout request events" with its description.

### Stage-6 captured runs, reused

- **Native Playwright JSON.** Both `stage6-production-e2e.json` and `stage6-development-e2e-final.json` record 186 expected, 0 skipped, 0 unexpected, 0 flaky. The new scenario passed in both modes.
- **`stage6-just-test-final.log`.** This run failed, but only because `TestTheFrontPageIndexesEveryCapabilityItDemonstrates` expected 25 entries and found 26. No other package failed.
- **`stage6-final-example-contracts.log` (14:27).** The example package passed.
- **Files changed after 14:05.** The only files with later timestamps are the e2e config, the homepage contract, and generated or source-edit fixture files that the dev suite rewrote and restored. Git shows no residual diff in `go-dev`, `app.html` or `lib`.
- **Scaffold logs.** The isolated scaffold has edited domain locals. Its authored file hashes are identical before and after repeated generation, and `TestFreshScaffoldServesCustomLocalsAndBothFetchPaths` passed.
  - The first scaffold attempt failed because the repository's `go.work` was in scope. That behaviour is pre-existing in `internal/newapp/newapp.go` on `main` and is environmental.

### Load-bearing check: mutation runs in a scratch copy

I ran these in `/tmp/opusmut`, a copy of the tree without `.git`, using `go test .`. The repository was not modified.

| Mutation | Result |
| --- | --- |
| Drop the locals binding in `bindRequestEvent` | 6 tests fail |
| Skip binding at typed resolve | 6 tests fail |
| Remove the nil-locals check | `TestTypedRefusalNilAndResolveGuards` fails |
| Client-address provider not memoised | 2 tests fail |
| Inner filter wins / inner preload wins | 2 tests fail each |
| Swap transform order | 3 tests fail |
| Before-only adapter forwards the incoming event, not the selected one | Fails |
| Hook-path `IsSubRequest` forced false | 4 tests fail |
| Typed once-only resolve guard disabled | **All pass** (finding 3) |
| `requestMetadataOf`'s `isSub` forced false | All pass; that path is shadowed by hook metadata, so this mutation is not meaningful |

### Dev-latency comparison against main

I compared `main`'s CI run 37658137422 ("Release gate", #269, Example BDD (dev), all passing under Playwright's default 30 s scenario timeout) with this branch's `stage6-development-e2e-final.json`.

## Findings

### 1. Issue (nonblocking for deployment; file it): adding a route in dev now takes about three times as long, and the raised timeout hides it

**Evidence.**

| Source-edit scenario (dev) | `main` CI (#269) | This branch (stage 6) | Ratio |
| --- | --- | --- | --- |
| Authored Go load edited | 10.6 s | 14.2 s | 1.3× |
| Go action edited | 11.0 s | 14.1 s | 1.3× |
| Go endpoint edited | 10.2 s | 13.7 s | 1.3× |
| Page options written | 18.6 s | 24.2 s | 1.3× |
| **New route (Go load + component)** | **21.3 s** | **67.1 s** | **3.1×** |
| **New server route (Go handler)** | **23.1 s** | **69.0 s** | **3.0×** |

Edit scenarios differ by a consistent 1.3×, which bounds the difference between the two machines. The new-package scenarios are about 2.4× worse than that. So the extra cost comes from something specific to adding a package, and it appeared in this branch.

The dev server log `ephemeral/tmp/stage6-dev-server.log` supports this. An ordinary regenerate, build and restart completes in about 7 s, for example 14:14:53 → 14:15:00 → 14:15:07 → 14:15:14. After the new route was added, the first `generating` at 14:15:15 was still running about 45 s later. The scenario's teardown removed the route, and that generation then failed with `open …/go-dev-added: no such file or directory`.

`example/e2e/playwright.config.ts:90` raises the source-edit budget to 120 s. That makes the suite green, but nothing now detects the regression. The worklog already says the change "does not prove generator peak CPU or latency improved".

**Impact.** No production impact, because prod answers these scenarios in under 200 ms. A developer adding a route now waits about a minute instead of about 20 s, and the test that would have caught this was loosened rather than kept. The likely cause is extra cold package loading for a new route package after this branch added locals, hook-package and layout-domain loading. I have not established that cause; the CPU sample is unsymbolized.

**Recommendation.** File this with the numbers above. Do not block delivery on it, consistent with the user's no-perfection rule.

### 2. Issue (nonblocking; undocumented limit of the same class as #275): Go fetches during prerender bypass the application's `HandleFetch` and `HandleError`

**Evidence.**

- `prerender_request.go:188` builds the logical request's fetcher with no `hook`. In `fetch.go:209-216`, `requestFetcher.hook` is the field the runtime uses to apply `FetchConfig.HandleFetch`.
- `PrerenderServiceOptions` (`prerender_request.go:20`) has no `HandleFetch` or `HandleError` field.
- The generated command (`internal/gen/prerender_command.go:35`) supplies only the boundary, `Matchers` and the unavailable address.
- At runtime, the example installs `handleFetch` (`example/server.go:230-246`) and `HandleError` on its `HandleConfig`. That `handleFetch` rewrites `/api/todos?via=…` and `/api/replay/…?alias`, and stamps `X-Fetch-Hook: Go`.
- The build has neither.
- The generated Kit `handleFetch` is only the cookie relay. Universal-load fetches during the build likewise never reach the Go hook.

**Failure scenario.** A prerendered page's Go load calls `event.Fetch("/api/replay/x?alias=1")`, or a universal load reads `x-fetch-hook`.

- At runtime it gets the rewritten `step=rewritten` response, stamped with the header.
- The build fetches the unrewritten URL, so it serializes different data or headers into the static HTML, with no error.
- A hook refusal during the build is also reported without the app's `HandleError` shaping or reporting.

**Why this is reported.** Stage 4 introduced Go-backed fetch during prerender, and the handoff says the build entry "binds actual hooks". Only #275, the renderer-level filter default, is listed as a runtime-default divergence. This is a second, larger divergence of the same kind, and it is not stated anywhere.

**Recommendation.** File it next to #275 and add a sentence to the handoff's limitations. The fix has the same shape as #275: hook-package selection for these hooks, or explicit `PrerenderServiceOptions` fields. This is not a deployment blocker, because no in-tree page currently prerenders through those fetch paths.

### 3. Nitpick: the typed once-only `resolve` guard inside a sequence is not load-bearing

`TestTypedRefusalNilAndResolveGuards/resolve-guards-sequence=true` (`request_handle_test.go:170-187`) wraps the double-resolving hook as `RequestSequence(h, hookMiddleware(nil))`. `RequestSequence` drops the nil inner hook, so the raw `Middleware` resolve guard (`middleware.go:280`) rejects the second call even when the typed guard is disabled.

Disabling `guardedRequestResolve`'s `CompareAndSwap` (`request_handle.go:101`) leaves the whole root suite green.

The typed guard is the only thing that stops a real inner hook from running twice: before-logic twice, plus a second hook-initialized locals forward, when an outer hook resolves twice. Using a non-nil inner hook that counts its invocations would make the test load-bearing. The implementation itself is correct today.

## Deployment-blocker assessment

I found **no material deployment blocker**:

- Locals ownership, explicit forwarding, refusal and nil serialization, request isolation, metadata and address inheritance, and option composition are covered by tests that fail when the behaviour is removed.
- Kit-correct platform-handle separation was confirmed against pinned Kit source.
- The layout reuse and rerun semantics behave correctly in the running app. Kit's client acts on Go's `uses` envelope as expected, and both browser modes are green with no skips.

Findings 1 and 2 are real, nonblocking bugs to file. Known limits #272, #275 and #276 remain as the handoff states, and the stage-6 options-fixture split correctly keeps the strict generated-entry proof separate from the permissive #275/#276 demonstrations.

Merged-main `just test` / `just e2e` verification remains the parent's delivery step. The latest branch `just test` log is red only because of the since-repaired homepage count. The repaired package passed in isolation; there has been no complete green `just test` run since then.

## Outcome

`material findings remain`. These are findings 1 and 2: nonblocking issues to file, not deployment blockers.
