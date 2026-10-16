# Development server goal

Run ordinary `skgo dev`, open the application, edit Svelte or Go, and see the
change automatically. Preserve healthy browser drafts and state where the
change permits. Show useful errors and recover when fixed.

The selected development architecture uses native Vite/SvelteKit SSR. Kit owns
its live modules, routing, rendering and normal HMR. Application loads, remotes,
actions, endpoints, hooks and data access remain Go implementations. The Go
coordinator owns the public listener; Vite and the Go application use sockets
inside an instance-private directory.

Compatible Go edits use native replacement. Other edits rebuild the application.
Arbitrary initialized Go state need not survive rebuilding. No custom waiting
page, forced navigation policy or modification of Vite's browser lifecycle is
part of this design.

Production remains one cgo-free Go binary with pooled goja SSR and no Node at
request time. Development and production require separate behavioral checks.

Automatic response-cookie propagation from a direct SSR UDS subfetch into Kit's
pending cookies and outer page response is an accepted temporary limitation.
It does not relax other cookie or authentication behavior.

[DESIGN](DESIGN.md) describes the implemented boundaries. [CONTINUE](CONTINUE.md)
records the current proof and remaining work.
