# Independent coordinator browser observations

Parent observer used actual Chrome tabs through CUA, separately from the fixture author. These are bounded live observations, not a completed development-server claim. The fixture uses the earlier native compilation path; do not substitute retained-preparation timing into these runs.

## Baseline and public transport

ALPHA public `http://127.0.0.1:56612/orders` (requested56611 deliberately occupied); BETA public `http://127.0.0.1:56621/orders`. Both hydrated true, remote total3000 and Go SSR total3000. Entered draft `COORD_ALPHA_42` / `COORD_BETA_84`, clicked each Increment once. ALPHA document `973ec88a-8b5b-407c-8a88-0a9ce0490490`; BETA document `1bd09fe2-7018-424d-a630-4e3a1b6440cf`.

CDP captured ALPHA request `73522.112` to `ws://127.0.0.1:56612/__hmr` with Vite token, HTTP101, `vite-hmr` subprotocol and public Origin56612. This is actual observed public WebSocket transport. The tool returned handshake headers; session cookies are intentionally not copied into this report.

## Go edits, failure and recovery

The first author edit left an unused local, so actual compile rejection was observed: total3000 retained, notice1, `GO_COMPILE_REJECTED`, same draft/count/document. Screenshot was visually inspected in CUA. After author repaired the source, ALPHA remote+SSR4101 and BETA remote+SSR5202, both notice2, own draft/count1/document/hydration unchanged. ALPHA rejected/accepted notifications used the same WS73522.112, with no observed close/create/frame error between these cursors.

After the author-triggered ALPHA child crash/recovery, observed notice3 and remote+SSR4101 with the same draft/count1/document/hydration. The recovery notification again used WS73522.112, with no observed close/create/frame error in the captured interval. Process/address replacement and app-down HTTP sampling belong to the author's raw run/probe logs.

## Independent HTTP assertion control

`httpcheck/identity_test.go` asserts exact public origin, instance label and supplied SSR4101/5202. Uncached positive→ALPHA pointed at BETA→restored-positive runs are captured in `http-positive.log`, `http-wrongtarget.log`, `http-restored.log`. The wrong-target run returned HTTP200 but failed ALPHA origin, label and value expectations. This proves the identity assertions detect a misrouted result; it did not corrupt the coordinator's internal target table.

## Vite outage and natural restart

Author stopped ALPHA Vite while leaving coordinator/Go and BETA alive. Observed actual WS73522.112 close and console `[vite] server connection lost. Polling for restart...`. During outage ALPHA's existing document still showed4101, draft/count/document unchanged. Independent Go HTTP identity test passed for both app responses during outage (`http-vite-down.log`); the author separately asserts tooling503.

Author restarted Vite; parent did not reload or navigate. A DOM command timed out during the natural navigation; subsequent screenshot (visually inspected), DOM and console showed a new hydrated document `0c4d01a7-bb8f-4764-b041-a5990dc016ff`, remote+SSR4101, empty draft/count0/notices0 and Vite connected. BETA retained5202/draft/count1/document. CDP's retrieved buffer contained no new handshake in this interval, so this report does not claim an observed vite-ping exchange or new WebSocket request ID. Actual document replacement and reconnection console messages were observed. Stable coordinator availability did not preserve ALPHA browser state across Vite restart.

## Real Svelte edit after Vite restart

After the author explicitly reinitialized the renderer artifact handle through the Go tooling request, an actual ALPHA component save changed the browser heading to `ALPHA FRONTEND_TWO`. Parent DOM and visually inspected screenshot showed hydratedtrue, remote+SSR4101 and the same post-restart document0c4d01a7-bb8f-4764-b041-a5990dc016ff. BETA remained `BETA FRONTEND_ONE`,5202, its seeded draft/count1/document. The author separately verifies fresh Go SSR and records renderer/pool replacement timing. This observation does not prove the renderer's post-Vite-restart initialization was automated; that handshake was explicitly invoked in the harness.

Screenshots were displayed and inspected through CUA. No local image file was returned by that interface, so no archived screenshot artifact is claimed. These DOM/transport records preserve the observations and their boundaries.
