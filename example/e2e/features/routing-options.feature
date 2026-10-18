Feature: Kit selects optional and matched Go routes

  Scenario: Optional matcher values can be omitted during Kit navigation
    Given I open "/routing/optional"
    Then the routing receipt is "optional|omitted"
    When I follow the "Optional order 42" link
    Then the routing receipt is "optional|Order #42"
    And the routing URL is "/routing/optional/42"
    When I follow the "Omitted order" link
    Then the routing receipt is "optional|omitted"
    And the routing URL is "/routing/optional"
    And exactly 1 document request was made

  Scenario: A rejected matcher selects the next route during Kit navigation
    Given I open "/routing/optional/42"
    Then the routing receipt is "optional|Order #42"
    When I follow the "Matched order 7" link
    Then the routing receipt is "matched|Order #7"
    And the routing URL is "/routing/fallback/7"
    When I follow the "Rejected order fallback" link
    Then the routing receipt is "fallback|no-order"
    And the routing URL is "/routing/fallback/no-order"
    And exactly 1 document request was made

  Scenario: Inherited and overridden slash policies survive Kit navigation
    Given I open "/routing/slash/always/"
    Then the routing receipt is "always|/routing/slash/always/"
    When I follow the "Never slash" link
    Then the routing receipt is "never|/routing/slash/never"
    And the routing URL is "/routing/slash/never"
    When I follow the "Ignore slash" link
    Then the routing receipt is "ignore|/routing/slash/ignore/"
    And the routing URL is "/routing/slash/ignore/"
    When I follow the "Always slash" link
    Then the routing receipt is "always|/routing/slash/always/"
    And the routing URL is "/routing/slash/always/"
    And exactly 1 document request was made
