# Kit's Playwright harness: `test/utils.js`, `test/setup.js`, per-app configs

All paths are relative to the token cache root `/Users/tyler/src/skgo/ephemeral/inspiration/`.

## Purpose

Reusable idioms from the harness kit uses to run its test apps, so the skgo example app's Playwright
suite can borrow the fixtures (`app`, `clicknav`, `read_errors`, `start_server`), the project split
(js / no-js), request-counting patterns, and the way kit boots an app in dev vs build.

## Key facts

### Fixtures (`reference/kit/packages/kit/test/utils.js`)

- `test = base.extend({...})` (L15). Fixtures:
  - `app`: programmatic control via globals the app's root layout puts on `window` — `goto`,
    `invalidate`, `beforeNavigate`, `afterNavigate`, `preloadCode`, `preloadData`, `match` (L16-L33).
    Backed by `reference/kit/packages/kit/test/setup.js:L12-L28`, which also exposes `svelte_tick` and
    adds `body.started` in `onMount`.
  - `clicknav(selector, {timeout, waitForURL})`: with JS, `Promise.all([page.waitForNavigation(),
    element.click(), page.waitForURL?])`; without JS just clicks (L35-L54).
  - `scroll_to`, `in_view`, `get_computed_style` (L56-L111).
  - `page` override: wraps `goto`, `goBack`, `reload` to wait for `body.started` (15 s locally, 30 s
    in CI) unless `{ wait_for_started: false }`; rewrites stack traces to point at the test (L113-L145).
  - `read_errors(path)`: reads `test/errors.jsonl` (appended by the basics `handleError` hook) and
    returns the last record for that pathname minus `path` (L148-L167). Written by
    `reference/kit/packages/kit/test/apps/basics/src/hooks.server.js:L33-L45`; cleared by
    `reference/kit/packages/kit/test/apps/basics/test/setup.js:L10`.
  - `read_traces(test_id)`: builds span trees from `test/spans.jsonl` (L170-L185) — OpenTelemetry
    tracing tests, not needed for skgo.
  - `start_server(handler)`: ad-hoc `http.createServer` on a random port with socket tracking and
    teardown; the `context` fixture depends on it to order setup/teardown (L188-L268). Used to test
    external fetches (`reference/kit/packages/kit/test/apps/basics/test/server.test.js:L42-L59`, `L822-L840`).
- Browser selection via `KIT_E2E_BROWSER` (chromium|firefox|webkit), chromium uses `channel: 'chrome'`
  (L271-L293).
- Projects: `${browser}-${mode}` with `javaScriptEnabled: true` and `${browser}-${mode}-no-js` with
  `false`; `KIT_E2E_PROJECT=js|no-js` selects one (L295-L309). Test files opt out with
  `test.skip(({ javaScriptEnabled }) => !javaScriptEnabled)` (client tests) or `=> javaScriptEnabled`
  (server tests) (`reference/kit/packages/kit/test/apps/async/test/client.test.js:L5`,
  `.../server.test.js:L8`).
- Fixed port per app (`test-async` 5300, `test-basics` 5301, `test-no-ssr` 5306, `test-options-2` 5308,
  ...) looked up by `package.json` name (L311-L332).
- `config`: `forbidOnly` in CI, timeout 45 s CI / 15 s local, `webServer` command `pnpm dev --force
  --port N --strictPort` when `DEV` else `pnpm build && pnpm preview --port N --strictPort`, retries 2
  in CI (`KIT_E2E_RETRIES` locally), `workers` from `KIT_E2E_WORKERS` (2 in CI), sharding via
  `KIT_E2E_SHARD=current/total`, screenshots and traces on failure, `testDir: 'test'`, `testMatch`
  `/(.+\.)?(test|spec)\.[jt]s/` (L341-L385). Reporter in CI adds a flaky-warning reporter (L377-L382).

### How each app runs

- `async`: `test` = `test:dev` (`DEV=true playwright test`) then `test:build` (`svelte-kit sync &&
  vitest run && playwright test`); the vitest part builds twice to check chunk-name stability and
  sourcemaps (`reference/kit/packages/kit/test/apps/async/package.json:L11-L13`,
  `reference/kit/packages/kit/test/apps/async/unit-test/node.spec.js:L10-L61`). Its playwright config
  spreads the shared `config` and overrides `webServer.command` without `--force`
  (`reference/kit/packages/kit/test/apps/async/playwright.config.js:L5-L13`).
- `basics`: `node test/setup.js` first (creates a symlink under `src/routes/routing`, deletes
  `test/errors.jsonl`), then `DEV=true playwright test` or `PUBLIC_PRERENDERING=false playwright test`;
  extra scripts for `ROUTER_RESOLUTION=server`, `SVELTE_ASYNC=true`, and `test/cross-platform/`
  (`reference/kit/packages/kit/test/apps/basics/package.json:L10-L20`,
  `reference/kit/packages/kit/test/apps/basics/test/setup.js:L1-L10`,
  `reference/kit/packages/kit/test/apps/basics/playwright.config.js:L1-L16`).
- `no-ssr` and `options-2` re-export or spread the shared config
  (`reference/kit/packages/kit/test/apps/no-ssr/playwright.config.js:L1`).
- The dev server allows serving kit's own `src` (`server.fs.allow: [path.resolve('../../../src')]`)
  because the root layout imports `../../../../setup.js` from `test/`
  (`reference/kit/packages/kit/test/apps/async/vite.config.js:L24-L28`).

### Idioms used across the suites

- Request counting on a URL substring (`/_app/remote`, `__data.json`, `.css`, `.js`):
  `page.on('request', (r) => (count += r.url().includes(X) ? 1 : 0))` then act, then
  `page.waitForTimeout(100)` before `expect(count).toBe(N)`
  (`reference/kit/packages/kit/test/apps/async/test/client.test.js:L75-L81`;
  `reference/kit/packages/kit/test/apps/options-2/test/test.js:L159-L173`).
- Capturing a specific response: `const [response] = await Promise.all([page.waitForResponse(r =>
  r.url().includes('/x')), page.click('#btn')])` (`.../async/test/test.js:L98-L101`).
- Capturing a request URL to replay it with the `request` fixture and forged headers
  (`.../async/test/test.js:L141-L148`).
- Raw HTTP without following redirects: `http.get(baseURL + path)` and read `statusCode`/`location`
  (`.../async/test/test.js:L69-L81`); or `request.get(url, { maxRedirects: 0 })`
  (`reference/kit/packages/kit/test/apps/options-2/test/test.js:L140-L143`).
- Cross-origin/CSRF probes use bare `fetch(baseURL + path, { method, headers: { 'content-type',
  origin } })` (`reference/kit/packages/kit/test/apps/basics/test/server.test.js:L67-L212`).
- Offline simulation: `context.setOffline(true)` / `false` (`.../async/test/client.test.js:L701-L705`).
- Polling for server-side counters exposed by a query: `expect.poll(async () => { await
  page.click('#stats'); return JSON.parse(text).cleanup_count })` (`.../async/test/client.test.js:L755-L762`).
- Serial groups for shared mutable server state: `test.describe.configure({ mode: 'serial' })` plus
  `afterEach` reset (`.../async/test/client.test.js:L63-L71`); otherwise `mode: 'parallel'`
  (`reference/kit/packages/kit/test/apps/no-ssr/test/test.js:L6`).
- Uniqueness per test run via URL params/keys: `` `/route/${Date.now()}${Math.random()}` ``
  (`.../async/test/client.test.js:L220`, `L231`, `L246`).
- Dev-only vs build-only guards: `test.skip(!!process.env.DEV, 'only applicable after build')`
  (`.../async/test/server.test.js:L14`), `if (process.env.DEV) { ... }` around tests that need
  unbuffered streaming because `vite preview` buffers responses
  (`reference/kit/packages/kit/test/apps/basics/test/client.test.js:L1523-L1525`).
- File uploads: `locator.setInputFiles({ name, mimeType, buffer })` including `Buffer.alloc(10 MB)`
  (`.../async/test/test.js:L843-L853`, `L867-L876`).
- Popup handling for `target=_blank`: `page.waitForEvent('popup')` (`.../async/test/test.js:L291-L298`).
- Reading page errors: `page.on('pageerror', ...)` (`.../async/test/client.test.js:L783-L786`) and
  `page.pageErrors()` in options-2 (`reference/kit/packages/kit/test/apps/options-2/test/test.js:L236`).

## Citations

- `reference/kit/packages/kit/test/utils.js:L1-L398`
- `reference/kit/packages/kit/test/setup.js:L1-L29`
- `reference/kit/packages/kit/test/apps/async/playwright.config.js:L1-L14`, `package.json:L6-L13`,
  `unit-test/node.spec.js:L1-L61`
- `reference/kit/packages/kit/test/apps/basics/playwright.config.js:L1-L16`, `package.json:L6-L20`,
  `test/setup.js:L1-L10`, `src/hooks.server.js:L33-L45`
- `reference/kit/packages/kit/test/apps/no-ssr/playwright.config.js:L1`
- `reference/kit/packages/kit/test/apps/options-2/test/test.js:L1-L8`, `L136-L144`, `L156-L173`

## skgo implications

- Reuse `utils.js` almost verbatim, minus tracing and the no-js project: keep `app`, `clicknav`,
  `page` (`body.started` wait), `read_errors` (have the Go server append `errors.jsonl` from its
  `handleError` equivalent, or replace with a `/__test/errors` endpoint), `start_server` (needed for
  external-fetch and cookie-forwarding tests).
- Replace `webServer.command` with the Go binary: build the kit app with the skgo adapter, then run
  the Go server on a fixed `--port`; `DEV` mode maps to whatever skgo's dev story is (kit's `vite dev`
  is not the product).
- Keep the `setup()` root-layout hook so tests can drive navigation programmatically and wait for
  `body.started`; it is plain `$app/navigation` usage and works in CSR.
- Adopt the request-counting idiom as the primary proof that single-flight refreshes and prerendered
  remote outputs work (it is how kit itself proves them).

## Gotchas

- `page.goto` in this harness resolves only after `body.started`; pages that never start (error.html)
  need `{ wait_for_started: false }` (`utils.js:L127-L131`).
- `clicknav` uses `page.waitForNavigation()`, which does not fire for same-document navigations that
  do not change the URL; several tests use `page.click` + `expect(...).toHaveText` instead.
- The `context` fixture is redefined to depend on `start_server`; if you drop `start_server`, drop the
  override too, or Playwright will complain about an unused dependency (`utils.js:L252-L268`).
- Chromium runs with `channel: 'chrome'` — a Chrome install is required, not bundled Chromium
  (`utils.js:L283`).
- Ports are hard-coded per app name; adding an app requires an entry or `port` is undefined and the
  module throws (`utils.js:L311-L332`).

## Recipes

- Minimal fixture file for the skgo example app (derived from `utils.js:L15-L54`, `L113-L145`):
  ```js
  export const test = base.extend({
    app: ({ page }, use) => use({
      goto: (url, opts) => page.evaluate(({ url, opts }) => goto(url, opts), { url, opts }),
      invalidate: (url) => page.evaluate((url) => invalidate(url), url),
      preloadData: (url) => page.evaluate((url) => preloadData(url), url)
    }),
    clicknav: async ({ page }, use) => use(async (sel, { waitForURL } = {}) => {
      await Promise.all([page.waitForNavigation(), page.locator(sel).click(),
        ...(waitForURL ? [page.waitForURL(waitForURL)] : [])]);
    }),
    page: async ({ page }, use) => { /* wrap goto/goBack/reload to wait for body.started */ await use(page); }
  });
  ```
- Root layout `setup()` (from `test/setup.js`): in `onMount`, `Object.assign(window, { goto,
  invalidate, preloadCode, preloadData, beforeNavigate, afterNavigate, match, svelte_tick: tick })`
  and `document.body.classList.add('started')`.
- Playwright config: `defineConfig({ ...config, webServer: { command: '<skgo binary> --port 5300',
  port: 5300 } })` mirroring `async/playwright.config.js:L5-L13`.
