# `query.live` SSE framing and `query.batch` (kit 3.0.0-next.25, pinned source)

**Purpose.** Byte-level contract for the streaming response a `query.live` remote
returns and for the batched request/response of `query.batch`, so a Go handler can
feed kit's unmodified client iterator.

All facts below are SOURCE-DERIVED from the pinned clone under the token cache root.

## Key facts

### `query.live` request

- GET only (405 otherwise). Argument in `?payload=<base64url>`; parsed with
  `parse_remote_arg`. `reference/kit/packages/kit/src/runtime/server/remote-functions.js:L192-L206`.
- Client fetches `${base}/${app_dir}/remote/${id}${payload ? '?payload=' + payload : ''}`
  with an `AbortSignal`, no custom headers. `reference/kit/packages/kit/src/runtime/client/remote-functions/query-live/iterator.js:L25-L29`.
- Client pre-checks: non-ok -> error as in request-handling.md; if `content-type`
  includes `application/json` the body is treated as a side-channel envelope
  (`{type:'redirect'}` -> navigate, `{type:'error'}` -> throw) and then throws
  `Invalid query.live response`; body must be a stream. `iterator.js:L34-L52`.

### Response headers

- `content-type: text/event-stream`, `cache-control: private, no-store`, plus
  `x-sveltekit-version` (see request-handling.md). `remote-functions.js:L133-L138,L154`.

### Frame format (server -> client)

`reference/kit/packages/kit/src/runtime/server/remote-functions.js:L35-L140`

- Every frame is `'data: ' + JSON.stringify(obj) + '\n\n'` (single `data:` line, one
  space after the colon, blank line terminator). `L67-L71`.
- Frame objects:
  - `{ "type": "result", "result": "<devalue JSON string>" }` — `result` is
    `stringify(value)` (transport-aware devalue) of the yielded value. **Deduplicated**:
    a yielded value whose stringified form equals the previous one is NOT sent
    (`if (result !== (result = stringify(value)))`). `L108-L111`.
  - `{ "type": "redirect", "location": "<url>" }` — a thrown `Redirect`; stream then
    closes. `L116-L117`.
  - `{ "type": "error", "error": App.Error }` — any other throw, shaped by
    `handle_error_and_jsonify`; stream then closes. `L118-L122`.
- Keep-alive: after 30 000 ms (`KEEP_ALIVE_INTERVAL`) of silence the server writes the
  SSE comment `': keep-alive\n\n'` (only if the stream has desiredSize > 0), and
  reschedules; every `send` resets the timer. `L27,L55-L64`.
- Completion: when the user generator finishes (`done`), the server simply closes the
  stream (no terminal frame). `L103-L106`. The client iterator ends; the client
  resource marks itself done and will only restart on `reconnect()`.
- Cancellation: client abort (request signal) or stream `cancel()` -> `teardown(true)`:
  aborts a derived `AbortController` that the user function sees as
  `event.request.signal`, and calls `generator.return()` without awaiting. `L36-L42,L73-L85,L129-L131`.
- The server reads the generator in a pull loop guarded by `pulling` so only one
  `next()` is in flight; backpressure comes from `ReadableStream.pull`. `L93-L128`.

### Client parsing

- `read_sse` splits the byte stream on `'\n\n'`, collects lines starting with `data:`
  (joined with `\n`, each `.slice(5).trimStart()`), and `JSON.parse`s the result;
  blocks with no `data:` line (i.e. comments/keep-alives) are skipped.
  `reference/kit/packages/kit/src/runtime/client/sse.js:L7-L32`,
  `reference/kit/packages/kit/src/runtime/client/stream.js:L9-L45`.
- `type:'result'` -> `devalue.parse(node.result, app.decoders)` is yielded; anything
  else goes to `handle_side_channel_response` then throws. `iterator.js:L59-L67`.
- On `for await` exit the client cancels the reader. `iterator.js:L68-L74`.

### Reconnect semantics (server-visible)

- When a command/form refreshes a live query (explicit `reconnect()` on the server or
  a `refreshes` key that names a live query), the remote response includes
  `data.l[key] = { v } | { e }`; the client applies `v` (or fails on `e`) and, on the
  success path only, calls `resource.reconnect()` which opens a NEW GET stream.
  `reference/kit/packages/kit/src/runtime/client/remote-functions/shared.svelte.js:L166-L182`.
  Server side, the `v` for a live query is its FIRST yielded value
  (`create_live_query_resource.get_first_value`,
  `reference/kit/packages/kit/src/runtime/app/server/remote/query.js:L519-L528`).
- Multiple `for await` consumers of the same live query inside one request share one
  generator via `SharedIterator` (latest-wins backpressure) keyed by remote key —
  `query.js:L583-L597,L614-L661`, `reference/kit/packages/kit/src/utils/shared-iterator.js:L1-L30`.
  Server-internal; a Go implementation needs no equivalent unless Go remotes consume
  other Go live queries.
- User function may return an (async) iterator/iterable or generator; anything else
  throws "`query.live '<name>' must return an Iterator, Iterable, AsyncIterator or AsyncIterable`".
  `reference/kit/packages/kit/src/runtime/app/server/remote/shared.js:L166-L219`.

### `query.batch`

- POST only; JSON body `{ "payloads": string[] }` (each a base64url devalue arg; the
  client dedupes identical payloads before sending and collects calls within one
  macrotask). `remote-functions.js:L208-L225`,
  `reference/kit/packages/kit/src/runtime/client/remote-functions/query-batch.svelte.js:L18-L50`.
- Request headers from the client: only `Content-Type: application/json` (no
  `x-sveltekit-pathname`). `query-batch.svelte.js:L32-L34`.
- Server: validates every arg (`Promise.all(args.map(validate))`), calls the user
  function ONCE with the array of validated args; it must return a
  `(arg, index) => result` accessor; each is invoked and wrapped as
  `{ type:'result', data }` or `{ type:'error', error: App.Error }`.
  `reference/kit/packages/kit/src/runtime/app/server/remote/query.js:L333-L361`.
- Response: standard envelope with `data._ = [ ...per-payload results in request order ]`.
  Client resolves/rejects each promise positionally (`query-batch.svelte.js:L66-L79`).
  If the whole call throws (e.g. one validator fails), the outer envelope is
  `type:'error'` and ALL batched promises reject.
- If the response contains `data.redirect` the client navigates and resolves every
  batched promise with `undefined`. `query-batch.svelte.js:L51-L63`.

### `create_async_iterator` (utils/streaming.js)

- `reference/kit/packages/kit/src/utils/streaming.js:L11-L39` is the SSR streamed-
  promise helper for `load` data, NOT used by remote functions. Ignore for skgo.

## Citations

- `reference/kit/packages/kit/src/runtime/server/remote-functions.js:L27-L140` — SSE writer.
- `reference/kit/packages/kit/src/runtime/client/sse.js:L7-L32` — SSE reader.
- `reference/kit/packages/kit/src/runtime/client/remote-functions/query-live/iterator.js:L19-L75` — client iterator.
- `reference/kit/packages/kit/src/runtime/app/server/remote/query.js:L153-L211,L247-L392,L515-L661` — live/batch internals.
- `reference/kit/packages/kit/src/runtime/client/remote-functions/query-batch.svelte.js:L18-L86` — batch client.

## Go implementation notes

- Live handler: set the three headers, `Flush()` after each write, write
  `data: {"type":"result","result":"<devalue>"}\n\n` per distinct value, a
  `: keep-alive\n\n` comment every 30 s of silence, and close the connection when the
  Go iterator ends. On `r.Context().Done()` cancel the producer.
- The `result` field is a JSON string containing devalue JSON — double encoding (JSON
  string escaping of the devalue text) is required. Errors: `data: {"type":"error","error":{"status":500,"message":"Internal Error"}}\n\n` then close.
- Dedup identical consecutive serialized values to match kit (optional; the client
  tolerates duplicates but reactive consumers would re-render).
- Batch handler: decode `payloads`, validate all (any failure -> whole-call error
  envelope), run, return `_` as an ordered array of `{type,data}`/`{type,error}`.
- Ignore: `SharedIterator`, `state.remote.live_iterators`, `pulling` guard.

## Gotchas

- No `event:`/`id:` SSE fields, no retry directive; the client's parser only reads
  `data:` lines. Multi-line `data:` would be joined with `\n`, which would break
  `JSON.parse` — keep one line per frame.
- The client only treats `content-type` containing `application/json` as a
  side-channel envelope; an HTML error page on a live URL yields a generic error.
- Keep-alive comments are dropped by the client parser (no `data:` line) — they must
  not be `data:` frames.
- Normal completion has no terminal frame; a reverse proxy that buffers SSE will
  break liveness — set `X-Accel-Buffering: no` or equivalent if fronted by nginx
  (not a kit requirement, an ops one).
- `query.batch` argument order in `_` must be the request's `payloads` order after the
  client's dedupe (the client sends `Array.from(batched.keys())`).

## Recipes

- To implement the SSE writer in Go: read `remote-functions.js:L35-L140` then `client/sse.js`.
- To implement batch: `remote-functions.js:L208-L225` + `remote/query.js:L333-L361` + `query-batch.svelte.js:L40-L80`.
- To implement server-triggered reconnect (`l` nodes): `remote-functions.js:L361-L431` + `client/remote-functions/shared.svelte.js:L166-L182`.
