# Research: `__data.json` in a CSR-only kit app (pinned kit 3.0.0-next.25)

Read by an Opus researcher on 2026-09-06 against the pinned source. `$K` =
`ephemeral/inspiration/reference/kit/packages/kit`. Re-verify any line you build on.

## 1. Cold SPA boot fetches `__data.json` — yes

With `ssr: false` the shell has no hydrate payload: `$K/src/runtime/server/page/render.js:481`
gates `{node_ids, data, form, error}` behind `if (page_config.ssr)`, so the boot script is
`app.start(element)` (`render.js:533-542`). `_start` takes the no-data branch
(`$K/src/runtime/client/client.js:589-600`) → `navigate({type:'enter'})` → `load_route`
(`client.js:2033`). `loaders = [...layouts, leaf]` (`:1421`); on cold boot `current.branch` is
empty so every server-load node is invalid (`:1441-1464`) and `load_data` runs (`:1468`).

URL construction (`client.js:3653-3666`):
```
const data_url = new URL(url);                       // query string carried over verbatim
data_url.pathname = add_data_suffix(url.pathname);
if (url.pathname.endsWith('/')) data_url.searchParams.append(TRAILING_SLASH_PARAM, '1');
data_url.searchParams.append(INVALIDATED_PARAM, invalid.map(i => i ? '1' : '0').join(''));
const res = await fetcher(data_url.href, {});        // no custom request headers
```
- `add_data_suffix` (`$K/src/pathname.js:10-13`): strips one trailing `/`, appends
  `/__data.json`; `foo.html` → `foo.html__data.json`.
- `INVALIDATED_PARAM = 'x-sveltekit-invalidated'`, `TRAILING_SLASH_PARAM =
  'x-sveltekit-trailing-slash'` (`$K/src/runtime/shared.js:18,20`). Query params, not headers.
- Only header involved is the response header `x-sveltekit-version` (`client.js:3669`; set
  at `$K/src/runtime/server/utils.js:47-52` only when version checks are enabled).
- Server undoes it at `$K/src/runtime/server/respond.js:156-165`.

## 2. How the client knows which nodes have a server load

Generated client manifest (`$K/src/core/sync/write_client_manifest.js:133-175`) exports
`nodes`, `server_loads`, `dictionary`, `hooks`, `decoders/encoders`, `hash`.
- Leaf: ones'-complement on the leaf index — `~leaf` means "leaf has a server load".
- Layouts: `export const server_loads = [...]` of layout node indices.
- Decoded in `$K/src/runtime/client/parse.js:41-57` into `[has_server_load, loader]`.

Source of truth is build analysis: `metadata[i].has_server_load`
(`write_client_manifest.js:75,94`) from `$K/src/core/postbuild/analyse.js:85` via
`$K/src/utils/routing.js:344-346`:
```
export function has_server_load(node) {
  return node.server?.load !== undefined || node.server?.trailingSlash !== undefined;
}
```
A `+page.server.ts` exporting only a throwing `load` counts. One exporting only
`actions`/`prerender` does not. Dev falls back to file existence
(`write_client_manifest.js:76,95`).

Kill switch: `__SVELTEKIT_HAS_SERVER_LOAD__` (`client.js:1440`, `:1703`) is `false` at build
if no node has a server load (`$K/src/exports/vite/index.js:1251-1258`); always true in dev
(`vite/index.js:516`).

## 3. Response body, node alignment, `uses`

Producer `$K/src/runtime/server/data/index.js`; serializer
`$K/src/runtime/server/page/data_serializer.js:130-217`.

Top-level (`data_serializer.js:212`): `{"type":"data","nodes":[ ... ]}\n`

Per-node (`data_serializer.js:189-202`): `null` (no node / no server load); `{"type":"skip"}`;
`{"type":"error","error":<App.Error>}`; or
`{"type":"data","data":<devalue flat array>,"uses":{...}[,"slash":"always"|"never"|"ignore"]}`.

Redirect replaces the whole response: `{"type":"redirect","status","location"}`
(`data/index.js:161-168`), HTTP 200, application/json.

Headers/status (`data/index.js:146-155`): 200, `content-type: application/json`,
`cache-control: private, no-store`, plus `x-sveltekit-version`. Top-level error → `App.Error`
JSON at `status = transformed.status` (`data/index.js:137`). 404 with empty body if the route
has no page (`data/index.js:29-32`). If any `load` returned a promise the response becomes
`content-type: text/sveltekit-data`, a stream whose later lines are
`{"type":"chunk","id":N,"data":…}` (`data/index.js:114-129`, `data_serializer.js:174`) — out of
scope for us.

Parser: the client always reads NDJSON — `client.js:3686-3749` via `read_ndjson`
(`$K/src/runtime/client/ndjson.js:8-15`). `!res.ok` short-circuits first (`client.js:3671-3684`).

Node alignment: server iterates `node_ids = [...route.page.layouts, route.page.leaf]`
(`data/index.js:35`); client indexes `server_data_nodes?.[i]` against
`loaders = [...route.layouts, route.leaf]` (`client.js:1421,1499`) where
`route.layouts = [0, ...dictionary_layouts]` (`parse.js:22`). Empty layout slots serialize as
`null` (`$K/src/runtime/server/page/load_data.js:21`).

`uses` — serialized sparsely at `$K/src/runtime/server/utils.js:77-97` (keys omitted when
empty; `parent`/`route`/`url` emitted as `1`). Rehydrated at `client.js:3755-3764`. Gating at
`client.js:1332-1361`:
```
if (force_invalidation) return true;      // invalidateAll
if (!uses) return false;
if (uses.parent && parent_changed) return true;
if (uses.route && route_changed) return true;
if (uses.url && url_changed) return true;
for (const p of uses.search_params) if (search_params_changed.has(p)) return true;
for (const p of uses.params) if (params[p] !== current.params[p]) return true;
for (const href of uses.dependencies) if (invalidated.some(fn => fn(new URL(href)))) return true;
```
A node whose invalidated bit is `0` is answered `{"type":"skip"}` (`data/index.js:84-88`) and
the client reuses the previous node (`client.js:1368-1372`).

## 4. devalue format

Flat array (`devalue.stringify`) with the same reducers as remote functions — not `uneval`.
Server: `devalue.stringify(node.data, reducers)` with `reducers = { ...encoders, Promise }`
(`data_serializer.js:135-138`, `:200`). Client: `devalue.unflatten(data, {...app.decoders,
Promise})` (`client.js:3711-3718`). Remote functions use the same transport pair
(`$K/src/runtime/app/internal/transport.js:51-52`). One Go devalue codec serves both.

## 5. Adapter surface

`builder.routes` gives `RouteDefinition` (`$K/src/exports/public.d.ts:644-657`): `{ id, api,
page, pattern: RegExp, prerender, segments, methods, config }` — no node indices, no
server-load flags.

The node list is in the generated server manifest (`builder.generateManifest`,
`builder.js:187-193` → `$K/src/core/generate_manifest/index.js`):
```
_.nodes: [__memo(() => import('./nodes/<i>.js')), ...]      // index.js:114-116
_.routes: [{ id, pattern: <JS RegExp literal>, params,
             page: { layouts: [..,], errors: [..,], leaf: N }, endpoint }]   // index.js:120-134
```
Node indices in `_` are reindexed (prerendered-only nodes dropped, `index.js:37-62`,
`:77-84`). adapter-node writes this manifest next to the server output and imports it; a
node module exports `server` when a `+page.server`/`+layout.server` file exists.

Per-route `[has_server_load, node_index]` pairs reach the manifest as `_.client` only when
`kit.router.resolution === 'server'` (`vite/index.js:1394`, `:1411-1426`). Otherwise derive
from node modules, or read `${outDir}/generated/build/client-optimized/app.js`.

Route pattern: `parse_route_id` at `$K/src/utils/routing.js:35-117` — `[...rest]` →
`(?:/([^]*))?`, `[[opt]]` → `(?:/([^/]+))?`, inline rest `([^]*?)`, inline opt `([^/]*)?`,
`[param]` → `([^/]+?)`, groups dropped (`routing.js:123-136`), anchored `^...` + `/?$`
(`routing.js:113`). Root routes are `/^\/$/` (`routing.js:40-41`). Param extraction: `exec`
at `routing.js:173`.

## 6. Dev server answers `__data.json` itself

`$K/src/exports/vite/dev/index.js:640` runs `server.respond(request, …)` for every request;
`internal_respond` branches on `has_data_suffix` (`respond.js:98,156`). Go must intercept
before proxying to `vite dev`.

## Traps

1. Body is NDJSON: trailing `\n`, never pretty-print (`data_serializer.js:212`).
2. Content type differs by content (promises → `text/sveltekit-data`). Don't infer from it.
3. Redirects are HTTP 200 + `{"type":"redirect"}`; a real 3xx breaks the client.
4. Errors have two shapes: per-node inside a 200, or top-level `App.Error` at the status —
   the client reads the latter only if `content-type` includes `application/json`
   (`client.js:3677`).
5. `__SVELTEKIT_HAS_SERVER_LOAD__` is compiled in: the client only asks for nodes kit was told
   have server loads. The stub must exist at build.
6. `+page.server.ts` exporting only `trailingSlash` counts as a server load.
7. Two node numbering spaces (client `dictionary` vs reindexed server `_`). The invalidated
   bitstring is positional over `[...layouts, leaf]`, not over either.
8. The invalidated string can be longer than the node list (`parse.js:29-32`); index
   defensively.
9. `add_data_suffix` strips exactly one trailing slash; stripping `/__data.json` from the root
   yields `''` → `'/'` (`respond.js:159`).
10. `event.url` is trailing-slash-normalized before loads run (`data/index.js:40-42`).
11. `uses` is sparse; `parent`/`route`/`url` are the number `1`.
12. `{"type":"skip"}` is mandatory for bit `0`; `null` makes the client discard cached data.
13. Server loads are memoized and parent-chained; one throwing node aborts later ones, which
    come back as `skip` (`data/index.js:45-81`).
14. `x-sveltekit-version` mismatch triggers a full reload (`client.js:3669`). Echo kit's
    value or omit.
15. Dev skips CSRF/origin checks (`respond.js:101`); prod does not.
16. **Kit's `[^]` is JavaScript-only regex syntax.** Go's RE2 rejects it. Rewrite to
    `[\s\S]` (or `(?s:.)`) when consuming the emitted pattern source.
