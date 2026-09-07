Feature: Pages arrive rendered

  A document leaves Go with its content already in it. The markup is rendered by
  SvelteKit's own renderer inside the Go process, every value in it was answered
  by a Go function while the page was being built, and the same values travel
  down with the document so that kit's client hydrates without asking for any of
  them again.

  Every value named below is a fixture with exactly one source in the app:
  `getSite` in src/routes/site.remote.go answers with the name "skgo" and its own
  path; `getItem` in src/routes/items/[id]/item.remote.go answers "Widget <id>"
  for the id in the URL; the loads under /account answer with the signed-in
  visitor's name; the load in src/routes/(marketing)/pricing/page.server.go
  answers with a featured plan priced at 4500 cents, an amount no other function
  in the app returns. The generated TypeScript beside each of those throws
  "skgo: implemented in Go", so none of these strings has a second way to appear.

  Scenario: The home page's HTML already contains the site name
    Given I open "/"
    Then the document already said the site is named "skgo"
    And the site is named "skgo"

  Scenario: A page under a layout arrives with the layout's data and its own
    Given I have signed in as "ada"
    When I visit "/account"
    Then the document already said "Account of ada"
    And the document already said "The layout loaded ada"
    And the account layout greets "ada"
    And the account page says its parent loaded "ada"

  Scenario: A remote query awaited in markup is answered by Go inside the document
    Given I open "/"
    Then the document already said "src/routes/site.remote.go"
    And the document never mentions "skgo: implemented in Go"
    And the answer came from "src/routes/site.remote.go"

  Scenario: A query with an argument is rendered with the argument Go was given
    Given I open "/items/93"
    Then the document already said the item is named "Widget 93"
    And the document never mentions "Widget 42"
    And the item is named "Widget 93"

  Scenario: Hydration does not refetch what the document already carried
    Given I have signed in as "ada"
    And I note the data request count
    And I note the remote request count
    When I visit "/account"
    Then the account layout greets "ada"
    And I am signed in as "ada"
    And exactly 0 data requests were made since
    And exactly 0 remote requests were made since

  Scenario: A custom-typed value round-trips through the document into the client
    Given I note the data request count
    When I open "/pricing"
    Then the featured plan costs "$45.00"
    And the document never mentions "$45.00"
    And exactly 0 data requests were made since

  Scenario: A page marked csr = false is plain HTML with no script tag
    Given I open "/plain"
    Then the document already said the site is named "skgo"
    And the document carries no script
    And the site is named "skgo"

  Scenario: A page marked ssr = false still arrives as the shell
    Given I open "/spa"
    Then the document carried no rendered page
    And the site is named "skgo"

  Rule: Errors and redirects are the document's

    A page that cannot render normally still arrives rendered, with the status
    kit would give it. Which `+error.svelte` answers, and how many layouts
    survive with it, are kit's decisions and not skgo's: kit walks outward from
    the node that failed to the nearest error page declared above it.

    The fixtures are one per failure and have one source each. The load in
    src/routes/error/expected/page.server.go refuses with 418 and the words
    "This page is a teapot". The one in
    src/routes/error/unexpected/page.server.go fails with an ordinary Go error
    whose text names a database password. The load in
    src/routes/account/statement/page.server.go refuses with 402 and "Your
    account is in arrears", under a section that declares its own
    `+error.svelte`. The layout load in src/routes/account/layout.server.go
    redirects a signed-out visitor to "/", and the query in
    src/routes/error/redirect/redirect.remote.go redirects to "/about" instead
    of answering.

    Scenario: A load that fails returns the error page with the status it threw
      Given I open "/error/expected"
      Then the document was answered with 418
      And the document already said the error page shows "Error 418" and "This page is a teapot"
      And the document already carried the root layout
      And the error message is "This page is a teapot"
      And the browser never asked for the page's data

    Scenario: A load that fails unexpectedly is a 500 that says nothing about why
      Given I open "/error/unexpected"
      Then the document was answered with 500
      And the document already said the error page shows "Error 500" and "Internal Error"
      And the document never mentions "hunter2"
      And the document never mentions "postgres://"

    Scenario: An unknown route returns the error page with 404
      Given I open "/no-such-page"
      Then the document was answered with 404
      And the document already said the error page shows "Error 404" and "Not Found"
      And the document already carried the root layout
      And the browser never asked for the page's data

    Scenario: An expected error in a nested page renders inside its layout
      Given I have signed in as "ada"
      When I visit "/account/statement"
      Then the document was answered with 402
      And the document already said "Account of ada"
      And the document already said the error page shows "Account error 402" and "Your account is in arrears"
      And the account layout greets "ada"
      And the browser never asked for the page's data

    Scenario: A page that catches its own failure still carries the failure's status
      Kit installs one error transform for the whole render and Svelte calls it
      for every boundary that has a `failed` snippet, the app's own included.
      So a page that handles its failure gracefully renders — and the document
      is still answered with the status the caught error had.
      src/routes/error/boundary/boundary.remote.go refuses with 409.

      Given I open "/error/boundary"
      Then the document was answered with 409
      And the document already said the page's own heading is "Sensor"
      And the document already said "The sensor is being calibrated"
      And I see "Sensor"
      And the browser never asked for the page's data

    Scenario: A redirect thrown from a load answers with a 3xx and no body
      Given nobody has signed in
      When I visit "/account"
      Then I land on "/"
      And I see "Home"
      When I ask for "/account" without following redirects
      Then it answered 307 to "/" with no body

    Scenario: A redirect thrown while the page renders answers the same way
      When I visit "/error/redirect"
      Then I land on "/about"
      When I ask for "/error/redirect" without following redirects
      Then it answered 307 to "/about" with no body

  Rule: The engine is Go's

    The renderer is a pool of engines inside the Go binary. Nothing in it reads a
    file, opens a socket or sets a timer, and the only way a value reaches it is
    a call back out to Go.

    Scenario: Two pages rendering at once do not share a runtime
      When "/items/11" and "/items/77" are asked for at the same moment
      Then the first document says the item is named "Widget 11"
      And the second document says the item is named "Widget 77"
      And neither document carries the other's item

    Scenario: A remote function called during render is answered by Go, not by a stub
      Given I open "/"
      Then the document already said "src/routes/site.remote.go"
      And the document never mentions "skgo: implemented in Go"
      And nothing on the page failed to load

    Scenario: A render that throws yields a Go error and a static error page, never a blank
      The root layout is the one node no `+error.svelte` can guard, so a throw
      there takes the render down and takes the retry down with it.
      src/routes/+layout.svelte throws for /error/render and nowhere else.

      Given I open "/error/render"
      Then the document was answered with 500
      And the document is kit's static error page saying 500 and "Internal Error"

    Scenario: A command called during render is refused
      src/routes/error/command/+page.svelte awaits a command in its markup.
      Kit refuses a command while a document is being rendered, because a
      document is produced on every navigation and the mutation would run
      again on every reload.

      Given I open "/error/command"
      Then the document was answered with 500
      And the document already said the error page shows "Error 500" and "Internal Error"
      And the document never mentions "Cannot call a command"
      And the tally is nowhere on the page
