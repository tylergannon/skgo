# Adversarial review: milestone 5, native ordinary-query cache — round 06

## Review target

- Branch `codex/native-query-cache`, HEAD `48d3493` ("docs: clean Swift builds
  after native core changes"), worktree `/Users/tyler/.codex/worktrees/5c57/skgo`.
  Merge base with `main` is `ad73806` (#287). Eight commits on the branch:
  `6b394cc`, `c515cf3`, `4c72e42`, `2f5bf76`, `15d1ca6`, `5b94b7d`, `976fc20`,
  `48d3493`.
- Scope: milestone 5 of `ephemeral/plans/zig-native-client.md` (lines 262–274),
  CLAUDE.md rules, and installed Kit 3.0.0 at
  `example/web/node_modules/@sveltejs/kit` as the specification.
- Previous artifact: round 05 (`20261008-native-query-cache-round-05.md`),
  written against `5b94b7d`. The two commits since then touch only
  `native/core/build.zig.zon`, `native/swift/README.md` and two worklogs
  (`git diff --stat 5b94b7d..HEAD`). No Zig, Swift, Go or test source changed,
  so the round 05 mutation table still describes the code under review; this
  round re-ran the full proof at HEAD and re-verified the one finding whose
  fix is in the new commits.
- Caller narrowing: none. Read-only and scratch-copy constraints honoured; the
  mutation run used a copy under the session scratchpad (`scratchpad/r06/mutclean`).
- Untracked `ephemeral/reviewer-logs/` is not part of the branch and was not
  reviewed.

## Evidence inspected

- Full diff `5b94b7d..HEAD`; `native/core/build.zig.zon` (devalue pin moved
  from `20a8d45` to release revision `f4c94b9`, new content hash); `go.mod:8`
  (`devalue/v5 v5.0.0`, matching the worklog's claim); `native/swift/README.md`
  lines 8–24; `native/swift/Package.swift`; `native/swift_test.go:21-32`;
  `.github/workflows/ci.yml:69-90` (`native-swift` job runs
  `go test -count=1 -race ./native ./internal/swiftgen`); both new worklog
  entries; the round 02 and round 05 artifacts.
- Implementation and tests as read in rounds 02 and 05 (`cache.zig`,
  `query_response.zig`, `abi.zig`, `cache_test.zig`, `Query.swift`,
  `RemoteClient.swift`, `QueryTests.swift`, `remote.go` `serveQuery`), unchanged
  at HEAD (`git diff --stat 5b94b7d..HEAD -- native/core/src native/swift/Tests
  native/swift_test.go` is empty).
- Kit 3.0.0 sources used as the spec, unchanged: `query/instance.svelte.js`
  (`#run`, `refresh`, `set`, `fail`), `shared.svelte.js` (`remote_request`,
  `fail_unhandled_refreshes`), `cache.svelte.js`, `command.svelte.js`,
  `src/runtime/server/remote-functions.js`.

Proof at HEAD `48d3493`, run this round (all green):

| Command | Result |
| --- | --- |
| `zig build test install --summary all` with an empty `ZIG_GLOBAL_CACHE_DIR` (forces a fresh fetch of the new devalue pin) | 10/10 steps, 25/25 tests pass |
| `swift test --package-path native/swift --scratch-path <empty dir>` | 14/14 pass |
| `go test -count=1 -race ./native` | ok, 11.0s |
| `go test -count=1 ./internal/gen -run 'TestGeneratedSwiftCallsTheGeneratedGoHandlers\|TestSwift'` | ok, 7.3s |

Verification of the round 05 finding 1 fix, on the scratch copy at HEAD,
mutation M13 (`query_response.zig:12`, HTTP errors rejected with kind
`.remote`; archive md5 `c24bd6d3…` → `19ac43ac…` after `zig build install`):

| Step | `completedQueriesPreserveHTTPRemoteAndRedirectErrors` |
| --- | --- |
| `swift test` reusing the existing `.build` | passes (stale link, the trap) |
| `swift package clean` then `swift test` | fails: `(state.error → .remote(403, "Forbidden")) == (expected → .http(403, "Forbidden"))` |

So the README's new step is load-bearing: the documented loop now catches a
Zig regression that the previous loop let through.

## Status of round 05 findings

1. Documented Swift loop tests a stale Zig library — **fixed**.
   `native/swift/README.md:11` inserts `swift package --package-path
   native/swift clean` between the Zig build and `swift test`, and lines 21–24
   explain why (SwiftPM does not track the external archive) and that the
   Go-driven tests use a fresh temporary build directory, which
   `native/swift_test.go:28-30` confirms. Verified by mutation above. The
   root cause in `Package.swift` (`unsafeFlags` + `linkedLibrary` against an
   untracked archive) is unchanged, so the guard is procedural rather than
   structural; CI is unaffected because it only runs the Go wrapper. Accepted
   as the stated developer contract.
2. Hung Swift test not ended by `.timeLimit` — **unchanged** (finding 1 below).
3. Double reject in `finishQuery` and `deinit` async release — **unchanged**
   (finding 2 below).

Also checked this round: the devalue refresh (`976fc20`) builds and passes
from a cold Zig cache, so the new hash is fetchable and the 25 Zig tests,
including the devalue-dependent codec tests, still pass against release
`f4c94b9`; the Go module already pins the same release, so the two codec
sources agree on version. Nothing in milestone 5's acceptance list (shared
request counts, refresh races, error state, bounded retention, results
readable after eviction, confinement, session/server discard) regressed, and
the round 02/05 table of load-bearing mutations (M1–M4, M6, M7, M9, M10, M12,
M13, M14, M15 in Zig) still applies to this identical source.

## Findings

### 1. nitpick — a hung Swift query test is not ended by its `.timeLimit`

`native/swift/Tests/SKGoNativeTests/QueryTests.swift:99` starts
`let pending = Task { try await c.refresh() }` and awaits its value at line
111. Under mutation M15 (pending tickets survive `Cache.fail`), that await
never completes and `swift test --filter commandRefreshes…` runs past the
harness timeout despite `.timeLimit(.minutes(1))` at line 86: the limit
cancels the test task, but a detached `Task`'s `value` is not cancellable from
outside, so the process does not exit. Only the 3-minute context in
`native/swift_test.go:22` turns this into a CI failure. Awaiting inside a task
group, or racing the value against a bounded sleep, would make the mutant fail
in seconds. Carried unchanged from round 05.

### 2. nitpick — two leftovers from round 02

- `native/swift/Sources/SKGoNative/RemoteClient.swift:175-184` (`finishQuery`)
  rejects twice: `cache.receive` already records the fault with the right kind
  inside Zig, so the Swift kind mapping in `Query.swift:76-85` is reachable
  only for transport errors; mutation M11 survives because the `.http` arm is
  dead for wire errors. One rejection path would remove the duplicate.
- `Query.swift:43-46` (`RemoteQuery.deinit`) spawns an async release on
  deallocation. The plan says explicit retain/release replaces GC mechanics;
  the README calls this a fallback. It keeps lease counts timing-dependent.

Carried unchanged from round 05.

### 3. nitpick — worklog hygiene

`ephemeral/worklog/native-query-cache.md` (new in `48d3493`) begins with a
blank line and contains only a note about the review agent's CLI turn
timeout. That is process friction for the reviewer loop, not intelligence
about the native cache that would change a future decision on this code; the
substantive correction and decision lines already live in
`ephemeral/worklog/20261008-native-query-cache.md`. Either fold the note into
the dated worklog or drop it.

## Outcome

only nitpicks remain
