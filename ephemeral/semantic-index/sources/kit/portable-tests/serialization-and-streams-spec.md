# Serialization and stream specs — remote args, binary forms, NDJSON/SSE framing

## Purpose

Collects every upstream unit test that pins how values and bytes cross the wire besides
devalue itself: remote-argument canonicalisation (`runtime/shared.spec.js`), base64 and
stream helpers (`runtime/utils.spec.js`), the binary form body (`runtime/form-utils.spec.js`),
the client-side frame parsers that define server framing (`client/ndjson.spec.js`,
`client/stream.spec.js`), chunk ordering (`utils/streaming.spec.js`,
`utils/shared-iterator.spec.js`), the prerender self-fetch contract
(`app/server/remote/prerender.spec.js`), and the SSR-only `serialize_data`/`load_data`
specs (recorded so nobody ports them by mistake).

## Key facts (SOURCE-DERIVED)

### Remote argument codec — `runtime/shared.spec.js` + `runtime/shared.js`

- Reducer keys ("sveltekit remote arg"): `__skrao` (object), `__skram` (Map), `__skras`
  (Set), `__skraf` (File), `__skrap` (promise guard), `__skrag` (regex guard)
  (`reference/kit/packages/kit/src/runtime/shared.js:L84-L91`).
- `stringify_remote_arg(value)`: `undefined` → `''`; else
  `devalue.stringify(value, create_remote_arg_reducers(true))` → url-safe base64 (strip
  `=`, `+`→`-`, `/`→`_`) (L252-L259, L311-L315). Must be "both a valid URL and a valid
  file name" (L248-L249) because prerendered remote results are written to disk.
- With `sort = true`: plain objects are cloned with keys sorted (`to_sorted`, L66-L82) and
  re-emitted as `["__skrao", <sorted clone>]`; a clone is marked with a symbol so the
  reducer declines it on the recursive pass (L155-L157); cycles are handled through the
  `clones` map (L159-L161). Map → `["__skram", [[stringify(k), stringify(v)], ...]]`
  sorted by (k, v) string (L111-L130); Set → `["__skras", [stringify(item)...].sort()]`
  (L133-L147). Nested `stringify` uses the **same** reducers (L170).
- RegExp anywhere throws `'Regular expressions are not valid remote function arguments'`
  (L100-L104); class instances fall through to devalue's
  `'Cannot stringify arbitrary non-POJOs'` (spec L211-L215).
- `stringify_command_arg` (async, `sort = false`): preserves key order; `File` reduced to
  `{ data: ArrayBuffer, lastModified, name, size, type }` via an allowed promise; any
  other promise throws `'Promises are not valid remote function arguments'` (L265-L304).
- `parse_remote_arg('')` → `undefined`; otherwise reverse the base64 mapping (no padding
  restore needed — `atob` tolerates it) and `devalue.parse(json, revivers)` where
  `__skrao` is identity, `__skram`/`__skras` re-parse each nested string, `__skraf` builds a
  `File` after strict field validation (L175-L245, L321-L331).
- Spec assertions (`reference/kit/packages/kit/src/runtime/shared.spec.js`):
  - reordered keys give identical strings: flat L37-L42, nested L44-L60, null-proto L62-L67,
    Map L69-L97, Set L99-L139, transported class instances inside Map/Set L141-L159
  - input not mutated L161-L174
  - cycles + repeated refs round-trip with sorted keys (`['a','z']`) L176-L190
  - Date / `Uint8Array` / `URL` round-trip L191-L203
  - RegExp rejection message L205-L209; non-POJO rejection L211-L215
  - sparse array `value[1_000_000] = {...}` round-trips with length `1_000_001` and `0 in parsed === false` L217-L226
  - command args: order preserved (different strings) L231-L235; `File` survives L238-L250
  - parse: `''` → undefined L253-L255; key order after parse is sorted L257-L272;
    Map keys ordered `['first','second']` and nested Sets sorted L274-L344; transport revivers
    inside Map/Set L346-L367; null-prototype restored L369-L381
- Keys: `create_remote_key(id, payload) = id + '/' + payload`; `split_remote_key` splits
  on the last `/`, throws `'Invalid remote key: <key>'` if none (L337-L356).

### base64 + stream helpers — `runtime/utils.spec.js`

- Vectors `'hello world'`, `''`, `'abcd'`, `'the quick brown fox jumps over the lazy dog'`,
  `'工欲善其事，必先利其器'` must equal Node `Buffer.toString('base64')` (standard alphabet,
  padded) both directions (`reference/kit/packages/kit/src/runtime/utils.spec.js:L4-L48`;
  subject `runtime/utils.js:L72-L110`).
- `stream_from_iterable` yields in order and cancelling the stream runs the iterable's
  `finally` (L50-L91). Go: an `io.Reader`/channel adapter whose consumer cancel triggers
  producer cleanup.

### Binary form body — `runtime/form-utils.spec.js` + `runtime/form-utils.js`

- Content type `application/x-sveltekit-formdata` (`form-utils.js:L105`). Layout
  (L108-L115): `1` byte version `0`, `u32 LE` header length, `u16 LE` offset-table length,
  header = `devalue.stringify([data, meta], { File })`, offset table = JSON array of file
  start offsets relative to the end of the table, then file bytes. Files are sorted
  smallest-first before offsets are assigned (L139-L148); the File reducer emits
  `[name, type, size, lastModified, index]` (L126-L133).
- Non-binary content types fall back to `request.formData()` + `convert_formdata(form_id, fd)`
  with `meta = {}` (L179-L182).
- Field-name grammar (`parse_form_key`, L26-L51): `<prefix?><path><[]?>/<form_id>`;
  prefix `n:` → number, `b:` → boolean; `[]` → array; wrong suffix throws
  `"Form contained a field that wasn't created with form.fields.as(...): <name>"`.
  Coercion (L58-L63): number `''` → `undefined` else `parseFloat`; boolean `=== 'on'`.
- `split_path` regex `^[a-zA-Z_$]\w*(\.[a-zA-Z_$]\w*|\[\d+\])*$` (L461-L472); spec good/bad
  cases `form-utils.spec.js:L23-L52` (`'[0]'`, `'foo.0'`, `'foo[bar]'` throw `Invalid path <p>`).
- `convert_formdata` (L73-L103): drops the phantom empty file (`name === '' && size === 0`,
  L83-L86), keeps real zero-byte files, errors on duplicated non-array keys, prototype
  pollution keys (`__proto__`, `constructor`, `prototype`) throw `Invalid key "<k>"`
  (L478-L485). Spec: L55-L157.
- `deserialize_binary_form` error strings (all wrapped as
  `SvelteKitError(400, 'Bad Request', 'Could not deserialize binary form: <msg>')`, L343-L345):
  `no body`, `too short`, `got version X, expected version 0`, `data too short`,
  `file offset table too short`, `invalid file offset table` (non-array / non-integer /
  negative), `invalid file metadata` (any of name/type string, size/lastModified/index
  number missing), `duplicate file offset table index`, `gaps in file data`,
  `overlapping file data` (spans sorted by offset then size, L311-L327). Validation uses the
  embedded lengths, not `Content-Length` (L245-L247). Spec cases: simple round trip
  L161-L186; 1-byte chunking L187-L245; `LazyFile` methods incl. `slice` L246-L277; with/without
  Content-Length L279-L329; no eager allocation for truncated bodies (`'data too short'`)
  L331-L401; crafted payload rejects (`invalid file metadata`) L403-L481; offset-table
  attacks L483-L553; `DataView` offset regression L556-L584; overlap/duplicate/gap/amplification
  L641-L748; zero-length files L750-L793.
- Raw payload builder for hand-crafted devalue headers: `build_raw_request` (spec L591-L610)
  and `build_raw_request_with_files` (L618-L639) — port these as Go test helpers.
- `deep_set` (L493-L528): own-property creation even for `toString` keys, array vs object
  intermediate detection, `DELETE_KEY` deletion; spec L796-L826. `normalize_issue` /
  `flatten_issues` (L534-L587) shape validation issues as `{ name, path, message, server }`
  and index them by `$` and every path prefix — no upstream test but wire-visible in form
  results.

### Client frame parsers (define server framing) — `client/ndjson.spec.js`, `client/stream.spec.js`

- `read_stream(reader, delimiter, options)` (`reference/kit/packages/kit/src/runtime/client/stream.js:L9-L46`):
  splits decoded text on `delimiter`, carries `delimiter.length - 1` trailing chars across
  chunks, yields a final partial frame if non-empty, never yields trailing empty frames.
  Spec (`stream.spec.js`): delimiter split across chunks (`'a\n' + '\nb'` with `'\n\n'` →
  `['a','b']`) L30-L34; UTF-8 code point split byte-by-byte L36-L41; 100 KiB frame L43-L53;
  short frames after long L55-L60; trailing frame L62-L65; malformed UTF-8 with
  `{ fatal: true }` throws `TypeError` L67-L70.
- `read_ndjson` = `read_stream(reader, '\n', { fatal: true })`, `trim()`, skip blank,
  `JSON.parse` (`ndjson.js:L8-L15`). Spec: split UTF-8 L33-L38; malformed UTF-8 rejects L40-L48.
- Server-side producer for `__data.json`
  (`reference/kit/packages/kit/src/runtime/server/page/data_serializer.js:L130-L217`):
  first line `{"type":"data","nodes":[<node>,...]}\n`; node = `null` |
  `{"type":"data","data":<devalue>,"uses":<json>[,"slash":<json>]}` |
  `JSON.stringify({type:'error',error})` | `{"type":"skip"}`; each deferred promise gets
  `{"type":"chunk","id":<n>,"data":<devalue>}\n` or `"error"` (L174); promise ids start at 1
  (L131). When no promises: plain JSON response (`data/index.js:L112-L116`); redirects are
  `{type:'redirect', status, location}` (`data/index.js:L162-L169`). Error node shape is
  `handle_error_and_jsonify` output (`errors.js:L44-L89`: `HttpError.body`, framework
  `{status, message: text}`, validation `{status:400, message:'Bad Request'}`, unknown
  `{status:500, message:'Internal Error'}` merged with `handleError` overrides).
- SSE for `query.live`: `data: <json>\n\n` frames and `: keep-alive\n\n` comments (see
  `remote-functions-spec.md`); the client presumably uses `read_stream(reader, '\n\n')`.

### Chunk ordering — `utils/streaming.spec.js`, `utils/shared-iterator.spec.js`

- `create_async_iterator` (`reference/kit/packages/kit/src/utils/streaming.js:L11-L39`):
  chunks are yielded in **resolution order**, not add order (`deferred[++resolved]`);
  `iterate` may grow while iterating; rejections propagate. Spec: fast consecutive
  resolutions L4-L16; rejection L18-L25 (`utils/streaming.spec.js`).
- `SharedIterator` (`utils/shared-iterator.js`): multi-subscriber broadcast with
  latest-wins backpressure, `start` on 0→1 and `stop` on →0, `done()`/`fail()` terminal
  states also affecting later subscribers, `return()` resolves pending `next()` as done,
  `throw()` rejects it. Spec `utils/shared-iterator.spec.js:L18-L178` (13 cases). Used by
  `query.live` fan-out; port as a Go broadcaster with a 1-slot latest-value mailbox per
  subscriber.

### Prerender wrapper self-fetch — `app/server/remote/prerender.spec.js`

- Fakes: `get_request_store` mocked to return `{ event: { request: { url }, isRemoteRequest: false, cookies: {} }, state: { remote: {}, prerendering: undefined, is_in_remote_query: false } }`,
  global `fetch` stubbed, wrapper `__.id = 'hash/fn'` (L30-L55).
- `{ type: 'error', error: { status: 418, message: 'teapot' } }` with HTTP 200 →
  rejects with `HandledHttpError` (status 418, body as-is), function **not** called, and
  `handle_error_and_jsonify` returns the body untouched without invoking `handleError`
  (L57-L83).
- `ValidationError(issues)` → `handleError` called with `{ kind: 'validation', error: { status: 400, message: 'Bad Request' }, issues, event }`; transformed is `{ status: 400, message: 'Bad Request' }` (issues not exposed) (L85-L104).
- `{ type: 'result', data: stringify({ _: 'prerendered' }) }` → resolves `'prerendered'`,
  function not called (L106-L116).
- Fetch rejection, non-ok status, or non-JSON body → falls back to running the function
  (L118-L137).

### SSR-only specs (do not port; recorded for completeness)

- `runtime/server/page/serialize_data.spec.js:L4-L103`: inlines fetched responses as
  `<script type="application/json" data-sveltekit-fetched data-url="…" [data-ttl="…"]>{"status":200,"statusText":"","headers":{},"body":"…"}</script>`
  with `</script>` → `\u003C/script>` and `<!--` → `\u003C!--`; ttl = `max-age - age`,
  disabled when `vary: *`. Only meaningful when rendering HTML.
- `runtime/server/page/load_data.spec.js:L30-L91`: `create_universal_fetch` CORS emulation
  (`mode: 'no-cors'` empties body, missing ACAO on cross-origin throws
  `"CORS error: No 'Access-Control-Allow-Origin' header is present on the requested resource"`,
  non-serialised headers throw). Applies to universal `load` running on the server during
  SSR; skgo has none.
- `runtime/server/page/crypto.spec.js:L6-L22`: `sha256(text)` → base64 digest (CSP hashes).
  Go stdlib; trivial if ever needed.
- `runtime/server/page/csp.spec.js` (587 lines, 22 tests, headings at L11-L565): CSP
  header/meta generation with nonces and hashes — HTML rendering only.

## Citations

- `reference/kit/packages/kit/src/runtime/shared.spec.js:L32-L383`; `reference/kit/packages/kit/src/runtime/shared.js:L84-L356`
- `reference/kit/packages/kit/src/runtime/utils.spec.js:L4-L91`; `reference/kit/packages/kit/src/runtime/utils.js:L72-L110`
- `reference/kit/packages/kit/src/runtime/form-utils.spec.js:L23-L852`; `reference/kit/packages/kit/src/runtime/form-utils.js:L26-L103`, `L105-L345`, `L461-L528`, `L534-L587`
- `reference/kit/packages/kit/src/runtime/client/stream.spec.js:L30-L70`; `reference/kit/packages/kit/src/runtime/client/stream.js:L9-L46`
- `reference/kit/packages/kit/src/runtime/client/ndjson.spec.js:L33-L48`; `reference/kit/packages/kit/src/runtime/client/ndjson.js:L8-L15`
- `reference/kit/packages/kit/src/runtime/server/page/data_serializer.js:L130-L217`; `reference/kit/packages/kit/src/runtime/server/data/index.js:L40-L169`
- `reference/kit/packages/kit/src/utils/streaming.spec.js:L4-L25`; `reference/kit/packages/kit/src/utils/streaming.js:L11-L39`
- `reference/kit/packages/kit/src/utils/shared-iterator.spec.js:L18-L178`
- `reference/kit/packages/kit/src/runtime/app/server/remote/prerender.spec.js:L30-L137`
- `reference/kit/packages/kit/src/runtime/server/errors.js:L44-L89`
- SSR-only: `reference/kit/packages/kit/src/runtime/server/page/serialize_data.spec.js:L4-L103`, `load_data.spec.js:L30-L91`, `crypto.spec.js:L6-L22`, `csp.spec.js:L11-L565`

## Go port notes

- `internal/remote/args`: `StringifyRemoteArg(v) string`, `ParseRemoteArg(s) (Value, error)`,
  `CreateRemoteKey`, `SplitRemoteKey`. Port every `shared.spec.js` case as a table test;
  equality of strings is the assertion, so Go must sort keys, sort Map entries by
  `(stringify(k), stringify(v))` string order (JS `<` on UTF-16 code units — for ASCII
  payloads Go byte order matches), and sort Set items by string.
- `internal/form/binary`: `Serialize(data, meta) []byte` (needed only for tests) and
  `Deserialize(r *http.Request, formID string) (data, meta, formData, error)`; port the raw
  request builders and every error-string case. Model `LazyFile` as an `io.ReadSeeker`
  backed by the body buffer.
- `internal/stream`: `FrameScanner(delim string)` mirroring `read_stream` (port all six
  cases with byte-sliced inputs; use `unicode/utf8` validation for the fatal case);
  `NDJSONWriter` producing the exact `__data.json` lines; `AsyncIterator` = resolution-order
  chunk queue; `Broadcaster` = `SharedIterator`.
- `internal/remote/prerender`: port the five self-fetch cases with an `http.RoundTripper`
  fake; `HandledHttpError` maps to a Go error type whose body bypasses `handleError`.

## Gotchas

- Remote-arg Map/Set are encoded as arrays of nested devalue **strings**, so a Go value
  containing a Map must be serialised recursively (string within JSON within base64).
- Sorting compares JS strings (UTF-16 code unit order); Go string comparison is byte order.
  They agree for BMP-only content except across surrogate pairs vs. U+E000–U+FFFF. Document
  or implement a UTF-16 comparator.
- `parse_remote_arg` accepts unpadded base64; Go must use `base64.RawURLEncoding` (or strip
  padding manually) and tolerate both.
- `sparse` arrays in args must round-trip `length` without allocating a million slots.
- `convert_formdata` iterates `data.keys()` which yields duplicates for repeated fields;
  `getAll` dedupes. Go's `url.Values`/`multipart.Form` maps lose order — the resulting
  object key order matters only for devalue string equality of form *results*, not inputs.
- `read_stream` carries `delimiter.length - 1` characters (not bytes) across chunk
  boundaries; a byte-oriented Go scanner must decode UTF-8 first or carry bytes carefully.
- `__data.json` error nodes are `JSON.stringify(node)` (not devalue) — the `error` object
  is plain JSON.

## Recipes

- Canonical remote key for a query call: `key := id + "/" + StringifyRemoteArg(arg)`; the
  client sends these back verbatim in `refreshes`.
- Emit a `__data.json` stream: write `{"type":"data","nodes":[...]}\n`, then for each
  deferred promise in resolution order `{"type":"chunk","id":N,"data":<devalue>}\n`.
- Hand-craft a binary form body in a Go test: `[0x00] + u32le(len(header)) + u16le(len(offsets)) + header + offsets + files`.
