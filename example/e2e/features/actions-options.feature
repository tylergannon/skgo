Feature: Page rendering options govern native Go action documents

  Scenario: A page without client JavaScript renders a successful action
    Given I open the Actions page without client JavaScript
    When I save Grace on the no-client page
    Then the no-client response is a script-free 200 receipt with the exact saved Grace profile

  Scenario: A no-client leaf error uses its surviving error branch options
    Given I open the Actions page without client JavaScript
    When I submit the forbidden no-client action
    Then the no-client error branch renders the 403 section error with its own client boot

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
      | outcome    | profile |
      | validation | Ada     |

  Scenario: A native redirect from a client-rendered page reaches its destination
    Given I open the client-rendered Actions page after boot
    When I submit native sign-in on the client-rendered page
    Then the native action redirects to the signed-in destination with its Go cookie
