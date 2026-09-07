# skgo

A SvelteKit application server written in Go.

A Go program owns the socket and the process. Its Kit-facing interface is an
ordinary SvelteKit adapter, so the frontend stays plain SvelteKit — kit's
tooling, kit's conventions, no Go-isms. Go serves kit's client-rendered output
natively, makes the trust decisions, and answers the endpoints kit's client
already calls: remote functions (`/_app/remote/...`), server loads
(`__data.json`) and the routes a `+server.ts` declares.

To a Go developer: a real frontend framework for a Go monolith. To a Svelte
developer: the app is still SvelteKit.

## What this means

**Every server endpoint is Go's.** Not "Go where you want it, kit otherwise" —
all of them. Loads and remote functions are written in Go with end-to-end
types, and a `+server.ts` route is an ordinary `net/http` handler written
beside it; the JavaScript kit requires is a generated stub that throws, so any
real response proves Go answered. Running remote functions or server routes in
TypeScript is not a supported mode.

**Pages are rendered in the Go process.** A page document leaves Go with its
markup already in it, rendered by SvelteKit's own renderer inside an embedded
JavaScript engine — one process, one binary, no Node at request time and no
sidecar. A page whose branch sets `ssr = false` still gets kit's SPA fallback,
and one that sets `csr = false` gets no script at all.

**No application I/O executes in JavaScript.** That is narrower than "no
JavaScript runs": the engine executes kit's root component, Svelte's renderer
and the app's components, and nothing in it reads a file, opens a socket or
sets a timer. Every remote function's body is still the generated stub that
throws; the only path by which a value reaches the engine is a call back out
to Go, which is what makes a rendered value proof that Go answered.

The engine is [goja](https://github.com/dop251/goja), pure Go, embedded in the
binary — no cgo. Renders are served from a pool of runtimes, which is
mandatory rather than an optimisation: a re-entrant render on one runtime
returns empty markup with no error.

One binary. One build gesture. Node is a build-time dependency only.

## Getting started

The `example/` directory is a working SvelteKit app served by skgo — the best
place to see the pieces fit together. The `Justfile` at the repo root holds
the recipes that build and run it:

```sh
just install    # node deps for the example app and its Gherkin suite
just build      # build the frontend (run this first in a fresh tree)
just vet        # go vet, both modules
just test       # go test, both modules
just serve      # the example server, against the built frontend
just dev        # vite in dev, for the proxied path
just e2e        # the Gherkin suite against a server you started
```

Run `just --list` for the full set, including `just generate` (regenerates the
link tree, the throwing stubs, and the wire types) and `just ports` (what is
already listening, before you bind one).

A fresh checkout needs `just build` before `just vet` or `just test` will
succeed: `example/web/dist.go` embeds the built frontend, so nothing under
`example/...` compiles until it exists once.

## Requirements

- Go, at the version pinned in `go.mod`.
- Node and [mise](https://mise.jdx.dev/), for the example app's frontend
  toolchain (`example/mise.toml`) — Node is not required to run a built skgo
  binary.

## License

MIT. See [LICENSE](LICENSE).
