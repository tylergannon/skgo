# Remote function request handling: `/_app/remote/*` (kit 3.0.0-next.25, pinned source)

**Purpose.** The exact server-side contract for the URL kit's client bundle calls for
every remote-function kind: routing, method rules, argument decoding, the response
envelope, error/redirect serialization, headers and CSRF. A Go handler that reproduces
this is indistinguishable from kit's Node server to the unmodified client.

All facts below are SOURCE-DERIVED from the pinned clone under the token cache root.

## Key facts

### Routing and id parsing

- A request is a remote call iff `url.pathname.startsWith(\`${base}/${app_dir}/remote/\`)`;
  the remainder is the `id`. `reference/kit/packages/kit/src/runtime/server/remote-functions.js:L588-L604`.
  `respond.js` computes `remote_id = get_remote_id(url)` up front
  `reference/kit/packages/kit/src/runtime/server/respond.js:L99`.
- `id.split('/')` -> `[hash, name, additional_args]`. Lookup: `manifest._.remotes[hash]`
  (404 if absent), then `module.default[name]` (404 if absent). `remote-functions.js:L165-L174`.
  Both 404s are thrown via `error(404)` INSIDE the try, so they come back as a
  `type:'error'` envelope with HTTP **200** (see status rules below), body
  `{status:404,message:'Error: 404'}` (`reference/kit/packages/kit/src/exports/index.js:L81-L97`).
- Remote requests skip route resolution, trailing-slash normalisation, and the
  `/_app` static 404 (`respond.js:L166-L176,L256,L360-L364,L390`). They still pass
  through the user `handle` hook and `resolve` (`respond.js:L476-L540,L635-L637`).
- `event.url`: if the request carries `x-sveltekit-pathname` (and optionally
  `x-sveltekit-search`), `event.url` is rewritten to that page URL; otherwise
  (plain `query` GETs do not send them) `event.url` stays the endpoint URL and route
  resolution is skipped. `respond.js:L166-L176`. `event.isRemoteRequest = !!remote_id`
  (`respond.js:L240`).

### CSRF / origin

- In prod (`!__SVELTEKIT_DEV__`), for remote requests: forbidden iff
  `request.method !== 'GET' && request_origin !== self_origin` -> `403` JSON
  `{ message: 'Cross-site remote requests are forbidden' }`. `trusted_origins` are
  NOT honoured for remote endpoints. `self_origin = paths.origin || url.origin`.
  `reference/kit/packages/kit/src/runtime/server/csrf.js:L19-L21,L63-L65`,
  `respond.js:L101-L115`.
- A missing `Origin` header on a POST is therefore rejected (`null !== self_origin`).
  Browsers always send `Origin` on same-origin POST fetches, so this is fine for the
  client; curl tests must add `-H 'Origin: <self>'`.

### Per-kind method / body rules (`remote-functions.js:L191-L311`)

| kind (`internals.type`) | method | argument source |
|---|---|---|
| `query` | any (client uses GET) | `?payload=<base64url>`; `parse_remote_arg` (empty/absent -> `undefined`) `L301-L310` |
| `query_live` | GET only, else 405 SvelteKitError "`` `query.live` functions must be invoked via GET request, not ${method} ``" | `?payload=` `L192-L206` -> SSE response (see live-and-stream.md) |
| `query_batch` | POST only, else 405 "`` `query.batch` functions must be invoked via POST request... ``" | JSON body `{ payloads: string[] }`, each parsed with `parse_remote_arg` `L208-L225` |
| `command` | any (client uses POST); command wrapper additionally throws if `event.request.method` not in `MUTATIVE_METHODS` | JSON body `{ payload: string, refreshes?: string[] }` `L279-L291` |
| `form` | POST only (405) and `is_form_content_type` else 415 "`` `form` functions expect form-encoded data — received ${ct} ``" | `deserialize_binary_form` (binary or FormData); `additional_args` = keyed-form key (JSON, URI-encoded) `L227-L277` |
| `prerender` | any (client uses GET) | `additional_args` path segment = payload; `parse_remote_arg(additional_args)` `L293-L299` |

- Method mismatches throw `SvelteKitError(405|415, text, message)` inside the try ->
  serialized as `{type:'error', error:{status:405, message:'Method Not Allowed'}}`
  with HTTP 200 (framework errors use `{status, message: error.text}`,
  `reference/kit/packages/kit/src/runtime/server/errors.js:L54-L55`).
- `refreshes` (command) / `meta.remote_refreshes` (form) are turned into
  `state.remote.requested: Map<id, payload[]>` via `split_remote_key` (split on LAST
  `/`). `remote-functions.js:L495-L512`, `reference/kit/packages/kit/src/runtime/shared.js:L345-L356`.
  Keys look like `<hash>/<name>/<payload>`; payload may be empty (`<hash>/<name>/`).

### Argument decoding

- `parse_remote_arg(s)`: `'' | null` -> `undefined`; else base64url->base64 (`-`->`+`,
  `_`->`/`, padding optional), UTF-8 decode, `devalue.parse(json, revivers)`.
  `reference/kit/packages/kit/src/runtime/shared.js:L321-L331`. Revivers in
  serialization.md.
- Validation: a remote created without a schema rejects ANY argument (`arg !== undefined`
  -> `error(400,'Bad Request')`); `'unchecked'` passes through; a Standard Schema
  validator throws `ValidationError(issues)` on failure.
  `reference/kit/packages/kit/src/runtime/app/server/remote/shared.js:L12-L45`.
  `ValidationError` serializes as `{status:400, message:'Bad Request'}` (issues are NOT
  put on the wire unless a `handleError` hook adds them) `errors.js:L56-L61,L72-L73`.

### Response envelope (`RemoteFunctionResponse`)

`reference/kit/packages/kit/src/types/internal.d.ts:L325-L359`

- Success: `Response.json({ type: 'result', data: stringify(data) })` where `data` is a
  `RemoteFunctionData` object and `stringify` = devalue.stringify with transport
  encoders. NOTE `data` is a **string** (devalue JSON) nested inside plain JSON.
  `remote-functions.js:L315-L321`.
- `RemoteFunctionData` keys (all optional): `_` (the call's own result), `q`, `l`, `p`,
  `f` (maps of `remote_key -> { v?: any, e?: App.Error }`), `r: true` (explicit
  refreshes happened), `redirect: string`. `internal.d.ts:L332-L347`.
- Per kind, `data._` is: query -> result; query_batch -> array of
  `{type:'result',data}|{type:'error',error}` (one per payload, same order)
  `reference/kit/packages/kit/src/runtime/app/server/remote/query.js:L333-L361`;
  command -> result; prerender -> result; form -> output object
  `{ submission: true, result?, issues?, input? }` (see form-and-prerender.md), or an
  array of issues when `meta.validate_only`.
- Form special-case: if `data._.issues` is set the response is emitted immediately
  WITHOUT `collect_remote_data` (no `q`/`l`/`r`). `remote-functions.js:L265-L274`.
- Otherwise `collect_remote_data(data, event, state)` appends `q`/`l`/`p`/`f` nodes:
  explicit refreshes (`refresh()`/`set()`/`reconnect()` called inside the command/form,
  or via `requested(...)`) are awaited, set `data.r = true`, and stored as
  `{ v }` or `{ e }` under `data[type][key]` with `type = 'l'` for `query_live` else
  `internals.type[0]` (`q` for both `query` and `query_batch`, `p`, `f`).
  `remote-functions.js:L361-L490`. Implicit (awaited-but-not-refreshed) queries are
  only included if already resolved at collection time (`L433-L485`) — an SSR concern;
  in a pure remote call they show up only if the command awaited a query.
- Redirect (thrown `Redirect`): `{ type:'result', data: stringify({ redirect: location, ...collected }) }`
  HTTP 200. `remote-functions.js:L323-L333`. The client reads `data.redirect` and
  navigates (`reference/kit/packages/kit/src/runtime/client/remote-functions/query/index.js:L29-L32`;
  commands throw "Redirects are not allowed in commands" on the client,
  `.../command.svelte.js:L57-L61`).
- Error: `{ type:'error', error: App.Error }` where `App.Error` is produced by
  `handle_error_and_jsonify`: `HttpError` -> its body (`{status,message,...props}`);
  `SvelteKitError` -> `{status, message: text}`; `ValidationError` -> `{status:400,message:'Bad Request'}`;
  anything else -> `{status:500, message:'Internal Error'}`; then the user's
  `handleError` hook may override fields. `errors.js:L44-L120`.
- **HTTP status of an error envelope is 200 at runtime** (`status: state.prerendering ? transformed.status : undefined`)
  with `cache-control: private, no-store`. `remote-functions.js:L335-L350`. The client
  handles both: non-2xx with a `type:'error'` JSON body -> `HandledHttpError({status, ...error})`;
  non-2xx otherwise -> `HttpError({status, message: statusText})`; 2xx with
  `type:'error'` -> `HandledHttpError(error)`.
  `reference/kit/packages/kit/src/runtime/client/remote-functions/shared.svelte.js:L113-L133`.
- A `Redirect` thrown OUTSIDE `handle_remote_call` (e.g. in the `handle` hook) becomes
  `{ type:'redirect', status, location }` JSON 200 (`respond.js:L451-L459`,
  `reference/kit/packages/kit/src/runtime/server/data/index.js:L161-L169`); only the
  live-query client path and `handle_side_channel_response` understand it
  (`shared.svelte.js:L189-L204`).

### Headers

- Success/redirect: `cache-control: private, no-store` (omitted while prerendering);
  `content-type: application/json` (from `Response.json`). `remote-functions.js:L185`.
- `x-sveltekit-version: <kit.version.name>` is set on every remote response when
  `output.bundleStrategy !== 'inline'` (`reference/kit/packages/kit/src/runtime/server/utils.js:L47-L52`,
  `remote-functions.js:L154`, `reference/kit/packages/kit/src/exports/vite/index.js:L480-L481`).
  `version.name` defaults to `Date.now().toString()` at build
  (`reference/kit/packages/kit/src/core/config/options.js:L307-L310`). The client
  compares it to its baked-in version and flips `updated.current` when different
  (`reference/kit/packages/kit/src/runtime/app/state/client.svelte.js:L196-L200`) —
  so a Go server must either send the SAME string the client bundle was built with, or
  omit the header (omitting disables new-deploy detection but never breaks).
- Cookies set via `event.cookies.set` inside commands/forms are appended by `resolve`
  (`respond.js:L516-L522`); queries/prerenders may not set cookies
  (`remote/shared.js:L89-L113`) and `setHeaders` is forbidden in all remotes (`L86-L88`).
- Cookie paths set in remotes must be absolute (`remote/shared.js:L96-L98`).

### Body size

- Kit itself imposes no body-size limit on remote requests; that is an adapter
  concern (adapter-node `BODY_SIZE_LIMIT`). Nothing in the pinned runtime enforces one.

## Citations

- `reference/kit/packages/kit/src/runtime/server/remote-functions.js:L143-L352` — the handler.
- `reference/kit/packages/kit/src/runtime/server/remote-functions.js:L361-L490` — collect_remote_data.
- `reference/kit/packages/kit/src/runtime/server/remote-functions.js:L495-L512` — requested map.
- `reference/kit/packages/kit/src/runtime/server/respond.js:L93-L176,L451-L459,L635-L637` — routing/CSRF/event.url.
- `reference/kit/packages/kit/src/runtime/server/csrf.js:L63-L65` — remote origin rule.
- `reference/kit/packages/kit/src/runtime/server/errors.js:L44-L120` — App.Error shaping.
- `reference/kit/packages/kit/src/runtime/shared.js:L321-L356` — parse_remote_arg, remote keys.
- `reference/kit/packages/kit/src/runtime/app/server/remote/shared.js:L12-L45` — validators.
- `reference/kit/packages/kit/src/runtime/client/remote-functions/shared.svelte.js:L95-L185` — what the client sends/expects.
- `reference/kit/packages/kit/src/types/internal.d.ts:L325-L359` — envelope types.

## Go implementation notes

- Mount one `http.Handler` per `<hash>/<name>` under `${base}/_app/remote/`. Parse the
  remainder by splitting on `/` into `hash`, `name`, `extra` (prerender payload or
  form key). Unknown -> respond 200 `{"type":"error","error":{"status":404,"message":"Error: 404"}}`
  (mirroring kit) or a real 404 with the same JSON body — the client accepts both.
- Enforce: POST-only for `query_batch`/`form`, GET-only for `query_live`; reject
  non-GET without matching `Origin` (403 `{message:'Cross-site remote requests are forbidden'}`).
- Query: read `payload` from the query string, base64url-decode, devalue-parse (Go
  needs a devalue decoder with kit's revivers), validate, run, respond
  `{"type":"result","data":"<devalue JSON of {_: result}>"}` plus
  `cache-control: private, no-store`.
- Command: JSON body `{payload, refreshes}`; run; if the Go command refreshed queries,
  include `q`/`l` nodes keyed by the EXACT client key (`<id>/<payload>` from
  `refreshes`) and set `r: true`; otherwise the client calls `refreshAll()`
  (invalidates everything) — omitting `r` is always safe, just less efficient.
- Errors: map Go errors to `{status,message}`; wrap in `type:'error'`, HTTP 200,
  `cache-control: private, no-store`.
- Redirect: `{type:'result', data: stringify({redirect: location})}` (query/form/
  prerender); for commands the client refuses redirects.
- Version header: send `x-sveltekit-version` only if you can pin the same
  `kit.version.name` into the client build (set `version.name` explicitly in the
  `sveltekit()` plugin options and mirror it in Go).
- Ignore: tracing spans, `with_request_store`, `state.remote.*` bookkeeping,
  `is_in_remote_*` flags — server-internal.
- Ignore (CSR-only): implicit `q`/`p`/`f` inlining into the HTML (`render.js:L522-L526`);
  with no SSR the client always fetches.

## Gotchas

- Error envelopes are HTTP 200 (except during prerender). Don't "fix" this to 4xx
  without a JSON body of the same shape — the client uses `statusText` as the message
  if the body isn't `{type:'error'}`.
- `hash` and `name` never contain `/`, but a keyed form's key segment can (JSON) — the
  no-JS form path rejoins `rest` (`remote-functions.js:L539-L542`); the enhanced path
  uses only the third segment (`L166,L255-L257`).
- `query` does not send `x-sveltekit-pathname`; `command`, `form`, `prerender` do.
  Don't require them.
- `ValidationError` issues are dropped from the wire body by default; only a custom
  `handleError` could expose them. Form validation issues travel separately in
  `data._.issues` (form-and-prerender.md).
- Kit 2 docs describe `query.batch` responses loosely; the pinned source puts an array
  of `{type,data|error}` objects in `_` in request order.
- CSRF `trusted_origins` do not apply to remote endpoints.

## Recipes

- To implement the dispatcher in Go: read `runtime/server/remote-functions.js:L165-L352` first, then `respond.js:L93-L176`.
- To implement error bodies: `runtime/server/errors.js:L44-L120` and `exports/index.js:L81-L97`.
- To implement single-flight refresh nodes (`q`/`l`, `r`): `remote-functions.js:L361-L431` and the client consumer `client/remote-functions/shared.svelte.js:L135-L185`.
- To test with curl: GET `/_app/remote/<hash>/<name>?payload=<b64url>`; POST with `Origin` header for commands.
