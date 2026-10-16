# Renderer architecture discrimination, 2026-10-15

**All three bounded renderer routes ran. The coherent artifact route is the cheapest demonstrated update for this fixture. The unchanged upstream ModuleRunner is compatible with actual Kit rendering; it is not ruled out.** Its retained-HMR variant received real Vite `full-reload` events for ordinary dependency edits, which makes upstream clear and reevaluate the graph. No selective evaluated-module reuse was demonstrated on this graph.

This extends the prior [renderer experiment](/Users/tyler/src/skgo-token-cache/experiments/vite-cooperation-20261015/renderer/README.md). It follows GOAL and the semantic index's framework, executed-experiment and candidate leaves. It does not implement a development server or select the whole architecture.

Run from the repository root:

```sh
go test ./ephemeral/experiments/20261015-architecture/renderer -run TestRendererArchitecture -v -count=1
```

Requires the installed example dependencies, `vp`, and Go. Missing prerequisites fail the test. The Go runner makes a disposable fixture, starts `vp dev`, drives it through HTTP, owns process cleanup, and removes its fixture. Each Vite process has an isolated cache directory. The adapter-boundary JavaScript uses the existing adapter environment and Vite libraries. No production files changed.

## Actual routes and assertions

The fixture is a real Svelte page in a layout, rendered with Kit's root and props closure through the existing `ssr.New` pool. Every expected string is a supplied literal. It runs four changes to a dependency with its page importer unchanged, then adds an imported Svelte component and changes the page's import edge. Both the changed heading and the new component's literal `69` render. This exercises Vite graph changes, but **not route/node-table membership changes**.

1. **Coherent artifact:** retained ViteBuilder rebuilds the adapter's goja environment, followed by its existing Rolldown fold. A new engine compiles the resulting script. It does not consume the development module graph.
2. **Fresh runner pool:** the unchanged upstream ModuleRunner imports actual Vite `fetchModule` output in goja. A new engine/pool is admitted for each revision. Vite owns resolution, transforms and graph invalidation.
3. **Retained runner pool with actual upstream HMR:** a Vite `DevEnvironment` custom channel records its real hot payloads. Each pooled VM receives them through the ModuleRunner's documented transport `connect` callback, and its unchanged upstream HMR handler applies them. The Go callback only transports data. An event cursor is local to each VM so both observe the same events. The experiment drains events before a request; it does not demonstrate asynchronous push into idle runtimes.

Eight concurrent HTTP requests exercise each route at selected stages and check positive literal bodies. Healthy engines create two lifetime VMs in their configured size-two pool. The retained engine creates a third lifetime VM after one deliberately failed render is discarded; `Created` is cumulative, not a live-slot counter. A rejected artifact update leaves its existing engine serving the exact last-good heading. A fresh runner candidate rejects invalid source. A retained HMR runtime reports the authored source error; repair updates both pooled runtimes and concurrent HTTP requests return the repaired literal. This does not guarantee retained-HMR last-good service on an invalid edit: one observed request failed and its VM was replaced.

## What unlocked ModuleRunner

The earlier NUL/file-URL compatibility normalization remains confined to the experiment bootstrap. Standard pinned Rolldown CJS output removes the default evaluator's unsupported native-import syntax. No `require` implementation was added.

The remaining OXC helper was not a missing JavaScript engine capability. Pinned Vite `fetchModule` unconditionally externalizes a **bare specifier with an importer**, independently of `noExternal`. Adapter lowering introduces `@oxc-project/runtime/helpers/awaitAsyncGenerator` after ordinary import analysis. Calling Vite's own `environment.pluginContainer.resolveId` returns its existing NUL-prefixed virtual helper module. Fetching that already-resolved ID without an importer internalizes the helper. The helper then traverses Vite's normal load/transform pipeline. No resolver, module graph or Vite algorithm was implemented in Go.

The first control retained the importer with the NUL ID and failed at Vite's bare-import resolution. `first.log` preserves that failure. The corrected bounded call produced real literal Kit markup (`second.log`). A later custom-channel control exposed Vite's filesystem check for the adapter entry outside the fixture; the trusted loopback experiment channel now uses Vite's documented `skipFsCheck` option, as the existing runnable environment did. This is not a proposed public security policy (`retained-first.log`).

## Measured comparison

`final.log` is the corrected final run, after coordination to avoid other agents' heavy build/tests. Four ordinary dependency revisions are the repeated sample. Values are observations from one small fixture, not application benchmarks. Wall durations include local HTTP overhead. Renderer timings exclude file-write/watch delay and browser delivery; they are not save-to-visible measurements.

| Stage | Observed range, n=4 | Work performed |
|---|---:|---|
| Artifact Vite update + HTTP | 41.52–51.89 ms | Environment build and Rolldown fold |
| Artifact new pool | 3.28–4.80 ms | Compile shared program and initialize first VM |
| Artifact first render | 0.189–0.230 ms | Render supplied literal; no module transport |
| Fresh runner first render | 149.68–176.41 ms | Warm Vite graph, fresh evaluated graph; 410 transport calls, approximately 3.61 MB |
| Fresh runner new pool | 2.04–2.35 ms | Compile bootstrap and initialize first VM; graph import excluded |
| Fresh runner subsequent warm render | 0.290–0.485 ms | One cached-entry transport call, 25 response bytes |
| Retained runner after dependency edit | 152.00–169.58 ms | Existing VM, upstream HMR full reload, 411–422 transport calls, approximately 3.61–3.70 MB |

Initial complete Kit build was 753.82 ms. Creating the custom development environment and compiling both runner bootstraps took 104.50 ms. First graph import with initially cold Vite took 383.25 ms. The new-component import-edge change took 49.87 ms artifact build, 174.81 ms fresh runner import, and 185.84 ms retained-HMR update. Inline maps are included in module wire bytes; removing/batching/caching them was not investigated. Some calls are getBuiltins, cache acknowledgements or event retrieval, not unique modules.

The exact hot payloads are preserved in `hot-payloads.log` in the cache. They contain `full-reload` with `triggeredBy` pointing to the actual dependency or page source. Upstream `hmrHandler.ts` then calls `runner.evaluatedModules.clear()` and reimports entrypoints. Retaining the VM therefore did not preserve evaluated dependency modules for these edits. This does **not** establish a universal lower bound for a runner with a different valid acceptance boundary or transport.

The final combined process held all three current engines and their pools alive while sampling resources: Go heap 92,303,272 bytes / 307,768 objects; Vite process RSS 551,780,352 bytes and heapUsed 161,761,480 bytes. This is a combined single observation, not per-route memory attribution or bounded-retention proof. The test run transferred approximately 98.9 MB over 11,377 transport calls across all imports, pool members, errors and repairs. The folded artifact was 123,897 bytes (124,053 after adding the component); runner bootstraps were 66,349 and 66,636 bytes, excluding fetched modules.

## Decision supported, and limits

Prefer the artifact/pool-replacement route for the next integrated proof on this evidence: lower measured update cost here, existing renderer compatibility, and explicit last-good candidate admission. Keep ModuleRunner as a demonstrated alternative. Its viability is now actual Kit execution, not merely a simple-module claim; a reason to select it would need to outweigh the observed full-graph HMR work, module traffic and per-VM coordination. No Go port follows from these findings.

The artifact route still needs dynamic Kit metadata and route membership; the fixed node table does not solve those. Both runner and artifact fixtures use production compilation for the renderer, because the existing adapter states `DEV=false`; the control middleware temporarily selects that mode. This is experimental setup, not safe concurrent product configuration. The stock browser dev compiler/HMR and this renderer need one integrated proof.

Not covered: application remotes/loads/universal hooks, navigation/hydration, active-render changes, coherent mixed-file transactions, source-map quality for runtime exceptions, large-app scaling, asynchronous event delivery/reconnect, Vite restart, arbitrary HMR accept/invalidate cycles, new routes or matcher/remote metadata changes. The channel's inbound invalidate callback is not wired; this fixture only uses Vite-originated full-reload. Pool retirement and memory reclamation are not qualified. Timing includes this particular per-module HTTP transport. A different transport is a candidate, not demonstrated optimization.

## Evidence and pins

- [Final assertions and measured output](final.log); [executable Go experiment](renderer_test.go); [Vite adapter boundary](vite.config.js); [artifact hashes and installed package versions](pins.json).
- Cache: `/Users/tyler/src/skgo-token-cache/experiments/architecture-20261015/renderer/`, including all raw successful/failed runs, `final-vite.log`, `hot-payloads.log`, and copied source.
- Kit 3.0.0 patched package, Svelte 5.57.1, Svelte Vite plugin 7.3.1, VitePlus Core 1.0.0 containing Vite 8.3.1, goja e9f698900731, goja_nodejs 1f56ff5bcf14. Package and relevant executed-file hashes are in `pins.json`.
- Exact installed Vite `dist/vite/node/module-runner.js` is copied under cache `source/`. Readable version-pinned source: `repos/dev-experience-20261015/frameworks/vite-8.3.1/packages/vite/src/node/ssr/fetchModule.ts:44`, `src/module-runner/hmrHandler.ts:38`, `src/shared/moduleRunnerTransport.ts:18`, and `src/node/server/environment.ts:148` under the token cache. Those are source maps for the real executed APIs, not independently executed upstream tests.
- The failed cumulative-runtime-count assertion is preserved as `created-counter-control.log` in the cache. It was corrected to the supplied fixture expectation of one discarded VM plus two healthy slots; no product change was needed.
