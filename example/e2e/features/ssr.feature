Feature: Pages arrive rendered, and kit's client takes them over

  Every scenario runs unchanged against the embedded production build and the
  live modules transformed by `vp dev`. In both modes Go owns the document and
  runs Kit's renderer; only the renderer's module source changes.

  What the document itself carries — the markup, the status, the error page, the
  hydration payload, the redirect — is asserted against the real handler in
  example/ssr_test.go, example/server_test.go and example/contracts_test.go.
  What is left here is what kit's client does with that document once it has
  booted: whether it asks again for values the document already carried, and
  what it renders that the server deliberately did not.

  Scenario: Hydration does not refetch what the document already carried
    Given I have signed in as "ada"
    And I note the data request count
    And I note the remote request count
    When I visit "/account"
    Then the account layout greets "ada"
    And I am signed in as "ada"
    And exactly 0 data requests were made since
    And exactly 0 remote requests were made since

  Scenario: The plans on the pricing page are asked for only once the browser has the page
    The plans sit in a boundary with a `pending` snippet, and Svelte's server
    compiler emits that snippet instead of the boundary's children, so `getPlans`
    is not called while the document is built. Kit's client calls it after
    boot.

    Given I open "/pricing"
    Then the document carried the plans as still loading
    And the plans are "Hobby, Team, Enterprise"

  Scenario: A page marked ssr = false still arrives as the shell
    Given I open "/spa"
    Then the document carried no rendered page
    And the site is named "skgo"

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
