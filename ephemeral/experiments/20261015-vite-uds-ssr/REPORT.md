# Native Kit development SSR over Unix sockets

**Result: the narrow runtime composition works.** Unmodified pinned Kit3.0.0/VitePlus1.0.0 (bundled Vite8.3.1) serves development SSR and browser modules from a Node HTTP server listening on `vite.sock`. A public Go HTTP reverse proxy reaches that socket, including an actual HMR WebSocket upgrade. An actual Kit `+page.server.js` load calls `event.fetch('/go/value?...')`; its explicit `handleFetch` hook delegates that request over `go.sock` to a Go handler. Application literals come from Go. No Goja renderer, custom waiting page, full-reload interception or Vite lifecycle changes are used in this fixture.

This is a transport/SSR POC, **not a complete SKGO development bridge**. Normal server-load/remote callbacks execute directly in Kit and are not automatically converted to HTTP by `handleFetch`; the separate bridge mapping owns that question.

## Executed input and run

`main.go` is the Go harness/proxy/fixture application; `server.js` is only the necessary supported middleware-mode Node HTTP host; `hooks.server.js` is the explicit fetch-to-UDS integration. No production source was edited. `vp exec node server.js` runs each frontend from a private generated web fixture sharing the installed authoritative dependencies. Pins were read from installed package source before execution.

From repository root:

```
go build -o /tmp/skgo-vite-uds-poc ./ephemeral/experiments/20261015-vite-uds-ssr
/tmp/skgo-vite-uds-poc
```

The executed detached run had an exit watcher and logged `run.log`. ALPHA public57331 and BETA public57332 used distinct `/private/tmp/skgo-uds-2510439863/{ALPHA,BETA}/{go.sock,vite.sock}`. Parent socket directory is0700. Node readiness reported `vite.httpServer===null`; Node's supplied HTTP server owns the Unix listener. Direct real requests to both Unix sockets succeeded (`direct-vite-uds.html`, `direct-go-uds.json`). `node-unix-listeners.txt` records Node UDS descriptors; `node-tcp-listeners.txt` is empty after querying both actual Node PIDs6542/6545 for TCP LISTEN. Only the Go public proxy binds the intended TCP frontend addresses.

## Actual results

- Kit SSR returned independently supplied `ALPHA_4101` versus `BETA_5202`, proper instance identity, request cookie `COOKIE_8831`, Authorization `Bearer AUTH_9942`, load request marker `LOAD_7719`, and each public Host. Complete SSR documents and headers are saved.
- A deliberate wrong-Go-socket header on the ALPHA SSR request reached BETA, yielding `BETA_5202` despite HTTP200; this fails the literal ALPHA expectation. Restoring the proper path returned `ALPHA_4101`. `negative-wrong-target-ssr.html` and `run.log` preserve this control.
- Real public proxy POST reached Go UDS with `BODY_5588`, query `/go/value?case=post`, Cookie `COOKIE_POST_3366`, marker `POST_4477`, public Host, and both separate Set-Cookie headers (`public-go-post.*`). This is direct application proxy fidelity, separate from SSR subfetch semantics.
- Browser hydration worked (Count0→1). Actual public HMR `ws://127.0.0.1:57331/__hmr?...` received HTTP101 with `vite-hmr`. Editing only ALPHA's Svelte heading caused the ordinary Vite JS-update frame and visible `UDS_FRONT_ONE→UDS_FRONT_TWO`; BETA stayed `UDS_FRONT_ONE`. No error/warning was observed. This component's Count reset1→0: arbitrary edited-component state preservation is not claimed.
- ALPHA's Go listener was closed and reopened at the **same** `go.sock` with supplied `ALPHA_7407`. Fresh native Kit SSR immediately obtained7407 without restarting Vite or retargeting either proxy. BETA stayed5202. A fresh hydrated browser document showed7407. The existing browser remained4101 until an explicit manual reload: automatic Go-change invalidation was not implemented or proved.

`BROWSER.json` is a clearly labelled transcription of actual CUA/CDP outputs, including exact HMR frame and request identity; full tool records remain in chat. Screenshots were visually inspected; no local screenshot file is claimed. Independent validator also opened a fresh tab, saw7407, and clicked the counter successfully.

## Request/cookie boundary

The small hook manually forwards inbound Cookie and Authorization for its same-origin `/go/` subfetch when credentials are not `omit`, plus the explicit fetch headers/method/body. It does not purport to implement every Kit fetch rule. Both Go Set-Cookie headers reach a direct public Go response, **but do not propagate from the SSR subfetch into Kit's outer document response** (`ALPHA-ssr-headers.txt`). This was independently confirmed. Kit's fallback fetch processing normally handles cookie inheritance and `set_internal`; the early hook response bypasses it. A complete bridge must preserve cookie scope/options and outer-response effects deliberately. Accept-Language/context forwarding, abort propagation, streaming and deferred responses are also unqualified.

## Pinned API support versus runtime proof

- Installed Kit `src/runtime/server/fetch.js:31` calls handleFetch before its inner fetch behavior; same-origin fallback later calls Kit internal respond. Therefore this hook explicitly selects `/go/`, rather than assuming native fetch reaches the external Go service.
- Installed VitePlus Core `dist/vite/node/index.d.ts` supports `server.middlewareMode:{server}` and `server.ws.server`; the provided server can listen on a Unix path using Node's HTTP API. Actual UDS SSR and WS results above establish this composition beyond type/API support.
- Public origin is configured before Kit/Vite initialization. Two separate cache/Kit/socket directories were executed. This run used fixed free public ports; it did not repeat occupied-port fallback or simultaneous startup races.

## Endpoint and remaining questions

The requested narrow composition—native Kit dev SSR, both private listeners on UDS, Go public proxy, Go-supplied value/cookie/request context, public HMR, two instances and same-path Go reconnection—has runtime evidence. It does not establish direct Go remotes/load/action/hook delegation, complete cookie/transport semantics, native-patch integration, structural-overlap behavior, Vite process-crash restart, production parity or end-to-end performance. No Vite behavior modification or forced-wait/reload design is justified by this POC.

The Go backend handlers and public proxies share one Go harness process in this narrow test; all calls still traverse real Unix-domain HTTP sockets. Replacement closes/reopens the Go listener, not a separate child-process crash. Full child supervision is not proved here.

Lifecycle cleanup status is recorded after independent inspection finishes.

Cleanup completed after independent inspection: harness exited normally with status0; actual Node PIDs6542/6545 are gone; no TCP LISTEN remains on57331/57332; private `/private/tmp/skgo-uds-2510439863` and generated fixture runtime directories were removed. Stop HTTP saw an empty reply because cancellation closes its server; actual process/listener/directory checks establish cleanup. Shell recursive-delete syntax was rejected; checked filesystem cleanup removed only this generated runtime directory.
