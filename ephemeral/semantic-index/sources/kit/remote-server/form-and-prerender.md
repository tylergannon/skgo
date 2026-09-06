# `form` and `prerender` remotes: wire formats and placement (kit 3.0.0-next.25, pinned source)

**Purpose.** How an enhanced `form` submission is encoded (binary form, field naming,
`meta`), what the server returns (result/issues/input), the no-JS fallback, and how
`prerender` results are written at build and fetched at runtime.

All facts below are SOURCE-DERIVED from the pinned clone under the token cache root.

## Key facts — `form`

### Field naming (what the browser posts)

- `form.fields.<path>.as(type)` renders `name = <prefix><path>[<'[]'>]/<form_id>`, where
  `form_id = '<hash>/<name>'` (keyed forms still use the un-keyed id), prefix is `n:`
  for number/range (or hidden/submit with numeric value), `b:` for single checkbox
  (or hidden/submit with boolean value), `[]` suffix for `file multiple`,
  `select multiple`, or checkbox with a string value.
  `reference/kit/packages/kit/src/runtime/form-utils.js:L613-L622,L757-L770`.
  Example: `n:age/abc123/signup`, `tags[]/abc123/signup`, `address.street/abc123/signup`.
- Server-side parse of a key: must end with `/<form_id>` (else throws
  "Form contained a field that wasn't created with form.fields.as(...)"), strips
  prefix/suffix, path validated by `/^[a-zA-Z_$]\w*(\.[a-zA-Z_$]\w*|\[\d+\])*$/`,
  deep-set into a POJO with `__proto__`/`constructor`/`prototype` rejected.
  `form-utils.js:L26-L51,L461-L528`.
- Coercion: `n:` -> `parseFloat` (`''` -> `undefined`), `b:` -> `value === 'on'`;
  duplicated non-array keys throw; empty file inputs are dropped. `form-utils.js:L58-L103`.

### Enhanced submission body: `application/x-sveltekit-formdata`

`reference/kit/packages/kit/src/runtime/form-utils.js:L105-L171` (serialize, client) and
`L178-L339` (deserialize, server).

- Client POSTs to `${base}/${app_dir}/remote/<hash>/<name>` with headers
  `Content-Type: application/x-sveltekit-formdata`, `x-sveltekit-pathname`,
  `x-sveltekit-search`. `reference/kit/packages/kit/src/runtime/client/remote-functions/form.svelte.js:L240-L257`.
- Binary layout (little-endian):
  1. `u8` version = `0`
  2. `u32` header length (bytes)
  3. `u16` file-offset-table length (bytes; `0` if no files)
  4. header: `devalue.stringify([data, meta], { File: f => [name, type, size, lastModified, fileIndex] })`
     — ONLY the `File` reducer, no transport encoders.
  5. file offset table: `JSON.stringify([offset0, offset1, ...])`, offsets relative to
     the end of the table; files are sorted smallest-first in the body but the table is
     indexed by original `fileIndex`.
  6. raw file bytes, concatenated.
- `data` is the converted POJO (`convert_formdata` on the client, plus `id: key` for
  keyed forms) — `form.svelte.js:L190-L200`. `meta` is
  `{ remote_refreshes?: string[], validate_only?: boolean }`
  (`reference/kit/packages/kit/src/types/internal.d.ts:L560-L563`).
- Server validation of the binary: version mismatch, short header, invalid offset
  table, duplicate file index, gaps/overlaps between file spans -> `SvelteKitError(400,
  'Bad Request', 'Could not deserialize binary form: <reason>')`. `form-utils.js:L235-L345`.
- Any other form content type (`application/x-www-form-urlencoded`, `multipart/form-data`,
  `text/plain`) is read with `request.formData()` and converted with `convert_formdata`
  (`meta = {}`, `form_data` retained). `form-utils.js:L179-L182`,
  `reference/kit/packages/kit/src/utils/http.js:L73-L83`. Non-form content types -> 415.

### Server processing (`remote/form.js`)

`reference/kit/packages/kit/src/runtime/app/server/remote/form.js:L90-L153`

- Order: `output = { submission: true }`; if a schema exists, validate `data`;
  if `meta.validate_only` -> return `issues.map(normalize_issue(issue, true)) ?? []`
  (so `data._` is an ARRAY of issues, possibly empty). Otherwise on schema issues ->
  `handle_issues`; else run user fn with `(data, invalid)`; a thrown `ValidationError`
  (from `invalid(...)`) -> `handle_issues`; other errors propagate to the error envelope.
- `handle_issues` sets `output.issues = [{ name, path, message, server: true }]`
  (`normalize_issue`: `name` is `a.b[0].c` dotted/bracketed, `path` array). For
  NON-enhanced submissions it also sets `output.input` from the raw FormData, redacting
  fields whose name starts with `_` (regex `/^[.\]]?_/`). `form.js:L280-L302`,
  `form-utils.js:L534-L559`.
- Success: `output.result = <user return>`. Response `data._ = output`; the client
  reads `{ issues = [], result } = response._` and treats `issues.length === 0` as
  success. `form.svelte.js:L260-L285`.
- When `data._.issues` exists the handler returns immediately without
  `collect_remote_data` (no `q`/`l`/`r`). `reference/kit/packages/kit/src/runtime/server/remote-functions.js:L265-L274`.
- `refreshes`: `meta.remote_refreshes` -> `state.remote.requested`; if the server does
  no explicit refresh and the client passed no `updates(...)`, the client calls
  `refreshAll()` after success (`should_refresh = refreshes === null && !response.r`).
  `form.svelte.js:L262-L282`.
- Keyed forms (`form.for(key)`): client action id `= id + '/' + JSON.stringify(key)`
  used only for cache/hydration keys; the POST URL is the un-keyed id and `key` rides
  inside `data.id`. `form.svelte.js:L77-L79`. Server: if `additional_args` present
  and `data` has no `id`, `input.id = JSON.parse(decodeURIComponent(additional_args))`.
  `remote-functions.js:L253-L257`.
- Redirect thrown in the form fn -> `{ redirect }` in `data` (HTTP 200); the client
  `goto`s with `refreshAll` per `should_refresh`. `form.svelte.js:L266-L271`.
- Validation-only (`validate_only`) calls are what `form.validate()`/preflight use;
  they hit the same URL with the same content type. `form.svelte.js:L727-L748`.

### No-JS fallback (progressive enhancement)

- `form.action` renders as `?<existing query>&/remote=<encodeURIComponent(action_id)>`
  on the CURRENT page URL, `method="POST"`. `form.js:L157-L168`, `form.svelte.js:L79`.
- The page route's POST handler detects `url.searchParams.get('/remote')`
  (`remote-functions.js:L609-L611`) and calls `handle_remote_form_post`
  (`reference/kit/packages/kit/src/runtime/server/page/index.js:L61-L80`): id split
  as `[hash, name, ...rest]` with `rest.join('/')` = JSON key; unknown form -> 405
  `{type:'error', error: SvelteKitError(405,...)}` with `allow: GET`; runs
  `__.fn(data, meta, form_data)` with `event.isRemoteRequest === false`; on success
  returns `{ type:'success', status:200, location }` and kit then SSRs the page,
  inlining the form output under `data.f['<id>[/<key>]']` for hydration
  (`form.js:L142-L149`, `reference/kit/packages/kit/src/runtime/server/page/render.js:L522-L526`,
  client consumption `reference/kit/packages/kit/src/runtime/client/client.js:L506-L509`,
  `form.svelte.js:L102-L108`). Redirects -> plain 3xx `Location` response.
  `remote-functions.js:L515-L583`.
- `location` for error/redirect results is the page URL with the first `/`-prefixed
  search param removed (`reference/kit/packages/kit/src/runtime/server/page/actions.js:L72-L83`).

## Key facts — `prerender`

`reference/kit/packages/kit/src/runtime/app/server/remote/prerender.js:L61-L163`

- URL: `${base}/${app_dir}/remote/<hash>/<name>` for no-arg functions, or
  `.../<hash>/<name>/<payload>` where `payload = stringify_remote_arg(arg)` (sorted,
  base64url). Client fetches it with GET and `x-sveltekit-pathname/search` headers,
  consulting the browser Cache API first. `reference/kit/packages/kit/src/runtime/client/remote-functions/prerender.svelte.js:L60-L115`.
- Build-time: every prerender export is enqueued — with `inputs()` results when
  `has_arg`, otherwise bare — and the response is saved to
  `${outDir}/output/prerendered/data/<appDir>/remote/<hash>/<name>[/<payload>]`
  (category `'data'` because the path starts with the remote prefix; no `.html`
  suffix because content-type is JSON). `reference/kit/packages/kit/src/core/postbuild/prerender.js:L248-L256,L268,L432,L457,L530-L546,L662-L719`.
- File body = `JSON.stringify({ type:'result', data: stringify({ _: result }) })`
  (same envelope as a runtime call), registered as a prerender dependency with a
  `Response.json` so the saved content-type is `application/json`. `prerender.js:L143-L149`.
  Errors during prerender produce non-200 (`status: transformed.status`) and fail the
  build unless `handleHttpError` allows (`remote-functions.js:L343-L345`).
- Runtime handler (`remote-functions.js:L293-L299`): `fn(parse_remote_arg(additional_args))`.
  Inside `fn` the wrapper re-canonicalises the arg (`stringify_remote_arg(arg)`) and,
  when NOT prerendering, NOT dev, and NOT a remote request, tries `fetch(url)` of the
  prerendered file before executing; for a direct remote request it always executes.
  `prerender.js:L85-L121`.
- `dynamic: false` (default) exports are stubbed in the server build to throw
  `Unexpectedly called prerender function. Did you forget to set { dynamic: true } ?`
  — so at runtime a non-prerendered input yields a 500 `Internal Error` envelope; only
  `{ dynamic: true }` functions may compute on demand.
  `reference/kit/packages/kit/src/exports/vite/build/remote.js:L65-L85`,
  `reference/kit/packages/kit/src/core/postbuild/analyse.js:L156-L163`.
- Prerender functions cannot set cookies or read `event.url/params/route`
  (`remote/shared.js:L89-L128`), and `query`/`query.live`/`query.batch` throw when
  called while prerendering (`remote/query.js:L95-L99,L194-L198,L373-L377`).

## Citations

- `reference/kit/packages/kit/src/runtime/form-utils.js:L26-L103,L105-L345,L461-L559,L613-L622,L757-L770`
- `reference/kit/packages/kit/src/runtime/app/server/remote/form.js:L65-L302`
- `reference/kit/packages/kit/src/runtime/server/remote-functions.js:L227-L277,L515-L583,L609-L611`
- `reference/kit/packages/kit/src/runtime/client/remote-functions/form.svelte.js:L77-L108,L190-L200,L225-L300,L727-L748`
- `reference/kit/packages/kit/src/runtime/app/server/remote/prerender.js:L61-L163`
- `reference/kit/packages/kit/src/core/postbuild/prerender.js:L248-L268,L432-L470,L530-L640,L654-L719`
- `reference/kit/packages/kit/src/runtime/client/remote-functions/prerender.svelte.js:L55-L115`

## Go implementation notes

- Form handler: accept POST at `/_app/remote/<hash>/<name>`; branch on `Content-Type`:
  `application/x-sveltekit-formdata` -> parse the binary layout (header devalue with a
  `File` reviver mapping `[name,type,size,lastModified,idx]` to a byte range);
  urlencoded/multipart -> `convert_formdata` semantics (prefix/suffix parsing,
  coercion, nesting). Reject anything else with 415.
- Respond `{"type":"result","data":stringify({_:{submission:true, result:<val>}})}` on
  success, `{_:{submission:true, issues:[{name,path,message,server:true}], input?}}`
  on validation failure (skip `q`/`l`/`r`), `{_: [issues...]}` when
  `meta.validate_only`.
- Name fields with the same `form_id` string Go used for the id so kit's client
  `parse_form_key` accepts them (skgo's generated Svelte-facing stub already carries
  the real id, so `form.fields.x.as()` produces the right names automatically).
- No-JS fallback: without SSR, Go cannot re-render the page with `f` data. Options:
  handle `POST <page>?/remote=<id>` in Go by running the form and responding with a
  303 redirect (kit's own behaviour for `redirect()`), or accept that the no-JS path
  only supports redirects. The client-only build never exercises `f` hydration.
- Prerender: at build, skgo must write `prerendered/data/_app/remote/<hash>/<name>[/<payload>]`
  files with the JSON envelope, or serve them dynamically at the same URLs with
  `content-type: application/json`. The client cannot tell the difference. Respect
  `dynamic`: refuse unknown payloads unless the Go function is marked dynamic.
- Ignore: `LazyFile` streaming, `state.prerendering.remote_responses` de-dupe, the
  `fetch()` self-call for SSR, Cache API on the client.

## Gotchas

- Binary form header uses ONLY the `File` reducer — transport encoders are not applied
  to form data (unlike query/command args and all responses).
- `validate_only` returns a bare array in `_`, not the `{submission,...}` object.
- Issue objects on the wire carry `name`, `path`, `message`, `server: true`; the
  client filters by `name` for per-field display.
- `input` echo (`output.input`) is only built for non-enhanced submissions and redacts
  `_`-prefixed fields; enhanced clients keep their own input.
- Prerender payloads in the URL are path segments (must be filename-safe: that is why
  base64url is used); the third segment for `form` is a URI-encoded JSON key instead —
  same slot, different meaning, disambiguated only by `internals.type`.
- Prerendered files have no extension; static hosts must be told the MIME type
  (kit records `type` from the response `content-type` in `prerendered.assets`).

## Recipes

- To implement binary form decoding in Go: read `form-utils.js:L105-L171` (writer) then `L178-L339` (reader, validation rules).
- To implement field-name parsing/coercion/nesting: `form-utils.js:L26-L103,L461-L528`.
- To implement issue shaping: `form-utils.js:L534-L587` and `remote/form.js:L280-L302`.
- To implement the no-JS path: `remote-functions.js:L515-L583` and `page/index.js:L61-L80`.
- To implement prerender file emission: `core/postbuild/prerender.js:L248-L256,L530-L546,L708-L719` and `remote/prerender.js:L143-L149`.
