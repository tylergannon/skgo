Feature: Classic actions share Kit routes with Go endpoints and remote forms

  Which handler a POST to a shared route reaches — the classic action, the
  sibling endpoint, or the remote form — is decided by headers, and those
  decisions are asserted in example/action_protocol_test.go. What stays here is
  what the page does with the answers once Kit's client has them.

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
      | native form     |

  Scenario: Kit-enhanced remote and classic forms keep separate protocols
    Given I open "/actions"
    Then the shared route offers classic and remote forms
    When I submit the enhanced remote form
    Then its remote Go receipt appears from the remote protocol
    When I save Grace through the classic form with Kit enhancement
    Then the page shows the classic Grace receipt and saved profile
