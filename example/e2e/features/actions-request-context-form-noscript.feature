Feature: Page action request context survives without JavaScript

  Scenario: A native data action's cookie reaches the same document load
    Given I open "/actions"
    Then the Actions editor shows the Ada fixture
    When I remember the action cookie with native form
    Then the submitting response and page show the immediate cookie
    And a later editor GET still shows the literal cookie and Ada fixture

  Scenario: A native save carries its response header
    Given I open "/actions"
    Then the Actions editor shows the Ada fixture
    When I save Grace and inspect the action header with native form
    Then the response has the fixed header and the page has Grace's saved profile

  Scenario: A native hook redirect stops the guarded edit
    Given I open "/actions"
    Then the Actions editor shows the Ada fixture
    When I attempt the guarded sign-in with native form
    Then the hook redirect has Kit's response and the sign-in destination
    And a later editor GET still has every Ada fixture field

  Scenario: A native hook error renders the fatal page
    Given I open "/actions"
    Then the Actions editor shows the Ada fixture
    When I attempt the guarded forbidden edit with native form
    Then the hook refusal has Kit's response and visible page behavior
    And a later editor GET still has every Ada fixture field
