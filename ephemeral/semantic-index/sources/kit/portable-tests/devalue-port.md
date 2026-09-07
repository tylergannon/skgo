# devalue port — encoding model, fixtures, and what Go must support

## Purpose

devalue is the value codec under every kit wire format skgo must speak: `__data.json`
nodes and chunks, `/_app/remote/*` results, remote-function arguments (`?payload=`,
`payloads[]`, `refreshes[]` keys), and the binary form body header. This leaf
characterises the pinned clone (`reference/devalue`, 5.9.2). The Go codec that
answers to it is `github.com/tylergannon/polytype/devalue`, maintained in its own
module; skgo anchors it in `devalue_wire_test.go`.

## Key facts (SOURCE-DERIVED)

### Three entry points; kit's server uses two

- `stringify(value, reducers?, options?)` → reduced-JSON string; `parse(serialized, revivers?, options?)` = `unflatten(JSON.parse(serialized), ...)`
  (`reference/devalue/src/stringify.js:L20-L23`, `reference/devalue/src/parse.js:L20-L22`).
- `stringifyAsync` awaits thenables in place and produces identical output to `stringify`
  for non-promise input (`reference/devalue/src/stringify.js:L31-L62`; parity suite
  `reference/devalue/test/index.test.js:L1688-L1698`).
- `uneval(value, replacer?)` emits JavaScript source (`(function(a){...})`) — used only by
  the SSR HTML serializer (`reference/kit/packages/kit/src/runtime/server/page/data_serializer.js:L43,L69,L94`).
  skgo has no SSR: **`uneval` need not be ported.**
- kit wires devalue through `transport.js`: `stringify = devalue.stringify(data, encoders)`,
  `parse = devalue.parse(data, decoders)` (`reference/kit/packages/kit/src/runtime/app/internal/transport.js:L47-L52`).

### The reduced-JSON ("flattened") format

- Output is a JSON array of *entries*; entry 0 is the root. Every non-primitive (and every
  string, number, boolean, null) gets its own entry index; containers refer to children by
  index (`reference/devalue/src/stringify.js:L96-L128`). Repeated references share one
  index via an identity map (`indexes`, L76-L120).
- Top-level special primitives are emitted as a bare negative number, not an array:
  `stringify(undefined) === '-1'` (L410-L413; parse side L34).
- Sentinels (`reference/devalue/src/constants.js:L1-L7`): `UNDEFINED=-1`, `HOLE=-2`,
  `NAN=-3`, `POSITIVE_INFINITY=-4`, `NEGATIVE_INFINITY=-5`, `NEGATIVE_ZERO=-6`,
  `SPARSE=-7`. Max array length `2**32-1`, max index one less (L11-L12).
- Primitives inside entries (`stringify_primitive`, L421-L428): strings via
  `stringify_string`, `undefined` → `-1`, `-0` → `-6`, bigint → `["BigInt","<decimal>"]`,
  else `String(thing)` (JS number formatting).
- Numbers: NaN/±Infinity/-0 are detected before dedupe and returned as sentinel indices
  (L107-L113).
- Tagged entries (L158-L402):
  - Boxed `Number|String|Boolean|BigInt` → `["Object",<idx>]`
  - `Date` → `["Date","<ISO>"]`; invalid date → `["Date",""]` (`operations.js:L69`)
  - `URL`/`URLSearchParams` → `["URL","<toString>"]`
  - `RegExp` → `["RegExp","<source>"]` or `["RegExp","<source>","<flags>"]`
  - `Array` → JSON array of indices; holes → `-2`; **very sparse** → `[-7,length,idx,val,idx,val,...]`
    chosen when `(length-population)*3 > 4+d+population*(d+1)` where `d` = digits of length (L186-L269)
  - `Set` → `["Set",idx,...]`; `Map` → `["Map",k,v,k,v,...]` (L272-L296)
  - Typed arrays → `["<Tag>",<bufferIdx>]` plus `,byteOffset,length` when it is a subview;
    `DataView` uses `,byteOffset,byteLength` (L298-L332)
  - `ArrayBuffer` → `["ArrayBuffer","<base64>"]` (L334-L339) — standard alphabet with `=`
    padding (`reference/devalue/src/base64.js:L15-L23`, fixture `"QUFBQQ=="` at
    `reference/devalue/test/index.test.js:L308`)
  - `Temporal.*` → `["Temporal.<Kind>","<toString>"]` (L341-L350) — not needed for kit
  - Plain object → JSON object `{"key":idx,...}`; null-prototype object → `["null","k",idx,...]`
    (L352-L401). `__proto__` keys throw (L366-L373, L384-L391).
  - Custom reducers run **before** builtins for every non-undefined value; first reducer
    returning a truthy value wins: `["<key>",<idx of reduced value>]` (L122-L128). Note
    truthiness: a reducer returning `0`, `''`, `false` is treated as "not mine".
- Errors: functions, symbols, symbol keys, non-POJOs, promises in sync mode throw
  `DevalueError` with `.path` like `.foo.array[0]` / `.foo["string-key"].get("key")`
  (`reference/devalue/src/utils.js:L16-L30`, tests `index.test.js:L1272-L1407`).
- `is_plain_object`: proto is `Object.prototype`, `null`, a null-proto object, or a proto
  whose own property names equal `Object.prototype`'s (cross-realm) (`utils.js:L37-L51`).

### String escaping (differs from JSON.stringify)

- `stringify_string` (`reference/devalue/src/utils.js:L58-L102`) escapes `"`, `\`, `\n`,
  `\r`, `\t`, `\b`, `\f`, U+2028 and U+2029 as `\u2028`/`\u2029`, **`<` as `\u003C`**, and other control chars
  `< 0x20` as `\u00XX`. Everything else (including lone surrogates and `>`/`&`) is emitted
  raw. Fixtures: `reference/devalue/test/index.test.js:L434-L507` (strings) and
  `L855-L883` (XSS: `</script>` → `\u003C/script>`).

### Parse / hydrate rules (`reference/devalue/src/parse.js:L30-L274`)

- Input must be a number or a non-empty array, else `'Invalid input'` (L34-L38).
- Negative sentinels at any position map to undefined/NaN/±Inf/-0 (L56-L60); `HOLE` is
  only legal inside a dense array (`hydrate(-2)` standalone throws).
- Index out of range → `'Invalid input'` (L68-L70).
- Custom revivers: `[["Key", idx]]`; if the payload is not a number it is pushed as a new
  entry (builtin-reduced shape) (L80-L88); cycle guard `'Invalid circular reference'`
  (L100-L108) except when the target is already hydrated (L96-L98).
- Builtin tags: `Date` (L114-L116), `Set`/`Map` (L118-L132), `RegExp`, `Object` (boxed;
  guards against non-BigInt object payload → `'Invalid input'`, L138-L151), `BigInt`,
  `null` (rejects `__proto__` key, L157-L167), typed arrays/DataView (buffer entry must be
  `["ArrayBuffer",...]` else `'Invalid data'`, L169-L194), `ArrayBuffer` (non-string →
  `'Invalid ArrayBuffer encoding'`, L196-L203), `URL`/`URLSearchParams`/`Temporal.*`
  (L205-L218), unknown tag → `'Unknown type <tag>'` (L220-L221).
- Sparse arrays: validate `length` and each index (`is_valid_array_index`, `< len`); must
  not eagerly allocate `length` slots (DoS tests `index.test.js:L1607-L1681`); `__proto__`
  as an index string is rejected (`L1529-L1583`).
- Plain object: `__proto__` key → `'Cannot parse an object with a `__proto__` property'` (L261-L264).

### Fixture table structure (`reference/devalue/test/index.test.js:L26-L1049`)

Each fixture is `{ name, value, js, json, validate?, replacer?, reducers?, revivers? }`.
The same table drives four suites: `uneval` (compare to `js`), `stringify` (compare to
`json`), `parse(json)` and `unflatten(JSON.parse(json))` (compare to `value` or run
`validate`) (L1051-L1107). Categories and line ranges:

| category | lines | Go-relevant? |
|---|---|---|
| primitives | L27-L116 | yes — sentinel numbers, bigint, `-0` |
| boxed_primitives | L118-L197 | parse-only tolerance; Go never emits |
| basics | L200-L432 | Date, Array/sparse, Object, Set, Map, typed arrays, ArrayBuffer, DataView, URL, URLSearchParams; Temporal rows skip |
| strings | L434-L507 | yes — escaping; lone-surrogate rows need WTF-8/UTF-16 handling |
| cycles | L509-L608 | yes — reference model |
| repetition | L610-L853 | yes — shared indices incl. Map keys (`L795-L853`) |
| XSS | L855-L883 | yes — `<` escaping |
| misc | L885-L919 | null-proto, cross-realm, symbol keys |
| custom | L921-L952 | reducers/revivers; reducer object with polluted prototype (`Object.create({polluted:true})`) must only use own names |
| custom_fallback | L954-L972 | reducer overriding builtin `Date` |
| functions | L974-L1048 | JS-only (`new Function`) |

Invalid-input table: `L1109-L1269` (messages quoted above). Async suites `L1685-L1866`.
Custom-type cycles `L1868-L1937`.

Other test files: `reference/devalue/test/operations.test.js` and
`parse-operations.test.js` exercise the `operations` hook API (foreign-runtime handles,
side-effect-free introspection) — **JS-only, do not port**. `reference/devalue/src/base64.test.js:L5-L42`
round-trips 13 strings through three base64 implementations (portable as vectors).
`reference/devalue/src/utils.test.js:L7-L61` covers `valid_array_indices` (JS array
semantics, port only the index-string validator).

### How kit uses devalue on the wire (what Go MUST support)

- `__data.json` nodes: `{"type":"data","data":<stringify(node.data, reducers)>,"uses":{...}}`
  and chunk lines `{"type":"chunk","id":N,"data"|"error":<stringify>}\n`; the `Promise`
  reducer returns a numeric id, so a deferred value is encoded `["Promise",<idx>]` whose
  entry is the id (`reference/kit/packages/kit/src/runtime/server/page/data_serializer.js:L135-L182,L199-L214`).
- Remote results: `{"type":"result","data":<stringify({_:..., q:..., ...}, encoders)>}`
  (`reference/kit/packages/kit/src/runtime/server/remote-functions.js:L315-L321`).
- Remote args: `stringify(value, reducers)` with the `__skra*` reducers then url-safe base64
  (`reference/kit/packages/kit/src/runtime/shared.js:L96-L173,L252-L259,L311-L315`).
  Map/Set inside args are reduced to arrays of **nested devalue strings** (sorted), not
  native `["Map",...]`.
- Binary forms: `devalue.stringify([data, meta], { File: ... })` with File reduced to
  `[name, type, size, lastModified, index]` (`reference/kit/packages/kit/src/runtime/form-utils.js:L126-L133`)
  and parsed with a validating `File` reviver (L277-L309).
- User `transport` hooks: `encode` returning truthy → `["<Key>",<idx>]`; `decode` on parse.

Therefore the Go encoder needs: undefined/null distinction, numbers (incl. NaN/±Inf/-0 and
JS number formatting), strings, booleans, bigint, Date, plain objects with **insertion
order**, arrays (dense + HOLE + SPARSE), Map, Set, ArrayBuffer/Uint8Array (File payloads,
`Uint8Array` args), URL, RegExp (parse tolerance; args reject it), custom reducers/revivers,
and reference sharing/cycles. It does **not** need: uneval, Temporal, boxed primitives
(emit), DataView/other typed arrays beyond parse tolerance, `operations` hooks, symbols.

## Citations

- `reference/devalue/src/constants.js:L1-L12`
- `reference/devalue/src/stringify.js:L20-L62` (entry points), `L70-L128` (flatten/dedupe/reducers), `L158-L402` (tags), `L421-L428` (primitives)
- `reference/devalue/src/parse.js:L20-L38`, `L55-L271`
- `reference/devalue/src/utils.js:L16-L30`, `L37-L51`, `L58-L102`, `L118-L168`
- `reference/devalue/src/base64.js:L15-L23`, `L56-L60`
- `reference/devalue/src/operations.js:L54-L108` (default semantics: `tagOf` = `Object.prototype.toString` slice, `toISOString` `''` for invalid dates)
- `reference/devalue/test/index.test.js:L26-L1049`, `L1109-L1269`, `L1409-L1681`, `L1685-L1866`
- `reference/devalue/src/base64.test.js:L5-L42`; `reference/devalue/src/utils.test.js:L7-L61`
- kit wiring: `reference/kit/packages/kit/src/runtime/app/internal/transport.js:L32-L53`; `reference/kit/packages/kit/src/runtime/shared.js:L84-L173`

## Go port notes

- Package `github.com/tylergannon/polytype/devalue`. Value model: a small tagged `Value` type (or `any` with
  documented dynamic types) — `Undefined` sentinel, `nil` for null, `float64`, `string`,
  `bool`, `*big.Int`, `time.Time` (+ invalid flag), `*OrderedMap` (plain object,
  insertion-ordered, with `NullProto bool`), `[]Value` with hole markers (`Hole` sentinel)
  or a `SparseArray{Length; Entries []struct{Index; Value}}`, `*Map` (ordered entries,
  any key), `*Set`, `[]byte`/`ArrayBuffer`, `*url.URL`, `RegExp{Source, Flags}`, and
  `Custom{Key string; Payload Value}` for reducer output.
- Reducers: `[]struct{Key string; Fn func(Value) (Value, bool)}` in **declaration order**
  (JS iterates `Object.getOwnPropertyNames(reducers)`), tried before builtins.
- Test shape: convert the fixture table to a Go table by hand (or a one-off generator that
  reads `index.test.js`'s `json` strings — the `value` side must be re-expressed in Go
  constructors). Assert `Stringify(v) == json` and `Parse(json)` deep-equals `v`
  (custom equality that treats `-0`, NaN, holes, and Map/Set order explicitly). Port the
  invalid-input table verbatim as `(json, wantErr)` pairs.
- Number formatting: implement JS `Number.prototype.toString()` (shortest round-trip,
  exponent form for `>= 1e21` or `< 1e-6`, no `+` in `e21`? — JS prints `1e+21` and
  `1e-7`). `strconv.FormatFloat(f, 'g', -1, 64)` is **not** equivalent (`1e+06` vs
  `1000000`); write a dedicated formatter and test against the primitive fixtures plus
  `0.1`, `1e21`, `1e-7`, `123456789012345680000`.
- JSON emission: hand-roll the writer (entries are concatenated strings in JS); do not use
  `encoding/json` for the outer array because of key order and `<` escaping rules.

## Gotchas

- `undefined` vs `null`: `[null]` vs `-1`/`[-1]`; an object property that is `undefined`
  is still emitted (`{"a":-1}`). Go must distinguish "absent" (skip) from "undefined".
- `-0`: `math.Signbit(f) && f == 0`. JSON parse in Go yields `float64(0)` for `-0` on
  input, but devalue never emits `-0` as a JSON number, only as sentinel `-6`.
- Object key order: JS orders integer-like keys ascending first, then string keys by
  insertion. `Object.keys({b:1, 2:1, a:1})` → `["2","b","a"]`. An ordered map that
  preserves insertion order alone is wrong for numeric-string keys; either implement the
  JS ordering or document that Go-side data never uses numeric-string keys.
- Map/Set are insertion-ordered; Map keys may be objects (shared indices, fixture L795-L853).
- BigInt is a decimal string; Go must not parse `"1"` as float.
- Lone surrogates in fixtures (`L448-L482`) are raw UTF-16 code units; Go strings cannot
  hold them. For kit's wire use this is irrelevant (JSON.parse on the client would need the
  same raw surrogate). Either represent strings as `[]uint16` internally or skip those
  rows with a documented reason.
- Reducer truthiness: JS `if (value)`; `0`, `""`, `false`, `null`, `undefined` mean "no".
  Kit's own `transport` encoders rely on returning `undefined` to decline.
- Dense-vs-sparse heuristic is byte-cost based and must be reproduced exactly or the
  `result !==` dedupe in `query.live` and the remote-arg cache keys diverge from Node.
- `Date` output uses `toISOString()` (always UTC, millisecond precision, `Z` suffix);
  Go: `t.UTC().Format("2006-01-02T15:04:05.000Z")`. Invalid → `""` and parse of `""`
  must yield an invalid date, not an error.
- Reference dedupe is by JS identity; primitives (strings/numbers) are also deduped by
  value because the `indexes` Map keys them by value (`[[1,1],"a string"]`, L610-L616).
  Go must intern equal primitive strings/numbers/booleans/null to the same index too
  (`[null,null]` → `[[1,1],null]`, L618-L623).

## Recipes

- Round-trip test skeleton: `for _, tc := range fixtures { got := devalue.Stringify(tc.value, nil); if got != tc.json {...}; back, err := devalue.Parse(tc.json, nil); ... }`.
- To encode a remote-function result envelope: `devalue.Stringify(OrderedObject{"_": v}, transportEncoders)` and then JSON-encode the outer `{type,data}` with HTML escaping off.
- To emulate the `Promise` reducer for `__data.json` streaming: return `Custom{"Promise", Number(id)}` from a reducer for deferred values; emit chunk lines later.
