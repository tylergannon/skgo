Feature: Fetches that reach the app's own rendered pages

  A universal load fetches a whole rendered page of the app, and a Go load
  fetches one of every kind of destination. In the document the page being
  rendered waits on a second render nested inside it. The values below are
  fixed in the scenarios, not read off the app, and the same claims run
  against the built bundle and `vp dev`. (In dev nothing is prerendered, so
  the shadow route's dynamic half is a prod-only claim, made by the Go tests.)

  Scenario: A universal load's fetch of a rendered page is in the cold document, survives hydration and a Kit navigation
    Given I have signed in as "ada"
    When I visit "/nested-universal"
    Then the document already said "harbour-lamp-4096"
    And the nested page shows "harbour-lamp-4096" for "ada" with status 200
    When I follow the "Home" link
    Then I see "Home"
    When I follow the "Nested fetch" link
    Then the nested page shows "harbour-lamp-4096" for "ada" with status 200

  Scenario: A Go load's fetches of every kind of destination are in the cold document and again after a Kit navigation
    When I visit "/destinations"
    Then the document already said 'data-path="/shadow/fixed">200 text/html; charset=utf-8'
    And the destinations page lists these answers:
      | path                                                   | answer                          |
      | /request-fetch                                         | 200 text/html; charset=utf-8    |
      | /request-fetch/__data.json?x-sveltekit-invalidated=01  | 200 application/json            |
      | /_app/remote/4cga8b/whoami                             | 200 application/json            |
      | /robots.txt                                            | 200 text/plain                  |
      | /prerender/atlas                                       | 200 text/html; charset=utf-8    |
      | /_app/remote/ks8sip/buildReceipt/WyJhdGxhcyJd          | 200 application/json            |
      | /shadow/fixed                                          | 200 text/html; charset=utf-8    |
      | /api/request-fetch                                     | 200 application/json            |
    When I follow the "Home" link
    Then I see "Home"
    When I note the data request count
    And I follow the "Destinations" link
    Then the destinations page lists these answers:
      | path                                                   | answer                          |
      | /request-fetch                                         | 200 text/html; charset=utf-8    |
      | /shadow/fixed                                          | 200 text/html; charset=utf-8    |
      | /api/request-fetch                                     | 200 application/json            |
    And exactly 1 data requests were made since
