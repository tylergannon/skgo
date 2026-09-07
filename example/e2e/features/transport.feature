Feature: Custom types keep their behaviour across the wire

  A domain type is not a bag of fields. `Money` knows how to write itself as a
  price, and a page that shows a price is calling that method rather than
  reading a property. Kit's `transport` hook is what lets it: the app declares
  an encode/decode pair, Go's half in `src/hooks.go` and the browser's in
  `src/hooks.ts`, and the value crosses as itself instead of as a plain object.

  Every amount below is written into the scenario as the price a person reads.
  Nothing on the wire is spelled that way — Go sends a whole number of cents —
  so a page can only show it by having called the method.

  Scenario: A price arrives in the browser as something with methods
    Given I open "/pricing"
    Then every part of the page loaded
    And the plan "Hobby" costs "$0.00"
    And the plan "Team" costs "$20.00"
    And the plan "Enterprise" costs "$200.00"

  Scenario: A price the browser builds reaches Go as the Go type
    Given I open "/pricing"
    When I ask Go about a price of "$20.00"
    Then Go says it heard "$20.00"
    And Go says double that is "$40.00"
