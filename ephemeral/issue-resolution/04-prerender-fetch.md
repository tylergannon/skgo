# Group 4: Correct prerender fetch destinations

## #272 Prerender Go fetch returns a misleading 404 for existing app page/data routes

https://github.com/tylergannon/skgo/issues/272

During prerender, Go Event.Fetch dispatches through the bound Go endpoint handler only. Fetching an existing same-origin page, data route or remote route falls through to a plain 404, although runtime dispatch reaches the full app stack. A load branching on that status can silently bake incorrect output.

Opus reproduced this in stage-4 RequestEvent review with manifest page routes /page and /other: a Go load fetching /other received `404 page not found` without a Go error. Kit renders the page for this server-side fetch.

Route such fetches through the appropriate Kit request context, or explicitly report an unavailable build-time capability rather than pretending the known route is missing. Add a real build/served comparison with literal expected content. Existing Go endpoint fetch and its request isolation must remain intact. Nonblocking follow-up; the separate production endpoint-manifest startup bug is being repaired in the feature branch.

### Comment by tylergannon at 2026-10-18T16:57:50Z

**Triage — 2026-10-18**

Checked against current [`main` at `920b6aca`](https://github.com/tylergannon/skgo/tree/920b6aca738a65d6c046a67f4db2373596fafd03). Classification: `bounded-hard`.

Still applicable: prerender_request.go constructs a Go endpoint-backed request fetcher, while native development routes fetch continuations back through Kit and production uses the full app handler. Runtime destination coverage therefore does not demonstrate build-time page/data dispatch. Implement a real build/served comparison and either correct Kit-context routing or explicit capability refusal; a known page must not masquerade as a missing route.

<!-- skgo-issue-triage-20261018-920b6aca -->

## Initial orchestration assessment

Goal: during a real build, Go Event.Fetch must not silently report a known same-origin page/data/remote destination as a missing404. Preserve correct Go endpoint fetch and per-request isolation. The issue explicitly accepts correct Kit-context routing or an explicit unavailable-capability error when that build-time context is unavailable.

Proof: real build/served comparison with literal known-route content or positive explicit-capability refusal, actual missing-route distinction, and endpoint isolation retained. Go tests at handler/build boundary; no browser needed for pure HTTP semantics.

Potential design question: does pinned Kit and the current prerender bridge already offer the appropriate fetch continuation context, or is explicit refusal the bounded supported result? Resolve from source/cache and technical feasibility; both stated outcomes are already authorized by the issue, so no HITL choice is presently established.

## Token-cache annotation: executable

Research entered through the [cache index](/Users/tyler/src/skgo-token-cache/index/README.md), then [fetch/page-option routes](/Users/tyler/src/skgo-token-cache/index/upstream-docs/topics.md:24) and the [native-Kit callback mapping route](/Users/tyler/src/skgo-token-cache/index/dev-native-kit-uds.md:10). The latter distinguishes direct Go load/remote callbacks from `event.fetch`, and identifies the prerender lifecycle as reusable prior art; it does not establish that a Go build callback can already call back into Kit. Cached [fetch documentation](/Users/tyler/src/skgo-token-cache/repos/dev-experience-20261015/frameworks/kit-3.0.0-upstream/documentation/docs/20-core-concepts/20-load.md:272) explains relative/credentialed server fetch. Current authoritative source is installed [Kit 3.1.0](/Users/tyler/.codex/worktrees/e413/skgo/example/web/node_modules/@sveltejs/kit/package.json:3).

- Pinned [create_fetch](/Users/tyler/.codex/worktrees/e413/skgo/example/web/node_modules/@sveltejs/kit/src/runtime/server/fetch.js:18) calls the application handleFetch with a supplied continuation. Same-origin, in-base destinations pass through asset/prerender special cases, credential inheritance and then [internal_fetch](/Users/tyler/.codex/worktrees/e413/skgo/example/web/node_modules/@sveltejs/kit/src/runtime/server/fetch.js:199), which forks subrequest state and calls the full Kit `respond`. Response cookies are harvested into the originating event. A page/data/remote route is therefore not generally absent merely because the Go endpoint registry does not match it. Preserve exact routing/base/method constraints rather than treating every path as a page.
- Current [Go prerender request setup](/Users/tyler/.codex/worktrees/e413/skgo/prerender_request.go:201) binds its fetcher to `Endpoints.Intercept(http.NotFoundHandler())`. The normal [requestFetcher subrequest path](/Users/tyler/.codex/worktrees/e413/skgo/fetch.go:430) deliberately drops originating locals/context values and preserves request facilities. The misleading404 arises from destination coverage, not permission to weaken this isolation.
- Current [prerender requestHandle](/Users/tyler/.codex/worktrees/e413/skgo/internal/adapter/skgo-adapter/prerender.js:815) has the full Kit event/route metadata and a Go request handle while `resolveRequest(event, options)` executes. [remoteLoad](/Users/tyler/.codex/worktrees/e413/skgo/internal/adapter/skgo-adapter/prerender.js:674) likewise has a Kit event, but sends a serialized load operation then awaits its response; [remoteFunction](/Users/tyler/.codex/worktrees/e413/skgo/internal/adapter/skgo-adapter/prerender.js:751) does the same for a remote. Those are request context ingredients, **not an existing reverse continuation channel from a running Go Event.Fetch**.
- Current [build requestFetch](/Users/tyler/.codex/worktrees/e413/skgo/internal/adapter/skgo-adapter/prerender.js:888) lets JS-originated Kit fetches continue through Kit and relays resulting internal cookies to Go. The [Go service operation dispatch](/Users/tyler/.codex/worktrees/e413/skgo/prerender_service.go:184) handles load/remote/endpoint/resolve-option/cookie operations; it does not expose the native-development fetch exchange. Native development's continuation is an example of the capability, not proof the build service already offers it.

Question resolved: an appropriate Kit event exists in the worker, but connecting a Go-originated fetch to that event is not presently provided by the build callback protocol. Whether a bounded continuation integration is practical is implementation work; **explicit unavailable-capability refusal is already authorized**, so it is not necessary to introduce a broad bridge architecture to resolve the issue. **#272 is executable now; no HITL design decision is missing.** The chosen implementation must prove real-build known-destination behavior (correct literal output or explicit refusal), compare against the actually served route, distinguish genuinely missing destinations, and preserve endpoint fetch/isolation. A bare404, success status alone, template inspection or native-dev success does not establish the build contract.
