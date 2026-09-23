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
skgo new myapp
# or, with no questions asked:
skgo new myapp -- --template demo --types jsdoc --add tailwindcss=plugins:none
```

In a terminal the questions are `sv`'s own: template, type checking and
add-ons. Everything after `--` is handed to `sv create` as written, and
without a terminal whatever is left open is the minimal TypeScript application.
`sv`'s minimal template is left as it is. Its demo template is a JavaScript
server application, so choosing it gives the skgo example instead: a focused
Go-backed query and command. The library and add-on templates are packages,
not applications, and are refused, as is an add-on selection that writes a
JavaScript server. pnpm stays the package manager, through VitePlus.

`skgo new` then resolves independent, exact compatible versions of the native
`@skgo/sv` add-on and the runtime-only `@skgo/sveltekit-adapter`, and has `sv`
add them. Every project gets Vitest through its upstream `sv` add-on and
Storybook through the upstream `create-storybook` installer, unless that was
already chosen. The generated instructions use `just dev`, `just storybook`, and
`just test`. Production is a standalone Go binary; Node is not needed while it
serves requests.

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

For Forms with flat scalar fields (including `polytype.Optional[T]` scalar
fields) and results made from scalars, generation also writes an importable Go package at
`<bindings>/client`. It exposes a typed method per supported Form. For example,
the example app's optional Form can be submitted over HTTP:

```go
forms := client.Client{FormClient: skgo.FormClient{BaseURL: "http://127.0.0.1:8080"}}
result, err := forms.Submit(ctx, optional.Input{Name: "Ada"})
```

Set `FormClient.HTTPClient` to an `*http.Client` with a Unix socket
`Transport.DialContext` to use UDS; the `BaseURL` may then be `http://skgo`:

```go
transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
}}
forms := client.Client{FormClient: skgo.FormClient{
	BaseURL: "http://skgo", HTTPClient: &http.Client{Transport: transport},
}}
```

The call returns the declared result type, `*skgo.Invalid` for field issues,
`*skgo.HTTPError` for server error envelopes, or the underlying transport
error. A caller's context cancels the request. Each submission sends one POST:
redirects, including 307 and 308, are returned as errors and never replayed.
Forms with files or nested data continue to generate their existing browser
and server bindings but are outside this Go client support.

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
