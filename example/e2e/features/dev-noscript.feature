@dev
Feature: A page under `vp dev` arrives styled

  There are no built stylesheets in dev. Vite serves a component's CSS through
  the JavaScript that imports it, so a document that only linked files would be
  linking files the dev server does not have, and a page that waited for its
  modules would paint unstyled and then restyle itself a second later. Kit's own
  dev document answers that by inlining every stylesheet the branch reaches —
  `inline_styles` walks vite's module graph and re-imports each CSS dep with
  `?inline` (`exports/vite/dev/index.js`) — into a `<style data-sveltekit>` tag
  its client removes once it has mounted. The document Go sends in dev does the
  same, so the two are the same document.

  These scenarios run in the Playwright project configured with
  `javaScriptEnabled: false`. Nothing the browser draws below can have been put
  there by a script: it is what Go sent, drawn.

  Each scenario names the declaration it is looking for and the component that
  declares it. The step checks the app really says it before it checks the
  document does, so a rule renamed in the app fails here rather than quietly
  passing against a document that carries nothing.

  Scenario: The front page carries the rules its own branch declares
    Given I open "/"
    Then the browser ran no script at all
    And the document carries "--lime: #c7f36b" from "src/routes/+layout.svelte"
    And the document carries "letter-spacing: -0.07em" from "src/routes/+page.svelte"
    And the app's nav is more than 40 pixels tall
    And the page's title is more than 60 pixels tall

  Scenario: A marketing page carries the root layout's rules
    Given I open "/pricing"
    Then the browser ran no script at all
    And I see "Pricing"
    And the document carries "--lime: #c7f36b" from "src/routes/+layout.svelte"
    And the app's nav is more than 40 pixels tall

  Scenario: A page behind a signed-in load carries them too
    Given a signed-in visitor whose browser runs no script
    When I visit "/account"
    Then the browser ran no script at all
    And the account layout greets "ada"
    And the document carries "--lime: #c7f36b" from "src/routes/+layout.svelte"
    And the app's nav is more than 40 pixels tall
