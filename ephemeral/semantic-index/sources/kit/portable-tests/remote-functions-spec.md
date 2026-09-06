# remote-functions.spec.js — the `/_app/remote/*` server protocol, test by test

## Purpose

`reference/kit/packages/kit/src/runtime/server/remote-functions.spec.js` is the only
upstream unit test that drives the server half of the remote-functions protocol
directly (everything else is Playwright). It is short (97 lines, 2 tests) but it pins
the SSE framing of `query.live`, and its subject `remote-functions.js` (611 lines) is
the exact dispatcher skgo must reproduce for `query`, `query.batch`, `query.live`,
`command`, `form`, and `prerender` calls. This leaf records every test case, the fakes
it uses, and the wire facts in the subject that a Go port must match byte-for-byte.

Target versions: `@sveltejs/kit` 3.0.0-next.25
(`reference/kit/packages/kit/package.json:L3`), devalue `^5.9.0` (pinned clone is 5.9.2,
`reference/devalue/package.json:L4`).

## Key facts (SOURCE-DERIVED)

### Test harness (spec L1-L32)

- The module is imported dynamically after stubbing a compile-time global and
  initialising transport: `vi.stubGlobal('__SVELTEKIT_DEV__', false)`,
  `init_transport({})`, then `import('./remote-functions.js')` and
  `import('./internal.js')` for `set_hooks`
  (`reference/kit/packages/kit/src/runtime/server/remote-functions.spec.js:L11-L16`).
  `init_transport({})` means `stringify` is plain `devalue.stringify(value, {})`
  (`reference/kit/packages/kit/src/runtime/app/internal/transport.js:L47-L52`).
- `create_response(run)` (spec L21-L32) builds the minimal fakes:
  - `event = { request: new Request('http://localhost/_app/remote/test?payload=undefined') }`
    — only `request` is populated; nothing else on `RequestEvent` is touched.
  - `state = {}` (cast to `RequestState`).
  - `internals = { run }` where `run` is an `async function*` receiving `(event, state, arg)`.
  - `arg = undefined`.
  - Calls `create_live_query_response(event, state, internals, undefined)` — the exported
    function under test (subject L35).

### Test 1 — "cancellation ignores a value that arrives after generator.next()" (spec L34-L68)

- Regression for sveltejs/kit#16778 (L34).
- Fakes: `handle_error = vi.fn(() => ({ message: 'oops' }))` installed with
  `set_hooks({ handleError })` (L36-L37); two manual promises `parked` (generator blocks
  on it) and `parked_on_next` (signals the test that the generator has parked) (L38-L43).
- Generator: `yield 'initial'; did_park(); await parked; yield 'late';` (L45-L50).
- Reads `response.body` with a `ReadableStream` reader. **First chunk must decode to
  exactly** `'data: {"type":"result","result":"[\\"initial\\"]"}\n\n'` (L52-L56). Note the
  inner value is the devalue string `["initial"]`, JSON-encoded again as a string field
  `result`.
- Starts a second `reader.read()` (pending), waits for `parked_on_next`, then
  `reader.cancel()` resolves to `undefined` and the pending read resolves
  `{ value: undefined, done: true }` (L57-L61).
- Resumes the generator (`resume()`), yields to the event loop, and asserts
  `handle_error` was **not** called — enqueueing the late value after cancellation must be
  a silent no-op, not routed through `handleError` (L63-L67).

### Test 2 — "cancellation aborts the generator request signal and runs cleanup" (spec L70-L97)

- Generator: `try { yield 'initial'; did_park(); await <abort of event.request.signal> } finally { cleaned_up() }`
  (L76-L84). This asserts that the `event` passed to `run` carries a `request.signal`
  which fires on stream cancellation.
- Same first-chunk assertion (L87-L90).
- After `reader.cancel()`, pending read resolves done (L94-L95), and `cleaned_up` is
  called exactly once (`vi.waitFor(... toHaveBeenCalledOnce())`, L96).

### Subject wire facts a Go port must replicate — `create_live_query_response` (subject L35-L140)

- Wraps the event: `live_event.request = new Request(event.request, { signal: AbortSignal.any([event.request.signal, cancellation.signal]) })`
  (`reference/kit/packages/kit/src/runtime/server/remote-functions.js:L36-L42`). In Go:
  derive a `context.Context` cancelled by either client disconnect or stream teardown and
  hand it to the generator.
- Keep-alive: `KEEP_ALIVE_INTERVAL = 30_000` ms; after the last message, emits the SSE
  comment `': keep-alive\n\n'` if there is downstream capacity, and reschedules
  (L27, L55-L64).
- Frame format: `'data: ' + JSON.stringify(data) + '\n\n'` (L67-L71). Three data shapes:
  - `{ type: 'result', result: <devalue string> }` (L109)
  - `{ type: 'redirect', location }` when the generator throws `Redirect` (L116-L117)
  - `{ type: 'error', error: <handle_error_and_jsonify(...)> }` otherwise (L119-L121),
    followed by stream close (L124).
- Dedupe: a yielded value is only sent when `stringify(value)` differs from the previous
  serialised result (`if (result !== (result = stringify(value)))`, L108-L111). Identical
  consecutive values are swallowed.
- Pull model: one `pull()` at a time (`pulling` guard, L94-L95); loops `generator.next()`
  until a *changed* value is produced (L98-L112).
- Teardown (L74-L83): idempotent; clears keep-alive; aborts the cancellation controller;
  closes the stream only when not cancelled; calls `generator.return(undefined)` without
  awaiting (comment L80-L81: `return()` cannot interrupt a pending `next()`, cleanup is
  cooperative via `request.signal`).
- Also tears down on client abort: `event.request.signal.addEventListener('abort', () => teardown(true), { once: true })` (L85).
- Response headers: `cache-control: private, no-store`, `content-type: text/event-stream`
  (L133-L138). No status override (200).

### Subject wire facts — `handle_remote_call` / `handle_remote_call_internal` (subject L143-L352)

- Wrapped in a tracing span `sveltekit.remote.call` and `with_request_store`; result passed
  through `with_version_header` (L143-L157).
- Routing key: `const [hash, name, additional_args] = id.split('/')` where `id` is the
  path after `${base}/${app_dir}/remote/` (L166; `strip_remote_prefix` L595-L597).
  404 if the hash is not an own property of `manifest._.remotes` or the module's default
  export lacks `name` (L169-L174).
- `headers = state.prerendering ? undefined : { 'cache-control': 'private, no-store' }` (L185).
- Dispatch on `internals.type` (L191-L311):
  - `query_live`: must be `GET` else `SvelteKitError(405, 'Method Not Allowed', '`query.live` functions must be invoked via GET request, not <METHOD>')`;
    arg = `parse_remote_arg(new URL(request.url).searchParams.get('payload'))`; returns the
    SSE response above (L192-L206).
  - `query_batch`: must be `POST` (405 text analogous, L209-L215); body JSON
    `{ payloads: string[] }`; each payload parsed with `parse_remote_arg`; `data._ = internals.run(args)` (L217-L222).
  - `form`: must be `POST` (405, L228-L234); must satisfy `is_form_content_type(request)`
    else `SvelteKitError(415, 'Unsupported Media Type', '`form` functions expect form-encoded data — received <content-type>')` (L236-L244).
    Body parsed by `deserialize_binary_form(event.request, internals.id)` giving
    `{ data: input, meta, form_data }` (L246-L250); `state.remote.requested = create_requested_map(meta.remote_refreshes)` (L251);
    for a keyed form (`form.for(key)`) with `additional_args` present and no `id` in
    input, `input.id = JSON.parse(decodeURIComponent(additional_args))` (L253-L257);
    `fn(input, meta, form_data)` runs with `state.is_in_remote_form_or_command = true` (L259-L263).
    If the result has `.issues`, respond immediately `{ type: 'result', data: stringify(data) }`
    **without** collecting refreshes (L265-L274).
  - `command`: body JSON `{ payload: string, refreshes?: string[] }`;
    `state.remote.requested = create_requested_map(refreshes)`; `fn(parse_remote_arg(payload))`
    under `is_in_remote_form_or_command = true` (L279-L291). Note: no method check here.
  - `prerender`: `fn(parse_remote_arg(additional_args))` — the arg is the third path
    segment, not a query param (L293-L299).
  - `query`: `fn(parse_remote_arg(searchParams.get('payload')))` (L301-L310).
- After dispatch: `await collect_remote_data(data, event, state)` then
  `Response.json({ type: 'result', data: stringify(data) }, { headers })` (L313-L321).
- Error paths (L322-L351):
  - `Redirect` → `collect_remote_data({ redirect: error.location }, ...)` then a
    **`type: 'result'`** response whose devalue payload has `redirect` instead of `_` (L323-L333).
  - anything else → `{ type: 'error', error: transformed }` with
    `status: state.prerendering ? transformed.status : undefined` (i.e. HTTP 200 at runtime)
    and `cache-control: private, no-store` (L335-L350).

### `collect_remote_data` — the `RemoteFunctionData` envelope (subject L361-L490)

- Output object keys: `_` (the primary result), `r: true` when explicit refreshes were
  applied (L396), and per-type maps `q`, `p`, `l`, `f` (L398-L400, L448-L450) keyed by
  remote key → `{ v }` on success or `{ e }` on error (L410, L416, L463, L475).
- Type letter: `internals.type === 'query_live' ? 'l' : internals.type[0]` → `q`
  (query/query_batch), `p` (prerender), `l` (query_live), `f` (form) (L398-L400).
- Remote key: `create_remote_key(id, payload)` = `id + '/' + payload`
  (`reference/kit/packages/kit/src/runtime/shared.js:L337-L339`); form outputs are keyed
  by the client action id directly (L442).
- Explicit pass (L382-L431): drains `state.remote.explicit` (a Map) repeatedly because
  settling a refresh can register more; each `fn()` is awaited; a fresh `{ v }` replaces
  the node entirely; `Redirect` rejections are skipped (handled elsewhere).
- Implicit pass (L433-L485): skips internals without `id` (private functions, L438),
  skips keys already processed explicitly (L446); races each promise against a resolved
  microtask — **still-pending promises are omitted entirely** so the client refetches
  (L452-L482). A Go port has no microtask race; the equivalent is "include only promises
  already settled at collection time".
- `create_requested_map(refreshes)` (L495-L512): `Map<id, payload[]>` built with
  `split_remote_key` (last `/`, `reference/kit/packages/kit/src/runtime/shared.js:L345-L356`).

### `handle_remote_form_post` — non-enhanced `<form>` POST to a page URL (subject L515-L583)

- Triggered when the page URL has `?/remote=<id>` (`get_remote_action` L609-L611); `id`
  is split as `[hash, name, ...rest]` and `action_id = rest.join('/')` because a
  JSON-stringified `form.for(key)` key may contain `/` (L539-L542).
- Missing form → `method_not_allowed_result(event, location)` (L550-L552).
- `location = get_action_location(event.url)` (L538) — same helper as page actions.
- Success returns `{ type: 'success', status: 200, location }` and deliberately does not
  return the data (comment L573-L574); errors go through `action_error_result` (L580-L582).

### URL helpers (subject L588-L611)

- `has_remote_prefix(url)`: `url.pathname.startsWith(`${base}/${app_dir}/remote/`)`.
- `strip_remote_prefix`, `get_remote_id` (returns `false` when not remote).
- `get_remote_action(url)`: `url.searchParams.get('/remote')`.

## Citations

- Spec: `reference/kit/packages/kit/src/runtime/server/remote-functions.spec.js:L1-L97`
- Subject: `reference/kit/packages/kit/src/runtime/server/remote-functions.js:L27-L140` (live),
  `L143-L352` (dispatch), `L361-L490` (collect), `L495-L512` (requested map),
  `L515-L583` (form post), `L588-L611` (URL helpers)
- Arg codec: `reference/kit/packages/kit/src/runtime/shared.js:L252-L331`, keys `L337-L356`
- Transport: `reference/kit/packages/kit/src/runtime/app/internal/transport.js:L32-L53`
- Error envelope: `reference/kit/packages/kit/src/runtime/server/errors.js:L44-L89`
- Form content type: `reference/kit/packages/kit/src/utils/http.js:L73-L79`
- Related spec (prerender wrapper self-fetch of the same response shapes):
  `reference/kit/packages/kit/src/runtime/app/server/remote/prerender.spec.js:L57-L137`

## Go port notes

- Package: `internal/remote`. Suggested surface:
  `func HandleCall(w, r, state, manifest, id string)`,
  `func LiveQueryResponse(ctx, run func(ctx, arg) <-chan Result, ...) http.Handler`,
  `func CollectRemoteData(...) RemoteFunctionData`.
- Mirror test 1 as a Go table test with an `httptest.ResponseRecorder`-free streaming
  harness: use `net/http/httptest.NewServer` + a real client, or a custom
  `http.ResponseWriter` that exposes written bytes through a channel. Feed a generator
  implemented as a Go func returning a channel; assert the first frame bytes equal
  `"data: {\"type\":\"result\",\"result\":\"[\\\"initial\\\"]\"}\n\n"`; cancel the request
  context; assert no further frames are written and the `handleError` fake was not invoked.
- Mirror test 2 by passing the derived context to the generator and asserting a `defer`
  ran once after cancellation (use a `sync.WaitGroup` + timeout instead of `vi.waitFor`).
- `stringify(value)` for the `result` field must be devalue-compatible (see
  `devalue-port.md`); `JSON.stringify` of the outer envelope must not escape `/` or `<`
  (Go's `encoding/json` escapes `<`, `>`, `&` by default — use
  `Encoder.SetEscapeHTML(false)`).
- Fakes to model: `set_hooks({ handleError })`, `manifest._.remotes[hash]()` returning a
  module whose `default[name].__` carries `{ type, id, name, fn/run }`, and
  `state.remote.{explicit, implicit, data, requested}`.
- Additional table tests worth writing even though upstream lacks them (all facts above
  are source-derived): method checks (405 texts), 415 text, 404 on unknown hash/name,
  redirect-as-result envelope, error envelope status 200 vs prerender status.

## Gotchas

- `payload=undefined` in the fake URL is a literal string; `parse_remote_arg('undefined')`
  would base64-decode garbage — the test never reaches parsing because it calls
  `create_live_query_response` directly with `arg = undefined`. Do not "fix" the fake.
- Dedupe compares serialised strings, so devalue output must be deterministic for equal
  values (Go map iteration order would break this — use ordered maps).
- `Response.json` sets `content-type: application/json`; kit relies on that default.
- Runtime errors return HTTP 200 with `type: 'error'`; only prerendering surfaces the real
  status. The client distinguishes by envelope `type`, not HTTP status.
- `command` has no method guard in the subject; CSRF for non-GET remote calls is enforced
  earlier by `is_remote_forbidden` (see `port-map.md`, csrf row).
- `additional_args` for `form` is `decodeURIComponent`-ed then `JSON.parse`-d; for
  `prerender` it is passed raw to `parse_remote_arg` (url-safe base64). Different codecs
  for the same path slot.

## Recipes

- To reproduce the live frame in Go: `frame := "data: " + jsonNoHTMLEscape(map[string]any{"type":"result","result": devalueStringify(v)}) + "\n\n"`.
- To emit a redirect from any remote type: respond `{"type":"result","data": devalueStringify({"redirect": location, ...collected})}` — not an HTTP 3xx.
- To build the implicit-pass omission rule without microtasks: only include entries whose
  promise/future is already resolved when `CollectRemoteData` runs; never block on them.
