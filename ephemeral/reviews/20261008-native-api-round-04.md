# Adversarial review: milestone 4, "Go signatures generate usable Swift APIs" (round 04)

## Review target

- Checkout: `/private/tmp/skgo-native-ci-repair`, branch `codex/zig-native-foundation`, HEAD `9d16418` ("Merge current runtime dependency into native foundation"), PR #287.
- Scope: everything since `70af58c` for milestone 4 of `ephemeral/plans/zig-native-client.md` (lines 241-259), judged against CLAUDE.md, the pinned Kit 3.0.0 source under `example/web/node_modules/@sveltejs/kit`, the plan's shared finite numeric contract, and the repository's CI gates.
- Caller narrowing: none. The launch prompt only set read-only boundaries, the artifact path and scratch-copy mutation checks.
- Change since round 03: the branch merged `main` (`89c09f4`, standalone `devalue/v5` and polytype 1.5.0). Milestone-4 files are byte-identical to the round-03 snapshot except the `devalue/v5` import in `internal/gen/swift_test.go` and the `UndefinedValue` path in `internal/gen/scan.go`. Commits `1804c9a` (cold Zig build budget) and `fcecd2d` (deterministic allocation-failure injection) were already in the round-03 tree and were re-read.

## Evidence inspected

- Local baseline on this checkout (log: scratchpad `base4.log`): `go build`, `go vet` incl. `./native`, `go test ./internal/swiftgen`, `go test ./internal/gen -run 'TestNative|TestGeneratedSwift|TestWebGrammar'` (darwin round trip 5.89s, web grammar unions, handler number domain, admission, unsupported shapes), `go test ./native` (`TestZigRemoteCalls` 1.47s, `TestSwiftRemoteCalls` 7.34s). All PASS, exit 0, no skips.
- Zig core in a scratch copy (`mise exec -- zig build test`): baseline green. Control mutation removing `release(allocator, &response.value)` in `allocationCase` fails with 2 leaks reported, so the allocation-failure test is load-bearing for owned buffers. Mutation deleting the `errdefer if (value) |v| allocator.free(v);` at `native/core/src/abi.zig:133` survives (see nitpick 2).
- Round 01-03 mutation set (11 guard/assertion deletions in `internal/gen` and `internal/swiftgen`) was not rerun because those files are unchanged apart from the import path; the round-03 results stand.
- CI for HEAD: `gh pr checks 287`, job 113279235861 steps, and `go run ./cmd/skgo-release-version --check HEAD` reproduced locally.
- Kit source re-consulted: `stringify_command_arg` (base64url devalue command payloads) and the remote response envelope shapes used by `native/core/src/root.zig` `receive`.

## Findings

### 1. Issue: PR head fails the Conventional Commit gate, so no Linux CI proof exists for HEAD

Evidence:
- `git log -1 --format=%s` on HEAD: `Merge current runtime dependency into native foundation`.
- `go run ./cmd/skgo-release-version --check HEAD` on this checkout exits 1: `"Merge current runtime dependency into native foundation" is not a Conventional Commit subject (want type(scope): description)`.
- GitHub job 113279235861 ("Build, tests, and Conventional Commit", run 37767687832): step "HEAD uses a Conventional Commit subject" failure; "Install build dependencies", "Build", "Install native core compiler", "Vet" and "Unit and focused integration tests" all skipped. PR `mergeStateStatus` is `UNSTABLE`. The macOS "Swift client and generated production HTTP calls" job was still queued at review time, so no CI verdict exists for it either.

Impact: the only Linux execution of this milestone's proof (the Zig allocation-determinism fix in `fcecd2d` and the cold-build budget in `1804c9a` both exist specifically for Linux CI) has not run for the code that is actually proposed for merge. The devalue v5 / polytype 1.5 switch pulled in by the merge is likewise unproven on Linux for this branch. CLAUDE.md's "a passing command is not evidence on its own" cuts both ways: a red check with all tests skipped is zero evidence, and the repository convention (worklog and merge gate) is that PR branches are rebased onto `main`, not merged with it, precisely because a merge commit cannot pass this gate.

Reproduction: on the branch, run `go run ./cmd/skgo-release-version --check HEAD`. Resolution is a rebase of the branch onto `89c09f4` (or a Conventional Commit subject on the integration commit), then a green run of both CI jobs on the new head before milestone exit.

### 2. Nitpick: unreachable `errdefer` in `decodeBuffers`

`native/core/src/abi.zig:133` frees `value` if duplicating `message` fails. `core.receive` (`native/core/src/root.zig:150-197`) never produces a response with both a non-undefined `value` and a `message`: results carry a value and no message, error/http-error carry a message and no value, redirects carry the location as `message` with no value. Deleting the line in a scratch copy leaves `zig build test` green, confirming it is dead rather than under-tested. Harmless, but it reads as a leak guard that the suite covers when it is a branch nothing can enter.

### 3. Nitpick: carried over from round 03

`internal/gen/swift.go:174` (pointer-nil guard) and `swift.go:211` (union `default` branch) remain unexercised by any test; unchanged since round 03.

### 4. Nitpick: untracked `ephemeral/reviewer-logs/`

Present as untracked in the worktree and not gitignored. Either track it or ignore it; `ephemeral/` is otherwise tracked working material.

## Outcome

material findings remain
