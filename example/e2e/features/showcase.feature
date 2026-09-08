Feature: The front page says what skgo can do

  A visitor who has never seen skgo lands on `/`. What they need there is not a
  welcome, it is the list of things this app proves: one entry per capability,
  linking to the page that demonstrates it, and a sentence saying what to look
  for once that page is open.

  The list below is written here rather than read off the page. A scenario that
  asked the page what it contains would pass over a page containing nothing, and
  would go on passing after somebody deleted an entry.

  Scenario: The front page indexes every capability this app demonstrates
    Given I open "/"
    Then the front page lists exactly these capabilities, in this order:
      | capability                                          | path                        |
      | Pages rendered in the Go process                    | /items/42                   |
      | Route parameters                                    | /items/7                    |
      | Rest parameters                                     | /docs/guide/getting-started |
      | Remote functions: queries, commands and forms       | /todos                      |
      | Refreshing a query after a command                  | /todos/gate                 |
      | Server loads, section-wide                          | /account                    |
      | A nested error page                                 | /account/statement          |
      | Values a load promises but does not have yet        | /stream                     |
      | Custom types that keep their methods                | /pricing                    |
      | A form that works with JavaScript switched off      | /contact                    |
      | HTTP endpoints written in Go                        | /api                        |
      | A page with no client-side JavaScript               | /plain                      |
      | A page rendered only in the browser                 | /spa                        |
      | An empty list is still a list                       | /empty                      |
      | What the renderer writes reaches Go's log           | /console                    |
      | A load that refuses                                 | /error/expected             |
      | A failure no error page can catch                   | /?boom=root-layout          |
    And every entry says what to look for

  Scenario Outline: An entry opens the page it names
    Signed in, because two of the entries point into a section that turns a
    signed-out visitor away, and the index is the same page either way.

    Given I have signed in as "ada"
    And I visit "/"
    When I open the capability "<capability>"
    Then I see "<heading>"
    And every part of the page loaded

    Examples:
      | capability                                     | heading            |
      | Pages rendered in the Go process               | Item 42            |
      | Route parameters                               | Item 7             |
      | Rest parameters                                | Docs               |
      | Remote functions: queries, commands and forms  | Todos              |
      | Refreshing a query after a command             | The refresh gate   |
      | Server loads, section-wide                     | Overview           |
      | A nested error page                            | Account error 402  |
      | Values a load promises but does not have yet   | Stream             |
      | Custom types that keep their methods           | Pricing            |
      | A form that works with JavaScript switched off | Contact            |
      | HTTP endpoints written in Go                   | API                |
      | A page with no client-side JavaScript          | Plain              |
      | A page rendered only in the browser            | SPA                |
      | An empty list is still a list                  | Empty              |
      | What the renderer writes reaches Go's log      | Console            |
      | A load that refuses                            | Error 418          |

  @prod
  Scenario: The entry for a failure no error page can catch is kit's static error page
    The root layout is the outermost node of every branch, so nothing above it
    can render an `+error.svelte` for it. Kit's answer to a load that fails
    there is the `error.html` the build carries: a whole document with no app in
    it and no script, so nothing boots and nothing tries again.

    src/routes/layout.server.go refuses with 503 when the URL says
    `boom=root-layout`, and refuses for no other reason.

    `@prod`, because the claim is about a document Go rendered. In dev the root
    layout turns SSR off, the document is kit's shell, and the refusal reaches
    the browser in `__data.json` instead.

    Given I have signed in as "ada"
    And I visit "/"
    When I open the capability "A failure no error page can catch"
    Then the document was answered with 503
    And the document is kit's static error page saying 503 and "The root layout could not reach the database"

  @prod
  Scenario: Every other page carries the root layout's own load
    The same load, on the way through. It runs for every page in the app
    because the root layout is in every branch, and "skgo example" is a string
    no remote function in this app answers with.

    Given I open "/stream"
    Then the document already said "skgo example"
    And the app says it is deployed as "skgo example"
