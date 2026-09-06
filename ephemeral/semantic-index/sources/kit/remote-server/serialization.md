# Serialization on the remote-function wire: devalue, transport, `__skra*` (kit 3.0.0-next.25 + devalue 5.9.2, pinned source)

**Purpose.** Everything a Go encoder/decoder must reproduce: devalue's flattened JSON
format, kit's `transport` hook integration, the remote-argument reducers/revivers
(`__skrao` etc.), base64url details, and which of `stringify`/`uneval`/`parse` is used
where.

All facts below are SOURCE-DERIVED from the pinned clones under the token cache root
(`reference/kit/...` and `reference/devalue/...`, version 5.9.2; kit pins `^5.9.0`
`reference/kit/packages/kit/package.json:L26`).

## Key facts

### Where each primitive is used

- `stringify` (= `devalue.stringify(data, encoders)`) — every remote RESPONSE `data`
  string, every live `result` frame, prerendered file bodies.
  `reference/kit/packages/kit/src/runtime/app/internal/transport.js:L51`,
  `reference/kit/packages/kit/src/runtime/server/remote-functions.js:L108,L270,L318,L329`.
- `parse` (= `devalue.parse(data, decoders)`) — server side only when SSR fetches a
  prerendered file (`remote/prerender.js:L119`); the CLIENT uses
  `devalue.parse(result.data, app.decoders)` on every response
  (`reference/kit/packages/kit/src/runtime/client/remote-functions/shared.svelte.js:L135-L137`).
- `uneval` — ONLY for inlining data into SSR HTML as JS source (`app.decode('key', ...)`
  calls). Never on the remote wire. `transport.js:L37-L45,L50`. **skgo can ignore `uneval`.**
- Remote ARGUMENTS use separate helpers in `reference/kit/packages/kit/src/runtime/shared.js`:
  `stringify_remote_arg` (query/query.live/query.batch/prerender; client and server),
  `stringify_command_arg` (command; client), `parse_remote_arg` (server).

### Transport hook

- `transport` is exported from the universal `hooks.ts` as
  `{ [key]: { encode(value) => encoded | falsy, decode(encoded) => value } }`;
  `init_transport` derives `encoders`/`decoders` maps keyed by `key`.
  `transport.js:L32-L53`; wired in `reference/kit/packages/kit/src/runtime/server/index.js:L161`.
- In devalue, custom reducers run BEFORE built-in tags for every non-primitive value,
  in `Object.getOwnPropertyNames(reducers)` order; the first that returns truthy wins
  and the value is emitted as `["<key>", <index-of-flattened-encoded-value>]`.
  `reference/devalue/src/stringify.js:L81-L85,L120-L126`.
- On parse, an array whose first element is a string naming a reviver is revived with
  `reviver(hydrate(value[1]))`; if `value[1]` is not a number it is pushed as a new slot
  first (`reference/devalue/src/parse.js:L77-L112`). Built-in tags (`Date`, `Map`,
  ...) are handled only if no custom reviver has that key.

### devalue flattened format (`stringify`)

`reference/devalue/src/stringify.js:L20-L23,L70-L410`, `reference/devalue/src/constants.js:L1-L7`

- Output is either a bare negative-number string for special root primitives or a
  JSON array `[slot0, slot1, ...]` where slot 0 is the root and every nested
  non-primitive is referenced by slot index (dedupe by identity; cycles allowed).
- Sentinels (used as "index" values): `-1` undefined, `-2` hole, `-3` NaN, `-4`
  +Infinity, `-5` -Infinity, `-6` -0, `-7` sparse-array marker.
- Primitives in slots: JSON strings (escaped: `"`, `\\`, `\n`, `\r`, `\t`, `\b`,
  `\f`, U+2028 as `\u2028`, U+2029 as `\u2029`, control chars as `\u00XX`, and **`<` as `\u003C`**),
  numbers, booleans, `null`. `reference/devalue/src/utils.js:L59-L102`.
- Tagged arrays: `["Object", i]` boxed primitive; `["Date","<ISO>"]`;
  `["URL","..."]`; `["URLSearchParams","..."]`; `["RegExp",src[,flags]]`;
  `["BigInt","123"]`; `["Set", i, i, ...]`; `["Map", k, v, k, v, ...]`;
  `["null", "key", i, ...]` null-prototype objects; `["<TypedArray>", bufIdx[, byteOffset, length]]`;
  `["DataView", bufIdx[, byteOffset, byteLength]]`; `["ArrayBuffer","<std base64>"]`;
  `["Temporal.*","<string>"]`. Plain objects are JSON objects `{"k": i, ...}` with
  index values; `__proto__` keys and symbol keys throw; non-POJOs throw
  `Cannot stringify arbitrary non-POJOs`. `stringify.js:L165-L400`.
- Arrays: `[i, i, ...]`; holes as `-2`, or `[-7, length, idx, i, idx, i, ...]` when
  sparse encoding is cheaper. `stringify.js:L182-L278`.
- Functions, symbols and (in sync mode) promises throw. `stringify.js:L127-L145`.
- `stringifyAsync` awaits thenables and writes the resolved value into the same slot;
  kit uses it only for command args (File bodies). `stringify.js:L31-L62`.

### Remote-argument encoding (`stringify_remote_arg`, sort = true)

`reference/kit/packages/kit/src/runtime/shared.js:L84-L173,L252-L259`

- `undefined` -> empty string (no `?payload`, no path segment).
- Reducers = `{ ...transport encoders, __skrag, __skram, __skras, __skrao }` in that
  key order (encoders first, then the remote ones as declared: `__skrag` is declared
  first inside `remote_fns_reducers`, then map/set/object when `sort`).
  - `__skrag` (RegExp guard): throws `Regular expressions are not valid remote function arguments`; never emits. `L100-L104`.
  - `__skram` (Map): emits `["__skram", i]` where slot i is an array of
    `[stringify(key), stringify(value)]` STRING pairs (each a complete nested devalue
    document using the same reducers), sorted lexicographically by key then value. `L111-L130`.
  - `__skras` (Set): `["__skras", i]` where slot i is a sorted array of
    `stringify(item)` strings. `L133-L147`.
  - `__skrao` (plain object): `["__skrao", i]` where slot i is a clone of the object
    with keys in sorted order (recursively; the clone is marked with a symbol so the
    reducer skips it and devalue serializes it as a normal `{}` slot). Same-identity
    objects reuse the clone (dedupe). `L62-L82,L150-L164`.
- Then `url_friendly_base64_encode(json)`: UTF-8 bytes -> standard base64 -> strip
  `=` -> `+`->`-`, `/`->`_`. `L311-L315`. Result is used verbatim as the cache key
  and as `?payload=` / path segment — **canonical form matters**: identical args must
  produce identical strings on client and server (server-side `refresh()` keys are
  computed by the server with this same function).

### Command-argument encoding (`stringify_command_arg`, sort = false)

`reference/kit/packages/kit/src/runtime/shared.js:L265-L304`

- Reducers = `{ ...encoders, __skrag, __skraf, __skrap }` — NO `__skrao/__skram/__skras`,
  so plain objects/Map/Set use devalue built-ins (`{}`/`["Map",...]`/`["Set",...]`),
  unsorted.
- `__skraf` (File): reducer returns a Promise of `{ data: ArrayBuffer, lastModified,
  name, size, type }`; `stringifyAsync` resolves it in place, so the wire is
  `["__skraf", i]` with slot i `{ "data": j, "lastModified": n, "name": k, "size": n, "type": m }`
  and slot j `["ArrayBuffer","<std base64>"]`.
- `__skrap` (promise guard): any other Promise throws `Promises are not valid remote function arguments`.
- Same base64url wrapping; sent as JSON `{ "payload": "<b64url>", "refreshes": [...] }`.

### Server-side revival (`parse_remote_arg`)

`reference/kit/packages/kit/src/runtime/shared.js:L175-L245,L321-L331`

- base64url -> base64 (`-`->`+`, `_`->`/`; padding not required), UTF-8 decode,
  `devalue.parse(json, { ...decoders, __skrao: identity, __skram, __skras, __skraf })`.
- `__skram` reviver expects an array of `[string,string]` pairs and parses each string
  recursively with the same revivers (`Invalid data for Map reviver` otherwise);
  `__skras` expects an array of strings; `__skraf` expects
  `{name:string,type:string,size:number,lastModified:number,data:ArrayBuffer}` and
  builds a `File`. There is no `__skrag`/`__skrap` reviver (they never emit).
- The same revivers are used regardless of which kind (query vs command) sent the
  payload, so a Go decoder must accept BOTH the `__skram/__skras` forms and the
  built-in `["Map"...]/["Set"...]` forms.

### Response encoding

- `Response.json({ type:'result', data: devalue.stringify(data, encoders) })` —
  `data` is a devalue document nested as a JSON string. For a query returning
  `{ id: 1, tags: ['a'] }` the body is
  `{"type":"result","data":"[{\"_\":1},{\"id\":2,\"tags\":3},1,[4],\"a\"]"}`.
  The client parses with `app.decoders` (transport `decode` functions).
- Batch entries, `q`/`l`/`p`/`f` nodes and `redirect` are ordinary fields inside the
  same devalue document (see request-handling.md).
- Binary form headers use devalue with ONLY a `File` reducer/reviver
  (form-and-prerender.md) — transport encoders are not applied there.

## Citations

- `reference/kit/packages/kit/src/runtime/shared.js:L84-L356` — all remote arg reducers/revivers, base64url, remote keys.
- `reference/kit/packages/kit/src/runtime/app/internal/transport.js:L32-L53` — transport wiring.
- `reference/kit/packages/kit/src/runtime/utils.js:L72-L110` — base64 encode/decode helpers.
- `reference/devalue/src/stringify.js:L20-L62,L70-L410` — flattened format.
- `reference/devalue/src/parse.js:L20-L112` — reviver dispatch.
- `reference/devalue/src/constants.js:L1-L7` — sentinels.
- `reference/devalue/src/utils.js:L59-L116` — string escaping, key rules.

## Go implementation notes

- Implement a devalue codec in Go: decoder (JSON array -> value graph with index
  references, sentinels, built-in tags, custom revivers map) and encoder (value ->
  flattened array with identity dedupe, `<` escaping, sentinels for
  NaN/±Inf/-0/undefined). Keep encoder output JSON-canonical enough that the client's
  `JSON.parse` + `unflatten` accepts it — slot order is free as long as indexes match.
- Provide kit's revivers: `__skrao` = identity, `__skram`/`__skras` = nested-document
  parse, `__skraf` = file struct with bytes, plus built-in `Map`/`Set`/`Date`/etc.
  For TS-typed Go remotes, map Map/Set to Go maps/slices as the generator sees fit.
- Provide `stringify_remote_arg` in Go (sorted-object clone, `__skrao/__skram/__skras`
  tags, base64url without padding) so Go-side `refresh()` of a query with a given arg
  produces the exact client cache key. When in doubt, prefer replaying the payload
  strings the client sent in `refreshes` verbatim rather than re-encoding.
- Transport hooks: if the app declares `transport` keys, the Go codec needs a matching
  encoder/decoder per key emitting/consuming `["<key>", i]`. Without a Go
  counterpart, such values cannot cross the boundary — document this as a generator
  constraint.
- Ignore: `uneval`, `has_custom_transporters`, `stringifyAsync` mechanics (Go can
  just read the file bytes synchronously).

## Gotchas

- `<` is escaped as `\u003C` (uppercase C) in devalue strings; a Go `encoding/json` encoder by
  default escapes `<`, `>`, `&` as `\u003c` (lowercase hex) — both parse identically
  in JS, but byte-for-byte fidelity requires custom escaping if you compare outputs.
- Sorting for Map/Set entries compares the devalue STRINGS (JS `<` on UTF-16), not the
  values; Go must sort by the encoded string using UTF-16 code-unit order to be
  identical (differs from byte order only for surrogate-pair characters).
- Query payloads reject RegExp and Promise; command payloads reject Promise but allow
  File. Query payloads cannot carry File (no `__skraf` reducer; File is a non-POJO and
  throws).
- `parse_remote_arg('')` is `undefined`, and validators without a schema reject any
  non-undefined arg with 400.
- `base64_decode` uses `Buffer.from(str,'base64')` on Node, which silently ignores
  invalid characters and missing padding; a strict Go decoder should use
  `base64.RawURLEncoding` and also tolerate padded input.
- Sparse arrays (`-7`) and holes (`-2`) are legal on the wire; decode them to slices
  with zero values (or reject) but don't crash.
- kit 2 `remote_object` etc. used the same `__skra*` names; the `__skrag`/`__skrap`
  guards and sorted `__skram/__skras` are present in the pinned build — verify against
  the pinned source, not older docs.

## Recipes

- To write the Go arg decoder: read `runtime/shared.js:L175-L245,L321-L331` then `devalue/src/parse.js:L20-L112`.
- To write the Go response encoder: `devalue/src/stringify.js:L70-L410` and `utils.js:L59-L102`.
- To write the Go canonical arg encoder (for server-side refresh keys): `runtime/shared.js:L62-L173,L252-L259,L311-L315`.
- To support `transport`: `transport.js:L32-L53` and `devalue/src/stringify.js:L120-L126`.
