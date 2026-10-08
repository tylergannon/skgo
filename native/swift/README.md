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

The current native core does not yet implement retained queries or caching.
