Feature: A Go load that fetches through its request event

  The page's Go load calls the Go endpoint /api/request-fetch with
  Event.Fetch and the endpoint's answer is what the page shows. The value is
  fixed in the scenario, not read off the app. The same claims run against the
  built bundle and `vp dev`.

  Scenario: The fetched value is in the document and again after a Kit navigation
    Given I have signed in as "ada"
    When I visit "/request-fetch"
    Then the document already said "harbour-lamp-4096"
    And the request-fetch page shows "harbour-lamp-4096" fetched as "ada"
    When I follow the "Home" link
    Then I see "Home"
    When I note the data request count
    And I follow the "Request fetch" link
    Then the request-fetch page shows "harbour-lamp-4096" fetched as "ada"
    And exactly 1 data requests were made since
