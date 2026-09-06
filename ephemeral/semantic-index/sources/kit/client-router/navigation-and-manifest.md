# Client route manifest, matching, navigation, preloading, `reroute`, and version polling

## Purpose

Go must resolve URLs to the same route and params the client resolves, in the same order, because the client decides *which* `__data.json` slots to request and Go decides *which* loads to run. This leaf pins the generated client manifest (`app.js`) shape, the route regex/matcher semantics, trailing-slash and base handling, `reroute`, the navigation entry points that produce data requests (`goto`, links, `preloadData`, `invalidate*`), and the `version.json` polling contract.

## Key facts (source-derived)

### Generated `app.js` (the `SvelteKitApp` object passed to `start`)

- Written by `write_client_manifest` (`reference/kit/packages/kit/src/core/sync/write_client_manifest.js:L131-175`). Exports:
  - `nodes = [() => import('./nodes/0'), () => import('./nodes/1'), ...]` — one lazy loader per `manifest_data.nodes` entry; with `router.resolution === 'server'` only the first two (`L43-55`). Each `nodes/N.js` exports `component` (the `.svelte` default) and, if a `+page.js`/`+layout.js` exists, `universal` (`L22-41`). Type `CSRPageNode { component, universal: { load?, trailingSlash? } }` (`types/internal.d.ts:L122-130`).
  - `server_loads = [<layout node ids that have a server load>]` (`L57`, `L88-102`, `L153`). Under client routing this includes every layout with `has_server_load`; under server routing only `0` if the root layout has one (`L116-121`).
  - `dictionary = { "<route id>": [leaf, [layouts...], [errors...]] }` for every route with a `+page` (`L59-114`). `leaf` is `~<node index>` (ones' complement, i.e. negative) when the page has a server load (`L81-83`); `layouts`/`errors` are the route's layout/error node ids **after slicing off index 0 (root)**, with `undefined` slots emitted as empty array elements (`[,3]`) and trailing empties trimmed (`L64-68`); the arrays are omitted when empty (`L104-106`).
  - `matchers` — `import { params as matchers } from '<generated params module>'` or `{}` (`L177-188`); `hooks = { handleError, init?, reroute, transport }` (`L157-164`); `decoders`/`encoders` derived from `hooks.transport` (`L166-167`); `hash = <router.type === 'hash'>` (`L169`); `decode(type, value)` (`L171`); `get_error_template()` lazily imports the compiled `src/error.html` (`L173`).
- `has_server_load` for a node is `node.server?.load !== undefined || node.server?.trailingSlash !== undefined` (`reference/kit/packages/kit/src/utils/routing.js:L344-346`) — determined from the **server build's analysed metadata** (`write_client_manifest` receives `metadata`, `L72-79`, `L93-97`).

### Client-side route table (`parse_routes`)

- `parse_routes({ nodes, server_loads, dictionary, matchers })` (`reference/kit/packages/kit/src/runtime/client/parse.js:L7-58`) maps `Object.entries(dictionary)` — **in dictionary insertion order, which is kit's sorted `manifest_data.routes` order** — to `CSRRoute { id, exec(path), errors: [nodes[1], ...errors], layouts: [[server_loads.has(0), nodes[0]], ...], leaf: [leaf < 0, nodes[~leaf]] }`. `errors.length` and `layouts.length` are padded to the same length (`L26-32`).
- Matching: `get_navigation_intent(url, invalidating)` returns undefined if `is_external_url(url, base, app.hash)` (`client.js:L1852-1854`; `utils.js:L229-239`: different origin, or pathname not starting with `base`, or hash-mode pathname mismatch). Otherwise it applies `reroute`, computes `path = decode_pathname(rerouted.pathname.slice(base.length)) || '/'` (hash mode: from `url.hash`), and returns the **first** route whose `exec(path)` returns params (`L1856-1874`, `L1895-1901`). Intent: `{ id: pathname+search, invalidating, route, params, url }` — note `url` is the *original* URL, not the rerouted one (`L1866-1872`).
- `decode_pathname` decodes with `decodeURI` except `%25` sequences (`utils/url.js:L81-83`).
- Regex construction `parse_route_id(id)` (`utils/routing.js:L35-117`): `/` and root groups `/(group)` → `/^\/$/`; otherwise segments from `get_route_segments` (drops `(group)` segments, `L134-136`) joined as `^` + `/segment`... + `/?$`. Per segment: whole-segment `[...rest]` (with optional `=matcher`) → `(?:/([^]*))?`; whole-segment `[[optional]]` → `(?:/([^/]+))?`; otherwise the segment is split on `[param]` boundaries: `[x+hh]`/`[u+hhhh]` escapes are decoded and regex-escaped, params become `([^]*?)` (rest), `([^/]*)?` (optional) or `([^/]+?)` (required), literal text is `escape()`d — with `%`, `/`, `?`, `#` matched in their **encoded** forms (`%25`, `%2[Ff]`, `%3[Ff]`, `%23`) because `decode_pathname` leaves them encoded (`L248-267`). The trailing `/?$` means every route also matches with one trailing slash.
- Param extraction `exec(match, params, matchers)` (`L173-246`): values are `decodeURIComponent`ed; a matcher is run via `matcher['~standard'].validate(value)` (Standard Schema; async results throw) and may **transform** the value (string/number/boolean/bigint allowed) (`L143-166`); a failed matcher on a chained optional param (`[[a=m]]`) skips it and rolls buffered values into a following `[...rest]` (`L188-195`, `L212-222`). A failing matcher on any other param means the route does not match and the loop continues to the next route.
- The server uses the same functions: `find_route(path, manifest._.routes, matchers)` (`utils/routing.js:L356-370`; called from `respond.js:L369-378`) on `resolved_path` after `decode_pathname` and base stripping (`respond.js:L297-302`, `L341-346`).

### Trailing slash and base

- After loads, `get_navigation_result_from_branch` picks `slash` = the last branch node with a defined `slash` (universal `trailingSlash` export or server node `slash`), default `'never'`, and forces `'always'` when `url.pathname` is `base` or `base + '/'` (`client.js:L1009-1020`). It then sets `url.pathname = normalize_path(url.pathname, slash)` and re-assigns `url.search` to drop a bare `?` (`L1022-1024`; `utils/url.js:L65-75`). `navigate` writes this normalised pathname into history (`L2124-2127`, `L2155-2156`).
- `$app/paths`: `base`, `assets`, `app_dir`, `resolve(id|path, params)` (prefixes `base`, and `#` in hash mode), `asset(file)`, `match(url)` (delegates to the client's `get_navigation_intent`) (`runtime/app/paths/client.js:L27-106`; `client.js:L438-453`).

### `reroute` (universal hook) on the client

- `get_rerouted_url(url)` calls `app.hooks.reroute({ url: new URL(url), fetch })`, accepts a `string` (treated as a pathname, or a hash in hash mode) or a `URL`, caches the promise per `href` for the page lifetime, and on a thrown error falls back to **native navigation** (`client.js:L1790-1842`).
- The rerouted URL is used **only for route matching**; `intent.url` and therefore the `__data.json` URL are built from the original URL (`L1860-1872`, `L3654-3655`). The server applies `reroute` itself to the incoming data-request path: `resolved_path = (await hooks.reroute({ url: new URL(url), fetch })) ?? url.pathname` runs after the `__data.json` suffix and kit params are stripped and before `decode_pathname`/`find_route` (`runtime/server/respond.js:L253-280`, `L297-302`, `L367-378`); it is skipped for route-ID resolution requests (`L255`).

### Navigation entry points that cause data requests

- Link clicks: `_start_router`'s click handler filters modifier keys, `target`, non-http protocols, `download`, `data-sveltekit-reload`, hash-only changes, then `_goto(url, { type: 'link', reset, replace: replace_state ?? !changed, refreshAll: !changed, event })` (`client.js:L3206-3328`). Link options are read from `data-sveltekit-preload-code|preload-data|reload|replacestate|reset` on the anchor or any ancestor (`runtime/client/utils.js:L31-37`, `L156-218`).
- GET `<form>` submissions: `navigate({ type: 'form', url: action + serialized FormData })` (`L3330-3374`).
- `goto(url, { replace, state, shallow, reset, refreshAll (alias invalidateAll), invalidate: [...], persistState })` (`L2670-2717`); `shallow` skips loads entirely (`update_state`, `L2957-3038`).
- `invalidate(resource, keepState?)`, `invalidateAll()`, `refreshAll()` → `_invalidate` re-runs `load_route` for the current intent with `invalidating: true`, applies the result in place, follows a redirect via `_goto(..., { replace: true })`, and also refreshes remote queries when forced (`L608-692`, `L2738-2776`).
- `preloadData(href)` → `_preload_data(intent)` → `load_route({ ...intent, preload })`, cached in `load_cache` keyed by page key; the next navigation to the same key consumes it (`L813-856`, `L1413-1417`, `L2790-2825`). Returns `{ type: 'loaded', status, data } | { type: 'redirect', status, location } | { type: 'error', status, error }`. Hover ("mouse comes to rest", 20ms) and tap/touchstart preloading do the same for anchors with `preload-data` (`L2365-2510`); an `IntersectionObserver` preloads *code* for `viewport`, and `eager` preloads code immediately after every navigation (`L2488-2509`). Preloading is disabled when `navigator.connection.saveData` (`L3200-3203`).
- `preloadCode(id)` takes a **route id** (`/blog/[slug]`), finds it in `routes` and imports its layout/leaf modules without running loads (`L2850-2897`).
- popstate/back-forward re-runs `navigate({ type: 'popstate' })` unless the entry is shallow (`L3376-3478`).
- Unmatched non-external URL: `server_fallback` → full `location.href = url` navigation unless it is the current un-hydrated pathname (`L2035-2058`, `L2328-2348`). This is how `+server.js`-only routes (absent from `dictionary`) and non-kit URLs are reached from a link click.

### Server-side route resolution (`router.resolution === 'server'`), for contrast

- The client `import()`s `<pathname>/__route.js` (`add_resolution_suffix`, `pathname.js:L40-43`; `client.js:L1875-1891`) and, for `preloadCode`, `<base>/_app/routes/<route id>/__route.js` (`runtime/pathname.js:L14-16`; `client.js:L865-890`). The module must `export const route = { id, errors, layouts, leaf, nodes: { '<n>': () => import('<asset url>') } }` and optionally `export const params = {...}`; endpoint-only routes export `endpoint_only = true` (`runtime/server/page/server_routing.js:L15-32`, `L105-128`, `L143-157`). `dictionary` is `{}` and `nodes` has two entries in this mode.

### `version.json` and `updated`

- Emitted asset `${appDir}/version.json` = `{"version": kit.version.name}` (`exports/vite/index.js:L1144-1148`). Client: `fetch(`${assets}/${__SVELTEKIT_APP_VERSION_FILE__}`, { headers: { 'cache-control': 'no-cache' } })`, non-OK → false, else `data.version !== version` (`runtime/app/state/client.svelte.js:L158-171`). Poll timer only when `version.pollInterval` > 0 (`L131`, `L176`, `L182-184`); also on `focus`/visible and after 4xx results or failed chunk imports (`client.js:L3187-3198`, `L2102-2107`, `L1575-1581`). `updated.current` becoming true does not reload by itself; a later ≥400 navigation does (`native_navigation`, after trying `registration.update()` on a service worker, `L258-265`).

## Citations

- `reference/kit/packages/kit/src/core/sync/write_client_manifest.js:L15-189`
- `reference/kit/packages/kit/src/runtime/client/parse.js:L1-77`
- `reference/kit/packages/kit/src/runtime/client/types.d.ts:L15-66`; `reference/kit/packages/kit/src/types/internal.d.ts:L122-153`
- `reference/kit/packages/kit/src/utils/routing.js:L35-136`, `L143-246`, `L248-267`, `L344-370`
- `reference/kit/packages/kit/src/utils/url.js:L65-83`
- `reference/kit/packages/kit/src/runtime/client/utils.js:L31-37`, `L125-151`, `L156-239`
- `reference/kit/packages/kit/src/runtime/client/client.js:L438-453`, `L608-692`, `L745-915`, `L1009-1024`, `L1412-1417`, `L1790-1906`, `L2035-2058`, `L2102-2107`, `L2328-2348`, `L2365-2510`, `L2670-2897`, `L3151-3529`
- `reference/kit/packages/kit/src/runtime/app/paths/client.js:L27-106`; `reference/kit/packages/kit/src/runtime/app/paths/internal/client.js:L8-36`
- `reference/kit/packages/kit/src/runtime/app/state/client.svelte.js:L131-200`
- `reference/kit/packages/kit/src/runtime/pathname.js:L14-37`; `reference/kit/packages/kit/src/pathname.js:L24-55`
- `reference/kit/packages/kit/src/runtime/server/page/server_routing.js:L15-60`, `L105-157`
- `reference/kit/packages/kit/src/runtime/server/respond.js:L253-280`, `L297-378`
- `reference/kit/packages/kit/src/exports/vite/index.js:L1144-1148`, `L1250-1258`, `L1391-1427`

## Go implementation notes

1. **Use kit's route order and ids verbatim.** The adapter should export the server manifest's `_.routes` (ordered, with `id`, `pattern` source, `params[]`, `page: { layouts, errors, leaf }`) to Go at build time. Go iterates in that order and takes the first match, exactly like `get_navigation_intent`. Do not re-sort.
2. **Port `parse_route_id` + `exec` to Go**, or precompile the patterns in the adapter and ship regex source + param metadata; Go's `regexp` is RE2 — kit's patterns use `[^]` (JS "any char"), lazy quantifiers `*?`/`+?`, and non-capturing/optional groups; translate `[^]` to `[\s\S]` (or `(?s).`), and note RE2 supports lazy quantifiers. Test parity with kit's `routing.spec.js` cases.
3. **Matchers transform values.** A `src/params/*.ts` matcher can return a number/boolean/bigint; the client passes the transformed value as `params`, and kit's server does the same for `event.params`. Go-side matchers must be reimplemented in Go with identical accept/transform semantics (or matchers must be restricted to pure string validation that Go mirrors). Plan for a Go matcher registry keyed by matcher name.
4. **Only `+page` routes are in the client dictionary.** `+server.js`-only routes are reached by full navigation; Go serves them directly. A route with both `+page` and `+server` is matched client-side for navigation and via `__data.json` for data; GET on the page URL with `Accept: text/html` must serve the shell, not the endpoint.
5. **Go-owned loads must exist in kit's tree** so `has_server_load` is true: the adapter/codegen should write `+page.server.ts`/`+layout.server.ts` stubs exporting `load` (a stub is enough — the metadata analysis only checks the export exists) wherever Go registers a load. Otherwise the client will never request that slot and `__SVELTEKIT_HAS_SERVER_LOAD__` may compile the fetch away entirely.
6. **Trailing slash**: Go needs each route's effective `trailingSlash` (nearest node's server/universal option) to normalise `event.url` for loads and to emit `slash` in `__data.json`; the client will `replaceState` to the normalised URL after the load.
7. **`reroute`**: universal `reroute` in `hooks.ts` runs in the browser; Go cannot execute it. Options: (a) skgo forbids `reroute` (build-time check in the adapter), (b) skgo ships a Go-side `Reroute` and requires the JS one to be a pure mirror, or (c) the adapter compiles a restricted declarative reroute table. Whatever the choice, Go must apply it to the **data-request path** because the client requests `__data.json` for the *original* pathname.
8. **Preload traffic**: expect `__data.json` requests for pages the user never opens (hover/tap preload); rate-limit by treating them as ordinary GETs, never as intent.
9. **Serve `/_app/version.json`** and keep `kit.version.name` stable per deployment; optionally add `x-sveltekit-version` to data responses so tabs learn about a redeploy sooner.

## Gotchas

- Dictionary routes are keyed by kit route id (`/blog/[slug]`, groups included like `/(app)/dashboard`); the regex ignores group segments. Params from `exec` are `decodeURIComponent`ed strings unless a matcher transforms them.
- `get_url_path` strips `base` by length, then `decode_pathname`; the server decodes and strips base in the opposite order but with the same result unless `base` contains percent-encodings. Keep `base` plain ASCII.
- The client compares `id !== get_page_key(current.url)` using `pathname + search` (no hash) — hash-only changes are handled without any request.
- `preloadCode` wants a route id, not a pathname; passing a pathname warns and does nothing.
- The `errors`/`layouts` arrays in `dictionary` are 0-based **after removing index 0** (root); the root error node is always `nodes[1]`, root layout `nodes[0]`.
- Route matching happens on every `goto`/click even during `_invalidate` (with the cached reroute); a `reroute` that throws sends the user to a full page load at the *original* URL, which will hit Go — Go must not 404 that.
- `navigate` aborts (no render) if another navigation started meanwhile (`navigation_token` check, `L2065-2068`); in-flight `__data.json` responses are simply dropped — Go should not care about client cancellation.

## Recipes

- **Parity test corpus**: dump `routes.map(r => ({ id: r.id, pattern: r.pattern.source, params: r.params }))` from the built server manifest into a JSON fixture; a Go table test compiles each and asserts match/params for a curated URL list (including `%2F`, `[x+2e]`, optional+rest, matcher failure fallthrough).
- **Decision function**: `func Resolve(path string) (route *Route, params map[string]any, ok bool)` used by both the shell handler (serve shell iff a page route matches or no route matches; serve endpoint iff only a `+server` route matches) and the `__data.json` handler.
- **Manual check**: in the browser console, `import('/_app/immutable/entry/app.<hash>.js').then(m => console.log(m.dictionary, m.server_loads))` shows exactly which slots the client will mark `'1'`.
