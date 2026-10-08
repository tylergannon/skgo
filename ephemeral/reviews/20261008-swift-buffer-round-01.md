# Adversarial review: milestone 2 Swift buffer boundary (round 01)

Date: 2026-10-08. Reviewer: Claude Fable 5.1, read-only except this file.

## Target

Commit `18320b9` ("feat: bridge the Zig remote core to Swift through owned
buffers"), the only commit after `1cf4e50` on `codex/zig-swift-boundary`.
Reviewed against `ephemeral/plans/zig-native-client.md` milestone 2, the
repository instructions in `CLAUDE.md`, and the caller's restated user
instruction: a simple buffer-oriented Zig/Swift ABI, Swift owns HTTP, Zig owns
protocol encoding/decoding/keys/cache, serialized byte buffers with explicit
ownership and release, no cross-language object model without a concrete need.

No caller instruction narrowed scope, predicted findings, or requested a
verdict. The read-only constraint and the artifact path were honored.

## Evidence inspected

- Diff `1cf4e50..HEAD` in full: `native/core/src/abi.zig`, `native/core/build.zig`,
  `native/swift/**` (Package.swift, README, C header/bridge, RemoteClient,
  Value, RemoteCLI, ClientTests), `native/swift_test.go`, `.github/workflows/ci.yml`,
  `.gitignore`, `ephemeral/worklog/20261008-swift-buffer-boundary.md`.
- Surrounding code: `native/core/src/root.zig` (prepare/receive, Response
  ownership), `native/remote_test.go` (milestone 1 harness), the pinned devalue
  Zig package in `native/core/zig-pkg/.../zig/src/root.zig` (Graph.node,
  get, arrayGet, put semantics), polytype v1.4.0 `devalue` (undefined and nil
  encoding), `Justfile` `test` recipe, CI triggers.
- Exit proof run: `go test -count=1 -race ./native` on this machine passed
  (Zig build + Zig ABI tests, Swift boundary tests, Swift CLI against the real
  `skgo.NewRemotes` handler including the 307 refusal). 14.7s warm.
- Scratch probes (scratch copy of the repo, nothing in the worktree touched):
  - Mutation: `completionHandler(nil)` → `completionHandler(request)` in
    `NoRedirect`. `TestSwiftRemoteCalls` failed with `http(500, "")` at
    `swift_test.go:107`. The redirect/replay assertion is load-bearing.
  - Probe test through `RemoteClient` with an injected transport: integer
    tokens `9223372036854775807`, `9223372036854775808`, `9007199254740992`,
    `9007199254740993` and `1e400` all fail with `unsupportedValue`; `1.0`
    encodes to wire `[1]`; `-0` encodes to `-6`. Result numbers serialize as
    `1`, `0.5`, `1000000000000000000000` (plain JSON, not exponent form).
  - Probe of result shapes: explicit `undefined` property (`{"a":-1}`), array
    hole, nested `undefined` element and a `Date` all fail with
    `unsupportedValue` rather than being coerced.
  - Checked devalue `Graph.node` only sorts the addressed node's own property
    list in place and never grows node storage, so `toJSON` iterating a parent's
    `properties.items` while recursing into children is not an invalidation
    hazard (the plan's warning applies to the addressed node, and the cycle
    check runs before `graph.node` is called on it).

## Assessment against the authoritative requirements

- ABI shape: three C exports (`sk_prepare`, `sk_decode`, `sk_buffer_release`),
  borrowed input `SKBytes`, owned output `SKBuffer`, release zeroes, failed
  calls leave outputs empty. No handles, no graph exposure, no Swift-managed Zig
  lifetimes. This meets the user instruction. The earlier graph/token API was
  removed per the worklog; nothing of it remains in the diff.
- Swift owns HTTP: `URLSessionTransport` performs requests; `RemoteTransport`
  is injectable; Zig never opens a socket. Redirects are refused at the task
  delegate so commands are not replayed; proved against the real handler.
- Milestone 2 boundary tests: allocation failure
  (`checkAllAllocationFailures` over prepare+decode with the testing
  allocator, leak-checked), release zeroing and output outliving parse (Zig),
  omitted-vs-null distinction, invalid input never dispatching, literal input
  and Swift-owned output, cancellation before dispatch, cancellation with a
  transport that ignores it plus late completion discarded by call ID,
  shutdown rejecting pending and future calls, sixteen concurrent callers each
  receiving their own result, HTTP error vs remote error vs Kit redirect vs
  broken wire vs cyclic value (Swift). Swift-to-handler literal input/output is
  the Go test against `skgo.NewRemotes`, not a mock.
- Rules: no skips (missing `mise`/`swift` fails the test), no acceptance
  runner, Go test at the real handler, worklog holds only corrections/traps.
- Query keys and caching are deferred to milestone 5 and the README says so;
  that is consistent with the plan, not a gap in milestone 2.

No critical findings and no issue-level findings were confirmed. The findings
below are genuine nitpicks.

## Findings

### 1. Nitpick: `SKReply.kind` values are undocumented and coupled to enum declaration order

`native/swift/Sources/CSKGo/include/skgo.h:11` documents the status codes but
not the `kind` values. `native/core/src/abi.zig:135` emits
`@backingInt(response.kind)` from the anonymous `enum { result, http_error,
remote_error, redirect }` declared at `native/core/src/root.zig:139`, and
`native/swift/Sources/SKGoNative/RemoteClient.swift:123-128` switches on the
literals 0..3. Reordering or inserting a variant in `root.zig` would silently
remap Swift's error classification; no test pins the numbers. Impact today is
nil because both sides agree, but the header is the ABI contract and should
state the four values (or Zig should export an explicit-valued enum).

### 2. Nitpick: the cyclic-value test is not load-bearing for the cycle check

`native/core/src/abi.zig:72` rejects a handle already on the ancestor stack and
`abi.zig:62` caps depth at 256; both errors map to status 3 at `abi.zig:19`.
Deleting the ancestor check still yields `unsupportedValue` for
`{"self":1}` in `ClientTests.swift:141` via `DepthLimit`, so the Swift fixture
cannot tell the two apart. Harmless for behavior (the call still fails
explicitly), but the one fixture that names cycles does not prove the cycle
path.

### 3. Nitpick: `go test ./native` builds the Zig library twice and writes into the source tree

`native/remote_test.go` builds into a temp prefix; `native/swift_test.go:24-28`
builds again into `native/core/zig-out` because `Package.swift:5-6` hard-codes
`../core/zig-out/lib`. The Swift scratch path is a fresh `t.TempDir()`, so the
Swift package is compiled cold on every run. `just test` runs `./...`, so on
macOS this chain is now part of the ordinary test gesture (14.7s warm here).
The "run toolchain once, assert many" rule suggests one build feeding both
harnesses, and build output in the checkout is a tracked-ignore rather than a
temp dir.

### 4. Nitpick: `unsafeFlags` makes `SKGoNative` unconsumable as a SwiftPM dependency

`native/swift/Package.swift:15` links with `.unsafeFlags(["-L", library])`.
SwiftPM rejects targets with unsafe flags when the package is pulled in as a
remote dependency. Fine for the milestone 2 command-line proof, but the plan's
end state has application packages consuming this library, so milestone 6 will
have to replace it (binaryTarget/XCFramework or a Go-driven copy of the static
library). Worth recording so it is not discovered late.

### 5. Nitpick: explicit `undefined` properties in a result fail the whole call

`abi.zig:80` returns `UnsupportedValue` for any object property whose value is
`.undefined`. Kit's own client accepts `{a: undefined}`, and polytype's
`Object.MarshalJSON` (value.go:252-258) omits such a property the way
`JSON.stringify` does; only a Go author writing `devalue.Undefined` explicitly
produces it (nil pointers encode as null, verified). The README documents the
rejection, so this is a conscious profile choice, but it is stricter than both
Kit and polytype's JSON projection. Revisit when milestone 4 defines how
generated optional fields are emitted; omitting the key would match the two
existing JSON views.

## Outcome

only nitpicks remain
