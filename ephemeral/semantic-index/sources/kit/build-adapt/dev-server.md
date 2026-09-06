# `vite dev` (`vp dev`) for a kit app: what it serves, what a Go proxy must forward vs intercept

## Purpose

The exact middleware chain kit installs into Vite's dev server, which URLs it answers itself (and how),
which it hands to kit's Node `Server`, and therefore what skgo's dev-mode Go front must intercept
(`__data.json`, `/_app/remote/*`, actions, Go endpoints) versus transparently proxy (everything else,
HMR websocket included). Also origin/CSRF and cookie implications of proxying.

## Key facts

### Vite config kit forces in dev (SOURCE-DERIVED, `reference/kit/packages/kit/src/exports/vite/index.js`)

- `appType: 'custom'` and `base: kit.paths.base || '/'` (not `paths.assets`, to keep the trailing-slash redirect) —
  `L1057-L1070`.
- `server.cors: { preflightContinue: true }` and `preview.cors` likewise (`L404-L405, L417-L419`); `server.fs.allow`
  extended with routes/src/outDir/node_modules/runtime dir/`#` import dirs/client hooks dir (`L356-L380, L406-L408`);
  watcher ignores `${outDir}/!(generated)` (`L410-L415`).
- `optimizeDeps.entries` = `routes/**/+*.{svelte,js,ts}` minus `+*server.*`; excludes `@sveltejs/kit`, `$app`, `$env`
  (`L420-L436`). With `experimental.remoteFunctions`, `.remote.*` modules are externalised from prebundling as
  `/@fs/...` URLs (`L454-L476`).
- Dev `define`s: `__SVELTEKIT_DEV__ = true`, `__SVELTEKIT_APP_VERSION_POLL_INTERVAL__ = '0'`,
  `__SVELTEKIT_HAS_SERVER_LOAD__ = 'true'`, `__SVELTEKIT_HAS_UNIVERSAL_LOAD__ = 'true'` (`L511-L518`).
- `configResolved` writes a placeholder `generated/dev` app manifest before dep scanning (`L565-L568`) and warns
  about plugins using `transformIndexHtml` (unsupported) (`L570-L586`).
- `plugin_remote.transform` in dev: server side appends `init_remote_functions` + `await Promise.resolve()`;
  client side **imports the server module through the SSR runner** to learn export types, then emits fetch stubs
  `__remote.<type>('<hash>/<name>')` plus `import.meta.hot?.accept()` (`L643-L758`, esp. `L711-L720, L733-L750`).

### The kit dev middleware (SOURCE-DERIVED, `reference/kit/packages/kit/src/exports/vite/dev/index.js`)

Setup (`dev()` runs in `configureServer`, `exports/vite/index.js:L1083-L1095`):

- Wraps global `fetch` to throw on relative URLs (`L69-L78`); writes tsconfig (`L80`).
- Builds an in-memory `SSRManifest` from `create_manifest_data` lazily on first request and on route/param/remote/
  client-hook file add/unlink (debounced 100 ms), sending `full-reload` or an error overlay via `hot.send`
  (`L153-L358, L402-L449`). `appTemplate` change → `full-reload` (`L453-L462`); app/error template, service worker
  or server hooks change → regenerate `generated/dev` server (`L464-L473`).
- **Pre-Vite middleware** (registered immediately, so it runs before Vite's own): serves files from `static/`
  when the URL starts with `assets` (= `paths.base`, or `/_svelte_kit_assets` if `paths.assets` set) and the file
  exists with correct case, via `sirv(..., { dev: true, etag: true, maxAge: 0, extensions: [] })` (`L475-L504`).

Post-Vite middleware (returned closure, installed after Vite's internal stack) (`L512-L679`):

1. Removes Vite's `viteServeStaticMiddleware` and `viteServePublicMiddleware` so route names like `/test` or
   `/package.json` are not 403'd (`L513-L520, L685-L693`).
2. Restores `req.url = req.originalUrl` (Vite's base middleware strips `base`) (`L528-L530`).
3. If the decoded path (minus base) maps to an existing file under project root inside `server.fs.allow` → serve
   it with Vite's static middleware (raw source files are reachable, e.g. `/src/app.html`) (`L536-L550`).
4. Chrome DevTools well-known probe handled (`L552-L554`); path not under `base` → kit `not_found` (`L556-L558`).
5. `<base>/service-worker.js` → `import '<base>/@fs/<abs path>';` shim with `application/javascript` (`L560-L574`).
6. Imports `instrumentation.server` per request if present (`L578-L590`), then `Server` from the runtime via the
   Vite module runner, `set_fix_stack_trace`, `set_assets(assets)`, `new Server(manifest)`, `server.init({ env, read })`
   — a **fresh `Server` per request** (`L593-L610`).
7. `getRequest({ base: http(s)://<:authority|host>, request, response })` — the origin is whatever `Host` the
   request carries (`L532-L535, L612-L616`).
8. If the manifest is broken → 500 with the error template (`L618-L638`).
9. `server.respond(request, { getClientAddress: socket remoteAddress, read, before_handle (AsyncLocalStorage
   for feature tracking), emulator })` (`L640-L659`). There is **no special-casing** of `__data.json`, `/_app/remote/*`,
   actions or endpoints in the middleware — all of that is inside kit's `Server`.
10. A kit 404 is retried through Vite's static middleware, then written as the kit response (`L661-L670`).
    Errors → 500 with the fixed stack (`L671-L677`).

### What Vite itself serves before kit sees the request (Vite behaviour; NOT from pinned kit source)

- `/@vite/client`, `/@id/*`, `/@fs/<abs>`, `/node_modules/.vite/deps/*`, transformed source URLs (`/src/...`),
  `?import`/`?raw`/`?url` queries, and the HMR websocket (`Upgrade: websocket`, subprotocol `vite-hmr`, path from
  `server.hmr.path`, default the server root). Verify against the pinned Vite/Vite+ (`vp`) in `node_modules`; kit
  does not touch `server.hmr` (no `hmr`/`upgrade` references in `exports/vite/index.js` or `dev/index.js`).

## Citations

- `reference/kit/packages/kit/src/exports/vite/index.js:L356-L380, L404-L436, L454-L476, L511-L518, L565-L586, L643-L758, L1057-L1070, L1083-L1095`
- `reference/kit/packages/kit/src/exports/vite/dev/index.js:L45-L78, L153-L358, L402-L504, L512-L679, L685-L693, L747-L757`
- `reference/kit/packages/kit/src/constants.js:L5` (`SVELTE_KIT_ASSETS = '/_svelte_kit_assets'`)
- `reference/kit/documentation/docs/98-reference/52-cli.md` (`vite dev`/`build`/`preview`; `svelte-kit sync`)

## Go implementation notes

- **Intercept in Go (never proxy):** `*/__data.json` (and the bare data request for `/`), `<base>/<appDir>/remote/**`,
  `POST` to page routes with Go actions, and `+server` endpoints implemented in Go. In dev the kit middleware would
  otherwise execute the JS stub `+page.server.js`/`.remote.js`/`+server.js` in Node and answer with wrong/empty data —
  and with `__SVELTEKIT_HAS_SERVER_LOAD__ = 'true'` in dev the client *will* ask for `__data.json`.
- **Proxy verbatim:** everything else, including `/@vite/*`, `/@fs/*`, `/@id/*`, `/node_modules/*`, `/src/*`,
  `<base>/<appDir>/*` (dev client entry and generated nodes come from `/@fs/.../.svelte-kit/generated/dev/client/`),
  page GETs (kit returns the CSR shell), `static/` files, `/service-worker.js`, and the websocket upgrade.
  Use `httputil.ReverseProxy` with `Director` preserving path+query, forward `Upgrade`/`Connection` (Go's
  ReverseProxy handles WebSocket upgrades since 1.12), and set `X-Forwarded-Host`/`X-Forwarded-Proto`.
- **Host header:** kit derives the request origin from `Host`/`:authority` (`dev/index.js:L532-L535`). Either forward
  the browser's `Host` unchanged (so `event.url.origin` equals what the user typed and cookies/redirects line up),
  or set `paths.origin` to Go's public origin. Forwarding `Host` is the simplest; ensure Vite's `server.allowedHosts`
  (Vite ≥ 6.0.9 host check) includes it.
- **CSRF:** kit's origin check applies to form-encoded `POST/PUT/PATCH/DELETE` and remote calls, comparing the
  `Origin` header with the request origin (`__SVELTEKIT_CSRF_CHECK_ORIGIN__`, `exports/vite/index.js:L495`;
  docs say checks only apply in production, `types/index.d.ts:L1636`). Since Go intercepts every mutating request,
  Go performs the same check against its own origin / `csrf.trustedOrigins`.
- **Cookies:** the browser talks only to Go, so cookies are scoped to Go's host/port. Nothing on the Vite side sets
  cookies in a CSR app unless a JS stub does; Go must not strip `Set-Cookie` from proxied responses regardless.
- **CORS:** kit enables `cors` on the dev server (`server.cors: { preflightContinue: true }`), so proxied responses may
  carry `Access-Control-Allow-Origin`. Harmless behind a same-origin Go proxy, but do not rely on it for Go endpoints.
- **Static files in dev:** `static/` is served by kit's pre-middleware from the source dir (`L475-L504`); Go may proxy
  these too, or serve `static/` directly in dev for parity — proxying is simpler and matches case-sensitivity rules.
- **HMR:** Vite's client connects to the same origin it was loaded from unless `server.hmr.host/port/clientPort` is
  set. Because the page is loaded through Go, the websocket hits Go; proxy `Upgrade` requests to the `vp dev` port.
  If the upgrade path is customised (`server.hmr.path`), read it from the Vite config or make it a skgo option.
- **404 semantics:** kit's dev 404 for unknown page paths returns kit's own error page via `server.respond`; in dev
  Go can let that through (proxy) rather than serving the production fallback.

## Gotchas (kit 3 vs kit 2, `next` churn)

- One `Server` instance is constructed **per request** in dev (`L605-L610`); fine for a proxy but means server
  hooks' module-level state resets only on file change, not per request — parity with prod differs.
- The dev client manifest uses `/@fs/<outDir>/generated/dev/client/app.js` and `nodes/<i>.js` (`L191-L205`) — these
  are not `<appDir>/immutable/*`, so Go's dev proxy must not apply immutable caching to anything.
- Dev `define` `__SVELTEKIT_HAS_SERVER_LOAD__ = 'true'` differs from prod (computed per build, `index.js:L1249-L1259`),
  so data-request behaviour can differ between dev and a build where no route has a server load.
- `server.fs.allow` gating (`L541-L549`) means a Go dev proxy exposes raw source under `/src/**` to the browser —
  normal for Vite, but keep Go's dev mode off in production.
- `svelte.config.js` presence throws at config time (`index.js:L283-L289`); `$lib` throws "has been removed. Use `#lib`"
  (`L58-L64`). Kit 2 templates fail to even start.
- `vp` (Vite+) wraps Vite; kit docs only reference `vite dev/build/preview` (`52-cli.md`). Any `vp`-specific flags are
  outside kit's source; verify with the pinned Vite+.

## Recipes

- Minimal Go dev router (pseudocode):
  ```go
  switch {
  case isDataRequest(r):            goData(w, r)      // *_/__data.json
  case strings.HasPrefix(p, remotePrefix): goRemote(w, r)
  case r.Method == "POST" && goActionRoute(p): goAction(w, r)
  case goEndpoint(p, r.Method):     goEndpoint(w, r)
  default:                          viteProxy.ServeHTTP(w, r) // incl. websocket upgrade
  }
  ```
- Verify HMR through the proxy: open the app via Go's port, edit a `.svelte`, expect a `vite:beforeUpdate` in the
  console without a full reload; edit `src/app.html`, expect a full reload (`dev/index.js:L453-L462`).
