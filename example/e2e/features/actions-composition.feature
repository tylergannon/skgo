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
      | native form     |

  Scenario Outline: The <choice> button selects its Go action with <submission>
    Given I open "/actions"
    Then the Actions editor shows the Ada fixture
    When I choose <choice> using <submission>
    Then the chosen <choice> action shows its matching saved state

    Examples:
      | choice  | submission      |
      | save    | Kit enhancement |
      | save    | native form     |
      | archive | Kit enhancement |
      | archive | native form     |

  Scenario Outline: Editing <selected> changes only that profile with <submission>
    Given I open "/actions"
    When I follow the two profile editor for <selected>
    Then both profiles show their independent fixtures
    When I save the selected profile with <submission>
    Then only <selected> has the literal edited values
    And a later profile GET retains both literal records

    Examples:
      | selected | submission      |
      | ada      | Kit enhancement |
      | ada      | native form     |
      | grace    | Kit enhancement |
      | grace    | native form     |
