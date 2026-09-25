Feature: Classic actions share Kit routes with Go endpoints and remote forms

  Scenario Outline: A classic receipt and an endpoint answer come from the same route with <submission>
    Given I open "/actions"
    Then the shared route offers classic and remote forms
    When I save Grace through the classic form with <submission>
    Then the page shows the classic Grace receipt and saved profile
    When I ask the sibling endpoint at that route
    Then the endpoint answer is distinct and the classic receipt remains

    Examples:
      | submission      |
      | Kit enhancement |

  Scenario: Native remote selection wins, while a JSON action request chooses the classic remote-named action
    Given I open "/actions"
    Then the shared route offers classic and remote forms
    When I submit the native remote form
    Then the remote Go receipt appears without a classic receipt
    And a JSON action POST to that same selector reaches the classic remote-named action

  Scenario: Kit-enhanced remote and classic forms keep separate protocols
    Given I open "/actions"
    Then the shared route offers classic and remote forms
    When I submit the enhanced remote form
    Then its remote Go receipt appears from the remote protocol
    When I save Grace through the classic form with Kit enhancement
    Then the page shows the classic Grace receipt and saved profile
