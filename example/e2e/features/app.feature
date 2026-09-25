Feature: SvelteKit served by Go

  Scenario: Client-side navigation does not reload the document
    Given I open "/"
    When I click the link to "/items/42"
    Then I see "Item 42"
    And exactly 1 document request was made
