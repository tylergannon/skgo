# Declared Inputs lifecycle test supplement — round 09

Outcome: **material findings remain**.

Target: implementation `39d7218` and the current untracked `internal/adapter/prerender_inputs_lifecycle_test.go`, SHA-256 `4702c4aa13e595748145ad07f93dd73f1823fe0ed21896ad984c69bf8acb07da` when inspected. This completes the test audit requested after immutable round 08 was written. Round 08's native async queue integration blocker remains unchanged; this is not merge/release approval.

Read all 660 lines of the lifecycle test, all 612 lines of the 39 maintained build fixture, the helper changes reviewed in round 08, and the installed Go 1.27.1 `os/exec` implementation/documentation. No product/test changes or test executions were performed by this reviewer. Findings are source-backed causal cases, not claimed new runtime reproductions.

## 1. Issue: the lifecycle harness can hang or race its own Wait during timeout cleanup

**Requirement:** ordinary Go lifecycle tests own fixture processes, deadlines and cleanup independently of the behavior being tested. A failing owner must produce a bounded test failure and must not strand the harness on inherited pipes.

`prerender_inputs_lifecycle_test.go:427–436` uses `exec.CommandContext(...).CombinedOutput()` and registers `killRecordedInputsGroup` only after that call returns. The controlled-drain test repeats the ordering at lines 271–272. Neither sets a nonzero `WaitDelay`. Killing the direct Node command when the context expires does not close stdout/stderr inherited by a surviving detached Go process or descendant; `CombinedOutput` can continue waiting for those pipes, so the cleanup registration is never reached. The native owner/Go descendants are exactly the process relationships these tests are supposed to challenge.

The signal tests register cleanup earlier, but lines 94–105 and 137–148 kill and `Wait` the outer command before killing the recorded Vite owner and Go group. If the descendants still own the output pipes, that join delays or prevents the actions needed to unblock it. Furthermore, `waitInputsCommand` has already launched a `cmd.Wait()` goroutine at line 568; after its timeout calls `t.Fatalf`, the cleanup callback may call `Wait` again based on the concurrently changing `ProcessState`. The installed Go source explicitly says Wait must not be called concurrently; `ProcessState` is not a synchronized single-wait ownership mechanism.

**Causal case:** regress owner cancellation so the signaled command or its descendants retain a pipe. The test's 20-second select expires, but cleanup can join the outer process before terminating the remaining pipe owners. A startup/readiness timeout reaches the same ordering before a Wait goroutine has even been created. A context timeout in the programmatic harness kills only Node and can remain blocked waiting for detached descendants. Finite fixture sleeps may eventually hide the bad ordering, but they do not make the advertised deadline an independent bound.

Register rescue cleanup before starting/waiting, keep exactly one Wait owner and completion channel, terminate recorded owned groups/owner before joining that channel, and bound pipe waiting independently (for example with `Cmd.WaitDelay` plus explicit rescue cleanup). The deadline-failure branch should drain/close output before reading a concurrently written buffer. Rescue must only run after the normal literal zero-child/private-directory/no-late-I/O assertions have failed; it must never make their pre-cleanup observation pass.

## Other reviewed proof boundaries

The lifecycle fixtures do distinguish direct-owner and outer-VP SIGTERM, and the programmatic observation records group/private-directory state at the outer build rejection. Controlled signaling denial exercises retained failed ownership and a later successful retry; this directly targets the round-05 drain-error issue. The compiler and protocol tests use real Go command variants, including a successful zero-exit control. These remain useful once their own failure-path cleanup is reliably bounded.

The maintained producer-failure case starts descendants inside the producer; the independent producer-waits-for-blocked-body fixture remains necessary for the stronger concurrent-body requirement. The full example fixture uses concurrency 4 and real Go loads; it cannot replace the minimal queue regression from round 08. The current successful JS/TS and inert-Goja fixture source is consistent with those limited claims. Its custom error hook records original private diagnostics and deliberately chooses public text; this does not independently establish the no-hook native fallback, which remains a separate policy proof.

No additional product-code defect was identified in this supplement beyond the unresolved native async queue integration recorded in round 08. Final qualification still requires the authoritative seven groups and exact-head checks; the published feature must remain draft while the native no-patch constraint prevents a correct queue fix.
