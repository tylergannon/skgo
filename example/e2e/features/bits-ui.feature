Feature: Third-party components hydrate over Go's rendered document

  Scenario: Bits UI dialog and scroll area remain interactive after hydration
    Given I open "/bits-ui"
    When I open the library dialog
    Then the library dialog is open
    When I close the library dialog
    Then the library dialog is closed
    And the library scroll area can reach its last row
