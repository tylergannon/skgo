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

## Start a project

Install VitePlus, then let `skgo new` delegate the frontend to VitePlus and
Svelte's own `sv` creator:

```sh
skgo new --starter minimal myapp
# or: skgo new --starter examples myapp
```

Both starting points include Storybook and Vitest through their upstream `sv`
add-ons. `minimal` leaves SvelteKit's upstream minimal page in place;
`examples` adds a focused Go-backed query and command. The generated
instructions use `just dev`, `just storybook`, and `just test`. Production is a
standalone Go binary; Node is not needed while it serves requests.

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

Run `skgo generate` from the bindings package, with `--web` pointing to the
SvelteKit application.

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
calls. Configure the frontend build and Go server with the same public origin,
and rebuild the complete binary when it changes. The listen address and public
origin may differ behind a reverse proxy, so do not infer one from the other.

The result is a normal Go executable. Copy it to the target, run it, and put
your usual TLS proxy or load balancer in front of it. Node is a build-time
dependency only.

## Reference

- [One-page overview](https://tylergannon.github.io/skgo/)
- [Go package documentation](https://pkg.go.dev/github.com/tylergannon/skgo)
- [Example server composition](example/server.go)
- [Agent skill](skills/skgo/SKILL.md)

MIT. See [LICENSE](LICENSE).
