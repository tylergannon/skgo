# Adversarial review: RequestEvent stage 5 — prerender resolve options (Opus)

**Target:** uncommitted stage-5 working tree over `81e92e1` in `/Users/tyler/.codex/worktrees/bad1/skgo`
(`prerender_request.go`, `prerender_service.go`, `internal/adapter/skgo-adapter{.js,/prerender.js,/prerender.d.ts}`,
`internal/adapter/index.d.ts`, the `prerender-production` fixture additions, and the new tests
`prerender_request_test.go` (additions), `internal/gen/prerender_resolve_options_test.go`,
`internal/adapter/prerender_predicate_test.go`, `internal/adapter/prerender_predicate_failure_build_test.go`).

**Specification:** `CLAUDE.md`; `ephemeral/plans/20261006-request-event.md` (stage table row 5, §2 "Resolve options",
prerender contract 4); `ephemeral/plans/20261006-prerender-predicate-design.md` (agreed relay and its definition of done);
installed Kit **3.0.0** (`example/web/node_modules/@sveltejs/kit/package.json`).

**Scope note:** the launch prompt's "stages 1–4 are accepted" was treated as context, not as a safe area. Where stage-5
changes alter proof of stage-4 behaviour, that is reported (finding 1).

## Evidence inspected

- Full diff and untracked files listed above; surrounding code: `document.go:1087-1117` (`documentOptions`),
  `handle.go:96-140` (`PreloadInput`, `defaultPreload`, `composeResolveOptions`), `prerender_request.go` `begin`/`acquire`/`end`,
  `prerender_service.go` callback binding (the `/resolve-option` context comes from `entry.request.Context()`, so
  `LocalsFrom` and request cancellation reach the Go callback), `internal/gen/prerender_command.go` (generated build main).
- Kit 3.0.0: `src/runtime/server/respond.js:51-58,287-289,594-596` (defaults; `preload` defaults to js/css),
  `src/runtime/server/page/render.js:322,333,348,373,616-621` (`preload` is synchronous after render; `transformPageChunk` is called
  once with `done: true` in 3.0.0), `page/load_data.js:469,485` (synchronous filter during header reads, including
  `set-cookie`), `page/serialize_data.js:52`, `src/core/postbuild/prerender.js:602-637` (non-200 under
  `handleHttpError: 'ignore'` is silently not written).
- Builder worklog `ephemeral/worklog/20261007-prerender-resolve-options.md`; captured logs `ephemeral/tmp/stage5-*.log`.
- **Freshness check of captured logs:** `skgo-adapter/prerender.js` was last modified at 13:33:33. `stage5-build.log`
  (the real Kit build for `TestRealKitBuildAppliesRequestResolveOptions`) was captured at 13:31, before that edit, and
  `stage5-failure-build.log` is an earlier failing run superseded by later logs. I therefore did not reuse those two.
  Reused as current: `stage5-worker-timeout-build.log` (13:36), `stage5-callback-timeout-build.log` (13:39, both timeout
  modes pass, including `BUILD_APP_REJECTED:… predicate timed out after 30000ms` for the worker notice), and
  `stage5-go-checks.log` (13:36).
- **Independent runs at the current candidate** (`PATH=/opt/homebrew/bin:$PATH`, pnpm 12.9.1):
  - `go test ./internal/gen -run 'TestRealKitBuild|TestCompiledProduction' -count=1 -v`: PASS, including
    `TestRealKitBuildAppliesRequestResolveOptions` (real Kit build) and `TestSharedRendererDefaultFilter` (compiled served handler).
  - `go test ./internal/adapter -run 'TestPrerenderPredicateRelay|TestPrerenderNilCallbacks|…/^killed$' -count=1 -v`:
    PASS (`NIL_CALLBACK_TRAFFIC:0 OWNER_CHANNEL_CLOSED`; killed service yields
    `BUILD_APP_REJECTED:… preload resolve callback failed: fetch failed` in about 2.7 s).
  - `go test . -run 'Prerender|ResolveOption' -count=1`: PASS.

### Is the proof load-bearing?

These conclusions come from reading the assertions against the fixture. I did not edit the implementation to mutate it.

- **Synchronous predicates are real booleans:** `options.html` must *lack* `"x-denied"`, `"x-default"` and
  `rel="modulepreload"`, and must contain the font `rel="preload"`. A Promise-returning (truthy) predicate would admit
  `x-denied` and `modulepreload`. An unforwarded option would leave Kit's default filter (nothing serialized) and default
  preload (`modulepreload` present, no font). Each direction fails the test.
- **Callbacks run late, so eager tables cannot pass:** `x-late` is admitted only after the remote `ready()` sets
  `HeadersReady`, which happens after the fetch. The font is admitted only after the page load sets `AssetsReady`. A
  table computed at fetch time or at `begin` fails both checks.
- **Sequence composition:** the transform chain `TRANSFORM_TOKEN → INNER_TOKEN → inner-outer` checks order. The inner
  filter/preload panic if called, which proves the outer filter/preload wins.
- **Universal-load header refusal:** checked through Kit's `load_response_header_not_serialized` text in the captured build
  output. This is anchored on `x-denied`, and the positive `x-public` path shows that the filter really is relayed.
- **Renderer-default parity:** `options-default.html` contains `"x-default"` only through
  `PrerenderServiceOptions.FilterSerializedResponseHeaders`. The served twin route asserts the same header through
  `SSROptions`. Both use the one shared `app.SerializedHeader`.
- **Failure handling:** the relay unit test drives the real private `predicate` code. It covers true/false,
  answer-before-wait (the `not-equal` path), a full error, a malformed reply, and timeout with a late reply. All errors are
  sticky, the call count stays at 1, and the owner receives a notice. The real permissive builds (`handleHttpError:
  'ignore'`) reject `buildApp` and leave no `build/` directory for the killed, owner-timeout and worker-timeout cases.
- **Nil options cause no traffic:** the test counts `predicate` messages at the real owner's channel during a real build
  and also checks that the channel is closed and the env var restored.

Stage-5 behaviour is consistent with the plan and the agreed relay design:

- The existing main-thread owner is reused, with no added worker.
- The option set is the composed one: the hook's options win, then `PrerenderServiceOptions` supplies the default filter,
  which matches `documentOptions`.
- Preload with no callback is left to Kit's identical js/css default.
- Callbacks run against the suspended request's context.
- `fail()` wakes outstanding cells before cleanup can block.

I found no deployment blocker.

## Findings

### 1. Issue (non-blocking): the shared production fixture now builds through a hand-written build service with a permissive error policy, so the generated prerender command and build-level failures are no longer proven there

`internal/gen/prerender_production_fixture_test.go:28` rewrites the fixture's adapter call to
`skgo({prerenderPackage: './buildservice'})` and adds `prerender: {handleHttpError: 'ignore', handleUnseenRoutes: 'ignore'}`.
That fixture is the one stage 4's real-build proof uses: `TestRealKitBuildSharesPrerenderLocalsAndWrapsTheRenderedResponse`,
`TestCompiledProductionPrerenderedEndpointsAndPages`, the remote SSR contracts and production startup.

Two consequences follow:

- **The generated command is no longer built here.** These lifecycle claims now run against
  `testdata/prerender-production/buildservice/main.go`, a hand-written copy, instead of the `prerender/skgo_gen.go` that
  `writePrerenderCommand` emits and that every ordinary app uses.
  - Example failure: change `internal/gen/prerender_command.go:35` to drop `Endpoints: generated.Endpoints()`, or to bind
    something other than `generated.RequestBoundary`. The stage-4 real-build lifecycle tests would still pass.
  - The adapter's minimal-app builds still exercise the generated command with a hook. They have no endpoints, matchers
    or loads with locals, so they cover only part of this.
- **Build-level failures are hidden.** Under `'ignore'`, Kit silently skips writing any non-200 prerendered page
  (`prerender.js:602-637`). Unseen prerender routes are no longer reported either. Tests that read a specific output file
  still notice it is missing, but any prerender error on a page no test reads now leaves the build green.

Only `options-rejected` needs the permissive policy and only the default-filter route needs the custom service. Both
should be isolated so the shared lifecycle fixture keeps building through the generated command under Kit's default
policy. Stage 6 is the natural place to restore this.

### 2. Issue (non-blocking): matching the renderer's default filter requires replacing the generated build command by hand; the default path silently diverges, and the fixture's replacement already drifts

The design requires that "a filter set only on `SSROptions` … filters the prerendered hydration data exactly as it filters
the served page." The implementation adds `PrerenderServiceOptions.FilterSerializedResponseHeaders`
(`prerender_request.go:24-26`), but the generated command never sets it (`internal/gen/prerender_command.go:35`). The
only way to supply it is the new public adapter option `prerenderPackage` (`internal/adapter/index.d.ts:10-13`), which
points at a hand-written copy of the generated `main`.

- **Failure scenario:** an app passes `SSROptions{FilterSerializedResponseHeaders: …}` in its server and uses the default
  `skgo()` adapter.
  - Served pages embed the admitted headers.
  - Prerendered pages run with Kit's `default_filter` (false), so their hydration data omits those headers.
  - A universal load that reads the header on the client after hydration therefore sees `null` on prerendered pages only.
  - Nothing reports the difference.
- **The copy is already drifting:** the fixture's `buildservice/main.go` passes a `nil` transport and a different
  client-address error message from the generated `main`. An app with `transport` hooks copying it would break.
- **Effect:** apps must maintain this duplicated wiring indefinitely, and it is a new public adapter surface.

The plan explicitly left open "where the build obtains" the default, so this is not a plan violation. It is a sharp edge
to resolve before the feature ships. Possible fixes:

- Let generation reference one configured symbol for the default filter, in the same way `HookPackage`/`HookSymbol`
  works, so the server and the generated command share it.
- Document that build-time defaults belong in the hook's `ResolveOptions`, which already reach both paths, and drop
  `prerenderPackage`.

Neither the example app nor the starter sets `SSROptions.FilterSerializedResponseHeaders` today, so no in-tree consumer
is affected now.

### 3. Nitpick: an application error from a Go transform makes the whole build fail regardless of Kit's `handleHttpError`, and this path has no real-build test

In `prerender.js` (`requestHandle`), any failure of the transform's `invokeService` goes through `failPredicateRelay`.
That marks the relay terminally failed and makes the owner fail the build. Kit 3.0.0 treats a throwing
`transformPageChunk` as a page error that is subject to `handleHttpError`.

The terminal-failure rule in the agreed design exists to stop late replies reaching the *synchronous* cell. The transform
uses the asynchronous HTTP bridge, where that hazard does not exist. Treating a transform's own error as a bridge failure
is stricter than Kit. It is safe, but it is undocumented.

Separately, the transform failure path is covered only by the Go 500 contract
(`TestPrerenderResolveOptionDefaultsAndFailures/transform`). No real build demonstrates that a failing transform rejects
`buildApp` under a permissive policy. The worker-timeout build does exercise the same failure-notice mechanism.

### 4. Nitpick: the captured real-build log is stale

`ephemeral/tmp/stage5-build.log` predates the final `prerender.js` edit (13:31 versus 13:33:33). The result held when I
re-ran it, as recorded above, but whoever relies on that log should note the timing.

## Outcome

`material findings remain`

Two non-blocking issues remain: the production fixture's proof coverage (finding 1) and the renderer-default ownership
(finding 2). Neither blocks deploying stage 5's behaviour, and stage 6 is where they should be resolved. The predicate,
transform and preload behaviour is correct against Kit 3.0.0, and its tests were re-run at the current candidate and pass.
