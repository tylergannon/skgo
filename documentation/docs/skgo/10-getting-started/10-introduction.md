---
title: Introduction
summary: What SKGo is, what it keeps from SvelteKit, and what it replaces with Go.
proof:
  - example/e2e/features/app.feature
---

## Before we begin

> [!NOTE] These docs assume you know SvelteKit. If you don't, start with the [SvelteKit tutorial](https://svelte.dev/tutorial/kit) and the [SvelteKit docs](https://svelte.dev/docs/kit). SKGo is pre-1.0. This release, %skgo%, is built against SvelteKit %kit%, Svelte %svelte% and Go %go%; [Toolchain](toolchain) lists every pinned version.

## What is SKGo?

SKGo is a framework for building Go applications whose user interface is a SvelteKit app. Pages, layouts, routing, navigation and client state are SvelteKit's. The server code is Go, and the application ships as one Go binary that needs no Node at request time. To see what you can build, read [Project types](project-types).

## What is SvelteKit?

SvelteKit is the application framework for Svelte. It provides routing, server rendering, data loading, forms and a build pipeline on top of Vite; its [introduction](https://svelte.dev/docs/kit/introduction#What-is-SvelteKit) covers the rest. SKGo runs SvelteKit's own router, client and renderer rather than reimplementing them.

## SKGo vs SvelteKit

In SvelteKit, server code is JavaScript, run by Node or a serverless platform through an adapter.

SKGo replaces that server with Go. You write [server loads](load), [form actions](form-actions), [remote functions](remote-functions), [`+server` routes](routing#server) and the [`handle` hook](https://svelte.dev/docs/kit/hooks) as Go files beside your Svelte files. A query looks like this:

```go
/// file: src/routes/todos/todos.remote.go
/// include: example/web/src/routes/todos/todos.remote.go#query
```

```go
/// file: src/routes/todos/todos.remote.go
/// include: example/web/src/routes/todos/todos.remote.go#markers
```

The function is ordinary Go. The `var` block at the end of the file declares which functions SvelteKit may call and as what: a `query`, a `command`, a `form`, a `query.live` or a `query.batch`. `skgo generate` reads those declarations and writes `todos.remote.ts` beside the Go file, with TypeScript types projected from the Go structs, so a component imports the query the way it imports any remote function:

```svelte
<!--- file: src/routes/todos/TodoList.svelte --->
<script lang="ts">
	import { getTodos } from './todos.remote';
</script>

<ul>
	{#each await getTodos() as todo (todo.id)}
		<li>{todo.text}</li>
	{/each}
</ul>
```

The generated TypeScript bodies throw; they exist so SvelteKit can build its client and manifests, and the Go function is the only thing that ever answers. The same pattern covers the other server files:

| You write, in Go                                                   | `skgo generate` writes                                 | SvelteKit sees                               |
| ------------------------------------------------------------------ | ------------------------------------------------------ | -------------------------------------------- |
| `*.remote.go` with `skgo.Query`, `Command`, `Form`, `LiveQuery` or `BatchQuery` | `*.remote.ts`, its types, the wire codecs and the Go registration | A remote function                            |
| `page.server.go` or `layout.server.go` with `skgo.Load`           | A typed `+page.server.ts` or `+layout.server.ts`       | A server load; `data` is typed from the Go result |
| `server.go` with `skgo.GET`, `POST` and the other methods          | A `+server.ts`                                         | An API route                                 |

[`skgo dev`](/docs/cli/dev) runs Vite for the frontend and your Go application beside it, updating the Go side in place for compatible edits. `just build` produces one Go binary that embeds the frontend build and renders pages with SvelteKit's own renderer running inside the process. Differences lists what changes for a SvelteKit developer; [Go and web standards](go-standards) shows how Kit's web APIs map onto Go.

## Targets

The web target is released: the binary above is what you deploy. Generated Swift clients for your Go queries and commands are also released. Hosting the application natively on macOS and iOS, with either bundled or hosted Go, including lifecycle, WebView coordination and distribution, is not finished. Native targets describes what exists today, and [/framework](/framework) tracks each target's evidence.
