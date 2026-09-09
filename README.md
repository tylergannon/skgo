# skgo

**SvelteKit on the frontend. Go on the server. One binary in production.**

skgo is a SvelteKit application server written in Go. You keep SvelteKit 3,
Svelte 5, Vite+, routing, components, remote functions, server loads, and
`+server.ts` conventions. Go owns the socket, trust decisions, application I/O,
and every server endpoint. In production, Go also renders pages with Kit's own
SSR bundle in an embedded JavaScript engine—there is no Node process or sidecar.

> skgo is pre-1.0 and tracks the SvelteKit 3 prerelease pinned by this
> repository. The [example app](example/) is the compatibility contract and the
> best tour of what currently works.

## Start an app

The default scaffold uses Go and [mise](https://mise.jdx.dev/); mise installs
the pinned Node and Vite+ versions used for the frontend build.

```sh
go run github.com/tylergannon/skgo/cmd/skgo@latest new hello
cd hello
mise trust
mise run build
./bin/hello
```

Open <http://127.0.0.1:8080>. The generated project is small on purpose: an
ordinary SvelteKit app in `web/`, Go beside the routes that use it, and a Go
binary in `cmd/`.

For development, build once and run two terminals:

```sh
mise run dev:web   # Vite+
mise run dev:go    # Go, proxying pages to Vite+
```

Go still answers remote functions in dev. Production serves the built frontend
and renders pages entirely inside the Go process.

`mise` is the default build entry point for compatibility, not a requirement of
skgo. Choose the style you want when generating the app:

```sh
skgo new --build-tool=mise hello      # mise.toml; Node and Vite+ are pinned for you
skgo new --build-tool=just hello      # Justfile; bring Node 24, pnpm 11, and just
skgo new --build-tool=scripts hello   # scripts/*.sh; bring Node 24 and pnpm 11
```

Each choice exposes the same build and two-process development workflow. The
generated README gives the exact commands for the selected style; only that
style's files are written.

## The model

| What you write | Where it lives | What happens |
| --- | --- | --- |
| Pages, layouts, components | `web/src/routes/**/*.svelte` | SvelteKit compiles and routes them normally |
| Queries, commands, forms, live and batch queries | `*.remote.go` beside their callers | skgo generates the Kit-facing `.remote.ts`, types, codecs, and Go registration |
| Server loads | `page.server.go` or `layout.server.go` beside Kit's generated `+page.server.ts` / `+layout.server.ts` stub | Go answers `__data.json` and render-time loads |
| HTTP endpoints | `server.go` beside a generated `+server.ts` stub | An ordinary `net/http` handler answers the route |
| Server composition | `server.go` in the app root | Go mounts loads, remotes, endpoints, SSR, static files, and hooks |

The generated TypeScript server bodies always throw. They exist so Kit can
compile its own client and manifests; a real response can only have come from
Go.

## Write a remote function

Put an ordinary Go function beside the route that uses it and mark the kind of
remote function Kit should expose:

```go
// web/src/routes/todos.remote.go
package routes

import (
	"context"
	"github.com/tylergannon/skgo"
)

type Todo struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

func todos(context.Context) ([]Todo, error) {
	return store.List(), nil
}

func addTodo(ctx context.Context, text string) (Todo, error) {
	todo := store.Add(text)
	return todo, skgo.RefreshRequestedNoArg(ctx, todos)
}

var (
	_ = skgo.Query(todos)
	_ = skgo.Command(addTodo)
)
```

Use it exactly as a SvelteKit developer expects:

```svelte
<script lang="ts">
	import { addTodo, todos } from './todos.remote';
</script>

{#each await todos() as todo}
	<p>{todo.text}</p>
{/each}

<button onclick={() => addTodo('ship it').updates(todos())}>Add</button>
```

Run the generated project's build gesture (`mise run build`, `just build`, or
`./scripts/build.sh`, according to `--build-tool`):

```sh
mise run build
```

`skgo generate` discovers the marked Go functions, projects their Go types to
TypeScript, writes strict codecs for the Kit wire format, and registers the Go
closures the server invokes. Generated files say `DO NOT EDIT`; source remains
the `.go` and `.svelte` files you authored.

The available markers mirror Kit: `skgo.Query`, `skgo.Command`, `skgo.Form`,
`skgo.LiveQuery`, and `skgo.BatchQuery`. Request state is available through
`skgo.EventFrom(ctx)`. Commands can refresh or reconnect typed Go functions;
see [`todos.remote.go`](example/web/src/routes/todos/todos.remote.go) for the
full pattern.

## Learn from the example

The front page of the [example app](example/web/src/routes/+page.svelte) links
to every capability it demonstrates and tells you what to look for. Good first
stops are:

| Route | Shows |
| --- | --- |
| `/todos` | queries, commands, forms, live updates, auth, and refresh |
| `/account` | nested Go server loads and layout reuse |
| `/api` | a `+server.ts` route implemented as `net/http` |
| `/stream` | deferred load values streamed into an SSR document |
| `/batch` | Kit's `query.batch` backed by one Go call |
| `/pricing` | a custom Go type transported with its TypeScript methods |
| `/plain` and `/spa` | Kit's `csr = false` and `ssr = false` branches |
| `/error/*` | expected errors, unexpected errors, boundaries, and redirects |

Run it from this repository:

```sh
just install
just build
just serve
```

Then open <http://127.0.0.1:8080>. `just --list` shows generation, tests, dev,
and the browser suite. A fresh checkout must be built once before Go can compile
the example because its binary intentionally embeds the frontend output.

## Deploy

The browser origin is part of a Kit build and skgo checks it on non-GET remote
calls. A generated project writes the origin once in its selected build file;
change `ORIGIN` there and rebuild the complete binary. The listen address and
public origin may differ behind a reverse proxy, so do not infer one from the
other.

The result is a normal Go executable. Copy it to the target, run it, and put
your usual TLS proxy or load balancer in front of it. Node is a build-time
dependency only.

## Reference

- [One-page overview](https://tylergannon.github.io/skgo/)
- [Go package documentation](https://pkg.go.dev/github.com/tylergannon/skgo)
- [Example server composition](example/server.go)
- [Agent skill](skills/skgo/SKILL.md)

MIT. See [LICENSE](LICENSE).
