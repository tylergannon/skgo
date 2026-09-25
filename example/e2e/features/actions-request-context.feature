Feature: Page actions preserve their request context

  Scenario Outline: A data action's cookie is visible in the following Go load with <submission>
    Given I open "/actions"
    Then the Actions editor shows the Ada fixture
    When I remember the action cookie with <submission>
    Then the submitting response and page show the immediate cookie
    And a later editor GET still shows the literal cookie and Ada fixture

    Examples:
      | submission      |
      | Kit enhancement |

  Scenario Outline: The handle hook refuses a guarded edit with <submission>
    Given I open "/actions"
    Then the Actions editor shows the Ada fixture
    When I attempt the guarded forbidden edit with <submission>
    Then the hook refusal has Kit's response and visible page behavior
    And a later editor GET still has every Ada fixture field

    Examples:
      | submission      |
      | native form     |
