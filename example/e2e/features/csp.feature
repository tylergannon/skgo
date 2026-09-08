Feature: A document's Content-Security-Policy header matches what it renders

  The app configures `csp: { mode: 'auto', directives: { 'script-src': ['self']
  } }` (web/vite.config.ts) — kit's own default mode (`list(['auto', 'hash',
  'nonce'])`, core/config/options.js), the one a SvelteKit developer gets
  without ever touching `csp.mode` at all. Kit's own rule for what `auto`
  resolves to is per page: `use_hashes = mode === 'hash' || (mode === 'auto'
  && prerender)` (`Csp`'s constructor, runtime/server/page/csp.js).

  Every page in this app is rendered by Go per request right now — none is
  prerendered (see vite.config.ts's own comment: the root layout gained a Go
  load for #81's fixture, and kit can only prerender a branch with nothing
  Go-only in it). So `auto` resolves to nonce mode unconditionally here
  (csp.go's `newDocumentCSP`); the other half of kit's ternary — a
  prerendered page resolving to hash mode, baked into the static file as a
  `<meta http-equiv>` tag — is proven at the Go test level instead
  (`csp_test.go`'s `TestCSPAutoMode_DynamicIsNonceModePrerenderedWouldBeHashMode`),
  anchored to the same kit source this feature is.

  A CSP violation does not fail a request — the browser just refuses to run
  the element it names and answers with the response it already had. A page
  whose nonce is wrong is a page that looks identical up to the moment
  something needed the script that never ran: it never subscribes to
  anything, never answers a click, and — for a value a load promised — never
  fills in, because the streamed chunk carrying it
  (`page/data_serializer.js:103`) is exactly the kind of inline script a
  nonce policy blocks unless it carries the same nonce the boot script did.
  The browser's own console is the only place that says why. So the proof
  here is the board on /live still updating live, a value /stream promised
  still filling in, and the console carrying no complaint about either — not
  just a header that is present.

  Scenario: A page Go renders per request gets a nonce, and the page still hydrates under it
    Given another tab is open at "/todos"
    And I open "/live"
    Then the response carries a Content-Security-Policy header naming the boot script's own nonce
    And the board is on stream frame 1
    When the other tab adds the todo "nonced and still hydrated"
    Then the board's newest todo is "nonced and still hydrated"
    And the board is on stream frame 2
    And every part of the page loaded
    And the browser reported no CSP violations

  Scenario: A value a load promised still fills in after the document streams under a nonce
    When I start loading "/stream"
    And every promised value has arrived
    Then the ticker says "the first thing to arrive"
    And the browser reported no CSP violations
