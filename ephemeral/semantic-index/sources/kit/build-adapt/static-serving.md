# Static / prerendered serving rules (adapter-node, `vite preview`) that Go must replicate

## Purpose

The precise HTTP behaviour kit's reference Node servers apply to client assets, `static/` files and
prerendered pages — cache headers, etag, precompression negotiation, MIME, trailing-slash redirects,
origin derivation, body-size limits — so the Go static handler can be written once, correctly, and tested
against the same expectations.

## Key facts

### adapter-node request pipeline (SOURCE-DERIVED, `reference/kit/packages/adapter-node/src/handler.js`)

Order: `serve(client, true)` → `serve_prerendered()` → `ssr` (`L275-L278`).

1. **Client dir** `sirv(<dir>/client, { etag: true, gzip: PRECOMPRESS, brotli: PRECOMPRESS, setHeaders })` (`L42-L70`):
   - sirv negotiates `Accept-Encoding` and serves `<file>.br` / `<file>.gz` when they exist (`gzip`/`brotli` opts).
   - `setHeaders`: if `PRECOMPRESS` and the extension is in `uncompressed_extensions` (extensions written by the
     build for which **no** `.br/.gz` exists), remove `Vary` (`L49-L52`); otherwise sirv's `Vary: Accept-Encoding` stays.
   - Content-type is taken from `manifest.mimeTypes[ext]` (kit's table, which extends `mrmime`), with
     `;charset=utf-8` appended for `text/html` (`L54-L57`).
   - `cache-control: public,max-age=31536000,immutable` only when `client === true`, the pathname starts with
     `/${manifest.appPath}/immutable/` and the status is 200 (`L59-L66`).
   - The dir root is `<dir>/client` and files were written to `client<base>`, so `<base>/...` requests resolve
     naturally (`adapter-node/index.js:L29`; `handler.js:L31`).
2. **Prerendered** (`L85-L112`): decode the pathname; if `prerendered.has(pathname)` hand to a second sirv over
   `<dir>/prerendered` (sirv's default `extensions` resolves `/foo` → `foo.html` and `/foo/` → `foo/index.html`);
   else compute the trailing-slash inverse (`/foo` ↔ `/foo/`); if that is in the set respond
   `308 Location: <relative>` where relative is `../<segment>` when the request ended with `/`, else `<segment>/`
   (`relative_pathname`, `L73-L83`), preserving the query string; else `next()`.
3. **SSR** (`L114-L200`): origin = `ORIGIN` (build-time `paths.origin`) else `get_origin(headers)`:
   protocol from `PROTOCOL_HEADER` header **defaulting to `https`**, rejects a value containing `:`; host from
   `HOST_HEADER` then `host`, error if absent; optional `PORT_HEADER` must be numeric (`L243-L273`).
   Failure → `400 Bad Request`. Body limit `BODY_SIZE_LIMIT` default `512K` (`L23-L29`, `utils.js:L9-L17`).
   `getClientAddress` reads `ADDRESS_HEADER`; for `x-forwarded-for` picks the address `XFF_DEPTH` from the right
   (`L147-L188`). SSE responses get `x-accel-buffering: no` (`L191-L197`).
4. Build-time constants replaced into the handler: `ENV_PREFIX`, `PRECOMPRESS`, `ORIGIN` (`adapter-node/index.js:L123-L141`).
5. `server/manifest.js` exports `prerendered` = `new Set(builder.prerendered.paths)` and
   `uncompressed_extensions` (`adapter-node/index.js:L68-L76`; computed `L46-L51`).

### Process-level behaviour (SOURCE-DERIVED, `adapter-node/src/index.js`)

- Env: `SOCKET_PATH`, `HOST` (`0.0.0.0`), `PORT` (`3000` unless socket path), `SHUTDOWN_TIMEOUT` (30 s),
  `IDLE_TIMEOUT`, `KEEP_ALIVE_TIMEOUT`, `HEADERS_TIMEOUT`, systemd `LISTEN_PID`/`LISTEN_FDS` (fd 3) — `L10-L30, L43-L53`.
- Graceful shutdown: `closeIdleConnections()`, `close()`, then `closeAllConnections()` after the timeout; emits
  `sveltekit:shutdown` with `SIGINT | SIGTERM | IDLE` (`L77-L128`). Env names may be prefixed via `envPrefix`
  (`env.js:L21-L44`), except `LISTEN_PID`/`LISTEN_FDS` (`env.js:L19`).

### `vite preview` rules (SOURCE-DERIVED, `reference/kit/packages/kit/src/exports/vite/preview/index.js`)

- Client assets: `sirv(output/client)` scoped under the assets prefix, immutable cache-control only for
  `/${appDir}/immutable` (`L82-L94`).
- `base` handling: if `base.length > 1` and pathname `=== base` → `307` to `base + '/'` (+search) (`L102-L110`);
  pathnames not under `base` → 404 (`L116-L121`).
- Prerendered dependencies: `sirv(output/prerendered/dependencies, { etag: true, maxAge: 0 })` (`L124-L127`, `L240-L246`).
- Prerendered pages (`L131-L202`): weak-etag stripping (`W/"..."` → `"..."`) and `304` on match with a single
  per-process etag `"${Date.now()}"`; choose dir `data` when pathname starts with `/${appDir}/remote/`, else
  `pages`; try exact file, then `<file>.html` or `<file>index.html` depending on trailing slash; if the
  *other* spelling exists → `308` with a **relative** `Location` (`relative_pathname`) plus search; serve with
  `content-type: lookup(pathname) || 'text/html'` and the etag.

### `vite dev` static bits (SOURCE-DERIVED, `dev/index.js`)

- `static/` served by `sirv(files.assets, { dev: true, etag: true, maxAge: 0, extensions: [] })` only when the file
  exists, is not a directory, and the on-disk case matches (`L475-L504`, `L747-L757`).

### Builder-level compression (SOURCE-DERIVED)

- `builder.compress(dir)` compresses only `.html .js .mjs .json .css .svg .xml .wasm .txt .md .mdx`
  (`core/adapt/builder.js:L31-L43`), gzip level 9 and brotli text mode max quality (`L290-L306`).

### DOCS

- adapter-node docs recommend doing compression at the reverse proxy; if in-process, use a streaming-capable
  middleware (`40-adapter-node.md:L39-L43`).
- `PROTOCOL_HEADER`/`HOST_HEADER`/`PORT_HEADER`/`ADDRESS_HEADER`/`XFF_DEPTH` semantics and spoofing warnings
  (`40-adapter-node.md:L69-L134`). Wrong origin manifests as "Cross-site POST form submissions are forbidden" (`L106-L108`).
- `BODY_SIZE_LIMIT` accepts `K/M/G` suffixes and `Infinity` (`40-adapter-node.md:L136-L138`).

## Citations

- `reference/kit/packages/adapter-node/src/handler.js:L13-L36, L42-L70, L73-L83, L85-L112, L114-L200, L243-L273, L275-L278`
- `reference/kit/packages/adapter-node/src/index.js:L10-L30, L41-L53, L77-L128`
- `reference/kit/packages/adapter-node/src/env.js:L3-L44`
- `reference/kit/packages/adapter-node/src/utils.js:L9-L17`
- `reference/kit/packages/adapter-node/index.js:L28-L51, L68-L76, L123-L141`
- `reference/kit/packages/kit/src/exports/vite/preview/index.js:L28, L82-L94, L96-L122, L124-L202, L240-L273`
- `reference/kit/packages/kit/src/exports/vite/dev/index.js:L475-L504, L747-L757`
- `reference/kit/packages/kit/src/core/adapt/builder.js:L31-L43, L116-L131, L290-L306`
- `reference/kit/packages/adapter-static/platforms.js:L63-L90`
- `reference/kit/documentation/docs/25-build-and-deploy/40-adapter-node.md:L39-L43, L69-L138`

## Go implementation notes (the spec)

Given `base`, `appDir`, `appPath`, the `client/` and `prerendered/` trees and the prerendered path set `P`:

1. **Reject outside base**: if `base != ""` and path == base → `307` to `base + "/"` (+query). If path does not
   start with `base` → 404 (kit does this in preview; adapter-node relies on directory layout).
2. **Client/static files** (path relative to base, must be a regular file under `client/`, no `..`):
   - Negotiate `Accept-Encoding`: prefer `br` then `gzip` if `<file>.br`/`<file>.gz` exists; set
     `Content-Encoding`, `Vary: Accept-Encoding`. Only send `Vary` when a compressed sibling *could* exist
     for that extension (adapter-node strips it for extensions never compressed); simpler and still correct:
     send `Vary` whenever a sibling exists.
   - `ETag`: strong etag from size+mtime (sirv style) or content hash; honour `If-None-Match` (strip `W/`).
   - MIME from a table matching kit's (`mrmime` + kit additions; `text/html` gets `;charset=utf-8`;
     `.js`/`.mjs` → `text/javascript`, `.wasm` → `application/wasm`).
   - If path starts with `/<appPath>/immutable/` and file exists → `Cache-Control: public,max-age=31536000,immutable`.
     If it starts with that prefix and does **not** exist → 404 with `Cache-Control: no-store`; never fall through
     to the SPA fallback for that prefix.
   - `/<appPath>/version.json`, `/<appPath>/manifest.js`: serve with `Cache-Control: no-cache` (or short max-age).
3. **Prerendered pages**: decode path; if `P` contains it → serve `prerendered/<file>` where `file` is
   `foo.html` / `foo/index.html` / exact (non-HTML asset) — use the `prerendered.pages`/`assets` maps from the
   adapter manifest rather than probing the filesystem; `Content-Type: text/html;charset=utf-8`, etag, honour
   `.br/.gz` siblings. If the trailing-slash inverse is in `P` → `308` with relative `Location` per
   `relative_pathname` (`../<seg>` or `<seg>/`) plus the original query.
4. **Prerendered dependencies**: `prerendered/<route>/__data.json`, `prerendered/<appDir>/remote/<hash>/<name>[/<arg>]`
   served as `application/json` (never `text/html`), etag, `max-age=0`.
5. **Everything else**: route to Go handlers (`__data.json`, `/<appDir>/remote/*`, actions, endpoints) or serve the
   SPA fallback HTML (`200`, `text/html;charset=utf-8`, `Cache-Control: no-cache`) for GET/HEAD of a path that
   matches a page route; `404` (still the fallback body so the client router shows its error page, or a plain 404
   for non-page paths) otherwise — see `spa-and-prerender.md`.
6. **Origin**: Go owns the socket, so `paths.origin` (if set) is the trusted origin; otherwise derive from
   `Host` (+ `X-Forwarded-Proto`/`X-Forwarded-Host` only behind a trusted proxy). Note adapter-node's default
   protocol is `https` when no header is configured — do not copy that blindly for local plain-HTTP runs.
7. **Body limit** default 512 KiB for actions/remote/endpoint bodies, with `K/M/G` suffix parsing if exposed as config.
8. Streaming responses (SSE): set `X-Accel-Buffering: no`.

## Gotchas

- sirv's `extensions` default (`html`, `htm`) means adapter-node would also answer `/foo` from `foo.html` for any
  file in `client/`; Go should not resolve extensions for `static/` files, only for prerendered pages via the set.
- Query strings on prerendered pages are ignored (prerender emits a TODO to warn — `prerender.js:L502-L504`).
- The preview etag is process-global (`"${Date.now()}"`, `preview/index.js:L28`), i.e. weaker than adapter-node's
  per-file sirv etag; use per-file.
- Precompressed siblings only exist for the extension list above; images/fonts are not compressed.
- adapter-node serves `client` **before** prerendered pages, so a `static/foo.html` shadows a prerendered `/foo`.
  Match that order.

## Recipes

- Table-driven Go tests: for each `(path, accept-encoding, if-none-match)` assert `(status, content-type,
  content-encoding, cache-control, vary, location)` against the rules above; seed the tree with
  `_app/immutable/x.js{,.br,.gz}`, `_app/version.json`, `about.html`, `about/__data.json`, `_app/remote/h/q`.
- Relative-location helper (port of `relative_pathname`): `seg := last(strings.TrimSuffix(to, "/"))`;
  `if strings.HasSuffix(from, "/") { return "../" + seg }; return seg + "/"`.
