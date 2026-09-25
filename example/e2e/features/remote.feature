Feature: Remote functions written in Go

  Every function in `src/routes/todos/todos.remote.ts` is generated from
  `todos.remote.go` and every body throws, so anything that renders below
  proves the Go server answered.

  Each browser is its own visitor with its own list, seeded from the store's
  fixtures: four todos anyone may see and one private one. So a visitor who
  has not signed in starts every scenario on a count of 4, and every number
  below is that fixture plus what the scenario itself added.

  Scenario: Adding a todo refreshes the list in a single flight
    Given I open "/todos"
    When the todo list has loaded
    And I note the remote request count
    And I add the todo "prove single flight"
    Then I see the todo "prove single flight"
    And exactly 1 remote request was made since

  Scenario: A deep link passes the query argument to Go
    Given I open "/todos/t2"
    Then the todo detail shows "serve remote functions"

  Scenario: Commands drive the live count, one todo at a time
    Given I open "/todos"
    When the todo list has loaded
    Then the live count is 4
    When I add the todo "the first of two"
    Then the live count is 5
    And the todo count still matches the todos on the page
    And every part of the page loaded
    When I add the todo "the second of two"
    Then the live count is 6
    And the todo count still matches the todos on the page
    And every part of the page loaded

  Scenario: Renaming a todo updates the open page without a second request
    Given I open "/todos"
    When the todo list has loaded
    And I add the todo "rename me"
    And I open the todo "rename me"
    And the todo detail has loaded
    And I note the remote request count
    And I rename the open todo to "renamed in one flight"
    Then the todo detail shows "renamed in one flight"
    And exactly 1 remote request was made since
