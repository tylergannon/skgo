Feature: Signing in

  The session cookie is written by a Go `command` and read by Go `query`
  functions, under the rules SvelteKit places on its own remote functions:
  only a command may write a cookie.

  Scenario: A visitor signs in and the session survives a reload
    Given I open "/todos"
    When I sign in as "ada"
    Then I am signed in as "ada"
    And every part of the page loaded
    When I reload the page
    Then I am signed in as "ada"
    And every part of the page loaded

  Scenario: Signing out ends the session
    Given I open "/todos"
    When I sign in as "grace"
    And I sign out
    Then I am signed out
    And every part of the page loaded
    When I reload the page
    Then I am signed out
    And every part of the page loaded

  Scenario: A signed-in visitor sees a todo a signed-out visitor cannot
    Nothing on the page may tell a signed-out visitor that the private todo is
    there — not the list, and not the count above it. The count is pinned to
    the list rather than to a literal because the list grows as the suite runs;
    a count that is ever one more than the rows is the leak.

    Given I open "/todos"
    When the todo list has loaded
    Then I do not see the todo "ship the private roadmap"
    And the live count is the number of todos I can see
    When I sign in as "ada"
    Then I see the todo "ship the private roadmap"
    And the live count is the number of todos I can see
    When I sign out
    Then I do not see the todo "ship the private roadmap"
    And the live count is the number of todos I can see
