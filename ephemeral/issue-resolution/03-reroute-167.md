# #167 — Go reroute support

Complete downloaded issue body and comments: [03-reroute-167.json](03-reroute-167.json).

## Readiness and mission

Ready for implementation; no HITL product choice remains. An application can use Go rerouting to select the same effective route and parameters for a document, server-data request, endpoint, and Kit client navigation in development and production. Preserve the original URL identities that Kit preserves. The hook/bridge API and implementation are the builder's work, grounded in the installed Kit pin, not an invitation to invent a static route-alias subsystem.

## Read-only token-cache evidence

Retrieval followed `/Users/tyler/src/skgo-token-cache/index/README.md` → `index/upstream-docs/topics.md` / `index/upstream-docs/kit-advanced.md`, and `index/promise-audit.md` → `promise-audit-2026-10-17/facts/matrix/init-reroute.md`.

- `repos/dev-experience-20261015/frameworks/kit-3.0.0-upstream/documentation/docs/30-advanced/20-hooks.md:308-337`: universal `reroute` runs before `handle`; returned pathname selects the route and params, defaults to `url.pathname`, and does not change `event.url` or the browser address bar.
- The same source, `:339-359`: the hook may be asynchronous and use its supplied `fetch`; route id and params are unavailable to `handleFetch` before selection. The function must be pure/idempotent; Kit caches the browser result per unique URL. This rules out silently treating the hook as a side-effecting per-request middleware or a permanent static alias table.
- `promise-audit-2026-10-17/facts/matrix/init-reroute.md:5-9`: browser-only rerouting without a Go counterpart creates conflicting server/client route identities. The cached source account is historical, not a current runtime verdict.
- The same source, `:15-19`: the proposed proof already distinguishes document/data/endpoint selection, translated params, original URL in the Go load, and browser navigation to the same translated path. Its adjacent startup/init and deployed-site suggestions concern the larger audit cell and are **outside #167**; do not import them into this task.

These cached sources are context at Kit 3.0.0. Installed `example/web/node_modules/@sveltejs/kit/package.json` reports **3.1.0** and is authoritative. The builder must independently map its server and client reroute implementation and constraints.

## Current repository boundaries

`README.md` and `documentation/docs/skgo/40-advanced/20-hooks.md` still state reroute is unsupported; the differences page repeats that limitation. Their final wording must reflect only implemented, qualified behavior. Render the documentation according to its existing authoring contract if source docs change.

`internal/adapter/skgo-adapter/app-paths.js` already routes production renderer `$app/paths.match` through Go, but its comments explicitly assume no reroute and contain stale matcher claims. `env.js` supplies an empty generated `get_hooks`; these are existing seams to assess rather than instructions for a specific implementation. Native development uses Kit-owned rendering, while production uses Go/goja; both require proof. Do not execute Go application logic or I/O in JavaScript to make either path pass.

Installed Kit source has distinct reroute handling in `src/runtime/server/respond.js` and `src/runtime/client/client.js`, plus the `Reroute` public type in `src/exports/hooks/public.d.ts`. Server rerouting precedes matching and has route-resolution/prerender boundaries; client rerouting caches by URL and uses its effective URL for navigation selection. Map those exact branches, including what the hook may return and the supplied fetch behavior, before settling the Go surface.

## Winning and proportional proof

Use one representative real application where an alias selects a parameterized target. A literal receipt from the target Go load/endpoint must identify the selected route, translated parameter, and original request URL separately. That receipt must differ from a direct route/unmapped-path control so bypassing reroute fails visibly. Cover document, `__data.json`, and endpoint requests against the actual served handlers, preserving query/URL distinctions required by pinned Kit. Include a no-change result so ordinary routing is not swallowed.

Use ordinary focused Go tests for exact HTTP/identity behavior and hook-order/fetch/error constraints that pinned Kit requires. Use focused browser navigation to the alias to prove Kit's own client and Go agree, with the alias retained in the address bar and the positive target receipt visible. Run the representative application in native development and built production, reusing build/session output across assertions. Inspect the page normally; do not infer success from an emitted hook event or a 200 alone.

Opus validates goal coverage, verifies that removing or bypassing reroute would fail the proof, runs focused checks, and reports only acceptance failures or reproducible regressions. Repeat only checks affected by a fix; broader regression testing belongs at the integration boundary rather than every comment.

## Questions resolved and exclusions

- **Redirect or reroute?** Kit answers: effective route selection changes; the original URL/address does not become the destination.
- **Can browser/server use different matching?** No: retained issue acceptance explicitly requires their identities to agree.
- **Is static alias support enough?** No: it would not establish the universal hook contract Kit supplies. The builder maps and mirrors that contract.
- **Does this include server init, prerender entries, deployment, or new measurement machinery?** No. Those are other capabilities, not required to close #167.
