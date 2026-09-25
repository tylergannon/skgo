Feature: Page actions preserve their request context

  Each scenario submits through Kit's `enhance` and asserts what Kit's client
  did with the answer. The native document answers to the same four requests
  are asserted in example/contracts_test.go, and posted by a browser with no
  script at all in actions-request-context-form-noscript.feature.

  Scenario: A data action's cookie is visible in the following Go load with Kit enhancement
    Given I open "/actions"
    Then the Actions editor shows the Ada fixture
    When I remember the action cookie with Kit enhancement
    Then the submitting response and page show the immediate cookie
    And a later editor GET still shows the literal cookie and Ada fixture

  Scenario: A successful save carries its response header with Kit enhancement
    Given I open "/actions"
    Then the Actions editor shows the Ada fixture
    When I save Grace and inspect the action header with Kit enhancement
    Then the response has the fixed header and the page has Grace's saved profile

  Scenario: The handle hook redirects a guarded edit with Kit enhancement
    Given I open "/actions"
    Then the Actions editor shows the Ada fixture
    When I attempt the guarded sign-in with Kit enhancement
    Then the hook redirect has Kit's response and the sign-in destination
    And a later editor GET still has every Ada fixture field

  Scenario: The handle hook refuses a guarded edit with Kit enhancement
    Given I open "/actions"
    Then the Actions editor shows the Ada fixture
    When I attempt the guarded forbidden edit with Kit enhancement
    Then the hook refusal has Kit's response and visible page behavior
    And a later editor GET still has every Ada fixture field
