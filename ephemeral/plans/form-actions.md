# Form actions: the example is the proof

**Historical proof requirements.** The feature shipped in `c0e0119`. Its
"Acceptance scenarios" matrix, its screenshot policy, its four fault probes and
"Completion that a person can verify" below describe how the original feature
was to be proved, and are not standing gates.
[validation.md](validation.md) replaces them: every server contract the tables
name (status, headers, cookies, redirects, dispatch, the native document) is a
Go test at the real handler in `example/action_protocol_test.go` and
`example/contracts_test.go`; the Gherkin suite keeps the scenarios whose claim
needs kit's client (enhanced submit, hydration of a native answer, client
rendering after boot) or a browser with scripting off posting a real form; a
passing scenario takes no screenshot. The visitor-facing outcomes in the tables
are still the product.

Design for upgrading the existing `./example` application. The feature is not
implemented or proved by this document.

Design consensus reached with Claude Fable 5.1 on 2026-09-24 after three
rounds: [final review](../reviews/form-actions-round-03.md), outcome **only
nitpicks remain**. Its wording clarifications were incorporated in commit
`380697d`. Earlier rounds remain beside that artifact. This is design
consensus, not implementation or runtime acceptance.

The work is on branch `codex/form-actions-proof`. Implementation has not
started; when requested, begin with stage 1 below. No application code,
executable action scenarios, generated action bindings, or runtime screenshots
were produced during planning. All review processes have finished.

Actions belong in the route's `page.server.go`, beside its page load, with
generated `actions` and `load` exports sharing `+page.server.ts`. `server.go`
continues to represent `+server.ts` endpoints. The earlier conversation's Go
function and generic-result signatures were illustrative; the exact public Go
API remains an implementation decision constrained by Kit, generated types,
and the acceptance scenarios below.

## What a visitor gets

The home page's existing capability tour gains **Page form actions**, linking
to `/actions`, with this invitation: **Save a profile, reject an invalid edit,
and follow a redirect. Try every outcome with and without JavaScript.** The
navigation also makes Actions easy to find. The existing remote-form demo stays
available as its own capability.

`/actions` is a small profile editor, using the example's existing visual style.
Keep the submitted fields, outcome, and saved profile together and legible in a
normal desktop viewport. The saved profile comes from a Go server load. The
outcome comes from Kit's `form` and `page.status`. A successful response shows
both what the action returned and what was actually saved; a rejected response
shows the field error beside the retained input and the unchanged saved profile.
Neither panel manufactures a success message from a button click.

Offer clearly labelled native and enhanced versions of the same form. The
enhanced version uses Kit's `$app/forms` enhancement. A short “Try these outcomes”
area supplies real controls for save, validation failure, archive without a
receipt, sign-in and redirect, permission failure, and unexpected failure.
Error pages name the error and offer a route back to the editor. Explanatory
copy says what each control demonstrates; it never declares the feature passed.

The default-action example has its own page under `/actions`; the main editor
demonstrates named actions and different submit buttons. Both remain reachable
from this one tour entry. Submission to another page has a visible destination
under the same section. Exact component structure and Go APIs belong to the
builder.

## Fixed fixtures and visible outcomes

Each scenario starts with its own server-side fixture: the saved profile is
**Ada Lovelace**, **ada@example.test**, biography **First programmer**, state
**Active**. The example gives each visitor a private demo workspace, identified
by a cookie created on first GET and seeded with these values. A fresh browser
context creates a fresh workspace; subsequent POSTs, redirects, and GETs keep
it. Authentication cookies do not replace that workspace cookie. Additional
profiles are scoped to the same workspace. This is the demo's normal visitor
behavior, so scenarios need neither a process-global reset nor order-dependent
setup. Expectations are literals supplied by the scenario, never values copied
from the live app.

| Journey | Visitor action | Required visible result |
| --- | --- | --- |
| Save with a result | Save Grace Hopper, grace@example.test, and “Compiler pioneer” | Receipt “Saved Grace Hopper” with a returned Money value formatted as $7.50; saved profile shows those exact three values and Active; a later GET still shows them |
| Save without a result | Archive the profile | Observe the submitting POST, then a saved profile saying Archived, including on a later GET; no receipt is present. Enhanced submission carries result status 204; the native POST returns a 200 document with null form data, indistinguishable from a GET by form data alone |
| Validation failure | Submit Grace Hopper, grace-at-example, and “Keep this biography” | “Enter a valid email address” beside email; all three entered values retained; saved profile still has all four Ada fixture values; status 422 |
| Redirect | Sign in as ada | Destination greets “Signed in as ada”; its server load reads the action's session cookie; a later GET remains signed in |
| Expected error | Attempt a forbidden edit | Nearest section error page says “Action error 403” and “You cannot edit this profile”; returning to the editor shows the original profile |
| Unexpected error | Trigger the demo service failure | Nearest section error page says “Action error 500”, “Something went wrong on our end.” and support ID “case-1121”; returning shows the original profile |

The unexpected failure contains a known private marker in the Go error. Assert
the positive public error content above and that the marker is absent from the
response and page. An empty page cannot satisfy this scenario.

Run all six journeys through all three browser paths, against both development
and the built Go application: **36 core journeys**.

| Browser path | Evidence that this path really ran |
| --- | --- |
| Enhanced | A POST to the page action URL with Kit's action header; Kit consumes its result; same-page success/failure replaces no document |
| Native, JavaScript disabled | Browser context actually disables scripting; a document POST returns the visible outcome; saved data and errors are complete without client queries |
| Native, then hydration | Native document POST with scripting enabled; returned HTML already contains the outcome; after an independently observed hydration interaction, the same outcome remains |

Hydration must be independently observed through a real client-only interaction
that neither fetches nor changes form data. Merely waiting for a load event or
timeout does not establish hydration. Error and redirect destinations must also
allow the observation. Capture the outcome before a later GET, because Kit does
not persist action data across a fresh GET. The receipt is read from Kit's form
prop/page.form, never supplied by a load as a substitute. Its Money method must
work in the initial server render and after hydration or enhancement.

Do not flatten Kit's status distinctions: no-data success is HTTP 200 with
`status: 204` in the enhanced result, versus a native HTTP 200 document. A
redirect is HTTP 200 carrying a redirect result for enhancement, versus a real
303 for the native POST followed by a destination GET. Check the submitting
response and the final page separately. Success with data is 200, validation
failure is 422, and the two error cases are 403 and 500.

## Acceptance scenarios

These sentences become scenarios in the existing Gherkin suite. Each row above
has the full positive assertions shown, including a screenshot at its outcome.
Browser configuration is selected before the context exists. Put disabled-JS
scenarios in a feature selected exclusively by the existing `form-noscript`
project match; keep the two scripting-enabled paths in the ordinary project.
Every disabled-JS scenario verifies the configured context and a visible
`<noscript>` message beside the substantive page content. Check that the report
actually contains all three paths. An Examples cell or a step name cannot turn
off scripting. The scripting-enabled outline is:

```gherkin
Scenario Outline: A rejected edit stays editable and leaves the saved profile intact
  Given my saved profile is Ada Lovelace, ada@example.test, First programmer, Active
  And I open the profile editor using <submission>
  When I submit Grace Hopper, grace-at-example, and Keep this biography
  Then the action reports status 422 and Enter a valid email address
  And the form still contains Grace Hopper, grace-at-example, and Keep this biography
  And the saved profile shows Ada Lovelace, ada@example.test, First programmer, Active
  And the outcome remains visible after the browser finishes the required path

  Examples:
    | submission            |
    | Kit enhancement       |
    | native then hydration |
```

The disabled-JS feature repeats the same fixture, submission and positive
outcome assertions as a native scenario, with the context and noscript checks.
It does not contain the enhanced or hydration rows. These three actual paths,
rather than three labels on one browser configuration, constitute the matrix.

The rest of the feature is covered by these additional business scenarios,
in development and production. Exercise both native and enhanced submission
where applicable; SSR option cases use the modes stated.

| Scenario | Observable acceptance |
| --- | --- |
| The home page leads to working actions | Its new capability opens the editor, with the actual Ada profile and usable controls; update the existing exhaustive home-tour assertion |
| A default action works without a name | Its plain form action URL saves the exact Grace profile; no named-action selector is needed |
| The chosen button determines the named action | Save produces the Grace receipt and Active profile; archive produces Archived without changing the name; the observed request names the chosen action |
| An action-only page works | The default-action page has no Go page load of its own; its action still returns a typed result that renders |
| A route parameter selects the record | Editing one of two seeded profiles through its dynamic route changes only that profile; both saved profiles are visibly checked against their supplied fixture values |
| Submission can land on another page | Submit success and validation failure from a second page; arrive at the action's destination with the matching receipt or retained errors and status 422. Enhancement uses client navigation to the Go result's location, removing the action selector and preserving an unrelated query parameter; a redirect reaches its specified destination |
| Cross-page errors use the destination's boundary | A rejected action on another page shows that section's distinctive 403/500 error page, with a working return link |
| Cookies are visible immediately | An action that sets a cookie and returns data has its value displayed by the load in the response/refresh, as well as after a later GET |
| Response headers survive the action | A successful action sets a fixed demo response header; the submitting response carries that exact value while the page shows the expected saved profile |
| Uploaded bytes reach Go | Submit the existing haiku.txt fixture; show filename, 72 bytes, and SHA-256 prefix 84cee680e822 computed by Go from the upload |
| Repeated form fields survive | Submit two selected interests, “math” and “computing”; the receipt shows both, exactly once, in submitted order |
| Custom types survive both delivery paths | A returned Money value renders $7.50 through its transported class method, from action data, both on the server and after hydration/enhancement |
| An endpoint can share the route | A page form still produces its receipt; an endpoint POST to that same route produces a distinct endpoint answer displayed by the demo. Also exercise the upstream fallback to page actions when the sibling endpoint has no POST handler |
| A hook can stop an action | A hook redirect leads to the sign-in destination. A hook error is displayed with status 403: its enhanced response is an App.Error-shaped JSON body, not an ActionResult; native submission follows Kit's fatal-error rendering. A subsequent editor GET proves the fixture was not changed |
| A page can deliberately omit client JavaScript | Its native action renders a receipt and saved profile; its validation failure retains entered fields and status 422; its error follows Kit's error-page option rules. Check script omission on the success/failure documents and the effective options of the rendered error branch |
| A page can deliberately disable SSR | Its enhanced action shows its receipt after the client boots; a native redirect works. Native data returns 200/422 with a CSR shell and loses form data; native error returns the error status with that shell and no server-rendered error page. After boot the ordinary page appears, not an invented action receipt/error. Without scripting, check the real styled shell and response status; DEV warnings follow Kit |
| Classic actions and remote forms coexist | A page offers both forms, each showing a distinct result from its own Go implementation. Native remote-form selectors and classic action selectors dispatch in Kit's order; their enhanced submissions retain their respective protocols |

For every edge scenario, also assert the pictured page has the expected heading
and substantive content. Network observations supplement the screenshot; they
are attached to the same existing scenario report when needed to substantiate
status, headers, or request selection. A screenshot alone cannot prove an HTTP
status or that JavaScript was disabled.

## Interface checks that support the browser proof

Ordinary Go and generated-type checks cover duplicate names, mixed default and
named actions, actions in a layout, prerendered pages with actions, unknown or
reserved action names, missing actions, unsupported bodies, and cross-origin
form rejection and permission. Same-origin and configured trusted-origin forms
are allowed; other form origins are rejected. Bodies with a non-form content
type pass the CSRF check and reach the action's content-type rejection (415), rather than being
misreported as CSRF failures (403). Verify Kit's corresponding status and Allow behavior, including
POST in a page's OPTIONS response when actions exist. Undeclared names such as
constructor or toString are missing actions, as in the upstream tests.
Independently vary Accept and x-sveltekit-action: Accept negotiates the action
response format; the action header disambiguates page-versus-endpoint dispatch.
A JSON-accepting POST to a page-only action works without the action header.
Keep Kit's remote-form selector precedence: native `?/remote=<id>` is checked
before classic action dispatch, whereas a JSON action request takes the classic
action path first. This is a dispatch distinction, not a new generator ban on
the name remote; only impose declaration restrictions Kit itself imposes.
The chosen-button cases also exercise its name/value and formenctype overrides.
Rejected
requests must not mutate the supplied fixture. Browser error outcomes remain
covered above; compiler and protocol checks need no fabricated UI.

The showcase consumes generated Kit action types. A deliberately wrong use of
its success or failure fields must fail type checking. Generated TypeScript
action bodies throw, so JavaScript cannot provide a second implementation.
Use the existing polytype devalue and transport paths. If a required value
cannot cross them, supply a minimal reproducer in an upstream polytype feature
request and leave that acceptance claim explicitly unmet.

## Completion that a person can verify

The feature is done when a reviewer can start at the existing Home page, open
Actions, and reproduce every outcome in the tables; switching off JavaScript
still leaves the native editor fully usable. The completed example is the
permanent demonstration and the Gherkin suite is its regression contract.

An independent validator must check that the scenarios are load-bearing and
then run them. In a disposable checkout, temporarily break representative
connections: bypass the Go mutation, drop failure data, drop native hydration
form data, and omit the redirect cookie. Each must fail its relevant scenario.
Restore those changes before the final run; do not add a fault-injection
framework to the product. Inspect the remaining assertions for equally concrete
expectations, including the no-data response and custom transported result.

The final run must execute all 36 core journeys and the applicable edge
scenarios in both server modes, with no skips or missing browser projects.
Use the existing report and assertion screenshots, with screenshot policy
`all`; current curated release captures do not cover these actions. Open every
image and check that the fields, result, saved record or expected error are
readable. An unexpected error boundary, blank page, stale frame, missing
receipt, or disappearing validation message is an unmet claim even if a test
reports success. A deliberate 403/500 page is evidence only for its named error
scenario with the exact expected content.

Delivery identifies the demonstrated commit and links the existing reports and
inspected screenshots for both modes, uploaded and reopened when attached to a
PR. It explicitly lists any unmet scenario. Passing unit tests or publishing
this design cannot substitute for those visible outcomes.

## Stages of implementation

1. **One real save, end to end.** The home tour opens the final named-action
   profile editor with one save button posting to `?/save`. Its Go page action
   saves and returns typed data, including Money, with the generated actions
   export already supplying Kit's action types.
   The real form channel uses the existing polytype devalue/transport support
   through Go-to-renderer, enhanced response, and hydration delivery. Native
   and enhanced submissions work in development and production; the Money
   method and receipt survive hydration. This establishes the complete path
   immediately, with no temporary plain-JSON form channel to replace later.
2. **Every outcome works.** The same editor demonstrates validation failure,
   success without data, redirect with a session cookie, expected error, and
   unexpected error. Generated action typing includes failure data as soon as
   validation is added. All six outcomes work across the three browser paths,
   with literal scenario expectations and visible saved state.
3. **Actions compose with normal Kit routes.** A separate default-action page,
   additional named buttons, action-only and dynamic pages, cross-page submission,
   hooks, shared endpoints, remote forms and page
   options satisfy the corresponding scenarios. Each addition remains visible
   through the existing tour.
4. **Real form data and types round-trip.** Uploads, repeated fields, submitter
   overrides, and deliberate-misuse checks of generated success/failure types satisfy the stated claims,
   extending the transport already proved in stage 1. Invalid declarations and
   requests obey Kit's restrictions. Any
   polytype gap is reduced and sent upstream before its claim can be closed.
5. **Independent proof closes the feature.** The validator checks that failures
   are detectable, runs the complete acceptance suite on the final candidate
   in both modes, opens every screenshot, and delivers the demonstrated commit
   and reports. The example remains the runnable demonstration.

## Source of the contract and scope

Kit is the specification at
`/Users/tyler/src/skgo/ephemeral/inspiration/reference/kit@3.0.0-next.27`:
`src/runtime/server/page/actions.js` supplies outcomes and dispatch;
`src/runtime/server/page/index.js` supplies native POST/load ordering and page
option restrictions; `src/runtime/server/page/render.js` supplies form state and
hydration; `src/runtime/app/forms/client.js` supplies enhancement and destination
navigation; `src/core/sync/write_types/index.js` supplies action type inference.
Builders must read those pinned sources directly.

The installed package omits upstream tests. Use the immutable repository
revision **090d77ff289bf844cf65403f8f5e7eec8a7bb902**, resolved from the official
`@sveltejs/kit@3.0.0-next.27` tag, for the corresponding tests and route fixtures:

- [Shared action tests](https://github.com/sveltejs/kit/blob/090d77ff289bf844cf65403f8f5e7eec8a7bb902/packages/kit/test/apps/basics/test/test.js): the Actions section covers success encodings, retained fields, enhancement, submitter overrides, errors, redirects, and cross-page results.
- [Client action tests](https://github.com/sveltejs/kit/blob/090d77ff289bf844cf65403f8f5e7eec8a7bb902/packages/kit/test/apps/basics/test/client.test.js): use the cross-page navigation cases to check Go-produced result data, status and destination. Client-only refresh, history and callback-option tests inform the mapping but are not additional skgo acceptance scenarios.
- [Server protocol tests](https://github.com/sveltejs/kit/blob/090d77ff289bf844cf65403f8f5e7eec8a7bb902/packages/kit/test/apps/basics/test/server.test.js): action OPTIONS, endpoint fallback, undefined success, stripped location, matching failure status, and CSRF.
- [Action fixtures](https://github.com/sveltejs/kit/tree/090d77ff289bf844cf65403f8f5e7eec8a7bb902/packages/kit/test/apps/basics/src/routes/actions): read each applicable test's actual form and server exports.

Port the applicable behaviors into this showcase's business-language scenarios,
using the fixed fixtures above instead of upstream clock-based baselines. The
rows above map the server-integration behavior exercised by those test groups;
Kit's client implementation remains Kit's. Do not port tests that fabricate an
ActionResult in browser code and then merely assert Kit's client behavior;
acceptance submissions must exercise the Go action and its actual response.
Mode-specific cases belong to the
appropriate browser project, not skipped rows counted as passing. For page
options, the pinned runtime is authoritative; the separate no-ssr/no-csr test
apps at this revision do not contain action-outcome coverage. Keep the root
pinned tree read-only; fetch missing tests at the immutable URLs rather than
silently treating an absent local test path as read evidence.

Implementation owns classic page actions and this example's action showcase,
using the existing Go runtime, generator, adapter and acceptance suite. It must
preserve existing loads, endpoints and remote forms. No new demonstration app,
test runner, pass/fail dashboard, serializer, or evidence manifest is needed.
