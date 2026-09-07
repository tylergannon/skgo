Feature: Server loads written in Go

  Every `+page.server.ts` and `+layout.server.ts` under `/account` is generated
  from a `page.server.go` or `layout.server.go` beside it, and every generated
  body throws. SvelteKit's own client asks Go for `/<route>/__data.json` on
  every navigation, so anything that renders below proves Go answered it.

  The section is protected by one rule, written once in its layout, and the
  session that rule reads is derived once by the app's `handle` hook.

  Scenario: A layout's data and its page's data arrive in the document itself
    Given I have signed in as "ada"
    When I visit "/account"
    Then the document response came from skgo in the expected mode
    And the account layout greets "ada"
    And the account page says its parent loaded "ada"
    And the browser never asked for the page's data

  Scenario Outline: A signed-out visitor is turned away from every page in the section
    Given nobody has signed in
    When I visit "<path>"
    Then I land on "/"
    And I see "Home"
    And the account layout is not on the page

    Examples:
      | path               |
      | /account           |
      | /account/orders    |
      | /account/statement |

  Scenario: The same rule lets a signed-in visitor through
    Given I have signed in as "grace"
    When I visit "/account/orders"
    Then the account layout greets "grace"

  Scenario: A value the load promised is settled before the page is rendered
    A load may hand back a value it does not have yet. Go waits for it and then
    renders, so the page arrives whole rather than in two pieces — the streaming
    kit does after the document is a separate thing, and skgo does not do it yet.

    Given I have signed in as "ada"
    When I visit "/account/orders"
    Then the document already carried as many orders as the total said
    And there are as many orders as the total said
    And the browser never asked for the page's data

  Scenario: Moving between two pages under one layout does not re-run the layout's load
    Given I have signed in as "ada"
    When I visit "/account"
    And I note the layout serial
    And I follow the "Orders" link
    Then I see "Orders"
    And the layout serial is unchanged
    When I follow the "Overview" link
    Then I see "Overview"
    And the layout serial is unchanged
    When I refresh the account layout
    Then the layout serial has changed
    And the account layout greets "ada"

  Scenario: A load that fails puts the visitor on an error page
    Given I have signed in as "ada"
    When I visit "/account/statement"
    Then I see "Error 402"
    And the error message is "Your account is in arrears"
