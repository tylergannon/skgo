# devalue 5.9.2 — wire format and Go-port semantics

Pinned source: `reference/devalue/` (package.json `"version": "5.9.2"`, commit
a3d30d9, 2026-08-27). Kit uses `stringify`/`parse`/`unflatten` for every
`__data.json` payload and remote-function response, and `uneval` for data
inlined into SSR HTML. This leaf covers the **implementation semantics** of
the format so that skgo can produce byte-compatible payloads from Go and parse
what kit's client sends back. The devalue *test suite* mapping is a separate
leaf (another worker).

## Purpose

Give a Go implementer the complete format spec of `devalue.stringify` output
(what kit's client `parse`s) and of `devalue.parse` input validation (what
kit's server accepts from clients), plus a concrete Go type mapping and a
list of the JS semantics that Go must fake.

## Key facts (SOURCE-DERIVED)

### Top-level shape

- Public API: `uneval`, `parse`, `unflatten`, `stringify`, `stringifyAsync`,
  `defaultStringifyOperations`, `defaultParseOperations`, `DevalueError`,
  `filterArrayIndices`. `reference/devalue/index.js:L1-L8`.
- `stringify(value, reducers?, options?)` returns either a **bare negative
  integer string** (when the root is one of the sentinel primitives) or a JSON
  array `[v0,v1,...]` whose element 0 is the root. `reference/devalue/src/stringify.js:L20-L23`, `:L409-L414`.
- Every non-sentinel value gets an **index** into that array, assigned in
  pre-order (`index ??= p++`) the first time it is seen.
  `reference/devalue/src/stringify.js:L115-L120`. Inside containers, children
  are referenced **only by index number**, never inlined — so
  `{message:'hello'}` becomes `[{"message":1},"hello"]` (README
  `reference/devalue/README.md:L61`). Even a scalar string child is its own
  array slot.
- Dedup/cycles: `indexes: Map<identity, index>`; on a repeat identity the
  existing index is returned immediately. Map keys use SameValueZero, and
  **primitives are deduped too** (`identify` default is the value itself), so
  two equal strings/numbers share one slot. `reference/devalue/src/stringify.js:L76-L77`, `:L115-L117`, `reference/devalue/src/operations.js:L55`.
  Self-reference: `{self: obj}` → `[{"message":1,"self":0},"hello"]`
  (`reference/devalue/README.md:L66`).

### Sentinel (negative) indices — `reference/devalue/src/constants.js:L1-L7`

| const | value | meaning |
|---|---|---|
| `UNDEFINED` | `-1` | `undefined` |
| `HOLE` | `-2` | array hole (only valid inside a dense array literal) |
| `NAN` | `-3` | `NaN` |
| `POSITIVE_INFINITY` | `-4` | `Infinity` |
| `NEGATIVE_INFINITY` | `-5` | `-Infinity` |
| `NEGATIVE_ZERO` | `-6` | `-0` |
| `SPARSE` | `-7` | sparse-array marker, first element of an array literal |

- `MAX_ARRAY_LEN = 2**32-1`, `MAX_ARRAY_INDEX = MAX_ARRAY_LEN-1`
  (`reference/devalue/src/constants.js:L11-L12`), used to validate sparse
  arrays on parse.
- Sentinel detection on stringify: `type === 'undefined'` → `-1`; numbers:
  `Number.isNaN` → `-3`, `=== Infinity` → `-4`, `=== -Infinity` → `-5`,
  `number === 0 && 1/number < 0` → `-6`. `reference/devalue/src/stringify.js:L99-L113`.
  These checks happen **before** identity/dedup, so sentinels never occupy a
  slot.
- Root sentinel: `stringify(undefined)` returns the string `"-1"`, not
  `"[-1]"`. `reference/devalue/src/stringify.js:L412`. Parse mirrors this:
  `unflatten(number)` → `hydrate(n, standalone=true)`, which accepts only the
  sentinel numbers and throws `Invalid input` for any other number.
  `reference/devalue/src/parse.js:L34`, `:L62-L64`.

### Per-type encodings (stringify) — `reference/devalue/src/stringify.js:L156-L402`

Slot contents by `tagOf(thing)` = `Object.prototype.toString.call(x).slice(8,-1)`
(`reference/devalue/src/utils.js:L54-L56`):

- **Primitives** (string/number/boolean/bigint/null) via `stringify_primitive`
  (`:L421-L428`): string → `stringify_string` (custom escaper, see below);
  `undefined` → `-1` (unreachable here, handled earlier); `-0` → `-6`;
  bigint → `["BigInt","<decimal>"]`; everything else `String(thing)` — so
  numbers use JS `Number#toString` formatting (shortest round-trip, `1e21`
  style exponents, no trailing `.0`), booleans `true`/`false`, `null`.
- **Boxed primitives** `Number|String|Boolean|BigInt` → `["Object",<idx>]`
  where `<idx>` is the slot of the unboxed value. `:L159-L163`.
- **Date** → `["Date","<ISO>"]` where ISO is `date.toISOString()`; an
  **invalid date serializes as `["Date",""]`**
  (`reference/devalue/src/operations.js:L69`). Note the ISO string is NOT
  passed through `stringify_string` — it is interpolated raw inside quotes
  (safe because ISO output is `[0-9TZ:.+-]`).
- **URL** → `["URL","<href>"]`, **URLSearchParams** →
  `["URLSearchParams","<string>"]` (string escaped). `:L170-L176`.
- **RegExp** → `["RegExp","<source>","<flags>"]`, or 2-element
  `["RegExp","<source>"]` when flags is empty. `:L178-L183`.
- **Array** (dense) → JSON array of child indexes, e.g. `[1,2,-2,3]`; holes
  become `-2`. `:L185-L269`.
  - **Sparse encoding** `[-7,<length>,<idx>,<slot>,<idx>,<slot>,...]`
    chosen when `hole_cost = (L-P)*3 > sparse_cost = 4 + d + P*(d+1)`
    (L=length, P=populated count, d=digits in L), evaluated at the **first
    hole** encountered; if HOLE encoding wins it is locked in
    (`mostly_dense = true`). `:L211-L263`. Populated indices come from
    `Object.keys(array)` filtered to the leading run of canonical array-index
    strings (`reference/devalue/src/utils.js:L134-L168`).
- **Set** → `["Set",<slot>,<slot>,...]` in insertion order. `:L271-L279`.
- **Map** → `["Map",<kslot>,<vslot>,<kslot>,<vslot>,...]` in insertion order;
  keys can be any value (objects included). `:L281-L296`.
- **Typed arrays** (`Int8Array Uint8Array Uint8ClampedArray Int16Array
  Uint16Array Float16Array Int32Array Uint32Array Float32Array Float64Array
  BigInt64Array BigUint64Array`) → `["<Tag>",<bufslot>]` for a full-buffer
  view, or `["<Tag>",<bufslot>,<byteOffset>,<length>]` when
  `byteLength !== buffer.byteLength` (`length` = element count). `:L298-L320`.
  The buffer is its own slot, so two views over one buffer share it.
- **DataView** → same, but the third number is `byteLength`. `:L322-L332`.
- **ArrayBuffer** → `["ArrayBuffer","<base64>"]`. `:L334-L339`.
- **Temporal.\*** (8 tags) → `["Temporal.Instant","<toString()>"]` etc. `:L341-L350`.
- **Plain object** → JSON object `{"key":<slot>,...}` with keys in
  `Object.keys` order (integer-like keys ascending first, then insertion
  order — JS OrdinaryOwnPropertyKeys). `:L380-L399`.
- **Null-prototype object** → `["null","k1",<slot>,"k2",<slot>,...]`. `:L363-L379`.
- **Custom reducer** → `["<name>",<slot>]` where `<slot>` is the slot of the
  reducer's return value; reducers are tried **before** builtin tags, in
  `Object.getOwnPropertyNames(reducers)` order, and the first **truthy**
  return wins. `:L81-L85`, `:L122-L128`. The reducer sees the raw value
  (also functions and symbols are offered to reducers before throwing,
  `:L130-L134`; CHANGELOG 5.5.0).
- **Thrown** (`DevalueError` with `.path`, `.value`, `.root`):
  functions, symbols, non-POJOs (`is_plain_object` false), POJOs with
  enumerable symbol keys, any key named `__proto__`, and Promises unless
  `stringifyAsync`. `:L130-L134`, `:L141-L149`, `:L355-L360`, `:L366-L373`,
  `:L384-L390`. `is_plain_object`: prototype is `Object.prototype`, `null`,
  a proto whose proto is null, or a proto with exactly `Object.prototype`'s
  own property names (cross-realm objects). `reference/devalue/src/utils.js:L37-L51`.
- `stringifyAsync` awaits thenables, writes the resolved value into the
  **same slot index** the promise was assigned, and if the resolved value is
  a sentinel, stores the negative number in the slot. Output format is
  identical to `stringify`. `reference/devalue/src/stringify.js:L31-L62`, `:L151-L154`.

### String escaping — `reference/devalue/src/utils.js:L58-L102`

`stringify_string` wraps in `"` and escapes: `"`→`\"`, `<`→`\u003C`,
`\`→`\\`, `\n`, `\r`, `\t`, `\b`, `\f`, U+2028→`\u2028`, U+2029→`\u2029`,
any other char `< ' '` → `\u00xx` (4 lowercase hex digits). Everything else
(including `/`, `>`, `&`, non-ASCII, lone surrogates) is emitted raw. This is
**valid JSON** (so `JSON.parse` reads it) but is NOT what `JSON.stringify`
produces (`<` escaped; lone surrogates not `\uXXXX`-escaped). Only `<` is a
script-injection defence; kit relies on it when inlining into `<script>`.

### Parse / hydrate — `reference/devalue/src/parse.js`

- `parse(str, revivers?, options?)` = `unflatten(JSON.parse(str), ...)`. `:L20-L22`.
- Input must be a number (sentinel only) or a **non-empty array**, else
  `Error('Invalid input')`. `:L34-L38`.
- `hydrate(index)`: sentinels first (`:L56-L60`); non-number or standalone
  non-sentinel → `Invalid input` (`:L62-L64`); memo `hydrated[index]`
  (`:L66`); `index >= values.length` → `Invalid input` (`:L68-L70`).
  **Negative non-sentinel indexes** (e.g. `-8`) are not explicitly rejected
  before `values[index]` — `values[-8]` is `undefined`, which falls into
  the primitive branch and hydrates as `undefined`. A Go port should reject
  them (CHANGELOG 5.9.2 "reject out-of-bounds indices" only covers `>=`).
- Slot dispatch (`:L74-L268`): falsy/non-object → primitive; array with
  string head → tagged; array with head `-7` → sparse; other array → dense
  array; object → plain object.
- Tagged dispatch order: **custom reviver first** if `Object.hasOwn(revivers,
  type)` (`:L80`) — so a user reviver named `Date` overrides the builtin.
  If `value[1]` is not a number (builtin payload shape like `["Date","..."]`)
  the payload is pushed onto `values` and re-indexed so the reviver receives
  a hydrated value (`:L83-L88`, CHANGELOG 5.4.2). Cycle guard: `hydrating`
  set; re-entering the same index while reviving → `Invalid circular
  reference`, unless already cached (`:L96-L110`, CHANGELOG 5.8.2).
- Builtins: `Date` → `new Date(iso)` (`""` → Invalid Date); `Set`/`Map`
  create-then-populate (cycle-safe); `RegExp` → `new RegExp(src, flags)`;
  `Object` → boxes `value[1]` but throws `Invalid input` if the wrapped slot
  is an object that is not a `["BigInt",..]` (`:L138-L151`); `BigInt` →
  `BigInt(value[1])`; `null` → null-proto object, throws on `__proto__` key;
  typed arrays/DataView require `values[value[1]][0] === 'ArrayBuffer'` else
  `Invalid data` (`:L169-L194`); `ArrayBuffer` requires string else
  `Invalid ArrayBuffer encoding` (`:L196-L203`); URL/URLSearchParams/Temporal
  via `fromStringValue`; unknown tag → `Unknown type <tag>` (`:L220-L221`).
- Sparse: `len` validated with `is_valid_array_len`, each idx with
  `is_valid_array_index` and `idx < len`, else `Invalid input`
  (`:L223-L245`). `createSparseArray` forces dictionary mode to avoid
  allocating `len` slots (`reference/devalue/src/operations.js:L154-L170`).
- Dense: `HOLE` entries are skipped leaving real holes (`:L246-L256`).
- Plain object: `__proto__` key → throw (`:L257-L268`).
- Order of hydration = depth-first from slot 0; slots unreachable from 0 are
  never hydrated (no error).

### base64 — `reference/devalue/src/base64.js`

Standard alphabet with `=` padding (`Uint8Array#toBase64()` default,
`Buffer#toString('base64')`, `btoa`). Decoding uses `Uint8Array.fromBase64`
(strict-ish: rejects invalid chars, allows whitespace per spec) or
`Buffer.from(b64,'base64')` (lenient: ignores invalid chars, no padding
required) depending on runtime (`:L56-L60`). Go: `base64.StdEncoding` for
encode; for decode accept both padded and unpadded (`StdEncoding` then
`RawStdEncoding` fallback) to match Node's lenient decoder.

### `operations` — what it is (and is not)

`operations` is **not** an ndjson/streaming patch API. It is a pluggable
introspection layer (5.9.0): `StringifyOperations` (`identify, typeOf,
toPrimitive, tagOf, isThenable, toPromise, unbox, toISOString,
toStringValue, regExpInfo, valuesOf, entriesOf, viewInfo, toArrayBuffer,
lengthOf, hasOwn, indicesOf, shapeOf, get`) and `ParseOperations`
(`fromPrimitive, fromISOString, fromStringValue, fromArrayBuffer,
fromRegExpInfo, fromViewInfo, box, createArray, createSparseArray,
createObject, createNullPrototypeObject, createSet, createMap, set, addValue,
addEntry`). `reference/devalue/src/operations.js:L54-L106`, `:L126-L191`;
`reference/devalue/src/types.d.ts`. Purpose: side-effect-free serialization
(no getters/proxies) and foreign-runtime/cross-realm values. `merge_operations`
iterates **default** keys and takes `overrides[key] ?? defaults[key]`
(`:L20-L30`). For the Go port this is exactly the seam to implement as a Go
interface: the algorithm in `run()`/`hydrate()` is realm-agnostic and every
JS-specific touch goes through these ~35 hooks. Kit's streaming of deferred
promises in `__data.json` is done by **kit** (chunked `{"type":"chunk",...}`
records, each independently `stringify`ed), not by devalue — verify in kit
(`packages/kit/src/runtime/server/data/index.js` and
`packages/kit/src/runtime/client/data.js`).

### `uneval` differences — `reference/devalue/src/uneval.js`

Emits **JavaScript source**, not JSON: primitives as literals (`void 0`,
`-0`, `1n`, `.5` for `0.5` via `/^(-)?0\./` strip `:L597-L606`), `new
Date(<ms>)`, `new RegExp(src,"flags")`, `new Map([[k,v]])`, `new Set([...])`,
`new Uint8Array([1,2]).buffer`, `{__proto__:null}` for null-proto,
`Object(…)` for boxed, `Temporal.X.from("…")`. Repeated references (count>1)
are hoisted into an IIFE `(function(a,b){a.x=b;...;return a}(<init>,<init>))`
with names from the alphabet `a..zA..Z_$` skipping reserved words (`:L13-L16`,
`:L562-L572`); >65534 names switches to a single-array-argument form
(`:L522-L526`). Object keys use `safe_key` (identifier or JSON string with
`<`/U+2028/9 escaped). Sparse arrays use `[,"a",,]` literals or
`Object.assign(Array(n),{i:v})` by a cost heuristic (`:L198-L272`).
`replacer(value, uneval)` returns a source string for custom types. **skgo
needs `uneval` only if Go renders the SSR HTML's inline data script itself**;
kit's Node sidecar does that today, so `uneval` is optional for the port.

### What kit uses on the wire (verify in kit — not read for this leaf)

From devalue's own docs and the shape of kit's API (verify against
`reference/kit/packages/kit/src/runtime/`):

- `__data.json` (server → client): kit calls `devalue.stringify(data,
  reducers)` per node, wrapped in kit's own JSON envelope
  (`{"type":"data","nodes":[{"type":"data","data":<devalue array>,"uses":..}]}`)
  and the client calls `devalue.unflatten(node.data, revivers)` on the
  already-`JSON.parse`d array — this is the documented `unflatten` use case
  (`reference/devalue/README.md:L90-L103`). Verify in kit:
  `packages/kit/src/runtime/server/data/index.js` (`data_response`,
  `stringify`/`uneval` of nodes and `chunk`s) and
  `packages/kit/src/runtime/client/data.js` / `client.js` (`unflatten`).
- SSR HTML: kit inlines `data` via `uneval` (with reducers derived from
  `transport`) into `<script>` — verify in
  `packages/kit/src/runtime/server/page/render.js`.
- Remote functions (kit 2.27+/3): arguments are `stringify`ed on the client
  and either put in the query string (`?payload=…`, URL-encoded) for
  `query`/`prerender` or POSTed for `command`/`form`; results are
  `stringify`ed on the server with `transport` encoders and `parse`d on the
  client with decoders. Verify in `packages/kit/src/runtime/shared.js`
  (`stringify_remote_arg`, `parse_remote_arg`) and
  `packages/kit/src/runtime/server/remote/*.js`.
- `transport` hook (universal in kit 3): `{ [name]: { encode(value) => any |
  false, decode(data) => value } }` maps 1:1 onto devalue reducers (`encode`,
  truthy return = match) and revivers (`decode`). The wire tag is the
  transport key: `["<name>",<slot>]`. Verify the exact glue in
  `packages/kit/src/runtime/shared/transport.js` or the hooks loader.
- Kit also has its own `data` payload extras (`uses`, `slash`, `status`,
  `error`) outside devalue — those are plain JSON.

## Citations

- Sentinels: `reference/devalue/src/constants.js:L1-L12`
- Stringify core loop and dedup: `reference/devalue/src/stringify.js:L70-L128`, `:L405-L415`
- Per-type encoders: `reference/devalue/src/stringify.js:L156-L402`, primitives `:L421-L428`
- Async: `reference/devalue/src/stringify.js:L31-L62`, `:L141-L154`
- Parse/hydrate: `reference/devalue/src/parse.js:L20-L274`
- Default ops (JS semantics to replicate): `reference/devalue/src/operations.js:L54-L106`, `:L126-L191`
- Escaping and plain-object test: `reference/devalue/src/utils.js:L4-L14`, `:L37-L56`, `:L58-L102`, `:L118-L168`
- base64: `reference/devalue/src/base64.js:L1-L60`
- uneval: `reference/devalue/src/uneval.js:L23-L530`, `:L562-L606`
- Public API and types: `reference/devalue/index.js:L1-L8`, `reference/devalue/src/types.d.ts`
- Behaviour history: `reference/devalue/CHANGELOG.md:L3-L150` (5.3.1 −0 fix, 5.3.2/5.6.3/5.6.4 `__proto__`, 5.6.1 hasOwn reviver, 5.6.2 buffer validation, 5.7.0 DataView/Float16/base64, 5.8.x sparse arrays + async, 5.9.x operations)

## Go port design

Package `devalue` (pure Go, no JS runtime). Two halves: a **value model** and
the **codec**.

### Value model (what `Parse` returns / `Stringify` accepts)

Go has no `undefined`, `-0` identity, ordered maps, sparse arrays, or object
identity for value types. Recommended: parse into a small closed set of Go
types via `any`, and provide typed helpers on top.

| devalue | Go representation (parse output) | notes |
|---|---|---|
| `undefined` (`-1`) | `devalue.Undefined{}` (empty struct sentinel) | distinct from `nil`; `Stringify` also emits `-1` for `Undefined{}` and for `nil` pointers *only if* the option says so (default: `nil` → `null`) |
| `null` | `nil` | |
| boolean | `bool` | |
| number | `float64` | `NaN`, `±Inf` map to sentinels `-3/-4/-5`; `-0` → `math.Signbit(f) && f==0` → `-6` |
| string | `string` | raw UTF-8; JS lone surrogates cannot round-trip — see Gotchas |
| bigint | `*big.Int` | wire `["BigInt","<decimal>"]` |
| Date | `time.Time` (UTC) | `toISOString` = `2006-01-02T15:04:05.000Z` **always 3 fractional digits, always `Z`**; years outside 0000–9999 use `±YYYYYY` (6 digits); invalid date `""` → `time.Time{}` + `IsZero()` |
| RegExp | `devalue.RegExp{Source, Flags string}` | never compile in Go |
| Map | `*devalue.OrderedMap` (slice of `[2]any` entries + index) | keys may be non-strings; insertion order matters for byte-equality |
| Set | `*devalue.OrderedSet` | |
| plain object | `*devalue.Object` (ordered `[]Entry{Key string; Value any}`) or `map[string]any` when order is irrelevant | for byte-equal output with kit you must preserve JS key order; see Gotchas |
| null-proto object | same as plain object with `NullProto bool` | wire `["null",k,v,...]` |
| Array | `[]any` | holes: `devalue.Hole{}` marker element; sparse arrays: `devalue.SparseArray{Length uint32; Entries []IndexValue}` |
| ArrayBuffer | `[]byte` | base64 std with padding |
| typed arrays / DataView | `devalue.TypedArray{Tag string; Buffer *[]byte; ByteOffset, Length int; Full bool}` | share the underlying `[]byte` pointer for dedup |
| URL / URLSearchParams | `*url.URL`? **No** — keep as `devalue.URLString`/`devalue.URLSearchParams string` | Go's `url.String()` re-normalises; store the original string |
| boxed `["Object",i]` | unwrap to the primitive on parse; never emit on stringify | kit never sends boxed primitives |
| Temporal.* | `devalue.Tagged{Tag, Text string}` passthrough | |
| custom `[name, slot]` | reviver `func(any) (any, error)` keyed by name | mirrors `transport.decode` |

`Stringify` should additionally accept ordinary Go values through reflection:
structs (exported fields in declaration order, `json` tags honoured for
names), `map[string]T` (**sorted keys** — document that this differs from JS
insertion order but is deterministic), slices, pointers (identity for dedup =
pointer address; non-pointer structs are copied so never dedup), `time.Time`
→ `Date`, `*big.Int` → BigInt, `[]byte` → `ArrayBuffer`? **No** — kit clients
expect `Uint8Array` for binary; emit `["Uint8Array",<bufslot>]` +
`["ArrayBuffer","…"]` for `[]byte` and let an option choose.

### API

```go
type Reducer func(v any) (out any, ok bool)   // ok=false → not matched (JS: falsy return)
type Reviver func(v any) (any, error)

func Stringify(v any, reducers map[string]Reducer, opts ...Option) (string, error)
func Parse(s string, revivers map[string]Reviver, opts ...Option) (any, error)
func Unflatten(parsed any /* []any or float64 */, revivers map[string]Reviver, opts ...Option) (any, error)
```

Reducer iteration order must be **deterministic**: JS iterates
`Object.getOwnPropertyNames(reducers)` (insertion order). Take reducers as an
ordered slice `[]NamedReducer` or sort map keys and document it; kit's
`transport` object order is the app's declaration order — verify in kit
whether kit passes the hook object directly (then order = user's file order)
before promising byte-equality across encoders that could both match.

### Encoder algorithm (port of `run`)

1. `flatten(v)`: if `Undefined` → return `-1`. If float: NaN→`-3`, +Inf→`-4`,
   −Inf→`-5`, `-0`→`-6`.
2. Identity key: for pointers/maps/slices use `reflect.Value.Pointer()` plus
   kind (slices: pointer+len is NOT enough for JS semantics — two different
   Go slices over the same array would alias; accept that or dedup only
   pointers/maps). For scalars use the value itself (`map[any]int` works for
   comparable scalars; strings included — matches JS primitive dedup).
3. Assign `index = p++`, record, then try reducers in order; on match write
   `["name",<flatten(out)>]`.
4. Otherwise switch on Go type per the table; children are flattened
   recursively **before** the parent string is finalised, but the parent's
   index was reserved first (that is what makes cycles terminate).
5. Root negative → return `strconv.Itoa(idx)`; else `"[" + join(slots, ",") + "]"`.

Number formatting: use `strconv.FormatFloat(f, 'g', -1, 64)`? **Not
identical** to JS. JS `Number#toString`: integers up to 1e21 print without
exponent (`1000000000000000000000` prints as `1e+21`), decimals use shortest
round-trip, exponent form `1e-7` for `< 1e-6`, exponent has explicit sign
(`1e+21`, `1e-7`). Implement a dedicated `jsNumberString(float64)` (there are
known Go ports of the ECMAScript `Number::toString` algorithm; the rule is
Number.prototype.toString step 5–10) and test it against the values in the
devalue test suite. `strconv` with `'f', -1` gets integers right but not
the `1e21` cutoff or the `1e-7` switch.

### Decoder algorithm (port of `unflatten`)

Use `encoding/json` `Decoder` with `UseNumber()` to get slot indexes as
`json.Number` and distinguish ints; then `hydrate(i)` exactly as
`reference/devalue/src/parse.js:L55-L271` with memo `[]any` + `[]bool`
"hydrated" flags (Go has no `in` operator). Reject: `i < -7`, `i >= len`,
non-integer numbers as indexes, `-2` (HOLE) outside dense arrays, `-7`
(SPARSE) outside array head position, wrapped `Object` of a non-BigInt
object, typed-array buffer slot that is not `["ArrayBuffer",string]`,
`__proto__` keys, unknown tags, circular reviver payloads. Populate
containers **before** recursing into children (create `*OrderedMap` then
`Set`, store in memo, then fill) so cycles resolve to the same pointer.

### Test list (Go)

Round-trip against the fixture strings in the devalue test suite (other
worker's leaf), plus: every sentinel at root and nested; `-0` inside
Float32Array (only via base64 bytes — fine); `["RegExp","a"]` two-element
form; `["Date",""]`; sparse arrays at both sides of the cost heuristic
(`[,1]` → HOLE form `[-2,1]`; `a[1000]=1` → `[-7,1001,1000,<slot>]`); Map with
object keys; shared buffer between two typed arrays; dedup of identical
strings; `__proto__` rejection; reviver-before-builtin precedence; reviver
receiving munged builtin payload (`["Date","…"]` with a user `Date` reviver);
`Invalid circular reference`; out-of-range indexes; number formatting table
(`0.1`, `1e21`, `1e-7`, `123456789012345680000`, `-0`, `5e-324`,
`1.7976931348623157e308`).

## Gotchas (JS semantics Go lacks)

- **`undefined` vs `null` vs missing key.** Kit `load` data frequently has
  `undefined` values (they serialize as `-1` slots, keys are *present*).
  Go `nil` must map to `null`; an explicit `Undefined{}` type is required.
  `json:",omitempty"` drops keys, which is a *third* behaviour.
- **Key order.** `Object.keys` order = integer-like keys ascending, then
  string keys in insertion order. A Go `map` cannot reproduce insertion
  order; a struct can (field order). Byte-equality with kit matters only if
  skgo compares payloads in tests or computes hashes; parsing is
  order-agnostic. Decide early: ordered object type vs "order not
  guaranteed" contract.
- **Dedup of primitives.** JS dedups by SameValueZero across *all* values:
  `["a","a"]` → `[[1,1],"a"]`. Go must do the same or output differs (still
  parses identically). `NaN` never reaches the map (sentinel), `+0`/`-0`:
  `-0` is a sentinel, `+0` deduped as `0`.
- **Number printing** (see above) and **integer-valued floats**: JS prints
  `1` for `1.0`; Go `%v` prints `1`; `strconv 'g'` prints `1`. OK, but `1e21`
  and small exponents differ.
- **Strings are UTF-16.** `stringify_string` walks UTF-16 code units; lone
  surrogates are emitted raw into JSON (invalid UTF-8 in Go). Go strings
  from `encoding/json` replace invalid sequences with U+FFFD. Accept the
  loss; kit data is practically never lone-surrogate.
- **Escape table differs from `encoding/json`.** Go escapes `<`,`>`,`&` as
  `\u003c`/`\u003e`/`\u0026` (lowercase hex) by default and U+2028/9; devalue escapes `<` as
  `\u003C` (uppercase C), does NOT escape `>`/`&`, and escapes control chars
  with lowercase hex (`\u001f`). Write a custom escaper; do not use
  `json.Marshal` for strings if byte-equality is wanted. Note `\/` is never
  produced (devalue) — README's `/` example is stale relative to
  `stringify_string` (`reference/devalue/src/utils.js:L58-L84` has no `/`
  case).
- **Date precision.** JS Date is integer milliseconds; `toISOString` always
  writes 3 fractional digits. Go `time.Time` has nanoseconds — truncate to
  ms on encode; on decode `time.Parse(time.RFC3339Nano, ...)` accepts
  `.000Z`. Year range: JS supports ±271821 years with `+YYYYYY-` form.
- **RegExp**: JS syntax and flags (`dgimsuvy`) are not Go RE2. Store, never
  compile.
- **Map/Set keys by identity.** A JS `Map` can have two distinct `{}` keys;
  Go map can't key by unhashable values. Use an entries slice.
- **Sparse arrays and holes** don't exist in Go; parse them into
  `SparseArray`/`Hole{}` markers, and never allocate `Length` elements from
  untrusted input (`reference/devalue/src/operations.js:L154-L170` warns
  about this DoS).
- **Boxed primitives** (`new String("x")`) — never emitted by Go; parse by
  unwrapping.
- **`__proto__`** is meaningless in Go but must still be *rejected* on parse
  for parity (a Go server forwarding data to a JS client would otherwise
  produce payloads the client refuses).
- **Reducer "truthy" test**: JS treats `0`, `""`, `false`, `null`, `undefined`
  as no-match; a reducer that legitimately wants to encode to `0` cannot.
  The Go API should use an explicit `ok bool`.
- **Cycles through reducers** are detected only on parse (`hydrating` set);
  stringify relies on index reservation. A Go reducer that returns a value
  containing the original object still terminates (index already reserved).
- **Identity of Go values**: structs passed by value are copied, so the same
  logical object appearing twice will serialize twice (no back-reference).
  Only pointers/maps/slices dedup. Document it.
- `stringify` throws on any class instance (non-POJO). Kit users therefore
  add `transport` encoders; Go's reflection-based struct encoding is more
  permissive — that is fine on the Go→client direction, but Go must not
  *expect* class instances from the client.

## Recipes

- **Emit a load result**: `devalue.Stringify(struct{...}{}, reducers)` →
  embed the string verbatim as the `"data"` member of kit's node JSON (it is
  already valid JSON, so `json.RawMessage`).
- **Read a remote-function argument**: URL-decode `payload`, then
  `devalue.Parse(payload, revivers)`; `revivers` come from the app's
  `transport` decoders expressed in Go.
- **Sentinel root fast path**: if the string does not start with `[`, it
  must be one of `-1 -3 -4 -5 -6` → return the sentinel value; anything
  else is `Invalid input`.
- **Streaming**: kit writes one devalue payload per chunk; Go writes the
  first `stringify` synchronously and later chunks as separate lines — the
  devalue codec itself needs no async mode; `stringifyAsync` exists so a
  single payload can embed resolved promises, which a Go server would
  simply await before calling `Stringify`.
- **Golden tests**: copy fixture pairs from `reference/devalue/test/` (other
  leaf) into Go table tests; compare `Stringify` output byte-for-byte and
  `Parse(Stringify(x))` structurally.
