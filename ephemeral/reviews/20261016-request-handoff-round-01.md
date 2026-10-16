# Request handoff adversarial review, round 01

Outcome: **material findings remain**.

Target: the complete working-tree implementation over `9ebd8ca` in `/Users/tyler/.codex/worktrees/skgo-request-handoff/skgo`, reviewed against `/tmp/skgo347-assignment.md` and repository `AGENTS.md`. Production diffs were reread after the base-prefixed HMR correction appeared. The response-lifetime finding below remained present in that final reread.

## Evidence inspected

- The full diff, including untracked admission implementation and tests, router handoff tests, real-session fixture/helper, adapter owner tests, and worklog.
- Surrounding coordinator startup, recovery, replacement and publication paths; host startup and termination; native Kit transport; native bridge response and logical request ownership; downstream lifetime handling.
- Installed SvelteKit `3.0.0`, including its actual Node `setResponse` implementation, and the development adapter HMR configuration.
- Independently executed focused admission/router/adapter contracts: passed.
- Independently executed `go test -race ./internal/devserver -count=1`: passed.
- Independently executed from `example/web`: `vp exec go -C ../.. test -tags skgo_dev_integration ./internal/devserver -run '^TestRealSessionRequestHandoff$' -count=1 -v`: passed in 22.62 seconds. This exercises a held constructor, pending document/data/action requests, canceled queued action, one literal durable action receipt, exclusive application ownership and startup-rejection recovery.
- `git diff --check`: passed.
- An additional read-only executable probe using the current bridge and installed Kit response writer, detailed below: reproduced premature public response completion.

The real Gimbal consumer proof is being performed separately by the parent agent; it is not claimed as independently inspected evidence in this review.

## Finding 1 — issue: explicit Content-Length releases admission before logical request cleanup

Evidence: `internal/adapter/skgo-adapter/dev-bridge.js:477-506`, especially preserving all incoming end-to-end headers at line 478 while deferring `finish()` until the stream's final pull at lines 489-492; `internal/devserver/router.go:80` releases admission when `ReverseProxy.ServeHTTP` returns; `internal/devserver/admission.go:93-97` immediately considers a zero public-request count drained; `internal/devserver/session.go:508-512` then kills the previous application.

The assignment requires preserving request/response lifetime and giving admitted finite requests a bounded chance to finish. The bodyless-response fix now awaits logical cleanup, but a regular response carrying an explicit `Content-Length` still has the same lifetime problem. Kit copies the header to its public response. Once that many bytes arrive, Go's HTTP transport regards the response body as complete without waiting for Kit's stream to close. The public request can leave admission while the bridge is still awaiting `/_skgo/dev/end`. A replacement arriving at that point sees no active request and kills the old application immediately, interrupting finite Go middleware cleanup instead of allowing the configured one-second drain. This can interrupt application work performed after the wrapped handler returns, despite the response having arrived successfully.

Reproduction performed without editing implementation or tests:

1. Load the current `dev-bridge.js` as a data-URL module, replacing only the Kit exception imports as the existing adapter owner tests do.
2. Start a private mock Go server. `/begin` returns a literal request handle with `resolve:true`. `/response` returns HTTP 200 with `Content-Length: 1` and body `X`. `/end` records entry and deliberately holds its response behind a promise.
3. Start an HTTP frontend whose handler calls current `requestHandle`, with its resolve callback returning `new Response('X', {headers:{'content-length':'1'}})`, then passes the result to `setResponse` imported directly from the installed Kit `src/exports/node/index.js`.
4. Perform an ordinary Node HTTP GET and await its `end` event before releasing `/end`.

Observed output:

```json
{"body":"X","endStarted":true,"endFinished":false,"clientFinishedBeforeLogicalEnd":true}
```

The held `/end` was released after this observation and both probe servers were closed. The public client received a complete finite response while the logical owner was still alive. The same declared-length EOF rule drives the coordinator's reverse proxy, so its admission bookkeeping cannot provide the claimed cleanup boundary for this case.

Repair should ensure admission does not end merely because declared response bytes have arrived while logical cleanup remains pending. Add a regression that holds actual logical cleanup on a normal explicit-length response and verifies the old application retains its bounded opportunity to finish. Merely checking the eventual healthy response, or the new HEAD/204 contract, does not catch this case.

No other material finding was established in the reviewed snapshot.
