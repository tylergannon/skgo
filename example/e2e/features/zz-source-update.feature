Feature: The document follows the source being served

  A production server renders the frontend build embedded in its binary. A
  development server renders the modules the running Vite server transformed.
  Editing a component therefore changes the next development document without
  rebuilding, while the already-built production document remains unchanged.

  These scenarios run after the read-only browser tour because adding a route
  makes Kit deliberately reload every open dev client while it rewrites that
  client's generated route graph.

  Scenario: A source edit reaches the live development document but not the production build
    Given I open "/"
    Then the document already said the page's own heading is "Home"
    When "src/routes/+page.svelte" has "Home" replaced with "Home, edited while running"
    Then a new document for "/" carries "Home, edited while running" from live source or "Home" from its build

  @release
  Scenario: The already-open page accepts the same document without a hydration mismatch and keeps Vite HMR
    Given I open "/"
    When "src/routes/+page.svelte" has "Home" replaced with "Home, hot updated"
    Then the open page hot-updates to "Home, hot updated" in dev or stays at built heading "Home" without reloading

  Scenario: A route added while both servers run comes from Kit's live route graph
    Given I open "/"
    When the route fixture is added while the servers keep running
    Then the live dev route renders its Go load while the production build stays unchanged
