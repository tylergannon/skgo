# kit client runtime: what the browser expects back from a remote-function endpoint

Source: `@sveltejs/kit` 3.0.0-next.25 (pinned clone). Central parser is `remote_request` in
`reference/kit/packages/kit/src/runtime/client/remote-functions/shared.svelte.js`; the shape
types are in `reference/kit/packages/kit/src/types/internal.d.ts`.

## Purpose

The response contract the Go server must satisfy for `query`, `query.batch`, `command`,
`form`, `prerender` (non-streaming kinds). Covers status handling, the JSON envelope, the
devalue-encoded `data` string and its `_ / q / l / p / f / r / redirect` fields, how
single-flight refresh data is applied to the client cache, and how errors reach app code.

## Key facts

### Envelope (`RemoteFunctionResponse`)

- Type (`reference/kit/packages/kit/src/types/internal.d.ts:L349-L359`):
  `{ type: 'result', data: string }` | `{ type: 'error', error: App.Error }` |
  `{ type: 'redirect', status, location, data: string }`.
  `App.Error` is at minimum `{ status: number, message: string }`
  (`reference/kit/packages/kit/src/types/ambient.d.ts:L27-L30`); `handleError` may add fields
  (`reference/kit/packages/kit/src/runtime/server/errors.js:L72-L83`, `merge = {...fallback, ...body}`).
- `data` is a **devalue string** (`devalue.stringify` with the app's transport encoders) of
  `RemoteFunctionData` (`internal.d.ts:L332-L347`):
  `_` (the function's own result), `p` (prerender nodes), `q` (query / batch nodes),
  `l` (live-query nodes), `f` (form outputs, SSR-only), `r?: true` (server did explicit
  refreshes), `redirect?: string`. Node = `{ v?: any }` or `{ e?: App.Error }` (`L325-L330`),
  keyed by remote key `'<hash>/<name>/<payload>'`.
- Kit's server emits `type:'result'` + `data` for success **and for redirects**
  (`redirect` field inside `data`, still HTTP 200); it emits `type:'error'` for thrown errors
  (`reference/kit/packages/kit/src/runtime/server/remote-functions.js:L313-L350`). The
  top-level `type:'redirect'` variant is only produced in SSE side-channel messages
  (`live-and-batch.md`).
- Kit sets `cache-control: private, no-store` on every non-prerendering remote response
  (`remote-functions.js:L185`) and `x-sveltekit-version: <kit.version.name>` unless the bundle
  strategy is `inline` (`runtime/server/utils.js:L47-L52`; define at `exports/vite/index.js:L480-L481`).
- Error responses at runtime are sent with **HTTP 200** (`status: state.prerendering ? transformed.status : undefined`,
  `remote-functions.js:L345`); the status the app sees is `error.status` inside the body.
  (The client also copes with non-200 — see below.)

### `remote_request` parsing algorithm (`shared.svelte.js:L113-L186`)

1. `fetch(url, init)`; read `x-sveltekit-version` → `notify_version` marks the app as updated if
   it differs from the built version (`L118`; `runtime/app/state/client.svelte.js:L196-L200`).
2. If `!response.ok`: try `response.json()`; if that has `type:'error'` throw
   `HandledHttpError({ status: response.status, ...result.error })` (the body's `status`, if
   present, overrides the HTTP status because it is spread last); otherwise throw
   `HttpError({ status, message: response.statusText })` (`L120-L126`;
   verified by `shared.transport.spec.js:L47-L82`).
3. Else parse JSON; if `type === 'error'` throw `HandledHttpError(result.error)` (`L130-L132`).
4. `data = result.data ? devalue.parse(result.data, app.decoders) : {}` (`L134-L136`).
   `data: null` / missing is accepted and yields `{}` (`shared.transport.spec.js:L84-L96`).
5. Apply `data.q`: for each key, split into `(id, payload)`; if a `Query` instance exists in
   `query_map`, `entry.resource.set(v)` or `.fail(HandledHttpError(e))`; if none exists and the node
   is a value (not an error), stash it in `query_responses[key]` so the next instance created with
   that key initializes from it (`L143-L166`; consumed at `query/instance.svelte.js:L70-L79`).
6. Apply `data.l`: same set/fail on `live_query_map` entries, then `resource.reconnect()` **only on
   the success path** (`L169-L183`).
7. Return `data` to the caller.

### Per-kind use of the returned `data`

- `query` (GET): the caller **ignores `data._`**; the value arrives only through `data.q` for
  the query's own key (`query/index.js:L26-L35` returns nothing; `Query.#run` then sees its
  pending resolver already cleared by `set()` and bails, `query/instance.svelte.js:L114-L129`,
  `L232-L243`). On kit's server this happens because the query registers itself in the
  implicit lookup (`runtime/app/server/remote/shared.js:L61-L74`) and `collect_remote_data`
  serializes it (`remote-functions.js:L433-L485`). **A GET query response must therefore contain
  `q: { "<hash>/<name>/<payload>": { v: <value> } }`** (plus `_`, which is harmless).
  If `data.redirect` is set the client navigates via `_goto(redirect)` (`query/index.js:L31-L34`).
- `query.batch` (POST): result is `data._`, an **array aligned with the request's `payloads`
  order**, each element `{ type: 'result', data: <value> }` or `{ type: 'error', error: App.Error }`
  (`query-batch.svelte.js:L66-L80`; server shape at `runtime/app/server/remote/query.js:L343-L357`).
  A `data.redirect` navigates and resolves every batched promise with `undefined` (`L51-L64`).
  Any thrown error rejects all batched promises (`L81-L87`). Kit's server *also* emits `q` for
  batch queries (they register implicitly), but the client uses `_` for batch.
- `command` (POST): resolves with `data._`; if `data.redirect` is present the client **throws**
  `'Redirects are not allowed in commands...'` (`command.svelte.js:L57-L63`). Commands never call
  `refreshAll()` — only `q`/`l` from the response update the page (`L48-L63`); after settle, any
  `withOverride` releases run (`L65`).
- `form` (POST): `({ issues = [], result } = data._ ?? {})` (`form.svelte.js:L260`).
  `issues` is `InternalRemoteFormIssue[]` = `{ name: string, path: (string|number)[], message: string, server?: boolean }`
  (`internal.d.ts:L643-L647`; kit's server sets `server: true` via `normalize_issue(issue, true)`,
  `form-utils.js:L534-L559`, `runtime/app/server/remote/form.js:L280-L281`). Client `name` is the
  dotted/bracketed path string (`a.b[0]`), used to match field inputs (`form-utils.js:L564-L587`).
  - `should_refresh = refreshes === null && !data.r`: if the developer did not call `.updates()`
    and the server did not flag `r: true`, a successful submission triggers `refreshAll()` (re-run
    every active query via GET and every load) (`form.svelte.js:L262-L286`;
    `refreshAll` at `client.js:L2773-L2776`).
  - `data.redirect` → `_goto(redirect, { refreshAll: should_refresh })`, returns `true` (`L266-L272`).
  - `succeeded = issues.length === 0` is the promise's boolean result (`L274-L286`).
  - `validate()` (meta `validate_only: true`): expects `data._` to be the **bare issues array**
    (`form.svelte.js:L728-L749`; server `form.js:L105-L107`).
  - Kit special-cases a result with `issues`: it returns `{type:'result', data}` **without**
    running `collect_remote_data` (no `q`/`l`/`r`) (`remote-functions.js:L265-L274`).
- `prerender` (GET): value is `data._` (`prerender.svelte.js:L107`); `data.redirect` navigates
  and resolves `undefined` (`L101-L105`). Successful values are stored in the Cache API keyed by
  URL, so the response must be deterministic for a given URL.

### How errors surface to app code

- Thrown `HandledHttpError` / `HttpError` carry `.status` and `.body` (`App.Error`)
  (`reference/kit/packages/kit/src/exports/internal/shared.js:L3-L24`).
- `Query`: on rejection, `handle_error(e, {...})` runs the client `handleError` hook unless the
  error is already `HandledHttpError` (server-produced bodies skip the hook) and stores the
  resulting `App.Error` in `query.error`; `query.current` keeps the previous value; awaiting the
  query rejects with `HandledHttpError(error)` (`query/instance.svelte.js:L131-L158`;
  `client.js:L2517-L2549`). An error received through `q[key].e` calls `fail()` directly, making
  `error = e` and the awaited promise reject (`L246-L259`).
- `Prerender`: same hook path, `resource.error` set, promise rejects (`prerender.svelte.js:L167-L176`).
- `command`: the promise rejects with the `HandledHttpError`; the app catches it.
- `form`: transport/`type:'error'` failures reset `result`/`issues` and rethrow (`form.svelte.js:L287-L290`);
  inside the attachment's submit handler the error is routed to `handle_error` then
  `set_nearest_error_page(error)` — i.e. the **nearest `+error.svelte` renders** (`L482-L488`).
- Validation failures on the server (schema issues for query/command/prerender) are jsonified as
  `{ status: 400, message: 'Bad Request' }`; the `issues` are **not** shipped to the client unless
  the app's `handleError` copies them (`runtime/server/errors.js:L56-L61`, `L72-L83`).

### Version header and `updated`

- `x-sveltekit-version` read on every non-streaming and streaming response
  (`shared.svelte.js:L118`, `query-live/iterator.js:L32`); if it differs from the bundle's
  `__SVELTEKIT_APP_VERSION__`, `updated.current` flips to true and the router will hard-reload on
  the next navigation. Omitting the header is safe (`new_version` falsy → no-op).

## Citations

- `reference/kit/packages/kit/src/runtime/client/remote-functions/shared.svelte.js:L113-L204`
- `reference/kit/packages/kit/src/runtime/client/remote-functions/shared.transport.spec.js:L42-L97`
- `reference/kit/packages/kit/src/runtime/client/remote-functions/query/index.js:L25-L35`
- `reference/kit/packages/kit/src/runtime/client/remote-functions/query/instance.svelte.js:L66-L80`, `L102-L161`, `L224-L259`
- `reference/kit/packages/kit/src/runtime/client/remote-functions/query-batch.svelte.js:L38-L88`
- `reference/kit/packages/kit/src/runtime/client/remote-functions/command.svelte.js:L37-L70`
- `reference/kit/packages/kit/src/runtime/client/remote-functions/form.svelte.js:L246-L303`, `L473-L491`, `L697-L761`
- `reference/kit/packages/kit/src/runtime/client/remote-functions/prerender.svelte.js:L99-L115`, `L158-L177`
- `reference/kit/packages/kit/src/runtime/client/client.js:L280-L295`, `L500-L510`, `L2517-L2549`, `L2773-L2776`
- `reference/kit/packages/kit/src/runtime/app/state/client.svelte.js:L196-L200`
- `reference/kit/packages/kit/src/types/internal.d.ts:L325-L359`, `L560-L563`, `L643-L647`
- `reference/kit/packages/kit/src/exports/internal/shared.js:L3-L44`, `L81-L90`
- `reference/kit/packages/kit/src/runtime/server/remote-functions.js:L185`, `L265-L274`, `L313-L350`, `L361-L490`
- `reference/kit/packages/kit/src/runtime/server/errors.js:L44-L83`
- `reference/kit/packages/kit/src/runtime/server/utils.js:L47-L52`
- `reference/kit/packages/kit/src/runtime/app/server/remote/form.js:L94-L145`, `L280-L302`
- `reference/kit/packages/kit/src/runtime/form-utils.js:L534-L587`

## Go implementation notes

- Response writer: `Content-Type: application/json`, `Cache-Control: private, no-store`,
  body `{"type":"result","data":"<devalue>"}`. Implement a devalue **stringifier** in Go (flat
  array, dedup by identity optional — the client accepts duplicated nodes; sentinels for
  `undefined`/NaN/±Inf/-0; `["Date", iso]` for time.Time; `["Map",k1,v1,...]`, `["Set",...]`,
  `["null", ...]` for prototype-less objects not needed).
- `query` GET handler: run the function; respond
  `{ _: value, q: { key: { v: value } } }` where `key = hash + "/" + name + "/" + rawPayload`
  (the raw string from `?payload=` or `""`). Errors: `{"type":"error","error":{"status":..,"message":..}}`
  with HTTP 200 (kit) — or a matching non-2xx status; the client handles both, but keep the body
  `type:'error'` so `status`/`message` survive.
- `command` POST handler: respond `{ _: result, q?: {...}, l?: {...}, r?: true }`. Provide a Go
  equivalent of `requested(query, limit)` that yields the client's `refreshes` entries for a given
  query function (split key on last `/`, decode payload, validate) — kit does **not** auto-refresh
  the requested keys; the command body must opt in (`runtime/app/server/remote/requested.js:L104-L175`,
  docs `reference/kit/documentation/docs/20-core-concepts/60-remote-functions.md:L1053-L1120`).
  Any `refresh()`/`set()` on a query inside a command must set `r: true` and add the node to `q`
  (`remote-functions.js:L392-L421`). Live queries reconnected by the server go in `l` with `{ v }`.
  Never put `redirect` in a command response.
- `form` POST handler: `_ = { result?, issues?, input? }`; on validation failure return
  `issues` (array of `{name, path, message, server: true}`) and skip `q`/`l`; on `validate_only`
  return `_ = issues[]`. Set `r: true` when the form body explicitly refreshed queries; otherwise
  the client will `refreshAll()` (a burst of GETs to every active query) — cheap correctness, but
  wasteful; prefer `q` + `r`. Redirect via `{ redirect: "/path" }` inside `data`, HTTP 200.
- `prerender` GET: `_ = value`; must be pure/deterministic per URL (client caches it in the Cache API
  across the app version). Optionally also emit `p: { key: { v } }` (kit does; the client ignores it
  for direct fetches).
- `query.batch` POST: `_ = [ {type:'result', data} | {type:'error', error} ]` in payload order.
- Emit `x-sveltekit-version` only if you know kit's `version.name` for the deployed bundle
  (kit's default is a build timestamp); otherwise omit it.

## Gotchas

- Forgetting `q` on a GET query response does **not** hang or error: the proxy's fetch fn returns
  `undefined`, so `Query.#run` marks the query `ready = true` with `current = undefined`
  (`query/instance.svelte.js:L114-L129`). Silent data loss, no error state. Always send `q`.
- `q` nodes with `e` for keys **not** in the client cache are dropped (`shared.svelte.js:L150-L155`);
  only values seed future instances.
- A `type:'error'` body on a non-2xx response: the client overrides `error.status` with the HTTP
  status (`{ status, ...result.error }` — note object spread order: `result.error.status` wins
  if present; the HTTP status is only the fallback). Keep them consistent.
- `data` must parse with the app's `transport` decoders; if skgo apps declare `transport` in
  hooks, Go must emit `["<name>", index]` tuples for those types.
- Form `issues` must be an array even when empty; the client destructures with default `[]` only
  for `undefined`. `null` would crash `.length`.
- A `command` response carrying `redirect` makes the client throw — use a result + client `goto`.
- The form client sends `refreshes` inside the binary meta (`remote_refreshes`), not JSON.

## Recipes

- Go struct for the envelope:
  `type envelope struct { Type string `json:"type"`; Data string `json:"data,omitempty"`; Error *AppError `json:"error,omitempty"` }`.
- Remote key helper: `func remoteKey(hash, name, payload string) string { return hash + "/" + name + "/" + payload }`;
  splitting: `i := strings.LastIndexByte(key, '/')`.
- Single-flight update from a command:
  `resp.Q[remoteKey(h, "getTodos", payload)] = node{V: todos}; resp.R = true`.
