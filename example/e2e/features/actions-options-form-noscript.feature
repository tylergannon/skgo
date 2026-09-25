Feature: Page rendering options with scripting disabled

  Scenario: A no-client action remains usable without browser scripting
    Given I open the Actions page without client JavaScript
    Then the browser has scripting disabled and the no-client form is visible
    When I save Grace on the no-client page
    Then the no-client response is a script-free 200 receipt with the exact saved Grace profile
