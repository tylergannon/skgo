# Development-server repair: independent changed-code review, round 01

Outcome: **material findings remain**.

Reviewed the complete repair diff from `1d9e23e1760266cc83367c00999105e28db19944` through requested target `5ebe046`, its surrounding implementation and tests, the prior development-server review and reproductions, and the current GOAL/DESIGN/PLAN/CONTINUE. Read repository instructions, agent-protocol and adversarial-review; followed the token-cache entrypoint/current native-Kit route and verified installed Kit 3.0.0. Installed Kit's load scheduling and header-effect rules were inspected. No review-scope narrowing was supplied or applied.

This review covers configuration publication, application construction, captured compiler/workspace selection, type-aware constructor instrumentation, callback effects/parent scheduling, fetch transport, metadata and cleanup, plus production-facing changes. The explicit development constructor is a substantial improvement over hidden fs.FS identity. The new tests exercise literal receipts for the reported failure paths. The accepted direct-SSR-subfetch cookie limitation remains accepted.

## Issue: continuation replies outlive the HTTP request body that backs them

**Evidence:** `dev_fetch.go:158` gives the Go HandleFetch continuation the live `/fetch-response` request body. At `dev_fetch.go:179` that response escapes into the Go consumer. The handler can then select a different pending continuation at `dev_fetch.go:190-213`, write its next-request frame, and return while the previous consumer is still reading. net/http closes the old request body and may reuse/read its connection after the handler returns. Giving each continuation a separate reply channel fixes reply identity, but does not transfer ownership of that HTTP body.

**Impact:** A valid HandleFetch hook making concurrent `next` calls can fail its SSR request with `http: invalid Read on closed Body`. The observed failure also includes a recovered net/http panic, `invalid concurrent Body.Read call`, and Node `EPIPE`. This is a product failure through the actual bridge, not a hypothetical concurrency objection.

**Reproduction:** Use the added `TestNativeBridgeConcurrentFetchContinuationsKeepTheirReplies`, whose hook starts two continuations and consumes both response bodies. With the managed Node returned by `vp exec node -p 'process.execPath'` from `example/web`:

```sh
PATH=/Users/tyler/.local/share/vite-plus/js_runtime/node/24.22.0/bin:$PATH go test . -run '^TestNativeBridgeConcurrentFetchContinuationsKeepTheirReplies$' -count=5 -timeout=60s
```

All five executions failed during this review under Node v24.22.0 and Go 1.27.2. Typical error: `skgo: development fetch-response HTTP 500: http: invalid Read on closed Body`. The first iteration additionally logged the recovered concurrent-Body.Read panic and EPIPE. An earlier focused native bridge/fetch/development run also failed this case; a ten-run comparison under ambient Node v26.11.1 failed all ten. This establishes that the failure is not confined to the ambient Node version.

The bridge must preserve the lifetime of each continuation's response body through consumption or cancellation, independently of when a later continuation is dispatched. Keep the concurrent-continuation regression and verify the repaired ownership path; do not serialize the application hook as a substitute for preserving its valid concurrency.

## Verification boundaries

The focused root tests create private UDS fixtures and read installed dependencies. I did not start shared development sessions, alter generated example artifacts, run broad browser/Go suites, or modify implementation files. Parent-owned broad checks and real-session qualification remain separate evidence. During review HEAD advanced to `421917dd7ddcbef08d9a959dfdcdc2575ac595f1` for an independently reported supervisor cleanup repair; the fetch source cited above remained present and the managed-Node reproduction failed at that checkout. This artifact assesses the requested repair round and the reproduced remaining defect; it does not pre-judge subsequent repair work.

No additional material findings were established in the inspected repair diff. Earlier source-backed concerns are not repeated merely because they appeared in the prior report.
