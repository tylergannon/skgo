Feature: A Go action can answer a form posted from another page

  Scenario Outline: Cross-page <outcome> reaches its Go destination with <submission>
    Given I open the cross-page action sender
    Then the sender offers the destination actions
    When I submit cross-page <outcome> with <submission>
    Then the cross-page <outcome> destination shows its exact outcome
    And the cross-page <outcome> fixture remains correct on a later GET

    Examples:
      | outcome     | submission      |
      | success     | native form     |
      | validation  | Kit enhancement |
