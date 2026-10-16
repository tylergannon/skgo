# Independent assessment: native Kit SSR over private UDS

**Verdict: supported as a narrow transport/topology candidate; not qualified as the complete SKGO development bridge.** The executed fixture demonstrates that pinned Kit/Vite development SSR can run behind a public Go proxy while Vite and Go each use session-private Unix sockets, and that Kit's `event.fetch` can reach Go over UDS. It does not show that a fetch hook makes Kit's server loads, actions, endpoints, and remotes Go-owned. Those callbacks are direct Kit dispatch points and need a Go callback bridge that preserves Kit's wrappers and request semantics.

## What the run establishes

The adjacent [runtime POC report](../20261015-vite-uds-ssr/REPORT.md) and its raw files provide the execution record. The fixture used installed Kit 3.0.0, VitePlus 1.0.0/bundled Vite 8.3.1, the actual Kit middleware-mode server, and Node's supplied HTTP server listening on a per-instance Vite socket. The Go harness owns the public TCP listener and proxies the normal application path to Vite UDS and `/go/` to Go UDS. `node-tcp-listeners.txt` is empty; `node-unix-listeners.txt` records the two Vite socket owners. The socket parent directory is mode 0700.

Kit `+page.server.js` in the fixture calls `event.fetch('/go/value?...')`; an explicit `handleFetch` hook sends that selected request over the matching Go socket. The result is rendered into native Kit SSR HTML. Independent ALPHA/BETA literals, request markers, cookies, authorization values, and public `Host` appear in their respective output documents. A deliberately wrong Alpha socket target yields HTTP 200 with BETA's literal and fails the independent Alpha expectation; restoring the target returns ALPHA. Direct public Go proxy POST also preserves method, query, body, cookie, marker, public Host, and both Set-Cookie headers.

The owner recorded an actual public `vite-hmr` WebSocket 101 and Svelte JS-update frame, with the heading changing from `UDS_FRONT_ONE` to `UDS_FRONT_TWO` on that connection. The edited component's count reset from 1 to 0; the POC makes no arbitrary edited-component state-preservation claim. I independently opened a fresh Chrome tab after the Go replacement, saw `UDS_FRONT_TWO` with `ALPHA_7407`, clicked Count 0→1 to verify hydration, then visited the separate BETA origin and saw `UDS_FRONT_ONE`/`BETA_5202`.

Closing and reopening ALPHA's Go listener at the same `go.sock` changed fresh Kit SSR to `ALPHA_7407` without restarting Vite or retargeting the proxy; BETA remained on `BETA_5202`. The already-open Alpha browser stayed on `ALPHA_4101` until manual reload. Thus UDS gives a stable address for replacing the HTTP process, but automatic Go-change invalidation is absent from this fixture.

## Kit boundaries checked against the installed pin

Kit's `runtime/server/respond.js` creates `event.fetch` through `create_fetch` and dispatches page data, endpoints, and remote calls through their own branches. `runtime/server/fetch.js` invokes `hooks.handleFetch` first. The supplied fallback then applies Kit's same-origin credential/cookie handling, routes same-origin requests back through Kit's internal `respond`, and transfers internal Set-Cookie effects to the outer request. The fixture's hook returns a raw Node HTTP response early, so it bypasses that fallback.

This is visible in the artifacts: Go sets `go_one` and `go_two`; both appear on a direct public Go response, while neither appears in `ALPHA-ssr-headers.txt` for the SSR subfetch. The hook manually forwards the test's inbound Cookie and Authorization, but it does not establish Accept-Language, credential omission/override, pending-cookie propagation, response-cookie propagation, abort handling, or streaming behavior. These are specific gaps, not objections to using UDS.

More importantly, a Go `handleFetch` hook does not intercept the other Kit callback boundaries. In pinned source:

- `runtime/server/page/load_data.js` directly calls `node.server.load`.
- `runtime/server/page/actions.js` directly invokes the selected action callback.
- `runtime/server/respond.js` directly dispatches endpoint modules and `handle_remote_call`.
- `runtime/server/remote-functions.js` and `runtime/app/server/remote/shared.js` run remote callbacks inside Kit's wrappers/request store.

The fixture's JavaScript server load is only a test vehicle for the fetch transport; it is not proof of the repository's required all-Go server-load contract. The separate [bridge mapping](../20261015-vite-uds-bridge/BRIDGE.md) identifies the feasible next seam: replace generated callback bodies while keeping Kit's live route graph and wrappers. Existing `prerender-modules.js` is a source precedent but is build-only, covers limited remote kinds, and omits actions. The development transform/callback lifecycle has not run here.

## Design assessment and remaining limits

The selected topology is coherent enough to continue as a bounded implementation candidate: Kit/Vite own development module/route identity and browser HMR; Go owns application callback bodies and the public proxy; each private listener has a stable session socket. This avoids maintaining a second client node table and allows the Go HTTP process to be replaced at a stable private address. No Vite behavior modification or forced page reload is needed to prove this transport slice.

The design is not ready to hand off as “fetch delegates all SSR to Go.” A complete bridge must bind callbacks from the live Kit graph to Go, preserve Kit's load/action/endpoint/remote semantics, and carry request identity, context, cookies and response effects deliberately. In particular, the existing Go remote HTTP response is a complete protocol envelope; returning it as a Kit callback value would be wrong. The mapping records an initial browser-remote exception as a possible narrower boundary, while direct SSR remote calls still need Kit-wrapper-compatible Go callbacks.

The current request bridge also needs a trusted public client address: Kit's Vite dev `getClientAddress` reads `req.socket.remoteAddress` and throws if absent, but an incoming Unix socket does not supply the browser's TCP peer address. Public origin is already known before Vite initialization in the fixture; address propagation and spoof rejection were not tested. The same-socket replacement proves new requests can use the replacement, not that an in-flight request pins its Go process/native revision until completion. Existing prerender callback handles are not sufficient evidence for that pinning. Structural route/remote changes, error recovery, replacement during an SSR callback, streamed/deferred values, and the effect of node/remote identity changes on an open browser remain unqualified.

The answer to the architectural question is therefore **conditional**: this UDS + native Kit SSR composition is feasible and directly demonstrated for one selected `event.fetch` data route, public proxying, HMR, instance isolation, and fresh-request Go replacement. Fetch-only is insufficient for SKGO's required endpoint ownership. The material remaining decision is the callback transport contract for direct Kit loads/actions/endpoints/remotes (including request lifecycle and Kit remote mutation effects), not the listener topology or renderer choice. Production Go/goja remains unchanged by this dev-only candidate.
