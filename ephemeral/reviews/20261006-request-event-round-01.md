# Adversarial review — RequestEvent plan, round 01

**Target:** `ephemeral/plans/20261006-request-event.md` (untracked, worktree `bad1`, baseline `916ccdc`).
**Kind:** design plan; implementation deferred until #262 lands. No code under review.
**Operating constraints honoured:** read-only except this artifact. No caller narrowing was
detected or ignored; the caller's restatement of the user's wants (application-owned typed
`Locals`, hook pointer assignment, layout params on the #261 sealed-union machinery, preserved
`RequestEvent` semantics) was treated as requirement, not as a predicted conclusion.

## Evidence inspected

- Repository instructions: `CLAUDE.md`, `agent-protocol` skill.
- The plan in full, its worklog `ephemeral/worklog/20261006-request-event-plan.md`, and the
  draft/live text of issue #262 (`gh issue view 262`).
- Pinned Kit 3.0.0 (`example/web/node_modules/@sveltejs/kit`, version verified in `package.json`):
  `types/index.d.ts` (`RequestEvent` 576–695, `ServerLoadEvent` 745+), `src/types/ambient.d.ts`
  (`App.Locals`), `src/runtime/server/respond.js` (event construction 186–245, remote URL
  160–182, route match 363–381), `src/core/sync/write_types/index.js` (`LayoutRouteId`/
  `LayoutParams` 280–326, `generate_params_type` 600–623), `src/core/sync/create_manifest_data/index.js`
  (`child_pages` 290–466), `src/runtime/server/page/load_data.js` (tracking/untrack 25–180),
  `src/runtime/app/server/remote/shared.js` (`derive_remote_function_event` 81–134),
  `src/runtime/server/state.js` (depth, `getClientAddress`), `src/core/postbuild/prerender.js:396–406`,
  `src/exports/hooks/sequence.js`.
- Current skgo: `event.go`, `request_event.go`, `loadevent.go`, `handle.go`, `middleware.go`,
  `downstream.go`, `remote.go:1000–1030`, `remote_caller.go`, `fetch.go`, `document.go:1270–1340,
  1864`, `document_action.go:74–85`, `internal/ssr/ssr.go:75`, `internal/gen/{gen.go,
  load_params.go, shared_params.go, scan.go}`, generated outputs under `example/`, the example's
  hook (`example/server.go`) and `example/web/src/{hooks.go,params.go}`, the scaffold
  (`cmd/skgo/main.go:142+`, `internal/newapp/gofiles/**`), `Justfile:56–57`.

Checks that passed (not findings): the plan's baseline statements are accurate — flags are
hook-only (`middleware.go:88–96`), classic actions carry no URL/route (`document_action.go:77`),
SSR already bridges a client address from `RemoteAddr` (`document.go:1281,1864`), locals are a
type-keyed store (`handle.go:181–218`). Shared `params.Params` already covers endpoint-only and
frontend-only routes (`load_params.go:142–146`; `/api/replay/[kind]` is in the generated switch),
so the hook's typed `Params` has a constructor for every matched route. The plan's reading of Kit's
layout membership (`child_pages`, resets, groups, endpoints not executing layouts), per-load
tracking, `isRemoteRequest = !!remote_id`, `isSubRequest = depth > 0`, fresh locals per internal
fetch, prerender address failure, and query restrictions all match the pinned source.

## Findings

### 1. Issue — the public surface the hook needs is not specified, though "Plan complete" requires it

The plan's own gate (§7 "Public type shape … explicit") is not met for the hook path.

- §2 shows `func handle(ctx, event *params.RequestEvent, resolve skgo.Resolve)` but never names
  the library type such a function is. Today `Middleware` is
  `func(ctx, *Event, Resolve)` (`handle.go:38`), `Sequence` threads one `*Event` (`handle.go:139–167`),
  and `Intercept` is a method on `Middleware`/`Handle` (`middleware.go:120`, `handle.go:245`). A
  typed hook needs a generic `Middleware[P, L]`/`Sequence`/`Intercept` or a generated wrapper; the
  plan does not say which, nor what becomes of `Handle func(ctx) error`, nor what
  `skgo.EventFrom(ctx) *Event` returns relative to the typed event.
- The library may not import the application's `app` or generated `params` (§1), so the typed hook
  event must be constructed either by generated code (`params.SkgoRequestEvent` as today,
  `shared_params.go:423`) or through a constructor handed into `Intercept`. This decides whether
  `HandleConfig`/`Intercept` keep their shape; it is a public-type decision, not a method.
- `ClientAddress()` is to come from "a configurable provider at the existing request boundary".
  There are five boundaries (`Handle.Intercept`, `Loads`, `Remotes`, `Endpoints`, SSR), and the
  scaffold mounts no hook at all (`internal/newapp/gofiles/server.go.tmpl:96` —
  `loads.Intercept(remotes.Intercept(endpoints.Intercept(pageHandler)))`). The plan's "No-hook path
  supplies an empty object" implies the same question for locals: which config owns the per-request
  slot when `Intercept` is absent? Unnamed today.
- §1 says the application package and `Locals` type are "selected through generation
  configuration". `gen.Config` (`internal/gen/gen.go:32–60`) and the scaffold's
  `//go:generate go tool skgo generate --web ../../web` have no such field or flag.

Impact: an implementer invents these, and the consensus loop on the plan cannot evaluate them.
Fix: add the library-side spellings (hook type, composition, no-hook boundary, address-provider
owner, generate flag) to §1/§2 at the same level of concreteness as `RequestEvent[P, L]`.

### 2. Issue — pointer replacement is an skgo extension Kit forbids, and the plan neither says so nor fixes the binding instant

Kit's spec: `readonly locals: App.Locals` (`types/index.d.ts:603`), one `locals: {}` per request
(`respond.js:202`), handlers in a `sequence` "pass data between handlers" by mutating it
(`sequence.js:78`). The object is never replaced. The user has asked for replacement, so this is
not a request to remove it, but CLAUDE.md requires the plan to map Kit first and state deviations
as deviations. §2 instead presents replacement as the design and then legislates behaviour Kit
never needs:

- Three different binding moments are stated: "The pointer is selected during hook
  initialization", "When the chain dispatches to application handlers, bind the selected pointer",
  and "A replacement in an outer hook is immediately visible to an inner hook". Only the `resolve`
  call corresponds to anything in Kit (`resolve(event)` is where the event goes downstream).
- "Replacing a hook's field after dispatch does not retroactively replace downstream events'
  locals" makes a late `event.Locals = x` a silent no-op — the shape of bug CLAUDE.md warns
  about (a check that passes because nothing happened).
- "Explicit nil assignment is rejected before dispatch with a diagnostic" invents a runtime
  failure with no Kit analogue and no stated status/body for the client.

Fix: state explicitly that Kit's `locals` is readonly and this is an skgo-only affordance; name
one binding instant (the `resolve` call); make a post-resolve write either an error from the
request or a documented, tested no-op — not both by omission; say what the nil case returns.

### 3. Issue — migration and back-compat deliverables contradict #262 and "Build the software"

Issue #262 (live text): "The only existing application is our small example app. Update it in
the same change and delete its old generated outputs directly. No general migration mechanism or
backward compatibility … is required." The plan nevertheless carries: §1 "existing apps receive a
migration diagnostic rather than an invented import path"; §2 "diagnostics must name the
replacement"; §6.1 "migration of existing consumers"; DoD row 1 "Old signatures/configuration
fail usefully"; §6.4 "Migration … part of the deliverable"; and DoD row "Generation survives
evolution — missing/broken prior outputs …", which restates #262's own acceptance. The real
consumers are the example, `internal/gen/testdata`, and `internal/newapp`, all updated in-tree.

Impact: engineering spent on diagnostics for users that do not exist, and a duplicated acceptance
row that will be "verified" twice or skipped once. Fix: replace migration rows with "example,
testdata fixtures and starter updated in the same change"; reference #262's acceptance instead of
restating it.

### 4. Issue — two definition-of-done rows cannot be satisfied as written

- "An SSR-rendered locals value survives hydration." Locals never leave the server (Kit serializes
  load data, never `event.locals`); the thing that hydrates is data a load derived from locals,
  which existing scenarios already cover. As written the row asserts a non-event.
- "Race-sensitive fixtures run under the race detector." `just test` is
  `go test -count=1 ./... ./example/...` (`Justfile:56–57`) — no `-race`. CLAUDE.md forbids a second
  command beside the real one, so either `just test` adopts `-race` project-wide (a cost the plan
  should own) or the row is unfulfillable and becomes a skip that reports green.

Fix: rewrite the first as a load-data claim or delete it; decide the `-race` question in the plan.

### 5. Nitpicks

- §1's rule "A hook that imports shared params lives outside `app`" is necessary but not
  sufficient: generated route params import the matcher package
  (`example/web/src/routes/account/skgo_params_gen.go:8` imports `hooks`), and the example's
  matcher package is `example/web/src` — the natural home of a Go `handle`. A typed hook there
  cycles (`hooks → params → hooks`). State "outside `app` and outside any matcher package".
- Kit dedupes a layout's descendant params by name, first matcher wins
  (`write_types/index.js:296–299`); the plan's sealed union for multi-type keys is deliberately
  stricter than the pin. Say so, so a later reviewer does not read it as a mirror error.
- §3 "Migrate the existing ambiguous route-local `RequestEvent` alias": `scan.go:528–531` requires
  the load parameter to be *identical* to the package-scope `RequestEvent`; the plan should note
  the scan rule becomes file-kind-keyed (`scan.go:346` already keys on `page.server.go` vs
  `layout.server.go`).
- §4 `ClientAddress` "inherited by internal subrequests" is right for Kit (`state` spread in
  `fetch.js`) but `fetch.go:439` copies `RemoteAddr` onto the subrequest today; the plan should say
  whether the provider runs again on the subrequest (and could disagree) or the value is carried.

## Outcome

`material findings remain`
