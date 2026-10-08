# SKGo native Swift client

The Zig core owns Kit protocol encoding and decoding. Swift executes HTTP with
URLSession. The C ABI exchanges byte buffers only; it exposes no value graph.

Build and test from the SKGo checkout:

```sh
mise -C native install
mise -C native/core exec -- zig build test install
swift test --package-path native/swift
go test -count=1 -race ./native
```

The last command runs the Swift boundary and its production-handler HTTP calls
on macOS; Linux runs the Zig handler integration. CI runs both platforms. No
Xcode project or simulator is needed for these command-line checks. Apple SDKs
and a Swift 6 compiler are required for the Apple command-line client.

`RemoteClient.call` accepts optional JSON `Data` and returns optional JSON `Data`.
This is the local ABI representation for finite model values. Zig translates it
to and from Kit's devalue wire. `nil` is omitted/undefined; JSON `null` is a
present null. Supplying empty argument Data is invalid. JSON objects, arrays,
strings, booleans and finite numbers are admitted. Unsafe integer input tokens,
cycles, sparse arrays, nested undefined and non-JSON devalue tags fail explicitly.
This does not introduce Go model JSON codecs or a Swift devalue implementation.

C input buffers are borrowed only during a synchronous call. Successful output
buffers own separate allocations; call `sk_buffer_release` once on each, including
error-message buffers. Failed calls initialize all outputs to empty. Release
zeroes the buffer. Swift copies outputs into its own Data before releasing them;
no buffer crosses an HTTP await. The actor confines all core calls to one owner.

Cancellation promptly releases the caller, even if an injected transport ignores
it. Late completions are discarded by call ID. `shutdown` cancels pending work,
rejects future calls and completes waiting callers. Cancellation cannot undo a
command already received by the server. URLSession transport refuses HTTP
redirects and does not replay commands. Kit redirect envelopes surface as a
separate `RemoteError.redirect`.

Generate typed calls with the application's existing `skgo generate` command:

```sh
skgo generate --web web --out generated --locals-package example.com/app/internal/app \
  --swift-out native/Generated.swift \
  --swift-remote 'src/data/data.remote.ts#read' \
  --swift-remote 'src/data/data.remote.ts#echo'
```

Paths in each selection are relative to the Vite root and name the generated
remote module and its Go export. Only selected ordinary queries and commands
enter `NativeAPI`; selection changes no authorization. Swift models and calls
come from the same published Polytype grammar used for Go's wire codecs, with
no application model declarations in Zig. Add the generated Swift source to the
native target and construct `NativeAPI(core: client)`.

Generated models keep Optional fields absent and Nullable fields explicitly
null, validate closed objects, enum membership and fixed array lengths, and
represent sealed unions with Swift enums. Recursive models are immutable Swift
classes; times remain wire strings. Naming collisions receive deterministic
suffixes. Calls preserve Kit's existing remote IDs. Custom transports and shapes
the grammar cannot describe fail generation with source diagnostics.

Native integer models use the JavaScript safe-integer range, intersected with
the Go/Swift integer width. Generated Swift checks run before HTTP; generated
Go checks run before result encoding and before a decoded argument reaches its
handler. Exact identifiers should be strings; numeric counters must fit this
range. Float values must be finite. Generated source is repeatable and refuses
to overwrite an authored Swift destination.

Selecting a function adopts that numeric contract in its shared Go handler,
including requests from Kit's browser client. An unsafe integer argument returns
a 400 remote error; an unsafe application result returns a 500 remote error,
just as other result-encoding failures do. Previously lossy browser values are
therefore refused when the function is selected. Choose string identifiers or
bounded counters in Go before selecting an existing function for native use.
## Retained ordinary queries

Generated query APIs expose `retainCounter()` alongside the one-shot
`counter()`. Retained queries share Zig's canonical argument key and initial
HTTP request. Read `snapshot()` for `ready`, `loading`, `current`, and `error`;
observe `changes()` for the latest state; await `value()` or `refresh()` for a
typed result. Release a UI lease explicitly with `await query.release()`.
An active await pins its result independently. Dropping a lease also releases
it asynchronously as a fallback.

Retention starts the initial request eagerly. `cacheCapacity` defaults to 256
entries, counting retained queries and prefetched values. Retaining a new key
when all entries are active throws `RemoteError.cacheFull`; increase the capacity
or release unused leases. Only unused prefetch entries may be evicted.

Pass `updates: [query.update]` to a generated command to request that query's
single-flight refresh. The Go command uses SKGo's existing requested-query
functions to fulfill or ignore it. An unhandled request puts the query in Kit's
400 error state, retaining its previous value. HTTP and top-level remote errors
leave the command's requested queries untouched. Only query-instance updates
are admitted; query-function updates, live queries, batching and optimistic
overrides are not part of this native API.

`RemoteClient` serializes all access, including graph reads, on its actor. Zig
stores serialized model bytes in a bounded cache; Swift owns HTTP tasks and
observation. A private core context is released with the client. No generated
model contains a Zig pointer, and every returned buffer is copied and released
before an await. Successful unsolicited updates can prefill unused capacity;
active retained entries cannot be evicted. Results remain Swift-owned after
release or eviction. Call `resetSession()` after changing authentication, or
`resetSession(origin:base:)` when changing servers. Both close existing leases
and reject pending work; old transport completions cannot update the new cache.
