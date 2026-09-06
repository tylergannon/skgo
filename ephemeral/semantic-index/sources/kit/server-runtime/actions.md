# Form actions protocol (kit 3.0.0-next.25 server runtime)

## Purpose

Classic `+page.server.js` `actions` — `?/name` POSTs — as the Go server must
serve them for a CSR app. Covers request detection (enhanced vs native),
action name resolution, content-type constraints, the `ActionResult` shape,
devalue encoding of `data`, `fail()`, redirects, error paths, status codes, and
how the client consumes the response. With `ssr = false` the non-enhanced
(native) path has no way to surface `form` data (kit warns), so the enhanced
JSON path is the one that matters for skgo.

## Key facts

### Request detection

- A route is treated as an action request when the leaf page is rendered
  with `POST` (`is_action_request`) — after `is_endpoint_request` has decided
  the page (not an endpoint) handles it. POST with header
  `x-sveltekit-action: true` always goes to the page.
  `reference/kit/packages/kit/src/runtime/server/page/actions.js:L152-L154`,
  `reference/kit/packages/kit/src/runtime/server/endpoint.js:L99-L113`
- Enhanced submissions are detected by `is_action_json_request`:
  `negotiate(accept ?? '*/*', ['application/json','text/html']) === 'application/json' && method === 'POST'`.
  `reference/kit/packages/kit/src/runtime/server/page/actions.js:L14-L21`
- `render_page` short-circuits for JSON action requests before any load:
  `handle_action_json_request(event, state, node.server)`.
  `reference/kit/packages/kit/src/runtime/server/page/index.js:L48-L51`
- Non-page methods on a page route: `OPTIONS` → 204 with `allow`; others → 405.
  `allow` includes `POST` only when the leaf has `server.actions`.
  `reference/kit/packages/kit/src/runtime/server/respond.js:L688-L711`

### What the client sends (`use:enhance`)

`reference/kit/packages/kit/src/runtime/app/forms/client.js:L107-L252`

- URL = form's `action` (or submitter `formaction`), i.e. `<form action="?/login">`
  resolves to `/current/path?/login` (plus any existing search params).
- Headers: `accept: application/json`, `x-sveltekit-action: true`;
  `Content-Type` = the form's enctype if `application/x-www-form-urlencoded`
  or `text/plain`, else `application/x-www-form-urlencoded`; omitted for
  `multipart/form-data` (browser sets boundary). `L159-L174`
- Body: `FormData` for multipart, `URLSearchParams(form_data)` otherwise;
  `cache: 'no-store'`; POST. `L176-L185`
- Response handling: reads `x-sveltekit-version`; `text = await response.text()`;
  `parsed = text === '' && !response.ok ? undefined : deserialize(text)`
  (`deserialize` = `JSON.parse` then `parsed.data = devalue.parse(parsed.data, decoders)` if `data` present). If `parsed.type` ∈ {success, failure, redirect, error} it is the result and for `error`/`failure` `result.status = response.status` (HTTP status overrides body). Otherwise for `!response.ok`: an `App.Error`-shaped body (`message` string) → `HttpError({...parsed, status})`, anything else → `SvelteKitError(status, statusText, ...)`; then routed through client `handle_error`. `L187-L235`,
  `reference/kit/packages/kit/src/runtime/app/forms/shared.js:L26-L34`
- Default `update()` behaviour: `success` → reset form + `refreshAll()`;
  navigates to `result.location` if it's not the current location (for
  non-redirect results); `redirect` → `goto(location, { refreshAll: true })`;
  `error` → nearest error page; success/failure → `page.form = data`,
  `page.status = status`. `L68-L105`,
  `reference/kit/packages/kit/src/runtime/client/client.js:L3049-L3076`

### Action name resolution

`reference/kit/packages/kit/src/runtime/server/page/actions.js:L221-L249`

- Uses `new URL(event.request.url)` (the raw request URL, not `event.url`),
  scans `searchParams` in order for the first key starting with `/`; name is
  the key minus the slash; none → `'default'`. `?/default` explicitly →
  `Error('Cannot use reserved action name "default"')` (500).
- Unknown name → `SvelteKitError(404, 'Not Found', "No action with name '<name>' found")`.
- `actions.default` alongside named actions → 500
  `When using named actions, the default action cannot be used...`. `L207-L213`
- Content-type must be form-like (`application/x-www-form-urlencoded`,
  `multipart/form-data`, `text/plain`, `application/x-sveltekit-formdata`)
  else `SvelteKitError(415, 'Unsupported Media Type', "Form actions expect form-encoded data — received <ct>")`.
  `reference/kit/packages/kit/src/utils/http.js:L73-L83`, `reference/kit/packages/kit/src/runtime/form-utils.js:L105`
- `location` for results = `get_action_location(event.url)` = pathname+search
  with the first `/`-prefixed param removed (so `?/login&x=1` → `?x=1`).
  `L72-L83`

### Result construction

`reference/kit/packages/kit/src/runtime/server/page/actions.js:L162-L202`

- No `actions` export at all → `method_not_allowed_result`: sets `allow: GET`
  header, result `{ type:'error', location, error: SvelteKitError(405, 'Method Not Allowed', 'POST method not allowed. No form actions exist for this page') }`. `L90-L105`
- Action returns `ActionFailure` (from `fail(status, data)`) → `{ type:'failure', status, location, data }`.
- Any other return value → `{ type:'success', status: 200, location, data }`.
- Throws: `Redirect` → `{ type:'redirect', status, location }`; throwing a
  `fail()` → replaced by `Error('Cannot "throw fail()". Use "return fail()"')`;
  anything else → `{ type:'error', location, error }`. `L112-L128, L196-L201`
- DEV-only guards: returning a `Redirect`/`HttpError` object is an error. `L277-L285`
- `ServerActionResult` type: success/failure/redirect from `ActionResult`, plus
  `{ type:'error'; location; error: Error | HttpError }`.
  `reference/kit/packages/kit/src/types/internal.d.ts:L307-L309`

### JSON encoding of the result (enhanced path)

`reference/kit/packages/kit/src/runtime/server/page/actions.js:L39-L67, L133-L147`

- All JSON action responses use `Response.json(data, init)` +
  `x-sveltekit-version` header (`action_json`). Content-type therefore
  `application/json`.
- `redirect` → HTTP 200, body `{"type":"redirect","status":<3xx>,"location":"..."}`.
- `error` → `error = await handle_error_and_jsonify(event, state, result.error)`;
  HTTP status `error.status`; body `{"type":"error","location":"...","error":{status,message,...}}`.
- `success` with no data → HTTP 200 (init omitted!), body
  `{"type":"success","status":204,"location":"..."}` (note: `status: 204` is
  in the body; `data` omitted).
- `success`/`failure` with data → HTTP `result.status`; body
  `{"type":"success"|"failure","status":S,"location":"...","data":"<devalue.stringify string>"}` —
  `data` is a **JSON string containing the devalue-flattened array**, not a
  nested array. Serialization uses `stringify` from transport (user encoders).
  Serialization failure → converted into an `error` result via
  `action_error_result` with messages `Data returned from action inside <route.id> is not serializable[: <msg> (data.<path>)]` (or the `Response`-returned variant). `L301-L325`
- Redirects thrown outside the action (e.g. in `handle`) for a page route
  whose request is an action JSON request use `action_json_redirect`.
  `reference/kit/packages/kit/src/runtime/server/respond.js:L452-L461`
- `ActionResult` public type: `{ type:'success'; status; data?; location }`,
  `{ type:'failure'; status; data?; location }`, `{ type:'redirect'; status; location }`,
  `{ type:'error'; status?; error: App.Error; location? }`.
  `reference/kit/packages/kit/src/runtime/app/forms/types.d.ts:L18-L25`

### Non-enhanced (native) POST with `ssr = false`

`reference/kit/packages/kit/src/runtime/server/page/index.js:L61-L156`

- `handle_action_request` runs first; `redirect` → real HTTP redirect
  `redirect_response(status, location)` (`location` header, empty body).
  `error` → status from the error; `failure` → its status. Then, because
  `ssr === false`, kit returns the CSR shell HTML with that status; the
  result data is lost. DEV warns: "The form action returned a value, but it
  isn't available in `page.form`, because SSR is off..." `L118-L131`
- Prerendered pages cannot have actions (`Cannot prerender pages with actions`). `L86-L90`

### Remote-function forms

- `get_remote_action(event.url)` / `handle_remote_form_post` handle remote
  `form()` submissions on the page path; that's the remote-functions worker's
  domain. `reference/kit/packages/kit/src/runtime/server/page/index.js:L62-L65`

### Cookies and headers

- `event.cookies.set` inside an action works as in loads; cookies are appended
  to the JSON response in `respond.js` (`add_cookies_to_headers`) and for the
  redirect-thrown-in-handle path. `reference/kit/packages/kit/src/runtime/server/respond.js:L508-L518, L460`
- `event.setHeaders` values are copied onto the response too (so `allow: GET`
  from the 405 case is emitted). `L512-L515`

### CSRF

- Actions are form-content-type mutating requests, so `is_csrf_forbidden`
  applies: `Origin` must equal the self origin (`kit.paths.origin` or request
  origin) or be in `csrf.trustedOrigins`; failure → 403 with body
  `Cross-site POST form submissions are forbidden` (JSON `{message}` when
  `accept === 'application/json'` exactly, else text/plain).
  `reference/kit/packages/kit/src/runtime/server/respond.js:L101-L135`,
  `reference/kit/packages/kit/src/runtime/server/csrf.js:L37-L44`

## Citations

- `reference/kit/packages/kit/src/runtime/server/page/actions.js:L14-L21, L28-L67, L72-L83, L90-L147, L152-L202, L207-L249, L277-L325`
- `reference/kit/packages/kit/src/runtime/server/page/index.js:L48-L51, L61-L156`
- `reference/kit/packages/kit/src/runtime/server/endpoint.js:L99-L113`
- `reference/kit/packages/kit/src/runtime/server/respond.js:L101-L135, L452-L461, L508-L518, L688-L711`
- `reference/kit/packages/kit/src/runtime/server/csrf.js:L37-L44`
- `reference/kit/packages/kit/src/runtime/server/utils.js:L47-L52`
- `reference/kit/packages/kit/src/runtime/app/forms/client.js:L54-L263`
- `reference/kit/packages/kit/src/runtime/app/forms/shared.js:L26-L34`
- `reference/kit/packages/kit/src/runtime/app/forms/types.d.ts:L18-L25`
- `reference/kit/packages/kit/src/runtime/app/internal/transport.js:L32-L53`
- `reference/kit/packages/kit/src/runtime/client/client.js:L3049-L3076`
- `reference/kit/packages/kit/src/exports/internal/shared.js:L26-L44, L67-L76`
- `reference/kit/packages/kit/src/utils/http.js:L9-L57, L73-L83`
- `reference/kit/packages/kit/src/types/internal.d.ts:L307-L309`

## Go implementation notes

Must reproduce:

1. Route POST to the page's Go action set when (`x-sveltekit-action: true`)
   or the negotiated accept is not an endpoint-preferring type; pick action
   by the first `?/name` query key of the raw request URL; default `default`.
2. Enforce content-type allowlist (415), unknown action (404), no actions
   (405 + `allow: GET`), reserved `default` (500), CSRF origin (403 with the
   exact message string).
3. Build the result and encode for JSON requests exactly:
   - `data` field is a *string* of devalue-flattened JSON (double-encoded),
     produced by the same flattener used for loads plus transport encoders.
   - HTTP status: success/failure → `status`; error → `App.Error.status`;
     redirect → 200; success without data → 200 with body `status: 204`.
   - Always `application/json`, `x-sveltekit-version`.
4. For non-JSON POSTs (native forms, no JS): run the action, then either issue
   the real 3xx redirect or return the CSR shell with the action's status.
   The `form` data cannot be delivered without SSR; document this limitation
   rather than inventing a channel.
5. `location` computation must strip only the first `/`-prefixed param.
6. Provide `fail(status, data)` and `redirect(status, location)` equivalents
   in Go; `redirect` status must be 300–308 (kit validates in `redirect()`),
   and the location must be a valid header value (Headers ctor check).
   `reference/kit/packages/kit/src/exports/index.js:L129-L140`

Ignore: OpenTelemetry spans; DEV validations (`validate_action_return`);
`uneval_action_response` (only used for SSR hydration script — CSR does not
embed `form` in HTML).

## Gotchas

- `data` is a string, not an object, in the action JSON. The client's
  `deserialize` does `JSON.parse` then `devalue.parse(parsed.data)`. Sending
  a raw array will break the client.
- The client overwrites `result.status` from the HTTP status for `error` and
  `failure`; keep the HTTP status equal to the body status.
- Accept negotiation is by `negotiate()` priorities: `*/*` alone does not
  count as JSON; `use:enhance` sends exactly `accept: application/json`.
  A `fetch()` from user code with `accept: application/json` but no
  `x-sveltekit-action` header will hit `is_endpoint_request` first — if the
  route also has a `+server` with POST, the endpoint wins.
- `?/default` is rejected explicitly, but `?/` (empty name) resolves to
  `name = ''` → 404 unless an action literally named `''` exists.
- `throw fail()` is an error, `return fail()` is the API.
- With CSR-only pages a native form POST that returns data silently drops it
  (kit warns in DEV only). For skgo, treat non-enhanced form success as a
  no-op re-render of the shell unless the action redirects.
- The 405 for missing actions is an `error` *result* (JSON body with
  `type: 'error'`), not a plain 405 text response — but the HTTP status is
  405 and `allow: GET` is set via `setHeaders`.

## Recipes

Enhanced success with data:

```
POST /login?/signin HTTP/1.1
accept: application/json
x-sveltekit-action: true
content-type: application/x-www-form-urlencoded

user=tyler&pw=...
---
HTTP/1.1 200
content-type: application/json
x-sveltekit-version: 1712345678

{"type":"success","status":200,"location":"/login","data":"[{\"ok\":1,\"user\":2},true,\"tyler\"]"}
```

Failure via `fail(400, { missing: true })`:

```
HTTP/1.1 400
{"type":"failure","status":400,"location":"/login","data":"[{\"missing\":1},true]"}
```

Redirect via `redirect(303, '/dashboard')`:

```
HTTP/1.1 200
{"type":"redirect","status":303,"location":"/dashboard"}
```

Error via `error(401, 'nope')`:

```
HTTP/1.1 401
{"type":"error","location":"/login","error":{"status":401,"message":"nope"}}
```
