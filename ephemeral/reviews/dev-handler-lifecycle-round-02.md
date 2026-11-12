# skgo #452 independent adversarial review, round 02

Outcome: **material findings remain**.

Target: the entire current uncommitted cohesive lifecycle repair in `/Users/tyler/.codex/worktrees/dev-handler-lifecycle-fix/skgo`, including the round-01 cancellation correction. This review describes the candidate before any subsequent terminal-signal repair. Reviewer: independent Codex agent `/root/lifecycle_validation`, inheriting model `gpt-6.1-sol` without override, per the caller and the previously inspected Gimbal role metadata. No delegation, implementation/test/workflow/outcome/worklog edits, commits, or delivery actions occurred. Only this new review file was written; round 01 is preserved.

## Finding

### Issue: terminal Ctrl-C still directly terminates the generated child before application cleanup

Evidence: `internal/hotreload/host.go:336` creates the child using `exec.CommandContext`, and the following new Cancel/WaitDelay configuration requests graceful EOF only when the parent context is cancelled. Start does not isolate the child's process group. Generated main (`hostSource`, lines 719 onward) does not register a signal handler. The CLI handles interrupt/SIGTERM via its own `signal.NotifyContext` (`cmd/skgo/main.go:45`). Terminal Ctrl-C is delivered to the foreground process group, so it reaches the generated child directly as well as cancelling the CLI's context. The child's default interrupt behavior can terminate it before EOF/draining and generated-main defers. The graceful parent context callback cannot stop that direct signal.

This remains a gap in the authoritative outcome's “final shutdown cleans the last generation once” and its requirement to assess actual generated development-host behavior. It is the usual interactive CLI shutdown path, not a requirement to survive arbitrary hostile process termination.

I reproduced the inherited signal path on the actual generated executable, with source unchanged. A Go test overlay temporarily used a copy of the retained lifecycle fixture stored in this allowed review file. Immediately after initial `handler-101` and `/state`, it inspected parent and child process groups, sent SIGINT to the child alone (avoiding an interrupt to unrelated processes in the group), waited for Done, called Host.Close, and required the literal final receipt:

```go
parentGroup, _ := syscall.Getpgid(os.Getpid())
childGroup, _ := syscall.Getpgid(host.PID)
t.Logf("foreground signal routing: parent_pgid=%d child_pgid=%d", parentGroup, childGroup)
if err := syscall.Kill(host.PID, syscall.SIGINT); err != nil { t.Fatal(err) }
<-host.Done()
if err := host.Close(); err != nil { t.Fatal(err) }
receipt, readErr := os.ReadFile(filepath.Join(work, "closed"))
t.Logf("child SIGINT: child=%s resource_open=%t close_receipts=%q read_error=%v", pid, resourceOpen("handler-101"), receipt, readErr)
if string(receipt) != "handler-101\n" {
    t.Fatalf("last generation Close bypassed by inherited terminal SIGINT: got=%q want=%q", receipt, "handler-101\n")
}
return
```

Actual command, exit **1**:

```sh
GOTOOLCHAIN=go1.27.3 mise -C example exec -- go -C .. test -overlay <(python3 -c 'import json,os; print(json.dumps({"Replace":{os.path.abspath("internal/hotreload/host_lifecycle_test.go"):os.path.abspath("ephemeral/reviews/dev-handler-lifecycle-round-02.md")}}))') -tags=skgo_qualification ./internal/hotreload -run '^TestHostReloadRetiresIdleApplicationResource$' -count=1 -v
```

The review-only source copy was replaced by this completed report after execution; no repository source was changed. Personally observed:

```text
GET /: status=200 body="handler-101" child=18729
GET /state: status=200 body="completed=1 active=0" child=18729
foreground signal routing: parent_pgid=18675 child_pgid=18675
child SIGINT: child=18729 resource_open=false close_receipts="" read_error=open .../closed: no such file or directory
last generation Close bypassed by inherited terminal SIGINT: got="" want="handler-101\n"
--- FAIL: TestHostReloadRetiresIdleApplicationResource (1.72s)
FAIL github.com/tylergannon/skgo/internal/hotreload 1.988s
```

The same process group is personally observed. Delivery to both processes by an interactive Ctrl-C follows terminal process-group semantics; this reproduction intentionally signals only the owned child. Its descriptor is released by death, while the absent receipt proves application Close did not run. Host.Close returned nil. Isolate the generated child from the terminal's foreground group or give that child a deliberate graceful signal path, and demonstrate the actual terminal delivery arrangement along with held native cleanup. A private child need not independently own the CLI's user-facing signal policy.

## Round-01 disposition

The previous context-cancellation finding is **resolved for programmatic session-context cancellation**. The custom Cmd.Cancel closes the command pipe, and WaitDelay keeps a bounded process fallback. I independently observed the added second session retain the live native resource after cancellation and join it before application Close. This does not protect against a signal independently delivered to the child, which is the remaining finding above.

## Full scope inspected

Re-read the authoritative repair outcome, repository instructions and adversarial-review instructions, current full production diffs, full new generated-runtime helper, full lifecycle fixture changes, native bridge test, and cohesive worklog including the accepted round-01 defect. Rechecked surrounding generated-host construction/publication/acquisition/retirement/start/close/control code, session Start/stop and CLI cancellation, native Handler and fetch/session lifetimes. The earlier round inspected installed Kit **3.1.1**, its Vite dev SSR boundary, and required current-Kit guidance; those sources and dependency pin are unchanged.

The current implementation retains the constructor-returned owner apart from wrappers; publication and HTTP acquisition use a common lock; retired HTTP owners remain live until release; native bridge cleanup cancels and joins logical downstream/fetch work before application Close. Rejected owner-plus-error and native-registry-rejected candidates close without replacing the healthy current generation. Application obligations explicitly cover ownerless construction failure, mutable globals, cross-generation aliases/shared pools, and app work escaping the supported lifetime. No arbitrary resource registry, finalizers, broad architecture rewrite, or proof framework was introduced. I found no additional material defect in those reviewed paths.

## Independently executed populated checks

### Generated host, captured host, and native lifetime

Exit **0**, three top-level tests, no skips:

```sh
GOTOOLCHAIN=go1.27.3 mise -C example exec -- go -C .. test -tags=skgo_qualification ./internal/hotreload -run '^(TestHostReloadRetiresIdleApplicationResource|TestCapturedNativeHTTPHost|TestGeneratedNativeLifetime)$' -count=1 -v
```

Personally observed actual generated child **18473** serving `handler-101`, `handler-202`, `handler-303`, and `handler-505` under the same PID; both `/state` snapshots were `completed=1 active=0`. The first acknowledged reload had `predecessor_open=false replacement_open=true close_receipts="handler-101\n"`. Handler 303 served while handler 202's gate and resource remained held; release returned exactly `held-handler-202:literal-resource-use-713` before retirement.

Candidate 404 returned the missing-SSR-registry diagnostic, candidate 405 returned `literal-constructor-rejection-405`, and handler 303 remained usable after each. Rejected locks became free. Native `/begin` returned **200 resolve=true** before logical middleware ended; replacement 505 served while 303's final native use stayed held and reload had not acknowledged. Resource use completed before Close when the gate opened. Explicit Host.Close similarly joined final native work and was idempotent.

Observed literal final sequence for the first child:

```text
close_receipts="handler-101\nhandler-202\nhandler-404\nhandler-405\nhandler-303\nhandler-505\n"
events="closed-handler-101\nused-handler-202\nclosed-handler-202\nclosed-handler-404\nclosed-handler-405\nused-handler-303\nclosed-handler-303\nused-handler-505\nclosed-handler-505\n"
```

The same compiled executable was reused for child **18474**, serving literal `handler-606`. Native `/begin` returned **200 resolve=true**. Cancelling its session context reached the parent-owned native teardown gate while its OS resource lock remained held and the child remained alive. Releasing the gate allowed the last descriptor write, then exactly one `handler-606\n` Close receipt; Host.Close reaped it and the lock became free. Literal event suffix was `used-handler-606\nclosed-handler-606\n`.

The captured-host check passed. The native lifetime check reported `go1.27.3 darwin/arm64, CGO_ENABLED=0, no C compiler`; it observed held native revision results before reclaiming old mappings, retained closure value **71** with mapped ranges after swap/GC, and bounded refusal when the retained revision budget was exhausted. Package time **6.535s**, lifecycle test time **1.81s**.

### Native bridge

Exit **0**, nine top-level tests, no skips:

```sh
GOTOOLCHAIN=go1.27.3 mise -C example exec -- go -C .. test . -run '^(TestNativeDevelopmentCloseJoinsFetchAfterHTTPReturn|TestNativeBridge.*)$' -count=1 -v
```

Passed action effects, independent loads/demanded parent, fetch body/metadata/unread cancellation, concurrent fetch replies, parent failure and Go cancellation, concurrent load effects, streaming continuation body, unfinished-producer cancellation, and the new fetch-hook lifetime check. Package time **0.825s**. The last populated check asserts that `/fetch-begin` returned state `next`, closing reaches a held post-continuation hook, closing cannot finish until release, and hook output is exactly `literal-fetch-last-use-829`. The check emits PASS; the literal is an asserted value rather than an extra printed log.

### Other evidence and limits

`git diff --check` exited **0** on the inspected candidate. The terminal-signal overlay exited **1** as recorded above. The worklog records the implementer's earlier ordinary and real Vite/Go session checks; these are not claimed as personally executed by this reviewer. A final stable ordinary recipe remains the implementer's obligation after material corrections. No browser is needed to demonstrate these resource/process lifetime changes. No release, deployment, memory-warning diagnosis, or whole-feature completion is claimed while the finding remains.
