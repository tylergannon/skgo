# skgo #452 independent adversarial review, round 01

Outcome: **material findings remain**.

Target: the uncommitted cohesive lifecycle repair in `/Users/tyler/.codex/worktrees/dev-handler-lifecycle-fix/skgo`, based on main `a0da95ecd54cb92aa98592a25110b98e911db3de`. This verdict covers the candidate inspected before the caller's subsequent cancellation repair. No implementation, tests, outcomes, workflow definitions, or worklog were edited by this reviewer. No commits, delivery actions, or delegation occurred.

Reviewer: independent Codex agent `/root/lifecycle_validation`. The parent reports this session inherits `gpt-6.1-sol` without an override; `.gimbal/runs/01M78F4P9NJXAEBMV575T67AM9/model_calls.json` independently identifies the parent coding role as `gpt-6.1-sol`. The model is metadata-derived, rather than guessed from behavior.

## Material finding

### Issue: ordinary development-session cancellation still kills the final generation before Close

Evidence: `internal/hotreload/host.go:334` creates the long-lived child with `exec.CommandContext(ctx, h.binary)` and leaves its default cancellation function in place. The new graceful `Host.Close` implementation at lines 424-454 only closes the control pipe when it is explicitly called. The actual development session passes its own context to `host.Start` (`internal/devserver/session.go:251`), returns when that context is cancelled (lines 326-327), and only then invokes its deferred `closeHost(active)`. The CLI context is cancelled by interrupt/SIGTERM (`cmd/skgo/main.go:45`). Consequently Go's CommandContext watcher can call `Process.Kill` immediately on the same ordinary shutdown event, independently of the EOF/draining path. This also races a `Host.Close` already waiting for graceful cleanup. Killing the child bypasses generated-main defers and application Close; `Host.Close` then sees Done and returns nil.

This misses the cohesive outcome's final-shutdown obligation: “final shutdown cleans the last generation once,” including actual generated development-host behavior. The existing qualification demonstrates explicit Host.Close while its Start context is still live, so it does not cover this normal caller path.

I reproduced the failure using the actual generated host and retained resource fixture, with no implementation edits. I temporarily stored a copy of `host_lifecycle_test.go` in this permitted review-artifact path, and selected it with Go's `-overlay`. Immediately after the initial `handler-101` and `/state` assertions, the copy executes:

```go
cancel()
<-host.Done()
if err := host.Close(); err != nil { t.Fatal(err) }
receipt, readErr := os.ReadFile(filepath.Join(work, "closed"))
t.Logf("session cancellation: child=%s resource_open=%t close_receipts=%q read_error=%v", pid, resourceOpen("handler-101"), receipt, readErr)
if string(receipt) != "handler-101\n" {
    t.Fatalf("last generation Close bypassed by session cancellation: got=%q want=%q", receipt, "handler-101\n")
}
return
```

The rest of the original fixture remains in the overlay to preserve imports and its expensive generated-host setup. The overlay JSON maps the absolute original test path to the absolute review-artifact path. This report replaces that temporary review-only source copy; repository source files were never changed.

Actual command, exit **1**:

```sh
GOTOOLCHAIN=go1.27.3 mise -C example exec -- go -C .. test -overlay <(python3 -c 'import json,os; print(json.dumps({"Replace":{os.path.abspath("internal/hotreload/host_lifecycle_test.go"):os.path.abspath("ephemeral/reviews/dev-handler-lifecycle-round-01.md")}}))') -tags=skgo_qualification ./internal/hotreload -run '^TestHostReloadRetiresIdleApplicationResource$' -count=1 -v
```

Personally observed literal output, child **16934**:

```text
GET /: status=200 body="handler-101" child=16934
GET /state: status=200 body="completed=1 active=0" child=16934
session cancellation: child=16934 resource_open=false close_receipts="" read_error=open .../closed: no such file or directory
last generation Close bypassed by session cancellation: got="" want="handler-101\n"
--- FAIL: TestHostReloadRetiresIdleApplicationResource (1.86s)
FAIL github.com/tylergannon/skgo/internal/hotreload 2.191s
```

The OS releases the descriptor on process death, but the absent Close receipt proves that this is not application cleanup. No HTTP/native work was active, so the failure does not depend on an uncooperative application or the 30-second fallback. Repair the process-cancellation path to request the same graceful shutdown with a bounded forced-exit fallback, and demonstrate cancellation with the generated fixture, including supported held native work.

## Evidence inspected

- Repository `AGENTS.md`, agent-protocol and adversarial-review skill instructions; no caller scope narrowing was present.
- The complete `ephemeral/dev-handler-lifecycle-repair-outcomes.json` and cohesive repair section of `ephemeral/worklog/dev-handler-lifecycle.md`, retaining the historical baseline as baseline-only evidence.
- Full relevant diffs for `dev_bridge.go`, `dev_bridge_test.go`, `internal/hotreload/host.go`, and `internal/hotreload/host_lifecycle_test.go`; full new `internal/hotreload/handler_runtime.go`.
- Surrounding constructor validation, host generation/start/control/close code, native Handler and response callbacks, `prerenderRequests` begin/acquire/end/close, fetch-exchange ownership/cancellation, development session start/stop, and the CLI signal context.
- Required current-Kit guidance; installed Kit package version **3.1.1** and its Vite dev SSR request/response boundary. The ownership change is in Go rather than a change to mirrored Kit semantics.
- Application obligations on NativeDevelopment explicitly cover ownerless construction failure, mutable globals, cross-generation aliases, shared pools/leases, and escaped application work. Those boundaries are appropriate and do not create an arbitrary registry or use finalizers.

The source's publication/acquisition locking protects ordinary HTTP leases, and the retained original owner survives wrappers. Rejected returned owners are closed, and the current generation is published only after validation. Bridge close cancels and joins native logical downstream requests and registered fetch-hook activity before owner Close. The populated successful commands below demonstrate these paths; they do not discharge the separate cancellation defect.

## Independently executed checks and observations

1. Exit **0**:

```sh
GOTOOLCHAIN=go1.27.3 mise -C example exec -- go -C .. test -tags=skgo_qualification ./internal/hotreload -run '^TestHostReloadRetiresIdleApplicationResource$' -count=1 -v
```

A real generated executable, child **16548**, served literal `handler-101`, `handler-202`, `handler-303`, and `handler-505` under the same PID. Initial and replacement `/state` each returned `completed=1 active=0`. The first acknowledged reload showed `predecessor_open=false replacement_open=true close_receipts="handler-101\n"`. Handler 303 served while handler 202's HTTP gate remained held and both locks remained held. Releasing the gate returned exactly `held-handler-202:literal-resource-use-713`, then 202's lock became free and its receipt appeared.

Rejected candidate 404 returned the literal missing-SSR-registry error; candidate 405 returned `literal-constructor-rejection-405`. Handler 303 served after each rejection; rejected locks became free and both Close receipts appeared once.

Native `/begin` returned **200 resolve=true** while its logical middleware retained ownership. During replacement teardown, handler 303's last use remained held and its lock stayed held while handler 505 served; reload had not acknowledged yet. After release the old descriptor accepted its final write and then closed. Explicit parent Host.Close similarly waited for the final native gate, returned successfully, and a second Close did not duplicate any receipt. Final lock was free.

Personally observed final literal receipts and event order:

```text
close_receipts="handler-101\nhandler-202\nhandler-404\nhandler-405\nhandler-303\nhandler-505\n"
events="closed-handler-101\nused-handler-202\nclosed-handler-202\nclosed-handler-404\nclosed-handler-405\nused-handler-303\nclosed-handler-303\nused-handler-505\nclosed-handler-505\n"
--- PASS: TestHostReloadRetiresIdleApplicationResource (1.93s)
ok github.com/tylergannon/skgo/internal/hotreload 2.198s
```

2. Exit **0**; **nine populated top-level tests**, no skips:

```sh
GOTOOLCHAIN=go1.27.3 mise -C example exec -- go -C .. test . -run '^(TestNativeDevelopmentCloseJoinsFetchAfterHTTPReturn|TestNativeBridge.*)$' -count=1 -v
```

Passed action effects with/without hooks, independent loads and demanded parent, fetch body/metadata/unread cancellation, concurrent fetch replies, parent failure and Go cancellation, concurrent load effects, streaming continuation body, unfinished-producer cancellation, and the new fetch-hook lifetime check. Total package time **0.727s**. The last check asserts that `/fetch-begin` has returned state `next`, cancellation reaches the held post-continuation application hook, close remains blocked before release, and the hook emits literal `literal-fetch-last-use-829` before finish returns. This is an independently run assertion, not an extra printed runtime log; the check itself prints PASS.

3. The normal-session cancellation overlay above exited **1**, with actual literal failure evidence recorded under the finding. This is a nonempty selected check, not a skip or setup failure.

4. `git diff --check` exited **0** on the inspected candidate. Full ordinary `just test` is the implementer's stable-repair obligation; this reviewer was specifically asked to independently run the focused qualification and native bridge checks and does not claim an independent full-suite run.

No browser was required for these process/resource ownership changes. The generated executable and real native bridge requests provide the requested running-host observation. This review does not claim release, publication, deployment, or a diagnosis of Teamwork's memory warning.
