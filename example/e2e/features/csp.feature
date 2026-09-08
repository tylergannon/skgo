@prod
Feature: A document's Content-Security-Policy header matches what it renders

  The app configures `csp: { mode: 'auto', directives: { 'script-src': ['self']
  } }` (web/vite.config.ts) — kit's own default mode (`list(['auto', 'hash',
  'nonce'])`, core/config/options.js), the one a SvelteKit developer gets
  without ever touching `csp.mode` at all. Kit's own rule for what `auto`
  resolves to is per page: `use_hashes = mode === 'hash' || (mode === 'auto'
  && prerender)` (`Csp`'s constructor, runtime/server/page/csp.js).

  `/about` is prerendered (routes/about/+page.ts) — kit's own Node build
  resolves `auto` to hash mode for it and bakes the result straight into the
  static file, entirely before skgo's binary exists; Go never computes
  anything for that page at all. Every other page is rendered by Go per
  request, where the engine never prerenders anything, so `auto` resolves to
  nonce mode unconditionally (csp.go's `newDocumentCSP`). `/live` and
  `/stream` are both pages of this second kind.

  A CSP violation does not fail a request — the browser just refuses to run
  the element it names and answers with the response it already had. A page
  whose hash or nonce is wrong is a page that looks identical up to the
  moment something needed the script that never ran: it never subscribes to
  anything, never answers a click, and — for a value a load promised — never
  fills in, because the streamed chunk carrying it
  (`page/data_serializer.js:103`) is exactly the kind of inline script a
  nonce policy blocks unless it carries the same nonce the boot script did.
  The browser's own console is the only place that says why. So the proof
  here is the board on /live still updating live, a value /stream promised
  still filling in, and the console carrying no complaint about either — not
  just a header or a meta tag that is present.

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

  Scenario: A page kit prerendered gets a hash, baked into the page itself
    Given I open "/about"
    Then the page carries a Content-Security-Policy meta tag naming the boot script's own hash
    And the browser reported no CSP violations

  Scenario: A value a load promised still fills in after the document streams under a nonce
    When I start loading "/stream"
    And every promised value has arrived
    Then the ticker says "the first thing to arrive"
    And the browser reported no CSP violations
