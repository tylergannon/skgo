# Continue from the native Kit SSR / UDS architecture

Read [GOAL](GOAL.md), [DESIGN](DESIGN.md), then [PLAN](PLAN.md). The current task is architecture validation/documentation. Product implementation is not authorized.

The user explicitly permits development SSR in Vite/Kit and production SSR in Go/goja. Both child servers use constant socket filenames in a unique session directory; only the Go coordinator binds the network interface. Older no-Node-dev and separate-renderer instructions are superseded. Preserve Kit's normal HMR/browser behavior.

Current evidence lives under:

- `ephemeral/experiments/20261015-vite-uds-ssr/`: actual two-instance KitSSR/UDS/public-proxy/HMR and sequential replacement POC, wrong-target control.
- `ephemeral/experiments/20261015-vite-uds-bridge/BRIDGE.md`: pinned-source callback and lifecycle mapping; fetch-only is incomplete.
- `ephemeral/experiments/20261015-vite-uds-validation/`: independent verdict and limits.

Read agent status before treating any fixture as live; owners clean only their own processes. Do not cancel other agents. Do not restart old split-renderer or forced-navigation experiments.

The remaining proof is the general Go callback bridge, not UDS support or another graph-publication mechanism. Native Kit owns documents, __data and actions; generated callback bodies execute Go. Use the existing complete Go wire handler for browser remotes initially, subject to joined SSR/browser ID/transport/cache proof. Preserve one Go process/request context across a render or fail clearly on loss; do not invent native-revision transactionality.

The minimal handleFetch POC did not propagate Go response cookies to outer SSR. Existing prerender integration has useful lifecycle/codecs but buffers streams and is build-specific. Do not claim it already solves ordinary remote/action, streaming or cancellation behavior. DESIGN lists concrete handoff missions and accepted proof.

Token cache: `/Users/tyler/src/skgo-token-cache/index/README.md`. Prefer the native-Kit/UDS route once indexed. Earlier evidence and measurements are preserved in [DESIGN-go-ssr-historical.md](DESIGN-go-ssr-historical.md); they do not describe the selected architecture's measured performance.

Answer user questions literally before action. Do not interpret permission to choose or validate an architecture as permission to implement the product.
