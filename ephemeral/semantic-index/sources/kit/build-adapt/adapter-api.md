# Kit 3 adapter API (`Builder`, `Adapter`, `Emulator`) — as pinned at 3.0.0-next.25

## Purpose

What an adapter is handed after `vite build`, exactly which methods exist on `Builder` today, what
`builder.config` contains, and how a ~60-line `adapter-skgo` would emit everything the Go binary
needs (client dir, prerendered dir, SPA fallback, route list, prerendered path list, remote hashes).
This is the deciding file for "adapter-static SPA mode vs custom adapter".

Pinned source: `reference/kit` at commit `a59327223dd1c6bde4dac71c7946b9603f6995f7`
("Version Packages (next) (#16856)"), `packages/kit/package.json` version `3.0.0-next.25`.

## Key facts

### The `Adapter` object (SOURCE-DERIVED)

- Shape: `{ name, adapt(builder), supports?: { read?, instrumentation? }, emulate?(), vite?: { plugins?: { pre?, post? } } }` —
  `reference/kit/packages/kit/types/index.d.ts:L15-L60`. `name` and `adapt` are required; the rest optional
  (`reference/kit/documentation/docs/25-build-and-deploy/99-writing-adapters.md:L51`).
- `adapter.vite.plugins.pre` / `.post` are **new in 3.0.0** (`types/index.d.ts:L46-L59`) and are spliced
  around kit's own plugin list: `reference/kit/packages/kit/src/exports/vite/index.js:L1704-L1738`
  (`svelte_config.adapter?.vite?.plugins?.pre` first, `.post` last). An adapter can therefore inject
  its own Vite plugin — e.g. one with a `buildApp` hook — without the user editing `vite.config.js`.
- `supports.instrumentation()` is consulted during both dev and build when `src/instrumentation.server.*`
  exists (`exports/vite/index.js:L870-L878`, `exports/vite/dev/index.js:L578-L590`); if the adapter lacks it the
  build throws `"<file> is unsupported in <adapter.name>"`. `supports.read` is consulted per-route by
  `check_feature` for `$app/server:read` (`core/postbuild/analyse.js:L118-L127`).
- `emulate()` returns an `Emulator` with only one method: `platform({ config, prerender })` → `App.Platform`
  (`types/index.d.ts:L329-L335`). It is invoked in dev (`dev/index.js:L507`), preview (`preview/index.js:L66`)
  and prerender (`postbuild/prerender.js:L178`).
- Validation of the `adapter` config key: must be an object with an `adapt` method —
  `reference/kit/packages/kit/src/exports/vite/options.js:L6-L15`. Missing adapter is only a warning
  ("No adapter specified") at the end of the build (`exports/vite/index.js:L1602-L1608`).

### When `adapt()` runs (SOURCE-DERIVED)

- `vite build` → kit's `plugin_compile.buildApp` builds SSR, analyses, builds client, prerenders, writes
  `output/server/manifest.js`, tree-shakes prerendered remotes, then stores a `finalise` closure
  (`exports/vite/index.js:L1151-L1611`). `plugin_adapter` (`buildApp`, `order: 'post'`) runs `finalise`,
  which builds the service worker (if any) and calls `adapt(...)` (`exports/vite/index.js:L1690-L1702`,
  `L1553-L1610`).
- `core/adapt/index.js` prints `> Using <name>`, builds the `Builder`, awaits `adapt(builder)`, logs `done`
  (`reference/kit/packages/kit/src/core/adapt/index.js:L26-L59`).
- **Deprecation:** `builder.config.kit` still exists but reading it warns once:
  `"Reading \`config.kit\` inside adapters is deprecated — it should access configuration on the \`config\` object directly"`
  (`core/adapt/index.js:L33-L45`). Kit 2 adapters that read `builder.config.kit.paths.base` keep working;
  a new adapter must read `builder.config.paths.base`.
- `builder.routes` is only routes with a page or an endpoint:
  `build_data.manifest_data.routes.filter((route) => route.page || route.endpoint)` (`core/adapt/index.js:L48`).

### The `Builder` surface (SOURCE-DERIVED, `core/adapt/builder.js`)

| member | what it does | cite |
|---|---|---|
| `log` | `Logger` (`(msg)`, `success`, `error`, `warn`, `minor`, `info`, `err`, `prettyError`); `info`/`minor` silent unless Vite `logLevel` is `info` | `types/index.d.ts:L109-L110, L833-L848` |
| `rimraf`, `mkdirp` | deprecated wrappers over `fs.rmSync`/`fs.mkdirSync` | `builder.js:L108-L109`; `types:L111-L120` |
| `copy(from, to, { filter?, replace? })` | recursive copy, returns written files | `builder.js:L110`; `types:L195-L202` |
| `config` | the `ValidatedConfig` (fully resolved kit config) — see "what the adapter sees" | `builder.js:L112` |
| `prerendered` | `{ pages: Map<path,{file}>, assets: Map<path,{type}>, redirects: Map<path,{status,location}>, paths: string[] }` | `builder.js:L113`; `types:L852-L885` |
| `routes` | `RouteDefinition[]` facade built from internal `RouteData` + `ServerMetadata`: `{ id, api:{methods}, page:{methods}, segments:[{dynamic,rest,content}], pattern: RegExp, prerender, methods, config }` | `builder.js:L76-L104`; `types:L628-L641` |
| `compress(dir)` | writes `<file>.gz` (gzip `Z_BEST_COMPRESSION`) and `<file>.br` (brotli `BROTLI_MODE_TEXT`, `BROTLI_MAX_QUALITY`, size hint) next to every file whose extension is in `['.html','.js','.mjs','.json','.css','.svg','.xml','.wasm','.txt','.md','.mdx']`; returns the list of files compressed | `builder.js:L31-L43, L116-L131, L290-L306` |
| `findServerAssets(routes)` | assets imported by server files for the given routes | `builder.js:L133-L139`; `generate_manifest/find_server_assets.js` |
| `generateFallback(dest)` | renders the SPA shell by running kit's `Server` against `output/server/manifest-full.js` for URL `<origin>/[fallback]` with `prerendering.fallback = true`; origin is `config.paths.origin \|\| 'http://sveltekit-prerender'`; warns if `dest` exists | `builder.js:L141-L160`; `postbuild/fallback.js:L18-L53` |
| `generateEnvModule()` | if the client bundle uses dynamic public env, writes `output/prerendered/dependencies/<appDir>/env.js` containing `export const env={...}` (devalue) with the *build-time* values of non-static public vars from the explicit env config | `builder.js:L162-L185` |
| `generateManifest({ relativePath, routes? })` | returns **JS source text** (not JSON) of an `SSRManifest` expression; default route subset excludes routes whose `prerender === true` | `builder.js:L187-L198`; `generate_manifest/index.js:L26-L163` |
| `getBuildDirectory(name)` | `<outDir>/<name>` e.g. `.svelte-kit/adapter-skgo` | `builder.js:L200-L202` |
| `getClientDirectory()` | `<outDir>/output/client` | `L204-L206` |
| `getServerDirectory()` | `<outDir>/output/server` | `L208-L210` |
| `getAppPath()` | `build_data.app_path` = `<base-without-leading-slash>/<appDir>` (or just `<appDir>`) | `L212-L214`; `exports/vite/index.js:L1181` |
| `writeClient(dest)` | copies `output/client` → `dest`, skipping `.vite` | `L216-L221` |
| `writePrerendered(dest)` | copies `output/prerendered/pages`, `/dependencies`, `/data` **all into the same** `dest` | `L223-L231` |
| `writeServer(dest)` | copies `output/server` → `dest` | `L233-L235` |
| `hasServerInstrumentationFile()` | `output/server/instrumentation.server.js` exists | `L237-L239` |
| `instrument({...})` | renames an entrypoint and writes an import-instrumentation-first facade | `L241-L282` |
| `createEntries` | **removed in 3.0** (type still present, marked deprecated) | `types:L129-L134` |

Not on `Builder`: the list of remote-function chunks (`remotes`), `build_data`, `server_metadata`,
`prerender_map`. They are all closed over in `create_builder` (`builder.js:L61-L72`) but not exposed —
see "Getting remote ids out" below.

### What `builder.config` contains that matters to skgo (SOURCE-DERIVED)

`ValidatedConfig` = `RecursiveRequired<Config>` (`types/index.d.ts:L908-L910`), i.e. every key has a value.
Keys skgo reads:

- `config.appDir` (default `"_app"`) — `types:L1557-L1563`
- `config.paths.base` (`''` or `/x`), `config.paths.assets` (`''` or absolute URL), `config.paths.origin`
  (`undefined` unless set, **@since 3.0**), `config.paths.relative` (default `true`) — `types:L1828-L1868`
- `config.outDir` (default `.svelte-kit`) — `types:L1757-L1761`
- `config.files.assets` (default `static`), `config.files.routes` (default `src/routes`) — `types:L1679-L1744`
- `config.prerender.entries` (default `['*']`), `.crawl`, `.concurrency`, `handle*` — `types:L1872-L1975`
- `config.router.type` (`'pathname' | 'hash'`), `config.router.resolution` (`'client' | 'server'`, default client) — `types:L1976-L2011`
- `config.output.bundleStrategy` (`'split' | 'single' | 'inline'`, default split), `config.output.linkHeaderPreload` (new 3.0) — `types:L1765-L1827`
- `config.version.name` (build timestamp unless set), `config.version.pollInterval` (default 3600000) — `types:L2078-L2107`
- `config.csrf.trustedOrigins` (default `[]`; `['*']` disables the check) — `types:L1617-L1644`
- `config.experimental.remoteFunctions` (default `false`) — `types:L1662-L1674`
- `config.adapter` — the adapter object itself (`types:L1546`)

### `RouteDefinition` details (SOURCE-DERIVED)

- `id` is the kit route id (`/blog/[slug]`), `pattern` is a `RegExp` (from internal `RouteData.pattern`),
  `segments` derive from `get_route_segments(route.id)` with `dynamic = segment.includes('[')`,
  `rest = segment.includes('[...')` — `builder.js:L86-L99`.
- `prerender` = `prerender_map.get(route.id) ?? false` — `true | false | 'auto'`. The map is seeded from
  `export const prerender` analysis (`postbuild/prerender.js:L143-L147`) and updated during crawling when a
  dependency response carries `x-sveltekit-prerender` (`prerender.js:L443-L453`).
- `methods` = union of page methods (`['GET']` plus `'POST'` when the leaf has `actions`) and endpoint methods
  (each of `ENDPOINT_METHODS` exported from `+server`, plus `'*'` if `fallback` is exported) —
  `postbuild/analyse.js:L129-L145, L175-L206, L212-L226`.
- `config` = merged `export const config` from page nodes/endpoint (must agree, else build error) —
  `analyse.js:L105-L115`.
- **Not exposed:** `params` (names, matchers, optional/rest/chained flags) and node indices. They are in the
  `generateManifest()` text (`params: [...]`, `page: { layouts, errors, leaf }`) — `generate_manifest/index.js:L120-L134`.

### Getting remote ids out of a build (SOURCE-DERIVED)

- Remote chunks are emitted as `output/server/chunks/remote-<hash>.js` where `hash = hash(posix relative path of the .remote.* file)` — `exports/vite/index.js:L650-L654, L684-L695`. (The hash algorithm lives in
  `src/utils/hash.js`; the remote-server segment owns it.)
- `generateManifest()` text contains
  `remotes: { '<hash>': __memo(() => import('<relativePath>/chunks/remote-<hash>.js')), ... }` —
  `generate_manifest/index.js:L117-L119`. Trivial to regex for hashes.
- Export names + types (`query`/`command`/`form`/`prerender`) are only discoverable by executing the chunk:
  build-time `analyse` does `await loader()` and reads `functions[name].__.type` (`analyse.js:L148-L166`);
  prerender does the same after `set_manifest`/`set_read_implementation` (`prerender.js:L654-L671`). An adapter
  can do exactly what `analyse.js:L44-L59` does (import `output/server/internal.js`, `set_building()`,
  `set_manifest(manifest)`, then import each chunk) to list `{hash, name, type}`.
- The client-side id is `'<hash>/<name>'` (`exports/vite/index.js:L678, L735`) and the request URL is
  `<base>/<appDir>/remote/<hash>/<name>[/<stringified-arg>]` (`prerender.js:L268, L711-L719`).

### Docs-only facts (DOCS)

- Adapter recipe: clear the build dir; `writeClient`/`writeServer`/`writePrerendered`; emit code that imports
  `Server` from `${builder.getServerDirectory()}/index.js`, instantiates it with `generateManifest({relativePath})`,
  and calls `server.respond(request, { getClientAddress })` — `99-writing-adapters.md:L53-L66`. skgo deliberately
  skips the `writeServer`/`Server` half (no JS runtime in prod).
- Recommended output locations: adapter output under `build/`, intermediates under `.svelte-kit/[adapter-name]`
  (`99-writing-adapters.md:L68-L70`).
- `vite preview` runs the app in Node and ignores adapter `platform` (`10-building-your-app.md:L26-L30`) — it also
  requires `output/server/manifest.js` (`preview/index.js:L30-L34`), so `vite preview` is *not* a skgo preview.

## Citations

- `reference/kit/packages/kit/src/core/adapt/builder.js:L31-L43, L61-L104, L106-L284, L290-L306`
- `reference/kit/packages/kit/src/core/adapt/index.js:L15-L60`
- `reference/kit/packages/kit/types/index.d.ts:L15-L60, L108-L250, L329-L335, L628-L641, L833-L848, L852-L887, L908-L910`
- `reference/kit/packages/kit/src/core/generate_manifest/index.js:L26-L163`
- `reference/kit/packages/kit/src/core/postbuild/analyse.js:L27-L169`
- `reference/kit/packages/kit/src/core/postbuild/fallback.js:L18-L53`
- `reference/kit/packages/kit/src/exports/vite/index.js:L650-L654, L684-L695, L1151-L1611, L1690-L1738`
- `reference/kit/packages/kit/src/exports/vite/options.js:L6-L15`
- `reference/kit/documentation/docs/25-build-and-deploy/99-writing-adapters.md`
- `reference/kit/documentation/docs/25-build-and-deploy/20-adapters.md`

## Go implementation notes

- Go never sees `Builder`; it sees files. The adapter's job is to turn the in-memory facts above into one JSON
  document Go can parse at startup (call it `skgo.manifest.json`, written next to `client/`). Everything Go needs
  is available at adapt time without touching kit internals:
  - `appDir`, `appPath` (`builder.getAppPath()`), `base`, `assets`, `origin`, `relative`, `version.name`,
    `router.type`, `router.resolution`, `bundleStrategy`, `trustedOrigins`.
  - `routes[]`: `{ id, pattern: route.pattern.source, segments, prerender, methods, page.methods, api.methods, config }`.
    Go should build its matcher from `id` (kit's route-id grammar) rather than trusting `pattern.source` to be RE2-safe;
    keep `pattern.source` for cross-checking in tests.
  - `prerendered`: `pages` as `{ path: file }`, `assets` as `{ path: mime }`, `redirects` as `{ path: {status, location} }`, `paths[]`.
  - `remotes[]`: at minimum the hashes regexed from `builder.generateManifest({ relativePath: './' })`;
    ideally `{ hash, file, exports: [{ name, type }] }` by importing the chunks as `analyse.js` does.
  - `fallback`: the filename passed to `generateFallback`.
- `builder.compress()` gives Go `.br`/`.gz` siblings for free; Go's static handler then only needs
  content-negotiation (see `static-serving.md`).
- `builder.generateEnvModule()` is only useful if you want a *static* `_app/env.js`. Since Go owns the process
  env, Go can instead answer `GET <base>/<appDir>/env.js` itself with `export const env={...}` built from the
  allowed public vars at runtime — which is what kit's Node `Server` does for non-prerendered apps. Decide one.
- `paths.origin` is baked into both bundles as `__SVELTEKIT_PATHS_ORIGIN__` (`exports/vite/index.js:L497`);
  the Go server's CSRF/origin logic must agree with it (see `config-kit3.md`).

## Gotchas (kit 3 vs kit 2, `next` churn)

- `svelte.config.js` is a hard error; config lives in `sveltekit({...})` inside `vite.config.js`
  (`exports/vite/index.js:L283-L289`). Kit 2 adapters that read `svelte.config.js` themselves break.
- `builder.config.kit` → deprecation warning; `createEntries` removed; `output.preloadStrategy` removed;
  `csrf.checkOrigin` removed (`types:L1623, L1784`).
- `builder.routes` includes prerendered routes ("An array of all routes (including prerendered)",
  `types:L126-L127`), but `generateManifest()` default subset excludes `prerender === true` routes
  (`builder.js:L194`). Use `route.prerender` yourself for filtering.
- `writePrerendered` merges `pages/`, `dependencies/` and `data/` into one tree — a prerendered page
  `foo.html` and a data file `foo/__data.json` land side by side; a prerendered remote response lands at
  `<dest>/_app/remote/<hash>/<name>`. Go must not treat `<dest>/_app/**` as immutable just because it is under `_app`.
- `generateFallback` overwrites silently (with a warning) — never name it `index.html` if `/` is prerendered
  (`55-single-page-apps.md:L41`, `50-adapter-static.md:L87`).
- The `Builder` type has `@since 2.31.0` members (`instrument`, `hasServerInstrumentationFile`) and `@since 3.0.0`
  adapter hooks; `next.*` releases may still rename. Pin kit and re-diff `builder.js` on every bump.

## Recipes

### Minimal `adapter-skgo` (`adapter/index.js`, JS is allowed here because kit requires it)

```js
import { rmSync, writeFileSync } from 'node:fs';
import path from 'node:path';

/** @param {{ out?: string; fallback?: string; precompress?: boolean }} [opts] */
export default function (opts = {}) {
  const { out = 'build', fallback = '__skgo_fallback.html', precompress = true } = opts;
  return {
    name: 'adapter-skgo',
    async adapt(builder) {
      const { config } = builder;              // NOT builder.config.kit (deprecated warning)
      rmSync(out, { force: true, recursive: true });

      const client = path.join(out, 'client');          // static + _app/immutable + _app/version.json
      const prerendered = path.join(out, 'prerendered'); // pages + dependencies + data merged
      builder.writeClient(client);
      builder.writePrerendered(prerendered);
      await builder.generateFallback(path.join(prerendered, fallback));
      if (precompress) {
        await Promise.all([builder.compress(client), builder.compress(prerendered)]);
      }

      const manifest_src = builder.generateManifest({ relativePath: './' });
      const remotes = [...manifest_src.matchAll(/'([0-9a-z]+)':\s*__memo\(\(\) => import\('\.\/chunks\/remote-\1\.js'\)\)/g)]
        .map((m) => m[1]);

      writeFileSync(path.join(out, 'skgo.manifest.json'), JSON.stringify({
        kit: '3.0.0-next.25',
        appDir: config.appDir,
        appPath: builder.getAppPath(),
        paths: config.paths,                    // base, assets, origin, relative
        version: config.version.name,
        router: config.router,
        bundleStrategy: config.output.bundleStrategy,
        trustedOrigins: config.csrf.trustedOrigins,
        fallback,
        routes: builder.routes.map((r) => ({
          id: r.id, pattern: r.pattern.source, segments: r.segments,
          prerender: r.prerender, methods: r.methods,
          page: r.page, api: r.api, config: r.config
        })),
        prerendered: {
          pages: Object.fromEntries(builder.prerendered.pages),
          assets: Object.fromEntries(builder.prerendered.assets),
          redirects: Object.fromEntries(builder.prerendered.redirects),
          paths: builder.prerendered.paths
        },
        remotes
      }, null, 2));
    },
    supports: { read: () => false, instrumentation: () => false }
  };
}
```

### Listing remote exports at adapt time (mirrors `analyse.js:L44-L59, L148-L166`)

```js
const server = builder.getServerDirectory();
const internal = await import(pathToFileURL(`${server}/internal.js`).href);
internal.set_building();
const { manifest } = await import(pathToFileURL(`${server}/manifest-full.js`).href);
internal.set_manifest(manifest);
for (const [hash, loader] of Object.entries(manifest._.remotes)) {
  const { default: fns } = await loader();
  for (const name in fns) console.log(hash, name, fns[name].__.type);
}
```
