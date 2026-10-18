@dev-session
Feature: Route edits while a development load waits for Go I/O

  The Go integration fixture starts the real development coordinator and Vite.
  It holds a Go load on network I/O and owns the source edits and release.

  Scenario: A route structure changes during pending Go I/O
    Given a development page is waiting for a Go network response
    Then an independent Go endpoint responds while the page waits
    When the route and layout structure changes before releasing the Go load
    Then the held page finishes with its Go data
    And the changed route has its new layout and hydrates
