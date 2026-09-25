Feature: Composed Go actions remain native without JavaScript

  Scenario: The default action saves Grace without JavaScript
    Given I open "/actions"
    When I follow the default action example
    Then the default page has no load and offers the Grace form
    When I submit the unnamed Grace action with native form
    Then the default action shows the exact Grace receipt and profile
    And a later Go load reads the stored default Grace profile

  Scenario: A chosen archive button dispatches without JavaScript
    Given I open "/actions"
    Then the Actions editor shows the Ada fixture
    When I choose archive using native form
    Then the chosen archive action shows its matching saved state

  Scenario: A parameter selects only Ada without JavaScript
    Given I open "/actions"
    When I follow the two profile editor for ada
    Then both profiles show their independent fixtures
    When I save the selected profile with native form
    Then only ada has the literal edited values
    And a later profile GET retains both literal records
