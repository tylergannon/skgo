Feature: A Go command refreshes a query it names

  `retitleTodo` is a Go command. The page that calls it passes no
  `updates(...)`, so the browser never asks for anything to be refreshed —
  which means anything that changes on screen after the command landed changed
  because `skgo.Refresh(ctx, getTodo, id)` in Go named the query function and
  the argument, and the new value rode back on the command's own response.

  The page shows two instances of one query, `getTodo("p1")` and
  `getTodo("p2")`. They differ only by their argument, which is the cache key,
  so which of the two panels moves is the whole question.

  Scenario: The refreshed value rides back on the command's own response
    Given I open "/todos/pair"
    And the pair of todos has loaded
    When I note the remote request count
    And I retitle "p1" to "refreshed from Go, in one flight" and refresh "p1"
    Then the pair panel "p1" shows "refreshed from Go, in one flight"
    And exactly 1 remote request was made since
    And every part of the page loaded

  Scenario: The argument decides which panel is told anything
    Given I open "/todos/pair"
    And the pair of todos has loaded
    When I retitle "p1" to "left, as this scenario set it" and refresh "p1"
    And I retitle "p2" to "right, as this scenario set it" and refresh "p2"
    Then the pair panel "p1" shows "left, as this scenario set it"
    And the pair panel "p2" shows "right, as this scenario set it"

    When I retitle "p2" to "right, changed behind the page's back" and refresh "p1"
    Then the pair panel "p2" still shows "right, as this scenario set it"
    And the pair panel "p1" still shows "left, as this scenario set it"

    When I note the remote request count
    And I retitle "p1" to "left, as this scenario set it" and refresh "p2"
    Then the pair panel "p2" shows "right, changed behind the page's back"
    And the pair panel "p1" still shows "left, as this scenario set it"
    And exactly 1 remote request was made since
    And every part of the page loaded
