# Adversarial review — generated native Swift API (milestone 4), round 03

Date: 2026-10-08. Worktree `/Users/tyler/.codex/worktrees/5c57/skgo`, branch
`codex/zig-native-foundation`, merge-base with `main` `89208d3` (main has not
moved). Target: the complete current work since `70af58c`, tracked and
untracked, re-reviewed against the same authoritative sources as rounds 01 and
02 (`CLAUDE.md`; plan milestone 4 at `ephemeral/plans/zig-native-client.md`
lines 241-259; Kit 3.0.0 at `example/web/node_modules/@sveltejs/kit`; Polytype
v1.4.0; skgo's remote server and `internal/remotearg`).

## Scope and operating constraints

Caller constraints accepted: read-only except this artifact; mutation checks in
scratch copies. No caller narrowing of subject matter was present.

## Evidence inspected

Changes since the round-02 snapshot, read in full: `internal/gen/swift_test.go`
(fixture gains `signedInput`, `numberInput`, `wideSigned`, `wideNumber`;
`TestNativeNumberDomain` now base64url-encodes each command payload as kit's
`stringify_command_arg` does, asserts literal result bytes for the safe
boundary values, the nested 400/500 for unsafe ones, and exact per-function
application call counts after every request), `internal/gen/swift_darwin_test.go`
(Swift probe round-trips `-9007199254740991` and `1.25`, refuses
`-9007199254740993`, `Double.infinity` and `Double.nan` client-side; request
counts now include `signedInput` and `numberInput`), `internal/swiftgen/swift.go`
`fieldNames` (reserves `self`/`Self`/`super`/`init`/`deinit`, maps `_` to
`field`), `internal/swiftgen/swift_darwin_test.go` (awkward-key fixture `_`,
`-`, `self`, `class`, `a-b`, `a_b` compiled and round-tripped with original JSON
keys), the worklog entries recording both corrections. Unchanged files were
re-read where cited below.

Fresh on the real worktree, all green (exit 0): `go build ./...`,
`go vet ./internal/gen ./internal/swiftgen ./cmd/skgo`,
`go test -count=1 ./internal/swiftgen`,
`go test -count=1 -v -run 'TestNative|TestGeneratedSwift|TestWebGrammar' ./internal/gen`.

Mutations in a fresh scratch rsync copy, each restored before the next:

| mutation | result |
|---|---|
| C disable `nativeIn` wiring (`emit.go:916`) | FAILS: `wideInput` answers 500 after calling the application |
| C2 disable `nativeOut` wiring (`emit.go:862`) | FAILS: `wide` emits `9007199254740992`; Swift probe fails |
| D delete uint64 guard line (`swift.go:164`) | FAILS (same as C2) |
| E delete int64 guard line (`swift.go:162`) | FAILS: `signedInput` returns `-9007199254740992` |
| E2 drop only the int64 lower bound | FAILS (same) |
| H delete float NaN/Inf guard line (`swift.go:166`) | FAILS: `numberInput` returns devalue NaN hole `-3` |
| H2 drop only the NaN check | FAILS (same) |
| I remove Swift `encodeNumber` finite check | FAILS: darwin probe sends infinity |
| J remove `fieldNames` reserved words | FAILS: generated Swift does not compile |
| K remove `_` → `field` sanitising | FAILS: generated Swift does not compile |
| (round 02) A1/A2 Swift ±2^53 bounds, B web union skip, G closed object | FAILED then; code unchanged since |

Round-02 finding 1 is resolved: the command payload now reaches the handler
(the safe `[9007199254740991]` case must return a 200 result with literal
bytes and increment the call counter, so a parse-level 400 can no longer
satisfy the test), and every Go guard branch has a value that exercises it.
Round-01 findings 1-5 remain resolved as recorded in round 02.

Milestone 4 exit conditions, checked against current proof: selection by
`module#export`, remote identity `ude9z8` from kit's own hash, consumer
compilation plus real-server round trips covering required/optional/nullable,
raw-value enum with alias, sealed union (value and pointer members, optional
and slice positions), slices, fixed-length arrays, time strings, finite
recursion, type and field naming collisions, unsupported kinds and custom
transports refused explicitly, Go and Swift out-of-domain refusal for int64,
uint64 and float64 on both argument and result before f64 rounding, and the
shared browser/native contract stated in plan, README, `gen.go` and CLI help.
No material gap found.

## Findings

### 1. nitpick — Pointer nil guard and union default branch remain unexercised

`internal/gen/swift.go:174` refuses a nil pointer inside a present wrapper or
pointer union member; `swift.go:211` refuses an unknown union member. No test
sends either shape, so a `Nullable[*Node]{Present:true, Value:nil}` result is
pinned neither as the 500 it currently produces nor as a `null`. Cheap and
defensive; not a milestone claim.

### 2. nitpick — Review tool logs remain untracked beside the reviews

`ephemeral/reviewer-logs/20261008-native-api-round-0{1,2,3}/` is untracked and
not ignored. Ignore or delete before the squash merge.

## Outcome

only nitpicks remain
