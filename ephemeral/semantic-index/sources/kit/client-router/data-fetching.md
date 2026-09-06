# `__data.json`: exact request the client builds, and the response it can parse

## Purpose

Under skgo every server `load` runs in Go and is fetched by kit's unmodified client router via `__data.json`. This leaf pins the request (URL suffix, query params, method, credentials, headers), the invalidation bitmask and `uses` semantics that decide *when* the client asks, and the response grammar (ndjson lines, node types, devalue-flattened `data`, deferred chunks, redirect/error nodes, status codes) that the client's parser accepts. The server-side emitter is another worker's segment; here we cite kit's server emitter only to pin the wire format the client consumes.

## Key facts (source-derived)

### Request construction (`load_data`)

- `load_data(url, invalid)` (`reference/kit/packages/kit/src/runtime/client/client.js:L3653-3691`):
  - `data_url = new URL(url); data_url.pathname = add_data_suffix(url.pathname)`.
  - `add_data_suffix`: if pathname ends with `.html` → replace with `.html__data.json`; else strip one trailing `/` and append `/__data.json` (`reference/kit/packages/kit/src/pathname.js:L1-13`). So `/` → `/__data.json`, `/blog/` → `/blog/__data.json`, `/a.html` → `/a.html__data.json`.
  - If `url.pathname.endsWith('/')` → `data_url.searchParams.append('x-sveltekit-trailing-slash', '1')` (`L3656-3658`; constant `TRAILING_SLASH_PARAM` at `runtime/shared.js:L20`).
  - Always: `data_url.searchParams.append('x-sveltekit-invalidated', invalid.map(i => i ? '1' : '0').join(''))` (`L3662`; constant `INVALIDATED_PARAM` at `runtime/shared.js:L18`). Existing query params of the page URL are preserved and come first; `append` never replaces them. In DEV a page URL already containing `x-sveltekit-invalidated` throws (`L3659-3661`).
  - Fetch: `window.fetch(data_url.href, {})` — **GET**, no custom headers, default `credentials: 'same-origin'`, default `redirect: 'follow'` (`L3664-3666`). In DEV it goes through `dev_fetch`, which only adds a non-enumerable `__sveltekit_fetch__` flag (`runtime/client/fetcher.js:L143-152`).
- These params are **query parameters, not HTTP headers**, despite their `x-sveltekit-` names. The server strips them: `url.pathname = strip_data_suffix(pathname) + (TRAILING_SLASH_PARAM === '1' ? '/' : '') || '/'`, then deletes both params and splits the invalidated string into `boolean[]` (`reference/kit/packages/kit/src/runtime/server/respond.js:L156-165`).

### Invalidation bitmask semantics

- `load_route` (`client.js:L1412-1635`) builds `loaders = [...route.layouts, route.leaf]` (`L1421`). `route.layouts` is `[0, ...layouts]` mapped to `[has_server_load, loader] | undefined` (gaps preserved), `route.leaf` is `[has_server_load, loader]` (`runtime/client/parse.js:L7-58`). The bitmask has **one character per slot of `loaders`**, in that order: root layout, nested layouts (with `undefined` gaps contributing `'0'`), then the leaf page. The server pairs it with `node_ids = [...route.page.layouts, route.page.leaf]` index-for-index (`runtime/server/data/index.js:L35-36`, `L83-91`).
- A slot is `'1'` iff (`L1441-1464`):
  - the loader has a server load (`loader[0]` true — from `server_loads` for layouts and the `~leaf` ones'-complement encoding for pages, `parse.js:L41-57`), **and**
  - either the loader identity differs from `current.branch[i].loader` (different route / first navigation), **or** `has_changed(parent_invalid, route_changed, url_changed, search_params_changed, previous.server?.uses, params)` is true.
  - After any `'1'`, `parent_invalid = true` for subsequent slots — but it only invalidates a later slot if that slot's `uses.parent` is set (`L1344`).
  - When `action_result.type === 'error'` the leaf is forced `'0'` (`L1442`).
- `has_changed` (`L1332-1361`): `force_invalidation` → true; no `uses` → false; `uses.parent && parent_changed`; `uses.route && route_changed`; `uses.url && url_changed`; any `uses.search_params` key in `diff_search_params(current.url, url)` (`L1378-1396`, compares sorted `getAll` value lists per key); any `uses.params` key whose value differs from `current.params`; any `uses.dependencies` href for which some registered predicate returns true (`invalidated.some(fn => fn(new URL(href)))`, `L1356-1358`).
  - `url_changed` = `id !== get_page_key(current.url)` where the key is `pathname + search` (hash-mode: hash + search) (`L1433`, `L1904-1906`). `route_changed` = `!current.route || route.id !== current.route.id` (`L1435`).
- Predicates come from `invalidate(resource)`: a function is stored as-is; a string/URL becomes `url.href === new URL(resource, location.href).href` (`L2738-2753`). `invalidateAll()` / `refreshAll()` set `force_invalidation = true` (`L2763-2776`); `goto(url, { refreshAll | invalidateAll })` sets it inside `accept()` and also `discard_load_cache()` (`L745-790`, `L2710-2716`). Same-URL link clicks navigate with `refreshAll: !changed`, i.e. clicking a link to the current URL refetches everything (`L3319-3327`).
- `reset_invalidation()` clears predicates and `force_invalidation` **only after a finished navigation**; redirects and follow-up invalidations reuse the same invalidation (`L2110-2112`, `L694-697`).
- If no slot is `'1'`, **no request is made** (`L1466`). If `__SVELTEKIT_HAS_SERVER_LOAD__` is compiled false, the entire block is absent (`L1440`).
- Universal (`+page.js`) loads are re-run client-side by the same `has_changed` logic against `previous.universal?.uses` (`L1502-1515`); their tracked `uses` come from the proxied `route`/`params`/`url`/`fetch`/`depends`/`parent` in `load_node` (`L1143-1301`). Universal `fetch` registers `resolved.href` as a dependency (`L1254-1258`).

### Response status handling

- `notify_version(res.headers.get('x-sveltekit-version'))` first (`L3669`).
- `!res.ok` (`L3671-3684`): `error = { status: res.status, message: 'Internal Error' }`; if `content-type` includes `application/json` → `error = { status: res.status, ...await res.json() }`; else if status is 404 → `message = 'Not Found'`. Thrown as `HandledHttpError(error)` (`HttpError` constructor takes the `App.Error` and copies `.status`, `reference/kit/packages/kit/src/exports/internal/shared.js:L3-24`), which `handle_error` returns verbatim **without** calling the client `handleError` hook (`L2517-2520`).
  - In `load_route` this becomes `load_root_error_page({ error, url, route })` (`L1467-1481`) — the root layout + root error page; for a preload it is thrown instead (`L1472-1474`).
  - In `load_root_error_page`, a 404 from `__data.json` never triggers a full reload; other failures reload only if the URL differs from the current location or the page was SSR-hydrated (`L1720-1730`).
- OK responses (2xx) are streamed through `process_stream` regardless of `content-type` (`L3686-3688`). kit's server uses `application/json` when there are no deferred chunks and `text/sveltekit-data` when streaming, both with `cache-control: private, no-store` (`runtime/server/data/index.js:L114-129`, `L146-156`); the client never inspects the content type on success.
- A real HTTP 3xx would be **followed by `fetch`** and the redirected body parsed as data — redirects must be expressed as a 200 JSON `redirect` node (below). kit's server does exactly that (`data/index.js:L133-134`, `L161-169`).
- A `+server.js`-only route answers `__data.json` with an empty 404 (`data/index.js:L29-32`).

### Response body grammar (`process_stream`, `read_ndjson`)

- Body is read as UTF-8 with `TextDecoder(undefined, { fatal: true })` and split on `'\n'`; each non-blank trimmed line is `JSON.parse`d (`runtime/client/ndjson.js:L8-15`, `runtime/client/stream.js:L9-46`). Invalid UTF-8 or invalid JSON rejects the whole load.
- Line 1 must be one of:
  - `{"type":"redirect","status":<3xx>,"location":"<string>"}` — resolves immediately (`client.js:L3722-3724`); `load_route` returns it (`L1483-1485`) and `navigate` follows it client-side: `new URL(location, url)`, max 20 hops, then a "Redirect loop" root error page (`L2072-2101`). `_invalidate` follows it with `_goto(..., { replace: true })` (`L649-656`).
  - `{"type":"data","nodes":[ ... ]}` — one entry per bitmask slot, in order (`L3726-3735`). Entries:
    - `null` — no server load at that slot (client treats as "no server data": `create_data_node(null)` → `null`, `L1368-1372`).
    - `{"type":"skip"}` — not re-run; client reuses `previous.server` for that slot (`L1370`, `L1534-1539`).
    - `{"type":"error","error":{"message":"...","status":<n>[,...App.Error]}}` — client throws `HandledHttpError(error)` for that slot (`L1517-1520`) and renders the nearest `+error.svelte` above it with `page.status = error.status ?? 200` (`L1553-1595`, `L1042-1044`, `L1120`). `status` inside `error` is what sets the page status — omit it and the error page renders as 200.
    - `{"type":"data","data":<devalue flattened JSON>,"uses":{...}[,"slash":"always"|"never"|"ignore"]}` — `node.uses = deserialize_uses(node.uses)`, `node.data = devalue.unflatten(node.data, { ...app.decoders, Promise: id => deferred })` (`L3710-3719`, `L3728-3733`).
  - An entry index beyond what the server sent is `undefined`; combined with `loader[0]` it is treated as `skip` (reuse previous), otherwise as `null` (`L1534-1539`).
- Subsequent lines (only after a `data` first line): `{"type":"chunk","id":<n>,"data":<devalue flattened>}` or `{"type":"chunk","id":<n>,"error":<devalue flattened App.Error>}` (`L3736-3747`). The client looks up `deferreds.get(id)` created while unflattening a `Promise` placeholder; `data` fulfils, `error` rejects. A chunk whose `id` was never declared throws a TypeError inside the already-resolved stream (unhandled rejection) — never emit orphan chunks. Any other `type` on later lines is ignored (`L3721-3748`).
- `uses` deserialisation (`L3755-3764`): `dependencies`, `params`, `search_params` become `Set`s from arrays (missing → empty); `parent`, `route`, `url` become booleans (missing → false). kit's server emits only the non-empty keys (`runtime/server/utils.js:L77-...`, `serialize_uses`). `dependencies` entries are compared via `new URL(href)` at invalidation time (`L1357`), so they must be absolute URLs or scheme-prefixed ids (`app:foo`) — a bare relative path throws during the *next* navigation.
- `slash` is folded into the branch node (`load_node` returns `slash: node.universal?.trailingSlash ?? server_data_node?.slash`, `L1299`) and used by `get_navigation_result_from_branch` to normalise `url.pathname` with the last defined node's option (`L1009-1024`); the history entry uses the normalised pathname (`L2124-2127`). Default when nothing sets it: `'never'`, except `base` root which is always `'always'` (`L1010-1015`).

### devalue flattened format (what `data` must look like)

- `data` must be the output of `devalue.stringify(value, reducers)` (pinned `devalue` `^5.9.0`, cache copy 5.9.2: `reference/kit/packages/kit/package.json:L26`; `reference/devalue/package.json:L4`). kit's server does `devalue.stringify(node.data, { ...encoders, Promise })` (`runtime/server/page/data_serializer.js:L135-182`, `L199-202`).
- Shape: a JSON array; element 0 is the root; objects are `{"key": <index>}`, arrays `[<index>, ...]`, strings/numbers/booleans/null inline; sentinel negative numbers: `-1` undefined, `-2` hole, `-3` NaN, `-4` +Infinity, `-5` -Infinity, `-6` -0 (`reference/devalue/src/constants.js:L1-6`). Built-in typed values are `["Date", "<iso>"]`, `["Set", i, ...]`, `["Map", k, v, ...]`, `["RegExp", src, flags]`, `["BigInt", "<digits>"]`, `["null", {...}]` for null-prototype objects, etc. A reducer hit is `["<reducerName>", <index of flattened encoded value>]`.
- The `Promise` reducer returns the numeric id (`data_serializer.js:L138-181`), so a streamed promise appears as `["Promise", <index>]` where the referenced element is the integer id; the client's `Promise: (id) => new Promise(...)` reviver receives that integer (`client.js:L3713-3717`). The matching chunk line is `{"type":"chunk","id":<id>,"data":<devalue.stringify(value, reducers)>}` (`data_serializer.js:L174`).
- Custom `transport` types use the same `["<name>", idx]` form with `app.decoders[name]` as reviver — see `transport-hook.md`.

## Citations

- `reference/kit/packages/kit/src/runtime/client/client.js:L745-810` (_goto), `L1143-1301` (load_node), `L1332-1396` (has_changed, create_data_node, diff_search_params), `L1412-1635` (load_route), `L1696-1731` (root error page data fetch), `L2072-2112` (redirect handling, reset_invalidation), `L2738-2776` (invalidate/invalidateAll/refreshAll), `L3653-3764` (load_data, process_stream, deserialize_uses)
- `reference/kit/packages/kit/src/runtime/client/parse.js:L7-58`
- `reference/kit/packages/kit/src/runtime/client/ndjson.js:L1-15`; `reference/kit/packages/kit/src/runtime/client/stream.js:L1-46`
- `reference/kit/packages/kit/src/runtime/client/fetcher.js:L143-152`
- `reference/kit/packages/kit/src/runtime/shared.js:L18-20`
- `reference/kit/packages/kit/src/pathname.js:L1-22`
- `reference/kit/packages/kit/src/exports/internal/shared.js:L3-24`
- `reference/kit/packages/kit/src/types/internal.d.ts:L311-323`, `L375-410`, `L547-554`
- `reference/kit/packages/kit/src/runtime/server/respond.js:L156-165`
- `reference/kit/packages/kit/src/runtime/server/data/index.js:L21-169`
- `reference/kit/packages/kit/src/runtime/server/page/data_serializer.js:L130-217`
- `reference/kit/packages/kit/src/runtime/server/utils.js:L47-52`, `L77-95`
- `reference/kit/packages/kit/package.json:L26`; `reference/devalue/package.json:L4`; `reference/devalue/src/constants.js:L1-6`

## Go implementation notes

1. **Route the request**: match `GET <base>/<page>/__data.json` (and `<page>.html__data.json`). Strip the suffix, re-add `/` if `x-sveltekit-trailing-slash=1`, delete both `x-sveltekit-*` query params, then run the *same* route matcher as the client (see `navigation-and-manifest.md`) on the decoded, base-stripped pathname. The remaining query string is the page's `url.search` for the loads.
2. **Bitmask**: parse `x-sveltekit-invalidated` into `[]bool`; index i corresponds to `[...route.page.layouts, route.page.leaf][i]` from kit's server manifest (same order as the client dictionary). Missing param ⇒ all true. For `false` slots emit `{"type":"skip"}`; for slots with no server load emit `null`; otherwise run the Go load. Slot count from Go must equal the client's `loaders.length` for that route (root layout + nested layout slots incl. gaps + leaf).
3. **Response**: status 200; first line `{"type":"data","nodes":[...]}` + `\n`. With no deferred promises use `content-type: application/json`; with streaming use `text/sveltekit-data` and flush each `{"type":"chunk",...}\n` as it resolves. Always `cache-control: private, no-store` and `x-sveltekit-version: <kit.version.name>` (or omit the header entirely).
4. **Errors**: a load that fails should become `{"type":"error","error":{"status":N,"message":"..."}}` at its slot, with later slots `{"type":"skip"}`; only a failure *before* any node can be produced should be a non-2xx `application/json` body `{"message":...,"status":N}` (the client re-adds `status` from the HTTP status). A 404 body for an unmatched route: status 404, `application/json`, `{"message":"Not Found"}`.
5. **Redirects**: never send 3xx. Send 200 + `{"type":"redirect","status":302,"location":"/target"}` (relative locations are resolved against the page URL by the client).
6. **`uses`**: Go loads need an explicit dependency-tracking API (params read, url/search params read, `parent()` called, `depends(...)`) to produce `uses`; the cheap correct default is `{"url":true,"route":true,"parent":true}` plus `params` for every route param — this re-runs the load on any change, i.e. never stale. Emit `dependencies` as absolute URLs (`https://host/api/x`) or `scheme:` ids.
7. **`slash`**: emit the route's trailing-slash option on at least one node whenever it is not `'never'`, and normalise the page URL the same way when computing `event.url` for loads.
8. **`data` encoding**: write a Go devalue "stringify" (flattened array + sentinels + built-in types) — this is the load-bearing piece for end-to-end types. Promises: reserve integer ids starting at 1 in the order values are encountered, and stream chunk lines in resolution order.
9. **Idempotency**: `preloadData`/hover preloading issue the same request without a navigation ever happening — loads must be side-effect free and safe to repeat.

## Gotchas

- The query-param names look like headers; they are not headers. Do not look for `X-Sveltekit-Invalidated` in `r.Header`.
- The page URL's own query string precedes the kit params; Go must not assume the kit params are the only ones, and must remove them before exposing `url` to loads (kit deletes them; a stray param changes `uses.search_params` behaviour and `page.url`).
- `null` vs `skip` vs missing are three different things on the client: `null` = "there is no server load here" (data becomes `null`), `skip` = "reuse what you had", missing = treated like `skip` if the client thinks the node has a server load. Always send the full-length array.
- `error.status` inside an error node controls `page.status`; without it the error page renders with status 200 and `updated.check()` is not triggered.
- A non-JSON error body (e.g. Go's default `http.Error` text) yields `message: 'Internal Error'` (or `'Not Found'` for 404) — fine, but user-facing messages require `application/json`.
- `fatal: true` decoding: any invalid UTF-8 byte in the body kills the navigation.
- `dependencies` hrefs are `new URL(href)`-parsed lazily on the next `invalidate()` — a malformed href produces a navigation failure long after the response that carried it.
- `diff_search_params` compares **sorted `getAll` lists**; multi-valued params are order-insensitive.
- A response that is a 2xx but not newline-terminated still works (`read_stream` yields the trailing block), but a body of exactly `""` resolves nothing — the navigation hangs forever. Always emit the data line.

## Recipes

- **Curl the contract**: `curl -i 'http://localhost/blog/hello/__data.json?x-sveltekit-invalidated=101'` → expect `200`, `content-type: application/json`, body `{"type":"data","nodes":[{"type":"data","data":[{"title":1},"Hello"],"uses":{}},{"type":"skip"},{"type":"data","data":[...],"uses":{"params":1,...}}]}`.
- **Go test skeleton**: table of `(routeID, bitmask, expected node types)`; assert `len(nodes) == len(layouts)+1`, and that `bitmask[i]==false ⇒ nodes[i].type=="skip"`.
- **Streaming**: write the first line, `Flush()`, then per resolved promise write `{"type":"chunk","id":k,"data":...}\n` and `Flush()`; set `content-type: text/sveltekit-data` up front so intermediaries do not buffer.
- **Redirect test**: return the redirect node and assert in Playwright that `location.pathname` changed with no full page load (`window.__marker` still set).
