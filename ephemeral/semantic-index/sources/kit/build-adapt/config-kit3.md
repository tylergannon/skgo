# Kit 3 configuration surface relevant to skgo (`sveltekit({...})` in `vite.config.js`)

## Purpose

The kit 3.0.0-next.25 config keys, defaults and validation rules that change how a Go server must behave:
flat `sveltekit({...})` options, `paths.origin` fixed at build time, `experimental.remoteFunctions`,
`router`, `output`, `prerender`, `csrf`, `csp`, `alias` vs `#lib`, `appDir`, `version`, and what was
removed since kit 2.

## Key facts

### Where config lives (SOURCE-DERIVED)

- `sveltekit(config)` splits the object into kit config and vite-plugin-svelte config (`split_config`), validates
  the kit half (`validate_config`), appends a warning preprocessor, and returns
  `[...vite_plugin_svelte.svelte(inline), ...kit({ svelte_config })]` with `configFile: false` so vite-plugin-svelte
  never reads `svelte.config.js` — `reference/kit/packages/kit/src/exports/vite/index.js:L157-L197`.
- Any `svelte.config.js`/`.ts` in the root throws
  "`<file> is no longer used. Please pass configuration via the `sveltekit(...)` plugin in your Vite config.`"
  (`L283-L289`). (DOCS) "Prior to SvelteKit 3, config lived in a `svelte.config.js` file, which is no longer
  supported. The ability to configure SvelteKit via `vite.config.js` was added in version 2.62." (`L151-L152`).
- `Config extends VitePluginSvelteOptions` — anything kit does not own (`compilerOptions`, `preprocess`,
  `inspector`, `extensions`, …) is forwarded; `experimental` is shared (`types/index.d.ts:L1538-L1541, L1662`;
  `index.js:L147-L149`).
- `adapter` must be an object with an `adapt` method (`exports/vite/options.js:L6-L15`).
- Removed module ids: `$lib` → `#lib` (or re-add `alias: { '$lib': 'src/lib' }`), `$service-worker` →
  `$app/manifest` + `$app/env` + `$app/paths` (`index.js:L58-L71`, enforced in `resolveId`, `L300-L319`).
  `#`-prefixed `package.json` `imports` keys are enforced (users may not alias them in Vite) (`L349-L354`).
- Runtime aliases still provided: `$app` → runtime `app`, `$env` → runtime `env`, `<sveltekit:generated>` →
  `<outDir>/generated/{build,dev}` (`L393-L402`). Kit 3 env is configured explicitly (`plugin_env_vars`,
  `explicit_env_config`, `EnvVarConfig` from `@sveltejs/kit/env`; `$app/env/public` is the client module —
  `index.js:L51, L590-L591, L1711-L1713`; `core/adapt/builder.js:L162-L185`).

### Keys and defaults (SOURCE-DERIVED from JSDoc `@default` in `reference/kit/packages/kit/types/index.d.ts`)

| key | default | notes / cite |
|---|---|---|
| `adapter` | `undefined` | build warns "No adapter specified" (`index.js:L1602-L1608`); `L1542-L1546` |
| `alias` | `{}` | **deprecated**, use subpath imports (`#lib`) (`L1547-L1556`) |
| `appDir` | `"_app"` | two app dirs when `paths.assets` set (`L1557-L1563`) |
| `csp.mode` / `directives` / `reportOnly` | — | nonces for dynamic pages, hashes for prerendered; prerendered pages get a `<meta http-equiv>` CSP where `frame-ancestors`, `report-uri`, `sandbox` are ignored (`L1564-L1613`) |
| `csrf.checkOrigin` | — | **removed in 3.0**, use `trustedOrigins: ['*']` (`L1617-L1625`) |
| `csrf.trustedOrigins` | `[]` | full origins; `'*'` trusts all; "CSRF checks only apply in production" (`L1626-L1644`); compiled as `__SVELTEKIT_CSRF_CHECK_ORIGIN__ = !trustedOrigins.includes('*')` (`index.js:L495`) |
| `embedded` | `false` | (`L1645-L1650`) |
| `env.dir` | `"."` | `.env` directory (`L1651-L1660`); loaded via `vite.loadEnv(mode, kit.env.dir, '')` (`index.js:L343`) |
| `experimental.remoteFunctions` | `false` | required for `.remote.*` files; without it the guard plugin errors (`index.js:L767-L785`); `L1662-L1667` |
| `experimental.forkPreloads` | `false` | `L1669-L1673` |
| `files.*` | `src`, `static`, `src/hooks.client`, `src/hooks.server`, `src/hooks`, `src/params`, `src/routes`, `src/service-worker`, `src/app.html`, `src/error.html` | all marked deprecated-but-supported (`L1675-L1744`) |
| `inlineStyleThreshold` | `0` | (`L1745-L1751`) |
| `moduleExtensions` | `[".js", ".ts"]` | (`L1752-L1756`) |
| `outDir` | `".svelte-kit"` | (`L1757-L1761`) |
| `output.linkHeaderPreload` | `false` | **new 3.0**; `Link` header preloads for non-prerendered pages (`L1766-L1775`) — SSR only, n/a to skgo |
| `output.preloadStrategy` | — | **removed in 3.0** (`L1776-L1786`) |
| `output.bundleStrategy` | `'split'` | `'single'`/`'inline'` (`L1787-L1826`) |
| `paths.assets` | `""` | `''` or absolute `http(s)://` URL (`L1829-L1833`) |
| `paths.base` | `""` | `''` or `/x`, no trailing slash (`L1834-L1838`) |
| `paths.origin` | `undefined` | **new 3.0**; trusted origin for CSRF on form submissions and remote calls, `url.origin` during prerendering (else `http://sveltekit-prerender`); baked in as `__SVELTEKIT_PATHS_ORIGIN__` (`L1839-L1851`; `index.js:L497`) |
| `paths.relative` | `true` | relative asset paths in SSR/prerendered HTML; fallback always absolute (`L1852-L1867`) |
| `prerender.concurrency` | `1` | (`L1873-L1877`) |
| `prerender.crawl` | `true` | (`L1878-L1882`) |
| `prerender.entries` | `["*"]` | (`L1883-L1887`) |
| `prerender.handleHttpError` / `handleMissingId` / `handleEntryGeneratorMismatch` / `handleUnseenRoutes` / `handleInvalidUrl` | `"fail"` | (`L1888-L1974`) |
| `router.type` | `"pathname"` | `'hash'` disables SSR/prerender; page options other than `load` error under hash (`L1976-L1986`; `analyse.js:L72-L81`) |
| `router.resolution` | `"client"` | `'server'` makes the server resolve routes per navigation (`L1987-L2010`) |
| `serviceWorker.register` | `true` | (`L2012-L2030`) |
| `tracing.server` | `false` | OTEL spans (`L2031-L2041`) |
| `typescript.config` | identity | deprecated (`L2042-L2056`) |
| `version.name` | build timestamp | must be deterministic if set; `x-sveltekit-version` header + `version.json` (`L2057-L2101`) |
| `version.pollInterval` | `3600000` | `0` disables polling (`L2102-L2106`) |

Not config: `ssr`, `csr`, `prerender`, `trailingSlash` are **page options** exported from `+page(.server).js` /
`+layout(.server).js` (`index.js:L56, L76-L98`).

### Build-time constants derived from config (SOURCE-DERIVED, `exports/vite/index.js:L478-L507`)

`__SVELTEKIT_APP_DIR__`, `__SVELTEKIT_APP_VERSION__`, `__SVELTEKIT_APP_VERSION_CHECKS_ENABLED__` (`bundleStrategy !== 'inline'`),
`__SVELTEKIT_EMBEDDED__`, `__SVELTEKIT_FORK_PRELOADS__`, `__SVELTEKIT_PATHS_ASSETS__`, `__SVELTEKIT_PATHS_BASE__`,
`__SVELTEKIT_PATHS_RELATIVE__`, `__SVELTEKIT_CLIENT_ROUTING__` (`router.resolution === 'client'`),
`__SVELTEKIT_HASH_ROUTING__`, `__SVELTEKIT_SERVER_TRACING_ENABLED__`, `__SVELTEKIT_SUPPORTS_ASYNC__`
(`compilerOptions.experimental.async`), `__SVELTEKIT_DEV__`, `__SVELTEKIT_GLOBAL_NAME__`, `__SVELTEKIT_CSRF_CHECK_ORIGIN__`,
`__SVELTEKIT_LINK_HEADER_PRELOAD__`, `__SVELTEKIT_PATHS_ORIGIN__`, `__SVELTEKIT_SERVICE_WORKER__`; build-only:
`__SVELTEKIT_ADAPTER_NAME__`, `__SVELTEKIT_APP_VERSION_FILE__ = '<appDir>/version.json'`, `__SVELTEKIT_APP_VERSION_POLL_INTERVAL__`.
These are why the app's origin, base and appDir are fixed at build time — the client bundle cannot be re-pointed at runtime.

### DOCS

- Adapter usage snippet and platform-specific `event.platform` (`documentation/docs/25-build-and-deploy/20-adapters.md`).
- `paths.origin` vs `PROTOCOL_HEADER`/`HOST_HEADER` in adapter-node (`40-adapter-node.md:L69-L104`).
- GitHub Pages recipe sets `paths.base` conditionally (`process.argv.includes('dev') ? '' : process.env.BASE_PATH`)
  (`50-adapter-static.md:L107-L127`).

## Citations

- `reference/kit/packages/kit/src/exports/vite/index.js:L56-L71, L76-L98, L147-L197, L283-L289, L300-L319, L343, L349-L354, L393-L402, L478-L507, L767-L785, L1602-L1608, L1711-L1713`
- `reference/kit/packages/kit/src/exports/vite/options.js:L6-L15`
- `reference/kit/packages/kit/types/index.d.ts:L1530-L2108`
- `reference/kit/packages/kit/src/core/postbuild/analyse.js:L72-L81`
- `reference/kit/packages/kit/src/core/adapt/builder.js:L162-L185`
- `reference/kit/documentation/docs/25-build-and-deploy/20-adapters.md`, `40-adapter-node.md:L69-L104`, `50-adapter-static.md:L107-L127`

## Go implementation notes

- The adapter should copy the resolved `builder.config` fields Go depends on into `skgo.manifest.json`
  (`appDir`, `paths.*`, `version.name`, `router.*`, `output.bundleStrategy`, `csrf.trustedOrigins`,
  `experimental.remoteFunctions`) so Go never parses `vite.config.js`.
- `paths.origin`: if the deployment origin is known, set it (via `process.env.ORIGIN` in `vite.config.js`) and have
  Go treat it as the trusted origin for CSRF and absolute-URL generation; if unset, Go derives the origin from
  `Host` (+ trusted forwarded headers) exactly as adapter-node does, and must reject mutating form/remote requests
  whose `Origin` mismatches unless listed in `trustedOrigins` (or `'*'`).
- `paths.base` must be honoured by every Go route: static, prerendered, `__data.json`, remote, fallback. Base is
  compiled into the client, so Go cannot mount the app elsewhere without a rebuild.
- `router.resolution` must stay `'client'`; `router.type` must stay `'pathname'` (hash routing skips all
  server-side features, including `__data.json`).
- `csp`: prerendered pages carry a `<meta>` CSP with hashes; dynamic pages would get nonce headers from kit's
  server — Go serving the fallback must either emit an equivalent `Content-Security-Policy` header (hash mode)
  or the project must not enable `csp`. Treat `csp` as unsupported in v1 and validate it is unset.
- `version.name`: read from the manifest, emit on `x-sveltekit-version` and serve `version.json`.

## Gotchas (kit 3 vs kit 2, `next` churn)

- `svelte.config.js` → hard error; `kit.*` nesting gone (flat keys); `$lib` → `#lib`; `$service-worker` removed;
  `csrf.checkOrigin` removed; `output.preloadStrategy` removed; `createEntries` removed; `alias` deprecated.
- `paths.origin` is `@since 3.0` and also changes prerender's origin and the fallback's absolute URLs
  (`builder.js:L149`; `prerender.js:L156`).
- `experimental.*` is shared with vite-plugin-svelte and explicitly "not subject to semantic versioning"
  (`types:L1661`) — `remoteFunctions` may move out of `experimental` in a later `next`.
- TypeScript 7 cannot build kit 3 (project CLAUDE.md fact); use the pinned toolchain.

## Recipes

- Reference `vite.config.js` for skgo:
  ```js
  import { sveltekit } from '@sveltejs/kit/vite';
  import { defineConfig } from 'vite';
  import skgo from './adapter/index.js';
  export default defineConfig({
    plugins: [sveltekit({
      adapter: skgo({ out: 'build', fallback: '__skgo_fallback.html', precompress: true }),
      experimental: { remoteFunctions: true },
      paths: { base: '', origin: process.env.ORIGIN },        // origin optional
      router: { type: 'pathname', resolution: 'client' },
      output: { bundleStrategy: 'split' },
      csrf: { trustedOrigins: [] },
      version: { name: process.env.GIT_SHA }                    // deterministic
    })]
  });
  ```
- Go startup validation: assert `router.type == "pathname"`, `router.resolution == "client"`,
  `bundleStrategy == "split"`, `csp` unset; log `base`, `appDir`, `origin`, `version`.
