---
title: Migrating from a Node SvelteKit app
description: Keep the frontend, move each server file to Go beside it, and let the generator write the TypeScript.
division: skgo
section: Appendix
order: 6050
---

Moving a SvelteKit application from `adapter-node` to SKGo leaves the frontend as it is and rewrites the server in Go. Components, universal loads, page options and client hooks move across unchanged. Every `+page.server.ts`, `+layout.server.ts`, `+server.ts` and `*.remote.ts` becomes a Go file in the same directory, and `skgo generate` writes the TypeScript module back.

## Before you start

SKGo uses one exact Kit 3 release. If your application is on Kit 2, follow Kit's [migration guide](https://svelte.dev/docs/kit/migrating-to-sveltekit-3) first and get it working on Kit 3 with Node. Then list the JavaScript packages your server code uses, such as database clients, auth libraries and SDKs, and find their Go equivalents, because none of them will run on the server any more. Universal loads and components run in the embedded renderer in production, so check their imports against [UI libraries](/docs/skgo/integrations#UI-libraries), and read [Differences from SvelteKit](/docs/skgo/differences) for anything your application depends on.

## Project layout

Create a new project and move your frontend into it:

```sh
skgo new my-app --module example.com/myapp
```

Copy your Kit project's `src/` and `static/` over the generated `web/src/` and `web/static/`, keeping the generated files unless your application has its own version. Leave out `src/hooks.server.ts`: generation refuses an authored one, and its logic moves to `internal/serverhooks/handle.go`. Add your frontend dependencies to `web/package.json` and run `vp install` in `web`; leave out server-only JavaScript dependencies.

| Node app | SKGo app |
| --- | --- |
| `src/`, `static/` | `web/src/`, `web/static/` |
| `+page.server.ts`, `+layout.server.ts` | `page.server.go`, `layout.server.go` in the same directory |
| `+server.ts` | `server.go` in the same directory |
| `x.remote.ts` | `x.remote.go` in the same directory |
| `src/hooks.server.ts` | `internal/serverhooks/handle.go`, plus the hooks `server.go` configures |
| `src/lib/server/` | Go packages, for example under `internal/` |
| `App.Locals` in `src/app.d.ts` | `Locals` in `internal/app/locals.go` |
| `$env/*` | `web/src/env.go`, which generates `src/env.ts` |
| `params.ts` matchers | `params.ts` plus Go matchers in `web/src/params.go` |
| `transport` in `src/hooks.ts` | `web/src/hooks.ts` plus `skgo.Transported` in `web/src/hooks.go` |
| `node build` | `just build`, then `./bin/my-app` |

Write each Go replacement before you run `skgo generate`; once a Go file is beside it, generation overwrites the TypeScript file with a generated module. A server module Kit compiles that SKGo did not generate fails the production build, so the build tells you what is left.

## Adapter

`skgo new` configured `@skgo/sveltekit-adapter` in `web/vite.config.ts` and turned on remote functions. Copy your other Kit options, such as `csp`, `csrf` and `version`, from your old config; Kit 3 fixes `paths.origin` at build time, and the generated config does not set it; see [Building your app](/docs/skgo/building-your-app#The-origin-is-compiled-in). `output.bundleStrategy: 'inline'` and `router.resolution: 'server'` are refused. adapter-node's environment variables have no effect: the origin is compiled in by `just build` and the listen address is the binary's `-listen` flag. See [Adapter](/docs/skgo/adapter) and [Deploying](/docs/skgo/deploying).

## Server loads

A server load becomes a Go function registered with `skgo.Load`. Its returned struct's JSON tags become the page's `data` type. The Node app's load:

```ts example/web/src/routes/samples/migrating/node-load.ts
```

becomes, in Go:

```go example/web/src/routes/prerender/[slug]/page.server.go
```

`PageRequestEvent` and its typed `Params` are generated into `skgo_gen.go` beside the file. `error()` becomes a returned `skgo.Errorf`, `redirect()` a returned `*skgo.Redirect`, `parent()` is `skgo.Parent[LayoutData](event.Context())`, and a streamed promise is a `skgo.Deferred` field. See [Loading data](/docs/skgo/load).

## Form actions

Each action becomes a Go function registered with `skgo.Action`, `skgo.ActionWithFailure` or `skgo.ActionNoData`, or `skgo.DefaultAction` for a page with one unnamed action. It reads the submission from `skgo.EventFrom(ctx).Request()` with `net/http`, and `fail()` becomes `skgo.Fail`:

```go example/web/src/routes/samples/migrating/page.server.go
```

The action's name is the Go function's name, so `<form method="POST" action="?/save">` and `use:enhance` stay as they were, and `form` is typed from the result and failure structs:

```svelte example/web/src/routes/samples/migrating/+page.svelte
```

See [Form actions](/docs/skgo/form-actions).

## +server routes

A `+server.ts` becomes `server.go` with one `net/http` handler per method, registered with `skgo.GET`, `skgo.POST` and the others. The handler owns the status, headers and body, and reads locals from the request's context:

```go example/web/src/routes/api/todos/server.go
```

See [+server](/docs/skgo/routing#server).

## Remote functions

Each export of a `.remote.ts` becomes a Go function in a `.remote.go` of the same name, declared with `skgo.Query`, `skgo.Command`, `skgo.Form`, `skgo.Prerender`, `skgo.BatchQuery` or `skgo.LiveQuery`:

```go example/web/src/routes/samples/migrating/greeting.remote.go
```

The Standard Schema validator is replaced by the Go argument type: an argument that does not decode into it gets a 400, and your function validates the values. Components keep importing from `./greeting.remote`, as the page above does. See [Remote functions](/docs/skgo/remote-functions).

## Hooks

- `handle` becomes `Handle` in `internal/serverhooks/handle.go`: `params.Handle(fn).Middleware()` for a before-only hook, `params.Sequence` to compose several. Write to `event.Locals` as you wrote to `event.locals`.
- `handleError` becomes a `HandleError` function set on the load, remote, endpoint and handle configurations in `server.go`.
- `handleFetch` becomes a `HandleFetch` on the fetch and renderer configurations.
- `init` becomes code in `cmd/main.go` that runs before the server listens.

See [Hooks](/docs/skgo/hooks) and [Errors](/docs/skgo/errors).

## Environment variables

Replace `$env/static/private`, `$env/dynamic/public` and the rest with one Go struct in `web/src/env.go` marked with `skgo.Environment`. `skgo generate` writes Kit's `src/env.ts` from it, so frontend imports of public variables keep working, and `skgo.LoadEnvironment` validates the values when the server starts. See [Environment variables](/docs/skgo/environment-variables).

## Cookies and sessions

`cookies.get`, `cookies.set` and `cookies.delete` become `Event.Cookie`, `SetCookie` and `DeleteCookie`, with options in `skgo.CookieOptions`; `Secure` follows the configured origin rather than the request. The session lookup moves into the Go `handle` hook, which stores the user in `Locals` for every load, action and remote function to read. See [Auth](/docs/skgo/auth) and [Cookies](/docs/skgo/cookies).
