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
    there — not the list, and not the count above it. The count is checked
    against the rows on the page, two answers from two endpoints; a count that
    is ever one more than the rows is the leak.

    Given I open "/todos"
    When the todo list has loaded
    Then I do not see the todo "ship the private roadmap"
    And the todo count is the number of todos on the page
    When I sign in as "ada"
    Then I see the todo "ship the private roadmap"
    And the todo count is the number of todos on the page
    When I sign out
    Then I do not see the todo "ship the private roadmap"
    And the todo count is the number of todos on the page

  Scenario: The todo count counts only the todos the visitor can see
    The store's fixtures are four todos anyone may see and one private one, and
    this browser's list is its own, so the count is 4 signed out and 5 signed
    in whatever else the suite is doing.

    Given I open "/todos"
    When the todo list has loaded
    Then the live count is 4
    And I do not see the todo "ship the private roadmap"
    And the todo count is the number of todos on the page
    And every part of the page loaded
    When I sign in as "ada"
    Then I see the todo "ship the private roadmap"
    And the live count is 5
    And the todo count is the number of todos on the page
    When I reload the page
    And the todo list has loaded
    Then I see the todo "ship the private roadmap"
    And the live count is 5
    And the todo count is the number of todos on the page
    And every part of the page loaded
    When I sign out
    Then I do not see the todo "ship the private roadmap"
    And the live count is 4
    And the todo count is the number of todos on the page
    And every part of the page loaded
