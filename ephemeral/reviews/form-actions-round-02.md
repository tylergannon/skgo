# Adversarial review: form-actions proof design, round 02

Date: 2026-09-24 (local). Reviewer: Claude (Fable 5.1), read-only.

## Target

`ephemeral/plans/form-actions.md` on branch `codex/form-actions-proof`, working
tree over `af56746` (uncommitted revision, 262 lines, incorporating the
round-01 responses: the "Save without a result" row, project-level browser
selection, the upstream test citations at revision
`090d77ff289bf844cf65403f8f5e7eec8a7bb902`, the stage-1 devalue/transport
channel, and the per-visitor workspace cookie). Still a design review before
implementation; no implementation diff exists.

Scope note. The caller's constraints (read-only apart from this artifact, the
artifact path, "same authoritative requirements and sources") are operating
constraints and were honoured. The authoritative user context from round 01
(upgrade `./example` to showcase the feature with unambiguous completion;
reuse existing devalue support and send genuine gaps upstream to polytype) was
treated as requirements to check against. No narrowing of subject matter was
requested, so none was ignored. Round 01 is at
`ephemeral/reviews/form-actions-round-01.md` and was not modified.

## Evidence inspected

Repository instructions: `CLAUDE.md` (kit is the specification; mirror kit's
rules; delegate missions not methods; load-bearing Gherkin scenarios; "never
derive what you expect from the thing you are testing"; no "done" command),
`ephemeral/sveltekit-current/SKILL.md`, worklog
`ephemeral/worklog/202609241333-form-actions-proof.md`, round-01 review.

Pinned kit `reference/kit@3.0.0-next.27/src`: `runtime/server/respond.js:640-
690` (endpoint-versus-page dispatch, "prefer the page if the endpoint can't
handle this POST", OPTIONS `allow` gains POST when the leaf has `actions`),
`runtime/server/endpoint.js:99-113` (`is_endpoint_request`),
`runtime/server/page/index.js:48-80` (JSON action requests, `get_remote_action`
consulted before `handle_action_request`, status from error/failure) and
`:108-150` (ssr=false: shell rendered with the action's status, DEV warnings only
without `x-sveltekit-action`), `runtime/server/page/actions.js:207-245`
(no mixing default with named, `default` reserved, `Object.hasOwn` lookup, 415),
`runtime/server/remote-functions.js:609-611` (`/remote` query key),
`runtime/client/client.js:3005-3030` (`applyAction` sets `page.status`).

Upstream tests at the plan's cited revision, fetched read-only into the session
scratchpad with `gh api` and checked against the plan's claims:
`repos/sveltejs/kit/git/ref/tags/@sveltejs/kit@3.0.0-next.27` resolves to
`090d77ff289bf844cf65403f8f5e7eec8a7bb902` (a commit object, "Version Packages
(next) (#16943)"), so the pin is correct. `packages/kit/test/apps/basics/test/
test.js` Actions section (file-without-enctype DEV error, error props, persisted
fields, success as form-data/urlencoded, `applyAction` prop/redirect/error,
"form prop stays after refresh and is reset on navigation", `use:enhance`
variants incl. non-ActionResult error responses, button name/formaction/
formenctype, redirect and redirect-in-handle, page.status on error, error level,
415, 404, cross-page success/failure/redirect/error, Object.prototype names
`toString`/`constructor`/`__proto__`/`hasOwnProperty` → 404);
`client.test.js` Actions section (client-side cross-page navigation, dropped
and stripped query params, refresh defaults, history entry with ephemeral form
data on forward, `navigate` option, `refreshAll: false`); `server.test.js`
(`OPTIONS` allow `GET, HEAD, OPTIONS, POST` at :867-872, fallback to page
actions when the sibling endpoint has no POST at :899, undefined success at
:932, stripped landing location at :949, matching `fail()` status at :970,
CSRF allowances at :175 and :201). Fixture
`src/routes/actions/update-form/+page.svelte` (drives `applyAction` and
`refreshAll` with hand-built results, no server action involved). The `no-ssr`
and `no-csr` test apps' route listings contain no action routes, as the plan
states.

skgo: `document.go:474-505` (POST dispatch: `?/remote` first, else 405 with
`Allow: GET`; `runFormAction`, `jarOf`), `endpoint.go:41-43, 613-665`
(`pageMethods`, page-preferred fallback and `isEndpointRequest` already
mirrored), `event.go:81-139` and `loadevent.go:165` (cookie read/set on
events and loads, so a workspace cookie set on first GET is possible in Go),
`internal/ssr/ssr.go:45-70, 137-144` (`Form any` JSON slot; `Form.Output`
already carries devalue flat form with transport for remote forms),
`internal/gen/` (loads, clients, transport emitters; no action emitter yet),
`example/web/src/routes/account/+page.server.ts` (generated stub shape),
`example/web/src/hooks.go:17` (`Transported[businesslogic.Money]("Money")`),
`example/businesslogic/store.go:49-70`.

e2e harness: `example/e2e/playwright.config.ts:41-52` (project selection by
filename), `steps/fixtures.ts:370-395` (`hydrated()`), `steps/form.ts:259-300`
(noscript counterparts, document-POST observation), `global-setup.ts`. No
Playwright install exists on this machine outside the browser cache, so the
rendering of `<noscript>` content under Chromium's script-execution emulation
was not run here; it is the plan's chosen positive marker and the builder
should observe it in the first noscript screenshot.

Round-01 findings, status in the current plan: 1 (no-data success on native
paths) resolved at :51; 2 (browser path selected by project) resolved at
:89-116; 3 (upstream tests as the source) resolved at :240-257 and verified
above; 4 (form seam in stage 1) resolved at :204-210; 5 (state isolation)
resolved at :39-46 with a mechanism skgo's API supports.

## Findings

### 1. issue — Two edge rows test kit's client rather than skgo, so they are not load-bearing for the feature and one of them is ambiguous

`form-actions.md:140-141` ("Normal enhancement behavior remains available" and
"A returned result has Kit's lifetime and destination") port kit's
`client.test.js` Actions section and the `applyAction`/`refreshAll` tests from
`test.js`. Those upstream tests exercise kit's client with hand-built results:
the `update-form` fixture calls `applyAction({type, status: 200, data:
{count}, location})` from a button and then `refreshAll()`, and "form prop
stays after refresh and is reset on navigation" (`test.js:1199-1225`) asserts
what the client does with a value it invented itself. Reset/refresh opt-outs,
forward-history ephemerality and `navigate: false` are decided entirely in
`runtime/app/forms/client.js` and `runtime/client/client.js`, which the plan
itself says "remains Kit's" (:252). A skgo defect cannot make them fail except
through the result fields the cross-page row (:129) already covers: `type`,
`status`, `data` and the stripped `location`.

`CLAUDE.md` requires each scenario to be one that "would actually fail if the
feature were broken or removed", and the validator step at :179 is told to
check exactly that; these rows would be struck by that check. The row at :141
is also ambiguous in a way that matters: "A receipt survives refresh" reads as
a browser reload, but kit's test means `refreshAll()`, and a reload after an
enhanced submission is a fresh GET that kit answers with `form: null`, which
the plan states at :73-74 and forbids papering over at :74-75. A builder who
reads "refresh" as reload can only satisfy it with a load-backed receipt.

Impact: acceptance rows that cannot fail on skgo, a contradiction inside the
contract, and a larger matrix for the validator to open screenshots for.
Reduce :140-141 to the server-dependent claims (cross-page enhancement
navigates client-side to the stripped location and preserves the unrelated
query parameter; redirects still navigate; a failure result carries the 422 and
its data) and fold them into :129, or state per row which skgo-produced field
each observation depends on. Rename "refresh" to "a data refresh via
`refreshAll`" if it stays.

### 2. issue — Stage 1 builds a default-action editor that kit's no-mixing rule forces stage 3 to rebuild

Stage 1 (:204-210) opens "a seeded profile editor whose default Go page action
saves". The design (:29-30) says the main editor "demonstrates named actions
and different submit buttons" and that the default action lives on "its own
page under `/actions`". Kit forbids the migration path between those two
states: `actions.js:207-212` throws "When using named actions, the default
action cannot be used" the moment a named action is added beside `default`.
So the stage-1 editor, its Gherkin, its Go declaration and its generated types
are all rewritten in stage 3 when Save and Archive become named actions, which
is the same "temporary channel to replace later" the revised stage 1 says it
avoids, one level up.

Impact: stage-1 proof is of a page shape that will not exist at the end, and
the stage-3 rewrite re-proves 12 of the 36 journeys. Make stage 1 the editor
with one named action (`?/save`) and a single button, which is kit-legal to
extend with `?/archive` later, and put the default-action page in stage 3
alongside "A default action works without a name" (:125).

### 3. nitpick — `remote` is an effectively reserved action name and the interface checks do not say so

`page/index.js:61-69` consults `get_remote_action(event.url)` (the `/remote`
query key, `remote-functions.js:609-611`) before `handle_action_request`, and
skgo's `document.go:478-479` does the same, so a Go action named `remote` is
unreachable: `?/remote=<anything>` is dispatched as a remote-form id. Kit
itself does not reject the declaration, so skgo's generator need not either,
but the interface check at :151-156 lists "reserved action names" with only
`default` behind it. Name `remote` as a second reserved name in the checks so
the collision is at least a test, given the example keeps its remote-form demo
on the same site (:11-12).

### 4. nitpick — The CSRF check is stated one-sided

`:153-154` says "cross-origin form rejection". The upstream server tests the
plan cites for CSRF (`server.test.js:175` "Allows non-form content types
regardless of origin", `:201` "Handles undefined origin correctly") are the
allow cases; kit's `csrf.js` rejects only form content types with a mismatched
origin. A check that only proves rejection can pass on an implementation that
rejects every cross-origin POST, including the JSON ones kit lets through.
State both directions.

## Outcome

material findings remain
