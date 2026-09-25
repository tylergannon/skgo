Feature: The engine's JavaScript is SvelteKit's own build

  The bundle the Go process renders pages with is an output of the app's
  SvelteKit build — a fourth environment of it, alongside the client, the server
  and the service worker — folded into one script. Production evaluates the
  bundle; development evaluates Kit's live module graph.

  That every route renders its own page's heading inside the root layout is a
  claim about the document, and example/contracts_test.go walks every route for
  it. What only a browser can show is that kit's client agrees with the engine
  about the name every remote function is addressed by.

  Scenario: A value Go answered during the render is in the document, under kit's own id
    The id a remote function is addressed by — `<hash>/<name>` — is appended by
    kit's build, not derived a second time by the adapter. The engine looks the
    Go host call up under it and Go writes the answer into the boot script
    under it, and kit's client reads its cache with the same string. Two
    spellings of that id would still render the page; the client would simply
    ask for every value again.

    "skgo" and "src/routes/site.remote.go" come from getSite in
    src/routes/site.remote.go, whose generated TypeScript throws.

    Given I have signed in as "ada"
    And I note the remote request count
    When I visit "/"
    Then the document already said the site is named "skgo"
    And the document already said "src/routes/site.remote.go"
    And the site is named "skgo"
    And exactly 0 remote requests were made since
