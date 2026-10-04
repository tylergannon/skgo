# Declared prerender Inputs adapter review — round 05

Outcome: **material findings remain**.

Review target: immutable `a5bec993d903bc757526042770df638cdb349e6c`, compared with round 04 and the whole declared-Inputs contract in `ephemeral/worklog/20261004-declared-prerender-inputs.md`. References below identify that commit, not later working-tree edits. This is not a completed-feature or merge verdict.

## Evidence and limits

Applied the repository agent-protocol, adversarial-review and pinned SvelteKit instructions. Re-read the complete process owner/helper and adapter lifecycle wiring, the Go dispatcher/diagnostic changes, the maintained fixture changes and surrounding error types. Revisited the installed Kit 3.0.0 enqueue, error/control-class and type-declaration sources, plus the earlier generator, native transport and engine-graph review evidence. No requested focus was treated as restricting defects outside that focus.

Read the independent Go proof source and actual `../prerender-inputs-proof/product-first-80e05ea.log` and `product-originals-a5bec99.log`, including the completed a5 run. At 80e, six malformed/infrastructure cases actually succeeded under ignore-500 policy. At a5, the unchanged bad-JSON, empty-envelope, bad-devalue, missing-result-member, null-Inputs, valid-JSON/nonzero-exit and early-stdin cases all reject. The a5 combined declared-only/dedup/empty-noarg/Money/counts/compiled-HTTP/SSR proof passes. Its programmatic native producer-failure proof also passes, recording zero live owned group, absent private directory and zero late I/O immediately after the outer rejection. These are stronger receipts than the original smoke.

This reviewer ran no additional build/test and made no product, test, dependency or worklog edit. The causal lifecycle cases below are source findings, not claims of newly executed OS fault injection. The uncommitted maintained-fixture and lifecycle-test edits were excluded from the immutable verdict. Only this review artifact was written.

## 1. Issue: cleanup discards failed tree drains and releases ownership anyway

**Requirement:** failure/signal cleanup must join owned process trees before reporting completion; inability to drain must remain observable rather than being treated as successful cleanup (acceptance group 6).

`internal/adapter/skgo-adapter/prerender.js:201–206` catches a failed `stopGroup`, removes the job and resolves its close promise before rejecting its result with the tree error. A concurrently running session cleanup waits only the close promise, not that result. It then retries all groups at line 319 with `catch(() => {})`, discarding the retry failure. Lines 337–342 remove the exit/signal handlers and clear the remaining ownership records regardless of whether a group was actually drained.

**Causal case:** cancellation begins while an owned group cannot be drained, for example a signaling error or the explicit five-second drain timeout at line 113. Child close records the tree error, but the cancellation barrier receives a fulfilled close promise. Its final retry also fails and is swallowed. Cleanup then resolves, pending requests receive failure responses and the original build rejection can propagate, while the undrained group has been forgotten. A nonzero primary build result does not establish the promised cleanup barrier; it also hides the material secondary cleanup failure from the caller.

Keep drain success/failure separate from stream closure. Collect and expose cleanup failures while preserving the original application failure, and do not mark failed drains as successfully released ownership. The primary failure must remain the primary failure; it must not erase the cleanup outcome. Exercise a controlled drain failure as well as the already-passing successful cancellation case.

## 2. Issue: completed jobs leave historical process-group IDs eligible for later kills

**Requirement:** the owner cancels its own live work and releases process resources after verified draining.

Every job enters `session.groups` at `prerender.js:158`. After successful `startDrain`/`stopGroup`, line 203 removes it only from `session.jobs`; its group record remains until final session cleanup. The exit handler at line 260 signals every historical record, and ordinary cleanup at line 319 calls `stopGroup` for every historical record. `groupAlive` checks only whether that numeric PGID currently exists.

**Causal case:** a compiler or early producer group fully exits and is reaped while later prerender work continues. The OS subsequently reuses that process-group ID for an unrelated process. Final cleanup sees the reused ID as alive and sends TERM/KILL to it; the abrupt-exit handler can send KILL directly. No surviving ownership relationship connects that later process to this build. Long-running builds on busy hosts make retaining all historical IDs materially different from retaining only currently owned groups.

Retire a group's signaling eligibility immediately after confirmed draining, while retaining records for groups whose drain failed. Completion bookkeeping may retain diagnostics, but must not keep an already-released numeric PGID as an active kill target. This complements finding 1: successful drains release ownership; unsuccessful drains retain an observable failure.

## 3. Issue: the new maintained fixture imports a removed Kit type and cannot pass its typecheck

`internal/adapter/prerender_inputs_build_test.go:99–102` writes `import type { Handle } from "@sveltejs/kit"`. The same test then runs `svelte-check` at lines 112–120. At the independently installed Kit 3.0.0 pin, `types/index.d.ts:1236–1248` declares `Handle` inside module `@sveltejs/kit/hooks`; it is not exported from the root module.

The checkpoint fixture therefore fails before its new native-artifact assertions can qualify the feature. The working tree already changes this import to `@sveltejs/kit/hooks`, which is the correct source-backed correction, but that uncommitted change is not part of a5. Include and execute the corrected maintained fixture before claiming its assertions passed.

## Round-04 corrections and qualification state

All four round-04 defects have appropriate source corrections. The successful buildApp path now checks the latched owner failure; independent permissive-policy cases confirm that this is load-bearing. Leader exit starts draining independently of stream close. Decoded Inputs must be arrays, and result values require an own `_` member; the unchanged null/missing-member cases now reject. Unknown-only diagnostic pointers preserve empty text and omit the field for authored HTTP errors; ordinary errors capture `err.Error()`, panics capture their private text before sanitization, and the worker reconstructs a generic Error from that diagnostic. Public fallback remains separately opaque. Native-hook/default-fallback bridge proof remains required in addition to Go envelope assertions.

The same-worker transport bootstrap and physical native control classes retain the source-backed conclusions of round 04. Main carries serialized protocol strings; worker hooks revive/encode values; native canonicalization still owns keys, distinct from body IPC. The retained stock-negative/initialized-native-positive/bootstrap-removal controls remain important because the initialization bridges an actual stock Kit singleton defect.

The a5 maintained fixture's producer receipts overwrite `called`, so they cannot establish exactly-once invocation; it also supplies no producer receipt environment during generation. These are evidence limitations, not additional product findings: the independent combined proof does use append counts and receipt environments for generation/build/runtime, and the pending maintained edit strengthens those checks. Its spawned shell does not inherit Go stdout/stderr, so absence of its late write does not exercise the formerly hanging inherited-pipe case. That case still needs its distinct real lifecycle proof.

The authoritative seven groups remain the completion standard. Existing a5 receipts establish substantial declared-input behavior and one awaited native failure barrier. They do not by themselves establish all remaining native crawler/SIGTERM/worker-death/compiler/inherited-pipe cleanup cases, native body-error hook and redirect policy, generated JavaScript and TypeScript qualification, removal controls at the final implementation, or exact-head regression/CI completion. Neither this partial evidence nor the source corrections approve merge or release.
