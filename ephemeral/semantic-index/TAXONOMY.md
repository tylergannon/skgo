# TAXONOMY — task-first routes

Each route lists leaves in reading order. Open the first one; it cites the upstream lines.

## A. Stand up the bare Go server (Phase A)
- Serve the built client + fallback like adapter-node: `sources/kit/build-adapt/static-serving.md`, `sources/kit/build-adapt/build-output.md`, `sources/libs/sirv.md`, `sources/libs/mrmime.md`.
- CSR boot: what the shell must contain and what the client fetches first: `sources/kit/server-runtime/csr-shell.md`, `sources/kit/client-router/csr-boot.md`, `sources/kit/build-adapt/spa-and-prerender.md`.
- Dev proxy to `vp dev` (what to intercept vs forward, HMR): `sources/kit/build-adapt/dev-server.md`, `sources/junkyard/app/toolchain.md`.
- Request pipeline parity (CSRF, cookies, headers, 413, `x-sveltekit-*`): `sources/kit/server-runtime/request-pipeline.md`.
- Kit-3 config the example app needs (`paths.origin`, flags): `sources/kit/build-adapt/config-kit3.md`, `sources/kit/docs/kit3-facts.md`.
- Build hook (thin adapter vs adapter-static): `sources/kit/build-adapt/adapter-api.md`.

## B. Implement the remote-function protocol in Go
- Compute ids without Node: `sources/kit/remote-server/ids-and-build.md` (hash + golden values), then `sources/kit/remote-client/client-transform.md`.
- Request → response for query/command: `sources/kit/remote-server/request-handling.md`, `sources/kit/remote-client/client-requests.md`, `sources/kit/remote-client/client-expectations.md`.
- Payload/response serialization (devalue, base64url, `__skra*`, transport): `sources/kit/remote-server/serialization.md`, `sources/libs/devalue.md`, `sources/kit/portable-tests/devalue-port.md`.
- `query.live` SSE and `query.batch`: `sources/kit/remote-server/live-and-stream.md`, `sources/kit/remote-client/live-and-batch.md`.
- `form` (binary formdata) and `prerender`: `sources/kit/remote-server/form-and-prerender.md`.
- Port the upstream unit tests: `sources/kit/portable-tests/remote-functions-spec.md`, `sources/kit/portable-tests/port-map.md`.
- What the junkyard already got right/wrong: `sources/junkyard/go/remote-runtime.md`, `sources/junkyard/oracle/wire-protocol-facts.md`, `sources/junkyard/remote-codegen/runtime-interface.md`.

## C. Implement server loads (`__data.json`) in Go
- Contract end to end: `sources/kit/server-runtime/data-requests.md`, `sources/kit/client-router/data-fetching.md`.
- Route tree, ids, params, sorting: `sources/kit/server-runtime/routing.md`, `sources/kit/portable-tests/routing-and-manifest-spec.md`, `sources/kit/client-router/navigation-and-manifest.md`.
- Streaming/deferred promises, error nodes: `sources/kit/portable-tests/serialization-and-streams-spec.md`.
- Authored surface to mirror (`depends`, `parent`, cookies, `error()`, `redirect()`): `sources/kit/docs/loads-and-routing-surface.md`.
- Transport hook on both sides: `sources/kit/client-router/transport-hook.md`.

## D. Code generation (`skgo gen-bindings`)
- Marker scanning by type identity: `sources/junkyard/go/remotegen.md`.
- Link tree + `src/routes/go.mod`: `sources/junkyard/colocation/link-tree.md`, `sources/junkyard/colocation/authored-example.md`.
- Stub shape kit's transform accepts: `sources/kit/remote-client/stub-shape.md`, `sources/kit/remote-client/client-transform.md`.
- Types: Go → JSON Schema → TS: `sources/libs/polytype.md`, `sources/junkyard/remote-codegen/typeglue.md`.
- Generated-file ownership/inventory: `sources/junkyard/go/remotegen.md` (generate.go), `sources/junkyard/go/testdata-fixture.md`.
- Old design and why parts are dropped: `sources/junkyard/remote-codegen/design-plan.md`, `sources/junkyard/remote-codegen/lessons-and-traps.md`, `sources/junkyard/colocation/lessons-and-traps.md`.

## E. Project generator (`skgo new`) and toolchain
- Exact pins and scripts that work today: `sources/junkyard/app/toolchain.md`.
- Kit-3 breaking facts every generated file must respect: `sources/kit/docs/kit3-facts.md`, `sources/kit/build-adapt/config-kit3.md`.
- Example app content to generate: `sources/junkyard/app/guestbook-app.md`, `sources/junkyard/colocation/authored-example.md`.

## F. Acceptance suite (Playwright against the Go binary)
- Reusable tests and their CSR verdicts: `sources/junkyard/app/e2e-suite.md`, `sources/junkyard/app/csr-portability.md`.
- Kit's own scenarios to borrow: `sources/kit/test-apps/remote-scenarios.md`, `sources/kit/test-apps/load-and-action-scenarios.md`, `sources/kit/test-apps/no-ssr-guarantees.md`, `sources/kit/test-apps/harness-helpers.md`.
- Classic form actions under CSR: `sources/kit/server-runtime/actions.md`.

## G. Writing Svelte/kit code in this repo (agents with kit-2 priors)
- `sources/kit/docs/kit3-facts.md` first, then `sources/kit/docs/remote-functions-surface.md`, `sources/kit/docs/loads-and-routing-surface.md`.

## H. What the junkyard concluded (decisions, interviews, do-not-resurrect)
- `sources/kit/docs/junkyard-plan-facts.md`, `sources/junkyard/oracle/process-lessons.md`, `sources/junkyard/oracle/oracle-lab.md`, `sources/junkyard/colocation/fixture-harness.md`, `sources/junkyard/go/cli.md`.
