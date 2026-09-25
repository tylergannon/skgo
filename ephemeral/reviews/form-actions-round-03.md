# Adversarial review: form-actions proof design, round 03

Date: 2026-09-24 14:40 (local). Reviewer: Claude (Fable 5.1), read-only.

## Target

`ephemeral/plans/form-actions.md` on branch `codex/form-actions-proof`, working
tree over `af56746` (uncommitted revision, 273 lines). Compared with the round-02
target it removes the two client-only edge rows, adds "Classic actions and
remote forms coexist", makes stage 1 the named-action editor posting to
`?/save` with the default-action page moved to stage 3, states the remote-form
selector precedence as a dispatch rule rather than a generator ban, and states
the CSRF check in both directions. Design review before implementation; no
implementation diff exists.

Scope note. The caller's constraints (read-only apart from this artifact, the
artifact path, "same authoritative requirements and sources") are operating
constraints and were honoured. The round-01 authoritative user context
(upgrade `./example` to showcase the feature with unambiguous completion; reuse
existing devalue support and send genuine gaps upstream to polytype) was treated
as requirements. No narrowing of subject matter was requested, so none was
ignored. Rounds 01 and 02 were not modified.

## Evidence inspected

Repository instructions (`CLAUDE.md`, `ephemeral/sveltekit-current/SKILL.md`),
worklog `ephemeral/worklog/202609241333-form-actions-proof.md`, rounds 01 and 02,
and the full `git diff` of the plan against `af56746`.

Pinned kit `reference/kit@3.0.0-next.27/src`: `runtime/server/csrf.js:1-45`
(`is_csrf_forbidden`: missing content type counts as form; trusted origins
honoured), `runtime/server/respond.js:116-120` (page POSTs, including native
`?/remote` ones, go through `is_csrf_forbidden` with `csrf_trusted_origins`),
`core/config/options.js:95-100` (`csrf.trustedOrigins`), `runtime/server/page/
index.js:48-80` (JSON action request handled before the `/remote` check; native
`/remote` checked before classic actions), `runtime/server/page/actions.js:207-
245`, `runtime/server/respond.js:640-690`, `runtime/server/endpoint.js:99-113`,
`runtime/client/client.js:3005-3030, 3309`.

Upstream tests at `090d77ff289bf844cf65403f8f5e7eec8a7bb902` (fetched in round
02, re-read for the changed rows): `server.test.js:175, 201` (CSRF allow cases),
`:899` (fallback to page actions), `:867-872` (OPTIONS allow), `client.test.js`
cross-page cases (`:1608-1735`), `test.js` Actions section. The route listings
of the `no-ssr` and `no-csr` test apps contain no action routes.

skgo: `document.go:474-505`, `document_form.go:118-134` (remote-form CSRF
check: compares Origin against the app origin only), `endpoint.go:135-137,
595-606` (`TrustedOrigins` honoured on the remote endpoint), `event.go:81-139`,
`loadevent.go:165`, `internal/ssr/ssr.go:45-70, 137-144`, `example/web/src/
hooks.go:17`, `example/businesslogic/store.go:49-70`.

e2e harness: `example/e2e/playwright.config.ts:41-52`, `steps/fixtures.ts:370-
395`, `steps/form.ts:259-300`, `global-setup.ts`.

Empirical check of the plan's positive no-JS marker (`:92-93`), run outside the
repository in the session scratchpad with `playwright-core@1.63.0` and the
cached Chromium build: a page containing `<noscript><p>NO SCRIPT RAN</p>
</noscript>` and a script that rewrites its heading. With
`javaScriptEnabled: true` the heading read "script ran" and the noscript
paragraph was absent from the DOM; with `javaScriptEnabled: false` the heading
was unchanged, the paragraph was present and `isVisible()` was true. The
mechanism the plan relies on for the 12 disabled-JS journeys works in the
harness's browser.

Round-02 findings, status: 1 (client-only rows) resolved by removing them and
by the "Do not port tests that fabricate an ActionResult" rule at `:257-259`;
2 (stage-1 default action) resolved at `:210-212`; 3 (`remote` precedence)
resolved at `:160-163` as a dispatch rule, which matches kit; 4 (one-sided
CSRF) resolved at `:152-156`.

## Findings

No material findings remain. The plan's kit claims check out against the pinned
runtime and the upstream tests, every scenario row now names a Go-produced
value or status it depends on, the browser paths are selected where the harness
can select them, the isolation mechanism is product behaviour skgo's API can
provide, and the stages build the final shape from the first commit.

### 1. nitpick — "Non-form bodies pass the CSRF check" is slightly wider than kit

`form-actions.md:154-156` says non-form bodies pass CSRF and reach the 415.
Kit's `is_csrf_forbidden` (`csrf.js:38-43`) treats a request with *no*
`content-type` header as a form: a cross-origin POST with no body type is 403,
not 415. Say "bodies with a non-form content type" so the interface check does
not assert a 415 kit would not give.

### 2. nitpick — The existing remote-form CSRF check is not the one to copy

The plan reuses "the existing Go runtime" (`:271`). The nearest existing code,
`document_form.go:129-134`, compares `Origin` against the app origin only,
while kit routes native page POSTs through `is_csrf_forbidden` with
`csrf_trusted_origins` (`respond.js:116-120`) and skgo already honours that
list on the remote endpoint (`endpoint.go:600`). The plan's "configured
trusted-origin forms are allowed" (`:153`) is right; a builder who lifts the
remote-form check will fail it. Worth one sentence pointing at `endpoint.go`
as the model, or a note that the remote-form path is itself stricter than kit.

### 3. nitpick — Say when the generated action types land

Stage 1 (`:210-216`) returns "typed data" and stage 4 (`:226-229`) has
"generated success/failure types satisfy the stated claims". Stage 2's
validation failure (`:217-220`) already needs the `ActionFailure<T>` half of
kit's `ActionData` union for the editor to type-check `form.errors`. State
that the `actions` export typing is generated in stage 1 and extended with
failure types in stage 2, leaving stage 4 the deliberate-misuse type-check only.

## Outcome

only nitpicks remain
