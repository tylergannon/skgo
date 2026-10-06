Feature: Server loads written in Go

  Every `+page.server.ts` and `+layout.server.ts` under `/account` is generated
  from a `page.server.go` or `layout.server.go` beside it, and every generated
  body throws. SvelteKit's own client asks Go for `/<route>/__data.json` on
  every navigation, so anything that renders below proves Go answered it.

  The section is protected by one rule, written once in its layout, and the
  session that rule reads is derived once by the app's `handle` hook.

  The same claims run against the bundle the adapter built and the modules
  `vp dev` transforms. Go renders the document and owns every load in both.

  Scenario: A named Go matcher result reaches a load after Kit navigation
    Given I open "/typed-load/0"
    Then the typed load says "Order #0"
    When I follow the "Order 42" link
    Then the typed load says "Order #42"
    And exactly 1 document request was made
    When I follow the "Order zero" link
    Then the typed load says "Order #0"
    And exactly 1 document request was made

  Scenario: Typed parameter reads retain only the current execution's dependencies
    Given I open "/typed-dependencies/0/42/first"
    Then the typed dependency load says "Order #0" with ignored "first" at serial 1
    When I follow the "Unread B" link
    Then the typed dependency URL is "/typed-dependencies/0/7/first"
    And the typed dependency load says "Order #0" with ignored "first" at serial 1
    When I follow the "Read A" link
    Then the typed dependency URL is "/typed-dependencies/42/7/first"
    And the typed dependency load says "Order #42" with ignored "first" at serial 2
    When I follow the "Untracked ignored" link
    Then the typed dependency URL is "/typed-dependencies/42/7/second"
    And the typed dependency load says "Order #42" with ignored "first" at serial 2
    When I follow the "Read B instead" link
    Then the typed dependency load says "Order #7" with ignored "second" at serial 3
    When I follow the "Now unread A" link
    Then the typed dependency URL is "/typed-dependencies/0/7/second?read=b"
    And the typed dependency load says "Order #7" with ignored "second" at serial 3
    When I follow the "Now read B" link
    Then the typed dependency load says "Order #42" with ignored "second" at serial 4
    And exactly 1 document request was made

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

  Scenario: Kit reruns only the Go load whose tracked inputs changed
    The page reads slug and x but not y. Its per-visitor serial starts at one,
    while the account layout has its own serial. Changing y cannot rerun the
    page. A refresh reruns both, and the page component keeps local state.

    Given I have signed in as "ada"
    When I visit "/account/reruns/one?x=1"
    Then the rerun page shows slug "one" and filter "1" at serial 1
    And the layout serial is 1
    When I follow the "Slug two" link
    Then the rerun page shows slug "two" and filter "1" at serial 2
    And the layout serial is still 1
    When I follow the "Tracked x" link
    Then the rerun page shows slug "two" and filter "2" at serial 3
    And the layout serial is still 1
    When I increment the rerun page's local count
    And I follow the "Untracked y" link
    Then the rerun page URL is "/account/reruns/two?x=2&y=1"
    Then the rerun page serial is still 3
    And the rerun page's local count is 1
    And the layout serial is still 1
    When I refresh all loads on the rerun page
    Then the rerun page shows slug "two" and filter "2" at serial 4
    And the layout serial is 2
    And the rerun page's local count is 1
    When I invalidate the rerun page's dependency by predicate
    Then the rerun page shows slug "two" and filter "2" at serial 5
    And the layout serial is still 2
    When I refresh all loads on the rerun page
    Then the rerun page shows slug "two" and filter "2" at serial 6
    And the layout serial is 3
