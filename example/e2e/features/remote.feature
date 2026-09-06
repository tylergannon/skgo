Feature: Remote functions answered by Go

  Every function body in `src/lib/todos.remote.ts` throws, so anything that
  renders below proves the Go server answered `/_app/remote/worolc/...`.

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
