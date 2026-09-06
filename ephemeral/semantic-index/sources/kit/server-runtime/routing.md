# Route manifest and matching (kit 3.0.0-next.25 server runtime)

## Purpose

How kit's server turns a decoded pathname into `{ route, params }`, what the
built server manifest (`manifest._.routes`) looks like, how route ids become
regexes (params, matchers, rest/optional, groups, escape sequences), route
ordering, and trailing-slash normalization. Go must reproduce this matcher —
either by consuming the built manifest or by scanning `src/routes` and
regenerating the same patterns and order — so `__data.json`, form actions and
`+server` endpoints resolve to the same route id/params as kit's client.

## Key facts

### Manifest shape

- Server manifest (`manifest._`): `client`, `nodes: [loader...]`, `remotes`,
  `routes: [{ id, pattern: RegExp, params: RouteParam[], page: { layouts: [..], errors: [..], leaf } | null, endpoint: loader | null }]`,
  `prerendered_routes: Set<string>`, `matchers: async () => params`,
  `server_assets`. Routes with neither page nor endpoint are omitted. Node
  indexes are re-indexed to only used nodes (root layout 0 and root error 1
  always included). Holes in `layouts`/`errors` are emitted as empty array
  slots (`[0,,2,]`).
  `reference/kit/packages/kit/src/core/generate_manifest/index.js:L43-L62, L76-L84, L106-L149`
- `SSRRoute { id; pattern; params: RouteParam[]; page: PageNodeIndexes | null; endpoint; endpoint_id? }`,
  `PageNodeIndexes { errors: Array<number|undefined>; layouts: Array<number|undefined>; leaf: number }`,
  `RouteParam { name; matcher; optional; rest; chained }`.
  `reference/kit/packages/kit/src/types/internal.d.ts:L265-L271, L510-L514, L527-L534`
- Client manifest `dictionary` (what the CSR bundle uses for client-side
  routing): `{ "<route id>": [leaf, [layouts...], [errors...]] }`, where a
  negative/`~`-prefixed leaf means the leaf has a server load
  (`uses_server_data = id < 0; id = ~id`), root layout/error omitted from the
  arrays, trailing empties trimmed; `server_loads` lists layout node indexes
  that have server loads. `reference/kit/packages/kit/src/core/sync/write_client_manifest.js:L57-L120`,
  `reference/kit/packages/kit/src/runtime/client/parse.js:L7-L58`
- `has_server_load` for a node is true when it has `load` OR `trailingSlash`
  in its server module ("we need to do a server request in that case to get
  its value"). `reference/kit/packages/kit/src/types/internal.d.ts:L425-L430`,
  `reference/kit/packages/kit/src/utils/routing.js:L344-L346`

### `find_route`

- Iterates routes in manifest order; first regex match whose `exec()` returns
  params wins. `reference/kit/packages/kit/src/utils/routing.js:L356-L370`
- Input is the decoded, base-stripped pathname (`decode_pathname` = `decodeURI`
  applied to every segment split on `%25`, so `%25` survives). Decode failure →
  400 `Malformed URI`. `reference/kit/packages/kit/src/utils/url.js:L81-L83`,
  `reference/kit/packages/kit/src/runtime/server/respond.js:L297-L302, L599-L611`
- Base handling: if `base` set and path does not start with it → 404 `Not found`;
  else `resolved_path = resolved_path.slice(base.length) || '/'`.
  `reference/kit/packages/kit/src/runtime/server/respond.js:L341-L346`
- Requests under `/${app_dir}` (default `_app`) that are not `env.js` or
  route-resolution requests → 404 with `cache-control: public, max-age=0, must-revalidate`.
  `reference/kit/packages/kit/src/runtime/server/respond.js:L356-L365`

### `parse_route_id` → regex

`reference/kit/packages/kit/src/utils/routing.js:L35-L117`

- `id === '/'` or a root group like `/(app)` → `/^\/$/`.
- Segments come from `get_route_segments(id)` = `id.slice(1).split('/')`
  filtered by `affects_path` (drops empty and `(group)` segments). `L123-L136`
- Whole-segment specials:
  - `[...rest]` (optionally `=matcher`) → `(?:/([^]*))?` and param
    `{ rest: true, optional: false, chained: true }`.
  - `[[optional]]` → `(?:/([^/]+))?` and `{ optional: true, chained: true }`.
- Otherwise the segment is split on `\[(.+?)\](?!\])` into static/param parts:
  - `[x+NN]` / `[u+NNNN]` escape sequences decode to the literal char, then
    escaped (see below).
  - param content matched by `/^(\[)?(\.\.\.)?([\w-]+)(?:=([\w-]+))?(\])?$/` →
    `rest` → `([^]*?)`, `optional` → `([^/]*)?`, plain → `([^/]+?)`;
    `chained` for an inline rest is `i === 1 && parts[0] === ''` (rest at the
    very start of the segment).
  - static text → `escape()`: NFC-normalize, then `%`→`%25`, `/`→`%2[Ff]`,
    `?`→`%3[Ff]`, `#`→`%23` (regex source), other chars regex-escaped.
    `L252-L267`
  - The segment contributes `'/' + result`.
- Final pattern: `^` + segments joined + `/?$` — a trailing slash is always
  tolerated by the regex; normalization happens separately.

### `exec` — params extraction and matchers

`reference/kit/packages/kit/src/utils/routing.js:L173-L246`

- Captured values are `decodeURIComponent`-ed individually.
- Missing optional → param omitted from result; missing rest → `''` (empty
  string, and the matcher still runs).
- Matcher: `matchers[name]['~standard'].validate(value)` (Standard Schema);
  async → error; `issues` → no match; a matcher may transform the value but the
  result must be string/number/boolean/bigint. `L143-L166`
- Chained optional/rest buffering: for `[[a=m]]/[...rest]`, values failing the
  matcher for chained optionals are "buffered" and rolled into the rest param.
  If anything is left buffered at the end → route does not match. `L180-L245`
- Result params may hold non-strings when a matcher transforms; the
  `event.params` values that flow into `load` are whatever `exec` produced.

### Ordering (build-time `sort_routes`)

`reference/kit/packages/kit/src/core/sync/create_manifest_data/sort.js:L18-L162`

- Compare segment-by-segment after stripping non-terminal `[[optional]]`
  parts; each segment split into alternating static/dynamic parts.
- Rules: shallower path beats deeper; static parts compared with `sort_static`
  (lexicographic except `foobar` outranks `foo`); dynamic parts: missing part
  wins; `[...rest]/x` outranks `[...rest]`; `[...rest]/x` outranks
  `[required]` but not `[required]/x`; a part with a matcher outranks one
  without; `[required]` outranks `[[optional]]`; final tiebreak is descending
  id string (`route_a.id < route_b.id ? +1 : -1`).
- Invalid ids rejected at build: `[[optional]]` after `[...rest]`,
  `[[...rest]]`. `reference/kit/packages/kit/src/core/sync/create_manifest_data/index.js:L181-L187`

### Trailing slash

- `normalize_path(path, 'never'|'always'|'ignore')`: `/` and `'ignore'` are
  untouched; `never` strips a trailing `/`; `always` adds one.
  `reference/kit/packages/kit/src/utils/url.js:L65-L75`
- Per request `trailing_slash` = `'always'` if `url.pathname === base || base + '/'`;
  else `PageNodes.trailing_slash()` = last non-undefined `trailingSlash` among
  `universal ?? server` options walking root → leaf, default `'never'`; for
  endpoint-only routes `node.trailingSlash ?? 'never'`.
  `reference/kit/packages/kit/src/runtime/server/respond.js:L389-L406`,
  `reference/kit/packages/kit/src/utils/page_nodes.js:L48-L54, L68-L70`
- Non-data requests whose pathname differs from the normalized one get a
  `308` with `location: relative_pathname(from, to) + search` and header
  `x-sveltekit-normalize: 1` (relative so proxies' prefixes survive;
  `?` alone dropped). `reference/kit/packages/kit/src/runtime/server/respond.js:L408-L422`,
  `reference/kit/packages/kit/src/utils/url.js:L36-L40`
- Data requests never 308; the load's `event.url` is normalized instead.
  `reference/kit/packages/kit/src/runtime/server/data/index.js:L40-L43`

### Server-side route resolution (`router.resolution = 'server'`)

- Request suffixes `/__route.js` / `.html__route.js`; and the route-id form
  `/_app/routes/<id>/__route.js`. Response is a JS module `export const route = {...}`
  (+ `export const params = {...}` for pathname form), or
  `export const endpoint_only = true;`, or empty module for unknown, with
  `content-type: application/javascript; charset=utf-8`. Only relevant if the
  app opts into server resolution; default is client resolution.
  `reference/kit/packages/kit/src/pathname.js:L24-L55`, `reference/kit/packages/kit/src/runtime/pathname.js:L14-L37`,
  `reference/kit/packages/kit/src/runtime/server/page/server_routing.js:L68-L157`

### Which handler a matched route gets (page vs endpoint)

- `is_endpoint_request`: methods in `ENDPOINT_METHODS` but not in
  `PAGE_METHODS` (`PUT PATCH DELETE OPTIONS QUERY`) → endpoint; POST with
  `x-sveltekit-action: true` → page; otherwise endpoint iff
  `negotiate(accept ?? '*/*', ['*', 'text/html']) !== 'text/html'`.
  `reference/kit/packages/kit/src/runtime/server/endpoint.js:L99-L113`,
  `reference/kit/packages/kit/src/constants.js:L9-L25`
- If the route has both and the endpoint cannot handle GET/HEAD/POST
  (no matching export/`fallback`), the page renders instead.
  `reference/kit/packages/kit/src/runtime/server/respond.js:L655-L672`
- Page routes with a non-page method: OPTIONS → 204 with `allow: GET, HEAD, OPTIONS[, POST]`
  (POST only if leaf has actions); others → 405 `<METHOD> method not allowed`
  with `allow`. `L688-L711`, `reference/kit/packages/kit/src/runtime/server/utils.js:L8-L30`
- Routes with both page and endpoint get `Vary: Accept` on GET/HEAD. `L718-L739`

## Citations

- `reference/kit/packages/kit/src/utils/routing.js:L5-L9, L35-L136, L143-L246, L252-L267, L344-L370`
- `reference/kit/packages/kit/src/utils/url.js:L36-L40, L65-L83`
- `reference/kit/packages/kit/src/utils/page_nodes.js:L48-L94`
- `reference/kit/packages/kit/src/runtime/server/respond.js:L297-L302, L341-L382, L389-L422, L599-L611, L655-L739`
- `reference/kit/packages/kit/src/runtime/server/endpoint.js:L99-L113`
- `reference/kit/packages/kit/src/runtime/server/page/server_routing.js:L68-L157`
- `reference/kit/packages/kit/src/core/generate_manifest/index.js:L43-L149`
- `reference/kit/packages/kit/src/core/sync/create_manifest_data/sort.js:L18-L162`
- `reference/kit/packages/kit/src/core/sync/write_client_manifest.js:L57-L120`
- `reference/kit/packages/kit/src/runtime/client/parse.js:L7-L58`
- `reference/kit/packages/kit/src/types/internal.d.ts:L265-L271, L425-L430, L510-L543`
- `reference/kit/packages/kit/src/constants.js:L9-L25`

## Go implementation notes

- Two viable sources of truth: (a) parse the built `server/manifest.js`
  (`routes[].id`, `pattern` regex source, `params`, `page` indexes) — the
  regex is JS syntax but only uses `[^/]`, `[^]`, `?`, `*?`, `(?:...)`, so
  translate `[^]` → `[\s\S]` for Go `regexp`; (b) re-implement
  `parse_route_id` + `sort_routes` from a route-directory scan. (b) is what a
  Go generator needs anyway to emit handlers per route id; use (a) in tests to
  assert equality of pattern source and order against kit's output.
- Matchers: kit 3 matchers are Standard Schema objects loaded from
  `src/params.ts` (`defineParams`), not per-file `match()` functions. Go must
  own matcher implementations (Go code registered by name) since JS matchers
  cannot run in the Go server; the generator should fail on unknown matcher
  names exactly like `validate_param_matchers` does.
  `reference/kit/packages/kit/src/utils/params.js:L27-L33`
- Reproduce `exec` precisely, including chained-optional buffering and the
  empty-string rest value fed to matchers.
- Route ids are what appear in `event.route.id`, `uses.route` decisions, and
  the client's dictionary; keep group segments in the id (`/(app)/blog/[slug]`)
  while excluding them from the pattern.
- `decode_pathname` must be done before matching; keep `%25` literal; encode
  `[x+2f]`-style literal chars into their `%XX` forms when comparing.
- 308 normalization with `x-sveltekit-normalize: 1` for non-data page requests
  (skip for data requests, apply `normalize_path` to `event.url` instead).
- Ignore: server-side route resolution (`__route.js`) unless skgo opts into
  `router.resolution = 'server'`; `state.prerendering` branches;
  `reroute` hook is Node-only (a Go server has no JS hooks — if `reroute` is
  needed it must be re-expressed in Go).

## Gotchas

- The pattern always ends in `/?$`; trailing slash acceptance is not the same
  as trailing slash policy — a mismatch yields a 308, not a 404.
- `[[optional]]` as a whole segment uses `(?:/([^/]+))?` (no empty value), but an
  inline optional (`foo-[[bar]]`) uses `([^/]*)?` — different semantics.
- The `chained` flag only matters for buffering with matchers; a plain
  `[[a]]/[...rest]` still works because unmatched optionals yield `undefined`
  and the rest consumes the remainder.
- `+server`-only routes exist in `_.routes` with `page: null`; a
  `/__data.json` against them returns 404.
- Root group ids like `/(marketing)` match only `/`; they are distinct routes
  from `/` and cannot coexist with it (build error elsewhere).
- Sort tiebreak is *descending* id; do not sort ascending.

## Recipes

Regex translations (JS → Go):

| id | JS pattern | params |
|---|---|---|
| `/` | `^/$` | `[]` |
| `/blog/[slug]` | `^/blog/([^/]+?)/?$` | `[{slug}]` |
| `/docs/[...path]` | `^/docs(?:/([^]*))?/?$` → Go `^/docs(?:/([\s\S]*))?/?$` | `[{path rest chained}]` |
| `/[[lang]]/about` | `^(?:/([^/]+))?/about/?$` | `[{lang optional chained}]` |
| `/files/[name].[ext=type]` | `^/files/([^/]+?)\.([^/]+?)/?$` | `[{name},{ext matcher:type}]` |
| `/(app)/dash` | `^/dash/?$` | `[]` |

Go match loop:

```go
for _, r := range routes /* kit order */ {
    m := r.re.FindStringSubmatch(decodedPath)
    if m == nil { continue }
    if params, ok := exec(m[1:], r.params, matchers); ok { return r, params }
}
```
