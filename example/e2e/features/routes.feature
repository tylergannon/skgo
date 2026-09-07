Feature: HTTP endpoints written in Go

  A route with a +server.ts is raw HTTP: no page, no devalue, no kit client.
  The handler is an ordinary Go function written in the route's own directory,
  and a caller gets back exactly what it wrote.

  Scenario: A page reads the endpoint and shows what it returned
    Given I open "/api"
    Then the document response came from skgo in the expected mode
    And the endpoint answered GET with 200 and "application/json"
    And the endpoint returned a todo saying "write the adapter"

  Scenario: A browser asking for the endpoint directly gets the endpoint
    Given I open "/api/todos"
    Then the document came from skgo in the expected mode
    And the document response status was 200
    And the document content type was "application/json"
    And the document is JSON listing a todo saying "serve remote functions"

  Scenario: The endpoint accepts a POST and keeps what it was given
    Given I open "/api"
    When I POST the todo "walk to the harbour"
    Then the endpoint answered POST with 201 and "application/json"
    And the endpoint returned a todo saying "walk to the harbour"

  Scenario: A method the route does not declare is refused, not answered with a page
    Given I open "/api"
    When I ask the endpoint to DELETE
    Then the endpoint answered DELETE with 405 and "text/plain"
    And the endpoint said the methods it allows are "GET, POST, HEAD"

  Scenario: A trailing slash is redirected rather than served twice
    When I request "/api/todos/" without following redirects
    Then the response status was 308
    And the redirect location was "../todos"

  Scenario: A prerendered page is the file the build wrote, not a fresh render
    Every other page is rendered when it is asked for, so it can say who is
    asking. A prerendered page was written before anyone was. The same visitor
    asks for one of each, and the difference between the two documents is the
    difference between the two mechanisms.

    Given I have signed in as "ada"
    When I visit "/"
    Then the document already said "Signed in as ada"
    When I visit "/about"
    Then the document came from skgo in the expected mode
    And the document response status was 200
    And the document never mentions "Signed in as ada"
    And I see "About"
