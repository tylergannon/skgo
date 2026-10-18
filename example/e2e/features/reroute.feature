Feature: Go owns universal reroute selection

  Scenario: Kit navigates to a Go-selected route while keeping its alias URL
    Given I open "/rerouted/42?receipt=direct"
    Then the reroute receipt is "page|route=/rerouted/[number=Order]|param=Order #42|url=/rerouted/42?receipt=direct"
    When I follow the "Go alias" link
    Then the reroute receipt is "page|route=/rerouted/[number=Order]|param=Order #73|url=/go-alias/page?receipt=alias"
    And the reroute URL is "/go-alias/page?receipt=alias"
    When I follow the "Direct route" link
    Then the reroute receipt is "page|route=/rerouted/[number=Order]|param=Order #42|url=/rerouted/42?receipt=direct"
    When I follow the "Go alias" link
    Then the reroute receipt is "page|route=/rerouted/[number=Order]|param=Order #73|url=/go-alias/page?receipt=alias"
    And the reroute URL is "/go-alias/page?receipt=alias"
    And exactly 1 document request was made
