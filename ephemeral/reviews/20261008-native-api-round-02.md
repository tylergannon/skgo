# Adversarial review — generated native Swift API (milestone 4), round 02

Date: 2026-10-08. Worktree `/Users/tyler/.codex/worktrees/5c57/skgo`, branch
`codex/zig-native-foundation`, merge-base with `main` `89208d3` (main has not
moved). Target: the complete current work since `70af58c`, tracked and
untracked, re-reviewed against the same authoritative sources as round 01.

## Scope and operating constraints

Caller constraints accepted: read-only except this artifact; mutation checks in
scratch copies. No caller narrowing of subject matter was present.

Authoritative sources: `CLAUDE.md`; `ephemeral/plans/zig-native-client.md`
milestone 4 (lines 241-259, including the paragraph added since round 01 that
makes the safe-number contract a property of each selected Go handler for
browser and native callers alike); Kit 3.0.0 at
`example/web/node_modules/@sveltejs/kit` (remote client in
`src/runtime/client/remote-functions/`); Polytype v1.4.0 grammar, typegrammar
and devalue; skgo's own remote server (`remote.go`, `remote_caller.go`,
`internal/remotearg/remotearg.go`).

## Evidence inspected

Source, read in full or at the cited regions: `internal/gen/swift.go` (241
lines), `internal/gen/emit.go` guard wiring (lines 862-864, 916-918),
`internal/gen/gen.go`, `internal/gen/types.go`, `internal/gen/wire_fields.go`
(`sealedUnionField`), `internal/swiftgen/swift.go` (reserved list line 57,
`allocate`), `internal/swiftgen/runtime.swift`, `cmd/skgo/main.go`,
`.github/workflows/ci.yml`, `native/swift/README.md`, the plan, the worklog,
`native/core/src/root.zig` request assembly, and the full diff of every test
file against the round-01 snapshot.

Tests, fresh on the real worktree (all green, exit 0): `go build ./...`,
`go vet ./internal/gen ./internal/swiftgen ./cmd/skgo`,
`go test -count=1 ./internal/swiftgen`, and
`go test -count=1 -v -run 'TestNative|TestGeneratedSwift|TestWebGrammar' ./internal/gen`
(darwin HTTP round trip 6.8s, web grammar 1.7s, generation+handler domain 2.2s,
admission and unsupported-kind cases).

Mutations, each in a scratch rsync copy with the fixture temp dir kept:

| mutation | result |
|---|---|
| A1 remove Swift decode safe bound (`abs(number) <= 2^53-1`) | swiftgen FAILS (`accepted Values`) |
| A2 remove Swift encode safe bounds | swiftgen FAILS; gen darwin FAILS (`RemoteError.unsupportedValue`) |
| B remove `sealedUnionField` skip in `wire_fields.go` | `TestWebGrammarUnionsAndWrappedPointers` FAILS; native generation FAILS |
| C disable `nativeIn` wiring in `emit.go` | **all gen tests stay green** (finding 1) |
| D delete uint64 guard line in `swift.go` | gen handler test FAILS (`wide emitted unsafe number`); darwin FAILS |
| E delete int64 guard line | **green** (finding 1) |
| F delete `*tg.Pointer` nil guard | green (finding 2) |
| G remove Swift closed-object key check | swiftgen FAILS (`accepted Unknown`) |
| H delete float NaN/Inf guard line | **green** (finding 1) |

Probes: the generated `Error2` fixture compiles; a scratch test appended 40
further Go type names (`Optional`, `Result`, `Dictionary`, `Hashable`,
`Equatable`, `Date`, `URL`, `Codable`, `Collection`, `Sequence`, `NSObject`,
`Int128`, `CGFloat`, `Task`, `Actor`, `RawRepresentable`, `CaseIterable`,
`Value`, `Root0`, `Root_0`, …) as roots and compiled each with `swiftc
-swift-version 6`; all compile. Direct decode of `[9007199254740992]` through
`devalue.Parse` + `DecodeRoot1` succeeds (the guard, not the decoder, is what
refuses it). Real command requests with kit's base64url payload against the
generated handler: unmutated gives `[1]` → 200 result, `[2^53-1]` → 200 result,
`[2^53]` → 400 with `WideCalls` unchanged; with mutation C, `[2^53]` → 500 and
`WideCalls` incremented, i.e. the unsafe argument reached the application.

## Round-01 findings, current state

1. Swift ±2^53 bounds unproven — **resolved**: signed boundary values on encode
   and decode; mutations A1/A2 fail.
2. Selection changes the browser-facing contract — **resolved as a decision**:
   plan lines 254-259, README, `gen.go` comment and `--swift-remote` help now
   state the shared contract. The browser-side 400 claim itself is still not
   proven by any test (see finding 1).
3. Web-facing union/pointer widening tested only natively — **resolved**:
   `TestWebGrammarUnionsAndWrappedPointers` asserts the TS contract, the
   absence of native guards in web-only generation, and the real handler;
   mutation B fails it.
4. Go type named `Error` — **resolved**: reserved; `Error2` compiled; the wider
   name probe above found no further collision.
5. Fixture failure mode — **resolved** (`nativeFixture.err` preset, setup uses
   `t.Fatal`). Untracked reviewer logs — still present (finding 3).

## Findings (most severe first)

### 1. issue — The Go-side argument guard is unproven: the handler test's command payload never parses, so its 400 is satisfied by any server

Evidence: `internal/gen/swift_test.go:189` posts
`{"payload":"[9007199254740992]","refreshes":[]}`. Kit's client sends the
command argument as `stringify_command_arg`, a base64url-encoded devalue
string (`command.svelte.js:59-61`), and skgo parses it that way:
`internal/remotearg/remotearg.go:94` `decodeBase64URL(payload)` before
`devalue.Parse`. The raw JSON array is not valid base64url, so
`remote.go:871-873` answers 400 before `remote_wideInput` is reached. The
test's two assertions for `wideInput` — nested status 400 and `WideCalls==0`
(`swift_test.go:198-200`) — therefore hold for every value, including `[1]`
and `[9007199254740991]` (probe output above: all 400, `calls=0`).

Reproduction: disable the `nativeIn` emission (`emit.go:916`, `if false {`),
regenerate, run `go test -run 'TestNativeSwiftGeneration|TestGeneratedSwift'
./internal/gen` → `ok`. The darwin round trip does not cover it either:
`wideInput(UInt64.max)` and `wideInput(9007199254740993)` are refused by
Swift's `encodeInteger` before any HTTP request, as the test itself expects
(`NativeModelError`). The implementation is correct (with a base64url payload
the unmutated handler answers 400 and leaves `WideCalls` at 0; with the guard
removed it answers 500 after calling the application), but no test in the
suite would notice its removal.

The same gap covers the int64 and float guard lines: mutations E and H stay
green because the only out-of-domain value any test sends through Go is the
uint64 result of `wide`. The plan's exit condition "Go and Swift checks reject
out-of-domain numbers before rounding" (plan line 250) and the newly documented
browser-facing promise "an unsafe integer argument returns a 400 remote error"
(README, plan lines 255-256) are proven only for uint64 output. This is the
"never derive what you expect from the thing you are testing" trap in
`CLAUDE.md`: an assertion that is exact but satisfied for an unrelated reason.

Impact: a regression in `nativeIn` wiring or in the int64/float guard bodies
ships green, and the one path a browser caller actually takes to the new 400 is
untested. Fix shape: encode the test payload the way kit does (base64url of the
devalue string), add a safe value that must reach the application
(`WideCalls==1`, 200 result) beside the unsafe one, and send one out-of-domain
int64 and one non-finite float through a real handler.

### 2. nitpick — Pointer nil guard and union default branch are unreachable in the suite

`internal/gen/swift.go:174` refuses a nil pointer inside a present wrapper or
pointer variant; `swift.go:211` refuses an unknown union member. Neither path
is exercised (mutation F green). They are defensive and cheap, but a 500 on
`Nullable[*Node]{Present:true, Value:nil}` versus a `null` on the wire is a
behaviour the suite does not pin either way.

### 3. nitpick — Review tool logs remain untracked beside the reviews

`ephemeral/reviewer-logs/20261008-native-api-round-0{1,2}/` is untracked and
not ignored (`git check-ignore` prints nothing). Either ignore the directory or
delete it before the squash merge; it is not actionable intelligence.

## Outcome

material findings remain
