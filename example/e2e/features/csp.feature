@prod
Feature: A document's Content-Security-Policy header matches what it renders

  The app configures `csp: { mode: 'hash', directives: { 'script-src':
  ['self'] } }` (web/vite.config.ts) — the same option a SvelteKit developer
  writes for any app, nothing skgo-specific about it. Kit's own rule is that
  the one script the document boots with earns its way into the header by its
  own SHA-256 hash, computed over the exact bytes between its `<script>` and
  `</script>` tags (runtime/server/page/csp.js). Nothing here is possible if
  skgo never reaches hash mode at all: with no policy, there is no header to
  check and every inline script runs regardless, so a scenario that only
  looked for markup could pass against a build that ignored `csp` completely.

  Hash mode, not nonce mode, because it is the one a shared app config can
  use everywhere: `/about` is prerendered (routes/about/+page.ts), and kit
  refuses outright to build a prerendered page under `mode: 'nonce'`. Nonce
  mode's header/attribute pairing is exercised at the Go test level instead
  (csp_test.go), anchored to the same kit source this feature is.

  A CSP violation does not fail a request — the browser just refuses to run
  the element it names and answers with the response it already had. A page
  whose hash is wrong is a page that looks identical up to the moment
  something needed the script that never ran: it never subscribes to
  anything, never answers a click, and the browser's own console is the only
  place that says why. So the proof here is the board on /live still updating
  live and the console carrying no complaint about it, not just a header
  that is present.

  Scenario: The header names the boot script's own hash, and the page still hydrates under it
    Given another tab is open at "/todos"
    And I open "/live"
    Then the response carries a Content-Security-Policy header naming the boot script's own hash
    And the board is on stream frame 1
    When the other tab adds the todo "hashed and still hydrated"
    Then the board's newest todo is "hashed and still hydrated"
    And the board is on stream frame 2
    And every part of the page loaded
    And the browser reported no CSP violations
