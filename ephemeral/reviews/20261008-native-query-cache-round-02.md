# Adversarial review: milestone 5, native ordinary-query cache — round 02

## Review target

- Branch `codex/native-query-cache`, HEAD `15d1ca6` ("fix: preserve completed
  native query error kinds"), worktree `/Users/tyler/.codex/worktrees/5c57/skgo`.
  Merge base with `main` is `ad73806` (#287). Five commits on the branch:
  `6b394cc`, `c515cf3`, `4c72e42`, `2f5bf76`, `15d1ca6`.
- Scope: milestone 5 of `ephemeral/plans/zig-native-client.md` ("Native
  ordinary queries have reusable state in Zig", lines 262–274), CLAUDE.md
  rules, and installed Kit 3.0.0 at `example/web/node_modules/@sveltejs/kit`
  as the specification.
- Round 01 was requested at
  `ephemeral/reviews/20261008-native-query-cache-round-01.md` but that artifact
  was never written: the session was interrupted during a mutation run while
  the branch was being rewritten underneath it (`2d80362`/`b140206` are no
  longer in HEAD's ancestry). This round re-reviews the whole current work from
  scratch; nothing here depends on round 01.
- Caller narrowing: none. "Do not edit other project files; use scratch copies"
  are operating constraints and were honoured. All mutation runs used copies
  under the session scratchpad (`scratchpad/mut2/native`).

## Evidence inspected

Implementation (read in full):
`native/core/src/cache.zig`, `query_response.zig`, `abi.zig`, `root.zig`,
`model_json.zig`, `cli.zig`, `tests.zig`, `cache_test.zig`, `build.zig`,
`build.zig.zon`, `native/mise.toml`; `native/swift/Sources/SKGoNative/Query.swift`,
`RemoteClient.swift`, `native/swift/Sources/CSKGo/include/skgo.h`,
`native/swift/Tests/SKGoNativeTests/QueryTests.swift`, `native/swift/README.md`;
`native/remote_test.go`, `native/swift_test.go`,
`internal/gen/swift_darwin_test.go`, `internal/gen/swift_test.go`,
`internal/swiftgen/swift.go`; `remote.go` `serveQuery` (lines 813–850);
`ephemeral/worklog/20261008-native-query-cache.md`.

Kit 3.0.0 sources used as the spec:
`src/runtime/client/remote-functions/query/instance.svelte.js` (`#run`,
`#get_promise` line 91, `set` 239–251, `fail` 260–267),
`shared.svelte.js` (`remote_request` q handling 140–155,
`fail_unhandled_refreshes` 195–204), `cache.svelte.js` (proxy_count /
FinalizationRegistry), `command.svelte.js`, `query/index.js`,
`src/runtime/shared.js` (`create_remote_key`),
`src/runtime/server/remote-functions.js` (`collect_remote_data`).

Proof runs at HEAD (all green):

| Command | Result |
| --- | --- |
| `zig build test --summary all` (cold cache) | 24/24 tests pass |
| `swift test --package-path native/swift` | 14/14 pass |
| `go test -count=1 -race ./native` | ok, 13.7s |
| `go test -count=1 ./internal/gen -run 'TestGeneratedSwiftCallsTheGeneratedGoHandlers\|TestSwift'` | ok, 6.4s |

Mutation checks on the scratch copy (killed = some test fails; survived = all
run suites still pass):

| # | Mutation | Zig tests | Swift tests | Verdict |
| --- | --- | --- | --- | --- |
| M1 | skip `fail` of unhandled requested keys | killed | — | load-bearing |
| M2 | `set` keeps pending refreshes | killed | — | load-bearing |
| M3 | `resolve` settles only the latest ticket | killed | — | load-bearing |
| M4 | ignored keys carry a bogus extra key | killed | — | load-bearing |
| M6 | Swift `start()` ignores an existing in-flight request | — | killed by `retainedCanonicalQueriesShareRequestsAndOwnEvictedResults` (`.invalidWire`); hangs `cancellingOneAwaiter…` and `sessionReplacement…` indefinitely | load-bearing, see finding 5 |
| M7 | command drops requested keys | — | killed | load-bearing |
| M9 | Swift `resetSession` skips Zig `reset` | — | killed | load-bearing |
| M10 | ABI snapshot reports every fault as kind 2 | survived | killed by `completedQueriesPreserve…` | covered only via Swift |
| M12 | Swift snapshot ignores kind | — | killed | load-bearing |
| M13 | Zig rejects HTTP errors as kind remote | survived | killed by `completedQueriesPreserve…` | covered only via Swift |
| M5 | `Cache.fail` stores errors on `refs == 0` entries | **survived** | **survived** | finding 3 |
| M14 | `Cache.fail` leaves `loading = true` | **survived** | **survived** | finding 2 |
| M11 | Swift `CoreCache.reject` maps `.http` to kind remote | — | survived | dead path, finding 5 |
| M8 | epoch guard removed from `applyUpdates` | survived | — | redundant with guards in `apply`/`set`/`fail`; not a finding |

## Findings

### 1. issue — the Swift refresh-race proof asserts an ordering production never produces, and the worklog records it as the design

`native/swift/Tests/SKGoNativeTests/QueryTests.swift:53-69`
(`queryRefreshOrderingAndObservedLoading`) finishes the second-issued request
first with `[{"_":1},2]`, then the first-issued request with `[{"_":1},1]`, and
asserts the value stays `2` ("Direct results settle predecessors", line 62).
The worklog's first line generalises this into a decision: preserve the
"settles predecessors" path distinct from q/`set`.

Both fixtures omit `q`. The real server never does that: `remote.go:842-848`
(`serveQuery`) always returns `{"_": v, "q": {self: {"v": v}}}`, mirroring
Kit's `collect_remote_data`. Kit's client reads a query's value from `q` via
`entry.resource.set(result.v)` (`shared.svelte.js:150`), and `set` clears every
pending refresh (`instance.svelte.js:239-251`). So in production the
last-arriving response wins, and the superseded-predecessor branch in
`cache.zig:155` (`resolve`) is unreachable. The Zig test at
`query_response.zig:103` proves the correct last-arrival behaviour on literal
envelopes, but the Swift test is the only place the Swift client's
flight/waiter machinery meets a refresh race, and it does so on a fixture shape
the Go handler cannot emit. A Swift-side regression in how late `q` arrivals
are published to waiters and observers would pass this test.

Impact: the only Swift-layer race proof is not anchored to the handler's bytes,
and the worklog documents the opposite ordering rule to the one that applies.
The plan requires "refresh races" to be proven; this test proves a race that
cannot happen. Fix shape: drive the Swift race with the `q`-bearing envelope
the handler sends (or a recorded body from `native/remote_test.go`) and assert
last-arrival-wins; correct the worklog line.

### 2. issue — `loading` after a failed or unhandled refresh is unasserted everywhere (M14 survived all suites)

Kit's `fail` sets `#loading = false` (`instance.svelte.js:266`). `cache.zig:237-247`
(`fail`) does the same, but no test anchors it: `cache_test.zig:130-153` checks
the unhandled entry's value and 400 fault and never its `loading`;
`QueryTests.swift:100-101` asserts `ready`, `current` and `error` for `c` and
not `loading`; `queryRefreshOrderingAndObservedLoading` asserts `!loading` only
after successful `set`. Removing `entry.loading = false` from `fail` passes
Zig 24/24 and every Swift query test.

Impact: a retained query whose requested refresh goes unhandled, or whose
`q` node is an error, would be reported as loading forever through
`snapshot()` and `changes()`, which is exactly the state the plan says the API
must let a developer inspect ("inspect loading/value/error"). The behaviour is
correct today; the proof does not hold it in place.

### 3. issue — `Cache.fail`'s drop-when-unretained rule is untested (M5 survived all suites)

`cache.zig:240` returns `false` when the entry exists with `refs == 0`,
mirroring Kit, where an error for a key without a live resource is dropped
rather than stored (`shared.svelte.js:146-152` only touches `entry.resource`;
`query_responses` keeps only `{v}` nodes). The nearest test,
`cache_test.zig:70-85`, exercises a *missing* key (`"hash/missing/"`), which
takes the `orelse return false` branch on line 239, never the `refs == 0`
branch. Deleting the guard passes every Zig and Swift test.

Impact if the guard is lost: an unsolicited `q` error for a prefetched,
unretained key is stored; the next `retain` + `queryValue` sees `fault != null`
in `begin` (`cache.zig:124`) and throws the stale error without ever issuing a
request. Kit would fetch. A coverage gap on a Kit rule the plan names
("Kit's key and fulfillment rules").

### 4. nitpick — two departures from Kit's cache lifecycle: retain can refuse, and retain fetches eagerly

- `cache.zig:86-91` (`ensure`) returns `CacheFull` from `retain` when the
  table is full of live (`refs > 0`) entries; `RemoteClient.swift:67` defaults
  capacity to 256. Kit's `CacheController` never refuses a resource; bounded
  retention in Kit applies to unreferenced entries only. A screen that retains
  257 distinct canonical keys gets a thrown `query()` instead of a cache. Either
  make capacity bound prefetch only, or document the ceiling as an API contract.
- `RemoteClient.swift:148-159` (`query`) dispatches HTTP at retain time.
  Kit's `QueryProxy` runs `#run` lazily on first read (`instance.svelte.js:91`,
  `#promise ??= #run()`). The README line "Retained queries share … initial
  HTTP request" describes the shared flight, not the eager start. Minor, but it
  changes request counts a developer would predict from Kit.

### 5. nitpick — proof and documentation hygiene

- Five of the six query tests have no `.timeLimit`; only
  `completedQueriesPreserve…` does. Under mutation M6 two tests hung for the
  full 180s timeout each and `swift test` run directly never completes; only
  the 3-minute context in `native/swift_test.go:22` turns that into a failure.
  Add time limits so a broken client fails in seconds rather than by watchdog.
- `RemoteClient.swift:175-184` (`finishQuery`) rejects twice: `cache.receive`
  already applied the fault inside Zig with the right kind, so the Swift
  `CoreCache.reject` kind mapping (`Query.swift:76-85`) is reachable only for
  transport errors. M11 survived because the `.http` arm is dead for wire
  errors. Simplify to one rejection path.
- `native/swift/README.md:41` still says "The current native core does not yet
  implement retained queries or caching", directly above the "Retained ordinary
  queries" section that contradicts it.
- `RemoteQuery.deinit` (`Query.swift:43-46`) spawns an async release on
  deallocation. The plan says explicit retain/release "replaces" GC mechanics;
  the README calls this a fallback. It is harmless but it is the thing the plan
  said would not exist, and it makes lease counts timing-dependent in tests.

Not a finding: `ephemeral/skgo-runtime-validation.tar.gz` and
`ephemeral/skgo-runtime-pr.md` are tracked, but they are already on `main`
(`git ls-tree main`) and are not this branch's change.

## Outcome

material findings remain
