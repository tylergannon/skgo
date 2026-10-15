# Implement native Kit development SSR with Go over UDS

[GOAL](GOAL.md) and [DESIGN](DESIGN.md) control. On 2026-10-15 the user authorized implementation in 3–5 chunks using Sol 6.1 subagents, with the parent monitoring scope and simplicity. Earlier architecture-only instructions remain history, not a current prohibition.

## Assessment

The topology, native Kit SSR/HMR and isolated sockets have executed successfully. Existing callback source mappings identify concrete reuse boundaries. This is enough to start implementation, not evidence that complete application semantics already work. The first joined callback/lifetime implementation must expose any remaining architecture obstacle promptly; do not conceal one with a new framework or silently reduced behavior.

## Five chunks

1. **Public supervisor and sockets.** A developer runs ordinary `skgo dev` and gets native Kit SSR/HMR through one public address, with isolated private Go/Vite sockets. Supervisor worker owns command/process/transport integration. Proof includes real child readiness/replacement/cleanup, HTTP/upgrades and trusted public request context.
2. **Kit callbacks into Go.** Native Kit loads, actions, endpoints and direct SSR queries execute Go while Kit retains wrappers and its live graph. Callback worker owns adapter/generation integration. Proof includes independently supplied Go values in documents/navigation, query transport/cache identities and mutation/failure/redirect behavior. Browser remote requests retain the existing Go wire handler.
3. **Go request lifetime.** Bridge worker owns Go callback service and generated native host. Actual app hooks/locals/config survive callbacks; delayed output and cancellation behave correctly; replacing a process never silently replays old callbacks/mutations. No new whole-render native-code transaction requirement.
4. **Go edit integration.** After transport/bridge seams join, supervisor worker connects existing retained preparation/native installation and rebuild fallback. Go edits become visible only after successful installation; errors recover. No second frontend renderer graph, custom waiting page or forced navigation policy.
5. **Independent joined validation.** A fresh Sol 6.1 validator checks that assertions fail when claimed behavior breaks, runs real-handler and necessary Kit-client checks, and opens the actual application. Include SSR/hydration/navigation, remote mutation refresh/transport, Svelte HMR, Go edit/error repair, delayed value/cancel/replacement and production regression checks. Report unsupported claims honestly; fix substantive failures through their owners.

Chunks 1–3 run concurrently with explicit ownership and shared-interface coordination. Chunk 4 depends on their joined interfaces. Chunk 5 follows runnable integration. The parent owns architecture/plan/handoff consistency and scope decisions, not a competing implementation.

## Bounded endpoint

Finish when ordinary `skgo dev` uses this topology with Go-owned application behavior and the joined claims above are demonstrated, or identify a concrete architectural incompatibility requiring a user decision. No PR merge/release, upstream request, broad benchmark campaign, extra platform framework or token-cache rebuild is part of this task. The accepted SSR subfetch response-cookie limitation remains non-blocking.

## Prior evidence

The UDS POC, independent browser assessment, callback mapping and cookie source findings remain linked from DESIGN. Their old timings and narrowly bounded results must not be relabelled as proof of the completed new product. Use the token-cache semantic index to retrieve prior work, then verify against installed Kit and current code.
