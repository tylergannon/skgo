# Development-server repair review, round 02

Outcome: **material findings remain**.

Target: the complete repair diff `1d9e23e1760266cc83367c00999105e28db19944..814801d` in `/Users/tyler/.codex/worktrees/5f35/skgo`. Product source was frozen during this review. Authority remains the user's request to implement the prior review findings and review the changed code, the full repository `AGENTS.md`, and `ephemeral/projects/dev-server/{GOAL,DESIGN,PLAN,CONTINUE}.md`. Read `agent-protocol` and `adversarial-review`; used the token-cache index/current native-Kit route and the installed Kit `3.0.0` source mapping carried forward from round 01. No caller instruction narrowed the defects considered. Only this review artifact was written; no shared app or generation check was started.

## Finding

### 1. issue — explicitly closing an unfinished continuation body waits for the producer instead of releasing it

`dev_fetch.go:329–332` implements the Go continuation response's `Body.Close()` by synchronously calling the underlying **incoming HTTP request** body's `Close()` and only then signaling `b.done`. The upload handler at lines 208–215 waits for that signal before acknowledging the upload. These are different ownership contracts: an HTTP client response body can be canceled by its consumer, whereas Go's HTTP/1 server request-body closer attempts to drain the incoming upload.

The actual Go `1.27.2` implementation confirms the causal path. `net/http/server.go:1098` sets `reqBody.doEarlyClose = true`; `net/http/transfer.go:1008–1043` then drains an unknown-length body in `Close()` using `io.CopyN(..., maxPostHandlerReadBytes+1)`. A chunked upload from an unfinished Web `ReadableStream` can therefore block there until the producer supplies EOF or more than 256 KiB. The bridge's `devFetchBody.Close()` has not yet signaled completion, and no body-close notification reaches the JS producer.

A valid Go `HandleFetch` hook can inspect a continuation's status, close its unused body, and return a synthetic replacement response. With an open streaming producer, that hook cannot return; the SSR page hangs until an unrelated request cancellation, producer completion or sufficient additional data. This violates `PLAN.md` chunk 3's requirement that delayed output and cancellation behave correctly, and the new body's own stated support for being closed without reading. Ending the logical request later does not repair this case because producing that request's response is waiting for the blocked hook.

My evidence is a direct source trace through the bridge and the installed standard library, not a fixture I executed. The parent separately reported that the other independent reviewer executed the exact explicit-close case through a private overlay under managed Node `24.22.0` and observed its 700 ms failure; their round-02 artifact owns that runtime evidence. The existing tests passing below do not cover a Go hook explicitly closing an unfinished continuation before returning a replacement. Keep body release/cancellation distinct from draining the producer, and add that case without weakening the progressive or concurrent-body controls.

## Round-01 finding

The concurrent-continuation premature-body-close finding is repaired at `814801d`. The upload handler now remains active while its Go consumer owns the body, and `/fetch-next` advances separately after an upload-accepted gate. The strengthened regression deliberately holds the first body unread until the second continuation returns, so it proves more than scheduling luck. I independently ran it successfully under managed Node, including the race run below. A separate test requires the first bytes to arrive before its producer is allowed to finish; this guards against solving the earlier failure by buffering the whole response.

The explicit-close finding above is a different remaining boundary in that repair; it does not repeat the resolved premature-close finding.

## Review and verification

Re-read the full relevant diff and surrounding ownership paths. Coverage included supervisor configuration publication and cancellation cleanup; readiness epochs; manifest/environment content identity; guarded Go reload and remote-prefix publication; captured compiler/workspace environment and native host views; explicit application development composition and generated/example constructors; type-resolved FetchConfig rewriting/comment preservation; on-demand parent transport; effect deltas; metadata restoration; fetch upload/next/cancellation; and their regressions. This is the entire current repair target, not just the latest fix.

Fresh reviewer checks from `example/web`:

```sh
vp exec go -C ../.. test . -run 'TestNativeBridge|TestNativeFetch|TestNativeDevelopment' -count=1 -v
vp exec go -C ../.. test -race . -run 'TestNativeBridge|TestNativeFetch|TestNativeDevelopment' -count=1 -v
```

Both passed: ordinary run `0.740s`, race run `1.843s`, with no failing/skipped selected test or race report. These exercise action/header effects with and without hooks, independent loads and demand-driven Parent, parent failure identity, concurrent load effects, large fetch payloads, URL/redirect/type metadata and cloning, owner retirement/unread cancellation, concurrent continuations, progressive body forwarding, configured Go handlers and ordinary filesystem composition. These are focused handler/transport fixtures, not full Kit-client qualification.

Inspected `/tmp/skgo-dev-repairs-session-final.log`: `TestRealSessionNativeRecoveryAndIsolation` passed `25.92s` (`26.243s` package), including configuration/base/appDir/trusted-origin and static dotenv refresh, subsequent compatible native edit, crash recovery and isolation assertions in the current test. This is a parent-run real-session result, not a second execution by this reviewer. The log contains transient expected startup/crash-recovery HTTP failures before success. The parent reports all eight owned Go/Vite PIDs gone and both private session directories removed; that cleanup receipt was not independently re-executed here.

Inspected `/tmp/skgo-dev-repairs-build.log`: current production frontend/goja build completed, including prerender shutdown and the final done receipt. Broad Go and complete development/production browser runs are held pending the newly identified explicit-close repair. They have not passed against this target, and the historical qualified runs in `DESIGN.md` are not relabeled as current proof. The accepted automatic direct SSR subfetch-cookie limitation and previously unexplained native lifetime assertion remain disclosed; neither is a new finding in this round.

No other material finding was established from the current full repair diff. Configuration/native-source-selection/composition repairs have concrete corresponding code and regression assertions; final runtime and release-completeness claims remain bounded by the available proof.
