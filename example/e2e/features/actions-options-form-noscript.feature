Feature: Page rendering options with scripting disabled

  Scenario: A no-client action remains usable without browser scripting
    Given I open the Actions page without client JavaScript
    Then the browser has scripting disabled and the no-client form is visible
    When I save Grace on the no-client page
    Then the no-client response is a script-free 200 receipt with the exact saved Grace profile

  Scenario: A no-client validation failure remains editable without scripting
    Given I open the Actions page without client JavaScript
    Then the browser has scripting disabled and the no-client form is visible
    When I submit the invalid Grace edit on the no-client page
    Then the no-client response is script-free 422 with the exact edit and unchanged Ada profile

  Scenario Outline: A client-rendered native <outcome> has a styled shell without scripting
    Given I request the client-rendered Actions page without scripting
    When I post <outcome> to the client-rendered page without scripting
    Then the <outcome> response is a styled shell with the action status and no action outcome
    And the page still has no rendered action or profile without scripting

    Examples:
      | outcome    |
      | success    |
      | validation |
      | forbidden  |
      | unavailable |
