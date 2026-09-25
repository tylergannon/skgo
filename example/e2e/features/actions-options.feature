Feature: Page rendering options govern native Go action documents

  Kit's client is what differs between these pages: one turns it off for the
  page and back on for its error branch, the other renders nothing on the server
  and draws everything after boot. The documents Go answers for the same posts
  are asserted in example/contracts_test.go.

  Scenario: A no-client leaf error uses its surviving error branch options
    Given I open the Actions page without client JavaScript
    When I submit the forbidden no-client action
    Then the no-client error branch renders the 403 section error with its own client boot

  Scenario: An unexpected no-client action uses the safe section error page
    Given I open the Actions page without client JavaScript
    When I submit the unavailable no-client action
    Then the no-client error branch renders the safe 500 section error with its own client boot

  Scenario: An enhanced client-rendered action displays the Go receipt after boot
    Given I open the client-rendered Actions page after boot
    When I save Grace with Kit enhancement on the client-rendered page
    Then the client-rendered page shows the exact Go receipt and saved Grace profile

  Scenario Outline: A native client-rendered <outcome> returns a shell and then the ordinary page
    Given I open the client-rendered Actions page after boot
    When I submit <outcome> natively on the client-rendered page
    Then the <outcome> response is a styled shell with the action status and no action outcome
    And after boot the ordinary client-rendered page shows <profile> without an action outcome

    Examples:
      | outcome     | profile |
      | success     | Grace   |
      | validation  | Ada     |
      | forbidden   | Ada     |
      | unavailable | Ada     |
