Feature: Cross-page Go actions work without JavaScript

  Scenario Outline: Native cross-page <outcome> works without JavaScript
    Given I open the cross-page action sender
    Then the sender offers the destination actions
    When I submit cross-page <outcome> with native form
    Then the cross-page <outcome> destination shows its exact outcome
    And the cross-page <outcome> fixture remains correct on a later GET

    Examples:
      | outcome     |
      | success     |
