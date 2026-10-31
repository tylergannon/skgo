# SKGo C# remote client

This .NET 10 client uses the same resolved Go model and Zig protocol/cache core
as Swift. It runs directly on macOS or Linux. No Go plugin, web view, or bridge
server runs inside the application. Windows loading and packaging are deferred.

Install the native-shell plugin paired with the exact official SKGo host as
usual. Add `--set language=csharp` and `--set out=client/Generated.cs` to the
plugin pin, with one `--select 'src/data.remote.ts#read'` per ordinary query or
command. Run `skgo generate --plugins` with the application's normal generation
options. The default language is Swift; C# defaults to `native/Generated.cs`.
The host protects authored/edited output and preserves unchanged generated files.

Export the SDK with `skgo plugin native-shell sdk --out native/SDK`. This copies
Swift, C#, and Zig sources from the independently versioned plugin artifact.
Keep application-authored code outside this disposable export directory.

From the exported SDK directory, run `mise install`, then
`mise -C core exec -- zig build shared`. Reference
`csharp/SKGoNative/SKGoNative.csproj` from your .NET 10 project and compile the
generated source in your application. Copy the host's
`core/zig-out/lib/libskgo_native_core_shared.dylib` (macOS) or
`libskgo_native_core_shared.so` (Linux) beside your executable. .NET resolves
`skgo_native_core_shared` through its normal native-library search. The dynamic
library has a separate name so building it cannot change Swift's static linking.
Build the library for the same architecture as the .NET process.

Construct an application-owned `HttpClient` whose handler has
`AllowAutoRedirect = false`, configure its headers/cookies, and wrap it in
`HttpClientTransport`. Pass that transport, the server origin, and optional
base path to `RemoteClient`, then construct `GeneratedRemote.NativeAPI(client)`.
An application can also implement `IRemoteTransport`. It must refuse HTTP
redirects to avoid command replay. Dispose the HTTP client in the application;
`RemoteClient` does not own it. Authentication policy belongs to the application.

Generated methods end in `Async` and accept `CancellationToken`. Query methods
also have `RetainName` methods returning `RemoteQuery<T>`: await `ValueAsync` or
`RefreshAsync`, inspect `Snapshot()`, or observe `Changes()` with `await foreach`.
Dispose leases explicitly. A pending await pins its result independently of its
lease. Commands accept query-instance `Update` tokens; Zig applies fulfilled,
ignored, and unhandled refreshes. Batching, live queries, query-function updates,
optimistic overrides, forms, and custom Go wire transports are rejected.

Models are closed objects with required properties, validated enums, sealed
union cases, recursive references, and fixed-length arrays. Times remain strings.
`Maybe<T>` represents both optional and nullable fields: its default value omits
an Optional field and writes JSON null for a Nullable field. A present value is
constructed with `new Maybe<T>(value)` or implicit conversion. Optional fields
reject explicit null on decode; Nullable fields require the key. Numeric values
must be finite and integers must fit both their Go width and JavaScript's safe
integer domain. Validation runs before HTTP. Naming collisions receive stable
suffixes; repeated generation is byte-identical.

The raw API exchanges optional JSON bytes with Zig: a null byte array means an
omitted/undefined argument or result, whereas UTF-8 `null` means present null.
Empty buffers and unsupported wire/model values fail explicitly. Zig alone
handles Kit's devalue wire and canonical cache keys. All cache operations run
under one client lock; ABI buffers are copied/freed before asynchronous work.
Observers keep only the latest state and close on release/reset/shutdown.

Call `ResetSession()` after changing application configuration, or pass a new
origin, base path, or transport. Reset closes old leases and rejects pending
queries/commands with `sessionChanged`; old completions cannot affect the new
session. Cancellation releases an awaiting caller even when transport ignores
it. `Shutdown()`/`Dispose()` rejects pending and future calls with `closed`.
Cancellation cannot undo a command already received by the server.

## Running consumer

From `plugins/native-shell`, run
`go test -tags skgo_qualification -count=1 -v ./sdk -run TestCSharpRemoteClient`.
It generates `RemoteCLI/Generated.cs` into a temporary SDK copy, builds the host
Zig library once, starts the real SKGo remote handler, and runs the standalone
console directly against it. Go owns server cleanup. The console checks literal
initial/changed/confirmed values, observed and explicit refreshes, errors,
omission/null, no HTTP redirect replay, cancellation, shutdown, and session reset
with delayed real HTTP completions. Its output records host/runtime versions.
No Windows build is part of this proof. `RemoteCLI` requires that generated
fixture API; the Go test supplies it without writing to the source checkout.
