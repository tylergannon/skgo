# kit client runtime: `query.live` stream consumption and `query.batch` protocol

Source: `@sveltejs/kit` 3.0.0-next.25 (pinned clone),
`reference/kit/packages/kit/src/runtime/client/remote-functions/query-live/*`,
`reference/kit/packages/kit/src/runtime/client/{sse,stream,ndjson}.js`,
`reference/kit/packages/kit/src/runtime/client/remote-functions/query-batch.svelte.js`.

## Purpose

What a Go `query.live` handler must stream (framing, message JSON, keep-alives, termination,
side-channel redirect/error) so kit's `LiveQuery` behaves — including its reconnection policy —
and the exact request/response contract for `query.batch`.

## Key facts

### Live request and response detection (`query-live/iterator.js:L19-L75`)

- `GET ${base}/${app_dir}/remote/${id}?payload=...` with an `AbortSignal`; no custom headers (`L25-L29`).
- `x-sveltekit-version` header is read (`L32`).
- Non-OK status: body parsed as JSON; `type:'error'` → `HandledHttpError(result.error)`, else
  `HttpError({ status, statusText })` (`L34-L41`).
- If `content-type` **includes `application/json`**, the body is treated as a one-shot
  side-channel `RemoteFunctionResponse`: `redirect` → `_goto(location)` then throws `Redirect(307)`;
  `error` → `HandledHttpError`; anything else → `HttpError(500, 'Invalid query.live response')`
  (`L43-L48`; `handle_side_channel_response` at `shared.svelte.js:L192-L204`).
- Otherwise the body must be a readable stream; it is parsed with `read_sse` (`L50-L67`).

### SSE framing the client parses (`sse.js:L1-L32`, `stream.js:L9-L46`)

- Text is decoded as UTF-8 and split on the **delimiter `\n\n`** (blank line); the trailing block
  at EOF is also yielded (`stream.js:L26-L45`).
- Within a block only lines starting with `data:` count; their remainders (after `data:` and
  leading whitespace trimmed via `trimStart`) are joined with `\n`; blocks with no `data:` lines
  (comments like `: keep-alive`, `event:`, `id:`, `retry:` lines) are skipped (`sse.js:L7-L31`).
- Each `data` payload is `JSON.parse`d and yielded (`sse.js:L29`).
- Only `\n\n` separates events; `\r\n\r\n` will not split correctly (`\r` would remain in the
  last line; JSON.parse tolerates trailing `\r` whitespace, but a leading `\r` line prefix breaks
  the `data:` match on subsequent lines). Use bare `\n`.
- `ndjson.js` exists (newline-delimited JSON with `fatal: true` decoding) but is **not** used by
  remote functions in this version (`ndjson.js:L1-L15`; no importer under `remote-functions/`).

### Live message JSON (`iterator.js:L59-L66`; kit's server at `runtime/server/remote-functions.js:L66-L127`)

- `{ "type": "result", "result": "<devalue string>" }` → `devalue.parse(result, app.decoders)` is
  yielded as the next value. Note the key is `result`, not `data`, and it is a devalue string of the
  **bare value** (not a `RemoteFunctionData` object).
- `{ "type": "redirect", "location": "<url>" }` → client navigates and the iterator throws `Redirect`.
- `{ "type": "error", "error": App.Error }` → throws `HandledHttpError`.
- Any other `type` → `HttpError(500, 'Invalid query.live response')`.
- Kit's server: `content-type: text/event-stream`, `cache-control: private, no-store`
  (`remote-functions.js:L134-L138`); frames as `'data: ' + JSON.stringify(msg) + '\n\n'` (`L69`);
  sends a `': keep-alive\n\n'` comment after 30 s of silence (`L27`, `L55-L64`); only sends a result
  when the stringified value **changed** from the previous one (`L108-L111`); closes the stream
  when the generator completes, or after sending a redirect/error (`L103-L106`, `L116-L124`);
  aborts the generator when the client disconnects (`L85`, `L129-L131`).

### `LiveQuery` state machine (`query-live/instance.svelte.js`)

- Awaiting a live query resolves with the **first** value; afterwards `await` resolves immediately
  with the current value (`L79-L84`, `L372-L387`).
- `#main` loop (`L120-L218`): open the iterator; on first `next()`:
  - stream closed without any value and never `ready` → `Error('Live query completed before yielding a value')`
    → terminal fail (`L146-L149`, `L177-L182`);
  - stream closed after values → `done = true`, fan-out closed, loop exits (`L150-L155`, `L163-L164`).
- Error policy (`L165-L207`):
  - aborted by us → exit;
  - `Redirect` → reset attempt counter and **reconnect immediately** (`L168-L175`);
  - not yet `ready` → any error is terminal (`#fail`) (`L177-L182`);
  - `HttpError` (server-sent error node, non-OK status, invalid message) after ready → terminal fail,
    keep last value (`L184-L188`);
  - anything else (network/transport) → keep last good value; if `navigator.onLine === false` stop
    and wait for the `online` event; else retry after `min(250·2^attempt, 10000)` ms ±20 % jitter
    (`L113-L117`, `L190-L204`).
- `connected` reflects an open stream; `attempt` resets on connect (`L136-L141`).
- Window listeners: `offline`/`pagehide`/`beforeunload` interrupt (abort fetch); `online` and
  bfcache `pageshow` restart if not `done` (`L220-L250`).
- `reconnect()` (called by the app, by `refreshAll()`/`invalidateAll`, by HMR, and by
  `remote_request` when the response contains `l[key]` without `e`) aborts the current stream,
  clears `done`, and resolves once the new stream **connects** (not once it yields)
  (`L353-L369`; `shared.svelte.js:L169-L183`; `client.js:L631-L639`).
- `fail()` is terminal: sets `done = true`, aborts, rejects the first-value promise or replaces the
  promise with a rejection, closes the fan-out (`L390-L412`). Only `reconnect()` revives it.
- Hydration seed (SSR-only): `query_responses[key]` `{v}` → `set(v)`; `{e}` → seeded failed but
  **still connects** (`L87-L106`).
- `for await (const v of live())` shares one connection; a new iterator first yields the current
  value; backpressure keeps only the latest pending value (`L266-L291`).

### `query.batch` protocol (`query-batch.svelte.js:L12-L96`)

- Request: `POST ${base}/${app_dir}/remote/${id}`, `Content-Type: application/json`,
  body `{"payloads": [p1, p2, ...]}` — distinct payloads gathered during one macrotask, in
  insertion order (`L22-L26`, `L38-L49`).
- Response: standard envelope; `data._` must be an array in the **same order** as `payloads`;
  element `{ type: 'error', error }` rejects that payload's promises with `HandledHttpError`, any
  other element resolves with `element.data` (`L66-L80`).
- `data.redirect` → `_goto(redirect)` and every promise resolves `undefined` (`L51-L64`).
- Transport-level failure (non-OK, `type:'error'` envelope, JSON/devalue parse failure) rejects all
  promises in the batch (`L81-L87`).
- The batch endpoint is reached through `QueryProxy`, so each payload is key-sorted like a plain
  query and cache keys are `id/payload`; `q` nodes in the response are applied too
  (`shared.svelte.js:L159-L166`) but the value the batch resolver uses is `_[i].data`.

## Citations

- `reference/kit/packages/kit/src/runtime/client/remote-functions/query-live/iterator.js:L19-L75`
- `reference/kit/packages/kit/src/runtime/client/remote-functions/query-live/instance.svelte.js:L75-L117`, `L120-L218`, `L220-L264`, `L266-L291`, `L353-L412`
- `reference/kit/packages/kit/src/runtime/client/remote-functions/query-live/proxy.js:L21-L35`, `L84-L93`
- `reference/kit/packages/kit/src/runtime/client/remote-functions/query-live/index.js:L13-L31`
- `reference/kit/packages/kit/src/runtime/client/remote-functions/shared.svelte.js:L169-L204`
- `reference/kit/packages/kit/src/runtime/client/sse.js:L1-L32`
- `reference/kit/packages/kit/src/runtime/client/stream.js:L9-L46`
- `reference/kit/packages/kit/src/runtime/client/ndjson.js:L1-L15`
- `reference/kit/packages/kit/src/runtime/client/remote-functions/query-batch.svelte.js:L12-L96`
- `reference/kit/packages/kit/src/runtime/client/client.js:L625-L639`, `L680-L690`
- `reference/kit/packages/kit/src/runtime/server/remote-functions.js:L27`, `L35-L140`, `L192-L206`
- `reference/kit/packages/kit/src/runtime/app/server/remote/query.js:L326-L357`

## Go implementation notes

- Live handler: only GET (405 otherwise). Write headers `Content-Type: text/event-stream`,
  `Cache-Control: private, no-store`, optionally `X-Accel-Buffering: no`; `http.Flusher.Flush()`
  after every frame. Frame = `"data: " + json + "\n\n"`. Keep-alive comment `": keep-alive\n\n"`
  every ≤30 s of silence (kit's own interval) to survive proxies. Stop when `r.Context()` is done.
- Message JSON: `{"type":"result","result":"<devalue of value>"}`. Dedupe by comparing the
  serialized string to the last sent one (kit semantics; optional but cheap).
- To end a stream cleanly: just close the response (generator completion). The client treats
  clean close as `done = true` and does **not** reconnect on its own; if the Go source is
  infinite, never close except on error.
- To redirect or error mid-stream: send `{"type":"redirect","location":...}` or
  `{"type":"error","error":{status,message}}` then close. Error is terminal on the client;
  redirect triggers navigation + immediate reconnect.
- If the request cannot be started (auth, validation of `payload`) respond
  `application/json` `{"type":"error","error":{...}}` — the client detects JSON content-type and
  fails terminally (or `{"type":"redirect","location"}`).
- Reconnect via command/form: put `l: { "<hash>/<name>/<payload>": { v: latest } }` in the
  mutation's `data`; the client sets the value and calls `reconnect()` (new GET). Go must expose a
  server-side "reconnect(liveQuery(arg))" that registers the key and computes the first value.
- Batch handler: decode `payloads[]`, run once with the argument slice, return per-item results in
  order; individual failures become `{type:'error', error}` items, not a whole-response error.
  Also emit `q` for each payload (`{ v }`) so the client cache and any later `refreshes` work
  identically to plain queries.

## Gotchas

- The SSE message uses `result` (a devalue string), not `data`; and `error.status` should be
  meaningful because it becomes `liveQuery.error.status`.
- An HTTP error status **before** the first value kills the live query permanently — there is no
  retry for non-transport errors; the app must call `reconnect()`.
- Transport errors (Go process restart, connection reset) are retried with backoff only if the
  client already had a value; a reset **during the first connection** is terminal.
- A `\r\n\r\n` delimiter or `event:` lines are fine but only `\n\n` splits blocks; do not rely on
  `event:`/`id:` — they are ignored.
- The client will happily process duplicate values; but each one re-triggers Svelte reactivity —
  dedupe on the Go side.
- Kit's server uses an `AbortSignal` to cancel the generator on disconnect; Go must watch
  `r.Context().Done()` in long-running producers or goroutines leak.
- `refreshAll()` / `invalidateAll` reconnect **every** live query (`client.js:L631-L639`), so a
  form submit without `r: true` reopens all live streams — set `r: true` when you already updated
  them via `l`.

## Recipes

- Go frame writer:
  ```go
  func writeEvent(w http.ResponseWriter, f http.Flusher, msg any) error {
      b, _ := json.Marshal(msg)
      if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil { return err }
      f.Flush(); return nil
  }
  ```
  with `msg = struct{Type string `json:"type"`; Result string `json:"result"`}{"result", devalueString}`.
- Keep-alive: `time.AfterFunc(30*time.Second, ...)` reset after every write, writing `": keep-alive\n\n"`.
- Batch response element structs:
  `type batchItem struct { Type string `json:"type"`; Data any `json:"data,omitempty"`; Error *AppError `json:"error,omitempty"` }` — serialize the whole `_` array via the devalue stringifier (it is inside `data`), not `encoding/json`.
