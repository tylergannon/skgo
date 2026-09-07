@prod
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
