Feature: Go page actions compose with ordinary pages

  Each scenario submits through Kit's `enhance`, so the page it lands on is the
  one Kit's client updated in place. The same forms posted natively, with no
  client at all, are in actions-composition-form-noscript.feature; the bytes Go
  answers either way are asserted in example/contracts_test.go.

  Scenario: An action-only page saves Grace through its unnamed action with Kit enhancement
    Given I open "/actions"
    When I follow the default action example
    Then the default page has no load and offers the Grace form
    When I submit the unnamed Grace action with Kit enhancement
    Then the default action shows the exact Grace receipt and profile
    And a later Go load reads the stored default Grace profile

  Scenario Outline: The <choice> button selects its Go action with Kit enhancement
    Given I open "/actions"
    Then the Actions editor shows the Ada fixture
    When I choose <choice> using Kit enhancement
    Then the chosen <choice> action shows its matching saved state

    Examples:
      | choice  |
      | save    |
      | archive |

  Scenario Outline: Editing <selected> changes only that profile with Kit enhancement
    Given I open "/actions"
    When I follow the two profile editor for <selected>
    Then both profiles show their independent fixtures
    When I save the selected profile with Kit enhancement
    Then only <selected> has the literal edited values
    And a later profile GET retains both literal records

    Examples:
      | selected |
      | ada      |
      | grace    |
