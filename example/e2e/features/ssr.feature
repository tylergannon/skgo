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
  visitor's name. The generated TypeScript beside each of those throws
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

  Scenario: A page marked csr = false is plain HTML with no script tag
    Given I open "/plain"
    Then the document already said the site is named "skgo"
    And the document carries no script
    And the site is named "skgo"

  Scenario: A page marked ssr = false still arrives as the shell
    Given I open "/spa"
    Then the document carried no rendered page
    And the site is named "skgo"

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
