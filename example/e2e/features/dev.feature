@dev
Feature: In dev the document is Go's too, from the modules vite transformed

  `vp dev` never runs an adapter — kit reaches `adapt()` only from the plugin
  that finalises a build — so there is no SSR bundle in dev and there never will
  be. What there is instead is the same `goja` environment the build compiles
  the bundle in, declared in the dev server by the same adapter plugin, and
  vite's own `fetchModule` over it. Go asks for one transformed module at a time,
  evaluates it in the engine the built bundle runs in, and renders the document
  itself. The browser still only talks to Go: modules, their CSS, the files in
  `static/` and the HMR socket all go through to vite.

  So a developer sees the document Go will send in production, over the sources
  vite is serving, and an edit reaches it without a build. That is what this
  file is for, and every scenario in it is about something only dev can show.

  The fixtures are ssr.feature's: `getSite` in src/routes/site.remote.go answers
  with the name "skgo" and its own path, and the loads under /account answer with
  the signed-in visitor's name. The generated TypeScript beside each of those
  throws "skgo: implemented in Go", so a value in the document is proof Go
  answered.

  Scenario: An edit to a component reaches the document Go sends, with no build
    The claim is about the bytes, before a single line of JavaScript runs: the
    page is fetched again and read as text. A browser that hot-reloaded would
    show the edit whether or not Go had it, which is exactly what this
    distinguishes.

    Given I open "/"
    Then the document already said the page's own heading is "Home"
    When "src/routes/+page.svelte" has "Home" replaced with "Home, edited while running"
    Then the document Go sends for "/" has the page's heading "Home, edited while running"
    And that document no longer has the heading "Home"

  Scenario: A component that can only fail on the server fails where a developer can see it
    src/routes/error/server-only/+page.svelte throws while `document` is
    undefined, which is true in the rendering engine and false in a browser. It
    is the failure dev could not have before Go rendered in dev, and the one a
    developer most needs to be shown: the visitor gets the app's error page at
    the status of the failure, not a blank document that fills itself in.

    Given I open "/error/server-only"
    Then the document was answered with 500
    And the document already said the error page shows "Error 500" and "Internal Error"
    And the document already carried the root layout
    And the document never mentions "this page only renders in the browser"
    And I see "Error 500"

  Scenario: A Go query's answer is in the dev document, and the stub beside it still throws
    Given I open "/"
    Then the document already said the site is named "skgo"
    And the document already said "src/routes/site.remote.go"
    And the page never mentions "skgo: implemented in Go"
    And nothing on the page failed to load

  Scenario: A Go load's value is in the dev document, for a page under a layout
    Given I have signed in as "ada"
    When I visit "/account"
    Then the document response came from skgo in the expected mode
    And the document already said "Account of ada"
    And the account layout greets "ada"
    And the account page says its parent loaded "ada"
    And the browser never asked for the page's data

  Scenario: A page marked ssr = false is the same shell it is in prod
    Given I open "/spa"
    Then the document carried no rendered page
    And the site is named "skgo"

  Rule: The routing Go serves with is the dev server's

    `vp dev` serves a route tree a developer is editing, and kit numbers a node
    by walking `src/routes` — layouts and error pages first, then leaves, in
    traversal order (core/sync/create_manifest_data). So a page added anywhere
    renumbers every leaf after it, and a route table Go read once at startup
    does not merely miss the new route: it points the routes that were already
    there at the wrong nodes, and the wrong page renders at 200. Both halves are
    below, because the second one is the silent one.

    The route each scenario names does not exist in the checkout. The scenario
    writes it, and deletes it again afterwards. "tuna-9137" is a literal it
    supplies and nothing in the app contains, so a document carrying it is the
    file the scenario just wrote; "skgo" beside it is `getSite` in
    src/routes/site.remote.go, whose generated `.remote.ts` throws, so a
    document carrying that is Go answering a remote function called from a route
    that did not exist when Go started.

    Scenario: A route added while both servers run answers, with no rebuild and no restart
      Given the app has no route "/aaa-added"
      When a page is added at "/aaa-added" showing "tuna-9137" and the site name
      Then the document Go sends for "/aaa-added" carries "tuna-9137"
      And that document also carries "skgo"
      When I visit "/aaa-added"
      Then I see "Added"
      And I see the words "tuna-9137"

    Scenario: The pages already there keep their own components and their own data
      The added route sorts ahead of every leaf in the app, so kit gives it an
      index one of them used to have. /about is the cheap half — a page whose
      whole content is its own heading. /account is the expensive one: a page
      under a layout, both with Go loads, whose data only lands if the branch
      still names the nodes those loads are registered against.

      Given the app has no route "/aaa-added"
      And I have signed in as "ada"
      When a page is added at "/aaa-added" showing "tuna-9137" and the site name
      Then the document Go sends for "/aaa-added" carries "tuna-9137"
      And the document Go sends for "/about" has the page's heading "About"
      And that document never mentions "tuna-9137"
      When I visit "/account"
      Then the document already said "Account of ada"
      And the account layout greets "ada"
      And the account page says its parent loaded "ada"

    Scenario: A page option still belongs to the page that set it
      /spa is the app's one page whose branch turns server rendering off, and
      the node it set that on is one the added route has just renumbered. A
      document with the page rendered into it would be the option having been
      read off whichever node used to hold that index.

      Given the app has no route "/aaa-added"
      When a page is added at "/aaa-added" showing "tuna-9137" and the site name
      Then the document Go sends for "/aaa-added" carries "tuna-9137"
      When I visit "/spa"
      Then the document carried no rendered page
      And the site is named "skgo"

  Rule: Go still answers the endpoints kit's client calls

    A rendered document does not stop the client asking Go for a branch on a
    later navigation, and dev is where that is easiest to see directly. The
    fixtures are ssr.feature's: 418 and "This page is a teapot" from
    src/routes/error/expected/page.server.go, an ordinary Go error naming a
    database password from src/routes/error/unexpected/page.server.go, 402 and
    "Your account is in arrears" from
    src/routes/account/statement/page.server.go, and the redirect to "/" from
    src/routes/account/layout.server.go.

    Scenario: A load that refuses answers the data endpoint with the status it threw
      Given I open "/error/expected"
      Then Go's data endpoint for "/error/expected" answered 418
      And I see "Error 418"
      And the error message is "This page is a teapot"

    Scenario: A load that fails unexpectedly says nothing about why
      Given I open "/error/unexpected"
      Then Go's data endpoint for "/error/unexpected" answered 500
      And I see "Error 500"
      And the error message is "Something went wrong on our end."
      And the error page shows the support id "case-1121"
      And the page never mentions "hunter2"
      And the page never mentions "postgres://"

    Scenario: An expected error in a nested page renders inside its layout
      Given I have signed in as "ada"
      When I visit "/account/statement"
      Then Go's data endpoint for "/account/statement" answered 402
      And the account layout greets "ada"
      And I see "Account error 402"
      And the error message is "Your account is in arrears"

    Scenario: A redirect thrown from a load reaches the browser
      Given nobody has signed in
      When I visit "/account"
      Then I land on "/"
      And I see "Home"
      And Go's data endpoint for "/account" answers with a redirect to "/"
