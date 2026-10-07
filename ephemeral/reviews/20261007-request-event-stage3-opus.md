# Adversarial review — RequestEvent stage 3 (request metadata)

Reviewer: Claude Opus 5.5, 2026-10-07. Read-only except this file.

## Target

Uncommitted stage-3 work over HEAD `06a6fd6` on `codex/request-event-plan`:
`document.go`, `document_action.go`, `event.go`, `fetch.go`, `handle.go`,
`internal/adapter/skgo-adapter/entry.js`, `internal/ssr/ssr.go`, `loadevent.go`,
`middleware.go`, `middleware_test.go`, `remote.go`, `remote_live.go`,
`request_event.go`, and new `request_metadata.go` / `request_metadata_test.go`.

Spec: `ephemeral/plans/20261006-request-event.md` stage 3 ("Route ID,
logical/original URL, request-kind flags and client address behave consistently
across the allowed request contexts, including actions and internal fetch. Query
restrictions remain intact. Handler fixtures demonstrate the distinctions and
address inheritance."), §4 "Common request information", and the "Metadata is
consistent" / "Restrictions still protect query caches" rows of the definition
of done. Repository rules from `AGENTS.md`/`CLAUDE.md`.

Scope note: the launch prompt's statement that stages 1–2 are accepted and its
invitation to reuse reported broad results were treated as context, not as a
limit. The whole stage-3 diff, including its edits to stage-1/2 code paths
(`request_event.go`, `remote_live.go`, `remote.go`), was reviewed.

## Kit mapping (installed pin, `@sveltejs/kit` 3.0.0)

- `src/runtime/server/respond.js:93-243`: `is_data_request` from the
  `__data.json` suffix; data URL strips the suffix, honours
  `x-sveltekit-trailing-slash=1`, deletes `x-sveltekit-invalidated`; remote URL
  is rewritten from `x-sveltekit-pathname`/`x-sveltekit-search`, and without
  them route resolution is skipped and `event.url` stays the endpoint URL;
  `isSubRequest: state.depth > 0`; `isRemoteRequest: !!remote_id`;
  `route: { id: null }` until matched (`:378`).
- `src/runtime/app/server/remote/shared.js:81-125`
  (`derive_remote_function_event`): query derivation hides only `url`, `params`,
  `route` and blocks cookie writes/`setHeaders`; flags, `request`, `locals`,
  `getClientAddress` survive.
- `src/runtime/server/state.js:31,53`: subrequest forks keep
  `getClientAddress` and increment `depth`.
- `src/runtime/server/fetch.js:23-53`: `event.fetch` resolves relative input
  and its same-origin test against `event.url`.
- `src/runtime/app/server/remote/form.js:140`: a native (`?/remote=`) form
  submission is not a remote request.
- JS consumers of the flags inside the renderer: `render.js:129` and
  `paths/server.js:49` (`isDataRequest`), `page/index.js:43` (`depth`),
  `remote/{query,form,prerender}.js` (`isRemoteRequest`). None are reachable
  with a data/remote request in skgo's renderer, so the renderer flags are
  observational today.

The implementation agrees with each of these. Specifically: hook metadata is
computed once at the boundary and carried by pointer through loads, actions,
endpoints, remotes and SSR; queries keep the flags and `Request()` while
`URL`/`RouteID`/`SearchParam`/`Params`/`Param`/`RemoteCallerValues` still panic
on both the current event and `EventFrom(e.Request().Context())`; the
`middleware_test.go` change (`!got.remote`) matches Kit, where a query keeps
`isRemoteRequest`; the client-address provider is a once-only lazy state
captured over the original request, carried across the valueless internal-fetch
context and reused by the child boundary (`withClientAddress` keeps an existing
state); the renderer reads it through a synchronous host callback and throws the
provider's error rather than fabricating `127.0.0.1`.

## Checks actually run

- `go test -count=1 . ./internal/ssr/` — both `ok` (2.3s, 0.9s).
- `go vet . ./internal/ssr/` — clean. `git diff --check` — clean.
- Focused stage-3 tests
  (`-run 'Metadata|ClientAddress|NestedSSR|RemoteFunctionBodies'`) — `ok`.
- Mutation probes in a throwaway copy (`/tmp/skgo-mut`, worktree untouched),
  each run against the focused tests:

  | Mutation | Result |
  | --- | --- |
  | Drop client-address carry across internal fetch (`fetch.go`) | FAIL (caught) |
  | Remove `sync.Once` memoization | FAIL |
  | Drop `newEvent` request-context rebinding (`remote.go`) | FAIL |
  | Drop `immutable()` request-context rebinding | FAIL |
  | Keep command context helper mutable (`request_event.go`) | FAIL |
  | Drop context-helper request rebinding | FAIL |
  | Live query binds `r.WithContext(ctx)` instead of `liveCtx` | FAIL |
  | Drop classic-action metadata | FAIL |
  | Drop load-event metadata | FAIL |
  | Hardcode renderer `IsSubRequest: true` | FAIL |
  | Drop renderer `ClientAddress` host | FAIL |
  | Remote flag fallback forced false without a boundary | survives (low-level path only; boundary is mandatory) |
  | `requestMetadataOf` always returns boundary metadata | survives (only differs for graph-less `HandleConfig{}` fixtures) |
  | Skip `net.ParseIP` validation after `SplitHostPort` | survives (cosmetic) |
  | Remove `fetchURL`'s new `metadata.url` preference | **survives the entire root suite** — see finding 1 |

- Served behaviour: built the example binary (`go build ./example/cmd`, whose
  embedded `example/web/build/ssr/bundle.js` already contains the new
  `__skgo_client_address` bridge), served it on `127.0.0.1:18431`, and fetched
  `/`, `/request-fetch`, `/fetch`, `/middleware`, `/todos`, `/live`,
  `/nested-universal`, `/actions`, `/docs`, `/spa`, `/__data.json`,
  `/request-fetch/__data.json`. All 200 except the non-existent `/async-ssr`
  (404). Rendered text of `/middleware` ("Go middleware authenticated this
  visit… visit-token-7 /middleware") and `/request-fetch` ("harbour-lamp-4096
  Fetched as guest") was read: real content, no error boundary or stack trace,
  no server log errors. The example never calls `getClientAddress`, so the
  removal of the renderer's `127.0.0.1` fallback did not break any served page.
- Not rerun: `just test`, example integration, `just e2e`. Sol's reported
  passing results were not independently reproduced; stage 3 changes no
  client-observable behaviour that would need the browser suite.

## Findings

### Blockers

None.

### Nonblocking concrete issues

1. **issue — unproven behaviour change in `Event.fetchURL`** (`fetch.go:160-164`).
   `fetchURL` now prefers `e.metadata.url` over the hook/load/transport URL.
   Its only observable effect is on events that previously fell through to
   `e.req.URL`: a remote command/form called with caller headers now resolves a
   relative `Fetch` (and its same-origin decision) against the caller page
   (`/items/42` → `fetch("data")` hits `/items/data`) instead of the remote
   endpoint (`/_app/remote/<hash>/data`), and a render-time nested query uses
   the logical page URL. This matches Kit (`fetch.js:23,35` use `event.url`,
   which `respond.js:166-174` rewrote to the caller page), so the direction is
   right, but deleting the four added lines leaves the whole root suite green.
   Reproduction: remove `fetch.go:160-163` and run `go test -count=1 .` — `ok`.
   Add a served command fixture with caller headers that fetches a relative
   path and asserts which endpoint answered.

2. **nitpick — the real renderer's flag plumbing is asserted only through
   substitute bundles.** `entry.js:65-68,171-185` now feeds
   `isDataRequest`/`isSubRequest`/`isRemoteRequest`, `depth` and
   `getClientAddress` from the request JSON and host callback, but every Go test
   of this path (`TestNestedSSRQueryKeepsPageRequestFlags`,
   `TestSSRClientAddressHostIsLazySharedAndPreservesErrors`) uses a hand-written
   bundle, and only `is_sub_request=false` is ever observed. A regression in
   `entry.js` itself (for example reverting `getClientAddress` to a constant)
   would stay green. No Kit code reachable in skgo's renderer currently branches
   on these values (see mapping), so the impact is limited to an app that reads
   `getRequestEvent()` during SSR.

3. **nitpick — `devRefresh` failure bypasses the binding** (`middleware.go`,
   the `cfg.Loads.devRefresh` branch before `withClientAddress`). The
   downstream handler that reports a dev graph-refresh failure receives a
   request with no client-address state and no boundary metadata, so it falls
   back to per-call peer resolution. This is dev-only and an error response, so
   it has no user impact today.

4. **nitpick — comment grammar** at `event.go` `hook` field: "Remote
   function's caller capabilities stay narrower…" should be "A remote
   function's…".

No over-engineering found: no migration machinery, compatibility shims or
additional public API beyond the planned `HandleConfig.ClientAddress` and
`Event.ClientAddress`.

## Outcome

material findings remain — one nonblocking issue (finding 1: a Kit-correct
behaviour change with no load-bearing test) and three nitpicks. Nothing blocks
stage 3. Under the user's policy, file finding 1 as a follow-up and continue to
stage 4.
