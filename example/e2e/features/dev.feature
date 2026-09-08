@dev
Feature: In dev the document is kit's and the answers are still Go's

  `vp dev` never runs the adapter, so there is no SSR bundle in dev and no
  engine for Go to render a document with. Kit's own dev server owns the
  document instead — and the only implementation it can reach for a load or a
  remote function is the generated stub, which throws by design. So the app
  turns server rendering off in dev (web/src/routes/+layout.ts: `ssr = !dev`),
  which is kit's own switch for "the browser renders this"
  (packages/kit/src/runtime/server/page/index.js returns the shell without
  running a single load when `ssr === false`).

  What is left is exactly the app skgo served before it had an engine: kit's
  shell arrives, kit's client boots, and every value on every page comes from
  Go — `__data.json` for a load and `/_app/remote/...` for a remote function.
  That is what this file is for. Each scenario is the dev half of a claim
  ssr.feature makes about the production document, in the same order, and none
  of them can pass unless Go answered.

  The fixtures are ssr.feature's: `getSite` in src/routes/site.remote.go answers
  with the name "skgo" and its own path, `getItem` in
  src/routes/items/[id]/item.remote.go answers "Widget <id>", the loads under
  /account answer with the signed-in visitor's name, and the load in
  src/routes/(marketing)/pricing/page.server.go answers with a featured plan
  priced at 4500 cents. The generated TypeScript beside each of those throws
  "skgo: implemented in Go".

  Scenario: The home page arrives as kit's shell and Go names the site
    Given I open "/"
    Then the document carried no rendered page
    And the site is named "skgo"
    And the page never mentions "skgo: implemented in Go"

  Scenario: A page under a layout gets both loads from Go
    Given I have signed in as "ada"
    When I visit "/account"
    Then the document carried no rendered page
    And the account layout greets "ada"
    And the account page says its parent loaded "ada"

  Scenario: A remote query is answered by Go, not by the stub beside it
    Given I open "/"
    Then the answer came from "src/routes/site.remote.go"
    And the page never mentions "skgo: implemented in Go"
    And nothing on the page failed to load

  Scenario: A query with an argument is answered with the argument Go was given
    Given I open "/items/93"
    Then the item is named "Widget 93"
    And the page never mentions "Widget 42"

  Scenario: The shell asks Go for the page's data exactly once
    A document that carries nothing has to be filled in, and kit's client does
    that with one request per navigation. One, not none — the prod half of this
    claim is that the number is zero — and never two, which would mean the app
    went back for something it had already been given.

    Given I have signed in as "ada"
    And I note the data request count
    When I visit "/account"
    Then the account layout greets "ada"
    And I am signed in as "ada"
    And exactly 1 data request was made since

  Scenario: A custom-typed value is rebuilt by the browser from what Go sent
    "$45.00" is `Money.format()` in src/hooks.ts over 4500 cents. In prod the
    engine has the app's transport and the price stands in the document; here
    the cents travel to the browser under the transport key and kit's client
    decodes them, so the method runs where the class is declared.

    Given I open "/pricing"
    Then the document carried no rendered page
    And the featured plan costs "$45.00"

  Scenario: A page that turns csr off has it back in dev
    `csr = false` is a claim about a document somebody rendered, and in dev
    nobody did: a branch with neither `ssr` nor `csr` leaves kit answering with
    an empty shell that boots nothing, and the page is blank. So
    web/src/routes/plain/+page.ts declares `csr = dev` and the page renders in
    the browser here, the same way /spa does.

    Given I open "/plain"
    Then the document carried no rendered page
    And the site is named "skgo"

  Scenario: A page marked ssr = false is the same shell it is in prod
    Given I open "/spa"
    Then the document carried no rendered page
    And the site is named "skgo"

  Rule: Errors and redirects reach the browser through Go's data endpoint

    A load that refuses puts its refusal in the branch `__data.json` carries,
    and kit's client renders the nearest `+error.svelte` from it. The document
    is a 200 shell in every case below, because kit's dev server wrote it before
    any load ran; the status the load chose is in the data response instead.

    The fixtures are ssr.feature's: 418 and "This page is a teapot" from
    src/routes/error/expected/page.server.go, an ordinary Go error naming a
    database password from src/routes/error/unexpected/page.server.go, 402 and
    "Your account is in arrears" from
    src/routes/account/statement/page.server.go, and the redirect to "/" from
    src/routes/account/layout.server.go.

    Scenario: A load that fails puts the visitor on an error page
      Given I open "/error/expected"
      Then the document carried no rendered page
      And Go's data endpoint for "/error/expected" answered 418
      And I see "Error 418"
      And the error message is "This page is a teapot"

    Scenario: A load that fails unexpectedly says nothing about why
      Given I open "/error/unexpected"
      Then Go's data endpoint for "/error/unexpected" answered 500
      And I see "Error 500"
      And the error message is "Something went wrong on our end."
      And the error page shows the support id "case-1121"
      And the page never mentions "hunter2"
      And the page never mentions "postgres://"

    Scenario: An unknown route is an error page saying 404
      Given I open "/no-such-page"
      Then the document was answered with 404
      And I see "Error 404"
      And the error message is "Not Found"

    Scenario: An expected error in a nested page renders inside its layout
      Given I have signed in as "ada"
      When I visit "/account/statement"
      Then Go's data endpoint for "/account/statement" answered 402
      And the account layout greets "ada"
      And I see "Account error 402"
      And the error message is "Your account is in arrears"

    Scenario: A page that catches its own failure renders anyway
      src/routes/error/boundary/boundary.remote.go refuses with 409, and the
      page's own boundary shows the refusal rather than being replaced by an
      error page.

      Given I open "/error/boundary"
      Then I see "Sensor"
      And I see the words "The sensor is being calibrated"

    Scenario: A redirect thrown from a load reaches the browser
      Given nobody has signed in
      When I visit "/account"
      Then I land on "/"
      And I see "Home"
      And Go's data endpoint for "/account" answers with a redirect to "/"
