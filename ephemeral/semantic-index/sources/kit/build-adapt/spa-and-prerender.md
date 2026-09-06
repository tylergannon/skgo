# CSR-only (ssr=false) apps: the SPA fallback, what still gets prerendered, prerendered remotes

## Purpose

What kit actually does when the root layout says `export const ssr = false`: how the fallback shell is
produced, which routes are still prerendered and how, how `prerender` remote functions are written at
build time, what the client expects from the server (`__data.json`, `_app/env.js`, version), and what
silently breaks in a CSR-only app that skgo must own instead.

## Key facts

### `ssr = false` is a page option, not config (SOURCE-DERIVED)

- `ssr`/`csr`/`prerender`/`trailingSlash` must be exported from `+page(.server).js` / `+layout(.server).js`;
  exporting them from a `.svelte` file triggers the warning
  "`export const ssr =` will be ignored — move it to +page(.server).js/ts instead"
  (`reference/kit/packages/kit/src/exports/vite/index.js:L56, L76-L98`). There is no `ssr` key in `Config`
  (`reference/kit/packages/kit/types/index.d.ts:L1541-L2108`).
- Build-time handling of a node with `page_options.ssr === false`: the server node module exports the page
  options object *as* `universal` instead of importing the universal module, and does not import the component
  at all when no child page uses SSR (`reference/kit/packages/kit/src/exports/vite/build/build_server.js:L137-L165`).
  In dev the same shortcut applies (`exports/vite/dev/index.js:L261-L270`).
- Client build is skipped only if **every** node has `csr === false` (`exports/vite/index.js:L1232-L1238`); a
  CSR app always gets a client build.

### How the fallback page is generated (SOURCE-DERIVED)

- `builder.generateFallback(dest)` → `generate_fallback({ manifest_path: output/server/manifest-full.js, env, out_dir,
  origin: paths.origin || 'http://sveltekit-prerender', assets: files.assets })` (`core/adapt/builder.js:L141-L160`).
- The forked worker imports `output/server/internal.js`, calls `set_building()`, constructs `new Server(manifest)`,
  `server.init({ env })` and responds to `new Request(origin + '/[fallback]')` with
  `prerendering: { fallback: true, dependencies: new Map(), remote_responses: new Map(), resolved_route_ids: new Set() }`;
  the response body text is the fallback HTML; a non-OK response throws "Could not create a fallback page"
  (`core/postbuild/fallback.js:L18-L53`).
- Hash router: prerender writes the fallback as `pages/index.html` for `/` and returns immediately — nothing else
  is prerendered (`core/postbuild/prerender.js:L158-L176`).
- (DOCS) The fallback "is an HTML page created by SvelteKit from your page template (e.g. `app.html`) that loads your
  app and navigates to the correct route" and "will always contain absolute asset paths … regardless of
  `paths.relative`" (`documentation/docs/25-build-and-deploy/55-single-page-apps.md:L39, L43`).
  Name it `200.html`/anything but `index.html` (`L41`; `50-adapter-static.md:L87`).

### Which routes are still prerendered in a CSR app (SOURCE-DERIVED)

- Prerendering runs whenever any route's `prerender` is truthy **or** any remote function is a `prerender` type
  (`prerender.js:L645-L675`). With ssr=false everywhere and no prerender remotes, prerender returns early with
  empty `prerendered` maps (`L673-L675`).
- Entry points: `config.prerender.entries` (default `['*']` = every prerenderable route with no required
  `[param]`, optional params dropped; `L687-L703`), route-level `entries` exports (`L705-L709`), and prerender
  remotes (`L711-L719`). Then crawl links from 200 HTML responses if `prerender.crawl` (`L473-L518`).
- (DOCS) In SPA mode you opt individual pages back in with `export const prerender = true; export const ssr = true;`
  (`55-single-page-apps.md:L45-L55`); with `ssr = false` left on, prerendering "will save an empty 'shell' page
  instead of the fully rendered content" (`50-adapter-static.md:L49`).
- A route that is prerendered is removed from `output/server/manifest.js` (`exports/vite/index.js:L1527-L1539`) —
  irrelevant to Go but explains why `generateManifest()` omits them.
- Build fails with "Cannot prerender a route with both +page and +server files" (`postbuild/analyse.js:L101-L103`),
  "Cannot prerender a +server file with POST, PUT, PATCH, DELETE handlers" (`L178-L187`), and by default
  `handleUnseenRoutes` fails the build if a `prerender = true` route was never reached (`prerender.js:L739-L750`).

### Prerendered remote functions (SOURCE-DERIVED)

- Before loading remote chunks, prerender sets the manifest and `read` implementation because remote modules
  may touch them at top level (`prerender.js:L654-L657`).
- For each `fn.__.type === 'prerender'`: if `has_arg`, enqueue `remote_prefix + id + '/' + stringify_remote_arg(arg)`
  for every value from `inputs()`; else `remote_prefix + id` (`L711-L719`), where
  `remote_prefix = ${paths.base}/${appDir}/remote/` and `id = '<hash>/<name>'` (`L268`; `exports/vite/index.js:L678`).
- Responses are saved under `output/prerendered/data/…` (category `data`, `L432-L433, L457-L467`) with the URL path as
  filename (no extension, `L248-L256`). `analyse` records `dynamic: type !== 'prerender' || internals.dynamic`
  per export (`analyse.js:L155-L163`).
- Post-prerender, `treeshake_prerendered_remotes` rewrites the server bundle (`exports/vite/index.js:L1541-L1550`) —
  server-only, not skgo's concern.

### What the client expects from *some* server in a CSR app (SOURCE-DERIVED where cited)

- `__data.json`: the client only fetches it for routes whose nodes have a **server** load; the per-node flag
  comes from `has_server_load` in build metadata (`exports/vite/index.js:L1249-L1259, L1419-L1423`) and in dev
  from `!!manifest_data.nodes[i].server` (`dev/index.js:L218-L222`). Therefore Go-implemented loads need a
  `+page.server.*`/`+layout.server.*` file to exist (a stub with a `load` export) so the client bundle knows to
  request data. (Data-request URL format is owned by the runtime/server segment.)
- `<appDir>/env.js`: requested only when the client imports `$app/env/public` **and** at least one public var is
  dynamic (`uses_env_dynamic_public`, `L1347-L1361`); `generateEnvModule()` writes a static one, otherwise the
  server must produce it (`core/adapt/builder.js:L162-L185`).
- `<appDir>/version.json` polled on `version.pollInterval`; `x-sveltekit-version` on data/remote/action responses
  short-circuits polling (`types/index.d.ts:L2059, L2103`).
- `<appDir>/remote/<hash>/<name>[/<arg>]` for remote functions (above).
- Form actions: `POST` to the page URL (route `page.methods` includes `POST` when `actions` exist — `analyse.js:L214-L215`).
- With `router.resolution === 'server'` the client asks the server to resolve routes on navigation and kit builds
  `build_data.client.routes/nodes/css` for that purpose (`exports/vite/index.js:L1391-L1427`). Keep the default
  `'client'` so Go never has to implement route resolution.

### What breaks in a CSR-only app (SOURCE-DERIVED + DOCS)

- No HTML for crawlers, slower first paint, unusable without JS (`55-single-page-apps.md:L7-L9`).
- `+page.server.js` `load`/`actions`, `+server.js` and remote functions have **no** runtime when the server output
  is not deployed — adapter-static simply does not ship `output/server` (`adapter-static/index.js:L76-L91`); those
  are exactly the things skgo reimplements in Go.
- Anything prerendered with `ssr=false` is an empty shell (see above), so prerendering only buys you a cached shell
  plus prerendered `__data.json`/remote payloads unless `ssr = true` is re-enabled per route.
- `handleUnseenRoutes` and `handleHttpError` default to `fail`; a Go-backed page load that is *also* marked
  `prerender = true` will be executed in **Node at build time** by kit's `Server` — only pure-JS loads can be
  prerendered; Go loads cannot participate in kit prerendering.

## Citations

- `reference/kit/packages/kit/src/exports/vite/index.js:L56, L76-L98, L678, L1232-L1259, L1347-L1361, L1391-L1427, L1480-L1550`
- `reference/kit/packages/kit/src/exports/vite/build/build_server.js:L137-L165`
- `reference/kit/packages/kit/src/exports/vite/dev/index.js:L218-L222, L261-L270`
- `reference/kit/packages/kit/src/core/adapt/builder.js:L141-L185`
- `reference/kit/packages/kit/src/core/postbuild/fallback.js:L18-L53`
- `reference/kit/packages/kit/src/core/postbuild/prerender.js:L143-L176, L248-L268, L432-L467, L473-L518, L645-L719, L739-L750`
- `reference/kit/packages/kit/src/core/postbuild/analyse.js:L101-L103, L148-L166, L178-L187, L212-L226`
- `reference/kit/packages/adapter-static/index.js:L9-L52, L66-L91`
- `reference/kit/documentation/docs/25-build-and-deploy/55-single-page-apps.md:L5-L55`
- `reference/kit/documentation/docs/25-build-and-deploy/50-adapter-static.md:L37-L49, L85-L97`

## Go implementation notes

- Fallback dispatch: for `GET`/`HEAD` whose path (after `base`) is not a file in `client/`, not in the prerendered
  set, and not a Go-owned URL (`__data.json`, `/<appDir>/remote/`, endpoint routes), serve the fallback HTML.
  Decide 200 vs 404 by matching the path against page-route ids from the adapter manifest: match → 200; no
  match → 404 with the fallback body (the client router will render its own 404). Never serve the fallback under
  `/<appDir>/immutable/`.
- Prerendered shells: if the team wants prerendered *content* for a route, that route must run in Node at build
  (`ssr = true` + `prerender = true`), so it cannot depend on Go loads. Document this as a project rule.
- `+page.server.*` stubs: skgo's codegen should emit `export const load = () => {}` (or equivalent marker) for
  each Go load so the client fetches `__data.json`; the actual JS body is never executed in production. In dev,
  the kit dev server *would* execute it — which is why Go must intercept `__data.json` before proxying.
- Prerendered remote outputs are a build-time cache Go can serve directly; Go must produce identical bytes for
  dynamic calls (format owned by the remote-server segment).
- `env.js`: recommend Go serves it dynamically (mirrors kit's server behaviour, keeps env out of the build),
  and the adapter does **not** call `generateEnvModule()`.

## Gotchas (kit 3 vs kit 2, `next` churn)

- Kit 3 fallback generation needs `manifest-full.js`, `internal.js`, `env.js` under `output/server` and the
  explicit env config (`set_env` from `<sveltekit:generated>/env/config.js`, `prerender.js:L63-L67`); kit 2's
  `$env/dynamic/*` model is gone.
- `$service-worker` removed → use `$app/manifest` (`immutable`, `assets`, `prerendered`) (`exports/vite/index.js:L65-L70`).
- `prerender.handleInvalidUrl` is new (@since 2.67) and defaults to `fail` (`types:L1963-L1974`) — odd `href`s in
  prerendered HTML now break builds.
- Remote function `prerender` inputs are enumerated at build (`inputs()`), so Go can't add prerendered remote
  arguments later without rebuilding.

## Recipes

- vite.config.js for a CSR app with a custom adapter:
  ```js
  export default defineConfig({ plugins: [sveltekit({ adapter: skgo({ fallback: '__skgo_fallback.html' }),
    experimental: { remoteFunctions: true }, paths: { origin: process.env.ORIGIN } })] });
  ```
  and `src/routes/+layout.js`: `export const ssr = false;` (optionally `export const prerender = true;` on
  routes that must ship static shells/`__data.json`).
- Sanity test for the build: `curl -I /` → fallback 200; `curl /about/__data.json` (prerendered) → JSON from
  `prerendered/`; `curl /_app/immutable/nope.js` → 404 `no-store`.
