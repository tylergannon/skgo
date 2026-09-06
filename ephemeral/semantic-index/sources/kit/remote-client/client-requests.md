# kit client runtime: what requests the browser sends for each remote-function kind

Source: `@sveltejs/kit` 3.0.0-next.25 (pinned clone), client runtime under
`reference/kit/packages/kit/src/runtime/client/remote-functions/` plus the shared
argument codec in `reference/kit/packages/kit/src/runtime/shared.js`.

## Purpose

Exact wire contract the Go server must accept: HTTP method, URL, argument encoding,
query-string / body shape, headers. Everything here is what kit's *unmodified* client
bundle does; the Go handlers are mounted at these URLs and must parse these bodies.

## Key facts

### URL prefix and id (all kinds)

- Every remote call goes to `${base}/${app_dir}/remote/${id}` where `id` is
  `'<hash>/<exportName>'` baked into the client bundle by the Vite transform
  (`reference/kit/packages/kit/src/exports/vite/index.js:L733-L737`, expression
  `` `${ns}.${type}('${remote.hash}/${name}')` ``). See `client-transform.md` for the hash.
- `base` comes from the hydration payload or the build-time constant
  (`reference/kit/packages/kit/src/runtime/app/paths/internal/client.js:L8-L10`:
  `base = payload.base ?? __SVELTEKIT_PATHS_BASE__`, `app_dir = __SVELTEKIT_APP_DIR__`).
  `__SVELTEKIT_APP_DIR__` is `kit.appDir` (default `_app`)
  (`reference/kit/packages/kit/src/exports/vite/index.js:L479`).
- The server side of kit splits the path after the prefix on `/`:
  `const [hash, name, additional_args] = id.split('/')`
  (`reference/kit/packages/kit/src/runtime/server/remote-functions.js:L166`). Only
  `prerender` uses the third segment (the payload); forms may carry a JSON key there
  in the non-enhanced path.
- Cross-site check kit applies to remote URLs in prod: any non-GET remote request whose
  `Origin` header differs from the app's own origin gets `403 {"message":"Cross-site remote requests are forbidden"}`
  (`reference/kit/packages/kit/src/runtime/server/respond.js:L99-L116`,
  `reference/kit/packages/kit/src/runtime/server/csrf.js:L63-L65`).

### Argument encoding (`payload`) — query, query.live, query.batch, prerender

- `stringify_remote_arg(value)`: `undefined` → `''` (empty string, no `?payload=`);
  otherwise `devalue.stringify(value, create_remote_arg_reducers(true))` then
  URL-friendly base64 (`reference/kit/packages/kit/src/runtime/shared.js:L252-L259`).
- URL-friendly base64 = standard base64 of the UTF-8 bytes with `=` padding **removed**,
  `+`→`-`, `/`→`_` (`reference/kit/packages/kit/src/runtime/shared.js:L311-L315`).
  Decoding reverses `-`→`+`, `_`→`/` and does not re-add padding
  (`reference/kit/packages/kit/src/runtime/shared.js:L321-L331`).
- The reducers used (in this order after the app's `transport` encoders):
  `__skrag` (throws on RegExp), `__skram` (Map), `__skras` (Set), `__skrao` (plain object)
  (`reference/kit/packages/kit/src/runtime/shared.js:L84-L173`).
  - `__skrao`: every plain object (prototype `Object.prototype` or `null`) is replaced by a
    clone with **keys sorted** so that `{b,a}` and `{a,b}` produce the same payload/cache key
    (`L66-L82`, `L149-L164`). Wire form: `["__skrao", i]` where node `i` is the sorted object.
  - `__skram`: a Map becomes an array of `[stringify(key), stringify(value)]` **nested
    devalue strings**, sorted by key string then value string (`L111-L130`).
  - `__skras`: a Set becomes a sorted array of nested devalue strings (`L133-L147`).
  - Custom `transport` encoders from the app's hooks come first (`L167`).
- Worked examples (produced by running devalue with kit's reducer logic):
  - `undefined` → payload `''`
  - `1` → `[1]` → `WzFd`
  - `"hi"` → `["hi"]` → `WyJoaSJd`
  - `{filter:'author:santa'}` → `[["__skrao",1],{"filter":2},"author:santa"]` →
    `W1siX19za3JhbyIsMV0seyJmaWx0ZXIiOjJ9LCJhdXRob3I6c2FudGEiXQ`
  - `{b:[1,2],a:{z:true,y:null}}` →
    `[["__skrao",1],{"a":2,"b":6},["__skrao",3],{"y":4,"z":5},null,true,[7,8],1,2]`
    (nested objects are wrapped too; keys sorted at every level).
  - `new Set([3,1,2])` → `[["__skras",1],[2,3,4],"[1]","[2]","[3]"]`.
- devalue flat format essentials (`reference/devalue/src/parse.js:L20-L200`): the string is
  JSON; a bare number is only valid for sentinels; otherwise an array of nodes with the root at
  index 0; object nodes are JSON objects whose values are node indices; array nodes are arrays of
  indices; `["Type", ...]` tuples are builtins (`Date`, `Map`, `Set`, `RegExp`, `Object`, `BigInt`,
  `null`, typed arrays, `ArrayBuffer`) or custom reducers `["name", index]`; sentinels
  `-1 undefined, -2 hole, -3 NaN, -4 Infinity, -5 -Infinity, -6 -0`
  (`reference/devalue/src/constants.js:L1-L6`).

### `query` (GET)

- `GET ${base}/${app_dir}/remote/${id}` plus `?payload=${payload}` only when payload is
  non-empty (`reference/kit/packages/kit/src/runtime/client/remote-functions/query/index.js:L27`).
- No custom headers, no body, `fetch(url)` with default init
  (`.../query/index.js:L29`, `.../shared.svelte.js:L113-L114`). Cookies ride along via
  the default `credentials: 'same-origin'`.
- The payload is placed raw in the query string (no `encodeURIComponent`) — safe because the
  alphabet is `[A-Za-z0-9_-]`.
- The value is delivered via `data.q`, not `data._` — see `client-expectations.md`.

### `query.batch` (POST)

- `POST ${base}/${app_dir}/remote/${id}` with header `Content-Type: application/json` and body
  `{"payloads": ["<payload>", ...]}` (one entry per distinct payload collected in the same
  macrotask) (`reference/kit/packages/kit/src/runtime/client/remote-functions/query-batch.svelte.js:L32-L49`).
- No `x-sveltekit-*` headers are sent for batch (`L32-L34`).
- Batching window: a `setTimeout(...,0)` after the first proxy in a tick; duplicates of the
  same payload are coalesced (`L22-L38`).

### `query.live` (GET, streaming)

- `GET ${base}/${app_dir}/remote/${id}?payload=...` (same URL rule as `query`) via plain
  `fetch(url, { signal })`; no custom headers
  (`reference/kit/packages/kit/src/runtime/client/remote-functions/query-live/iterator.js:L25-L29`).
- Response consumption is in `live-and-batch.md`.

### `command` (POST)

- `POST ${base}/${app_dir}/remote/${id}` with headers
  `Content-Type: application/json`, `x-sveltekit-pathname: <page pathname>`,
  `x-sveltekit-search: <page search>`
  (`reference/kit/packages/kit/src/runtime/client/remote-functions/command.svelte.js:L32-L55`,
  `.../shared.svelte.js:L95-L107`).
- Body: `{"payload": "<command payload>", "refreshes": ["<hash>/<name>/<payload>", ...]}`.
  `refreshes` is always present (empty array when `.updates()` was not used) (`L50-L53`).
- Command payload uses `stringify_command_arg`: same devalue+base64 but **without** key sorting
  (`create_remote_arg_reducers(false)`), with `File` support via reducer `__skraf`, which
  emits `{data: ArrayBuffer, lastModified, name, size, type}` (so the wire form is
  `["__skraf", i]` with node `i` an object whose `data` is `["ArrayBuffer","<base64>"]`),
  and a guard `__skrap` that rejects arbitrary Promises
  (`reference/kit/packages/kit/src/runtime/shared.js:L265-L304`).
- Refresh keys in `refreshes` are `create_remote_key(id, payload)` = `id + '/' + payload`;
  the server splits on the **last** `/` (`reference/kit/packages/kit/src/runtime/shared.js:L337-L356`).
  How they're built: a query *function* expands to every active instance in the client cache;
  a query *instance* contributes its own key; a `withOverride` release fn contributes the
  instance key and is released after the response (`.../shared.svelte.js:L214-L280`).

### `form` (POST, binary body) — enhanced submit

- `POST ${base}/${app_dir}/remote/${action_id_without_key}` — i.e. `hash/name`, **never**
  including the `form.for(key)` key in the URL; the key travels inside the data as `id`
  (`reference/kit/packages/kit/src/runtime/client/remote-functions/form.svelte.js:L195-L201`, `L246-L258`).
- Headers: `Content-Type: application/x-sveltekit-formdata`, `x-sveltekit-pathname`,
  `x-sveltekit-search` (from `location`) (`L249-L255`;
  `BINARY_FORM_CONTENT_TYPE` at `reference/kit/packages/kit/src/runtime/form-utils.js:L105`).
- Body = `serialize_binary_form(data, meta)` (`form-utils.js:L108-L171`):
  1. byte 0: format version `0`
  2. u32 LE: byte length of the header string
  3. u16 LE: byte length of the file-offset-table string
  4. header: `devalue.stringify([data, meta], { File: f => [name, type, size, lastModified, index] })`
  5. offset table: `JSON.stringify([offset0, offset1, ...])` (empty if no files); offsets are
     relative to the end of the table; files are **sorted smallest-first** before assigning offsets
  6. raw file bytes concatenated.
  `meta` is `{ remote_refreshes: string[] }` for submit and `{ validate_only: true }` for
  programmatic `validate()` (`form.svelte.js:L242-L244`, `L738-L740`;
  `BinaryFormMeta` at `reference/kit/packages/kit/src/types/internal.d.ts:L560-L563`).
- `data` is the POJO produced by `convert_formdata(form_id, FormData)`: field names carry the
  form id as a suffix and optional type prefixes:
  `[n:|b:]<path>[[]]/<hash>/<name>` (`form-utils.js:L26-L51`, `L73-L103`); `n:` → parseFloat
  (empty → undefined), `b:` → `value === 'on'`; `[]` suffix → array; nested paths `a.b[0].c`
  become nested objects via `deep_set`. Prefixes are chosen by `fields.x.as(type)`
  (`form-utils.js:L613-L622`, `L763-L768`).
- A keyed instance adds `data.id = key` if absent (`form.svelte.js:L197-L200`).
- Non-enhanced fallback (`<form method="POST" action="?/remote=<encodeURIComponent(hash/name[/JSON(key)])>">`)
  posts native `multipart/form-data` **to the page URL**, not to `/_app/remote/...`
  (`form.svelte.js:L76-L101`; server routing at
  `reference/kit/packages/kit/src/runtime/server/remote-functions.js:L609-L611` and
  `page/index.js:L62-L64`). This path requires SSR of the page afterwards.

### `prerender` (GET, path-segment payload)

- `GET ${base}/${app_dir}/remote/${id}` plus `/${payload}` as a **path segment** (not a query
  string) when payload is non-empty
  (`reference/kit/packages/kit/src/runtime/client/remote-functions/prerender.svelte.js:L69`).
- Headers: `x-sveltekit-pathname`, `x-sveltekit-search` (`L83`, `L99`).
- Before fetching, the client consults `caches.open('sveltekit:<version>')` (Cache API) keyed by
  the URL; a hit short-circuits the network (`L11-L32`, `L86-L97`); successful responses are
  written back as `devalue.stringify(data, app.encoders)` (`L109-L112`).

## Citations

- `reference/kit/packages/kit/src/runtime/shared.js:L84-L173`, `L252-L331`, `L337-L356`
- `reference/kit/packages/kit/src/runtime/client/remote-functions/shared.svelte.js:L95-L107`, `L113-L114`, `L214-L280`
- `reference/kit/packages/kit/src/runtime/client/remote-functions/query/index.js:L25-L35`
- `reference/kit/packages/kit/src/runtime/client/remote-functions/query-batch.svelte.js:L17-L49`
- `reference/kit/packages/kit/src/runtime/client/remote-functions/query-live/iterator.js:L25-L29`
- `reference/kit/packages/kit/src/runtime/client/remote-functions/command.svelte.js:L32-L55`
- `reference/kit/packages/kit/src/runtime/client/remote-functions/form.svelte.js:L76-L101`, `L195-L201`, `L242-L258`, `L728-L742`
- `reference/kit/packages/kit/src/runtime/client/remote-functions/prerender.svelte.js:L58-L115`
- `reference/kit/packages/kit/src/runtime/form-utils.js:L26-L51`, `L73-L171`, `L613-L622`
- `reference/kit/packages/kit/src/runtime/app/paths/internal/client.js:L8-L10`
- `reference/kit/packages/kit/src/runtime/server/remote-functions.js:L166`, `L191-L310`, `L588-L611`
- `reference/kit/packages/kit/src/runtime/server/csrf.js:L63-L65`
- `reference/devalue/src/parse.js:L20-L200`, `reference/devalue/src/constants.js:L1-L6`

## Go implementation notes

- Mount one `http.Handler` per exported function at `/<base>/<appDir>/remote/<hash>/<name>`
  and dispatch on method: `query`/`query.live`/`prerender` = GET, `command`/`query.batch`/`form` = POST
  (kit returns 405 for wrong methods on live/batch/form; `remote-functions.js:L193-L244`).
  `prerender` handlers must also match `/<hash>/<name>/<payload>`.
- Implement `parse_remote_arg` in Go: base64url-without-padding decode (accept both padded and
  unpadded; JS `atob` tolerates missing padding), then a devalue **parser** that understands
  the flat array format, the sentinels, the builtins, and the four custom tags `__skrao`
  (identity on the referenced object), `__skram` (array of `[k,v]` **nested devalue strings**),
  `__skras` (array of nested devalue strings), `__skraf` (File object with `data` as
  `["ArrayBuffer","<base64>"]`). Also accept the app's `transport` encoder names if skgo lets
  apps declare them.
- Cache-key identity: because the client sorts keys, Go must treat the *raw payload string* as the
  cache key for `refreshes`/`q` responses — never re-serialize the parsed argument to build the
  key; echo the string the client sent (kit does exactly this, `remote-functions.js:L201-L205`).
- For `form`: implement the binary container reader (version byte, u32 LE, u16 LE, header string,
  JSON offset table, concatenated files) and the `convert_formdata` rules for the native
  `multipart/form-data` fallback. Validate offset tables (no gaps/overlaps/duplicates) as kit does
  (`form-utils.js:L253-L327`).
- Reject non-GET remote requests whose `Origin` != own origin with 403 JSON `{message}` to match
  kit's `is_remote_forbidden`.
- `x-sveltekit-pathname`/`x-sveltekit-search` are informational (kit uses them to reconstruct
  `event.url` for the page the call came from). Only command/form/prerender send them; plain
  query/live/batch do not — do not require them.

## Gotchas

- `?payload=` is absent when the argument is `undefined`; `""` and `undefined` are different
  payloads (`""` → `WyIiXQ`). Kit's validator rejects any non-undefined argument for functions
  declared without a schema (`400 Bad Request`) (`runtime/app/server/remote/shared.js:L12-L20`).
- Map/Set entries are *nested devalue documents inside strings*; a naive parser will see plain
  strings and produce wrong types.
- Command payloads are **not** key-sorted; query payloads are. Don't assume sorted keys when
  parsing commands, and don't sort when echoing keys.
- `refreshes` keys split on the **last** slash; payloads never contain `/` (base64url) but ids do
  (`hash/name`).
- Forms with `for(key)`: the URL is still `hash/name`; the key is `data.id`. Only the non-enhanced
  (`?/remote=`) path puts the JSON key after the id (`remote-functions.js:L541-L557`).
- The file-offset table's u16 length caps it at 65535 bytes of JSON; files are re-ordered
  smallest-first, so file `index` in the header is not the on-wire order.

## Recipes

- Go djb2 + URL construction: see `client-transform.md` (hash) and use
  `path.Join(base, appDir, "remote", hash, name)`.
- Minimal Go decoder skeleton for payloads:
  1. `s = strings.NewReplacer("-", "+", "_", "/").Replace(payload)`; pad to a multiple of 4; `base64.StdEncoding.DecodeString`.
  2. `json.Unmarshal` into `[]json.RawMessage` (or a bare number for `-1`).
  3. Recursively hydrate node 0, memoizing by index; on `["__skrao", i]` hydrate `i`; on
     `["__skram", i]` hydrate `i` to `[][2]string` and recursively decode each string with the same
     decoder; same for `__skras`.
- To reproduce a client payload in a Go test, use the worked examples above as golden values.
