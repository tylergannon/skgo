Feature: Forms written in Go

  `sendMessage` in `src/routes/contact/contact.remote.go` is a SvelteKit form.
  The generated `contact.remote.ts` throws in every body, so anything the page
  shows below came back from Go.

  A form is the only remote kind whose argument the browser builds. Kit's
  client reads the controls, coerces each one, and posts them in its own binary
  envelope with any uploaded file's bytes appended raw — which is why a form
  cannot go down the JSON path the other kinds use.

  Scenario: A form submission reaches Go and updates the page
    Given I open "/contact"
    When the contact page has loaded
    And I note the remote request count
    And I fill the contact form with name "Ada Lovelace", email "ada@example.com" and message "The analytical engine weaves algebraic patterns."
    And I send the message
    Then the receipt greets "Ada Lovelace"
    And the inbox shows a message from "Ada Lovelace" saying "The analytical engine weaves algebraic patterns."
    And exactly 1 remote request was made since

  Scenario: A rejected submission puts each message on its own field and keeps what was typed
    Given I open "/contact"
    When the contact page has loaded
    # One good message first, so that the absence asserted at the end is about
    # a list that is provably rendering rather than about an empty page.
    And I fill the contact form with name "Grace Hopper", email "grace@example.com" and message "A compiler translates one language into another."
    And I send the message
    Then the inbox shows a message from "Grace Hopper" saying "A compiler translates one language into another."
    When I fill the contact form with name "Grace Hopper", email "grace-at-example" and message "too short"
    And I send the message
    Then the form reports it was not sent
    And the field "email" carries the message "\"grace-at-example\" is not an email address"
    And the field "body" carries the message "A message needs at least 10 characters; this one has 9"
    And the field "from" carries no message
    And the contact form still holds name "Grace Hopper", email "grace-at-example" and message "too short"
    And the inbox shows a message from "Grace Hopper" saying "A compiler translates one language into another."
    And the inbox has no message saying "too short"

  Scenario: An uploaded file's bytes reach Go
    Given I open "/contact"
    When the contact page has loaded
    And I fill the contact form with name "Matsuo Basho", email "basho@example.com" and message "A haiku is attached for your consideration."
    And I attach the fixture "haiku.txt"
    And I send the message
    Then the inbox shows a message from "Matsuo Basho" saying "A haiku is attached for your consideration."
    And the message saying "A haiku is attached for your consideration." reports the attachment "haiku.txt" of 72 bytes with digest "84cee680e822"

  Scenario: A submission that bypasses kit's client hydrates to the state Go rendered
    Given I open "/contact"
    When the contact page has loaded
    And I fill the contact form with name "Alan Turing", email "alan@example.com" and message "Machines take me by surprise with great frequency."
    # `form.submit()` fires no submit event, so `enhance` never sees it: the
    # browser makes the same POST a visitor with scripting off makes, and then
    # boots kit's client over the document Go answered with.
    And the browser submits the form itself, without kit's client
    Then a new document answered the submission
    And the receipt greets "Alan Turing"
    # The inbox is rendered by kit's client running the query after hydration,
    # so seeing the message here says two things at once: Go really recorded
    # the submission, and the client booted over the answer without wiping the
    # receipt above off the page.
    And the inbox shows a message from "Alan Turing" saying "Machines take me by surprise with great frequency."

  Scenario: The issues of a submission that bypasses kit's client survive hydration
    Given I open "/contact"
    When the contact page has loaded
    And I fill the contact form with name "Katherine Johnson", email "katherine@example.com" and message "The numbers had to be checked by hand."
    And I send the message
    Then the inbox shows a message from "Katherine Johnson" saying "The numbers had to be checked by hand."
    When I fill the contact form with name "Katherine Johnson", email "katherine-at-example" and message "nope"
    And the browser submits the form itself, without kit's client
    Then a new document answered the submission
    # Proves hydration finished: nothing but kit's client puts the inbox on a
    # freshly loaded document.
    And the inbox shows a message from "Katherine Johnson" saying "The numbers had to be checked by hand."
    # And the issues Go put in the document are still the form's, which they
    # would not be if `<global>.data.f` were missing — the client would then
    # start the form with no issues and re-render them away.
    And the field "email" carries the message "\"katherine-at-example\" is not an email address"
    And the field "body" carries the message "A message needs at least 10 characters; this one has 4"
    And the form reports it was not sent
