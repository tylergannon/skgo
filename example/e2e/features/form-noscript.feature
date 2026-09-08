Feature: A form works without JavaScript

  The same `sendMessage` form as `form.feature`, submitted by a browser with
  scripting turned off. There is no kit client to intercept the submit, so the
  browser performs the form's own POST to `?/remote=<id>` on the page's URL and
  Go has to answer it with the whole page again, rendered over whatever the
  submission did.

  These scenarios run in their own Playwright project, the one configured with
  `javaScriptEnabled: false`. Nothing on the page below can have been put there
  by a script, and the Go handler is still the only implementation there is —
  the generated `contact.remote.ts` throws in every body.

  The inbox is deliberately not asserted here. It renders inside a boundary
  with a `pending` snippet, whose children Svelte's server renderer does not
  render, so with scripting off it stays at "loading…" forever. That is the
  example page's shape and not this feature's subject; `form.feature` asserts
  the inbox with scripting on, where the same submission proves Go recorded it.

  Scenario: The account layout and page data are already rendered without JavaScript
    Given the no-script browser has a session for "ada"
    When I visit "/account"
    Then the account layout greets "ada"
    And the account page says its parent loaded "ada"
    And the browser never asked for the page's data

  Scenario: Custom transported values render without JavaScript
    Given I open "/pricing"
    Then I see "Pricing"
    And the featured plan costs "$45.00"
    And the browser never asked for the page's data

  Scenario: A deliberately clientless page is complete without JavaScript
    Given I open "/plain"
    Then I see "Plain"
    And the document carries no script

  Scenario: A submission the browser posted itself comes back as the page, carrying its issues
    Given I open "/contact"
    When the contact form is on the page
    And I fill the contact form with name "Grace Hopper", email "grace-at-example" and message "too short"
    And I send the message
    Then the browser ran no script
    And a new document answered the submission
    And the field "email" carries the message "\"grace-at-example\" is not an email address"
    And the field "body" carries the message "A message needs at least 10 characters; this one has 9"
    And the field "from" carries no message
    And the form reports it was not sent
    And the contact form still holds name "Grace Hopper" and email "grace-at-example"

  Scenario: A submission the browser posted itself comes back as the page, carrying its result
    Given I open "/contact"
    When the contact form is on the page
    And I fill the contact form with name "Ada Lovelace", email "ada@example.com" and message "The analytical engine weaves algebraic patterns."
    And I attach the fixture "haiku.txt"
    And I send the message
    Then the browser ran no script
    And a new document answered the submission
    And the receipt greets "Ada Lovelace"
    And the form does not report it was not sent

  # `sendMessage.for("k1")` renders as its own `<form>`, whose action kit's
  # client writes as `?/remote=<hash>/sendMessage/%22k1%22` — the base id, a
  # slash, and the key JSON-stringified and then percent-encoded
  # (`runtime/app/server/remote/form.js`, `form.svelte.js:163`). A browser
  # with scripting off posts exactly that, unmodified, so the key reaches Go
  # exactly as kit's own server would receive it: as the `id` field of the
  # form's argument, not as a control on the page.
  Scenario: A keyed submission the browser posted itself carries its key to the Go handler
    Given I open "/contact"
    When the keyed contact form is on the page
    And I fill the keyed contact form with name "Marie Curie", email "marie@example.com" and message "Two elements, one Nobel each."
    And I send the keyed message
    Then the browser ran no script
    And a new document answered the submission
    And the keyed receipt greets "Marie Curie"
    And the keyed receipt carries the key "k1"
    And the keyed form does not report it was not sent

  # A keyed instance is cached separately from the bare form kit's client
  # would otherwise conflate it with — `form.for(key)` gives each key its own
  # `result`/`issues`/`fields` (`runtime/app/server/remote/form.js`, the
  # `state.remote.forms` cache keyed by id and key together). A rejected
  # keyed submission has to prove both halves of that: its own issues render,
  # and the untouched bare form next to it still reports nothing at all.
  Scenario: A keyed submission the browser posted itself comes back as the page, carrying its own issues, independent of the bare form
    Given I open "/contact"
    When the keyed contact form is on the page
    And I fill the keyed contact form with name "Grace Hopper", email "grace-at-example" and message "too short"
    And I send the keyed message
    Then the browser ran no script
    And a new document answered the submission
    And the keyed field "email" carries the message "\"grace-at-example\" is not an email address"
    And the keyed field "body" carries the message "A message needs at least 10 characters; this one has 9"
    And the keyed form reports it was not sent
    And the keyed contact form still holds name "Grace Hopper" and email "grace-at-example"
    And the bare contact form reports nothing sent and nothing refused
