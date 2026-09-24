# Form actions: the example is the proof

Design for upgrading the existing `./example` application. The feature is not
implemented or proved by this document.

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
**Active**. State is isolated between scenarios and browser modes. Expectations
are literals supplied by the scenario, never values copied from the live app.

| Journey | Visitor action | Required visible result |
| --- | --- | --- |
| Save with a result | Save Grace Hopper, grace@example.test, and “Compiler pioneer” | Receipt “Saved Grace Hopper”; saved profile shows those exact three values and Active; a later GET still shows them |
| Save without a result | Archive the profile | Saved profile says Archived, including on a later GET; the outcome panel says no action data was returned, based on the real form value |
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

For the hydration observation, a small client-only “Show details” interaction
can reveal an explanatory panel without fetching or changing form data. Merely
waiting for a load event or timeout does not establish hydration. Error and
redirect destinations must also allow the observation. Capture the outcome
before a later GET, because Kit does not persist action data across a fresh GET.

Do not flatten Kit's status distinctions: no-data success is HTTP 200 with
`status: 204` in the enhanced result, versus a native HTTP 200 document. A
redirect is HTTP 200 carrying a redirect result for enhancement, versus a real
303 for the native POST followed by a destination GET. Check the submitting
response and the final page separately. Success with data is 200, validation
failure is 422, and the two error cases are 403 and 500.

## Acceptance scenarios

These sentences become scenarios in the existing Gherkin suite. Each row above
has the full positive assertions shown, including a screenshot at its outcome.
For example:

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
    | no JavaScript         |
    | native then hydration |
```

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
| Submission can land on another page | Submit success and validation failure from a second page; arrive at the action's destination with the matching receipt or retained errors; enhancement removes the action selector and preserves an unrelated query parameter |
| Cross-page errors use the destination's boundary | A rejected action on another page shows that section's distinctive 403/500 error page, with a working return link |
| Cookies are visible immediately | An action that sets a cookie and returns data has its value displayed by the load in the response/refresh, as well as after a later GET |
| Response headers survive the action | A successful action sets a fixed demo response header; the submitting response carries that exact value while the page shows the expected saved profile |
| Uploaded bytes reach Go | Submit the existing haiku.txt fixture; show filename, 72 bytes, and SHA-256 prefix 84cee680e822 computed by Go from the upload |
| Repeated form fields survive | Submit two selected interests, “math” and “computing”; the receipt shows both, exactly once, in submitted order |
| Custom types survive both delivery paths | A returned Money value renders $7.50 through its transported class method, from action data, both on the server and after hydration/enhancement |
| An endpoint can share the route | A page form still produces its receipt; an endpoint POST to that same route produces a distinct endpoint answer displayed by the demo; observations confirm Kit's dispatch rules |
| A hook can stop an action | A hook redirect leads to the sign-in destination and a hook refusal shows its 403 result correctly in both submission modes; a subsequent editor GET proves the fixture was not changed |
| A page can deliberately omit client JavaScript | Its native action succeeds and renders a receipt and saved profile; the returned document contains no scripts |
| A page can deliberately disable SSR | Its enhanced action shows its receipt after the client boots; a native redirect works; a native data return follows Kit's documented loss of form data rather than claiming an SSR receipt |

For every edge scenario, also assert the pictured page has the expected heading
and substantive content. Network observations supplement the screenshot; they
are attached to the same existing scenario report when needed to substantiate
status, headers, or request selection. A screenshot alone cannot prove an HTTP
status or that JavaScript was disabled.

## Interface checks that support the browser proof

Ordinary Go and generated-type checks cover duplicate names, mixed default and
named actions, actions in a layout, prerendered pages with actions, unknown or
reserved action names, missing actions, unsupported bodies, and cross-origin
form rejection. Verify Kit's corresponding status and Allow behavior. Rejected
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

## Source of the contract and scope

Kit is the specification at
`/Users/tyler/src/skgo/ephemeral/inspiration/reference/kit@3.0.0-next.27`:
`src/runtime/server/page/actions.js` supplies outcomes and dispatch;
`src/runtime/server/page/index.js` supplies native POST/load ordering and page
option restrictions; `src/runtime/server/page/render.js` supplies form state and
hydration; `src/runtime/app/forms/client.js` supplies enhancement and destination
navigation; `src/core/sync/write_types/index.js` supplies action type inference.
Builders must read those pinned sources directly.

Implementation owns classic page actions and this example's action showcase,
using the existing Go runtime, generator, adapter and acceptance suite. It must
preserve existing loads, endpoints and remote forms. No new demonstration app,
test runner, pass/fail dashboard, serializer, or evidence manifest is needed.
