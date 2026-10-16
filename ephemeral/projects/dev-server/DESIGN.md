# Native Kit development with Go over Unix sockets

[GOAL](GOAL.md) controls. Ordinary `skgo dev` uses this topology:

```text
Browser → Go coordinator public listener → Vite/Kit over vite.sock
                                      └→ Go remote wire handler over go.sock
Vite/Kit → request-aware Go callbacks over go.sock
Go       → Kit internal-fetch continuation over vite.sock
```

Each session owns a short unique directory containing `vite.sock` and `go.sock`.
The coordinator establishes the public origin, proxies HTTP and WebSocket
upgrades, supervises both children, waits for listener readiness and owns cleanup.
Private sockets, caches and Go session state are isolated between instances.

Kit owns page documents, data requests, action orchestration, route selection,
serialization, assets and HMR through its live graph. Generated callback bodies
execute application loads, actions, endpoints, hooks and SSR remote calls in Go.
Browser remote requests use the existing Go wire handler at its configured
prefix. There is no second development renderer graph or custom reload policy.

The application supplies
`NewDevelopmentHandler(fs.FS, string, *skgo.NativeDevelopment)` and shares setup
with its production constructor. Development factories retain the application's
Go registries, hooks and options without creating goja runtimes; nil factories
use ordinary production construction. Filesystems remain ordinary `fs.FS`.
Captured compiler inputs govern analysis, compilation and execution, including
Go workspace members and replacements.

A Vite instance epoch identifies restarts. Manifest and environment content
identify constructor changes, so effective configuration updates replace the Go
handler without resetting healthy Go state for unrelated frontend edits.
Private environment inputs and artifacts are denied through the public server.

One Go request handle owns a logical Kit request. Callback completion, fetches
and cancellation use that owner; a restarted child rejects stale handles rather
than replaying mutations. Fetch continuations retain each response body until
read or explicitly closed. Kit's supplied fetch remains the continuation for
internal routing; synchronous ResolveOptions predicates use a transport-only
worker. Neither seam moves application behavior into JavaScript.

Compatible Go edits use the native preparation/install path. Unsupported edits
rebuild. Successful installation publishes the refresh; failed edits retain the
healthy service until repair. Arbitrary initialized state and seamless in-flight
completion across child crashes are not promised.

Direct SSR UDS subfetch response cookies are server-readable but are not
automatically imported into Kit's pending cookies or outer page response. This
accepted limitation is distinct from normal internal Kit fetch handling. Kit's
public cookie parse/set APIs are a candidate explicit remedy; automatic relay
has not been qualified.

Production still builds its renderer for pooled goja inside the Go binary.
Native development success does not establish production behavior, or vice
versa. Read the installed pinned Kit source using
[the Kit source guide](../../sveltekit-current/SKILL.md) before changing mirrored
behavior. [CONTINUE](CONTINUE.md) gives current qualification and limitations.
