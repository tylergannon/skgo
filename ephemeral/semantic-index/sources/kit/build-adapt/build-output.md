# Kit 3 build output layout (`.svelte-kit/output/*`) and what adapters copy

## Purpose

The exact on-disk layout `vite build` (`vp build`) leaves in `<outDir>/output/`, which files matter to a
Go server that serves the client natively, where `_app/immutable`, `_app/version.json`, `_app/manifest.js`,
`_app/env.js`, prerendered `__data.json` and remote responses live, and how `appDir`, `paths.base`,
`paths.assets`, `paths.relative` and `paths.origin` change URLs.

## Key facts

### Build phases (SOURCE-DERIVED, `reference/kit/packages/kit/src/exports/vite/index.js`)

1. `config` hook: `out_dir = posixify(kit.outDir)`, `out = ${out_dir}/output` (`L331-L333`); on build,
   `manifest_data = create_manifest_data(...)` and `sync.all(...)` (`L509-L510`).
2. `plugin_compile.config` (build): defines `server_input` (`index`, `internal`, `env`, `remote-entry`,
   `entries/endpoints/**`, `entries/pages/**`, `entries/fallbacks/*`, `entries/params`,
   `entries/hooks.server`, `entries/hooks.universal`, `instrumentation.server`) — `L821-L878`;
   `client_input` for `split` strategy is `entry/start`, `entry/payload`, `entry/app`, `nodes/<i>` — `L883-L895`.
3. Vite `base` is `'./'` when `paths.relative !== false || !!paths.assets`, else `(paths.assets || paths.base) + '/'`
   (`L900-L917`). `renderBuiltUrl` returns relative URLs for JS on the client and absolute (`base + filename`)
   on SSR (`L1025-L1053`).
4. Output naming (`L957-L1024`):
   - SSR: `outDir: ${out}/server`, `target: 'node22'`, `entryFileNames: '[name].js'`, `chunkFileNames: 'chunks/[name].js'`,
     `copyPublicDir: false`.
   - Client: `outDir: ${out}/client`, `entryFileNames: '<appDir>/immutable/[name].[hash].js'`,
     chunk names `'<appDir>/immutable/chunks/[hash].js'` **except** the `sveltekit-manifest` chunk which is
     written to the *non-hashed* `'<appDir>/manifest.js'` (`L986-L1005`); assets to
     `'<appDir>/immutable/assets/[name].[hash][extname]'` (`L926`); `format` is `iife` for `inline`, else `esm`.
   - `build.manifest: true` → `.vite/manifest.json` in both dirs (`L922`); `ssrEmitAssets: true` (`L951`).
   - `publicDir: kit.files.assets` → the contents of `static/` are copied into `output/client/` by Vite (`L1054`;
     the SSR env has `copyPublicDir: false` so they are not duplicated under `server/`).
5. `generateBundle` (client only) emits `<appDir>/version.json` with `{"version": "<kit.version.name>"}` (`L1142-L1148`).
6. `buildApp` (`L1151-L1611`): `rm -rf out`; SSR build; replace `__SVELTEKIT_MANIFEST_*` placeholders
   (`L1161-L1169`, `L1785-L1861`); read `server/.vite/manifest.json`; write `server/manifest-full.js`
   (`L1175-L1199`); `build_server_nodes` (`L1202-L1211`, `build/build_server.js`); `analyse` (`L1215-L1225`);
   client build unless *every* node has `csr === false` (`L1232-L1238`); copy server-emitted assets into
   `client/<appDir>/immutable/assets` except CSS already in the client bundle (`L1268-L1292`); compute
   `immutable` list from the client manifest (`L1335`, `L1750-L1769`); build `build_data.client`
   (`L1363-L1451`); rewrite `manifest-full.js` (`L1454-L1464`); rebuild nodes with the client manifest
   (`L1467-L1476`); prerender (`L1480-L1505`); replace the `prerendered` sentinel in both output trees
   (`L1510-L1515`); write `server/manifest.js` **without** `prerender === true` routes (`L1527-L1539`);
   `treeshake_prerendered_remotes` (`L1541-L1550`); `finalise` → service worker → `adapt` (`L1553-L1610`).

### Resulting directory tree (SOURCE-DERIVED)

```
<outDir>/                              # default .svelte-kit
  generated/{build,dev}/...            # kit-internal, never served
  output/
    client/                            # builder.getClientDirectory()
      .vite/manifest.json              # excluded by writeClient
      <static/**>                      # copy of config.files.assets (publicDir)
      <appDir>/version.json            # {"version":"..."}; NOT immutable
      <appDir>/manifest.js             # client route manifest chunk (split strategy); NOT immutable, not hashed
      <appDir>/immutable/entry/start.<hash>.js
      <appDir>/immutable/entry/app.<hash>.js
      <appDir>/immutable/entry/payload.<hash>.js
      <appDir>/immutable/nodes/<i>.<hash>.js
      <appDir>/immutable/chunks/<hash>.js
      <appDir>/immutable/assets/<name>.<hash>.<ext>   # css, fonts, images, wasm...
      service-worker.js                # only if src/service-worker.* exists
    server/                            # builder.getServerDirectory(); skgo does not ship this
      index.js internal.js env.js remote-entry.js
      manifest.js manifest-full.js
      nodes/<i>.js stylesheets/*.js chunks/*.js chunks/remote-<hash>.js
      entries/{pages,endpoints,fallbacks}/** entries/params.js entries/hooks.server.js entries/hooks.universal.js
      instrumentation.server.js        # optional
      <appDir>/immutable/assets/**     # ssrEmitAssets
      .vite/manifest.json
    prerendered/
      pages/**.html                    # prerendered pages (`foo.html` or `foo/index.html`)
      dependencies/**                  # e.g. foo/__data.json, favicon.ico, <appDir>/env.js (generateEnvModule)
      data/<appDir>/remote/<hash>/<name>[/<arg>]   # prerendered remote-function responses
```

- `output_filename(path, is_html)`: strips `base` + leading `/`, empty → `index.html`; HTML responses get
  `.html` appended (`/foo` → `foo.html`, `/foo/` → `foo/index.html`); non-HTML keeps the path verbatim —
  `reference/kit/packages/kit/src/core/postbuild/prerender.js:L248-L256`.
- Category rule in `save()`: a *visited* URL under `remote_prefix` (`<base>/<appDir>/remote/`) goes to `data/`,
  else `pages/`; a *fetched dependency* goes to `data/` if under `remote_prefix`, else `dependencies/` —
  `prerender.js:L432-L433, L457-L467`. `__data.json` files are dependencies, so they land in `dependencies/`.
- Redirect responses become tiny HTML files `<script>location.href=...;</script><meta http-equiv="refresh" ...>`
  in `pages/` (unless `x-sveltekit-normalize` is set) and are recorded in `prerendered.redirects` —
  `prerender.js:L552-L590`.
- Prerender refuses to crawl anything already in `output/client` plus `<appDir>/env.js` and files under
  `output/server/<appDir>/immutable` (`prerender.js:L258-L266, L352-L354`).
- `Prerendered.paths` are pushed as the *decoded request path* (`prerender.js:L582, L623`); the type comment says
  "without trailing slashes, regardless of the trailingSlash config" (`types/index.d.ts:L883`) but the map keys
  follow the crawled URL (`/foo/` → `foo/index.html`). Treat both spellings as equivalent (adapter-node does).

### What the adapters produce (SOURCE-DERIVED)

- **adapter-node** (`reference/kit/packages/adapter-node/index.js:L20-L44`):
  `build/client<base>/**` (writeClient), `build/prerendered<base>/**` (writePrerendered), optional `.gz`/`.br`
  siblings via `builder.compress` on both dirs, then a Rolldown bundle of `index.js`, `env.js`, `handler.js`,
  `server/chunks/*`, with `server/manifest.js` exporting `manifest`, `prerendered` (Set of paths), `base`,
  `uncompressed_extensions` (`L68-L76`).
- **adapter-static** (`reference/kit/packages/adapter-static/index.js:L66-L91`): `rm -rf pages, assets`;
  `generateEnvModule()`; `writeClient(assets)`; `writePrerendered(pages)`; `generateFallback(<pages>/<fallback>)`
  if set; `compress` if `precompress`. Defaults `pages = 'build'`, `assets = pages`, `precompress` falsy.
  No manifest of any kind is written (Vercel zero-config aside, `platforms.js`).

### `immutable` list and `$app/manifest` (SOURCE-DERIVED)

- `collect_immutable` = every `file`, `css[]` and `assets[]` in the client Vite manifest whose path starts with
  `<appDir>/immutable`, minus inlined bundle files (`exports/vite/index.js:L1750-L1769`). This is the authoritative
  "safe to cache forever" list and is baked into the bundles as `__SVELTEKIT_MANIFEST_IMMUTABLE__`
  (`L1788-L1790`, `L1345`), together with `assets` (static files) and `routes` (`{id, page, endpoint}`), and
  `prerendered` paths (relative, base stripped: `p.replace(kit.paths.base, '').slice(1)` — `L1510-L1512`).

### `appDir` / `paths.*` effects (SOURCE-DERIVED + DOCS)

- `appPath = base.slice(1) + (base ? '/' : '') + appDir` (`exports/vite/index.js:L1181`); used by adapter-node's
  immutable check as `/${manifest.appPath}/immutable/` (`adapter-node/src/handler.js:L62`).
- `paths.assets` set → two app dirs, `${paths.assets}/${appDir}` for the bundle and `${paths.base}/${appDir}`
  for internal routes (`types/index.d.ts:L1560`). In dev/preview a fake prefix `/_svelte_kit_assets` is used
  (`reference/kit/packages/kit/src/constants.js:L5`; `dev/index.js:L475`; `preview/index.js:L24`).
- `paths.relative` default `true` → client JS/CSS reference each other relatively (Vite `base: './'`), which is
  why prerendered HTML is portable; SPA fallback is always absolute (`types:L1858`, `55-single-page-apps.md:L43`).
- `paths.origin` (3.0) is compiled in as `__SVELTEKIT_PATHS_ORIGIN__` (`L497`) and used as the prerender origin
  (`prerender.js:L156`; `fallback` too, `builder.js:L149`).
- `define`s worth knowing because they change client behaviour: `__SVELTEKIT_APP_VERSION_FILE__ = '<appDir>/version.json'`,
  `__SVELTEKIT_APP_VERSION_POLL_INTERVAL__`, `__SVELTEKIT_CLIENT_ROUTING__`, `__SVELTEKIT_HASH_ROUTING__`,
  `__SVELTEKIT_CSRF_CHECK_ORIGIN__` (`L478-L507`).

## Citations

- `reference/kit/packages/kit/src/exports/vite/index.js:L331-L333, L478-L507, L509-L510, L821-L1055, L1142-L1148, L1151-L1611, L1750-L1861`
- `reference/kit/packages/kit/src/exports/vite/build/build_server.js:L28-L279`
- `reference/kit/packages/kit/src/exports/vite/build/utils.js:L13-L117`
- `reference/kit/packages/kit/src/core/postbuild/prerender.js:L248-L268, L352-L354, L432-L467, L530-L635`
- `reference/kit/packages/kit/src/core/adapt/builder.js:L216-L235`
- `reference/kit/packages/adapter-node/index.js:L20-L76`
- `reference/kit/packages/adapter-static/index.js:L66-L91`
- `reference/kit/packages/kit/src/constants.js:L5`
- `reference/kit/packages/kit/types/index.d.ts:L1557-L1563, L1828-L1868, L2078-L2107`

## Go implementation notes

- Read one JSON (from the adapter) and two directories: `client/` (static + `_app`) and `prerendered/`.
  Do not read `output/server/`.
- Cache policy by path (all relative to `base`):
  - `/<appDir>/immutable/**` → `Cache-Control: public,max-age=31536000,immutable` on 200 only; 404 for misses
    with `no-store` (adapter-static's Vercel rule, `platforms.js:L76-L88`) so a missing hashed file is never cached.
  - `/<appDir>/version.json`, `/<appDir>/manifest.js`, `/<appDir>/env.js` → no immutable header; short/`no-cache`.
  - everything else from `static/` → etag, `max-age=0` (what preview/dev do).
- `_app/manifest.js` (non-hashed, `split` strategy) is loaded by the client on boot — serve it with the JS MIME type
  and never with immutable caching; cache-bust relies on `version.json` / `x-sveltekit-version`.
- Prerendered `__data.json` files (`prerendered/<route>/__data.json`) are static responses Go can serve verbatim
  for routes whose server load was prerendered; for everything else Go computes `__data.json` itself.
- Prerendered remote responses live at `prerendered/<appDir>/remote/<hash>/<name>[/<arg>]` with no extension;
  serve as `application/json` (kit's preview uses `lookup(pathname) || 'text/html'`, which would be wrong for
  these — use the content-type recorded in `prerendered.assets` or hard-code JSON).
- Version: `x-sveltekit-version` header on data/remote/action responses lets the client detect new deploys without
  polling (`types/index.d.ts:L2059`); Go should emit it with `version.name` from the manifest.

## Gotchas (kit 3 vs kit 2, `next` churn)

- New in 3: `entry/payload` client entry, `_app/manifest.js` fixed-name chunk, `__SVELTEKIT_MANIFEST_*` sentinel
  replacement (`L1785-L1861`), `remote-entry.js` server entry, `env.js` server entry from `<sveltekit:generated>/env/config.js`.
  Kit 2 layouts without these are stale.
- `output/server/manifest.js` excludes prerendered routes; `manifest-full.js` includes all. The SPA fallback is
  generated from `manifest-full.js` (`builder.js:L142`).
- With `bundleStrategy: 'inline'` the entry file is deleted after being inlined into HTML (`L1439-L1450`) — the
  client dir will not contain a start script. Use `split` (default) for skgo.
- If every node has `csr === false` the client build is skipped entirely and only server-emitted assets + `static`
  are copied to `client/` (`L1232-L1238`) — irrelevant for a CSR app, but a footgun if someone flips `csr`.
- The target is `node22` for the server build (`L962`); irrelevant to Go but confirms the sidecar/build Node floor.

## Recipes

- Enumerate immutable files without the adapter: parse `client/.vite/manifest.json` (before `writeClient` filters
  it out) and apply the `collect_immutable` rule; or just treat the `<appDir>/immutable/` prefix as the rule,
  which is what every kit adapter does.
- To validate a build dir at Go startup: require `client/<appDir>/version.json`, `client/<appDir>/immutable/entry/start.*.js`,
  the fallback HTML, and `skgo.manifest.json`; log the version.
