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
- Initial cache assessment: no root cause for native `kill EPERM` was established by the indexed material consulted. The full issue explicitly preserves contradictory passing evidence; a successful isolated rerun does not resolve it. Diagnosis must retain the first failure's process-owner/group state and original error chain. The later native counterfactual qualification appears below.
- Independent proof: preserve the real native-crawler failure with a live owned producer, and assert at rejection time that the recorded owned group is dead and the recorded private directory is absent. Keep original crawler error diagnostics and no-skip semantics. Add only a bounded regression for any verified diagnosed cleanup race; do not substitute repeated full suites, retry-until-green, or weaken the group/directory assertions.

### Design/HITL assessment

Both issues are executable now. No unanswered product/design question requires HITL: Kit selects application-error policy, and #250 already states the resource-owner contract. The implementation agent must map current installed Kit itself and diagnose the cleanup failure; historical source is context, not secondhand specification. Keep #251's outer-launcher contract and unrelated historical review findings outside this group. No common root cause between #250 and #276 is assumed.


### #250 reopening assessment — before native qualification

The user correctly challenged completion: retaining failed-drain ownership/recovery and passing an injected signal-denial regression do not establish a repair of the reported native crawler failure. #250 remains unresolved until its original native rejection-time drain behavior is genuinely qualified. Passing isolated/full reruns and Opus acceptance of the separate hardening are not causal proof. #276 is independent.

Bounded historical lookup started again at the cache index. The full original `ephemeral/tmp/test-final.log` was not recovered from the indexed `skgo-dev-868c5db`, `skgo-dev-bd7a4c9` or `skgo-current` snapshots; the exact original `e9c3` worktree/root-checkout paths checked are absent, and that path is absent from the Git trees at `68e8db304786b8beb07feaf56899789bef5b241b` and `e9dd3ba1523afc2861d5051aca1cec4d9649e922`. The downloaded #250 issue retains its first-failure excerpt, including group94451, `groupAlive:true`, retained private directory and `signalTree`/exit-fallback EPERM stack. It does not retain the group's member states, credentials, reaping state or full owner-event sequence. Do not reconstruct those missing observations from the later implementation.

Relevant historical evidence exists for **different #256 failures**, under cache prefix `repos/dev-experience-20261015/skgo-dev-bd7a4c9/`:

- `ephemeral/reviews/issue-256-opus-round-02.md:44-46` records a native Darwin25.6 platform observation: an exited-but-unreaped group's signal0 returns EPERM and returns ESRCH after reaping. Lines95-108 quote a stable-run successful-build cleanup failure at `alive()` and a separate producer failure with group absent/directory retained; the second attribution is explicitly inference.
- `ephemeral/reviews/issue-256-opus-round-03.md:68-115` explains the later narrow `signalTree`/150ms/Node-exit-listener ordering window, but line107 explicitly says **no capture shows that window**. Platform checks at lines38-41 cover TERM/KILL as well as signal0; they are not the original #250 process observation.
- `ephemeral/reviews/issue-256-opus-round-04.md:18-25,40-53` records the correction: retain EPERM without relying on Node's observed exit, then let bounded child-close and actual group absence decide cleanup success. `ephemeral/worklog/prerender-service.md:20-25` records passing runs and the user's later request to delete historical logs. The cited raw `test-output-stable.log` and `adapter-output-drain-fix.log` are absent from this cache snapshot; their surviving reviews are summaries, not full first-failure logs.

`05ec9902` merged the #256 one-service replacement and that Darwin handling. It is more than unexplained hardening for the reviewed #256 platform condition, but the surviving evidence does **not** establish that #250's older crawler rejection and exit-fallback stack had the same process state, nor provide a before/after counterfactual for #250. Keep that historical explanation as a diagnostic lead. Preserve the original native group/private-directory assertions and full first-failure diagnostics; neither synthetic denial nor green retries can close the gap.

### Native counterfactual qualification after reopening #250

The bounded experiment now reproduces the reported resource failure through the **actual historical owner and its public build-failure boundary**, rather than an injected syscall denial. The Darwin-only ordinary Go regression is `internal/adapter/prerender_native_darwin_test.go:TestPrerenderFailureWaitsForNativeExitedGroupReap`. A real compiled Go leader creates a Go reaper in a sibling process group; the reaper owns an exited child in the leader's group and holds `Wait` behind an explicit gate. The Node-owned leader has already closed before the failure boundary is invoked. This controls the remaining kernel group lifetime without leaving a running process in that group. `ps` independently verifies the exited child has state `Z`, the expected parent/group, and UID501; unmodified native signal0, TERM and KILL all return EPERM. No `process.kill` override or invented error is used. The sibling reaper is fixture control, and its eventual ESRCH plus its completed Go `Wait` receipt are asserted before the test completes.

On this host (Darwin27.0.0 arm64, xnu13432.1.9, Node24.22.0), loading the exact `68e8db304786b8beb07feaf56899789bef5b241b` adapter and exporting only its private `createJob` to attach that fixture produced group19725/stateZ. Its unchanged public `createPrerenderOwner` and `installPrerenderFailureBoundary` rejected with `probe:EPERM`, `privateDirExists:true`, `reaped:false`, and aggregate `kill EPERM`. The full **reproduced historical cleanup** stack runs through `signalTree:142`, `stopGroup:155`, and owner `cleanup:409`. The original issue instead prints `signalTree:142` from `process.onExit:341`, the exit fallback. This counterfactual reproduces the cleanup failure and retained resources, not that original printed exit-fallback stack. Capture: `01-native-eperm-historical-owner.log`. The historical adapter was loaded into the disposable test fixture; production source was not replaced with it.

A second counterfactual keeps today's owner/boundary and changes only its signalTree EPERM-retention branch to the old throw policy. The same real Go fixture rejects with group still EPERM, directory retained, and native `kill EPERM` appended to the original error. Capture: `01-native-eperm-policy-counterfactual.log`. This production mutation was restored immediately. Together these controls identify the causal behavior: treating an exited-but-unreaped Darwin group as an unrecoverable signal failure settles rejection before actual reaping. The existing `05ec9902` policy already supplies the applicable repair: retain EPERM, then await bounded child closure and actual group disappearance. No additional production rewrite is warranted.

The normal focused command was:

```text
go test ./internal/adapter -run '^(TestPrerenderFailureWaitsForNativeExitedGroupReap|TestPrerenderInputsBuildAppWaitsForUnrelatedCrawlerFailureDrain)$' -count=1 -v
```

`01-native-eperm-current.log` preserves the pre-review run: native group21996/stateZ/EPERM, followed at rejection by ESRCH, private-directory absence and actual reaping; the fixture reaper also exited. That run passed (native0.59s, unchanged real crawler3.65s, package3.947s), but did not establish fixture reliability: Opus subsequently found the `Wait` error-handling race described below. The corrected qualification is recorded after that finding. The independent real crawler/blocked Go producer, native Kit diagnostics, and rejection-time group/directory assertions were preserved. The historical and single-policy counterfactuals use that same focused native test command with only the module/export adaptation or signal-policy mutation described above; both fail on the resource/error assertions as intended.

The first missing-receipt failure is retained as `01-native-eperm-receipt-race.log`. It was initially attributed to receipt publication trailing kernel ESRCH, but that cause was not established: the same observation is also consistent with the fixture reaper panicking after `Wait` returned an exit-status error. Opus independently reproduced the latter with a 50ms leaf delay and production unchanged. The owner correctly sent SIGTERM to a leaf that had not exited yet; Go `Wait` returned `*exec.ExitError`, which still means the child was reaped, and the fixture incorrectly panicked instead of writing its receipt. The fixture now accepts that exit-status error while preserving failure on other `Wait` errors, and retains the 50ms leaf delay to exercise this timing. Rejection still asserts actual group ESRCH, positive child PID ESRCH, the explicit reap gate, and private-directory absence; the completed `Wait` receipt is checked after the fixture reaper exits.

The original94451 process-state snapshot remains unavailable. These experiments establish a real native mechanism and an old/new owner counterfactual for the same rejection/group/directory failure, not a recovered forensic observation of that lost PID. Darwin's published XNU `killpg1` implementation corroborates the mechanism: it excludes SZOMB members when counting eligible group targets and returns EPERM when the POSIX group scan finds none ([Apple XNU source](https://github.com/apple-oss-distributions/xnu/blob/main/bsd/kern/kern_sig.c)). That published main source is corroboration, not asserted to be this host's exact kernel revision.

Boundary distinction: the new native regression supplies a minimal fixture builder whose `buildApp` throws a literal original error, then invokes the adapter's actual public `installPrerenderFailureBoundary` wrapper. It does not invoke Kit's crawler itself. The separate unchanged `TestPrerenderInputsBuildAppWaitsForUnrelatedCrawlerFailureDrain` runs the installed native Kit crawler against an independent blocked Go producer and checks real Kit failure diagnostics plus rejection-time process-group/private-directory drain. The two controls establish different parts of the claim and are not interchangeable.

After the fixture correction, the same focused pair passed once: native0.64s, unchanged real Kit crawler7.17s, package7.416s. `01-native-eperm-current-wait-fixed.log` records group39973/stateZ/EPERM, actual group/child disappearance and directory absence at rejection, followed by `FIXTURE_REAPER_EXITED:reaped: signal: terminated`. This positively exercises completed reaping with `*exec.ExitError`, without requiring the leaf to exit successfully. A single fixture-only counterfactual reverted only the `Wait` error acceptance while retaining the 50ms delay and untouched production; it failed with missing receipt despite correct production ESRCH/directory removal. Its full output is preserved in `01-native-eperm-wait-exiterror-counterfactual.log` (package0.938s). The fixture mutation was restored; no production changes, broad CI runs, or extra framework builds were made for this correction.

Actual Claude Opus (`claude-opus-5-5`) accepted #250's rejection-time drain contract after the fixture correction: `01-native-opus-followup.json`. Its independent focused pair passed once in3.950s (native0.58s, real Kit crawler3.69s), with group46859/stateZ/EPERM before the boundary and group ESRCH/private-directory absence at rejection. The prior review (`01-native-opus-review.json`) also verified that treating EPERM as gone, throwing immediately on EPERM, or removing the group-absence wait each breaks the native regression; all mutations were restored. The accepted resolution combines the existing native reaping repair with this branch's retained failed-cleanup ownership/recovery hardening and the new Darwin regression. It establishes the resource contract through the paired native and real-crawler controls, without claiming the lost PID94451's exact state was recovered. The prior review's observation that its own JSON was empty was output buffering during execution, explicitly withdrawn in the follow-up.
