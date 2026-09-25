Feature: Kit reserves its own query parameter names

  Scenario: An application page cannot use a query parameter reserved by Kit
    Given I open "/?x-sveltekit-private=1"
    Then the document response status was 400
    And the document displays the reserved query parameter error for "x-sveltekit-private"
