# CSR boot: what the fallback shell must contain and what the client does before its first `__data.json`

## Purpose

skgo serves kit's unmodified client bundle from a Go-served HTML shell with `ssr=false`. This leaf pins, from pinned kit source (`@sveltejs/kit` 3.0.0-next.25), exactly what that shell has to contain, which globals and endpoints the client reads before and during `start()`, and the code path the client takes when `start()` is called **without** hydration data. Everything Go must inject or serve for boot is listed under "Go implementation notes".

## Key facts (source-derived)

### Entry points and the `start()` call

- Three client entry files exist. `client-entry.js` re-exports `start`/`load_css` from `client.js` so the rest can be treeshaken (`reference/kit/packages/kit/src/runtime/client/client-entry.js:L1-3`). `entry.js` is the entry for `bundleStrategy: 'split'` (the default) and dev: it exports `init(payload)` which calls `set_payload(payload)` and only *then* dynamically imports `client-entry.js`, "so that modules such as `$app/paths` can read it during initialization"; `start(...args)` awaits that import and forwards (`runtime/client/entry.js:L1-24`). `bundle.js` is the entry for `'single'`/`'inline'` and calls `kit.start(app, element, options)` directly with `app` statically imported from `<sveltekit:generated>/client-optimized/app.js` (`runtime/client/bundle.js:L1-17`).
- Vite input names for split builds: `entry/start` = `runtime/client/entry.js`, `entry/payload` = `runtime/client/payload.js`, `entry/app` = `.svelte-kit/generated/build/client-optimized/app.js`, plus `nodes/N` per node with a component or universal module (`reference/kit/packages/kit/src/exports/vite/index.js:L883-895`). Output file names are `${app_immutable}/[name].[hash].js` with chunks at `${app_immutable}/chunks/[hash].js`; the manifest chunk gets the fixed name `${kit.appDir}/manifest.js` (`exports/vite/index.js:L985-994`).
- After the client build, `build_data.client = { start: <entry/start file>, app: <app.js file>, imports: [start.imports, runtime.file, runtime.imports, app.file, app.imports], stylesheets, fonts, uses_env_dynamic_public }` (`exports/vite/index.js:L1363-1389`). `uses_env_dynamic_public` is true only if the app imports `$app/env/public` **and** at least one public env var is dynamic (`L1351-1361`).
- The server render emits the boot script. For split builds it is literally:
  ```js
  import("<prefixed client.start>").then(async (kit) => {
      kit.init(__sveltekit_<hash>);
      const app = await import("<prefixed client.app>");
      kit.start(app, element[, hydrate_opts]);
  });
  ```
  (`reference/kit/packages/kit/src/runtime/server/page/render.js:L529-542`). `element` is declared as `const element = document.currentScript.parentElement;` (`L479`), so **the boot `<script>` must be a child of the element you want the app mounted into**, i.e. it sits where `%sveltekit.body%` is.
- `args = ['element']` and the hydrate-options object is pushed **only if `page_config.ssr`** (`render.js:L477-520`). With `ssr = false` the call is `kit.start(app, element)` with no third argument — this is what selects the CSR boot path below.
- `prefixed(path)`: paths starting with `/` get `paths.base` prepended (dev), otherwise `${assets}/${path}` (`render.js:L283-292`). In production `client.start`/`client.app` are root-relative file names like `_app/immutable/entry/start.<hash>.js`, so the shell imports `${assets}/_app/immutable/entry/start.<hash>.js`.

### The `__sveltekit_<hash>` global (the "payload")

- The global's name is `__sveltekit_${hash(kit.version.name)}` in builds and `__sveltekit_dev` in dev, where `hash` is the djb2 base-36 hash from `utils/hash.js` (`reference/kit/packages/kit/src/core/utils.js:L24-26`; `utils/hash.js:L5-22`). It is baked into the client bundle as `__SVELTEKIT_GLOBAL_NAME__` (`exports/vite/index.js:L494`).
- The shell assigns it before booting: `__sveltekit_<hash> = { base: <base_expression>, version: "<kit.version.name>"[, assets: "<paths.assets>"][, env: <uneval public env> | null][, defer, resolve] };` (`render.js:L417-425`, `L473-475`). `defer`/`resolve` only exist when SSR produced streamed promise chunks — never for CSR-only. Type: `SvelteKitPayload { version, base, assets?, env?, data?, defer?, resolve? }` (`reference/kit/packages/kit/src/types/internal.d.ts:L751-767`).
- `base_expression` is `JSON.stringify(paths.base)` unless `paths.relative` is on **and** this is not the fallback page (`render.js:L112-140`). For the SPA fallback (`state.prerendering.fallback === true`) it stays absolute, except with hash routing where it becomes `new URL('.', location).pathname.slice(0, -1)` (`L136-139`).
- `payload.js`: `export let payload = __SVELTEKIT_PAYLOAD__ ?? {}` and `set_payload(value)` (`runtime/client/payload.js:L7-12`). In split builds `__SVELTEKIT_PAYLOAD__` is defined as the string `'undefined'`, so `payload` is `{}` until `kit.init(global)` sets it (`exports/vite/index.js:L1019-1022`); in dev and single/inline builds it is the global itself (`L515`, `L1020-1021`).
- Consumers of the payload, all evaluated during module init (hence `init()` before `import`):
  - `$app/paths` client: `base = payload.base ?? __SVELTEKIT_PATHS_BASE__`, `assets = payload.assets ?? base ?? __SVELTEKIT_PATHS_ASSETS__`, `app_dir = __SVELTEKIT_APP_DIR__`, `hash_routing = __SVELTEKIT_HASH_ROUTING__` (`reference/kit/packages/kit/src/runtime/app/paths/internal/client.js:L8-11`).
  - `$app/env` client: `version = payload.version` (`runtime/app/env/client.js:L24`); `browser = true`, `dev = __SVELTEKIT_DEV__`, `building = false`.
  - `$app/env/public` (dynamic public env) client module in builds: `import { payload } from '.../client/payload.js'; const env = payload.env;` (`reference/kit/packages/kit/src/core/env.js:L255-259`). In dev it reads `const { env } = globalThis.__sveltekit_dev`.
  - `_start` seeds remote-function SSR results from `payload.data` (`q`, `p`, `l`, `f` maps) if present (`runtime/client/client.js:L501-510`) — absent for CSR.

### `/_app/env.js` (only when `uses_env_dynamic_public`)

- When the page is prerendered (the fallback counts: `state.prerendering.fallback`) and the client uses dynamic public env, `load_env_eagerly` is true: the shell adds `modulepreload` for `${app_dir}/env.js` and wraps the boot in `import("<base>/_app/env.js").then(({ env }) => { __sveltekit_<hash>.env = env; <boot> })`, with the payload's `env` property set to `null` (`render.js:L374-379`, `L423-425`, `L544-549`).
- The server answers `GET <base>/_app/env.js` with `export const env=<devalue.uneval(rendered_env)>`, `content-type: application/javascript; charset=utf-8`, a weak ETag, and 304 on `if-none-match` (`reference/kit/packages/kit/src/runtime/server/env_module.js:L17-32`; dispatch in `runtime/server/respond.js:L356-358`). Any other unmatched `/_app/*` path is a 404 with `cache-control: public, max-age=0, must-revalidate` (`respond.js:L360-365`).

### `%sveltekit.*%` placeholders and head contents

- `src/app.html` must contain `%sveltekit.head%` and `%sveltekit.body%` (`reference/kit/packages/kit/src/core/config/index.js:L86-91`). The generated server replaces `%sveltekit.head%`, `%sveltekit.body%`, `%sveltekit.assets%` (→ `assets`), `%sveltekit.nonce%`, `%sveltekit.version%` (inlined at build), and `%sveltekit.env.NAME%` (→ `env[NAME] ?? ""` from `explicit_public_env`) (`reference/kit/packages/kit/src/core/sync/write_server.js:L36-46`; `render.js:L620-626`).
- Head for a CSR page = `[...http_equiv, ...link_tags, rendered_head(''), ...style_tags, ...stylesheet_links].join('\n\t\t')` (`render.js:L683-691`). Link tags are `<link href=... rel="modulepreload">` for every entry in `modulepreloads` (= `client.imports` ∪ each branch node's `imports`) unless `output.linkHeaderPreload` moves them to a `Link` response header (`render.js:L77`, `L263-266`, `L313-322`, `L381-389`). Stylesheets are `<link href=... rel="stylesheet">` (`L324-340`). Fonts get `rel="preload" as="font"` (`L342-350`).
- Body for CSR = `rendered.body` (empty string, `L259-261`) + the boot `<script>` (`L583-585`). Response headers: `x-sveltekit-page: true`, `content-type: text/html`, `etag: "<hash(html)>"` (`L588-591`, `L636`); when prerendering, CSP goes into `<meta http-equiv>` instead of headers (`L593-604`).
- kit's own SPA fallback generator simply renders `GET <origin>/[fallback]` with `prerendering: { fallback: true, ... }` and takes `response.text()` (`reference/kit/packages/kit/src/core/postbuild/fallback.js:L18-53`). That output is exactly the shell described here.

### What the client does on `start(app, element)` with no data

- `start` → `_start` (`client.js:L466-474`, `L494-606`). Order inside `_start`:
  1. DEV warning if the target is `document.body` (`L495-499`).
  2. Seed `query_responses`/`prerender_responses` from `payload.data` (`L501-510`).
  3. If `document.URL !== location.href` (basic-auth credentials in URL) reload (`L515-518`).
  4. `app = _app; init_transport(app.hooks.transport ?? {}); await app.hooks.init?.()` (`L520-524`) — the client `init` hook runs **before** any routing.
  5. `routes = parse_routes(app)` when `__SVELTEKIT_CLIENT_ROUTING__` (`L526`); `container = document.documentElement` unless embedded (`L527`).
  6. Eagerly import `nodes[0]` (root layout) and `nodes[1]` (root error) (`L532-535`) and build the root `Props`/`RenderNode` (`L538-546`).
  7. Read/initialise history metadata under `history.state['sveltekit:metadata']` (`L548-577`).
  8. **Because `data` is undefined**: `await navigate({ type: 'enter', url: resolve_url(app.hash ? decode_hash(new URL(location.href)) : location.href), replace_state: true, state, persist_state })` (`L589-603`). `_hydrate` is only used when hydrate options are passed (`L592`).
  9. `_start_router()` installs click/submit/popstate/hashchange listeners, `visibilitychange`/`focus` version checks, and preload observers (`L605`, `L3151-3529`).
- `navigate` for `type: 'enter'`: `intent = await get_navigation_intent(url, false)` (`L1997`); `create_navigation(...)` without firing `beforeNavigate` (`L1998-2000`); `load_route(intent)` (`L2033`) which is where the first `__data.json` fetch happens if any invalid server-load node exists (see `data-fetching.md`); then, since `started` is still false, `initialize(navigation_result, target, false)` → `mount(Root, { target, props, transformError })` (not `hydrate`) (`L2237-2239`, `L922-984`). `history.replaceState` is used (not push) because `replace_state: true` (`L2140-2157`), and the URL pathname is normalised to the route's trailing-slash option first (`L2124-2127`).
- If no route matches the URL on boot: `navigation_result` is undefined; the URL is not external (same origin, under `base`), so `server_fallback(url, { id: null }, <404 SvelteKitError>)` runs (`L2035-2058`). Because `url.pathname === location.pathname && !hydrated`, it does **not** reload — it renders the root layout + root `+error.svelte` with status 404 via `load_root_error_page` (`L2328-2337`, `L1696-1783`). Any 404 page under skgo therefore comes from the client, not Go, as long as Go serves the shell for unknown paths.
- `load_root_error_page` fetches `__data.json` with `?x-sveltekit-invalidated=1` **only if `app.server_loads[0] === 0`** (root layout has a server load) (`L1703-1719`). If that fetch fails with a non-404 error and we are on the same, un-hydrated pathname it still does not reload (`L1720-1730`) — loops are avoided.
- After a navigation result with `page.status >= 400` the client calls `updated.check()`; if `version.json` reports a different version it does a full `location.href = url` reload (`L2102-2107`). Same when a node's dynamic import fails (`L1575-1581`).
- `version.json`: the client build emits `${appDir}/version.json` with body `{"version":"<kit.version.name>"}` (`exports/vite/index.js:L1144-1148`). The client fetches `${assets}/${__SVELTEKIT_APP_VERSION_FILE__}` with header `cache-control: no-cache`, treats non-OK as "not updated", and compares `data.version !== version` (`reference/kit/packages/kit/src/runtime/app/state/client.svelte.js:L158-171`). Checks run on a poll timer only if `version.pollInterval > 0` (`L131`, `L176`, `L182-184`), on `focus`, on `visibilitychange` → visible (`client.js:L3187-3198`), and on the >=400 / failed-import paths above. `updated.check()` is a no-op in DEV (`L150`).
- `x-sveltekit-version` on `__data.json`/remote responses is fed to `notify_version`, which flips `updated.current` when it differs from `payload.version` (only if `__SVELTEKIT_APP_VERSION_CHECKS_ENABLED__`, i.e. not the inline bundle strategy) (`state/client.svelte.js:L196-200`; `runtime/server/utils.js:L47-52`).
- `app.hash` (hash routing): URL path for matching comes from `url.hash` (`client.js:L1895-1901`); the server returns 404 for every path except `base + '/'` and `/[fallback]` (`respond.js:L137-139`). Not the mode skgo wants unless every deep link must be `/#/...`.

## Citations

- `reference/kit/packages/kit/src/runtime/client/client-entry.js:L1-3`
- `reference/kit/packages/kit/src/runtime/client/entry.js:L1-24`
- `reference/kit/packages/kit/src/runtime/client/bundle.js:L1-17`
- `reference/kit/packages/kit/src/runtime/client/payload.js:L1-17`
- `reference/kit/packages/kit/src/runtime/client/client.js:L466-606` (start/_start), `L922-984` (initialize), `L1696-1783` (load_root_error_page), `L1977-2260` (navigate), `L2328-2348` (server_fallback), `L3151-3529` (_start_router), `L3536-3646` (_hydrate, for contrast)
- `reference/kit/packages/kit/src/runtime/app/paths/internal/client.js:L8-11`
- `reference/kit/packages/kit/src/runtime/app/env/client.js:L1-24`
- `reference/kit/packages/kit/src/runtime/app/state/client.svelte.js:L131-200`
- `reference/kit/packages/kit/src/runtime/server/page/render.js:L75-140`, `L259-300`, `L313-425`, `L473-591`, `L620-663`, `L665-723`
- `reference/kit/packages/kit/src/runtime/server/env_module.js:L17-32`
- `reference/kit/packages/kit/src/runtime/server/respond.js:L137-139`, `L356-365`
- `reference/kit/packages/kit/src/core/postbuild/fallback.js:L18-53`
- `reference/kit/packages/kit/src/core/adapt/builder.js:L141`; `reference/kit/packages/kit/src/exports/public.d.ts:L158`
- `reference/kit/packages/kit/src/core/sync/write_server.js:L31-49`
- `reference/kit/packages/kit/src/core/config/index.js:L84-91`
- `reference/kit/packages/kit/src/core/env.js:L218`, `L255-259`
- `reference/kit/packages/kit/src/core/utils.js:L24-26`; `reference/kit/packages/kit/src/utils/hash.js:L5-22`
- `reference/kit/packages/kit/src/exports/vite/index.js:L335-336`, `L478-518`, `L883-895`, `L978-1022`, `L1144-1148`, `L1351-1389`
- `reference/kit/packages/kit/src/types/internal.d.ts:L751-767`

## Go implementation notes

1. **Do not hand-write the shell.** Have the skgo adapter call kit's own fallback generation (`builder.generateFallback(dest)`, `reference/kit/packages/kit/src/core/adapt/builder.js:L141`; typed at `exports/public.d.ts:L158`; it runs `core/postbuild/fallback.js`) and ship that file inside the Go binary. It already contains the correct `__sveltekit_<hash>` assignment, the modulepreload/stylesheet links for `client.imports`, and the `kit.init(...); kit.start(app, element)` boot with the hashed file names. Go's job is then: serve it for every path that is not a static asset, not `/_app/*`, not a `__data.json`, not a remote endpoint, and not a Go-owned `+server` route.
2. **Serve it with the right shape**: `content-type: text/html`, status 200 (kit itself serves the fallback as 200; the client decides 404 by route matching). Add `x-sveltekit-page: true` for parity; an `etag` computed from the file is fine.
3. **`/_app/version.json`** must be served from the client build output (it is a real emitted asset) with the same `version` string that the shell's payload carries. If Go embeds assets from multiple builds or rewrites version names, the strings must agree or every 4xx page turns into a reload loop attempt.
4. **`/_app/env.js`** is required only when `client.uses_env_dynamic_public` (readable from the adapter's `builder` / server manifest `_.client`). If Go serves it, body must be `export const env={...}` (a JS object literal, `devalue.uneval` output — plain JSON is a valid subset for string values) with `content-type: application/javascript; charset=utf-8`; keep the weak-ETag/304 behaviour. If the app does not use dynamic public env, Go can 404 it; the shell will not request it.
5. **`/_app/immutable/*`** files must be served byte-exact with long-lived caching; the shell references hashed names, and the client imports `nodes/N.<hash>.js` chunks lazily on navigation. A 404 on a chunk after redeploy triggers `updated.check()` → reload, which only converges if `version.json` changed.
6. **Base path**: if `paths.base` is set, the shell's payload `base` is absolute; Go must mount everything (`/_app`, pages, `__data.json`) under that prefix and treat paths outside it as not-app.
7. **Do not rely on the shell's `%sveltekit.env.X%`** for runtime values: those are inlined at build (`explicit_public_env`); runtime values flow via `/_app/env.js` only.
8. **Trusting the client's 404**: because unknown paths render the client's root error page, Go must serve the shell (200) for them unless Go wants to serve its own error page. Serving a Go 404 HTML page for unknown paths is fine too, but then deep links to client-only routes must still get the shell — Go needs the route list (see `navigation-and-manifest.md`) to decide.

## Gotchas

- `element = document.currentScript.parentElement`: if you inline the boot script somewhere else (e.g. `<head>`), the app mounts in the wrong element. Keep the script where `%sveltekit.body%` was, inside a wrapper `<div style="display: contents">` (kit warns against `document.body` as target: `client.js:L495-499`).
- `kit.init(payload)` must run before `import(app.js)`; otherwise `$app/paths` and `$app/env` capture `{}` and `base`/`version` are undefined. If you generate your own shell, copy the exact three-step boot.
- The `__sveltekit_<hash>` name changes whenever `kit.version.name` changes (default: build timestamp). Never hard-code it in Go; read it from the generated fallback or from the adapter's config.
- `payload.version` vs `version.json` mismatch is not fatal by itself, but `updated.current` becomes true and the next 4xx navigation reloads the page.
- `x-sveltekit-version` on `__data.json` responses should equal `kit.version.name`; omitting the header is safe (`notify_version(null)` is a no-op), sending a wrong one is not.
- With `ssr=false`, `%sveltekit.head%` contains no page-specific `<title>`/meta; that is expected.
- `hooks.init` (client) is awaited before route matching; a throwing `init` aborts boot with no rendered error page.
- `__SVELTEKIT_HAS_SERVER_LOAD__` is set from the server build's metadata (`exports/vite/index.js:L1250-1258`). If **no** node in the kit project has a server `load`, the client bundle has `load_data` compiled out and will never request `__data.json` regardless of what Go does. Go-owned loads must still be visible to kit's build as `+page.server.ts`/`+layout.server.ts` files exporting `load` (or `trailingSlash`) — see `navigation-and-manifest.md`.

## Recipes

- **Minimal Go handler set for boot**: `GET /_app/immutable/**` (static, immutable), `GET /_app/version.json` (static), `GET /_app/env.js` (dynamic, only if needed), `GET /favicon.*`/static dir, everything else that `Accept`s HTML → fallback shell (200, `text/html`).
- **Verify boot without a browser**: fetch the shell, extract the `__sveltekit_[0-9a-z]+` name and the `import("...entry/start...")`/`import("...entry/app...")` paths, and assert each resolves to a 200 `application/javascript` from Go; assert `/_app/version.json` returns `{"version":<same as payload>}`.
- **Playwright smoke**: load `/some/unknown/path`, expect the root `+error.svelte` content and `page.status === 404` with no navigation/reload; load a real route and expect exactly one `GET .../__data.json?x-sveltekit-invalidated=...` per server-load node group.
