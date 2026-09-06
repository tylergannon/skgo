# The universal `transport` hook on the client: how `__data.json` and remote payloads are decoded

## Purpose

Kit 3 makes `transport` a *universal* hook (`src/hooks.ts`), so the client bundle carries the same encode/decode table the server used. Go produces `__data.json` (and, in the other worker's segment, remote-function payloads) without running that JavaScript. This leaf pins how the client wires `transport` into `devalue`, what the wire form of a custom type is, and what Go must therefore emit — or refuse to emit — for custom types.

## Key facts (source-derived)

### Where the transport table comes from

- Generated `app.js`: `hooks.transport = universal_hooks.transport || {}` (only if `src/hooks.{js,ts}` exists), and `decoders = Object.fromEntries(Object.entries(hooks.transport).map(([k, v]) => [k, v.decode]))`, `encoders = ...v.encode` (`reference/kit/packages/kit/src/core/sync/write_client_manifest.js:L157-167`). `app.decode = (type, value) => decoders[type](value)` (`L171`) is used only by SSR-inlined `app.decode('Name', ...)` calls.
- `_start` calls `init_transport(app.hooks.transport ?? {})` before `hooks.init` and before routing (`runtime/client/client.js:L520-524`). `init_transport` (`reference/kit/packages/kit/src/runtime/app/internal/transport.js:L32-53`) sets module-level `encoders`/`decoders`, `has_custom_transporters`, and three functions: `stringify = data => devalue.stringify(data, encoders)`, `parse = data => devalue.parse(data, decoders)`, `uneval = data => devalue.uneval(data, replacer)` where `replacer` tries each `transport[key].encode(thing)` in key order and emits `app.decode('<key>', <uneval(encoded)>)` for the first truthy result (`L38-45`). Before `init_transport` these throw (`L6-18`).
- `Transport` shape (from `@sveltejs/kit/hooks`): `Record<string, { encode(value: unknown): unknown /* falsy = not mine */, decode(encoded: unknown): unknown }>`.

### How `__data.json` uses it

- `process_stream` deserialises every `data` node and every `chunk` with `devalue.unflatten(data, { ...app.decoders, Promise: (id) => <deferred> })` (`client.js:L3710-3719`, `L3728-3733`, `L3742-3746`). `app.decoders` are the raw `decode` functions keyed by transport name.
- Wire form: `devalue.stringify(value, reducers)` on the server (`runtime/server/page/data_serializer.js:L135-182`, `L199-202`) produces, for a value claimed by reducer `Name`, the flattened element `["Name", <index>]` where `<index>` points to the flattened form of whatever `encode` returned. `devalue.unflatten`/`parse` with revivers calls `decoders["Name"](revivedValueAtIndex)`. The encoder's return value is itself recursively flattened, so it may contain nested custom types or built-ins (Date, Map, Set, BigInt, ...).
- Reducer order matters and "falsy means no": kit's server iterates `for (const key in encoders)` (insertion order of the hook object) and takes the first truthy `encoded` (`data_serializer.js:L66-72`; SSR `uneval` path identical in `transport.js:L38-45`). An `encode` that returns `0`, `''`, or `false` for a real value is treated as "not mine".
- Unknown reviver name on the client (e.g. Go emits `["Money", ...]` but the app's `transport` has no `Money`) → `devalue.unflatten` throws → the navigation fails with an error page (`load_route` catches into `handle_error`, `client.js:L1553-1595`).

### How remote-function payloads use it (cross-reference; owned by another segment)

- `remote_request` reads `response.json()` and, for `{ type: 'result', data }`, does `devalue.parse(result.data, app.decoders)` — note `data` is a **string** containing devalue JSON, nested inside the outer JSON (`reference/kit/packages/kit/src/runtime/client/remote-functions/shared.svelte.js:L113-136`). Errors: non-OK with `{ type: 'error', error }` → `HandledHttpError({ status, ...error })`; a 2xx `{ type: 'error', error }` → `HandledHttpError(error)` (`L120-132`).
- Live queries parse each streamed node's `result` with `devalue.parse(node.result, app.decoders)` (`runtime/client/remote-functions/query-live/iterator.js:L61`); prerender results are cached as `devalue.stringify(data, app.encoders)` (`prerender.svelte.js:L75`, `L92`, `L111`).
- Client → server arguments: `stringify_remote_arg` = `devalue.stringify(value, { ...encoders, __skrag, __skram, __skras, __skrao })` (sorted Map/Set/plain-object reducers for canonical cache keys) then URL-safe base64 without padding (`reference/kit/packages/kit/src/runtime/shared.js:L84-91`, `L96-173`, `L252-259`, `L311-315`); commands additionally encode `File` as `__skraf` via `stringifyAsync` (`L265-304`). The server side reverses with `parse_remote_arg` using `{ ...decoders, __skrao, __skram, __skras, __skraf }` (`L175-245`, `L321-331`). Custom-type *arguments* therefore arrive at Go as `["Name", idx]` too, and Go must be able to run `decode` semantics for them.

### Other client uses of `stringify`/`parse`

- `page.state` for history entries is round-tripped through `stringify`/`parse` (`client.js:L2132-2136`, `L2989-2990`, `L3394`), and navigation snapshots likewise (`runtime/client/snapshots.js:L44-46`, `L56-58`). Client-only; Go is not involved.

## Citations

- `reference/kit/packages/kit/src/core/sync/write_client_manifest.js:L157-171`
- `reference/kit/packages/kit/src/runtime/app/internal/transport.js:L1-53`
- `reference/kit/packages/kit/src/runtime/client/client.js:L520-524`, `L1553-1595`, `L2132-2136`, `L3698-3749`
- `reference/kit/packages/kit/src/runtime/client/remote-functions/shared.svelte.js:L113-136`
- `reference/kit/packages/kit/src/runtime/client/remote-functions/query-live/iterator.js:L61`
- `reference/kit/packages/kit/src/runtime/client/remote-functions/prerender.svelte.js:L75`, `L92`, `L111`
- `reference/kit/packages/kit/src/runtime/client/snapshots.js:L44-58`
- `reference/kit/packages/kit/src/runtime/shared.js:L84-91`, `L96-245`, `L252-331`
- `reference/kit/packages/kit/src/runtime/server/page/data_serializer.js:L66-72`, `L135-182`, `L199-202`
- `reference/kit/packages/kit/src/types/internal.d.ts:L171-176` (`ClientHooks`)
- `reference/devalue/package.json:L4`; `reference/devalue/src/constants.js:L1-6`

## Go implementation notes

1. **Go must know the transport table by name.** The adapter should extract the list of transport keys from `hooks.ts` at build time (static export analysis, or evaluate the module in Node during the build and print `Object.keys(transport)`), and fail the build if Go's registry does not have a codec for every key that Go-authored loads/remote functions can produce or receive. Key order must match the JS object's insertion order because the first matching encoder wins.
2. **Go codec interface**: `type Transport interface { Name() string; Encode(v any) (encoded any, ok bool); Decode(encoded any) (any, error) }` mapped onto devalue: when Go's devalue writer meets a value claimed by codec `Name`, it emits `["Name", idx]` and flattens `encoded` at `idx`; the reader does the inverse. Encoded values must be devalue-representable (plain objects/arrays/strings/numbers/Date/Map/Set/BigInt/etc.).
3. **Type-safety story**: for end-to-end types, generate the TS `transport` entries and the Go codecs from one source (e.g. a Go struct with a `//skgo:transport Name` marker generating both `encode/decode` TS and the Go codec). This is the only way to keep the two encoders' *claims* (which values they own) in sync — a value the TS side would encode but Go serialises as a plain object simply arrives as a plain object; nothing warns.
4. **Safer subset**: if a codec is not mirrored, Go loads should refuse (compile-time) to return values of that type instead of emitting a name the client cannot decode; unknown names are a hard client failure, not a degradation.
5. **Built-ins are free**: `Date`, `Map`, `Set`, `RegExp`, `BigInt`, `undefined`, `NaN`, `±Infinity`, `-0`, null-prototype objects, typed arrays (per pinned devalue 5.9.x) need no transport entry; Go's devalue writer should support at least Date/Map/Set/BigInt/undefined so common Go types (time.Time, map, big.Int) round-trip without custom transports.

## Gotchas

- `encode` returning a falsy value for a legitimately claimed value silently falls through to plain serialisation on the JS side; Go's `(encoded, ok)` return should be explicit so Go never has this ambiguity, but the *TS* side still has it — document it for app authors.
- The transport hook is applied at three different layers with three different devalue entry points: `stringify`/`parse` (flattened JSON, used for `__data.json` and remote payloads), `uneval` (JS expression, used only for SSR script injection and `/_app/env.js`), and `unflatten` (already-parsed JSON, used by the ndjson reader). Go only ever needs the flattened-JSON form.
- `app.decode(...)` calls exist only in SSR-inlined scripts; a CSR-only app never executes them, so Go does not need to emit them.
- Reviver functions receive the *fully revived* encoded value (nested Dates/Maps already reconstructed), not raw JSON — Go's `Decode` should receive the same.
- `Promise` is a reserved reducer/reviver name in `__data.json` (`client.js:L3713`); a transport key named `Promise` would be shadowed.
- devalue rejects functions, symbols, class instances without a reducer, and cyclic structures are fine (indices allow references) — Go's writer may emit shared references by index; the client reconstructs identity.

## Recipes

- **Build-time check**: adapter step `node -e "import('./src/hooks.ts').then(m => console.log(JSON.stringify(Object.keys(m.transport ?? {}))))"` (via Vite SSR load) → write `transport-keys.json` → `go generate` asserts each key has a registered Go codec.
- **Round-trip test**: for each codec, encode in Go, feed the flattened JSON to a Node script that runs `devalue.unflatten(json, decoders)`, assert deep-equality with the TS `decode` result; and the reverse for remote-function args (`stringify_remote_arg` in Node → `parse_remote_arg` in Go).
- **Minimal wire example**: Go load returns `{ when: time.Now() }` with no custom transport → `[{"when":1},["Date","2026-09-05T12:00:00.000Z"]]`; with a `Money` transport whose encode yields `[amount, currency]` → `[{"price":1},["Money",2],[3,4],1999,"USD"]`.
