# Development-server repairs: independent full review, round 02

Outcome: **material findings remain**.

Target: branch `codex/dev-server-review-fixes`, commit `814801d09c7dc98aeec522484f0f89c3bf2bb449`, complete diff from `1d9e23e1760266cc83367c00999105e28db19944`. Authorities: the user request to implement the prior review findings, repository instructions, agent-protocol, current `ephemeral/projects/dev-server/{GOAL,DESIGN,PLAN,CONTINUE}.md`, and the original review in skgo-project. Reused the installed-Kit 3.0.0 mapping and token-cache current architecture route inspected in round 01. Reviewed the full repair set and surrounding code, including configuration publication, source selection/workspaces, explicit application construction, callback scheduling/effects and fetch ownership. No scope narrowing was supplied or applied.

The original concurrent-response-body finding is repaired: uploads and next-step negotiation now have separate HTTP lifetimes, and the deterministic test delays the first body read until the second continuation returns. Existing tests for that case, progressive streaming, body payload size, URL/redirect metadata, unread-owner cleanup, parent demand, header effects and explicit fs.FS composition passed in ten focused repetitions. The supervisor cancellation fix also preserves a valid frontend pointer during shutdown. The real-session integration log ends with a pass at this commit, covering configuration/environment publication and the retained native path. These repairs are substantive; they do not eliminate the distinct close/cancellation defect below.

## Issue: closing an unfinished continuation body waits for its producer

**Location:** `dev_fetch.go:329-332`, with the upload handler waiting at `dev_fetch.go:207-217`.

A valid Go HandleFetch hook can inspect the continuation's status/headers, close its body, and return a replacement response. `devFetchBody.Close` currently calls the underlying HTTP **request** body's Close before signaling `done`. Go's server request-body Close drains up to 256 KiB to preserve its connection. For a chunked continuation stream that remains open, it waits for further bytes/EOF. The hook therefore cannot return its replacement, the upload handler cannot acknowledge release, and JS cannot cancel the unfinished producer through the normal fetch completion path. The logical owner cannot finish either because it is still awaiting that fetch. This is distinct from the repaired early-handler-return race.

**Reproduction:** A private Go overlay used the existing real Go/JS bridge fixture. The hook calls `next(request)`, calls `response.Body.Close()` without reading, and returns literal HTTP 202/body `literal-substitute-472`. The JS continuation yields `literal-open-stream-883` and remains open. The expected replacement never arrives. A 700 ms failure timer ends the fixture, after which request-owner cancellation cleans it up. One run and then three repeated runs all failed with `continuation Body.Close blocked on unfinished producer`, under managed Node v24.22.0 and Go 1.27.2. The implementation cause is confirmed in Go's `net/http/transfer.go` body.Close (`doEarlyClose` draining branch) and `net/http/server.go` assignment of that request-body mode.

**Impact:** Hooks that discard a streaming response to return a cached/fallback response hang until the producer finishes, even though the Go consumer explicitly closed it. A never-ending producer prevents the SSR request from completing. Closing must release/cancel the individual continuation body without waiting for all producer bytes, while retaining the repaired independent upload lifetimes and ordinary progressive delivery.

The exact overlay test is retained below. It was appended to the existing `dev_bridge_regression_test.go` contents through `go test -overlay`; implementation/tests on disk were not changed. The only writable file was this review artifact, temporarily holding the overlay source before being replaced by this complete report.

```go
func TestReviewNativeContinuationCloseDoesNotAwaitProducer(t *testing.T) {
 handler:=nativeBridgeRegressionFixture(t,nil,nil,SSROptions{HandleFetch:func(ctx context.Context, request *http.Request,next Fetch)(*http.Response,error){
  response,err:=next(request);if err!=nil{return nil,err}
  if err=response.Body.Close();err!=nil{return nil,err}
  return &http.Response{StatusCode:202,Header:http.Header{},Body:io.NopCloser(strings.NewReader("literal-substitute-472"))},nil
 }},false)
 runNativeBridgeRegression(t,handler,`
const e=event();let timer;
try {
 const page=await requestHandle(e,async()=>{
  const result=await Promise.race([
   requestFetch({event:e,request:new Request('http://app.test/stream'),fetch:async()=>new Response(new ReadableStream({start(controller){controller.enqueue(new TextEncoder().encode('literal-open-stream-883'));}}))}),
   new Promise((_,reject)=>{timer=setTimeout(()=>reject(new Error('continuation Body.Close blocked on unfinished producer')),700);})
  ]);
  assert.equal(result.status,202);assert.equal(await result.text(),'literal-substitute-472');
  return new Response('literal-close-final-913');
 });
 assert.equal(await page.text(),'literal-close-final-913');
}finally{clearTimeout(timer);}
`)
}
```

## Evidence and limits

- Fresh: `go test . -run 'TestNative(Bridge|Fetch|Development)' -count=10 -timeout=90s`, with PATH using the Node returned by `vp exec node -p 'process.execPath'` in `example/web`: passed in 5.004 s.
- Fresh: the close-without-consumption overlay above failed once, then 3/3 repeated executions. It uses the actual JS requestFetch/requestHandle and actual Go NativeDevelopment service over private UDS.
- Inspected parent-run `/tmp/skgo-dev-repairs-session-final.log`: `TestRealSessionNativeRecoveryAndIsolation` passed in 25.92 s, package 26.243 s. This is retained independent execution evidence, not my own second session run.
- The parent reports that the production build passed at this commit; broad Go and development/production browser qualification are held pending this repair. No release, merge, deployment, full-suite completion, new latency result, or Linux qualification is inferred.

I did not change source, generated example files or shared applications. The accepted direct-SSR-subfetch response-cookie limitation remains accepted. No other material findings were established in the complete current repair diff; resolved earlier findings are not retained as current defects.
