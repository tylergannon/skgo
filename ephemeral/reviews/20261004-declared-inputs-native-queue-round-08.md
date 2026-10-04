# Declared Inputs native queue review — round 08

Outcome: **material findings remain**.

Target: immutable implementation `39d7218`, the authoritative whole Inputs scope and seven acceptance groups, and the newly exposed minimal-build failure. This review does not approve merge or release. The reviewer remained product/test/dependency read-only and wrote only this artifact.

## Evidence inspected

Re-read installed Kit 3.0.0 `core/postbuild/queue.js`, prerender setup/enqueue/seeding/completion, native remote declarations, the adapter/emulator public types and native server emulator invocation sites. Read the full a5-to-39 helper changes, current adapter integration and maintained fixture changes, plus the earlier generator/transport/error-origin findings and their corrections. No requested focus was treated as excluding defects elsewhere in the declared-Inputs goal.

Read the independent actual `../prerender-inputs-proof/product-minimal-clean-a5bec99.log`, its minimal fixture source in `product/product_test.go`, and both fixture build logs. Go generation succeeds, but the noarg-only and two-package/same-export real builds fail at native enqueue. These fixtures contain a simple prerendered root and their declared remotes; they do not use the full example's unrelated Go loads to keep work running. The relevant producer/crawler integration is unchanged at 39.

Read `../prerender-inputs-proof/native/native_test.go:394–413`, `native/native-async-queue.log`, and the actual `native-fixtures/async-queue-closed` source/build log. This stock-native reference uses an unmodified Kit install, native TS remote and noop native adapter. The async producer waits 500 milliseconds, then returns atlas/beacon. Its root HTML positively finishes, its trace is exactly `inputs:start` then `inputs:end`, no body runs, and the build fails with the same closed-queue error. The native test's PASS denotes successful reproduction of the negative; it is not a successful native build. No new build or test was run by this reviewer.

## 1. Issue: legal async Inputs can outlive Kit's queue and make minimal builds fail

**Requirement:** noarg-only and same-export/package declarations must work through real native builds, and asynchronous Go Inputs must preserve native crawling/body overlap. Groups 1–3 and 6 cannot be satisfied by adding unrelated slow work to the fixture.

Native `src/types/internal.d.ts:561` explicitly defines the producer as `() => MaybePromise<Input[]>`; `runtime/app/server/remote/prerender.js:80` preserves the authored callback as `__.inputs`. Async producers are supported API, not invalid declarations.

`core/postbuild/prerender.js:694–704` starts route work, then lines 708–724 await each Inputs producer and enqueue its values (or the noarg remote). Only after that seeding loop does line 728 await `q.done()`. However, `queue.js:40–44` permanently closes and resolves as soon as an existing task completes with no remaining active/queued tasks. It does not wait for the explicit seeding-completion call. `queue.js:52` then rejects the later remote enqueue. A cold Go compilation makes this easy to reproduce; warming compilation does not eliminate arbitrary producer latency or gaps between multiple producers.

This is an independently reproduced stock Kit bug inherited by the integration. It remains a feature blocker: the full-app positive had unrelated in-flight work masking it, while required minimal apps fail. The correct source-level lifecycle is to keep accepting initial additions until seeding is explicitly complete, then resolve once idle; task rejection must retain its existing early failure behavior.

## Bridge-only alternatives do not preserve the locked semantics

The native prerender worker awaits `adapter.emulate()` at line 176, before queue construction at 254. That is a real early integration hook, but pre-resolving every producer there reorders producers before any route or remote body can run. It deadlocks the required valid overlap control in which a producer waits for an already-running blocked Go body's readiness receipt. It is therefore not an acceptable correction to the whole assignment.

The public `Emulator` interface exposes only an asynchronous `platform` factory (`types/index.d.ts:408–414`). Native `runtime/server/respond.js:204–208,442` awaits it before request execution. Holding requests there until producers finish likewise prevents the bodies those producers can depend on from starting, especially at native concurrency 1. There is no public post-response or queue hold/reopen hook. The prerender queue is a private lexical value, not an adapter-owned object.

Synchronous worker IPC/blocking would stop the native event-loop work needed to begin queued bodies. Synthetic tasks/loads, added routes, changed concurrency, preexecuted bodies, rewritten native keys/fields, or test-only delays would change behavior or hide the counterexample. Returning cached producers after an eager preload does not restore the required original overlap. Merely compiling early still leaves async producer latency.

No correct bridge-only repair has been established under the current no-native-patch constraint. The bounded actual repair is in native queue seeding/closure, delivered as an upstream fix or an explicitly authorized native/dependency correction. This conflicts with the existing constraint and requires an owner/user decision; this reviewer does not authorize or implement that exception. Keep the minimal/noarg/same-export assertions and overlap proof unchanged, and retain the stock negative. Do not claim completion by excluding minimal apps or serializing producers ahead of bodies.

## Current checkpoint observations

39 contains the reviewed parent-loss predicate, using VP-local Vite resolution, a main-session observer, awaited cleanup and nonzero self-termination. It removes successfully drained group records and collects drain failures before releasing remaining ownership; those source changes address the round-05 lifecycle findings. The corrected Kit hook type import and expanded JS/TS fixture are present. These changes do not affect the queue defect above and are not a substitute for final exact-head qualification.

The existing main-owner design remains the right place for subprocess ownership. The new blocker does not justify worker-local spawning, a runtime Node dependency, or a new supervisor. Continue to use all seven acceptance groups as the completion standard while the native queue incompatibility is adjudicated.
