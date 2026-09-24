# skgo

**A Go backend for SvelteKit.**

skgo lets you write the server side of a SvelteKit app in Go. Remote functions,
server loads and API routes are Go code that sits next to the Svelte files that
use them. skgo generates TypeScript types and bindings from that code, so you
write a function in Go and await it in Svelte, the same way you would call any
SvelteKit remote function. Pages are rendered by SvelteKit's own renderer
running inside the Go process, and the app deploys as a single Go binary. Node
is only needed to build it.

> **Status:** pre-1.0. skgo tracks the SvelteKit 3 prerelease pinned in this
> repository. All server code is written in Go: TypeScript remote functions,
> loads and API routes are not supported. The [example app](example/) shows what
> works today.

## Example

A query, written in Go:

```go
// web/src/routes/todos/todos.remote.go
package todos

import (
	"context"

	"github.com/tylergannon/skgo"
)

type Todo struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

func todos(ctx context.Context) ([]Todo, error) {
	return store.List(ctx)
}

var _ = skgo.Query(todos)
```

Called from a Svelte component:

```svelte
<!-- web/src/routes/todos/+page.svelte -->
<script lang="ts">
	import { todos } from './todos.remote';
</script>

{#each await todos() as todo}
	<p>{todo.text}</p>
{/each}
```

`skgo generate` writes `todos.remote.ts` next to the Go file. The query runs in
Go, and `todo` has the type of the `Todo` struct.

## What you write and what skgo generates

| You write, in Go | skgo generates | SvelteKit sees |
| --- | --- | --- |
| `todos.remote.go`, using `skgo.Query`, `Command`, `Form`, `LiveQuery` or `BatchQuery` | `todos.remote.ts`, its TypeScript types, the wire codecs and the Go registration | A remote function, imported from `./todos.remote` |
| `page.server.go` or `layout.server.go` | A typed `+page.server.ts` or `+layout.server.ts` | A server load. `data` is typed from the Go result. |
| `server.go`, a `net/http` handler | A `+server.ts` | An API route |

The generated TypeScript function bodies throw an error. They exist so SvelteKit
can build its client and manifests, which means every response comes from Go.

## For Go developers

- The build embeds the frontend, its assets and the server-rendering bundle in
  one binary. Node runs at build time only.
- A remote function is an ordinary Go function that takes a `context.Context`
  and returns `(T, error)`. A one-line marker such as `skgo.Query(todos)`
  registers it.
- API routes are `http.Handler`s. You assemble loads, remote functions, API
  routes and page rendering into your own `net/http` server (see
  [`example/server.go`](example/server.go)).
- The request and its cookies are available from `skgo.EventFrom(ctx)`.
- The JSON shape of your Go types becomes the TypeScript types. After you change
  a type, regenerate, and the TypeScript checker reports every component that
  still uses the old shape.
- There is no cgo. The JavaScript engine, [goja](https://github.com/dop251/goja),
  is written in Go.
- Generated files are ordinary Go and TypeScript, marked `DO NOT EDIT`.

## For SvelteKit developers

- Components are ordinary Svelte 5.
- Routing, layouts, error pages, navigation and the remote-function client are
  SvelteKit's own code, not a reimplementation.
- Queries, commands, forms, live queries and batch queries are imported and
  called as in any SvelteKit app. Only the implementation is Go.
- A Go load's result type becomes the page's `data` type.
- Pages are rendered on the server. Streamed promises, `ssr = false` and
  `csr = false` work.
- Remote forms still work when JavaScript is disabled.
- In development, `just dev` runs `vp dev` behind the Go server. The Go server
  renders pages and forwards modules and hot reloading.
- `skgo new` creates the project with `sv create`, then adds Vitest and
  Storybook.

## Not supported

- TypeScript server code. Remote functions, loads and API routes must be
  written in Go.
- Classic form actions (`actions` in `+page.server.ts`). Use a remote `form`
  instead.
- Prerendering a page whose route has a Go server load.
- Pointer, map and interface types in remote function signatures. Use value
  types, named structs or `polytype.Nullable`.
- `output.bundleStrategy: 'inline'` and `router.resolution: 'server'`.
- `sv` library templates, and add-ons that write JavaScript server code.
  `skgo new` refuses them.

## Start a project

You need Go, [VitePlus](https://viteplus.dev) (`vp`) and
[just](https://just.systems).

```sh
go install github.com/tylergannon/skgo/cmd/skgo@latest
skgo new myapp
cd myapp
just dev
```

`skgo new` runs `sv create` through VitePlus and asks `sv`'s usual questions.
Anything after `--` is passed to `sv create`.

## Build and deploy

```sh
just build      # go generate, vp build, go build → bin/myapp
./bin/myapp
```

The build compiles in the app's public origin, and skgo checks it on non-GET
remote calls. If the origin changes, rebuild. Behind a reverse proxy the listen
address can differ from the origin. Put your usual TLS proxy or load balancer in
front.

## Example app

| Route | Shows |
| --- | --- |
| `/todos` | Queries, commands, forms, live updates and refresh |
| `/account` | Nested Go server loads |
| `/api` | An API route answered by a `net/http` handler |
| `/stream` | Load values streamed into the page |
| `/plain`, `/spa` | `csr = false` and `ssr = false` |

From this repository:

```sh
just install && just build && just serve    # http://127.0.0.1:8080
```

## Links

- [Overview](https://tylergannon.github.io/skgo/)
- [Go package documentation](https://pkg.go.dev/github.com/tylergannon/skgo)
- [Agent skill](skills/skgo/SKILL.md)

skgo is pronounced "skay-go". MIT licensed; see [LICENSE](LICENSE).
