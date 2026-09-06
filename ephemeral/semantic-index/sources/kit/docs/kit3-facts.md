# kit 3 facts: the corrective list

## Purpose

Every model prior about SvelteKit is kit-2-shaped. This file is the list of things that are
*different* on the pinned line (`@sveltejs/kit 3.0.0-next.25`, adapter-node `6.0.0-next.10`),
each tied to where it was seen. `VERIFIED-LIVE` means the junkyard hit it building and running
a real app (`junkyard/.agents/skills/sveltekit-current/SKILL.md`, verified 2026-08-27..09-01).
`DOCS` means it comes from the pinned kit docs (`reference/kit/documentation/docs/...`), which
are authoritative for the authored surface but occasionally stale (see Gotchas). When the two
disagree, VERIFIED-LIVE wins.

Citation paths are relative to `/Users/tyler/src/skgo/ephemeral/inspiration/`.

## Key facts

### Configuration

- **`svelte.config.js` is gone and kit refuses to run if it exists.** All config is passed to
  `sveltekit({...})` in `vite.config.ts`, **flat** (no `kit:` namespace), with
  `compilerOptions` beside it. VERIFIED-LIVE
  `junkyard/.agents/skills/sveltekit-current/SKILL.md:L14-L32`; DOCS
  `reference/kit/documentation/docs/60-appendix/35-migrating-to-sveltekit-3.md:L25-L48`.
  ```ts
  export default defineConfig({
    plugins: [sveltekit({
      adapter: adapter(),
      experimental: { remoteFunctions: true },
      compilerOptions: { experimental: { async: true } }
    })]
  });
  ```
- **Removed options**: `files.lib`, `experimental.handleRenderingErrors`,
  `experimental.instrumentation`, `experimental.tracing` (now top-level `tracing`),
  `vitePlugin` (pass vite-plugin-svelte options like `inspector` directly), `preloadStrategy`,
  `prerender.origin` (→ `paths.origin`), `csrf.checkOrigin` (→ `csrf.trustedOrigins`). DOCS
  `35-migrating-to-sveltekit-3.md:L50-L61`.
- **Added options**: `output.linkHeaderPreload` (default off: `<link>` elements, not `Link`
  headers), `csrf.trustedOrigins`, `paths.origin`. DOCS `35-migrating-to-sveltekit-3.md:L63-L67`.
- **`version.pollInterval` now defaults to one hour** — the client polls for new deployments
  and flips `updated.current`. DOCS `35-migrating-to-sveltekit-3.md:L69-L71`, `L211-L220`.
  `updated.current` also flips on any navigation that fetches data, on any remote function
  call, and on window focus.

### Minimum toolchain

- Node ≥ 22.17, **TypeScript ≥ 6**, Svelte ≥ 5.56.4, Vite ≥ 8.0.12 (rolldown),
  `@sveltejs/vite-plugin-svelte` ≥ 7. DOCS `35-migrating-to-sveltekit-3.md:L13-L23`.
- **TypeScript 7 (tsgo) cannot build kit.** typescript@7 ships no JS compiler API
  (`ts.sys` undefined); kit's sync calls `ts.readConfigFile(file, ts.sys.readFile)` and dies
  with "Cannot read properties of undefined (reading 'readFile')". A pnpm `overrides` entry
  does *not* fix it (peers resolve from the root). The root `typescript` devDependency must be
  6.x (junkyard used 6.0.3). Vite+ bundles its own tsgo for `vp check`, so nothing needs TS 7.
  VERIFIED-LIVE `SKILL.md:L103-L108`.
- **Vite+ (`vp`) is the JS front door**: `vp install/dev/build/test/lint/fmt/check`.
  Provisioned via mise (`"npm:vite-plus" = "0.3.0"` in `mise.toml`); bundles vite 8.2.2
  (Rolldown), vitest 4, oxlint + tsgolint, oxfmt. `vp install` pins pnpm through
  `devEngines` in `package.json`; **after that `npm`/`npx` refuse to run in the project
  (EBADDEVENGINES)**. Run package CLIs as `mise x -- node node_modules/<pkg>/cli.js`.
  VERIFIED-LIVE `SKILL.md:L110-L117`; `junkyard/AGENTS.md:L38-L41`.

### Module aliases and tsconfig

- **`$lib` is gone; use `#lib`** (Node subpath imports). Build fails with "`$lib` has been
  removed. Use `#lib` instead". Needs a `package.json` `imports` map. VERIFIED-LIVE
  `SKILL.md:L52-L55` (form `"imports": { "#lib/*": "./src/lib/*" }`); DOCS
  `35-migrating-to-sveltekit-3.md:L73-L93` adds `"#lib": "./src/lib/index.js"` and notes
  **module extensions are now required** in those imports (`#lib/foo.js`, not `#lib/foo`).
- **`svelte-kit sync` writes `node_modules/$app/tsconfig.json`** (a literal `$app` directory),
  not `.svelte-kit/tsconfig.json`. User tsconfig must be `{ "extends": "$app/tsconfig" }` plus
  its own `include`/`exclude` (the generated one does not set them). Extending
  `./.svelte-kit/tsconfig.json` breaks with "Tsconfig not found". Service-worker variant:
  `$app/tsconfig/service-worker` from `src/service-worker/tsconfig.json`. VERIFIED-LIVE
  `SKILL.md:L41-L50`; DOCS `35-migrating-to-sveltekit-3.md:L234-L258`.
- **Renamed/removed `$app` modules**: `$app/environment` → `$app/env`; `$app/stores` removed
  (use `$app/state`, drop the `$` prefix); `$env/*` deprecated in favour of
  `$app/env/private` and `$app/env/public`; `$service-worker` removed (`version` from
  `$app/env`, `assets`/`immutable`/`prerendered` from the new `$app/manifest`, `resolved` from
  `$app/paths`). DOCS `35-migrating-to-sveltekit-3.md:L95-L105`, `L222-L232`, `L260-L266`.
- **`$app/paths`**: `base`, `assets`, `resolveRoute` removed; use `asset('foo.png')` and
  `resolve('/blog/[slug]', { slug })`. `Pathname`/`Asset` types renamed `Path`/`AssetPath` and
  lost their leading `/` — **only route IDs start with `/` now**. DOCS
  `35-migrating-to-sveltekit-3.md:L169-L190`; `98-reference/22-$app-types.md:L16-L119`
  (`RouteId`, `PageRouteId`, `EndpointRouteId`, `Path`, `ResolvedPathname`, `RouteParams<T>`,
  `LayoutParams<T>`).
- **Types moved**: `Handle`/`HandleServerError`/`Transport`/`Reroute` in `@sveltejs/kit/hooks`;
  `defineParams` and param types in `@sveltejs/kit/params`; env types and `defineEnvVars` in
  `@sveltejs/kit/env`; **remote function types (`RemoteQuery`, `RemoteForm`, `RemoteCommand`,
  …) in `$app/server`**. DOCS `35-migrating-to-sveltekit-3.md:L278-L296`.

### Hooks

- **`transport` is a UNIVERSAL hook** — `src/hooks.ts`, never `src/hooks.server.ts`. Kit
  imports it only from the universal hooks file (`core/sync/write_server.js:52-69`,
  `core/sync/write_client_manifest.js:123-166`) because the client needs the same codecs to
  decode. Misplacing it stalled a junkyard work item. VERIFIED-LIVE `SKILL.md:L34-L39`; DOCS
  `30-advanced/20-hooks.md:L361-L379` (shape: `{ Name: { encode(value) → falsy | encoded,
  decode(encoded) → value } }`).
- `reroute` also lives in `src/hooks.ts` (universal, may be async, must be pure; cached on the
  client). DOCS `20-hooks.md:L308-L359`.
- **`handleValidationError` no longer exists.** Remote-function validation failures pass
  through server `handleError` with `kind: 'validation'`, `error: { status: 400, message:
  'Bad Request' }`, and `issues`. VERIFIED-LIVE `SKILL.md:L91-L93`; DOCS
  `35-migrating-to-sveltekit-3.md:L382-L384`; `20-hooks.md:L165-L168`.
- **`handleError` receives ALL errors** (kit 2 skipped `error(...)` ones) and carries a `kind`
  discriminant: `'app'` | `'framework'` | `'validation'` (server only) | `'unknown'`. It may
  return `status` to change the HTTP status. Rendering errors are routed through it too and
  land in the nearest `<svelte:boundary>` (auto-created per `+error.svelte`). Redirects never
  reach it. DOCS `20-hooks.md:L148-L284`; `35-migrating-to-sveltekit-3.md:L386-L398`.
- `resolve` in `handle` is typed `Promise<Response>` always. DOCS `35-…:L491-L493`.
- `handle` during a **remote function request sees `route`/`params`/`url` of the page the
  call came from, not the remote endpoint** — never authorize on them. DOCS
  `20-hooks.md:L36`.

### Errors

- **`App.Error` always includes `status`** alongside `message`. DOCS
  `35-migrating-to-sveltekit-3.md:L372-L374`; `30-advanced/25-errors.md:L189`.
- **`error(status, message, extras?)`** — second arg must be a string; extra `App.Error`
  properties go in a third argument. DOCS `35-…:L376-L380`; `25-errors.md:L54-L74`.
- Framework errors (404, 405, 413) reach `handleError` as `kind: 'framework'` with a safe
  `{ status, message }`; unknown errors default to `{ status: 500, message: 'Internal Error' }`
  and expose nothing. DOCS `25-errors.md:L78-L126`.
- Errors inside `handle` or `+server.js` produce JSON or `src/error.html` (placeholders
  `%sveltekit.status%`, `%sveltekit.error.message%`) depending on `Accept`; `+error.svelte` is
  not used there. DOCS `25-errors.md:L144-L169`; `20-core-concepts/10-routing.md:L162-L168`.
- `isHttpError`/`isRedirect` from `@sveltejs/kit` replace `instanceof` against internal
  classes. DOCS `35-…:L270-L272`.
- `json()`/`text()` deprecated → `Response.json(...)` / `new Response(text)`. DOCS
  `35-…:L274-L276`.

### Origin, CSRF, cookies

- **The app's origin is a build-time constant.** adapter-node 6 no longer reads `ORIGIN` at
  runtime; it inlines `config.paths.origin` at build (`adapter-node/index.js`, the
  `\bORIGIN\b` replacement). Without it the runtime derives `https://` + host, so over plain
  http the app reports an `https://` URL and **kit's CSRF check rejects every browser form
  POST with 403**. `PROTOCOL_HEADER`/`HOST_HEADER` still exist for proxies. VERIFIED-LIVE
  `SKILL.md:L57-L63`; DOCS `35-…:L67`, `L460-L463`; `junkyard/README.md:L103-L107`.
- **CSRF is always on.** `csrf.checkOrigin: false` is gone; allow external origins via
  `csrf.trustedOrigins: [...]`. The check applies to form submissions **and remote function
  calls**. Cross-origin mutative requests **without a `Content-Type` header are rejected**.
  DOCS `35-…:L306-L331`.
- Dev CORS for static assets is delegated to Vite (`server.cors`). DOCS `35-…:L333-L346`.
- **Cookies**: `cookie` v2 — names ASCII-only; `CookieSerializeOptions` → `SerializeOptions`;
  **`path` defaults to `'/'`** when omitted (was forbidden). DOCS `35-…:L348-L368`.
- **External redirects must opt in**: `redirect(307, 'https://…', { external: true })` or an
  array of allowed origins. DOCS `35-…:L569-L577`.

### Routing and files

- **Param matchers are one file**: `src/params.ts` exporting
  `params = defineParams({...})` from `@sveltejs/kit/params`. A matcher is a function
  returning the parsed value (or `undefined` for no match) or a Standard Schema; its output
  type (must extend `string | boolean | number | bigint`) types `params.x`. Transforms should
  be symmetrical (`toString()` round-trips) so `resolve()` can build paths. DOCS
  `35-…:L408-L428`; `30-advanced/10-advanced-routing.md:L72-L128`.
- **Server-only modules** are any file with a `server` filename segment (`stuff.server.ts`,
  `server.ts`) and anything under any directory named `server` except inside `src/routes` and
  `static`. DOCS `35-…:L495-L503`.
- **Remote modules** are any file with a `remote` segment (`x.remote.ts`, `remote.ts`,
  `x.remote.test.ts`); **their mere existence errors if `experimental.remoteFunctions` is
  off**. DOCS `35-…:L530-L534`.
- Universal `config` (`+page.js`) takes precedence over server `config`. DOCS `35-…:L579-L581`.
- `204`/empty `2xx` from `+server.js` now has no body (no kit envelope). DOCS `35-…:L487-L489`.
- `data-sveltekit-*` attributes use `false`, not `'off'`; a link to the current page triggers
  `refreshAll()`. DOCS `35-…:L554-L567`.
- Service workers are registered as `type: 'module'`. DOCS `35-…:L583-L585`.
- Observability: `src/instrumentation.server.js` runs automatically if present; OTel via
  `tracing: { server: true }`. DOCS `35-…:L430-L448`.
- Sourcemaps are generated by default and applied to stack traces; adapters must not rebundle
  destructively. DOCS `35-…:L404-L406`.

### `$app/navigation`

- `invalidateAll` deprecated → `refreshAll` (does not reset `page.state`; reruns every load
  and all active queries). `invalidate()` during navigation no longer aborts it. DOCS
  `35-…:L130-L134`; `20-core-concepts/20-load.md:L639-L643`.
- `goto` options: `invalidateAll`→`refreshAll`, `keepFocus`+`noScroll`→`reset: false`,
  `replaceState`→`replace`; shallow routing is `goto(url, { shallow: true, state })`
  (`pushState`/`replaceState` deprecated); `goto` **rejects** for URLs that resolve to no
  route (use `window.location.href` for external). DOCS `35-…:L107-L146`.
- `preloadData` can return `{ type: 'error', status, error }`. DOCS `35-…:L152-L167`.
- `page.url` is a `ReadonlyURL`. DOCS `35-…:L200-L209`.

### Remote functions (flags and gating)

- Require BOTH `experimental: { remoteFunctions: true }` and
  `compilerOptions: { experimental: { async: true } }`. Authored in `.remote.ts` via
  `$app/server` (`query`, `query.batch`, `query.live`, `command`, `form`, `prerender`,
  `getRequestEvent`, `requested`). First argument is validation: omitted (no-arg),
  `'unchecked'`, or a Standard Schema v1 object. VERIFIED-LIVE `SKILL.md:L78-L96`; DOCS
  `20-core-concepts/60-remote-functions.md:L13-L38`, `L1256-L1266`.
- **Prerendered remote functions are fetched over HTTP at runtime.** During SSR of a dynamic
  page kit fetches `<origin>/_app/remote/<hash>/<fn>` from its *own* origin
  (`runtime/app/server/remote/prerender.js:105`) and falls back to a stub that throws
  "Unexpectedly called prerender function". Any host must serve the prerendered remote files
  and be reachable at the configured origin. VERIFIED-LIVE `SKILL.md:L65-L70`.
- Single-flight is real: `void getMessages().refresh()` inside a `command`/`form` handler
  ships refreshed data back in the same response — exactly one programmatic request.
  VERIFIED-LIVE `SKILL.md:L97-L99`.
- Inside a `query` (incl. `batch`/`live`), touching `event.url`/`params`/`route` **throws**.
  DOCS `35-…:L536-L538`; `60-remote-functions.md:L1307-L1311`.
- Form controls must use `field.as(...)`; a hand-written `name=` is rejected on submit. DOCS
  `35-…:L544-L552`.
- Remote resources' `error` is typed `App.Error | undefined`. DOCS `35-…:L540-L542`.

### Adapter API

- `builder.config.kit` no longer exists — configuration is at the top level of `config`
  (observed as a deprecation warning from adapter-node itself). `builder.createEntries`
  removed (use `writeClient`/`writeServer`/`writePrerendered`); `builder.compress` returns the
  list of compressed files; `builder.mkdirp`/`rimraf` deprecated; adapters may add Vite
  plugins. VERIFIED-LIVE `SKILL.md:L74-L76`; DOCS `35-…:L475-L483`.
- **adapter-node `precompress` defaults to `true`** (`adapter-node/index.js:16`); adapter-node
  bundles with rolldown. VERIFIED-LIVE `SKILL.md:L72`; DOCS `35-…:L460-L463`.
- `@sveltejs/kit/node` `getRequest`/`setResponse` are now synchronous;
  `@sveltejs/kit/node/polyfills` removed. DOCS `35-…:L298-L304`.

### Svelte 5 authoring

- Runes mode: `let { data, children, params } = $props()`; `{@render children()}`; remote
  query results via `{#await q(arg) then v}` or top-level `await` (async compile flag on).
  VERIFIED-LIVE `SKILL.md:L119-L123`; DOCS `10-routing.md:L57-L72` (pages get a typed
  `params` prop since 2.24).

## Citations

- `junkyard/.agents/skills/sveltekit-current/SKILL.md:L1-L123` (whole file; every item above
  marked VERIFIED-LIVE)
- `reference/kit/documentation/docs/60-appendix/35-migrating-to-sveltekit-3.md:L1-L585`
- `reference/kit/documentation/docs/30-advanced/20-hooks.md:L1-L384`
- `reference/kit/documentation/docs/30-advanced/25-errors.md:L1-L194`
- `reference/kit/documentation/docs/30-advanced/10-advanced-routing.md:L72-L128`
- `reference/kit/documentation/docs/20-core-concepts/60-remote-functions.md:L13-L38`,
  `L1232-L1266`, `L1307-L1311`
- `reference/kit/documentation/docs/98-reference/22-$app-types.md:L16-L119`
- `reference/kit/documentation/docs/10-getting-started/30-project-structure.md:L7-L91`
- `junkyard/README.md:L103-L112`; `junkyard/AGENTS.md:L38-L41`

## skgo implications

- **skgo's adapter** reads `config.paths.origin`, `config.paths.base`, `config.appDir`, etc.
  directly off `config` (never `config.kit`), records `origin` into whatever manifest it
  emits, and must not rebundle in a way that discards sourcemaps.
- **skgo must fix the origin at build**. The example app's `vite.config.ts` must set
  `paths: { origin }` (junkyard read it from an `ORIGIN` env var at build time,
  `junkyard/README.md:L105-L107`) and the Go server must listen where that origin resolves —
  otherwise every remote `command`/`form` POST is a 403 from kit's client-side expectations
  and, when Go implements the CSRF check natively, from Go too. `skgo dev` (proxying
  `vp dev`) and `skgo build` need one agreed origin per environment.
- **Go's CSRF check must mirror kit 3's**: always-on origin comparison for mutative
  requests, `trustedOrigins` allowlist, reject cross-origin mutations lacking `Content-Type`.
  Go serves the bundle and answers remote endpoints, so this is Go's job in production.
- **The generator's TS stubs** live in `*.remote.ts` files (a `remote` filename segment) and
  the example app must enable both experimental flags or the stubs' existence is a build
  error. Stubs must call the real `query`/`command`/`form`/`prerender` from `$app/server` with
  the right validation shape (`'unchecked'` or a schema) so kit's client transform emits fetch
  wrappers; the body throws so nothing runs in Node.
- **Generated TS types** for loads/remotes import from `$app/server` (remote types) and
  `./$types` (`PageServerLoad`, `LayoutServerLoad`, `PageProps`); routing helpers use
  `$app/types` (`RouteId`, `RouteParams<'/x/[id]'>`). Don't emit imports of `@sveltejs/kit`
  for remote types — they moved.
- **Go's validation-error mapping**: a bad argument to a Go remote function must produce the
  generic `400 Bad Request` body kit produces, and skgo's Go-side hook (if any) plays the role
  of `handleError` with `kind: 'validation'` + issues. Do not leak issues by default.
- **Go's error shape**: `{ status, message }` always (App.Error has `status`), plus optional
  extras; unknown errors collapse to `{ 500, 'Internal Error' }`.
- **Cookies from Go** must follow kit 3 defaults: path `/` when omitted, `httpOnly` and
  `secure` defaults, ASCII names.
- **Example app** must use `#lib/...js` imports with extensions, `{ "extends": "$app/tsconfig",
  "include": [...] }`, `src/params.ts` with `defineParams`, `src/hooks.ts` for `transport`,
  runes-mode components, `$app/state` not `$app/stores`, `$app/env` not `$app/environment`.
- **Toolchain**: mise provisions node + `vp`; run everything as `mise x -- vp ...` from the app
  dir; never `npm`/`npx`; root `typescript` 6.x; Go's build tooling shells out to `vp build`.

## Gotchas

- `10-getting-started/30-project-structure.md` is **stale** on two points: it still shows
  `src/params/` (L12-L13, L43) and `.svelte-kit/tsconfig.json` (L85). The migration doc and
  SKILL.md are right (single `src/params.ts`; `$app/tsconfig`).
- The `$app/server`, `$app/navigation`, and `@sveltejs/kit` reference pages
  (`98-reference/10-@sveltejs-kit.md`, `20-$app-server.md`, `20-$app-navigation.md`) are
  **generator stubs** (`> MODULE:` markers) with no type text in the checkout; real shapes are
  in `reference/kit/packages/kit/src/runtime/app/server/public.d.ts` (see
  `remote-functions-surface.md`).
- The `Available since 2.27` banner on the remote-functions doc is a kit-2 relic; the surface
  documented is the kit-3 one (`requested`, `query.live`, `invalid`, `field.as`).
- SKILL.md's `#lib` map is only `"#lib/*"`; the migration doc adds a bare `"#lib"` entry too.
  Emit both.
- `handleValidationError` still appears in kit-2-era blog posts and model priors. Gone.
- `ORIGIN` env var still appears in adapter-node docs from kit 2. Gone at runtime; build-time
  only via `paths.origin`.
- The docs' `validate({ includeUntouched: true })` (`60-remote-functions.md:L642`) does not
  match the type (`validate({ all?, preflightOnly? })`, `public.d.ts:L312-L322`). Trust the
  type.

## Recipes

- **Minimal kit-3 `vite.config.ts` for the example app**
  ```ts
  import { sveltekit } from '@sveltejs/kit/vite';
  import { defineConfig } from 'vite';
  import adapter from '<skgo adapter>';
  export default defineConfig({
    plugins: [sveltekit({
      adapter: adapter({ /* out dir the Go embed reads */ }),
      paths: { origin: process.env.ORIGIN ?? 'http://127.0.0.1:4173' },
      experimental: { remoteFunctions: true },
      compilerOptions: { experimental: { async: true } }
    })]
  });
  ```
- **`package.json` essentials**: `"type": "module"`, `"imports": { "#lib": "./src/lib/index.js",
  "#lib/*": "./src/lib/*" }`, `devEngines` pinned by `vp install`, `typescript` 6.x.
- **`tsconfig.json`**: `{ "extends": "$app/tsconfig", "include": ["src", "test", "*"],
  "exclude": ["src/service-worker"] }`.
- **CSR-only root layout**: `src/routes/+layout.ts` with `export const ssr = false;` as a
  literal so kit evaluates it statically (`40-page-options.md:L132`).
- **Run Playwright without npx**: `mise x -- node node_modules/@playwright/test/cli.js test`.
