Feature: SvelteKit served by Go

  Scenario: Home page renders a Svelte component through Go
    Given I open "/"
    Then the document response came from skgo
    And every part of the page loaded
    And I see the greeting component

  Scenario: Client-side navigation does not reload the document
    Given I open "/"
    When I click the link to "/items/42"
    Then I see "Item 42"
    And exactly 1 document request was made

  Scenario: Deep link to a dynamic route
    Given I open "/items/7"
    Then I see "Item 7"
