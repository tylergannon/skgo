Feature: Go page actions compose with ordinary pages

  Scenario Outline: An action-only page saves Grace through its unnamed action with <submission>
    Given I open "/actions"
    When I follow the default action example
    Then the default page has no load and offers the Grace form
    When I submit the unnamed Grace action with <submission>
    Then the default action shows the exact Grace receipt and profile
    And a later Go load reads the stored default Grace profile

    Examples:
      | submission      |
      | Kit enhancement |

  Scenario Outline: Editing <selected> changes only that profile with <submission>
    Given I open "/actions"
    When I follow the two profile editor for <selected>
    Then both profiles show their independent fixtures
    When I save the selected profile with <submission>
    Then only <selected> has the literal edited values
    And a later profile GET retains both literal records

    Examples:
      | selected | submission      |
      | ada      | native form     |
