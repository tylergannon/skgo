# skgo #452 independent adversarial review, round 03

Outcome: **no findings**.

No material findings or genuine nitpicks remain in the reviewed candidate. Both prior material findings are repaired and independently demonstrated below. This is a whole-candidate review against the cohesive repair outcome, not acceptance of only the original baseline reproduction.

## Target and independence

Reviewed the entire uncommitted repair in `/Users/tyler/.codex/worktrees/dev-handler-lifecycle-fix/skgo`, based on main `a0da95ecd54cb92aa98592a25110b98e911db3de`, including the programmatic-cancellation and terminal-signal corrections. Reviewer: independent Codex agent `/root/lifecycle_validation`, inheriting `gpt-6.1-sol` without an override, per the caller and previously inspected Gimbal role metadata. The implementation role metadata also names `gpt-6.1-sol`; planner `gpt-6-astra` is a separate role.

Read-only except for this new review artifact. No implementation, tests, outcomes, workflows, worklog, or documentation were edited by this reviewer. No delegation, commits, pushes, merges, deployment, or release occurred. Previous review rounds remain intact. No caller instruction narrowed the defects or subject matter considered.

## Evidence and reasoning

Inspected repository AGENTS.md, applicable agent-protocol/adversarial-review requirements, the complete authoritative `ephemeral/dev-handler-lifecycle-repair-outcomes.json`, the cohesive repair worklog and its two accepted corrections, the complete relevant production diff (`dev_bridge.go`, `internal/hotreload/host.go`), full new `internal/hotreload/handler_runtime.go`, and all changed regression code/fixtures (`dev_bridge_test.go`, `internal/hotreload/host_lifecycle_test.go`). Rechecked surrounding host construction/start/control/close, CLI signal context and real development-session Start/stop, native Handler/response callbacks, logical request begin/acquire/end/close, and fetch exchange activity/cancellation. Required current-Kit guidance and installed Kit **3.1.1**/Vite SSR boundary were inspected during this review series; the dependency pin and mirrored behavior are unchanged.

The constructor-returned owner is retained separately from native and hot-handler wrappers. Publication and HTTP acquisition share a lock, so a request acquires its generation before retirement can observe it idle. Held HTTP requests retain their resources; after their supported use ends, native bridge shutdown cancels and joins logical downstream and fetch-hook work before calling application Close. Each generation has one retirement path and one owner Close, and the shutdown wait group joins older retired generations as well as the last generation. Failed constructors returning an owner and native-registry rejection close the transferred owner before returning the error; the healthy generation is preserved.

Explicit parent Close and cancellation now request control-pipe EOF rather than immediate process kill. Generated main stops HTTP service before joining every generation. Direct SIGINT/SIGTERM is handled by generated main through a signal-select beside decoded control commands, so signal handling does not depend on interrupting the inherited pipe's blocking read. The prior normal cancellation and foreground signal defects are resolved. Parent process errors are published before Done closes and surfaced from Close for abnormal child exit; successful exit after context cancellation is accepted. Forced shutdown remains bounded by the shared 30-second fallback.

Application obligations are explicit: ownerless construction failure cleanup belongs to the application; mutable globals need synchronization/lifetime discipline; cross-generation aliases and shared pools require app-owned leases; escaped application work must be joined by Close before resources are freed. These correctly avoid claiming arbitrary resource-sharing inference. No finalizers/AddCleanup, arbitrary global registry, pointer-swap cleanup, unrelated architecture rewrite, or new proof framework was added.

## Personally executed qualification and running-host evidence

Exit **0**, three populated top-level tests, no skips:

```sh
GOTOOLCHAIN=go1.27.3 mise -C example exec -- go -C .. test -tags=skgo_qualification ./internal/hotreload -run '^(TestHostReloadRetiresIdleApplicationResource|TestCapturedNativeHTTPHost|TestGeneratedNativeLifetime)$' -count=1 -v
```

Package time **6.438s**. The lifecycle fixture compiles and starts the actual generated development executable and drives public Host.Reload through its control pipe. Raw OS descriptor locks are probed through independently opened file descriptions; Close receipts and successful final descriptor writes complement those independent lock observations. Literal expected identities/order are fixture constants rather than values derived from the framework implementation.

Actual generated child **20512** served HTTP 200 with literal bodies `handler-101`, `handler-202`, `handler-303`, and `handler-505` under the same PID. Both initial and replacement `/state` returned `completed=1 active=0`. After the first acknowledged reload:

```text
resources after reload: predecessor_open=false replacement_open=true close_receipts="handler-101\n"
```

Handler 303 served while handler 202 was deterministically held behind the parent-owned HTTP gate, and both resource locks remained held. Releasing the gate yielded exactly `held-handler-202:literal-resource-use-713`; only then did 202's lock become free and its Close receipt appear.

Candidate 404 was rejected with `skgo: native development requires the application's SSR registries`; candidate 405 was rejected with `literal-constructor-rejection-405`. After each, the same child still served `handler-303`. Both rejected candidates' resources became free and each recorded one Close.

Native `/_skgo/dev/begin` returned **200 resolve=true** before its logical middleware ended. While replacement cancelled the old native lifetime, the old middleware's last resource use stayed held, the old 303 lock stayed held, new handler 505 served, and reload had not acknowledged. Releasing the native gate permitted its real descriptor write before Close and successful reload acknowledgement. Explicit parent Host.Close likewise waited for the last held native use, then reaped the child and released the final lock. A second Close did not add a receipt.

Personally observed complete first-child literals:

```text
close_receipts="handler-101\nhandler-202\nhandler-404\nhandler-405\nhandler-303\nhandler-505\n"
events="closed-handler-101\nused-handler-202\nclosed-handler-202\nclosed-handler-404\nclosed-handler-405\nused-handler-303\nclosed-handler-303\nused-handler-505\nclosed-handler-505\n"
```

The fixture reused the compiled executable for cancellation child **20515**, serving literal `handler-606`. Native /begin returned **200 resolve=true**. Cancelling the Start/session context reached the native teardown gate with the 606 resource lock still held and child still alive. Releasing it allowed final descriptor use, exactly one 606 Close, successful parent Close and a free lock. This repairs round 01's independently observed missing receipt.

The same executable was reused for signal child **20516**, serving literal `handler-707`. I personally observed:

```text
terminal delivery: parent_pgid=20432 child_pgid=20432; sending actual SIGINT to owned child=20516
```

The actual SIGINT was sent only to the owned child to preserve the test runner/unrelated processes. Despite sharing the foreground process group, the child reached its held native teardown, retained the 707 resource lock and stayed alive. After release, final descriptor use completed before exactly one Close; the child exited gracefully, parent Close succeeded, and the final lock was free. This repairs round 02's independently observed direct signal termination. Terminal group membership and signal delivery to the owned child are observed; interactive terminal delivery to the whole group was not performed.

Final exact Close sequence:

```text
handler-101\nhandler-202\nhandler-404\nhandler-405\nhandler-303\nhandler-505\nhandler-606\nhandler-707\n
```

Final exact event sequence extends the first-child events with:

```text
used-handler-606\nclosed-handler-606\nused-handler-707\nclosed-handler-707\n
```

The lifecycle test passed in **1.82s**. TestCapturedNativeHTTPHost passed in **1.99s**. TestGeneratedNativeLifetime passed in **2.38s**, reporting `go1.27.3 darwin/arm64, CGO_ENABLED=0, no C compiler`. Its native revision check observed a held old invocation finish before old mappings became unmapped, retained closure literal **71** with mapped ranges after swap/GC, and refusal when the retained-revision budget was exhausted. These checks also guard surrounding generated-host/native lifetime behavior.

## Personally executed native bridge checks

Exit **0**, nine populated top-level tests, no skips:

```sh
GOTOOLCHAIN=go1.27.3 mise -C example exec -- go -C .. test . -run '^(TestNativeDevelopmentCloseJoinsFetchAfterHTTPReturn|TestNativeBridge.*)$' -count=1 -v
```

Package time **0.731s**. Passed action effects with/without hooks, independent loads/demanded parent, fetch body/metadata/unread cancellation, concurrent fetch replies, parent failure with Go cancellation, concurrent load effects, streaming continuation body, unfinished-producer cancellation, and the new fetch-hook lifetime check.

The last check asserts that /fetch-begin has returned state `next`, bridge cancellation reaches a held post-continuation HandleFetch hook, finish cannot return before release, and final hook output equals `literal-fetch-last-use-829`. PASS is printed; that literal is an asserted hook value rather than a separate emitted runtime log. Together with the generated host's real downstream/resource observations, this covers native work outliving an outer HTTP call and application cleanup after native work finishes.

`git diff --check` exited **0** on this candidate.

## Proof limits and remaining delivery boundary

This verdict covers implementation correctness and the independently observed running-host ownership behavior above. The worklog also records implementer-run ordinary and real Vite/Go session checks; those are not claimed as personally run by this reviewer. The caller must finish/record the required stable `just test` and focused development-session results after the latest signal correction before declaring the entire workflow outcome complete. This reviewer does not repeat the broad ordinary recipe merely to duplicate that role's obligation.

The 30-second uncooperative-child fallback was inspected in source but not deliberately exhausted, and arbitrary app resource sharing is governed by the documented application obligations. Neither limitation invalidates the supported deterministic ownership paths exercised here. No browser is needed for this Go process/resource change. This is not release, publication, deployment, a diagnosis of Teamwork's reported memory warning, or a claim that passing tests alone authorize release.
