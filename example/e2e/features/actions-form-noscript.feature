Feature: A named Go page action works without JavaScript

  Scenario: A native POST saves Grace with scripting disabled
    Given I open "/"
    When I open the capability "Page form actions"
    Then the Actions editor shows the Ada fixture
    And the Actions browser has JavaScript disabled
    When I save Grace with the native form
    Then the native response already contains the Grace receipt and saved profile
    And the Actions page shows the Grace receipt and saved profile

  Scenario: A native rejected edit retains fields with scripting disabled
    Given I open "/actions"
    Then the Actions editor shows the Ada fixture
    And the Actions browser has JavaScript disabled
    When I submit an invalid Grace edit with the native form
    Then the native failure response already contains the retained fields and Ada profile
    And the Actions page retains the invalid Grace fields and Ada profile
