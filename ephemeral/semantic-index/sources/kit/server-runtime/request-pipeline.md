# Request pipeline: `respond.js` walkthrough, CSRF, cookies, headers, hooks, errors, endpoints

## Purpose

The ordered decision list kit's Node server applies to every request
(`Server.respond` → `internal_respond` → `handle` → `resolve`). The Go server
replaces this pipeline for the routes it hosts (data requests, actions,
endpoints, shell); this file records the exact ordering, header names, error
text, cookie semantics and status codes so Go's behaviour is indistinguishable
to kit's client and to browsers.

## Key facts

### Server lifecycle

- `new Server(manifest)`; `server.init({ env, read })` sets env, `read`,
  loads hooks once (`handle`, `handleError`, `handleFetch`, `reroute`,
  `transport`, `init`); defaults: `handle = resolve(event)`, `handleError`
  logs only `kind === 'unknown'` stacks (and validation issues), `reroute = noop`.
  `reference/kit/packages/kit/src/runtime/server/index.js:L60-L181`
- `server.respond(request, { getClientAddress, platform, read, before_handle, emulator, prerendering })`
  creates `RequestState { depth: 0, error: false, prerender_default, rerouted_url, ... }`;
  after respond, if `state.rerouted_url` is set, header
  `x-sveltekit-rerouted-url` is added. `L187-L202`,
  `reference/kit/packages/kit/src/runtime/server/state.js:L27-L54`,
  `reference/kit/packages/kit/src/constants.js:L36`

### `internal_respond` order of operations

`reference/kit/packages/kit/src/runtime/server/respond.js:L93-L467`

1. Parse `url`; compute `is_route_resolution_request` (`__route.js`),
   `is_data_request` (`__data.json`), `remote_id`. `L95-L99`
2. CSRF (prod only): remote requests → `is_remote_forbidden` (non-GET with
   `Origin !== self`) → 403 JSON `{ message: 'Cross-site remote requests are forbidden' }`.
   Otherwise if `csrf.checkOrigin` (i.e. trustedOrigins doesn't include `*`):
   `is_csrf_forbidden` = (no `content-type` OR form content type) AND method
   ∈ {POST, PUT, PATCH, DELETE} AND `Origin !== self_origin` AND (no Origin OR
   Origin not in `trusted_origins`) → 403 with
   `Cross-site ${method} form submissions are forbidden` — JSON `{message}`
   iff `accept === 'application/json'` (exact string match), else `text/plain`.
   `self_origin = kit.paths.origin || url.origin`. `L101-L135`,
   `reference/kit/packages/kit/src/runtime/server/csrf.js:L19-L65`
3. Hash routing: any path other than `base + '/'` or `/[fallback]` → 404. `L137-L139`
4. URL rewriting: route-resolution suffix strip; data-request suffix strip +
   `x-sveltekit-trailing-slash` / `x-sveltekit-invalidated` extraction; remote
   requests read `x-sveltekit-pathname` / `x-sveltekit-search` headers to
   restore the page URL. `L149-L176`
5. Cookies object from `cookie` header + url (`get_cookies`). `L181-L184`
6. `event` = `{ cookies, fetch, getClientAddress, locals: {}, params: {}, platform, request, route: { id: null }, setHeaders, url, isDataRequest, isSubRequest: depth > 0, isRemoteRequest }`. `L186-L250`
   - `setHeaders`: lowercases keys; `set-cookie` → throws
     `Use \`event.cookies.set(name, value, options)\` instead of \`event.setHeaders\` to set cookies`;
     duplicate key → throws `"<key>" header is already set` except `server-timing`
     which is appended with `, `. Headers are applied to the final response
     after `resolve`. DEV validates `cache-control`/`content-type` values with
     warnings. `L208-L236`, `reference/kit/packages/kit/src/runtime/server/validate-headers.js:L1-L64`
7. `reroute` hook (not for remote/route-id requests); failure → 500
   `Internal Server Error`. `L253-L282`
8. `decode_pathname` → failure → `resolved_path = null` → later 400
   `Malformed URI` via `respond_with_error`. `L297-L302, L599-L611`
9. Rerouted prerendered path → proxied via `fetch`. `L304-L336`
10. `base` check (404 `Not found` if not under base) and strip. `L341-L346`
11. Route resolution requests → JS module response. `L348-L354`
12. `/${app_dir}/env.js` → public env module; any other `/${app_dir}/*` → 404
    with `cache-control: public, max-age=0, must-revalidate`. `L356-L365`
13. `find_route(resolved_path, manifest._.routes, matchers)` → sets
    `event.route = { id }`, `event.params`. `L367-L382`
14. Load `page_nodes`; compute `trailing_slash`; for non-data requests 308 to
    the normalized path with `x-sveltekit-normalize: 1` and a *relative*
    `location`. `L384-L422`
15. `before_handle`/emulator (adapter hooks). `L424-L447`
16. `handle()`: `set_trailing_slash(trailing_slash)` (flushes queued cookie
    sets), then `hooks.handle({ event, resolve })`; `resolve` runs the
    request and then copies `headers` from `setHeaders`, appends
    `set-cookie` for `new_cookies`, and sets `x-sveltekit-routeid` when
    prerendering. `L469-L539`
17. After `handle`: ETag → 304 (keeps `etag`, `cache-control`,
    `content-location`, `date`, `expires`, `vary`, `set-cookie`). `L541-L570`
18. Data request + `handle` returned 300–308 with `location` → converted to
    the redirect JSON envelope. `L572-L579`
19. Thrown `Redirect` from steps 13–16: data/remote → JSON redirect envelope;
    page + JSON action request → action JSON redirect; else real redirect
    (`location` header, empty body). Cookies attached. Other throws →
    `handle_fatal_error`. `L451-L467`

### `resolve()` dispatch

`reference/kit/packages/kit/src/runtime/server/respond.js:L589-L822`

- `resolved_path === null` → 400 `Malformed URI` (`Failed to decode URI: <path>`).
- Hash routing or fallback → CSR shell (see csr-shell.md).
- `remote_id` → `handle_remote_call`.
- Route matched:
  - data request → `render_data` (see data-requests.md);
  - endpoint chosen if `route.endpoint && (!route.page || (!prerendering && is_endpoint_request(event)))`
    and, for GET/HEAD/POST on page+endpoint routes, only if the endpoint can
    handle that method (`POST`/`fallback`, or `GET`/`fallback`/`HEAD`);
  - else page: GET/HEAD/POST → `render_page`; OPTIONS → 204 `allow`; others → 405;
  - GET/HEAD on page+endpoint routes → append `Vary: Accept` unless already
    `accept`/`*`. `L639-L742`
- No route: sub-request with `state.error` → proxy with `x-sveltekit-error: true`;
  `state.error` → 500 text; top-level (`depth === 0`): data request with
  bitmask `[true]` → root-layout `render_data`; `sec-fetch-dest` in the
  non-HTML set (`image`, `script`, `style`, `font`, `json`, ...) → 404 text
  `Not Found` with `vary: Sec-Fetch-Dest`; else `respond_with_error(404, 'Not Found', 'Not found: <pathname>')`. `L744-L794`
- `finally`: `event.cookies.set` and `event.setHeaders` become throwing stubs
  (`Cannot use \`cookies.set(...)\` after the response has been generated`). `L812-L821`

### Endpoints (`+server.js`)

`reference/kit/packages/kit/src/runtime/server/endpoint.js:L13-L94`

- `handler = mod[method] || mod.fallback`; HEAD falls back to GET; none →
  `method_not_allowed` (405 text `<METHOD> method not allowed`, `allow` header
  listing exported methods; `HEAD` added when `GET` exists).
  `reference/kit/packages/kit/src/runtime/server/utils.js:L8-L30`
- `ENDPOINT_METHODS = GET POST PUT PATCH DELETE OPTIONS HEAD QUERY`. `reference/kit/packages/kit/src/constants.js:L9-L18`
- Handler must return a `Response` (`Invalid response from route <path>: handler should return a Response object`). Thrown `Redirect` → `Response(status, { location })`. Other errors propagate to `handle_fatal_error`.
- Endpoint `trailingSlash` export controls the 308 normalization for
  endpoint-only routes (default `'never'`). `reference/kit/packages/kit/src/runtime/server/respond.js:L400-L406`

### Errors and `handleError`

`reference/kit/packages/kit/src/runtime/server/errors.js:L19-L157`

- `handle_error_and_jsonify(event, state, error)` → `App.Error`:
  - `HandledHttpError` → `error.body` as-is (hook NOT called).
  - `HttpError` (from `error(status, ...)`) → `{ kind: 'app', error: body }`.
  - `SvelteKitError` (404/405/415/400 framework errors) → `{ kind: 'framework', error: { status, message: text } }`.
  - `ValidationError` → `{ kind: 'validation', error: { status: 400, message: 'Bad Request' }, issues }`.
  - anything else → `{ kind: 'unknown', error }` with fallback `{ status: 500, message: 'Internal Error' }`.
  - The hook's return is merged over the fallback (`{ ...fallback, ...body }`);
    hook throwing → `{ status: fallback.status, message: 'Internal Error' }`. `L44-L120`
- `handle_fatal_error`: `event.isDataRequest` or negotiated accept
  `application/json` → `Response.json(App.Error, { status })`; else
  `static_error_page(status, message)` = `src/error.html` template with
  `%sveltekit.status%`/`%sveltekit.error.message%` (message HTML-escaped),
  `content-type: text/html; charset=utf-8`. `L19-L36, L145-L157`
- `respond_with_error` renders `+error.svelte` inside the root layout (SSR)
  or the CSR shell with the error status; `x-sveltekit-error` request header
  forces the static error page (loop guard). `reference/kit/packages/kit/src/runtime/server/page/respond_with_error.js:L22-L107`
- Error/redirect classes: `HttpError { status, body }`, `Redirect { status: 300–308, location }` (location validated as a header value), `SvelteKitError extends Error { status, text }`, `ActionFailure { status, data }`. `reference/kit/packages/kit/src/exports/internal/shared.js:L3-L76`
- `error()` requires 400–599; `redirect()` requires 300–308.
  `reference/kit/packages/kit/src/exports/index.js:L81-L96, L129-L140`

### Cookies API semantics

`reference/kit/packages/kit/src/runtime/server/cookie.js:L40-L335`

- Defaults for `set`: `httpOnly: true`, `path: '/'`, `sameSite: 'lax'`,
  `secure: !DEV && !(hostname === 'localhost' && protocol === 'http:')`. `L72-L77`
- `path` is resolved against the normalized request URL when the cookie has
  no `domain` or `domain === url.hostname` (`resolve(normalized_url, path)`),
  so relative paths like `'.'` or `''` work and `'/'` stays `/`. Sets made
  before the route/trailing slash is known are queued and flushed by
  `set_trailing_slash`. `L229-L272`
- `get(name)` prefers cookies set during this request (most specific
  matching path; `maxAge === 0` tombstone → `undefined`), then the parsed
  `cookie` header (decoded with `decodeURIComponent` unless `opts.decode`).
  `getAll` merges likewise; tombstones delete. `L86-L160`
- `delete(name, opts)` = `set(name, '', { ...opts, maxAge: 0 })`. `L166-L168`
- `serialize(name, value, opts)` applies defaults and the same path
  resolution; throws `Cannot serialize cookies until after the route is determined` if called too early. `L172-L183`
- Response: one `set-cookie` per new cookie via `stringifySetCookie`; for
  cookie paths ending in `.html` a duplicate cookie with path
  `<path>.html__data.json` is added so the data request receives it. `L307-L327`
- `event.fetch` forwards request cookies + newly set cookies to same-site
  hosts (`.hostname` suffix match) unless `credentials: 'omit'`; `set-cookie`
  from internal fetches is captured back into `new_cookies` with `path`
  defaulting to the fetched URL's directory. `reference/kit/packages/kit/src/runtime/server/fetch.js:L61-L81, L129-L166`

### `event.fetch` (server-side fetch)

`reference/kit/packages/kit/src/runtime/server/fetch.js:L19-L224`

- Wrapped in `hooks.handleFetch`; sets `origin` header (then removes it for
  same-origin/no-cors GET/HEAD); same-origin requests under `base` are served
  in-process by recursively calling `respond` with a forked state
  (`depth + 1`), after resolving static assets from the manifest (`read`),
  and bailing to a real HTTP fetch for prerendered paths. Forwards
  `authorization`, `accept-language`, defaults `accept: */*`. Abort signals
  honoured. `L40-L167, L200-L224`
- `MAX_DEPTH = 10` for page render recursion → 404. `reference/kit/packages/kit/src/runtime/server/page/index.js:L29, L41-L46`

### `x-sveltekit-*` and other special headers/params summary

| Name | Direction | Meaning | Source |
|---|---|---|---|
| `x-sveltekit-invalidated` (query) | client→server | per-node bitstring for `__data.json` | `runtime/shared.js:L18` |
| `x-sveltekit-trailing-slash` (query) | client→server | `1` if page path ended with `/` | `runtime/shared.js:L20` |
| `x-sveltekit-action: true` | client→server | enhanced form POST → page, not endpoint | `endpoint.js:L108`, `app/forms/client.js:L161` |
| `x-sveltekit-pathname` / `x-sveltekit-search` | client→server | remote-function page URL | `respond.js:L168-L175` |
| `x-sveltekit-version` | server→client | app version on data/action responses | `server/utils.js:L47-L52` |
| `x-sveltekit-normalize: 1` | server→client | on 308 trailing-slash redirects (prerenderer uses it) | `respond.js:L415` |
| `x-sveltekit-page: true` | server→client | HTML page responses | `render.js:L589` |
| `x-sveltekit-routeid` | server (prerender) | encoded route id on responses while prerendering | `respond.js:L519-L521` |
| `x-sveltekit-error: true` | server→self | sub-request during error rendering | `respond.js:L744-L750` |
| `x-sveltekit-prerender` | server (prerender) | endpoint prerender flag | `endpoint.js:L68` |
| `x-sveltekit-rerouted-url` | server→adapter | rewritten URL for catch-all serverless | `constants.js:L36`, `server/index.js:L197-L199` |

### Body size limit (adapter/Node layer, not `respond`)

- `getRequest` in `exports/node/index.js` streams the body and errors with
  `SvelteKitError(413, 'Payload Too Large', ...)` when `content-length`
  exceeds the limit, when streamed bytes exceed `BODY_SIZE_LIMIT`, or when
  bytes exceed the declared `content-length`. No body is created without a
  `content-type` header. `reference/kit/packages/kit/src/exports/node/index.js:L13-L100`
- The `413` surfaces when the handler reads the body; adapter-node's default
  limit is 512 KiB (adapter config, outside these sources).

## Citations

- `reference/kit/packages/kit/src/runtime/server/index.js:L60-L202`
- `reference/kit/packages/kit/src/runtime/server/state.js:L27-L54`
- `reference/kit/packages/kit/src/runtime/server/respond.js:L59-L83, L93-L467, L469-L582, L589-L822, L829-L835`
- `reference/kit/packages/kit/src/runtime/server/csrf.js:L19-L65`
- `reference/kit/packages/kit/src/runtime/server/validate-headers.js:L1-L64`
- `reference/kit/packages/kit/src/runtime/server/cookie.js:L40-L335`
- `reference/kit/packages/kit/src/runtime/server/fetch.js:L19-L224`
- `reference/kit/packages/kit/src/runtime/server/endpoint.js:L13-L113`
- `reference/kit/packages/kit/src/runtime/server/errors.js:L19-L157`
- `reference/kit/packages/kit/src/runtime/server/utils.js:L8-L52, L104-L109`
- `reference/kit/packages/kit/src/runtime/server/page/respond_with_error.js:L22-L107`
- `reference/kit/packages/kit/src/runtime/server/page/index.js:L29-L51`
- `reference/kit/packages/kit/src/exports/internal/shared.js:L3-L90`
- `reference/kit/packages/kit/src/exports/index.js:L81-L96, L129-L140, L155-L199`
- `reference/kit/packages/kit/src/exports/node/index.js:L13-L100`
- `reference/kit/packages/kit/src/utils/http.js:L9-L83`
- `reference/kit/packages/kit/src/constants.js:L9-L36`

## Go implementation notes

Must reproduce (observable by browsers/kit client):

- Ordering: CSRF → suffix detection → base strip → `/_app/*` 404 → route
  match → trailing-slash 308 (non-data) → dispatch. Keep CSRF before routing
  so a forbidden POST never reaches Go handlers.
- CSRF rule and message strings verbatim, JSON only when `accept` is exactly
  `application/json`; treat missing `content-type` as forbidden-eligible
  (browsers always send one for forms, but `fetch` with a body may not).
- `setHeaders` semantics (single-set, `server-timing` append, no `set-cookie`).
- Cookie defaults and path resolution against the *normalized* URL; multiple
  `Set-Cookie` headers; `.html` data-suffix duplication; tombstone semantics
  in `get/getAll` within the same request.
- 405/OPTIONS responses with `allow` for page and endpoint routes; HEAD via
  GET without body; `Vary: Accept` on page+endpoint GET/HEAD.
- Error JSON shape `{ status, message, ...extra }` and `kind` semantics if
  skgo exposes a Go `handleError`; unknown errors must become
  `500 Internal Error` without leaking messages.
- 404 for `sec-fetch-dest` subresources as plain text with `vary: Sec-Fetch-Dest`
  (cheap, avoids serving the shell to `<img>` requests).
- `x-sveltekit-version` on data/action responses.

Node-internal / ignorable: `hooks.handle`/`reroute`/`handleFetch` JS hooks
(no JS runs in Go; if the app defines `hooks.server.ts` skgo must decide to
run it in the sidecar or reject), tracing spans, `with_request_store`,
`state.prerendering`/`emulator`/`before_handle`, webcontainer serialization,
DEV header validation, route-resolution modules, remote-function branches
(other worker), `event.fetch` in-process recursion (Go loads call Go
directly), the ETag 304 dance (optional).

## Gotchas

- `event.url` for data requests has both `x-sveltekit-*` params removed and
  the pathname normalized; `event.request.url` still has the raw
  `/__data.json?...` — actions use `event.request.url` for `?/name` lookup.
- `setHeaders` throws on duplicates — a Go API that silently overwrites would
  diverge; keep it strict or at least deterministic.
- `secure` cookie default is off for `http://localhost` only; `127.0.0.1`
  gets `Secure` and the browser will drop it over http.
- CSRF `self_origin` honours `kit.paths.origin` — if skgo fixes the origin at
  build time (kit 3 does), the Go server must compare against that same
  configured origin, not the incoming `Host`.
- Trailing-slash 308 uses a relative `location` (`../foo` or `foo/`), not an
  absolute path.
- A `Redirect` thrown from `handle` on a page route with an enhanced action
  request becomes action JSON (`{"type":"redirect"}` 200), not a 3xx.
- After the response is generated, `cookies.set`/`setHeaders` throw — streamed
  promise resolvers therefore cannot set cookies.
- `HEAD` on a page renders the page (kit relies on the platform to drop the
  body); Go should serve the shell headers with an empty body.

## Recipes

Go middleware order sketch:

```go
func (s *Server) ServeHTTP(w, r) {
  if forbidden := csrf(r); forbidden { write403(...) ; return }
  if strings.HasPrefix(path, "/"+appDir+"/immutable/") { static(...); return }
  u := r.URL; isData := hasDataSuffix(u.Path)
  if isData { u.Path = stripData(...) ; extract x-sveltekit-* }
  if base != "" { if !strings.HasPrefix(u.Path, base) { 404 }; u.Path = strings.TrimPrefix(...) or "/" }
  if strings.HasPrefix(u.Path, "/"+appDir) { 404 must-revalidate; return }
  route, params := match(decode(u.Path))
  ts := trailingSlash(route)
  if route != nil && !isData && normalize(u.Path, ts) != u.Path { 308 relative + x-sveltekit-normalize }
  ev := newEvent(r, u, params, route, cookies(ts))
  switch { case isData: renderData(ev); case endpointWins(ev): endpoint(ev); case r.Method == POST: action(ev)+shell; default: shell }
  applyHeaders(ev); applyCookies(ev)
}
```

CSRF check in Go:

```go
mutating := m == "POST" || m == "PUT" || m == "PATCH" || m == "DELETE"
ct := r.Header.Get("Content-Type")
formLike := ct == "" || isFormContentType(ct) // urlencoded, multipart, text/plain, application/x-sveltekit-formdata
origin := r.Header.Get("Origin")
if formLike && mutating && origin != selfOrigin && (origin == "" || !trusted[origin]) { forbid }
```
