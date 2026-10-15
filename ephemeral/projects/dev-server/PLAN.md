# Validate native Kit dev SSR and Go over UDS

[GOAL](GOAL.md) and [DESIGN](DESIGN.md) replace the old split-renderer publication plan. The user selected native Vite/Kit SSR for development, Go application behavior, two private Unix sockets and one public Go supervisor/proxy. Production remains Go/goja. This task is validation and documentation, not product implementation.

## Current state and next work

The bounded UDS runtime POC, callback source mapping and independent assessment are complete. Their reports and the current architecture are saved. The user subsequently accepted missing automatic SSR response-cookie propagation as a non-blocking limitation and requested parallel remedy research while architecture documentation continues.

Completed cookie subagent: `native_dev_host` (Sol6.1), owns `ephemeral/experiments/20261015-vite-uds-cookies/FINDINGS.md`. It found the supported Kit3 parse/set API (source-only; no new runtime test) and completed the upstream assessment: prefer a documentation clarification; existing public APIs suffice for a candidate remedy, while automatic adoption would have compatibility risks. No external posting is authorized. Parent owns architecture/handoff/continuation documents. Other prior UDS agents completed and cleaned their owned runs.

The final cookie assessment is folded into DESIGN without making it a prerequisite. Current documentation/handoff reflects the selected topology and accepted limitation. There is no pending agent result; any next work must remain within architecture scope unless separately authorized. Do not reopen split-renderer synchronization or start product implementation. Current user asked to persist all post-compaction context; CONTINUE is the resume entrypoint.

## Completed bounded validation

- Sol 6.1 `listener_ownership`: actual Kit SSR, Go fetch over UDS, two sessions, public HMR, wrong-target control and same-path Go replacement. Owns `ephemeral/experiments/20261015-vite-uds-ssr/`.
- Sol 6.1 `native_dev_host`: pinned Kit/existing prerender bridge mapping, direct-call versus fetch boundary, routing and concrete reuse gaps. Owns `ephemeral/experiments/20261015-vite-uds-bridge/`.
- Luna `retained_result_check`: independently inspect/run the narrow controls and record supported versus unproved claims. Owns `ephemeral/experiments/20261015-vite-uds-validation/`.
- Parent: read linked discussion, integrate findings, update goal/design/continuation and retrieval pointers. No product edits.

Stop this validation after the bounded POC, source mapping and independent appraisal are recorded in DESIGN. Do not turn a missing general callback bridge into authorization to build it. The design must clearly say topology/fetch feasibility is demonstrated while complete application semantics remain unqualified.

## Constraints

Use the token-cache semantic index and installed Kit pin. Vite owns normal SSR/HMR and its graph; do not modify its internals or browser lifecycle. No forced waiting page, custom full reload or deliberate browser state loss. Generated bridge code is transport for Go application behavior, not a TypeScript application mode. Use Go for owned harnesses and `vp` for frontend tooling. Keep all experiments isolated and preserve existing source/evidence.

## Next proof if continued

DESIGN records the exact bounded Go callback/lifecycle slice needed before claiming full native-dev integration: Go load and direct SSR query, browser remote protocol parity, action/endpoint behavior, hooks/context and cookie behavior outside the accepted SSR propagation limitation, delayed output/cancellation and replacement during one render. Earlier captured/native proofs remain reusable. No new listener comparison or split-renderer synchronization campaign is needed.
