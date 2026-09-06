Feature: Signing in

  The session cookie is written by a Go `command` and read by Go `query`
  functions, under the rules SvelteKit places on its own remote functions:
  only a command may write a cookie.

  Scenario: A visitor signs in and the session survives a reload
    Given I open "/todos"
    When I sign in as "ada"
    Then I am signed in as "ada"
    When I reload the page
    Then I am signed in as "ada"

  Scenario: Signing out ends the session
    Given I open "/todos"
    When I sign in as "grace"
    And I sign out
    Then I am signed out
    When I reload the page
    Then I am signed out

  Scenario: A signed-in visitor sees a todo a signed-out visitor cannot
    Given I open "/todos"
    When the todo list has loaded
    Then I do not see the todo "ship the private roadmap"
    When I sign in as "ada"
    Then I see the todo "ship the private roadmap"
    When I sign out
    Then I do not see the todo "ship the private roadmap"
