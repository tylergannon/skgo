# Correct prerender failure handling — #250 and #276

Captured issue records are retained in adjacent JSON files. Full bodies and comments follow, unchanged.

## #250: bug: intermittent prerender crawler failure rejects before owner drain with kill EPERM

Source: https://github.com/tylergannon/skgo/issues/250

### Original body

A native prerender crawler failure intermittently makes `buildApp` reject before its owned producer process group and private directory are drained. The captured cleanup path also throws native `kill EPERM`. Cause **UNKNOWN**; this report records the failure and contradictory passing evidence without prescribing a retry strategy.

Observed on macOS 27.0.1 arm64, Go 1.27.1, Node 24.21.0, installed SvelteKit 3.0.0, revision `68e8db304786b8beb07feaf56899789bef5b241b` in PR #249. Production process signal/drain code was unchanged by that revision.

Command (verbosity belongs on `go test`, not in `GOFLAGS`):

```sh
PATH="$PWD/.tools/bin:$PATH" go test -count=1 -v ./... ./example/...
```

`ephemeral/tmp/test-final.log` recorded 804 top-level PASS, one FAIL, zero SKIP. The failed test was `TestPrerenderInputsBuildAppWaitsForUnrelatedCrawlerFailureDrain`, after 33.08s. Concise captured excerpt:

```text
prerender_inputs_lifecycle_test.go:82: buildApp rejected before owned group/private directory drain:
  observation=map[group:94451 groupAlive:true pid:94451 privateDirExists:true]
500 GET /z-native-failure
Failed to prerender `/z-native-failure`
skgo prerender owner failure: __handled__; cleanup failed: skgo could not drain every owned prerender process tree
DRAIN_OBSERVATION:{"pid":94451,"group":94451,"groupAlive":true,"privateDirExists":true}
BUILD_APP_REJECTED:Prerendering failed
Error: kill EPERM
  at process.kill (node:internal/process/per_thread:277:13)
  at signalTree (internal/adapter/skgo-adapter/prerender.js:142:11)
  at process.onExit (internal/adapter/skgo-adapter/prerender.js:341:40)
  errno: -1, code: 'EPERM', syscall: 'kill'
--- FAIL: TestPrerenderInputsBuildAppWaitsForUnrelatedCrawlerFailureDrain (33.08s)
```

The unchanged isolated rerun passed in 18.22s:

```sh
PATH="$PWD/.tools/bin:$PATH" go test -count=1 -v ./internal/adapter -run '^TestPrerenderInputsBuildAppWaitsForUnrelatedCrawlerFailureDrain$'
```

The normal full [CI gate also passed](https://github.com/tylergannon/skgo/actions/runs/37516913536) at subsequent typed-load revision `e9dd3ba1523afc2861d5051aca1cec4d9649e922`, whose lifecycle signal/drain code is likewise unchanged. Passing reruns establish intermittence, not a root cause or repair.

Expected contract: a prerender owner must terminate and reap its owned process trees and remove its private directory before build rejection settles; a cleanup failure must remain visible and ownership must not be discarded. The fixture's native crawler error intentionally triggers cleanup; it does not inject signal denial.

Earlier remote-worktree observations included outer-CLI exit before Vite-owner cleanup completed and a separately controlled signal-cleanup recovery hang. Those are separate symptoms; no common cause with this native EPERM failure is established. Their proposed lifecycle changes are excluded from PR #249. #247 mentions unresolved cleanup failures but explicitly excludes repairing them; it is not a fix for this issue. Preserve the group/directory assertions and no-skip semantics while diagnosing the operating-system/process-owner state at failure.


<!-- agent-session-group-20261018 -->
## Coordinated agent session: Prerender failure policy and cleanup

Work together: #250, #276.

Application transform failures follow pinned Kit's configured strict/permissive build policy, while terminal failures drain owned process groups and private compilation state before rejection settles. Both touch the build owner's application-error versus terminal-transport-failure boundary, so changing them together avoids conflicting failure lifecycles. They remain independent defects: no shared cause is established, and each requires its own regression. The outer-launcher contract decision in #251 stays separate HITL work.

Use the installed pinned Kit implementation as the specification. The session is complete only when both issues' retained acceptance criteria are satisfied; partial delivery does not close the other issue.


### Comment by tylergannon — 2026-10-18T16:58:08Z

Source: https://github.com/tylergannon/skgo/issues/250#issuecomment-6252600664

**Triage — 2026-10-18**

Checked against current [`main` at `920b6aca`](https://github.com/tylergannon/skgo/tree/920b6aca738a65d6c046a67f4db2373596fafd03). Classification: `bounded-hard`. 

The prerender process-owner cleanup and TestPrerenderInputsBuildAppWaitsForUnrelatedCrawlerFailureDrain remain in current production adapter tests. Later fixture consolidation and separate build/shutdown bounds (#281/#282/#285) do not themselves explain or repair native kill EPERM. The endpoint is clear: the live owner drains owned groups/private directories before settling build rejection, or reports retained cleanup failure; preserve full first-failure diagnostics.

<!-- skgo-issue-triage-20261018-920b6aca -->

## #276: Match Kit error policy for application transformPageChunk failures during prerender

Source: https://github.com/tylergannon/skgo/issues/276

### Original body

The stage-5 asynchronous transform proxy routes an application transform error through failPredicateRelay, making the entire build terminal even when Kit handleHttpError would allow the page error. The synchronous predicate relay needs sticky terminal failure for late-reply safety; asynchronous application transform errors have no such cell hazard. Distinguish application transform errors from broken transport and demonstrate the behavior in a real build under strict and permissive Kit policies. Nonblocking behavior difference from ephemeral/reviews/20261007-request-event-stage5-opus.md finding 3.

<!-- agent-session-group-20261018 -->
## Coordinated agent session: Prerender failure policy and cleanup

Work together: #250, #276.

Application transform failures follow pinned Kit's configured strict/permissive build policy, while terminal failures drain owned process groups and private compilation state before rejection settles. Both touch the build owner's application-error versus terminal-transport-failure boundary, so changing them together avoids conflicting failure lifecycles. They remain independent defects: no shared cause is established, and each requires its own regression. The outer-launcher contract decision in #251 stays separate HITL work.

Use the installed pinned Kit implementation as the specification. The session is complete only when both issues' retained acceptance criteria are satisfied; partial delivery does not close the other issue.


### Comment by tylergannon — 2026-10-18T16:57:42Z

Source: https://github.com/tylergannon/skgo/issues/276#issuecomment-6252596164

**Triage — 2026-10-18**

Checked against current [`main` at `920b6aca`](https://github.com/tylergannon/skgo/tree/920b6aca738a65d6c046a67f4db2373596fafd03). Classification: `bounded-hard`. 

Still present in the current build bridge: asynchronous transformPageChunk catches route through failPredicateRelay, which makes the owner terminal. That is the same mechanism the report identifies; it was not removed with the old development server. Bounded endpoint: distinguish application transform errors from transport failure and prove strict/permissive pinned-Kit build policy with real builds.

<!-- skgo-issue-triage-20261018-920b6aca -->

## Cache annotations and execution contract

Cache root: `/Users/tyler/src/skgo-token-cache`; entered through `index/README.md`, then `index/upstream-docs/kit-reference.md` and `index/source-families.md`. Cache snapshots are historical context. The installed worktree package is verified as Kit **3.1.0**, which controls implementation.

### #276 — application transform failure follows Kit policy

Desired capability: a Go `transformPageChunk` application failure behaves like Kit's throwing application transform. A permissive prerender policy can continue building healthy routes; a strict policy fails. A broken bridge remains terminal, including under a permissive policy.

- The original Opus finding is preserved at `repos/architecture-20261015/skgo-dev-868c5db/ephemeral/reviews/20261007-request-event-stage5-opus.md:133-145`. It explicitly separates asynchronous transform failures from synchronous predicate cells: sticky failure exists to protect late synchronous replies; asynchronous transforms have no such hazard.
- Cached Kit policy signatures and semantics are at `repos/dev-experience-20261015/frameworks/kit-3.0.0-upstream/packages/kit/types/index.d.ts:1957-2005`; asynchronous transform shape is at `:1385-1394`. Cache index routes: `index/upstream-docs/kit-reference.md:47` and `:69`.
- Current installed Kit confirms the rule: `example/web/node_modules/@sveltejs/kit/src/runtime/server/page/render.js:617-622` awaits the transform; `src/core/postbuild/prerender.js:82-124` implements strict/warn/ignore/custom handling; `:630-637` passes non-OK responses to `handle_http_error`.
- Independent proof: real Kit builds with a deliberately failing Go transform and a healthy route. Strict policy rejects; permissive policy completes and writes the healthy route's literal fixture content while omitting the failed page artifact, as Kit does. Preserve a terminal transport-failure control under permissive policy. Assert supplied fixture values rather than output-derived expectations. One build's output can support multiple assertions.

### #250 — terminal rejection waits for owned resources

Desired capability: after a native crawler failure, `buildApp` does not settle rejection until its owned process trees have terminated/reaped and its private directory is gone. Cleanup failure remains visible and ownership remains available for cleanup rather than silently being discarded.

- The cached existing real-build regression is at `repos/architecture-20261015/skgo-dev-868c5db/internal/adapter/prerender_inputs_lifecycle_test.go:45-61`. Its independently blocked Go producer is established before a separate Kit-native server load fails (`:64-98`). This is a genuine crawler-failure boundary, not injected signal denial.
- No root cause for native `kill EPERM` is established by the indexed material consulted. The full issue explicitly preserves contradictory passing evidence; a successful isolated rerun does not resolve it. Diagnosis must retain the first failure's process-owner/group state and original error chain.
- Independent proof: preserve the real native-crawler failure with a live owned producer, and assert at rejection time that the recorded owned group is dead and the recorded private directory is absent. Keep original crawler error diagnostics and no-skip semantics. Add only a bounded regression for any verified diagnosed cleanup race; do not substitute repeated full suites, retry-until-green, or weaken the group/directory assertions.

### Design/HITL assessment

Both issues are executable now. No unanswered product/design question requires HITL: Kit selects application-error policy, and #250 already states the resource-owner contract. The implementation agent must map current installed Kit itself and diagnose the cleanup failure; historical source is context, not secondhand specification. Keep #251's outer-launcher contract and unrelated historical review findings outside this group. No common root cause between #250 and #276 is assumed.
