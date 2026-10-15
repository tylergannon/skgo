# Who owns the development URL?

The listener decision determines which component must remain alive to preserve the browser's application origin and Vite connection. It does not decide native patching, module delivery to goja, or whether Go updates preserve page state. Those are separate axes.

## Three concrete arrangements

```text
Vite public:       browser -> Vite -> Go application
                              | own modules and HMR
Go app public:     browser -> Go application -> Vite modules and HMR
                              | own app handlers
Coordinator public: browser -> skgo dev -> Go application
                                    \-> Vite modules and HMR
```

The coordinator is a stable responsibility outside the replaceable Go application. It need not be a separate executable from supervision. Listener counts and process counts are different questions. Vite middleware hosting can change the arrangement again, but Vite's middleware API is Node middleware, not a Go handler.

| Practical consequence | Vite public | Go application public | Coordinator public |
|---|---|---|---|
| Ordinary frontend HMR | Vite handles its own connection directly | Needs upgrade proxy or deliberately separate HMR origin | Needs upgrade proxy or deliberately separate HMR origin |
| Go native patch without restart | Public listener stays; app refresh needed after installation | Public listener stays; app refresh needed after installation | Public listener stays; app refresh needed after installation |
| Go fallback restart | Public listener and Vite socket survive | Public listener disappears unless socket ownership is extracted/inherited | Public listener and Vite socket survive |
| App absent/bad build | Vite can still serve tooling; app proxy needs useful failure response | No public app server during absence, unless restart preserves listener elsewhere | Coordinator can answer a useful app-unavailable page while tooling continues |
| Vite restart | Public URL temporarily disappears; browser reconnect recovery tied to Vite | App URL remains, but proxied HMR disconnects | App URL remains, but proxied HMR disconnects |
| Whole dev coordinator restart | Usually replaces supervised children too; policy-dependent | Same | Public listener itself disappears |
| Routing responsibility | Must forward every app route without swallowing tooling | Must identify Vite tooling without swallowing app URLs | Must identify both; most routing machinery |
| Extra app HTTP hop | Yes | No | Yes |
| Private discovery | Go address -> Vite forwarding | Vite address -> app proxy | Both addresses -> stable edge |

These lifecycle comparisons follow the ownership of sockets; they are not measured browser behavior. Existing cooperation experiments already demonstrated real Kit hydration through both first and second front doors. Their one canonical origin and fixed routing do not establish arbitrary app routing or multiple instances.

## Pinned source facts

Installed `example/web/node_modules/@sveltejs/kit/package.json` is 3.0.0. Read `ephemeral/sveltekit-current/SKILL.md` first. Vite source below is the readable cached upstream tag at `/Users/tyler/src/skgo-token-cache/repos/dev-experience-20261015/frameworks/vite-8.3.1`; it is distinct from the bundled VitePlus artifact.

- Kit `src/exports/vite/index.js:497` substitutes `__SVELTEKIT_PATHS_ORIGIN__` from `kit.paths.origin`; `src/core/config/options.js:194-213` validates the configured origin. Pick/learn the actual public address before constructing origin-dependent transformed output, and rebuild/reinitialize it when that address changes. A private upstream port changing need not change public origin. This ordering is an integration requirement to demonstrate, not a proved dynamic-origin implementation.
- Kit's dev static middleware runs at `src/exports/vite/dev/index.js:272-295`; its returned post-handler installs application response handling at `:300-322` and calls Kit's server at `:388-397`. Therefore an app catch-all after Kit's responder is too late, and an unconditional catch-all before Vite would steal tooling. Fixture route lists are not a general solution. App assets versus tooling assets also need an explicit ownership policy consistent with Go owning application I/O.
- Vite `packages/vite/src/node/server/middlewares/proxy.ts:77-84` matches configured prefixes or regexes. Its default error handling at `:113-136` returns HTTP502 with an empty body for HTTP proxy errors, plus console diagnostics. A useful compile/restart page is additional integration work; default proxying does not supply it.
- Vite `packages/vite/src/node/plugins/clientInjections.ts:79-109` reads **server.ws** host/port/clientPort/path for socket addressing; **server.hmr** controls the overlay at `:88-90`. `packages/vite/src/client/client.ts:33-76` normally infers socket address from the imported Vite client URL, then permits direct fallback if no explicit socket port was injected. Set the public `server.ws.clientPort` in a proxy-attribution experiment so a broken proxy cannot silently pass through the private Vite port. Verify emitted client bytes on the actual VitePlus runtime; source-version matching alone is insufficient.
- Vite WS upgrade handler checks HMR/vite-ping subprotocol and exact HMR base/path at `packages/vite/src/node/server/ws.ts:220-236`. Browser connections also check the WS token at `:180-190`. Forward path, query token, Upgrade/Connection, Host/Origin as intended, and both `vite-hmr` and `vite-ping`. Routing every upgrade to Vite would steal an application's own WebSocket endpoint.
- Vite `packages/vite/src/client/client.ts:274-282` reacts to lost server connection by polling the old socket address and then `location.reload()`. Its ping uses `vite-ping` at `:480-500`. A live page surviving Go restart with its Vite connection intact has different state consequences from losing that connection and recovering via a full document reload. No topology promises arbitrary draft retention across error boundaries or Vite restart.
- Vite `packages/vite/src/node/http.ts:260-274` implements requested port0 by selecting a free port then trying a real bind; the source explicitly identifies the intervening race and throws if occupied. Requested nonzero ports can increment; strictPort fails at `:282-314`. Do not assume requested port equals actual public port. Bind-first Go `net.Listen(...:0)` retains its reservation and exposes Addr; closing that reservation before a child binds reintroduces a race.

## Coordinator probe actually executed

`go test -v ./ephemeral/experiments/20261015-listener-ownership -count=1`

`coordinator_test.go` starts a public Go ReverseProxy, two successive private HTTP app servers, and a synthetic tool server. It asserts literal application replies and public Host/Origin; obtains HTTP101 through the public proxy; keeps the same upgraded byte stream open; stops the application; receives a useful HTTP503 from the stable edge; exchanges bytes over the still-open tooling stream; replaces the app on a different private port; and receives the new literal application reply at the unchanged public origin. Exactly one tooling connection is observed. `coordinator.log` preserves the successful execution. No detached processes remain.

This establishes that a stable coordinator can isolate Go app replacement without listener inheritance, a fixed private app port, or a second browser-origin. It does **not** establish Vite HMR, browser reconnection, WebSocket framing, Kit origin initialization, native patching, or whole-app state preservation. The synthetic upgrade deliberately tests standard Go ReverseProxy's byte tunnel and lifecycle boundary; it is not labeled a real Vite WebSocket.

## What the evidence suggests

Vite-first is the smallest promising arrangement when preserving Vite's frontend loop and isolating Go fallback restarts are the priorities. It already owns the most fragile long-lived browser connection. It still needs complete application dispatch before Kit's Node responder, a real public-origin initialization sequence, and useful app-down diagnostics. It cannot be selected merely from tiny fixture hydration.

The coordinator alternative buys an independently stable application address and error surface through **either** upstream restart, and gives the Go supervisor direct control of failure presentation and upstream handoff. It pays for another HTTP routing seam, upgrade forwarding, and ownership policy. It does not prevent a Vite WS loss from causing Vite's full reload, and it is not a way to avoid Vite graph/renderer integration.

Go-application-first has the shortest ordinary app request path and direct handler control, but makes fallback app replacement a public-connection event. Moving listener/proxy ownership out of that replaceable process to repair this drawback converges on the coordinator alternative. Socket inheritance is another possible mechanism, not automatically required; it does not by itself preserve an upgraded connection owned by the exiting process.

## Smallest decisive next experiment

Use the already-hydrated real Kit fixture, add the coordinator front door, and inject explicit public WS clientPort to forbid direct fallback. Open the real browser through the coordinator; verify exact public HMR handshake; seed literal draft and counter; stop only the private Go app; verify the **existing** Vite WS remains connected; replace Go on a different private port; emit the existing Vite Go-ready event only after the new literal Go value is request-visible; verify refresh reaches that value and retains draft/counter. Then stop only Vite and observe recovery, explicitly allowing its documented full reload/state loss. HTTP success alone is insufficient.

Run the same app-stop boundary on Vite-first. If both preserve the Go edit/restart loop equally, coordinator selection needs a concrete benefit from its independent failure page or Vite-upstream replacement; an extra stable proxy is not intrinsically better. Full multi-instance tests and combined native-save timing belong to the parallel integration lane, not this synthetic probe.
