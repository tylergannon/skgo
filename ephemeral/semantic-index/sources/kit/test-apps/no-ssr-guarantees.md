# What kit guarantees for CSR-only apps (`ssr = false`): the `no-ssr` test app and related tests

All paths are relative to the token cache root `/Users/tyler/src/skgo/ephemeral/inspiration/`.
Pinned `@sveltejs/kit` 3.0.0-next.25.

## Purpose

skgo serves every page as a client-rendered shell (`ssr=false`, no JS runtime in production). The
`no-ssr` test app is kit's own definition of what a globally-CSR app must still do: fallback shell,
server data via `__data.json`, error pages, redirects, styles. This file collects those guarantees,
plus the SSR-vs-CSR branches scattered through the `async` and `basics` suites that tell us which
behaviours disappear or change when there is no server render.

## Key facts

### The `no-ssr` app itself

- Global CSR switch: `export const ssr = false` in the root `+layout.js`; the same load redirects
  `/redirect` → `/` (302) and throws `error(500, 'Root layout load failed')` for `/root-layout-error`
  (`reference/kit/packages/kit/test/apps/no-ssr/src/routes/+layout.js:L1-L17`).
- A `+layout.server.js` still exists and returns `{ server_route_id: route.id }`
  (`reference/kit/packages/kit/test/apps/no-ssr/src/routes/+layout.server.js:L1-L5`), rendered in
  `<footer id="server-route-id">` (`.../+layout.svelte:L1-L11`). So **server loads run in CSR apps** —
  the client fetches them as `__data.json` (see the route-interception test below).
- `browser-globals/+page.js` reads `document.location.pathname` at module top level and declares
  `ssr = false`; importing it on the server would throw
  (`reference/kit/packages/kit/test/apps/no-ssr/src/routes/browser-globals/+page.js:L1-L11`).
- `src/error.html` is the static fallback with `%sveltekit.status%` in `#error-status` and
  `%sveltekit.error.message%` in `#error-message` (`reference/kit/packages/kit/test/apps/no-ssr/src/error.html:L1-L11`).
- `app.html` is the ordinary template (`%sveltekit.head%`, `%sveltekit.body%`, `%sveltekit.assets%`)
  (`reference/kit/packages/kit/test/apps/no-ssr/src/app.html:L1-L12`).
- `styles/+page.svelte` (blue) and `styles/prerendered/+page.svelte` (red, `prerender = true`)
  exist only to check CSS delivery (`.../styles/**`).
- Config: plain `sveltekit()` with no options, `build.minify: false`
  (`reference/kit/packages/kit/test/apps/no-ssr/vite.config.js:L5-L16`). Playwright config is the shared
  one re-exported verbatim (`.../playwright.config.js:L1`); port 5306 (`reference/kit/packages/kit/test/utils.js:L318`).

### Tests with JS enabled (`reference/kit/packages/kit/test/apps/no-ssr/test/test.js`)

- Non-existent route renders the default error page with `h1` = `404` — the shell is served and the
  client router produces the 404 (L8-L11).
- Non-existent route `/redirect` is redirected client-side by the root layout load to `/` (`h1` =
  `home`) (L13-L18).
- Universal pages/layouts are not executed on the server: `pathname: /browser-globals` (L20-L23).
- **Route-dependent server data is refetched after an error page**: intercept `**/__data.json*`,
  fail the first with 500 → `h1` = `500`; unroute; navigate to `/b` → footer shows `/b` (L25-L43).
  This pins the `__data.json` transport as the server-load channel in CSR mode.
- Root layout load throwing in SPA mode displays `error.html` with status `500` and message
  `Root layout load failed`, URL unchanged (L45-L50). Note `wait_for_started: false` because the app
  never starts.

### Tests with JS disabled (`reference/kit/packages/kit/test/apps/no-ssr/test/no-script.test.js`)

- Build mode only: exactly one `.css` request for a non-prerendered route and for a prerendered
  route — styles are linked in the shell before CSR starts, not injected later (L9-L33).

### CSR-relevant branches elsewhere

- `basics` `SPA mode / no SSR` describe: announcer styles applied on client nav; browser-only globals
  usable when `ssr=false` is set via `handle`, `+layout.js`, or `+page.js`; using a browser global on a
  page whose `+page.js` overrides `ssr` back to true fails with `document is not defined (500 Internal
  Error)`; `afterNavigate` called once at start (`reference/kit/packages/kit/test/apps/basics/test/client.test.js:L422-L476`).
- `basics` server test: an error thrown in a server load renders the error page honouring page options
  (`csr`) — the served HTML must not contain `kit.start(app` when CSR is off at that level
  (`reference/kit/packages/kit/test/apps/basics/test/server.test.js:L750-L760`;
  `reference/kit/packages/kit/test/apps/basics/src/routes/errors/load-error-page-options/csr/+layout.js:L1`).
- `basics` server test: a `__data.json` request for a missing route with `x-sveltekit-invalidated=1`
  returns 200 JSON `{type:'data', nodes:[{type:'data', data:[... 'rootlayout' ...]}]}` (root layout data
  for the error page); without the param it is a 404 HTML page; a crafted `x-sveltekit-invalidated=0`
  is a 404 (`reference/kit/packages/kit/test/apps/basics/test/server.test.js:L761-L785`).
- `basics` client test: `__data.json` responses have `cache-control: private, no-store`
  (`reference/kit/packages/kit/test/apps/basics/test/client.test.js:L231-L241`); no `__data.json` is
  requested when the target has no server load (`:L406-L420`); `x-sveltekit-version` header is sent on
  data responses but not documents (`reference/kit/packages/kit/test/apps/basics/test/server.test.js:L1069-L1078`).
- `async` tests that branch on `javaScriptEnabled` show what SSR uniquely provides:
  `query-loading-state` shows `loading` without JS and `slow data` with JS (`reference/kit/packages/kit/test/apps/async/test/test.js:L903-L918`);
  `live-ssr-value` shows `loading` without JS (`:L949-L963`); `query-boundary` shows the pending snippet
  during SSR (`:L888-L901`); native form submissions clear inputs while enhanced ones do not (`:L396-L405`).
- `options-2` documents `paths.base: '/basepath'` with `paths.relative: true` and `bundleStrategy:
  'single'`; the no-JS project sees relative `./` / `../../` bases while the JS project sees
  `/basepath/` (`reference/kit/packages/kit/test/apps/options-2/test/test.js:L20-L47`); remote
  query/prerender/command/form calls from the client account for the base path (`:L69-L116`;
  `reference/kit/packages/kit/test/apps/options-2/src/routes/remote/count.remote.js:L12-L36`).

## Citations

- `reference/kit/packages/kit/test/apps/no-ssr/src/routes/+layout.js:L1-L17`
- `reference/kit/packages/kit/test/apps/no-ssr/src/routes/+layout.server.js:L1-L5`
- `reference/kit/packages/kit/test/apps/no-ssr/src/routes/+layout.svelte:L1-L11`
- `reference/kit/packages/kit/test/apps/no-ssr/src/routes/browser-globals/+page.js:L1-L11`
- `reference/kit/packages/kit/test/apps/no-ssr/src/error.html:L1-L11`
- `reference/kit/packages/kit/test/apps/no-ssr/vite.config.js:L1-L18`
- `reference/kit/packages/kit/test/apps/no-ssr/test/test.js:L1-L50`
- `reference/kit/packages/kit/test/apps/no-ssr/test/no-script.test.js:L1-L33`
- `reference/kit/packages/kit/test/apps/basics/test/client.test.js:L422-L476`, `:L231-L241`, `:L406-L420`
- `reference/kit/packages/kit/test/apps/basics/test/server.test.js:L750-L785`, `:L1069-L1078`
- `reference/kit/packages/kit/test/apps/options-2/test/test.js:L20-L47`, `:L69-L116`
- `reference/kit/packages/kit/test/apps/options-2/vite.config.js:L11-L47`

## skgo implications

- The Go server must serve the fallback shell for every non-static route with status 200 (or the
  matched status for errors) and let the client router decide 404s; `error.html` with
  `%sveltekit.status%` / `%sveltekit.error.message%` substitution is the only server-rendered error
  surface, used when the root layout load fails.
- Server loads written in Go are reached through `GET <route>/__data.json` (with
  `x-sveltekit-invalidated` bits); the response must be kit's node-list JSON with `cache-control:
  private, no-store` and an `x-sveltekit-version` header, and must return root-layout data for
  unknown routes when invalidated so the client can render its error page.
- Copy into the example app's suite: no-ssr `test.js` L8-L50 verbatim (404 page, root-layout redirect,
  browser globals, `__data.json` failure then recovery, `error.html`), and no-script L9-L33 as a
  build-mode check that the shell links CSS (one `.css` request).
- Add the options-2 base-path remote tests if skgo supports `paths.base`; otherwise record that
  the base path is fixed at build time.

## Gotchas

- `page.goto(url, { wait_for_started: false })` is required for pages that never start the app
  (error.html case); the shared `page` fixture otherwise waits for `body.started`
  (`reference/kit/packages/kit/test/utils.js:L113-L145`).
- The no-JS Playwright project in kit exists to test SSR output; for skgo it only makes sense for
  static-asset and header checks (CSS link, `content-type`), never for page content.
- `+layout.server.js` in a CSR app still runs on every navigation that needs it — the Go server owns
  it; the client never sees Go code, only `__data.json`.
- `ssr` can be flipped per route (`+page.js` `export const ssr`) in kit; skgo fixes it globally, so
  tests like `ssr-page-config/layout/overwrite` are not applicable.

## Recipes

- Simulate a failing server load once and verify recovery:
  ```js
  let failed = false;
  await page.route('**/__data.json*', (route) => {
    if (failed) return route.continue();
    failed = true;
    return route.fulfill({ status: 500, body: 'nope' });
  });
  await app.goto('/a'); expect(await page.textContent('h1')).toBe('500');
  await page.unroute('**/__data.json*');
  await app.goto('/b'); await expect(page.locator('#server-route-id')).toHaveText('/b');
  ```
  (`reference/kit/packages/kit/test/apps/no-ssr/test/test.js:L25-L43`)
- Count stylesheet requests to prove the shell links CSS:
  `page.on('request', r => r.url().endsWith('.css') && requests.push(r.url()))` then
  `expect(requests.length).toBe(1)` (`no-script.test.js:L12-L19`).
