# Group 3: Consistent production error hooks

## #354 Remote function errors skip HandleError in production

https://github.com/tylergannon/skgo/issues/354

At v0.36.0 (`3b382953`, which is also current `main`), errors from remote functions skip the application's `HandleError` in production but go through it in development.

**Kit 3.1.0:** `runtime/server/remote-functions.js` sends remote-function errors through `handle_error_and_jsonify` (lines 120, 336 and 370), so the app's `handleError` shapes them.

**skgo production:** `RemoteConfig` has no `HandleError` field ([remote.go:474-515](https://github.com/tylergannon/skgo/blob/3b382953fb97f77aecd1f5122d76dfbe586fec89/remote.go#L474-L515)). Each call site writes `asHTTPError(err)` directly ([remote.go:808](https://github.com/tylergannon/skgo/blob/3b382953fb97f77aecd1f5122d76dfbe586fec89/remote.go#L808), [842](https://github.com/tylergannon/skgo/blob/3b382953fb97f77aecd1f5122d76dfbe586fec89/remote.go#L842), [890](https://github.com/tylergannon/skgo/blob/3b382953fb97f77aecd1f5122d76dfbe586fec89/remote.go#L890), then `writeError` at [1061-1071](https://github.com/tylergannon/skgo/blob/3b382953fb97f77aecd1f5122d76dfbe586fec89/remote.go#L1061-L1071)), and a panic becomes a bare `{500, "Internal Error"}` after `OnPanic` ([remote.go:1000-1010](https://github.com/tylergannon/skgo/blob/3b382953fb97f77aecd1f5122d76dfbe586fec89/remote.go#L1000-L1010)). An unknown error therefore never reaches the hook, and the client's `error` carries no fields the hook would add.

**skgo development:** the native remote bridge calls `d.renderer.loads.dataError(...)`, which is `handleErrorAndJSONify` with the app's hook ([dev_remote.go:41-51](https://github.com/tylergannon/skgo/blob/3b382953fb97f77aecd1f5122d76dfbe586fec89/dev_remote.go#L41-L51)).

**Expected:** production remote functions run the same HandleError the loads and documents use, for both returned errors and panics, matching Kit and `skgo dev`. Add a test that checks a hook-supplied field in a remote error envelope in production.

The HandleError godoc should be corrected at the same time; that is filed in the godoc issue.

🤖 Generated with [Claude Code](https://claude.com/claude-code)


<!-- agent-session-group-20261018 -->
## Coordinated agent session: Production application error hooks

Work together: #354, #353.

Production remote-function errors and SSR component-boundary errors reach the application's configured Go HandleError and expose the intended public fields, consistently with native development and pinned Kit. Both issues concern missing production entry points into the same application error policy; retain distinct remote and boundary contracts, including private-detail protection and known-error/redirect behavior.

Use the installed pinned Kit implementation as the specification. The session is complete only when both issues' retained acceptance criteria are satisfied; partial delivery does not close the other issue.

### Comment by tylergannon at 2026-10-18T16:57:41Z

**Triage — 2026-10-18**

Checked against current [`main` at `920b6aca`](https://github.com/tylergannon/skgo/tree/920b6aca738a65d6c046a67f4db2373596fafd03). Classification: `bounded-hard`.

The production RemoteConfig/error-writing path still lacks the HandleError integration used by the native development bridge. Clear endpoint: returned unexpected errors and panics produce the configured public error shape in production, without leaking private details or changing known HTTP/redirect handling. Multiple remote kinds and shared error semantics make this larger-model work.

<!-- skgo-issue-triage-20261018-920b6aca -->
## #353 Svelte boundary errors skip HandleError in production SSR

https://github.com/tylergannon/skgo/issues/353

At v0.36.0 (`3b382953`, which is also current `main`), an error caught by a `<svelte:boundary>` during production SSR never reaches the application's `HandleError`. In `skgo dev` it does, so the two modes differ.

**Production (goja entry):** [internal/adapter/skgo-adapter/entry.js:43-56](https://github.com/tylergannon/skgo/blob/3b382953fb97f77aecd1f5122d76dfbe586fec89/internal/adapter/skgo-adapter/entry.js#L43-L56) defines

> kit's handle_error_and_jsonify (runtime/server/errors.js) with no handleError hook … skgo has no handleError hook, so there is nothing here to await or merge.

and the render options' `transformError` ([entry.js:441-449](https://github.com/tylergannon/skgo/blob/3b382953fb97f77aecd1f5122d76dfbe586fec89/internal/adapter/skgo-adapter/entry.js#L441-L449)), which Kit sends every boundary's error through, calls that hookless `handle_error`. Kit 3.1.0's own `transformError` (`runtime/server/page/render.js`) calls `handle_error_and_jsonify(event, render_state, e)`, which runs the app's `handleError`.

The comment is out of date: skgo does have a Go handleError hook, `HandleError` ([handle_error.go:51-78](https://github.com/tylergannon/skgo/blob/3b382953fb97f77aecd1f5122d76dfbe586fec89/handle_error.go#L51-L78)), and Go calls it for load and render errors on the document path (#59).

**Development:** the generated `hooks.server` module exports `handleError = input => handleGoError(input)` ([internal/adapter/skgo-adapter/dev-modules.js:52](https://github.com/tylergannon/skgo/blob/3b382953fb97f77aecd1f5122d76dfbe586fec89/internal/adapter/skgo-adapter/dev-modules.js#L52), [dev-bridge.js:234](https://github.com/tylergannon/skgo/blob/3b382953fb97f77aecd1f5122d76dfbe586fec89/internal/adapter/skgo-adapter/dev-bridge.js#L234)). Kit's real renderer then runs Go's hook for boundary errors.

**Effect:** in production, a component that throws inside a boundary shows `{status: 500, message: "Internal Error"}` in the failed snippet with none of the fields the app's HandleError would add, such as an error id. Nothing is logged through the hook either. In development the same page shows the hook's fields.

**Expected:** `transformError` in the goja entry calls into Go's HandleError (the same `handleErrorAndJSONify` the document path uses) and keeps the HttpError/SvelteKitError/redirect handling. Add a production test in which a boundary error shows a field that only the app's hook supplies, and confirm the same result in `skgo dev`.

🤖 Generated with [Claude Code](https://claude.com/claude-code)


<!-- agent-session-group-20261018 -->
## Coordinated agent session: Production application error hooks

Work together: #354, #353.

Production remote-function errors and SSR component-boundary errors reach the application's configured Go HandleError and expose the intended public fields, consistently with native development and pinned Kit. Both issues concern missing production entry points into the same application error policy; retain distinct remote and boundary contracts, including private-detail protection and known-error/redirect behavior.

Use the installed pinned Kit implementation as the specification. The session is complete only when both issues' retained acceptance criteria are satisfied; partial delivery does not close the other issue.

### Comment by tylergannon at 2026-10-18T16:57:30Z

**Triage — 2026-10-18**

Checked against current [`main` at `920b6aca`](https://github.com/tylergannon/skgo/tree/920b6aca738a65d6c046a67f4db2373596fafd03). Classification: `bounded-hard`.

The production entry.js boundary transformError path still uses its local hookless error conversion, while native development delegates to Go. Clear endpoint: a production boundary shows a field supplied only by the configured Go HandleError, with existing known-error/redirect semantics preserved. This crosses the embedded-renderer callback boundary and needs focused production proof.

<!-- skgo-issue-triage-20261018-920b6aca -->

## Initial orchestration assessment

Goal: production remotes and SSR component boundaries use the configured Go application error policy for unexpected errors/panics, exposing hook-supplied public fields while protecting private details and preserving pinned Kit's known-error/redirect behavior.

Proof: distinct literal handler contracts for production remote envelopes and actual goja-rendered boundary output, including hook-only field and private sentinel absence alongside positive expected content. Match boundary behavior in native dev with focused actual runtime proof. Browser checks only where client behavior is required. No redesign/shared policy framework.

Potential design questions: does pinned Kit settle when the hook runs/bypasses HttpError, SvelteKitError and redirect, and which event/status/message fields reach it for each remote kind/boundary? Map current source before implementation. These appear to be specification mapping, not a missing HITL product choice.

## Token-cache annotation: #354 executable

Research entered through the [cache index](/Users/tyler/src/skgo-token-cache/index/README.md) and its [hooks/errors route](/Users/tyler/src/skgo-token-cache/index/upstream-docs/topics.md:59). Cached [hook documentation](/Users/tyler/src/skgo-token-cache/repos/dev-experience-20261015/frameworks/kit-3.0.0-upstream/documentation/docs/30-advanced/20-hooks.md:148) explains safe public representations, partial overrides and the app/framework/validation/unknown kinds. That older source is corroborating context; current authority is installed [Kit 3.1.0](/Users/tyler/.codex/worktrees/e413/skgo/example/web/node_modules/@sveltejs/kit/package.json:3).

- **Known errors are not generally exempt from the hook.** Installed [handle_error_and_jsonify](/Users/tyler/.codex/worktrees/e413/skgo/example/web/node_modules/@sveltejs/kit/src/runtime/server/errors.js:21) bypasses only `HandledHttpError` (already processed). Ordinary `HttpError` becomes `kind: 'app'` with its public body; `SvelteKitError` becomes `kind: 'framework'` with safe status/text; validation becomes `400 Bad Request`; unknown errors give the hook the original error but default to `500 Internal Error`. The hook runs with the request event and request store, and its returned fields merge over that safe fallback. A failed hook preserves fallback status and returns `Internal Error`.
- Installed [remote catch](/Users/tyler/.codex/worktrees/e413/skgo/example/web/node_modules/@sveltejs/kit/src/runtime/server/remote-functions.js:323) handles redirects before error conversion. Runtime failure responses use HTTP 200 with `{type:'error', error: transformed}` and private/no-store caching; prerender failure uses the transformed status. [Live-query errors](/Users/tyler/.codex/worktrees/e413/skgo/example/web/node_modules/@sveltejs/kit/src/runtime/server/remote-functions.js:114) and [collected remote-data errors](/Users/tyler/.codex/worktrees/e413/skgo/example/web/node_modules/@sveltejs/kit/src/runtime/server/remote-functions.js:362) use the same conversion. Redirects remain redirects, with no hook invocation.
- Current [Go error policy](/Users/tyler/.codex/worktrees/e413/skgo/handle_error.go:102) already provides handled-body bypass, safe unknown fallback, merge and hook-panic protection. Its documented Go `CaughtError` currently narrows classification to app/unknown; this issue is missing production hook entry points, not a requirement to redesign the classification API. The [native remote bridge](/Users/tyler/.codex/worktrees/e413/skgo/dev_remote.go:39) supplies the per-call event and delegates errors to that existing policy.
- Current [remote panic recovery](/Users/tyler/.codex/worktrees/e413/skgo/remote.go:993) has independent `OnPanic` reporting and preserves `http.ErrAbortHandler`, then reduces a panic to a public HTTPError. Preserve reporting/abort behavior while ensuring the error policy sees the unexpected nature of the panic. Private causes must not enter the envelope except through deliberately returned public hook fields.

Question resolved: pinned Kit and the existing Go policy settle invocation, redirect bypass and public-field semantics. **#354 is executable now; no HITL design decision is missing.** Distinct production remote envelopes must prove a hook-only literal field for returned errors and panics, positive safe fallback/known-error content, no private sentinel, and unchanged redirect behavior. Multiple remote kinds are existing surfaces of that goal, not permission for new error-policy architecture or broad unrelated conformance work.

## Token-cache annotation: #353 executable

- Following the [cached Svelte boundary route](/Users/tyler/src/skgo-token-cache/index/upstream-docs/svelte-runtime-reference.md:5), [Svelte's transformError documentation](/Users/tyler/src/skgo-token-cache/repos/dev-experience-20261015/frameworks/svelte-5.57.1/documentation/docs/05-special-elements/01-svelte-boundary.md:106) states that the transformed error is JSON-serialized for hydration of the failed snippet. This explains why a safe public field is part of actual rendered output and why private message/stack redaction matters; it does not prove a current SKGO callback exists.
- Current installed [Kit render transform](/Users/tyler/.codex/worktrees/e413/skgo/example/web/node_modules/@sveltejs/kit/src/runtime/server/page/render.js:187) creates a render-state request context, rethrows redirects, and calls the same `handle_error_and_jsonify` mapped above. It assigns the transformed body to page.error and its status to page.status before returning that body to the boundary. Ordinary HttpError and SvelteKitError still run through the hook; an already-handled body bypasses another invocation.
- Current production [entry transform](/Users/tyler/.codex/worktrees/e413/skgo/internal/adapter/skgo-adapter/entry.js:441) updates those page/result fields but calls the [hookless converter](/Users/tyler/.codex/worktrees/e413/skgo/internal/adapter/skgo-adapter/entry.js:43). The gap is the application error-policy callback during actual component rendering. Existing [Go document policy](/Users/tyler/.codex/worktrees/e413/skgo/handle_error.go:91) and [public error extra fields](/Users/tyler/.codex/worktrees/e413/skgo/internal/ssr/ssr.go:151) already supply the semantics to preserve; no new error framework is needed.
- **Native-development premise requires verification/correction.** Contrary to the issue's shorthand, current [handleGoError](/Users/tyler/.codex/worktrees/e413/skgo/internal/adapter/skgo-adapter/dev-bridge.js:234) returns a WeakMap body only for failures already handled in Go. For errors originating in native Kit/universal JavaScript it logs unknown/validation errors and returns nothing. A new component-thrown boundary error is not proven to reach Go's hook merely because Kit calls that function. The issue's requested native parity is still the desired proof; if the focused actual-runtime test exposes this gap, repairing that same callback path remains within the stated goal.
- The existing [render-result status test](/Users/tyler/.codex/worktrees/e413/skgo/internal/ssr/ssr_test.go:389) uses a handcrafted reporting bundle and establishes result transport. It does not establish that the real adapter/Svelte boundary calls the Go application hook. Keep production proof at the actual rendered document and configured hook, with positive failed-snippet content plus a hook-only field and private-sentinel protection. Hydration preservation matters if client behavior is asserted; no full browser suite is needed to prove only HTTP/rendered output.

Question resolved: pinned Kit supplies the behavior and error semantics. **#353 is executable now; no HITL product decision is missing.** Use the existing configured Go policy for the real boundary path, preserve redirect and already-handled semantics, and prove native-development parity instead of assuming the issue's description establishes it. The callback's representation and engine plumbing are implementation judgments bounded by that goal and the no-JavaScript-application-I/O rule.
