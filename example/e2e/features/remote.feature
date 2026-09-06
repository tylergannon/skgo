Feature: Remote functions written in Go

  Every function in `src/routes/todos/todos.remote.ts` is generated from
  `todos.remote.go` and every body throws, so anything that renders below
  proves the Go server answered.

  Scenario: The todo list is served by Go
    Given I open "/todos"
    Then the document response came from skgo in the expected mode
    And I see the todo "write the adapter"
    And I see the todo "serve remote functions"

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

  Scenario: The live count grows when a todo is added
    Given I open "/todos"
    When the todo list has loaded
    And I note the live count
    And I add the todo "watch the live count"
    Then the live count increased by 1

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
