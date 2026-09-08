Feature: Go beside the route it serves

  SvelteKit names a route directory after the URL it serves, so it can be
  `[id]`, `(marketing)` or `[...rest]`. A developer puts the Go for that route
  in that directory and nothing else about the app changes. Every page below
  reports the file that answered it, and that file lives in the route's own
  folder.

  Scenario: A dynamic route is answered from its own bracketed directory
    Given I open "/items/7"
    Then the document response came from skgo
    And every part of the page loaded
    And I see "Item 7"
    And the item is named "Widget 7"
    And the answer came from "src/routes/items/[id]/item.remote.go"

  Scenario: A page inside a layout group is answered from inside the parentheses
    Given I open "/"
    When I click the link to "/pricing"
    Then I see "Pricing"
    And the plans are "Hobby, Team, Enterprise"
    And exactly 1 document request was made

  Scenario: A rest parameter reaches the Go function in its own directory
    Given I open "/docs/guide/getting-started"
    Then every part of the page loaded
    And I see "Docs"
    And the doc is titled "guide / getting-started"
    And the doc is 2 segments deep

  Scenario: The route tree's own root directory holds Go too
    Given I open "/"
    Then every part of the page loaded
    And the site is named "skgo"
    And the answer came from "src/routes/site.remote.go"
