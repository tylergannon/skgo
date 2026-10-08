# Adversarial review — milestone 4 (Go signatures generate usable Swift APIs), round 01

Date: 2026-10-08. Branch `codex/zig-native-foundation`, worktree
`/Users/tyler/.codex/worktrees/5c57/skgo`. Target: every change since `70af58c`,
including the untracked `internal/gen/swift*.go`, `internal/swiftgen/`, the
worklog and `ephemeral/reviewer-logs/`. Merge-base with `main` is `89208d3` and
`main` has not moved past it.

## Scope and operating constraints

The caller restated the plan's boundary (Swift owns HTTP, Zig owns protocol/keys/
caching, byte-buffer ABI with explicit release) and the user's requirement of
phased work with correctness validation and no specialized proof machinery. No
caller instruction limited the defects, files or subject matter considered, and
none predicted a verdict, so nothing was ignored. Operating constraints honoured:
read-only except this artifact; mutation checks ran on an rsync'd scratch copy
under the session scratchpad with the pinned Kit `node_modules` symlinked in.

Authoritative sources used: `CLAUDE.md`; `ephemeral/plans/zig-native-client.md`
milestone 4 and the component-boundary section; installed Kit 3.0.0 under
`example/web/node_modules/@sveltejs/kit`; Polytype v1.4.0 from the module cache
(`typegrammar/grammar.go`, `sealed_union.go`); Zig 0.17.0 std (`json/Stringify.zig`).

## Evidence inspected

- Diff of tracked files: `.github/workflows/ci.yml`, `cmd/skgo/main.go`,
  `internal/gen/{emit,gen,scan,types,wire_fields}.go`, `native/swift/README.md`.
- New files: `internal/gen/swift.go` (selection, Go-side domain guards),
  `internal/gen/swift_test.go`, `internal/gen/swift_darwin_test.go`,
  `internal/swiftgen/{swift.go,runtime.swift,swift_test.go,swift_darwin_test.go}`.
- Unchanged but load-bearing: `native/core/src/{abi,root}.zig`,
  `native/swift/Sources/SKGoNative/{RemoteClient,Value}.swift`, `skgo.h`,
  `internal/gen/{codecs,output}.go` (root planning, part merging into `skgo_gen.go`).
- Runs on this machine (all green): `go build ./...`, `go vet` on the touched
  packages, `go test ./internal/swiftgen`, and
  `go test -run 'TestNative|TestGeneratedSwift' ./internal/gen` (the darwin test
  builds the Zig core via mise, SwiftPM, a `swiftc` consumer and a real Go server;
  request counts hit `GET …/ude9z8/read`, `POST …/ude9z8/echo`, `GET …/ude9z8/wide`).
- Mutation checks on the scratch copy:
  - Remove the closed-object key check in `runtime.swift` → `TestGeneratedModelsCompileAndEnforceTheWireShape` fails ("accepted Unknown"). Load-bearing.
  - Remove the `nativeOut` guard emission in `emit.go:862` → `TestNativeNumberDomain` fails; the unguarded server emits `9007199254740992` for Go `9007199254740993`. Load-bearing, and it shows the f64 rounding the guard exists for.
  - Remove the ±(2^53−1) bound from `_SKGoWire.encodeInteger` → suite still green.
  - Remove the ±(2^53−1) bound from `_SKGoWire.integer` (decode) → suite still green. See finding 1.
- Probes on the scratch copy: an interface field with an unexported method but no
  `polytype.SealedUnion` declaration now generates without error for web and Swift;
  Polytype lowers it to an implicit `type`-discriminated union and projects
  `Omit<Note,"type"> & {"type":"Note"}` in `types.ts`, so Go/TS/Swift agree. A raw
  `*Node` field is still refused with the pointer diagnostic. A Go model type named
  `Error` produces Swift that does not compile. See findings 3 and 4.

## Milestone 4 exit conditions, as checked

The darwin round trip proves, over real HTTP against the production `skgo.NewRemotes`
handler, with literal fixtures: required/optional/nullable fields, string enum with
alias member, sealed union as field, optional union, union slice, slices, fixed-length
array, time as wire string, a finite recursive model, a cross-package naming collision
(`Thing`/`Thing2`), a no-argument query, a command with an argument, Go-side refusal of
an out-of-domain result (500 envelope) and argument (400 envelope, handler never
called). Unsupported kinds (live query) and custom transports fail with a
source-located diagnostic and publish no Swift. Generation is deterministic and
refuses an authored destination. CI runs the macOS job without a skip path: a
missing `swiftc`, `mise`, `zig` or Kit install fails the test rather than skipping.
Kit remote identity reuses `kithash.Kit(module)+"/"+name`, the same expression the
remotes manifest uses (`emit.go:1085`). The Zig/Swift boundary is unchanged by this
diff and still exchanges JSON byte buffers with `sk_buffer_release`; Swift copies
before release; Zig alone produces the devalue wire. No Zig model declarations exist.

## Findings (most severe first)

### 1. issue — The Swift-side JavaScript safe-integer bound is unproven on both encode and decode

Evidence: `internal/swiftgen/runtime.swift:44` and `:55` carry the ±9_007_199_254_740_991
bounds. The only Swift-side domain probes are `_SKGoWire.encodeInteger(UInt64.max)`
(`internal/swiftgen/swift_darwin_test.go:38`) and `api.wideInput(UInt64.max)`
(`internal/gen/swift_darwin_test.go:122`). Both are rejected by `Int64(exactly:)`
before the bound is consulted. The decode bound is never reached because `api.wide()`
is refused by the Go guard first (`:123` expects `RemoteError`, not `NativeModelError`).

Reproduction: on a scratch copy, delete `number >= -9_007_199_254_740_991, number <= 9_007_199_254_740_991`
from `encodeInteger`, run `go test ./internal/swiftgen` → `ok`. Delete
`abs(number) <= 9_007_199_254_740_991` from `integer` → `ok` again.

Impact: the README's "Generated Swift checks run before HTTP" and the plan's "Go and
Swift checks reject out-of-domain numbers before rounding" are, for the Swift half,
a claim without a test. A regression would let a Swift client send `9007199254740993`
and rely on the server's 400, and would let a hand-crafted or future lossy server
response decode to a rounded `Int64` silently. A fixture value such as
`Values: [9007199254740993]` (fits `Int64`, outside the safe range) on both the encode
and decode sides closes the gap in milliseconds.

### 2. issue — Native selection silently changes the server's response contract for every caller, including Kit's browser client

Evidence: `internal/gen/emit.go:862-864` and `:916-918` emit `skgoNativeCheckN` into
the shared handler only when a function is listed in `--swift-remote`. The handler is
the one the browser hits. With `wide` selected, a browser `GET` that previously got a
200 with a (lossy) number now gets a 500 error envelope; with `wideInput` selected, a
browser argument of 2^53 is now a 400. `TestNativeNumberDomain` asserts exactly this,
with a plain `httptest` request that carries no native marker.

Impact: a client-generation flag alters server behaviour for a different client. The
plan frames selection as "an explicit generation configuration" that "does not change
the server's authorization"; it is silent on changing status codes for the web app,
and the only statement of the coupling is one sentence in `native/swift/README.md`.
Either the guard is a correctness fix for everyone (the mutation run shows the browser
receives a rounded integer today) and should not depend on selection, or it should be
confined to the native path. As shipped, adding a Swift target can start failing an
existing browser flow, and a 500 reports an application result as an internal error
rather than the typed refusal it is. This is a design decision to make explicitly,
not a hidden side effect of a flag.

### 3. issue — The sealed-union and wrapped-pointer admission widening is web-facing but tested only through the native fixture

Evidence: `internal/gen/wire_fields.go:85` now skips the field projection error for any
interface with an unexported method (`sealedUnionField`, `:98-118`), and
`internal/gen/types.go:111-115` unwraps a pointer inside `Optional`/`Nullable`. Both
changes apply to every generated app, not only selected native functions. Before this
diff a struct field of interface type was refused for all apps ("must be concrete",
`types.go:185`). No test in `internal/gen` exercises a sealed union or wrapped pointer
in a web-only app; the only coverage is the native `Envelope` fixture.

Verified on the scratch copy: the loosened path is coherent, because Polytype v1.4.0
infers same-package variants and defaults the discriminator to `type` even without a
`SealedUnion` declaration, and `types.ts` gets `Omit<Note,"type"> & {"type":"Note"}`.
So this is not a wrong result today; it is an unowned change to the web wire contract.
A Go handler test with a literal browser-shaped request/response for a union field,
and a `.d.ts`/`types.ts` assertion, would anchor the behaviour independently of the
Swift milestone and give `sealedUnionField` a test of its own.

### 4. issue — A Go model type named `Error` produces Swift that does not compile

Evidence: `internal/swiftgen/swift.go:57` reserves `String Bool Int … Set Array
JSONSerialization _SKGoWire Foundation CoreFoundation SKGoNative` but not `Error`.
`runtime.swift:1` declares `public enum NativeModelError: Error`. A definition named
`Error` allocates `public final class Error`, which shadows `Swift.Error` in the same
file.

Reproduction (scratch copy, `go test -run TestZZShadowing ./internal/swiftgen` with a
one-object definition named `Error`): `swiftc` fails with "raw type 'Error' …" and
"enum with raw type cannot have cases with arguments". `Optional` and `Result` as type
names compile. `Error` is a common Go type name; the fix is one more entry in the
reserved list, the mechanism already handles it (`Thing2`).

### 5. nitpick — Shared fixture failure mode and untracked review logs

- `internal/gen/swift_test.go:71-98`: `generatedNativeFixture` runs `foreignFixtureIn(t, …)`
  inside `sync.Once`; that helper calls `t.Fatal`. If the first caller fails inside
  `Do`, later tests see `err == nil` and `root == ""` and fail on an unrelated path.
  Capture the error instead of letting the helper fatal the first test.
- `ephemeral/reviewer-logs/20261008-native-api-round-01/events/*.jsonl` is untracked
  tool output with no reader in the repository. CLAUDE.md asks for no narration;
  commit it only if something will read it.

## Outcome

material findings remain
