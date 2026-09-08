@prod
Feature: The engine's JavaScript is SvelteKit's own build

  The bundle the Go process renders pages with is an output of the app's
  SvelteKit build — a fourth environment of it, alongside the client, the server
  and the service worker — folded into one script. It used to be assembled by a
  second bundler inside the adapter, which had to reimplement the parts of the
  build it could not reach: the Svelte server compile, the TypeScript strip, the
  `$app/*` aliases, the twenty-three compile-time constants, and the epilogue
  that gives every remote function the id the browser addresses it by. None of
  that is skgo's any more, and this feature is how a developer can see it.

  It is all `@prod` for the same reason ssr.feature is: the bundle exists only in
  the built app. In dev, kit's own server owns the document.

  Nothing a visitor sees was supposed to change, so what is claimed here is that
  nothing did — stated as the two things a wrong build breaks quietly.

  The first is which component renders. Kit numbers the nodes of a build twice:
  once in the modules it writes, and again in the manifest it generates, from
  which it has dropped the nodes of every page it prerendered. The two agree
  only up to the first prerendered page — `/about` — and diverge by one for
  every node after it. A bundle that read the wrong numbering renders the wrong
  page's component, with the right layout around it and a 200 beside it, and the
  home page looks perfect. So every route below is walked, and each is asked for
  its own heading: the `<h1>` its own `+page.svelte` writes and no other page's.

  The second is the request URL. The engine has no web platform of its own; the
  `URL` it parses every request with is bound from Go when a runtime is created,
  along with the text codecs and base64 that kit's runtime constructs while its
  modules are still evaluating. They used to be three hundred lines of
  hand-written JavaScript shipped inside the bundle. A page reaching the engine
  at all means Go's `URL` parsed its address, which is why a path with a query
  string and one with a rest parameter are walked beside the rest.

  Every heading below is written here, not read from the app.

  Background:
    Given I have signed in as "ada"

  Scenario Outline: <path> arrives carrying its own page
    When I visit "<path>"
    Then the document already said the page's own heading is "<heading>"
    And the document already carried the root layout
    And I see "<heading>"

    Examples: pages before the first prerendered node
      | path         | heading |
      | /            | Home    |
      | /plain       | Plain   |

    Examples: pages after it, where the two numberings disagree
      | path                        | heading            |
      | /items/93                   | Item 93            |
      | /items/93?from=nav          | Item 93            |
      | /todos                      | Todos              |
      | /todos/t1                   | Todo               |
      | /todos/gate                 | The refresh gate   |
      | /todos/pair                 | A pair of todos    |
      | /empty                      | Empty              |
      | /pricing                    | Pricing            |
      | /docs/guide/getting-started | Docs               |
      | /contact                    | Contact            |
      | /stream                     | Stream             |
      | /console                    | Console            |
      | /live                       | Live               |
      | /batch                      | Batch              |
      | /account/orders             | Orders             |

  Scenario: A prerendered page is still the file kit's own server build wrote
    The engine is the only place `$app/server` means something different: kit's
    own server build — the one that prerenders — keeps kit's real module, and if
    skgo's substitute reached it the prerender would call into a Go process that
    is not running.

    So /about is written to disk during the build, before any visitor exists.
    The root layout puts a signed-in visitor's name on every page it renders;
    the copy on disk cannot have one, because it was rendered when nobody was
    signed in. The session appears a moment later, once the page has hydrated,
    which is what says the absence is prerendering rather than a failed sign-in.

    When I visit "/about"
    Then the document already said the page's own heading is "About"
    And the document never mentions "Signed in as ada"
    And I am signed in as "ada"

  Scenario: A value Go answered during the render is in the document, under kit's own id
    The id a remote function is addressed by — `<hash>/<name>` — is appended by
    kit's build now, not derived a second time by the adapter. The engine looks
    the Go host call up under it and Go writes the answer into the boot script
    under it, and kit's client reads its cache with the same string. Two
    spellings of that id would still render the page; the client would simply
    ask for every value again.

    "skgo" and "src/routes/site.remote.go" come from getSite in
    src/routes/site.remote.go, whose generated TypeScript throws.

    Given I note the remote request count
    When I visit "/"
    Then the document already said the site is named "skgo"
    And the document already said "src/routes/site.remote.go"
    And the document never mentions "skgo: implemented in Go"
    And the site is named "skgo"
    And exactly 0 remote requests were made since
