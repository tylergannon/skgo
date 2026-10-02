Feature: Go middleware wraps the whole request

  The example's Go middleware runs as kit's `handle`: it refreshes a visit
  cookie before anything answers, so the load that runs next reads the token
  from the same cookie the visitor receives, and it marks the response it gets
  back with the route it matched. The bytes themselves are asserted in
  example/middleware_test.go; these scenarios need kit's client to have acted.

  Scenario: Middleware authenticates a cold document and kit's client carries on
    Given the browser holds the stale visit cookie "stale-token"
    When I open "/middleware"
    Then the document was authenticated by middleware as "visit-token-7"
    And the browser now holds the visit cookie "visit-token-7"
    And the document was transformed by middleware and kit still hydrated it
    When I follow the middleware link to "alpha"
    Then the page shows slug "alpha" authenticated as "visit-token-7" from a data request
    And the data response was marked "/middleware/[slug] data=true"
    And exactly 1 document request was made

  Scenario: An enhanced form is answered through the middleware
    Given the browser holds the stale visit cookie "stale-token"
    When I open "/middleware"
    Then the document was authenticated by middleware as "visit-token-7"
    When I save the middleware note with Kit enhancement
    Then the middleware answered the enhanced submission
    And the page shows the receipt "Noted remember-this for visit-token-7"
