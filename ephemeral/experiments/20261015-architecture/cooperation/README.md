# Two front doors over a real Go-rendered Kit page

Bounded architecture experiment, 2026-10-15. GOAL remains authority. The fixture joins the previously proven artifact-bundle renderer to the existing Go `NewSSR`, `NewLoads`, `NewRemotes`, and static handler, with stock Kit dev client modules and Vite HMR. The application page is rendered by pooled goja in the Go process. Kit-required server load, remote and endpoint stubs throw if their bodies run in Node.

## What ran

`COOPERATION_INTERACTIVE=1 go test -tags skgo_cooperation_experiment -timeout 35m -v ./ephemeral/experiments/20261015-architecture/cooperation -run TestCooperation -count=1`

- Application listener 56312: Go answers all application requests directly.
- Go edge 56313: application requests run directly in Go; tooling paths/WebSocket upgrades are proxied to Vite 56311.
- Vite edge 56311: Vite's standard `server.proxy` forwards the fixture application paths to Go 56312; tooling stays in Vite.
- Two processes: the Go test hosts both Go listeners for comparison, and `vp dev` hosts Vite. Extra Go edge listener is experimental comparison apparatus, not an architecture requirement.

For all three paths the Go test asserted document HTTP200 containing literal `Go load: 3000`, actual `__data.json` containing3000, actual remote query containing3000, static text `GO_STATIC_731`, and header `X-Cooperation-Owner: Go`. POST to `/api` preserved literal body `BODY_983`, cookie `draft=COOKIE_419`, HTTP201, and two independent Set-Cookie headers through either edge.

Stock Kit's client transform of `src/lib/value.remote.ts` produced `__remote.query('4o58so/value')` while the callback remained a throwing Node stub. Actual Go remote returned3000. `remote-transform.txt` preserves the exact transformation. This supports ordinary Kit metadata construction for the generated-stub shape without invoking application callback I/O in Node.

## Request cost

30 sequential loopback document requests per path in the corrected isolated-cache run, warmed Go engine, sequential order direct→Go-edge→Vite-edge. p95 is sample index28, not a modeled confidence interval. Raw sorted samples are in `run.log`. Machine had other experiment activity; these are preliminary bounded observations, not general throughput results.

| Path | p50 | p95 |
|---|---:|---:|
| Go application direct |0.335ms|0.904ms|
| Go edge, in-process application dispatch |0.238ms|0.794ms|
| Vite edge, HTTP hop to Go |0.881ms|1.628ms|

The additional HTTP hop is measurable but below a millisecond at these percentiles on this tiny fixture. Both topologies are feasible at the application-byte boundary. This is not a reason on its own to select either.

## Browser failure and corrected run

Initial HTTP-only checks passed, but actual browser stayed Hydrated:false. The transitive optimized `esm-env` and `devalue` module requests returned504. Fixture default cacheDir resolved through the node_modules symlink into a shared cache used by concurrent different configurations. Run2 isolates `.vite-cooperation` inside the fixture. Parent browser then observed correct nonempty page, Go3000 and Hydrated:true, with no error/warn logs. The failed logs are retained as `run-initial-optimizer-failure.log` and `vite-initial-optimizer-failure.log`; successful HTTP checks alone did not establish hydration.

## Browser/control instructions

Open `http://127.0.0.1:56313/`: initial heading `COOPERATION_FRONTEND_ONE`, Go value3000, initially false hydration marker becoming true, draft input, Count button. The Go2400 button changes fixture Go data then invokes stock `refreshAll`. Break/Repair exercise Go load error handling, not compiler errors or native patching. `/__control/value?value=900` repairs backend data even when an error boundary removed buttons. `/__control/frontend` edits the Svelte source and rebuilds the artifact renderer, letting Vite own frontend HMR. `/__control/stop` ends the experiment.

Vite edge `http://127.0.0.1:56311/` serves the same Go document and dev modules. Controls currently belong to the Go edge; this fixture's Vite forwarding rule enumerates only application routes, so use Go-edge control URLs when manipulating it from elsewhere.

## Source mapping and limits

Entry through semantic index README → dev-executed-experiments, dev-framework-runtime, dev-experiment-candidates → preserved prior renderer source and pinned upstream source. Kit3.0.0 installed under this worktree is verified. `src/exports/vite/dev/generate_manifest.js` supplies dev client entry/app path shapes; `src/core/utils.js` supplies `__sveltekit_dev` and runtime filesystem URL behavior. Existing Go `document_assemble.go` emits Kit's bootstrap with those fixture paths. Vite config's proxy is the standard Vite API; Go proxy is net/http/httputil.

The node table, route set, and bootstrap metadata are fixed for this three-node fixture. Live add/delete routes, computed options, metadata refresh, CSS SSR collection, base paths, production CSP, arbitrary asset-route collisions, fallback app restart and WebSocket reconnection are not established. The origin is canonically56313, so a second functioning URL is not a supported multi-origin contract. HMR delivery alone does not prove the socket traversed the Go proxy rather than Vite's direct fallback. Native compilation/installation is deliberately left to the separate native lane; the fixture Go value switch is not native live patch proof. Artifact rebuild is a separate current candidate, not a selected renderer architecture.

## Joined frontend rebuild observation

Parent clicked the fixture's actual source-edit control. Vite recorded ordinary HMR for `/src/routes/+page.svelte`. Retained artifact environment build took70.0465ms; complete control request interval through build plus fresh two-slot Go renderer installation took77.1267ms (one sample, excludes browser completion). Independent fresh HTTP document captured as `document-after-frontend.html` contains literal `COOPERATION_FRONTEND_TWO` and `Go load: 1800`, establishing the server renderer also adopted the edit rather than only the mounted browser.

Parent's healthy Go-data action seeded draft `ARCH_DRAFT_KEEP_73` and Count3, then observed Go2400 with draft/count retained and Hydrated:true. Browser timer15.80ms covers direct control fetch + public Kit refreshAll + Svelte tick, one sample. The timer excludes native compilation/installation and does not cover the separate Vite custom-event path.

## Browser state and error-repair results

Parent browser saw ordinary Svelte HMR change FRONTEND_ONE→FRONTEND_TWO and retain Go2400, while the component draft emptied and count reset0. It then seeded literal draft `VITE_EVENT_KEEP_91` and Count2. Backend control changed Go data to1800, followed by Vite `/__cooperation/notify`. Stock Vite custom event → mounted handler → public Kit refreshAll visibly produced1800 while preserving that draft and count.

Breaking the Go load visibly produced `GO_LOAD_ERROR 500`. Repairing Go data to900 then sending the same Vite event visibly recovered `COOPERATION_FRONTEND_TWO`, Go900 and Hydrated:true. Draft/count were empty/0 after this error-boundary transition. This establishes useful failed-load recovery in this case, not arbitrary error/state preservation. Parent preserves actual DOM/log/screenshots under the sibling browser evidence directory.

The build tag isolates this deliberately interactive experiment from ordinary `go test ./...`; the original recorded runs precede adding that tag and otherwise execute the same test body.

Vite-edge browser also hydrated the same Go-rendered page correctly: initial FRONTEND_TWO, Go900, Hydrated:true, no errors. Parent seeded `VITE_EDGE_KEEP_52` and Count1, changed backend to1200 and sent the Vite event; browser visibly reached1200 while retaining the draft/count. This establishes the healthy Vite-edge event path, alongside the Go-edge path above.

Cleanup: after parent browser inspection, stop control completed the experiment with PASS (273.33s includes interactive idle time). All three listening ports were confirmed closed and generated fixture directory removed. Preserved source regenerates the fixture. No production files were changed.
