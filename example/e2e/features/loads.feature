Feature: Server loads written in Go

  Every `+page.server.ts` and `+layout.server.ts` under `/account` is generated
  from a `page.server.go` or `layout.server.go` beside it, and every generated
  body throws. SvelteKit's own client asks Go for `/<route>/__data.json` on
  every navigation, so anything that renders below proves Go answered it.

  The section is protected by one rule, written once in its layout, and the
  session that rule reads is derived once by the app's `handle` hook.

  The same claims run against the bundle the adapter built and the modules
  `vp dev` transforms. Go renders the document and owns every load in both.

  Scenario: Moving between two pages under one layout does not reload the layout's data
    The layout counts its own runs for each visitor, and this browser is a
    visitor nobody else is, so its first visit to the section is run 1.

    Coming back to Overview runs the layout once more without showing it: the
    Overview page's load reads its parent, and kit runs a parent load the
    client told it to skip whenever a child asks for its data, then leaves it
    out of the response (`runtime/server/data/index.js`). So the page still
    shows run 1, and the refresh after it is run 3.

    Given I have signed in as "ada"
    When I visit "/account"
    Then the layout serial is 1
    When I follow the "Orders" link
    Then I see "Orders"
    And the layout serial is still 1
    When I follow the "Overview" link
    Then I see "Overview"
    And the layout serial is still 1
    When I refresh the account layout
    Then the layout serial is 3
    And the account layout greets "ada"
