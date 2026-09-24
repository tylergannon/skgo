# Adversarial review: form-actions proof design, round 01

Date: 2026-09-24 14:25 (local). Reviewer: Claude (Fable 5.1), read-only.

## Target

`ephemeral/plans/form-actions.md` on branch `codex/form-actions-proof` at
`af56746` ("docs: define form action showcase and acceptance proof"), plus the
uncommitted "Stages of implementation" section (stages 1–5) in the working
tree. This is a design review before implementation; no implementation diff
exists yet.

Scope note. The caller's constraints (read-only apart from this artifact, the
artifact path, "design review before implementation") are operating
constraints and were honoured. The caller's "authoritative user context"
(upgrade `./example` to showcase the feature with unambiguous completion; reuse
existing devalue support and send genuine gaps upstream to polytype) was
treated as a requirement to check the plan against, not as a safe area or a
predicted verdict. No narrowing of subject matter was requested, so none was
ignored.

## Evidence inspected

Repository instructions: `CLAUDE.md` (kit is the specification; delegate
missions not methods; Gherkin acceptance with load-bearing scenarios and
screenshots; "never derive what you expect from the thing you are testing";
pinned sources at one absolute path), `ephemeral/sveltekit-current/SKILL.md`,
worklog `ephemeral/worklog/202609241333-form-actions-proof.md`, earlier scoping
in `ephemeral/plans/2026-09-05-skgo-build-plan.md`, semantic-index dossiers
`ephemeral/semantic-index/sources/kit/server-runtime/actions.md` and
`.../test-apps/load-and-action-scenarios.md`.

Pinned kit `reference/kit@3.0.0-next.27/src`:
`runtime/server/page/actions.js` (dispatch, `action_result_json`, 405/415/404,
`fail`, redirect, `get_action_location`), `runtime/server/page/index.js`
(action-then-load ordering, prerender/ssr restrictions), `runtime/server/page/
render.js` (`form_value`, hydrate `form:` and `status:` rules), `runtime/app/
forms/client.js` (enhance headers, result handling, destination navigation,
hook-error shape), `runtime/client/client.js` (`applyAction`; container submit
listener intercepts only GET forms), `core/sync/write_types/index.js`
(ActionData/SubmitFunction inference), `utils/exports.js`, `runtime/server/
csrf.js`, `runtime/server/respond.js`. Confirmed that the pinned tree contains
no `test/` directory (kit's `test/apps/basics` action fixtures are absent).

skgo: `document.go:474-500` (POST without `?/remote` → 405, comment at :482),
`document_form.go` (package comment, `runFormAction`, CSRF, seed → `ssr.Form`),
`document_assemble.go:212-220` (`"form: null"` literal, status push rule),
`endpoint.go:650-664`, `internal/ssr/ssr.go:52-62`, `internal/adapter/
skgo-adapter/entry.js:229,242`, devalue call sites (`StringifyWith`,
`UnevalWith`, `unevalJSON`), polytype v1.1.0 devalue API, `internal/gen`,
`internal/newapp/newapp.go:388`, `example/web/src/hooks.go`,
`example/businesslogic/store.go:49` (global `Default` store, no reset),
`example/web/src/routes/+page.svelte`, `+layout.svelte`,
`example/web/src/routes/account/+page.server.ts`.

e2e harness: `example/e2e/playwright.config.ts` (projects `chromium`
`testIgnore: /form-noscript/` and `noscript` `javaScriptEnabled:false`
`testMatch: /form-noscript/`), `global-setup.ts` (server must already run;
mode from `x-skgo-mode`), `steps/fixtures.ts` (`shot`, AfterStep capture,
`SKGO_E2E_SCREENSHOTS` policy, `hydrated()`), `steps/form.ts:297` ("the browser
ran no script"), `steps/showcase.ts`, `features/showcase.feature`,
`features/form-noscript.feature`.

`ephemeral/tmp/form-actions-review-01/` contains only agent logs, not proof.

## Findings

### 1. issue — "Save without a result" asserts something the native paths cannot show from the real form value

Plan `ephemeral/plans/form-actions.md:45` requires, for all three browser
paths, that "the outcome panel says no action data was returned, based on the
real form value". Kit's contract makes that visible only on the enhanced path:

- `actions.js:50` — enhanced no-data success is `action_json({...result,
  status: 204, data: undefined})`. That is the only place 204 exists.
- `actions.js:189-190` — the server-side result for the native document is
  `{type: 'success', status: 200, data: undefined}`.
- `render.js:95-97` — `form_value = data ?? null`, so the rendered document and
  the `form` prop carry `null`.
- `render.js:500` — `status:` is only written into the hydrate script when
  `status !== 200 && !error`, so a no-data success hydrates identically to a
  fresh GET of the same page.

On the native and native-then-hydration rows, `form === null` and `page.status
=== 200` are exactly the state of a page nobody has submitted. A panel that
says "no action data was returned" on that evidence is manufacturing a message
from the absence of data, which the plan itself forbids ("neither panel
manufactures a success message") and which `CLAUDE.md` names as the empty-page
trap ("a `Then` whose only content is 'X is absent'"). Line 70-72 correctly
states the 204-versus-200 distinction, so the row at line 45 contradicts the
plan's own rule.

Impact: builders either invent a sentinel in the Go action (a non-null value,
which then is not "no data") or write a step that passes on an unsubmitted
page. Either way the scenario is not load-bearing for two of its three rows.
Rewrite the row: native evidence is the saved state (Archived) plus the absence
of a receipt *and* a positive marker that a POST happened (e.g. the submitting
response observed at 200 with the document, and the enhanced row alone
asserting `status: 204` in the JSON result).

### 2. issue — The Scenario Outline shape cannot select a browser context, so "no JavaScript" rows would run with scripting on

The illustrative outline at `form-actions.md:80-97` varies the browser path via
an Examples column (`Kit enhancement` / `no JavaScript` / `native then
hydration`) consumed by a `Given I open the profile editor using <submission>`
step. In the existing harness, JavaScript being off is a property of the
Playwright project, not of a step: `playwright.config.ts:41-52` selects
`javaScriptEnabled: false` only for the `noscript` project and routes features
to it by filename (`testMatch: /form-noscript/`), with the config comment at
:35-40 saying exactly why. The only existing "ran no script" step
(`steps/form.ts:297`) asserts a contact-page-specific `messages-pending`
element, so it cannot be reused for the editor.

A step cannot turn scripting off in an already-created context, so the `no
JavaScript` row, as designed, executes under `chromium` with kit's client
loaded. Kit's client does not intercept POST forms (`client.js:3309`: `if
(method !== 'get') return;`), so the row still submits natively and every
assertion passes, yet the plan's own evidence requirement at :61 ("Browser
context actually disables scripting") is never met. The plan also says the
evidence "cannot" come from a screenshot alone (:126-127) but names no
mechanism.

Impact: the 36-journey matrix claims a no-JS column that the suite cannot
distinguish from native-with-scripting; twelve journeys would be counted twice
under different names. The plan must state that browser path is selected at
feature/tag/project level (a `@noscript` tag or a second feature file matched
by the existing `noscript` project, and an extension of `testIgnore` so it does
not also run under `chromium`) and that every no-JS scenario includes a
positive context-level check (e.g. an element rendered only by a `<noscript>`
block, or a script-set attribute being absent while a server-set attribute is
present).

### 3. issue — Scenarios are invented rather than ported from kit's own action tests, and the plan's contract sources omit them

`CLAUDE.md` ("kit is the specification"; the memory rule "port, don't
reverse-engineer") requires mirrored features to be mapped from kit's own
implementation *and tests*. The plan's "Source of the contract" section
(`form-actions.md:199-207`) lists five runtime/source files and says
"Builders must read those pinned sources directly", but kit's action test app
and its Playwright specs are not among them, and they are not in the pinned
tree at all: `reference/kit@3.0.0-next.27` contains only `LICENSE`,
`README.md`, `package.json`, `src`, `svelte-kit.js`, `types`. The
semantic-index dossiers `ephemeral/semantic-index/sources/kit/server-runtime/
actions.md` and `.../test-apps/load-and-action-scenarios.md` cite
`reference/kit/packages/kit/test/apps/basics/...` paths that do not exist,
so an agent following them lands on an empty path and proceeds on priors,
which is exactly the failure `CLAUDE.md` warns about for the pinned sources.

The consequence is visible in the plan: the edge table's "hook refusal shows
its 403 result correctly in both submission modes" (:119) treats the enhanced
hook-error the same as an action result, but `forms/client.js:207-222`
shows a handle-hook `error()` on the JSON path arrives as an App.Error-shaped
body turned into an `HttpError`, not an ActionResult with `data`; the
csr=false row (:120) and the ssr=false row (:121) omit the native error case
that kit's tests exercise; and nothing covers the `x-sveltekit-action` header's
real role (it only decides page-vs-endpoint dispatch; enhancement is detected
by `Accept: application/json`, `actions.js` `is_action_json_request`).

Impact: the acceptance suite mirrors an author's reading of five files instead
of kit's demonstrated behaviour, so the wrong scenarios can pass green. The
plan should require that kit's action test fixtures (`packages/kit/test/apps/
basics/src/routes/actions/**` and `test/apps/basics/test/*.js` at
3.0.0-next.27) be fetched into the pinned reference, and that the edge table is
derived from them, with the two rows above corrected.

### 4. issue — The `form` wire seam is deferred to stage 4 though stage 1 cannot ship without it

Stage 1 (`form-actions.md:177-181`) promises "typed data" returned from Go and
"a native result survives hydration", while stage 4 (:189-192) defers "custom
transported results". In skgo today the slot the result must travel through is
not a devalue channel at all:

- `internal/ssr/ssr.go:59` — `Form any \`json:"form"\`` is JSON; the package
  comment in `document_form.go:17` says this is a slot for the classic actions
  "kit also has" and that skgo has no equivalent.
- `internal/adapter/skgo-adapter/entry.js:229,242` — `form: req.form ?? null`
  passes that JSON through undecoded, so a `Money` or a `Date` would reach the
  root component as a plain object.
- `document_assemble.go:215` — the hydrate script hardcodes `"form: null"`,
  and the `status:` push rule at :212-220 mirrors `render.js:500` only for the
  loads path.

Kit's contract is one wire format for both paths: `render.js:480-500`
hydrates `form: uneval_action_response(...)` (devalue uneval through the
transport encoders) and `actions.js` `action_result_json` stringifies enhanced
data through the same transport. Building stage 1 on the existing JSON slot
means the first "typed" receipt is untyped on the SSR side, the hydration
assertion in stage 1 passes on a document that still says `form: null` (a
Svelte component reading `form` from the *data* prop would look identical),
and stage 4 rewrites the seam that stages 1-3 were validated against.

Impact: guaranteed rework and a stage-1 proof that is not proof of the
feature's real path. The `form` seam (devalue text from Go via the existing
`StringifyWith`/`UnevalWith` reducers, decoded through transport in entry.js,
`form:` and `status:` emitted in the assembler by kit's rules) belongs in
stage 1, with `Money` used as the stage-1 receipt so the transport is exercised
from the first commit. This is also what the caller's "reuse existing devalue
support" requires; the plan's sentence at :139-141 says it but the staging
does not.

### 5. issue — "State is isolated between scenarios and browser modes" names no mechanism, and the only obvious one is forbidden by the plan

`form-actions.md:38-40` requires each scenario to start from the Ada Lovelace
fixture and isolation "between scenarios and browser modes". The suite runs
against a server that is already up (`global-setup.ts:14-20`, config comment
at `playwright.config.ts:15`) with `workers: 1`, and the example's state is a
process-global store (`example/businesslogic/store.go:49` `var Default =
NewStore()`) with no reset entry point. The chromium and noscript projects hit
the same process, and `dev` and `prod` runs may hit the same binary in
sequence. The plan simultaneously forbids "test infrastructure in the product"
(:211-212, :156-157), so a test-only reset route is off the table and the
plan does not say what replaces it.

The scenarios that follow depend on this being solved: "Saved profile says
Archived, including on a later GET" (:45), "a subsequent editor GET proves the
fixture was not changed" (:119), and every 422/403/500 row's "saved profile
shows Ada Lovelace...". Without isolation, a scenario that runs after "Save
with a result" sees Grace Hopper and either fails spuriously or, worse, the
builder relaxes the literal expectations to make the order work, which is the
"derive expectations from the thing under test" trap.

Impact: an unresolved design decision that shapes the Go example (per-session
or per-cookie profile keyed in the store versus a seed step) and the Gherkin
`Given`. The plan should decide it explicitly. A session-scoped profile (a
cookie the editor sets on first GET, which the redirect journey already needs)
is product behaviour rather than test infrastructure and satisfies both
constraints.

### Nitpicks (not counted toward the five)

- `form-actions.md:64-66` prescribes a specific UI ("Show details" panel) for
  hydration observation. `CLAUDE.md` says missions, not methods; the existing
  `hydrated()` fixture already waits for kit's client to take over
  (`scrollRestoration === 'manual'`), and any real client-only interaction is
  sufficient evidence. State the observation requirement, not the widget.
- The edge table's ssr=false row (:121) should say that kit emits DEV warnings
  and the *client* receives the `form` value via the enhanced path only; it is
  currently ambiguous whether "native redirect works" is a claim about kit or
  skgo.

## Outcome

material findings remain
