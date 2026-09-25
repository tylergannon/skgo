Feature: Native remote and classic forms share the Actions page without JavaScript

  Scenario: Both Go forms answer document POSTs without JavaScript
    Given I open "/actions"
    Then the shared route offers native forms with scripting disabled
    When I submit the native remote form
    Then the remote Go receipt appears without a classic receipt
    When I save Grace through the classic form with native form
    Then the page shows the classic Grace receipt and saved profile
