# Independent browser observations

Two actual browser tabs used Vite public origins `http://127.0.0.1:56411/orders` (ALPHA) and `http://127.0.0.1:56421/orders` (BETA). The parent operated them through the supported browser tool. The validator changed actual Go source files; the fixture watched, compiled and installed native code, then sent Vite events. There were no Go-value control endpoints in this sequence. Times in the native log cover preparation/installation, not independently measured save-to-visible latency.

Both corrected pages hydrated with remote and server-load values 3000. ALPHA was seeded with draft `ALPHA_DRAFT_73`, count2 and document ID `26771b22-a074-4a49-a2dd-fb49ac68d1a5`; BETA with `BETA_DRAFT_91`, count1 and document ID `4619343f-88ef-4509-9eb7-8742c81c30fd`.

| Action | ALPHA browser | BETA browser |
|---|---|---|
| Save ALPHA Go body returning4101 | Remote and load4101, notice1 | Both3000, notice0 |
| Save BETA Go body returning5202 | Both4101, notice1 | Both5202, notice1 |
| Malformed ALPHA Go source | Both4101, notice2, visible `GO_COMPILE_REJECTED` | Both5202, notice1 |
| Repair ALPHA Go body returning6303 | Both6303, notice3, Published ALPHA | Both5202, notice1 |
| Restart ALPHA initial host binary on a new private port | Both3000, notice4 | Both5202, notice1 |

Every row above retained the original respective draft, counter and document ID, with hydration true and no reported browser warnings/errors. The compile-error label is visible rejection, not a full useful compiler diagnostic. Restart here reused the initial host binary and reset native code to3000; it is not structural rebuild or crash-recovery proof. The separately executed recovery followup, if present, has its own observations.

An actual ALPHA Svelte source edit changed FRONTEND_ONE to FRONTEND_TWO through ordinary Vite HMR and independently rebuilt Go SSR. Its draft/count/notices reset, while document ID remained unchanged. BETA retained FRONTEND_ONE,5202 and all original state. No browser errors were recorded at that step.

Adding/removing a route exposed a separate failure: Kit changed generated node membership while Go's fixture retained fixed metadata. Browser diagnostics recorded a failed generated `nodes/2.js` import and `hmr_reload_after_error`; ALPHA's document ID changed to `c3e81d01-1dc1-4757-a62a-3a3127585a99`. Its page recovered after removal, but this is not successful structural-change support. BETA was unaffected.

Initial hydration failure is retained in `initial-hydration-failure.json` and `initial-network.json`: requests for `/.vite-listener/deps/*.js` were incorrectly forwarded to Go and returned404 with the application-origin header. Correcting the tooling exclusion produced the healthy baseline. A whole-fixture restart left earlier tabs on a connection-refused error page; fresh tabs reached the corrected application. No browser safety setting was changed.

Each named JSON file contains actual DOM snapshots and, where relevant, captured console/network observations. `alpha-native-repaired.jpg` was inspected and shows the native value6303 with the original browser state. The document marker is fixture instrumentation retained on `window`; no general component identity or universal browser-state guarantee follows.

`browser-tool-records.jsonl` preserves the actual browser calls and paired outputs from initial listener-tab creation through final recovery inspection. It is a structured JSON reserialization selected by call IDs from parent rollout `/Users/tyler/.codex/sessions/2026/10/15/rollout-2026-10-15T12-52-44-01a140e8-d934-7be3-9b32-c790cd31675d.jsonl`, not reconstructed prose or a byte-identical excerpt.

## Automatic child-exit recovery followup

A fresh fixture corrected readiness to require its own instance/root identity. New browser documents hydrated at3000; parent seeded ALPHA `RECOVERY_ALPHA_74`/count2 and BETA `RECOVERY_BETA_85`/count1. Their document markers were respectively `7c1264bd-04d7-4df7-affb-c7108d26a6c5` and `698986f6-e7b0-408c-add1-970794620a9d`.

Actual source edits installed7407 in ALPHA and8508 in BETA. Both browser remote and load values changed to their respective literals, notice1 each, preserving all seeded state. The author then killed exact ALPHA native PID22532. Its child-exit supervisor started PID23114 at a new private port63147 (previous63032), reinstalled the last successful native candidate before publishing the new address, and sent the Vite event. BETA PID22552/port63046 was unchanged.

After recovery ALPHA displayed7407 in both remote and load, notice2, with the original draft/count/document. BETA remained8508/notice1 with its original state. No browser errors were captured. CDP observation began before the source edits: both ALPHA ready messages arrived on the same WebSocket request ID `22545.7`, with no observed close/create/error event and a nontruncated event result. This is direct connection-continuity evidence across this one Go child crash; it is not Vite restart/reconnection proof. `recovery-websocket-events.json` retains those frames. `automatic-recovery.jpg` was inspected.

This recovery reused the same initial binary plus the last successful body-edit candidate. It proves automatic process replacement and candidate reinstallation, not structural-change recompilation, arbitrary Go state continuity, complete application admission, or a finished `skgo dev` implementation.
