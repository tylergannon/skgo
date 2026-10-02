Feature: The document follows the source being served

  A production server renders the frontend build embedded in its binary. A
  development server renders the modules the running Vite server transformed.
  Editing a component therefore changes the open page and the next development
  document without rebuilding, while the already-built production document
  remains unchanged.

  These scenarios run after the read-only browser tour because adding a route
  makes Kit deliberately reload every open dev client while it rewrites that
  client's generated route graph.

  Scenario: A source edit reaches the open page and the next document in dev, and neither in prod
    Given I open "/"
    Then the document already said the page's own heading is "Home"
    When "src/routes/+page.svelte" has "Home" replaced with "Home, hot updated"
    Then the open page hot-updates to "Home, hot updated" in dev or stays at built heading "Home" without reloading
    And a new document for "/" carries "Home, hot updated" from live source or "Home" from its build

  Scenario: A dependency edited while its importer's bytes stay the same reaches the next document and Kit's navigation in dev, and neither in prod
    Given I open "/go-dev"
    Then a cold document for "/go-dev" has "go-dev-dependency" reading "Dependency revision one" in dev or "Dependency revision one" in prod
    When "src/lib/go-dev-dependency.ts" has "Dependency revision one" replaced with "Dependency revision two"
    Then a cold document for "/go-dev" has "go-dev-dependency" reading "Dependency revision two" in dev or "Dependency revision one" in prod
    And client navigation from "/" to "/go-dev" shows "go-dev-dependency" as "Dependency revision two" in dev or "Dependency revision one" in prod

  Scenario: An authored Go load edited while the servers run reaches the next document and Kit's navigation in dev, and neither in prod
    Given I open "/go-dev"
    Then a cold document for "/go-dev" has "go-dev-load" reading "Go revision one" in dev or "Go revision one" in prod
    When "src/routes/go-dev/page.server.go" has "Go revision one" replaced with "Go revision two"
    Then a cold document for "/go-dev" has "go-dev-load" reading "Go revision two" in dev or "Go revision one" in prod
    And client navigation from "/" to "/go-dev" shows "go-dev-load" as "Go revision two" in dev or "Go revision one" in prod

  Scenario: A remote whose Go result type changes is regenerated and invoked by Kit with the new shape in dev, and unchanged in prod
    Given I open "/go-dev"
    Then a cold document for "/go-dev" has "go-dev-remote" reading "\"Go wire revision one\"" in dev or "\"Go wire revision one\"" in prod
    When "src/routes/go-dev/go-dev.remote.go" is replaced by the fixture "go-dev/go-dev.remote.go.txt"
    Then the generated file "src/routes/go-dev/go-dev.remote.ts" contains "Revision" in dev or "(): string" in prod
    And the generated file "src/routes/go-dev/types.ts" contains "message: string;" in dev or does not exist in prod
    And a cold document for "/go-dev" has "go-dev-remote" reading "{\"message\":\"Go wire revision two\",\"revision\":2}" in dev or "\"Go wire revision one\"" in prod
    And client navigation from "/" to "/go-dev" shows "go-dev-remote" as "{\"message\":\"Go wire revision two\",\"revision\":2}" in dev or "\"Go wire revision one\"" in prod

  Scenario: A route whose Go load and component did not exist at launch is served, navigated to and removed again
    Given I open "/go-dev"
    Then a cold request for "/go-dev-added" is answered 404 in dev and 404 in prod
    When the route "go-dev-added" is created from the fixture directory "go-dev-added"
    Then a cold request for "/go-dev-added" is answered 200 in dev and 404 in prod
    And in dev a cold document for "/go-dev-added" has "go-dev-added-load" reading "New Go route revision one"
    And in dev client navigation from "/go-dev" to "/go-dev-added" shows "go-dev-added-load" as "New Go route revision one"
    When the route "go-dev-added" is removed
    Then a cold request for "/go-dev-added" is answered 404 in dev and 404 in prod

  Scenario: A Go error is reported with its file and the same session recovers when it is corrected
    Given I open "/go-dev"
    When "src/routes/go-dev/page.server.go" has "PageData{Revision: \"Go revision one\"}" replaced with "PageData{Revision: 7}"
    Then a request for "/go-dev" is answered by the build failure naming "page.server.go" in dev or by "go-dev-load" reading "Go revision one" in prod
    When "src/routes/go-dev/page.server.go" has "PageData{Revision: 7}" replaced with "PageData{Revision: \"Go revision recovered\"}"
    Then a cold document for "/go-dev" has "go-dev-load" reading "Go revision recovered" in dev or "Go revision one" in prod

  Scenario: An authored Go action edited while the servers run reaches Kit's enhanced form and a native form with scripting disabled in dev, and neither in prod
    Given I open "/go-dev"
    Then the enhanced Go development form answers "Go action revision one" in dev or "Go action revision one" in prod
    And the native Go development form with scripting disabled answers "Go action revision one" in dev or "Go action revision one" in prod
    When "src/routes/go-dev/page.server.go" has "Go action revision one" replaced with "Go action revision two"
    Then the enhanced Go development form answers "Go action revision two" in dev or "Go action revision one" in prod
    And the native Go development form with scripting disabled answers "Go action revision two" in dev or "Go action revision one" in prod

  Scenario: An authored Go endpoint's status, body and header edited while the servers run reach the composed handler and the hydrated page's fetch in dev, and not in prod
    Given I open "/go-dev"
    Then the endpoint "/go-dev/endpoint" answers 200 "Go endpoint revision one" with header "one" in dev or 200 "Go endpoint revision one" with header "one" in prod
    When "src/routes/go-dev/endpoint/server.go" is replaced by the fixture "go-dev/endpoint.server.go.txt"
    Then the endpoint "/go-dev/endpoint" answers 202 "Go endpoint revision two" with header "two" in dev or 200 "Go endpoint revision one" with header "one" in prod
    And the hydrated page's fetch of the endpoint reads status 202 body "Go endpoint revision two" header "two" in dev or status 200 body "Go endpoint revision one" header "one" in prod

  Scenario: A server route and its Go handler that did not exist at launch are served and removed again
    Given I open "/go-dev"
    Then a cold request for "/go-dev-endpoint" is answered 404 in dev and 404 in prod
    When the route "go-dev-endpoint" is created from the fixture directory "go-dev-endpoint"
    Then in dev the endpoint "/go-dev-endpoint" answers 200 "New Go endpoint revision one" with header "new-one"
    And a cold request for "/go-dev-endpoint" is answered 200 in dev and 404 in prod
    When the route "go-dev-endpoint" is removed
    Then a cold request for "/go-dev-endpoint" is answered 404 in dev and 404 in prod

  Scenario: A template marker edited while the servers run reaches the next dev document, and not the build prod embeds
    Given I open "/go-dev"
    Then a cold document for "/go-dev" has the template marker "Go template revision one" in dev or "Go template revision one" in prod
    When "src/app.html" has "Go template revision one" replaced with "Go template revision two"
    Then a cold document for "/go-dev" has the template marker "Go template revision two" in dev or "Go template revision one" in prod

  Scenario: Page options written while the servers run are honored by the next dev document and by Kit's client, and not by prod's build
    Given I open "/go-dev"
    When the file "src/routes/go-dev/+page.ts" is written from the fixture "go-dev/csr-false.page.ts.txt"
    Then a cold document for "/go-dev" has no script in dev or a script in prod
    And the enhanced Go development form submits as a document in dev or as a Kit fetch in prod
    When the file "src/routes/go-dev/+page.ts" is written from the fixture "go-dev/ssr-false.page.ts.txt"
    Then a cold document for "/go-dev" has no "go-dev-load" markup in dev or has it in prod
    And Kit renders "go-dev-load" as "Go revision one" from the data Go answers
