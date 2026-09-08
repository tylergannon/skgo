Feature: Pages arrive rendered

  Every scenario runs unchanged against the embedded production build and the
  live modules transformed by `vp dev`. In both modes Go owns the document and
  runs Kit's renderer; only the renderer's module source changes.

  A document leaves Go with its content already in it. The markup is rendered by
  SvelteKit's own renderer inside the Go process, every value in it was answered
  by a Go function while the page was being built, and the same values travel
  down with the document so that kit's client hydrates without asking for any of
  them again.

  Every value named below is a fixture with exactly one source in the app:
  `getSite` in src/routes/site.remote.go answers with the name "skgo" and its own
  path; `getItem` in src/routes/items/[id]/item.remote.go answers "Widget <id>"
  for the id in the URL; the loads under /account answer with the signed-in
  visitor's name; the load in src/routes/(marketing)/pricing/page.server.go
  answers with a featured plan priced at 4500 cents, an amount no other function
  in the app returns. The generated TypeScript beside each of those throws
  "skgo: implemented in Go", so none of these strings has a second way to appear.

  "$45.00" is what `Money.format()` in src/hooks.ts makes of those 4500 cents. It
  is a method on a class, so a page that received a plain object cannot write it
  — which is why the price standing in the raw document says the engine was
  handed a Money, decoded by the app's own `transport` hook, before it rendered.

  Scenario: The home page's HTML already contains the site name
    Given I open "/"
    Then the document already said the site is named "skgo"
    And the site is named "skgo"

  Scenario: A page under a layout arrives with the layout's data and its own
    Given I have signed in as "ada"
    When I visit "/account"
    Then the document already said "Account of ada"
    And the document already said "The layout loaded ada"
    And the account layout greets "ada"
    And the account page says its parent loaded "ada"

  Scenario: A remote query awaited in markup is answered by Go inside the document
    Given I open "/"
    Then the document already said "src/routes/site.remote.go"
    And the document never mentions "skgo: implemented in Go"
    And the answer came from "src/routes/site.remote.go"

  Scenario: A query with an argument is rendered with the argument Go was given
    Given I open "/items/93"
    Then the document already said the item is named "Widget 93"
    And the document never mentions "Widget 42"
    And the item is named "Widget 93"

  Scenario: Hydration does not refetch what the document already carried
    Given I have signed in as "ada"
    And I note the data request count
    And I note the remote request count
    When I visit "/account"
    Then the account layout greets "ada"
    And I am signed in as "ada"
    And exactly 0 data requests were made since
    And exactly 0 remote requests were made since

  Scenario: A custom-typed value is rendered by its own method on the server
    Given I note the data request count
    When I open "/pricing"
    Then the document already said "Startup — $45.00"
    And the document carried the price as a Money of 4500 cents
    And the featured plan costs "$45.00"
    And exactly 0 data requests were made since

  Scenario: A custom-typed value in a remote answer is rendered by its own method
    The one above is a server load's value, which travels down inside the
    document because a load always runs. This one is a remote function's, called
    back out to Go while the page was being rendered — a different path through
    the engine, and the one the plans list below has never taken.

    The plans sit in a boundary with a `pending` snippet, and Svelte's server
    compiler emits that snippet instead of the boundary's children
    (svelte/src/compiler/phases/3-transform/server/visitors/SvelteBoundary.js),
    so `getPlans` is not called while the document is built. The spotlight is in
    a boundary with no pending snippet, so it is.

    750 cents is an amount no other function in this app returns, and "$7.50" is
    what `Money.format()` makes of them. It is a method on the class in
    src/hooks.ts, so a page handed a plain object could not have written it: the
    engine was given a Money, rebuilt by the app's own transport decoder out of
    what Go sent, before the line was rendered.

    Given I open "/pricing"
    Then the document already said "Student — $7.50"
    And the document carried the price as a Money of 750 cents
    And the document never mentions "skgo: implemented in Go"
    And the spotlight plan costs "$7.50"

  Scenario: The plans beside it were not asked for until the browser had the page
    The other half of the same claim, so that the one above is about a boundary
    that renders during SSR rather than about the page as a whole.

    Given I open "/pricing"
    Then the document carried the plans as still loading
    And the plans are "Hobby, Team, Enterprise"

  Scenario: A page marked csr = false is plain HTML with no script tag
    Given I open "/plain"
    Then the document already said the site is named "skgo"
    And the document carries no script
    And the site is named "skgo"

  Scenario: A page marked ssr = false still arrives as the shell
    Given I open "/spa"
    Then the document carried no rendered page
    And the site is named "skgo"

  Rule: Errors and redirects are the document's

    A page that cannot render normally still arrives rendered, with the status
    kit would give it. Which `+error.svelte` answers, and how many layouts
    survive with it, are kit's decisions and not skgo's: kit walks outward from
    the node that failed to the nearest error page declared above it.

    The fixtures are one per failure and have one source each. The load in
    src/routes/error/expected/page.server.go refuses with 418 and the words
    "This page is a teapot". The one in
    src/routes/error/unexpected/page.server.go fails with an ordinary Go error
    whose text names a database password. The load in
    src/routes/account/statement/page.server.go refuses with 402 and "Your
    account is in arrears", under a section that declares its own
    `+error.svelte`. The layout load in src/routes/account/layout.server.go
    redirects a signed-out visitor to "/", and the query in
    src/routes/error/redirect/redirect.remote.go redirects to "/about" instead
    of answering.

    Every one of those failures also reaches the app's own `handleError` hook
    — example.HandleError, in example/server.go — before the error page ever
    renders. Kit's own contract runs that hook for expected and unexpected
    errors alike, and only lets it add to or override what the visitor is
    told; example.HandleError adds a support id, "case-1121", to everything it
    sees, and additionally replaces an unknown error's own message, which kit
    never lets reach the visitor unchanged, with words of its own.

    Scenario: A load that fails returns the error page with the status it threw
      Given I open "/error/expected"
      Then the document was answered with 418
      And the document already said the error page shows "Error 418" and "This page is a teapot"
      And the document already carried the root layout
      And the error message is "This page is a teapot"
      And the browser never asked for the page's data

    Scenario: The handleError hook runs for an error the app raised on purpose, not only an unexpected one
      Kit's own hook contract makes no exception for an error thrown with
      `error(status, message)`: only an error already run through the hook by
      an earlier layer is skipped, and nothing on a cold render produces one
      of those. So the app's hook still runs here, on the very failure the
      previous scenario just showed keeps its own message — it only cannot
      override it unless it chooses to.

      Given I open "/error/expected"
      Then the document already said the error page shows "Error 418" and "This page is a teapot"
      And the document already said "case-1121"
      And the error page shows the support id "case-1121"

    Scenario: The handleError hook decides what an unexpected failure's visitor is told
      Kit's own rule for an error nobody meant to happen is that its real text
      never reaches the visitor — case: it names a database password. What
      does reach them is now the app's own hook, not a generic default: it
      replaces the message and adds a support id, and the error page shows
      exactly that.

      Given I open "/error/unexpected"
      Then the document was answered with 500
      And the document already said the error page shows "Error 500" and "Something went wrong on our end."
      And the document already said "case-1121"
      And the document never mentions "Internal Error"
      And the document never mentions "hunter2"
      And the document never mentions "postgres://"
      And the error page shows the support id "case-1121"

    Scenario: An unknown route returns the error page with 404
      Given I open "/no-such-page"
      Then the document was answered with 404
      And the document already said the error page shows "Error 404" and "Not Found"
      And the document already carried the root layout
      And the browser never asked for the page's data

    Scenario: An expected error in a nested page renders inside its layout
      Given I have signed in as "ada"
      When I visit "/account/statement"
      Then the document was answered with 402
      And the document already said "Account of ada"
      And the document already said the error page shows "Account error 402" and "Your account is in arrears"
      And the account layout greets "ada"
      And the browser never asked for the page's data

    Scenario: A page that catches its own failure still carries the failure's status
      Kit installs one error transform for the whole render and Svelte calls it
      for every boundary that has a `failed` snippet, the app's own included.
      So a page that handles its failure gracefully renders — and the document
      is still answered with the status the caught error had.
      src/routes/error/boundary/boundary.remote.go refuses with 409.

      Given I open "/error/boundary"
      Then the document was answered with 409
      And the document already said the page's own heading is "Sensor"
      And the document already said "The sensor is being calibrated"
      And I see "Sensor"
      And the browser never asked for the page's data

    Scenario: A redirect thrown from a load answers with a 3xx and no body
      Given nobody has signed in
      When I visit "/account"
      Then I land on "/"
      And I see "Home"
      When I ask for "/account" without following redirects
      Then it answered 307 to "/" with no body

    Scenario: A redirect thrown while the page renders answers the same way
      When I visit "/error/redirect"
      Then I land on "/about"
      When I ask for "/error/redirect" without following redirects
      Then it answered 307 to "/about" with no body

  Rule: The engine is Go's

    The renderer is a pool of engines inside the Go binary. Nothing in it reads a
    file, opens a socket or sets a timer, and the only way a value reaches it is
    a call back out to Go.

    Scenario: Two pages rendering at once do not share a runtime
      When "/items/11" and "/items/77" are asked for at the same moment
      Then the first document says the item is named "Widget 11"
      And the second document says the item is named "Widget 77"
      And neither document carries the other's item

    Scenario: A remote function called during render is answered by Go, not by a stub
      Given I open "/"
      Then the document already said "src/routes/site.remote.go"
      And the document never mentions "skgo: implemented in Go"
      And nothing on the page failed to load

    Scenario: A render that throws yields a Go error and a static error page, never a blank
      The root layout is the one node no `+error.svelte` can guard, so a throw
      there takes the render down and takes the retry down with it.
      src/routes/+layout.svelte throws for /error/render and nowhere else.

      Kit's own respond_with_error consults the hook before it even tries the
      retry, so the static page still carries the app's own words rather than
      the generic default — but error.html has no slot for anything beyond
      status and message, so the support id every other error page shows does
      not reach this one.

      Given I open "/error/render"
      Then the document was answered with 500
      And the document is kit's static error page saying 500 and "Something went wrong on our end."
      And the document never mentions "case-1121"

    Scenario: A command called during render is refused
      src/routes/error/command/+page.svelte awaits a command in its markup.
      Kit refuses a command while a document is being rendered, because a
      document is produced on every navigation and the mutation would run
      again on every reload.

      Given I open "/error/command"
      Then the document was answered with 500
      And the document already said the error page shows "Error 500" and "Internal Error"
      And the document never mentions "Cannot call a command"
      And the tally is nowhere on the page

  Rule: A value the load only promised arrives on the same document

    A load may return a value it does not have yet. Kit sends the document at
    once, with the page's own loading state already in it, and appends the value
    to the same response when it arrives: no second request, and nobody waits
    for the slow half of a page before they can read the fast half. skgo's
    document is assembled by Go and does the same thing, which is the only way
    an `{#await}` in a page can mean anything on a cold load.

    The fixture is the one load in the app that promises anything,
    src/routes/account/orders/page.server.go. It says there are 2 orders
    straight away and takes a second and a half to say that they are "a slow
    parcel" and "a slower parcel" — two strings that appear nowhere else in the
    app, beside a generated TypeScript load that throws "skgo: implemented in
    Go".

    Scenario: A cold load shows the loading state first and fills it in later
      Given I have signed in as "ada"
      And I note the remote request count
      When I start loading "/account/orders"
      Then the page says there are 2 orders and is still fetching them
      When the orders arrive
      Then the orders are "a slow parcel" and "a slower parcel"
      And the document carried the loading state, and the orders after it ended
      And the browser never asked for the page's data
      And exactly 0 remote requests were made since

  Rule: $app/paths answers the same during a render as it does everywhere else

    `resolve`, `asset` and `match` are kit's own functions
    (runtime/app/paths/server.js), called from src/routes/render-paths/+page.svelte
    while Go renders that page. `resolve` and `asset` need nothing this engine
    lacks — they are string logic over the app's compiled-in base and assets
    path — so they run unmodified. `match` needs a manifest this engine has
    none of, so it asks Go's own route table for the answer instead of
    building a second router for the engine.

    Every href below is a literal derived from kit's own rules, not from
    skgo's output: `paths.relative` defaults to `true` in this kit version,
    so during server rendering a same-depth link is prefixed `.` rather than
    made absolute (runtime/app/paths/server.js, `resolve`) — /render-paths has
    one path segment, the same depth as /api/todos, /items/42 and
    /robots.txt, so the prefix is `.` for all three. /items/[id] is a real
    route in this app; `resolve('/items/[id]', { id: '42' })` populates it the
    same way kit's own `resolveRoute` example does. /items/77 is that same
    route with a different id, so `match` finds it by `/items/[id]` with
    `{id: "77"}` — the parameters a real request to /items/77 would carry.

    Kit's own client-side `resolve`/`asset` (runtime/app/paths/client.js) are
    documented to answer differently — always the absolute, base-prefixed
    href, never the relative one server rendering gives — so a screenshot of
    this page taken after the client has hydrated legitimately shows
    `/api/todos` where the document this scenario checks said `./api/todos`.
    That is kit's own designed difference between the two environments, not a
    disagreement to resolve.

    Scenario: resolve and asset produce the relative hrefs kit's own rules give this page
      Given I open "/render-paths"
      Then the document already said "resolve('/api/todos') = ./api/todos"
      And the document already said "resolve('/items/[id]', id: '42') = ./items/42"
      And the document already said "asset('robots.txt') = ./robots.txt"

    Scenario: match finds the same route id a real request to it resolves
      Given I open "/render-paths"
      Then the document already said "match('/items/77') = /items/[id] {\"id\":\"77\"}"
