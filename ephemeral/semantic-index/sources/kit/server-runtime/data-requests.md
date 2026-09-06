# `__data.json` data requests — the end-to-end contract (kit 3.0.0-next.25)

## Purpose

This is the one wire protocol skgo's Go server MUST reproduce bit-for-bit for a
CSR app: when kit's client navigates (including the initial `enter` navigation
of an `ssr = false` shell) it fetches `<pathname>/__data.json` and expects a
devalue-encoded node envelope, optionally followed by ndjson chunks for deferred
promises. Everything in this file is derived from the pinned kit source at
`reference/kit/packages/kit/src/`. Go implements `page.server.go` /
`layout.server.go` loads; this document says exactly what the response must
look like and which nodes must run.

## Key facts

### 1. Request URL built by the client

- Client builds the URL in `load_data(url, invalid)`:
  `data_url.pathname = add_data_suffix(url.pathname)`; if the page pathname
  ends with `/`, appends `x-sveltekit-trailing-slash=1`; always appends
  `x-sveltekit-invalidated=<bitstring>` where each char is `1`/`0` per node.
  Fetched with `window.fetch(data_url.href, {})` — a plain GET, no custom
  headers, cookies sent by the browser as usual.
  `reference/kit/packages/kit/src/runtime/client/client.js:L3653-L3666`
- Param name constants: `INVALIDATED_PARAM = 'x-sveltekit-invalidated'`,
  `TRAILING_SLASH_PARAM = 'x-sveltekit-trailing-slash'`.
  `reference/kit/packages/kit/src/runtime/shared.js:L18-L20`
- Suffix rules (`pathname.js`): `DATA_SUFFIX = '/__data.json'`,
  `HTML_DATA_SUFFIX = '.html__data.json'`. `add_data_suffix` strips a trailing
  `/` then appends `/__data.json`, except paths ending in `.html` become
  `foo.html__data.json`. `has_data_suffix` accepts either form.
  `reference/kit/packages/kit/src/pathname.js:L1-L22`
- Consequence: `/` → `/__data.json`; `/blog/` → `/blog/__data.json?x-sveltekit-trailing-slash=1`;
  `/a.html` → `/a.html__data.json`. The original search params of the page are
  preserved on the data URL (the two `x-sveltekit-*` params are appended).

### 2. Server-side detection and URL reconstruction (respond.js)

- `is_data_request = has_data_suffix(url.pathname)` computed before anything
  else. `reference/kit/packages/kit/src/runtime/server/respond.js:L95-L99`
- Reconstruct the page URL: 
  ```js
  url.pathname = strip_data_suffix(url.pathname) +
      (url.searchParams.get(TRAILING_SLASH_PARAM) === '1' ? '/' : '') || '/';
  url.searchParams.delete(TRAILING_SLASH_PARAM);
  invalidated_data_nodes = url.searchParams.get(INVALIDATED_PARAM)?.split('').map(n => n === '1');
  url.searchParams.delete(INVALIDATED_PARAM);
  ```
  `event.url` seen by `load` is therefore the page URL with both params
  removed. `reference/kit/packages/kit/src/runtime/server/respond.js:L156-L165`
- `event.isDataRequest = is_data_request` is exposed on the RequestEvent.
  `reference/kit/packages/kit/src/runtime/server/respond.js:L238`
- CSRF check runs before this (GET is never forbidden; see request-pipeline.md).
- Route matching uses the reconstructed pathname (base stripped, decoded) via
  `find_route` — same as a page request.
  `reference/kit/packages/kit/src/runtime/server/respond.js:L297-L302, L341-L382`
- Trailing-slash normalization: for data requests the 308 normalize redirect is
  skipped (`if (!is_data_request)`), but `trailing_slash` is still computed
  from the page nodes (`page_nodes.trailing_slash()`, default `'never'`; forced
  `'always'` when `url.pathname === base || base + '/'`) and passed into
  `render_data`. `reference/kit/packages/kit/src/runtime/server/respond.js:L389-L422`
- `hooks.handle` runs with the data-request event; `resolve()` dispatches to
  `render_data(event, state, route, manifest, invalidated_data_nodes, trailing_slash)`.
  `reference/kit/packages/kit/src/runtime/server/respond.js:L645-L653`
- 404 special case: if no route matched and this is a data request whose
  bitmask is exactly `[true]` (length 1), kit still runs `render_data` with a
  synthetic route `{ page: { layouts: [], leaf: 0 } }` and trailing slash
  `'ignore'` — this is the client's root-error-page fetch of root layout data.
  `reference/kit/packages/kit/src/runtime/server/respond.js:L760-L778`
  The client issues that fetch in `load_root_error_page` as `load_data(url, [true])`
  and only if `app.server_loads[0] === 0` (root layout has a server load).
  `reference/kit/packages/kit/src/runtime/client/client.js:L1703-L1719`
- Otherwise a 404 falls through to `respond_with_error`, and because
  `event.isDataRequest` is true `handle_fatal_error` returns
  `Response.json(body, { status })` — JSON, not HTML.
  `reference/kit/packages/kit/src/runtime/server/errors.js:L19-L36`
- If `handle` returns a 3xx Response with `location` for a data request, kit
  converts it to the redirect JSON envelope (`redirect_json_response`).
  `reference/kit/packages/kit/src/runtime/server/respond.js:L572-L579`
- Thrown `Redirect` anywhere in the outer pipeline for a data request →
  `redirect_json_response(e)` with cookies attached.
  `reference/kit/packages/kit/src/runtime/server/respond.js:L452-L461`

### 3. `render_data` — which nodes run and in what order

`reference/kit/packages/kit/src/runtime/server/data/index.js:L21-L140`

- `+server.js`-only route: `/__data.json` returns an empty 404 with the
  `x-sveltekit-version` header. `L29-L32`
- `node_ids = [...route.page.layouts, route.page.leaf]` — node order is
  root layout, nested layouts (with `undefined`/`null` holes for missing
  layouts), leaf page. `invalidated = invalidated_data_nodes ?? all true`. If
  the client sent a shorter bitstring, missing positions are `undefined`
  (falsy) → treated as not invalidated → `skip`. `L35-L36`
- `event.url` for loads is the normalized URL: `url.pathname = normalize_path(url.pathname, trailing_slash)`. `L40-L43`
- Each node's loader is wrapped in `once(...)`; `functions[i]` calls
  `load_server_data({ event, node, parent })` where `parent()` awaits
  `functions[0..i-1]` in order and `Object.assign`s their `.data` — so parents
  run lazily only when a child calls `parent()` or when they themselves are
  invalidated. A node that is NOT invalidated returns `{ type: 'skip' }`
  without running, BUT if a later invalidated node calls `await parent()` the
  parent's load DOES execute (via `functions[j]()`), its result is used for
  the merged parent data, and yet the response slot for that parent stays
  `skip`. `L45-L91`
- Node loads run concurrently (`Promise.all` over `promises`); errors from any
  node: `Redirect` rethrows (whole response becomes a redirect envelope);
  anything else is passed through `handle_error_and_jsonify` and becomes
  `{ type: 'error', error: App.Error }` in that node's slot. Once one node
  throws, `aborted = true` and any node loaders not yet started return `skip`.
  `L46-L52, L76-L79, L94-L111`
- `manifest._.nodes[n]()` is `undefined`/`null` for missing layouts → `load_server_data` returns `null` → serialized as literal `null`. `L55`,
  `reference/kit/packages/kit/src/runtime/server/page/load_data.js:L21`

### 4. `load_server_data` — node shape and `uses` tracking

`reference/kit/packages/kit/src/runtime/server/page/load_data.js:L20-L189`

- No `+page.server.js`/`+layout.server.js` → returns `null`. Has server
  module but no `load` → `{ type: 'data', data: null, uses, slash }` (all
  `uses` empty; `slash = node.server.trailingSlash`). `L21, L34-L40`
- `uses` initial: `{ dependencies: Set, params: Set, parent: false, route: false, url: false, search_params: Set }`. `L25-L32`
- Tracking rules (what Go must emulate when the Go load touches things):
  - `url.href|pathname|search|toString|toJSON` (and `hash` only if allowed) → `uses.url = true`; `url.searchParams.get/getAll/has(k)` → `search_params.add(k)`; any other `searchParams` access (iteration, `keys`, `entries`, `size`…) → `uses.url = true`. `reference/kit/packages/kit/src/utils/url.js:L114-L157`
  - `params.<k>` read → `uses.params.add(k)`. `L117-L132`
  - `route.id` read → `uses.route = true`. `L145-L160`
  - `await parent()` → `uses.parent = true`. `L133-L144`
  - `depends(...deps)` → each resolved via `new URL(dep, event.url).href` and added to `dependencies`. `L100-L116`
  - `fetch()` inside a server load is NOT added to dependencies ("security concerns"). `L96-L97`
  - `untrack(fn)` disables tracking during `fn`. `L162-L169`
- Returns `{ type: 'data', data: result ?? null, uses, slash }`. `L183-L188`
- `slash` = `node.server.trailingSlash` (only from the server module of that
  node; a TODO notes it ignores layouts' `trailingSlash`). `L36`

### 5. Serialization — `server_data_serializer_json`

`reference/kit/packages/kit/src/runtime/server/page/data_serializer.js:L130-L217`

- Per node string:
  - `null` node → `null`
  - `{type:'error'}` / `{type:'skip'}` → plain `JSON.stringify(node)`
  - data node → `{"type":"data","data":<devalue.stringify(node.data, reducers)>,"uses":<JSON uses>}` plus `,"slash":"<always|never|ignore>"` only when `node.slash` is set. `L187-L208`
- `serialize_uses` emits only non-empty keys: `dependencies: string[]`,
  `search_params: string[]`, `params: string[]`, and `parent: 1`, `route: 1`,
  `url: 1` (the number 1, not `true`). Empty `uses` serialize as `{}`.
  `reference/kit/packages/kit/src/runtime/server/utils.js:L77-L97`
- Head document: `{"type":"data","nodes":[<n0>,<n1>,...]}\n` — note the
  trailing newline; `nodes` is an array with one slot per `[...layouts, leaf]`
  entry, `null` for holes. `L210-L215`
- Reducers = user `transport` encoders (`encoders[key](value)` → `["key", encoded]`)
  plus a built-in `Promise` reducer: any thenable is replaced with a numeric
  id starting at 1 (`promise_id++`), i.e. flattened as `["Promise", <id>]`
  inside the devalue array. `L135-L182`
- Deferred chunk: when the promise settles it is serialized as
  `{"type":"chunk","id":<id>,"data":<devalue>}\n` or
  `{"type":"chunk","id":<id>,"error":<devalue App.Error>}\n`. Rejections go
  through `handle_error_and_jsonify`; a serialization failure of the resolved
  value becomes an error chunk with message
  `Failed to serialize promise while rendering <route.id>`. Chunks may
  themselves contain further promises (nested ids). `L148-L176`
- Serialization failure of the top-level node throws with a clarified message
  `Data returned from \`load\` while rendering <route.id> is not serializable: ... (data.<path>)`. `L203-L207`, `reference/kit/packages/kit/src/runtime/server/utils.js:L58-L72`
- devalue flattened JSON (what `devalue.stringify` produces and
  `devalue.unflatten` consumes): a JSON array; element 0 is the root; objects
  map keys to indexes; arrays hold indexes; special numbers as negative
  sentinels `UNDEFINED=-1, HOLE=-2, NAN=-3, +Inf=-4, -Inf=-5, -0=-6, SPARSE=-7`;
  tagged values `["Date","<iso>"]`, `["BigInt","<n>"]`, `["RegExp",src,flags]`,
  `["Map", k1,v1,...]`, `["Set", ...]`, `["Object", idx]` (boxed primitives),
  `["null", ...]` (null-proto), `["URL", str]`, `["URLSearchParams", str]`,
  typed arrays, and custom `["<reducerKey>", idx]`. Strings/booleans/
  non-special numbers are stored directly. Repeated references dedupe by
  index. `reference/devalue/src/constants.js:L1-L7`, `reference/devalue/src/stringify.js:L70-L130, L155-L270`

### 6. Response framing, status, headers

`reference/kit/packages/kit/src/runtime/server/data/index.js:L112-L156`

- No promises (`chunks === null`): `json_response(data)` → status 200, body is
  the head document (already a string, still ends with `\n`), headers
  `content-type: application/json`, `cache-control: private, no-store`, plus
  `x-sveltekit-version: <kit.version.name>` (only when
  `__SVELTEKIT_APP_VERSION_CHECKS_ENABLED__`, i.e. bundleStrategy !== 'inline').
  `L146-L156`, `reference/kit/packages/kit/src/runtime/server/utils.js:L47-L52`
- With promises: streamed `Response(stream_text(data, chunks))` — head string
  first, then each non-empty chunk, no extra separators (each line already
  ends with `\n`). Headers: `content-type: text/sveltekit-data` (proprietary
  "to prevent buffering"), `cache-control: private, no-store`, version header.
  Status 200 (default). `L120-L129`, `reference/kit/packages/kit/src/runtime/utils.js:L35-L44`
- Chunk order: `create_async_iterator` yields chunks in completion order (not
  declaration order), and the iterator keeps draining while new promises are
  added by resolved chunks. `reference/kit/packages/kit/src/utils/streaming.js:L11-L39`
- Redirect envelope: `{"type":"redirect","status":<3xx>,"location":"<str>"}`
  with HTTP status **200** and `application/json` (status is inside the body,
  not the HTTP status). `L161-L169`
- Fatal error (thrown outside node handling): `json_response(App.Error, App.Error.status)` — the HTTP status equals the error's status and the body
  is the bare `App.Error` object `{status, message, ...}` (no `type`). `L130-L139`
- `kit.text()` adds `content-length` automatically for non-streamed bodies.
  `reference/kit/packages/kit/src/exports/index.js:L183-L199`
- Additional headers from `event.setHeaders`, `set-cookie` from
  `event.cookies`, and (page+endpoint routes on GET/HEAD) `Vary: Accept`
  are applied in `respond.js` after `resolve`.
  `reference/kit/packages/kit/src/runtime/server/respond.js:L508-L531, L718-L739`
- ETag 304 handling applies only if the response has an `etag` header (data
  responses don't by default). `L541-L570`

### 7. How the client consumes it

`reference/kit/packages/kit/src/runtime/client/client.js:L3653-L3764`

- `!res.ok` → builds `App.Error`: if `content-type` includes
  `application/json`, `{ status: res.status, ...(await res.json()) }`;
  else `{ status, message: 'Internal Error' }` (or `'Not Found'` for 404).
  Thrown as `HandledHttpError` — the client does NOT call `handleError` again. `L3671-L3684`
- `process_stream` reads the body with `read_ndjson` (split on `\n`, `JSON.parse` each non-blank trimmed line, UTF-8 `fatal: true`). `reference/kit/packages/kit/src/runtime/client/ndjson.js:L8-L15`, `reference/kit/packages/kit/src/runtime/client/stream.js:L9-L46`
- First line: `type === 'redirect'` → resolve immediately;
  `type === 'data'` → for each node with `type === 'data'`: `uses = deserialize_uses(uses)`, `data = devalue.unflatten(data, { ...app.decoders, Promise: id => new Promise(...) })`, resolve. `L3721-L3735`
- Subsequent `type === 'chunk'` lines: look up `deferreds.get(id)`, reject with
  `deserialize(error)` or fulfil with `deserialize(data)`. `L3736-L3747`
- `deserialize_uses` tolerates missing keys (`uses?.dependencies ?? []` etc.; `parent/route/url` coerced with `!!`). `L3755-L3764`
- In `load_route`: node `type === 'error'` → thrown as `HandledHttpError(node.error)`; `type === 'skip'` or index missing → previous data reused (`create_data_node`); server data for node `i` is `server_data.nodes[i]`. `L1499-L1540, L1368-L1372`
- Invalidation bitmask computation (client side): for each loader in
  `[...layouts, leaf]`, invalid iff loader has server load (`loader[0]`) AND
  (loader identity changed OR `has_changed(parent_invalid, route_changed, url_changed, search_params_changed, previous.server.uses, params)`);
  `parent_invalid` cascades downward once any ancestor is invalid. If the
  action result is an error the leaf is forced not-invalid. If no node is
  invalid, no `__data.json` request is made at all. `L1440-L1487`
- `has_changed`: `force_invalidation` (invalidateAll) → true; `uses.parent && parent_changed`; `uses.route && route_changed`; `uses.url && url_changed`; any tracked search param in the changed set; any tracked param value differs; any `dependencies` href matched by an `invalidate()` predicate. `L1332-L1361`
- Response `x-sveltekit-version` is fed to `notify_version`; a differing value
  marks the app as updated, which triggers a full reload on the next ≥400 or
  failed navigation. `L3669`, `reference/kit/packages/kit/src/runtime/app/state/client.svelte.js:L196-L200`
- `slash` from the node (or universal `trailingSlash`) is used client-side to
  normalize the URL after navigation. `L1299, L1009-L1024`

### 8. Types

- `ServerNodesResponse = { type: 'data'; nodes: Array<ServerDataNode | ServerDataSkippedNode | ServerErrorNode | null> }`; `ServerRedirectNode = { type: 'redirect'; status; location }`; `ServerDataNode = { type: 'data'; data: Record|null; uses: Uses; slash?: TrailingSlash }`; `ServerDataChunkNode = { type: 'chunk'; id: number; data?; error? }`; `ServerDataSkippedNode = { type: 'skip' }`; `ServerErrorNode = { type: 'error'; error: App.Error }`; `Uses = { dependencies: Set<string>; params: Set<string>; parent; route; url: boolean; search_params: Set<string> }`.
  `reference/kit/packages/kit/src/types/internal.d.ts:L311-L323, L375-L410, L547-L554`

## Citations

- `reference/kit/packages/kit/src/runtime/server/respond.js:L93-L176, L238, L297-L382, L389-L422, L452-L467, L572-L579, L645-L653, L760-L778`
- `reference/kit/packages/kit/src/runtime/server/data/index.js:L21-L169`
- `reference/kit/packages/kit/src/runtime/server/page/load_data.js:L20-L189`
- `reference/kit/packages/kit/src/runtime/server/page/data_serializer.js:L130-L217`
- `reference/kit/packages/kit/src/runtime/server/utils.js:L47-L52, L58-L72, L77-L97`
- `reference/kit/packages/kit/src/runtime/server/errors.js:L19-L120`
- `reference/kit/packages/kit/src/runtime/utils.js:L35-L44`
- `reference/kit/packages/kit/src/utils/streaming.js:L11-L39`
- `reference/kit/packages/kit/src/utils/url.js:L65-L75, L114-L157`
- `reference/kit/packages/kit/src/pathname.js:L1-L22`
- `reference/kit/packages/kit/src/runtime/shared.js:L18-L20`
- `reference/kit/packages/kit/src/runtime/client/client.js:L1332-L1372, L1440-L1540, L1703-L1730, L3653-L3764`
- `reference/kit/packages/kit/src/runtime/client/ndjson.js:L8-L15`, `reference/kit/packages/kit/src/runtime/client/stream.js:L9-L46`
- `reference/kit/packages/kit/src/types/internal.d.ts:L311-L323, L375-L410, L547-L554`
- `reference/devalue/src/constants.js:L1-L7`, `reference/devalue/src/stringify.js:L70-L130`

## Go implementation notes

Must reproduce for kit's client:

1. Detect `has_data_suffix` (both `/__data.json` and `.html__data.json`), strip
   it, re-add `/` iff `x-sveltekit-trailing-slash=1`, parse and delete both
   `x-sveltekit-*` params, then match the route on the resulting pathname
   (after base strip + `decode_pathname`). Empty result → `/`.
2. Build `node_ids = [...layouts, leaf]` in manifest order (holes preserved as
   `null` slots) and honour the bitmask positionally. Missing bitmask → all
   true. Short bitmask → remaining nodes `skip`.
3. Execute invalidated Go loads concurrently; `parent()` must lazily run
   ancestors (even non-invalidated ones) and merge their data in order; a
   non-invalidated ancestor still reports `skip` in its slot.
4. Track `uses` exactly: the Go load API should expose `URL`, `Params`,
   `RouteID`, `Parent()`, `Depends()`, `Untrack()` that record into a `Uses`
   struct; `event.fetch` must NOT record dependencies. Emit only non-empty
   keys and `1` for the booleans.
5. Emit `{"type":"data","nodes":[...]}\n` where each data node is
   `{"type":"data","data":<devalue-flattened>,"uses":{...}[,"slash":"..."]}`,
   with holes `null`, skips `{"type":"skip"}`, errors `{"type":"error","error":{...}}`.
   A Go devalue *flattener* (`stringify` semantics, not `uneval`) is required —
   including the `["Promise", id]` reducer for streamed values and user
   `transport` encoders (`["<key>", <encoded>]`) if skgo supports transport.
6. Streaming: if any promise-like values exist, respond with
   `content-type: text/sveltekit-data`, write the head line, flush, then write
   each `{"type":"chunk","id":N,"data"|"error":...}\n` as it resolves (any
   order). Otherwise `application/json` with `content-length`. Always
   `cache-control: private, no-store`; add `x-sveltekit-version` matching the
   client bundle's `kit.version.name` so `updated` detection keeps working.
7. Redirects: HTTP 200 + `{"type":"redirect","status":S,"location":L}`.
   Thrown/fatal errors: HTTP status = `App.Error.status`, body = bare
   `App.Error` JSON. Per-node load errors: HTTP 200 with the error node inline.
8. 404 (no route): return JSON `{"status":404,"message":"Not Found"}` with
   HTTP 404 (client only needs `application/json` content-type to read the
   body), except the root-layout special case (bitmask exactly `1`, root
   layout has server load) which must return a normal data envelope with one
   node so the client can render its root error page.
9. `+server`-only route → empty 404 (with version header).
10. The client sends no `Accept: application/json`; do not gate on it. Cookies
    set during loads must be attached as `set-cookie` (see request-pipeline.md
    for path resolution rules).

Node-internal / ignorable: `record_span` tracing, `with_request_store`,
`once()` memo, DEV warnings about post-return access, `disable_search` during
prerender, `state.prerendering.dependencies` capture, `text/sveltekit-data`
being chosen "for inspectability" (any non-buffering content-type would
technically work but keep the exact string).

## Gotchas

- `uses.parent/route/url` serialize as `1`, not `true`; empty sets are omitted
  entirely. The client's `deserialize_uses` is lenient, but keep parity.
- `slash` is only present when the node's own server module declares
  `trailingSlash`; the client falls back to `'never'` and normalizes the URL
  from it. If Go generates `+page.server.ts` stubs that lack `trailingSlash`
  while the Go side sets it, the client's `slash` will still come from Go's
  envelope — that is the source of truth for CSR.
- The redirect envelope is HTTP 200. A real 3xx from a data request only
  happens if `handle` returns one, and kit converts it to the envelope anyway.
- Layout holes: `page.layouts` can contain `undefined` (route without a
  `+layout` at some depth); the envelope must keep the slot as `null` so
  indices line up with the client's `loaders` array.
- Once any node throws, later not-yet-started nodes report `skip`, not
  `error`; only the throwing node is `error`. The client will throw at the
  first error node index and render the nearest `+error.svelte`.
- `text/sveltekit-data` responses have no `content-length`; do not add one for
  streamed bodies.
- `event.url.searchParams` iteration (not `get/has/getAll`) flips `uses.url`,
  which is much broader than tracking a param name. A Go API should mirror
  this asymmetry to avoid under-invalidation.
- The client never re-runs `handleError` on a node-level error — the server's
  `handleError` output (`App.Error`) is shown verbatim. Keep messages generic
  for `unknown` errors: `{ status: 500, message: 'Internal Error' }`.
- A `HandledHttpError` (thrown on the server from a previously-handled body)
  bypasses `handleError` and its `.body` is used as-is.
  `reference/kit/packages/kit/src/runtime/server/errors.js:L45-L47`

## Recipes

Reference envelope for route `/blog/[slug]` with layouts `[0, undefined, 3]`,
leaf `5`, bitmask `1011` (node 1 is a hole, node 2 = layout 3 skipped):

```
{"type":"data","nodes":[{"type":"data","data":[{"user":1},"tyler"],"uses":{}},null,{"type":"skip"},{"type":"data","data":[{"post":1,"comments":3},{"title":2},"Hello",["Promise",1]],"uses":{"params":["slug"],"parent":1},"slash":"never"}]}
{"type":"chunk","id":1,"data":[[1,2],{"body":3},{"body":4},"a","b"]}
```

- Response headers for the streamed case: `content-type: text/sveltekit-data`,
  `cache-control: private, no-store`, `x-sveltekit-version: <name>`.

Go pseudo-flow:

```
if hasDataSuffix(path) {
  page := stripDataSuffix(path); if q.Get("x-sveltekit-trailing-slash")=="1" { page += "/" }; if page=="" { page="/" }
  q.Del(both); bitmask := parse(q.Get("x-sveltekit-invalidated"))
  route, params := match(decode(stripBase(page)))
  if route == nil { if len(bitmask)==1 && bitmask[0] && rootHasServerLoad { renderData(synthetic root route) } else { json 404 {"status":404,"message":"Not Found"} } }
  if route.page == nil { 404 empty + version header }
  renderData(route, bitmask, trailingSlash(route))
}
```
