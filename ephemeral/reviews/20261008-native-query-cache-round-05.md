# Adversarial review: milestone 5, native ordinary-query cache — round 05

## Review target

- Branch `codex/native-query-cache`, HEAD `5b94b7d` ("test: anchor native
  query races and errors to Kit responses"), worktree
  `/Users/tyler/.codex/worktrees/5c57/skgo`. Merge base with `main` is
  `ad73806` (#287). Six commits on the branch: `6b394cc`, `c515cf3`,
  `4c72e42`, `2f5bf76`, `15d1ca6`, `5b94b7d`.
- Scope: milestone 5 of `ephemeral/plans/zig-native-client.md` (lines 262–274),
  CLAUDE.md rules, and installed Kit 3.0.0 at
  `example/web/node_modules/@sveltejs/kit` as the specification.
- Previous artifact: round 02 (`20261008-native-query-cache-round-02.md`),
  written against `15d1ca6`. Rounds 03 and 04 were requested but never
  written: each session was interrupted by the turn timeout during Swift
  mutation runs (see the untracked worklog `ephemeral/worklog/native-query-cache.md`).
  This round re-reviews the whole current work and reports the status of every
  round 02 finding.
- Caller narrowing: none. Read-only and scratch-copy constraints honoured; all
  mutation runs used copies under the session scratchpad.

## Evidence inspected

Implementation and proof read in full at HEAD: `native/core/src/cache.zig`,
`query_response.zig`, `abi.zig`, `root.zig`, `cache_test.zig`, `tests.zig`,
`build.zig`, `build.zig.zon`; `native/swift/Package.swift`,
`Sources/SKGoNative/Query.swift`, `RemoteClient.swift`, `Value.swift`,
`Sources/CSKGo/include/skgo.h`, `Tests/SKGoNativeTests/QueryTests.swift`,
`native/swift/README.md`; `native/swift_test.go`, `native/remote_test.go`,
`internal/gen/swift_darwin_test.go`; `remote.go` `serveQuery` (813–850); the
full diff `15d1ca6..5b94b7d`; both worklogs.

Kit 3.0.0 sources: `query/instance.svelte.js` (`#run` 110–168, `then` 170,
`refresh` 231–234, `set` 239–256, `fail` 260–271), `shared.svelte.js`
(`remote_request` q handling 140–155, `fail_unhandled_refreshes` 195–204),
`cache.svelte.js`, `command.svelte.js`, `query/index.js`,
`src/runtime/server/remote-functions.js`.

Proof at HEAD `5b94b7d` (all green, run this round):

| Command | Result |
| --- | --- |
| `zig build test install --summary all` (cold cache) | 25/25 tests pass |
| `swift test --package-path native/swift` | 14/14 pass |
| `go test -count=1 -race ./native` | ok, 11.2s |
| `go test -count=1 ./internal/gen -run 'TestGeneratedSwiftCallsTheGeneratedGoHandlers\|TestSwift'` | ok, 6.7s |

Mutations on scratch copies of `native/` at HEAD. Swift results for Zig
mutations are only trusted when run with a fresh `--scratch-path` (see
finding 1 for why):

| # | Mutation | Zig tests | Swift tests (fresh link) | Verdict |
| --- | --- | --- | --- | --- |
| M5 | `Cache.fail` stores errors on `refs == 0` entries | **killed** (`prefetch retention is bounded…`) | survives `commandRefreshes…` | fixed since r02; Zig-level coverage is the right layer |
| M14 | `Cache.fail` leaves `loading = true` | **killed** (2 tests) | **killed** (`commandRefreshes…`) | fixed since r02 |
| M15 | `Cache.fail` keeps pending tickets | killed (`an explicit q failure settles…`) | `commandRefreshes…` hangs > 600s | finding 2 |
| M2 | `set` keeps pending tickets | killed | survives `queryRefreshOrdering…` (masked: `resolve` settles the same tickets) | Zig coverage suffices |
| M13 | Zig rejects HTTP errors as kind remote | survives | **killed** with fresh link; *passes* with a reused `.build` | finding 1 |
| M6 | Swift `start()` ignores existing flight | — | killed by 3 tests, each within 1s | fixed hang mode since r02 |
| M11 | Swift `CoreCache.reject` maps `.http` as remote | — | survives | dead path, finding 3 |

## Status of round 02 findings

1. Swift refresh race on non-production fixtures — **fixed**. Both race
   envelopes in `QueryTests.swift:64,71` now carry the query's own `q` node
   exactly as `remote.go:848` emits it, the test asserts last-arrival-wins,
   and the worklog line was rewritten as a correction.
2. `loading` after `fail` unasserted — **fixed**. `cache_test.zig:156`,
   `cache_test.zig:162-172` and `QueryTests.swift:110` now anchor it; M14 is
   killed in both suites.
3. `refs == 0` drop rule untested — **fixed**. `cache_test.zig:80-84` fails an
   unretained prefetched key and asserts it keeps its value with no fault; M5
   is killed in Zig.
4. `CacheFull` on retain and eager dispatch — now **documented** in
   `native/swift/README.md:87-90` as the API contract. Still a departure from
   Kit's never-refusing cache, but the plan requires bounded retention and
   explicit ownership, so this is accepted as a stated contract rather than a
   finding.
5. Hygiene — time limits added to all six query tests, stale README line
   removed. Double reject and `deinit` fallback remain (finding 3).

## Findings

### 1. issue — the documented Swift test loop silently tests a stale Zig library

`native/swift/Package.swift:5-6,14` links `libskgo_native_core.a` from
`../core/zig-out/lib` through `unsafeFlags(["-L", …])` plus `linkedLibrary`.
SwiftPM does not track that archive as an input, so after `zig build install`
produces a new library, `swift test` with an existing `.build` reuses the
previously linked test bundle. `native/swift/README.md:9-12` tells a developer
to run exactly that sequence.

Reproduction (scratch copy at HEAD): apply M13 to
`query_response.zig` (`.kind = .remote` for HTTP errors), run
`zig build install` (archive md5 changes from `c43c2b72…` to `4d33e947…`),
then `swift test --filter completedQueriesPreserveHTTPRemoteAndRedirectErrors`:

- with the existing `.build`: 1 test passed;
- with `--scratch-path` pointing at an empty directory: fails with
  `(state.error → .remote(403, "Forbidden")) == (expected → .http(403, "Forbidden"))`.

Touching a Swift source recompiles the module but still does not relink the
test bundle against the new archive (verified with four mutations).

Impact: the CI proof is sound because `native/swift_test.go:28-30` uses
`t.TempDir()` as the scratch path. The developer loop the README prescribes is
not: a green `swift test` after a Zig change is "a passing command that is not
evidence" in CLAUDE.md's sense, and it is precisely how a Zig regression would
reach a PR with a green local run. Fix shape: have `Package.swift` declare the
archive as a binary target or resource, or make the README (and a `just`
recipe if one is added) pass a fresh `--scratch-path`, and say why.

### 2. nitpick — a hung Swift query test is not ended by its `.timeLimit`

`QueryTests.swift:99` awaits a detached `Task { try await c.refresh() }` at
line 111. Under mutation M15 (pending tickets survive `fail`), that await never
completes, and `swift test --filter commandRefreshes…` ran past the 600-second
harness timeout despite `.timeLimit(.minutes(1))` on line 86: the time limit
cancels the test task, but a detached task's `value` is not cancellable from
the outside, so the process does not exit. Only the 3-minute context in
`native/swift_test.go:22` converts this into a CI failure. Awaiting with
`withThrowingTaskGroup` or checking the value under a bounded `withTimeout`
helper would make the mutant fail in seconds.

### 3. nitpick — two leftovers from round 02

- `RemoteClient.swift:175-184` (`finishQuery`) rejects twice: `cache.receive`
  already records the fault with the right kind inside Zig, so the Swift kind
  mapping in `Query.swift:76-85` is reachable only for transport errors; M11
  survives because the `.http` arm is dead for wire errors. One rejection path
  would remove the duplicate.
- `Query.swift:43-46` (`RemoteQuery.deinit`) spawns an async release on
  deallocation, which the README calls a fallback. The plan says explicit
  retain/release replaces GC mechanics; this keeps lease counts timing-dependent.

## Outcome

material findings remain
