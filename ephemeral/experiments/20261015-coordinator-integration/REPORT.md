# Real coordinator-public Kit/Go integration

**A stable Go coordinator successfully fronted two actual Kit browsers and Go/goja applications.** It selected its real public address before Kit initialization, isolated native updates, survived an application crash with HTTP503 during absence, and kept the existing Vite connection and browser state through application replacement. During Vite loss the application HTTP path remained usable; Vite recovery naturally reloaded the browser and reset its local state. This is an executed listener comparison, not a selected architecture or released skgo dev.

## Topology and public-origin setup

```text
browser ALPHA -> coordinator56612 -> app private51389, later51841
                                \-> Vite private56613
browser BETA  -> coordinator56621 -> app private51401
                                \-> Vite private56623
```

The Go test owns the coordinator and supervision responsibilities in one process; no separate coordinator executable is implied. Each replaceable Go child owns application handlers and existing size-two pooled goja rendering. Vite owns modules, transforms and HMR. Generated remote/load stubs throw; Node did not render application pages or execute application data bodies.

Requested56611 was deliberately occupied. The coordinator attempted actual binds and retained listener56612 before passing that actual origin into Kit config, generated output and Go load/remote origin configuration. BETA bound56621. This demonstrates coherent initial public-origin sequencing after a real collision, unlike the earlier Vite port-fallback experiment that retained the old configured origin. It does not qualify arbitrary hosts, TLS, changing public address mid-session or simultaneous launch races.

Vite config contains **no private Go target or application proxy**. The supervisor publishes the actual child address into its own atomic forwarding pointer after readiness/candidate reinstallation. Kit still needs the public origin. No private-port update needs to cross into Vite when Go restarts. Tooling routes and the explicit `/__hmr` upgrade go through the coordinator; application routes go directly to Go.

## Executed application and transport observations

`run.log` and `http-baseline.log` record two live roots, literal Go SSR3000 and exact public-origin headers. Parent independently opened both actual browsers and seeded separate drafts/counts/document identities. `browser/OBSERVATIONS.md` is the independent browser account; that directory belongs to the observer.

Emitted `/@vite/client` source has explicit public hmrPort56612/56621 and `/__hmr`, disabling direct private-socket fallback. The observer captured actual ALPHA HTTP101 `vite-hmr` at public56612, requestID73522.112 and public Origin56612. Coordinator-side `WS_PROXY` entries corroborate the upgrade path. These are real Vite WebSocket observations, unlike the earlier synthetic byte-tunnel test.

Actual watched native source edits installed ALPHA4101 and BETA5202, visible in independent Go SSR and hydrated Kit remote/load values. Each page retained its own draft/count/document and hydration. The first edit accidentally replaced `return total` with a literal, leaving `total` unused; both real compiler rejections retained3000 and browser state. Correcting to `return total+1101/+2202` installed the intended values. Raw rejection and repair remain in run.log; notices2 reflect error then repair. Do not label the accidental source error a successfully installed revision.

These listener fixtures reuse the prior native recovery harness with legacy Lower and package-only tc.Compile. They do **not** add whole-app/HostView speed qualification; that remains the separate retained-qualification evidence. ALPHA preparation967ms and BETA1.78s included serialization/contention and are not a controlled listener latency comparison.

## Actual Go crash and unavailable interval

The experiment killed exact owned ALPHA PID81340. A rapid Go HTTP sampler captured public56612 HTTP503 with body `COORDINATOR_GO_UNAVAILABLE`, then public200 with literalSSR4101 after recovery. Supervisor reinstalled the last successful native candidate before replacing ALPHA's private target51389→51841, PID85470. BETA PID81390/private51401 stayed unchanged. Evidence: `crash-http.log`, `crash-503.txt`, `crash-recovered.html`, run.log AUTO_RECOVERY.

Parent saw ALPHA notice3, literal4101, original draft/count/document, and the same WS73522.112 with no observed close/create/error. BETA remained5202 with its original state. This demonstrates request-visible target replacement and connection continuity across this actual child crash. It does not preserve arbitrary initialized Go state or implement structural rebuild.

The independent HTTP identity assertion passed, failed when its ALPHA input URL deliberately pointed to BETA despite HTTP200, then passed restored. Exact origin/label/value all detected the wrong instance. This is an assertion/input mutation, **not** corruption of the coordinator's internal forwarding table. Logs are under browser/.

## Vite loss and natural recovery are different

After native recovery, only ALPHA's owned Vite process group was killed; the restarted Vite helper waited for the observer before proceeding. Public `/orders` stayed200/SSR4101 while public `/@vite/client` returned503 `COORDINATOR_VITE_UNAVAILABLE` (`vite-down-http.log`). Parent observed the existing WS close and Vite's polling-for-restart console message while the mounted page initially retained state. BETA stayed healthy.

Vite restarted on private56613 with unchanged public56612. Without manual browser navigation/reload, ALPHA naturally loaded a new document, hydrated and showed4101, with empty draft/count0/notices0. BETA preserved its independent state. The browser event buffer did not retain the replacement handshake, so no replacement CDP handshake is claimed. Coordinator logs separately recorded `vite-ping` upgrades followed by `vite-hmr`; these are server-side protocol observations.

After this frontend recovery, the harness explicitly called `/__tool/build` to restore the restarted process's in-memory renderer-artifact rebuild handle. That startup handshake is harness-driven; it was not necessary for the browser's own automatic reconnect/reload, and automatic renderer tooling reinitialization remains an integration responsibility. An actual subsequent ALPHA Svelte edit changed FRONTEND_ONE→FRONTEND_TWO through ordinary HMR and fresh Go SSR. Artifact/pool replacement71.791ms was one sample. Parent inspected the rendered result; BETA stayed FRONTEND_ONE/5202. `http-final.log` independently asserted both headings, Go literals, origins and emitted public WS settings.

## Comparison and recommendation

The coordinator's main demonstrated benefit is operational ownership: supervision already knows replacement addresses and updates its own target table; there is no Vite target-propagation API or per-request backend-address read in Vite. Its public application address and useful app/tooling failure responses survive either upstream's failure. These benefits make coordinator ownership a credible preferred composition for this responsibility split, subject to the remaining route/metadata and production integration work; the user has not ratified it.

Both coordinator and Vite-first retain Vite's connection through Go replacement. Both use one HTTP hop for application requests. The coordinator adds a tooling HTTP hop and a proxied WebSocket seam that Vite-first handles directly, plus explicit routing precedence, upgrade/path/token forwarding and proxy-error ownership. The configured optimizer-cache path must be recognized or hydration can fail, as the earlier fixture proved. No controlled throughput or overhead timings were measured here, so this is not a performance win claim. A coordinator does not prevent Vite's normal full reload/state loss after Vite restart.

Go-application-public ownership avoids the normal application proxy hop but ties fallback process replacement to public connections unless listener ownership is extracted or inherited. This experiment supplies real evidence for the previously synthetic third alternative; it does not erase the source-supported tradeoffs of the other two.

## Source, reproduction and cleanup

`coordinator_test.go.txt`, `overlay.json`, Vite config, page and `source-snapshots/` preserve the exact fixture. Installed Kit3.0.0 was verified. Critical source mapped: Kit `src/exports/vite/index.js:497` origin definition; Vite8.3.1 `client/client.ts:33-76` inferred port/direct fallback; `server/ws.ts:220-236` exact path and HMR/ping subprotocol dispatch. Explicit-tag reproduction from this worktree:

```text
go test -overlay ephemeral/experiments/20261015-coordinator-integration/overlay.json -tags skgo_coordinator_experiment ./internal/hotreload -run '^TestCoordinatorIntegration$' -count=1 -v -timeout 45m
```

The test regenerates its owned runtime tree and remains interactive until runtime/STOP. Standalone Go commands preserve HTTP checks, crash sampling, Vite cycle and cleanup; inspect their literal expectations and current owned process identities before reuse. `source-snapshots/executed-crash-main.go.txt` is the executed fixed-PID sampler; the reusable crash command now discovers the current ALPHA identity from its own run log and verifies the matching backend marker before signaling. That safety-only command adjustment was not a new runtime qualification. Overlay paths are absolute to this worktree and require rebasing if moved.

All owned processes stopped: restarted Vite helper ended, original test stopped naturally with PASS361.60s, and every public/private/sentinel listener was checked closed. Owned runtime was removed only after source/log snapshots. `cleanup.log` preserves successful closure checks. The first cleanup attempt saw sentinel56611 still open and failed before removing runtime; a following formatting command masked that shell exit status. Parent detected the residual tree. The failed assertion is preserved as `failure-cleanup-port-still-open.log`; a fresh listener inspection and standalone cleanup then passed and removed runtime. Do not infer cleanup from a later command's exit status. No production packages, plans, worklogs, index or other experiments were edited by this lane. Initial generated-host import setup failure is retained as `failure-generated-host-imports.log`.
