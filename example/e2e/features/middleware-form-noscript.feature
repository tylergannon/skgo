Feature: Middleware-authenticated forms work without JavaScript

  Scenario: A native form posted with no script is answered through the middleware
    Given the browser holds the stale visit cookie "stale-token"
    When I open "/middleware"
    Then the document was authenticated by middleware as "visit-token-7"
    When I save the middleware note with native form
    Then the middleware answered the native submission
    And the page shows the receipt "Noted remember-this for visit-token-7"
