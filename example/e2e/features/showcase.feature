Feature: The front page says what skgo can do

  A visitor who has never seen skgo lands on `/`. What they need there is not a
  welcome, it is the list of things this app proves: one entry per capability,
  linking to the page that demonstrates it, and a sentence saying what to look
  for once that page is open.

  The index itself — every entry, in order, with its link and its sentence — is
  in the document Go renders, and example/contracts_test.go asserts it against a
  list written there. What stays here is that each entry, followed through kit's
  client router, lands on the page it names with every part of that page loaded.

  It is meant to be exhaustive, so what it leaves out is worth writing down. A
  page that is a second view of a capability already listed is not an entry of
  its own: /todos/[id] and /account/orders are read by the entries above them,
  and /error/render makes the same claim as "A failure no error page can catch"
  by a different route. /error/command and /error/redirect are kit's rules about
  what a render may do rather than things skgo offers a developer. /about is not
  listed because the app cannot prerender while its root layout has a Go load —
  see the PR that added this page, and #81.

  Scenario Outline: An entry opens the page it names
    Signed in, because two of the entries point into a section that turns a
    signed-out visitor away, and the index is the same page either way.

    Given I have signed in as "ada"
    And I visit "/"
    When I open the capability "<capability>"
    Then I see "<heading>"
    And every part of the page loaded

    Examples:
      | capability                                             | heading           |
      | Pages rendered in the Go process                       | Item 42           |
      | Route parameters                                       | Item 7            |
      | Rest parameters                                        | Docs              |
      | Remote functions: queries, commands and forms          | Todos             |
      | Refreshing a query after a command                     | The refresh gate  |
      | One query, one answer per argument                     | A pair of todos   |
      | A query that goes on answering                         | Live              |
      | Many calls answered by one                             | Batch             |
      | Server loads, section-wide                             | Overview          |
      | A nested error page                                    | Account error 402 |
      | Values a load promises but does not have yet           | Stream            |
      | Custom types that keep their methods                   | Pricing           |
      | A form that works with JavaScript switched off         | Contact           |
      | Page form actions                                       | Actions           |
      | HTTP endpoints written in Go                           | API               |
      | Links and asset URLs worked out while the page renders | Render-time paths |
      | A page with no client-side JavaScript                  | Plain             |
      | A page rendered only in the browser                    | SPA               |
      | An empty list is still a list                          | Empty             |
      | What the renderer writes reaches Go's log              | Console           |
      | A load that refuses                                    | Error 418         |
      | An error that is a bug says nothing about itself       | Error 500         |

  Scenario: The entry for a failure the page catches itself keeps the rest of the page
    The one error path where nothing is replaced. The query refuses, the
    boundary standing beside it renders what the error said, and the page it
    is on carries on — which is the difference between this entry and the two
    below it, where a whole page goes away.

    Given I have signed in as "ada"
    And I visit "/"
    When I open the capability "A failure the page catches itself"
    Then I see "Sensor"
    And I see the words "The sensor is being calibrated"

  Scenario: Client-side navigation shows what the handleError hook chose
    src/routes/error/unexpected/page.server.go fails with an ordinary Go error
    reading "the connection string is postgres://ada:hunter2@db". The expected
    message and support id below are literals from example.HandleError, not
    values read from the document path. Starting on the index and following its
    link proves exactly one `__data.json` response carried that choice.

    Given I have signed in as "ada"
    And I visit "/"
    And I see "Home"
    And I note the data request count
    When I open the capability "An error that is a bug says nothing about itself"
    Then I see "Error 500"
    And the error message is "Something went wrong on our end."
    And the error page shows the support id "case-1121"
    And exactly 1 data request was made since
    And the page never mentions "hunter2"
    And the page never mentions "postgres://ada"
