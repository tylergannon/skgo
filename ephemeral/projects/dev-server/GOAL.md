# SKGO development experience: goal and responsibilities

Run `skgo dev`, open the application, edit Svelte or Go, and see the change quickly and automatically. Preserve healthy browser drafts and state where the change permits. Show useful errors and recover when fixed.

## User-selected development direction — 2026-10-15

Development and production may use different rendering arrangements; both must be tested. The user explicitly selected native Vite/SvelteKit SSR during development, with Go supplying application behavior. This supersedes the earlier requirement that the development Go process also run the renderer.

1. **Vite/Kit owns development rendering and frontend development.** It owns its live server/client modules, route/node identities, SSR, transforms, watching and normal HMR. Do not copy that graph into a separately built Go development renderer or alter Vite's browser lifecycle.
2. **Go owns application behavior.** Loads, remotes, actions, endpoint bodies, request hooks and application data access remain Go implementations. Generated development glue delegates to Go; it is not a second TypeScript application mode.
3. **The Go coordinator owns the public listener and supervision.** Vite and the Go application use Unix sockets with constant basenames inside an instance-private directory; the coordinator alone binds the network interface. Public origin remains distinct from private transport location.
4. **The two servers cooperate through a request-aware bridge.** SSR fetch can delegate to Go over UDS. Calls that Kit executes directly, including generated loads and remote callbacks, also require delegation. Preserve Kit's cookies, credentials, transport values and request behavior rather than assuming URL rewriting covers them.
5. **Go changes retain the native fast path.** Prepare and install compatible edits; rebuild/restart when needed. Arbitrary initialized Go state need not survive process replacement.

Production remains one cgo-free Go binary with pooled goja SSR, no Node at request time. The development exception permits Node SSR and transport glue; it does not move application logic or data access to JavaScript.

## Scope

Architecture validation is complete enough to begin bounded implementation. On 2026-10-15 the user authorized 3–5 work chunks implemented by Sol 6.1 subagents, with the parent monitoring scope and simplicity. Implement and qualify the selected architecture; do not reopen alternatives without a concrete incompatibility. Tactics from earlier experiments are not requirements. No forced waiting page, custom reload policy or deliberate browser state loss is authorized; preserve Kit's normal behavior and disclose any unavoidable tradeoff.

Use `/Users/tyler/src/skgo-token-cache/index/README.md` and the semantic index for prior work. Map the installed Kit pin before proposing mirrored behavior. Distinguish demonstrated runtime behavior, source-backed feasibility and remaining integration proof.

The source discussion is chat `01a141a2-423e-7360-9ab1-e17234bd620e`, especially the decisions to permit native development SSR, use per-instance UDS, and inspect `handleFetch`. The subsequent user request authorized bounded delegated implementation after assessment. [DESIGN.md](DESIGN.md) records the result and its limits.

## Accepted temporary limitation

The user accepts absent automatic response-cookie propagation from a direct SSR UDS fetch into Kit's pending cookies/outer page response. This is non-blocking for architecture work. Research a supported remedy in parallel; do not make it a new implementation gate. SSR header access itself is available. All other scope and behavior constraints remain.
