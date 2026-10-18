---
title: "Migrating from a Node SvelteKit app"
summary: "Keep the frontend, move each server file to Go beside it, and let the generator write the TypeScript."
---

Moving a SvelteKit application from `adapter-node` to SKGo leaves the frontend as it is and rewrites the server in Go. Components, universal loads, page options and client hooks move across unchanged. Every `+page.server.ts`, `+layout.server.ts`, `+server.ts` and `*.remote.ts` becomes a Go file in the same directory, and `skgo generate` writes the TypeScript module back.

## Before you start

SKGo uses Kit 3.1.0 exactly. If your application is on Kit 2, follow Kit's [migration guide](https://svelte.dev/docs/kit/migrating-to-sveltekit-3) first and get it working on Kit 3 with Node. Then list the JavaScript packages your server code uses, such as database clients, auth libraries and SDKs, and find their Go equivalents, because none of them will run on the server any more. Universal loads and components run in the embedded renderer in production, so check their imports against [UI libraries](integrations#UI-libraries), and read [Differences from SvelteKit](differences) for anything your application depends on.

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

`skgo new` configured `@skgo/sveltekit-adapter` in `web/vite.config.ts` and turned on remote functions. Copy your other Kit options, such as `csp`, `csrf` and `version`, from your old config; Kit 3 fixes `paths.origin` at build time, and the generated config reads it from `ORIGIN`. `output.bundleStrategy: 'inline'` and `router.resolution: 'server'` are refused. adapter-node's environment variables have no effect: the origin is compiled in by `just build` and the listen address is the binary's `-listen` flag. See [Adapter](adapter) and [Deploying](deploying).

## Server loads

A server load becomes a Go function registered with `skgo.Load`. Its returned struct's JSON tags become the page's `data` type:

```ts
/// file: src/routes/prerender/[slug]/+page.server.ts
export async function load({ params }) {
	return { slug: params.slug, receipt: `Go entry load: ${params.slug}` };
}
```

```go
/// file: src/routes/prerender/[slug]/page.server.go
package prerender

import (
	"github.com/tylergannon/skgo"
)

type PageData struct {
	Slug    string `json:"slug"`
	Receipt string `json:"receipt"`
}

func load(event PageRequestEvent) (PageData, error) {
	slug := event.Params.Slug()
	return PageData{Slug: slug, Receipt: "Go entry load: " + slug}, nil
}

var _ = skgo.Load(load)

// The catalog lives only in Go. Kit requests these raw URL values during analyse.
func entries() ([]EntryParams, error) {
	return []EntryParams{{Slug: "atlas"}, {Slug: "beacon"}}, nil
}

var _ = skgo.Entries(entries)
```

`PageRequestEvent` and its typed `Params` are generated into `skgo_gen.go` beside the file. `error()` becomes a returned `skgo.Errorf`, `redirect()` a returned `*skgo.Redirect`, `parent()` is `skgo.Parent[LayoutData](event.Context())`, and a streamed promise is a `skgo.Deferred` field. See [Loading data](load).

## Form actions

Each action becomes a Go function registered with `skgo.Action`, `skgo.ActionWithFailure` or `skgo.ActionNoData`, or `skgo.DefaultAction` for a page with one unnamed action. It reads the submission from `skgo.EventFrom(ctx).Request()` with `net/http`, and `fail()` becomes `skgo.Fail`:

```go
/// file: src/routes/actions/page.server.go
type ValidationFailure struct {
	Name       string `json:"name"`
	Email      string `json:"email"`
	Biography  string `json:"biography"`
	EmailError string `json:"emailError"`
}
```

```go
/// file: src/routes/actions/page.server.go
func save(ctx context.Context) (SaveResult, error) {
	id, err := workspace(ctx)
	if err != nil {
		return SaveResult{}, err
	}
	request := skgo.EventFrom(ctx).Request()
	if err := request.ParseForm(); err != nil {
		return SaveResult{}, skgo.Errorf(http.StatusBadRequest, "Invalid form")
	}
	profile := Profile{
		Name:      request.PostForm.Get("name"),
		Email:     request.PostForm.Get("email"),
		Biography: request.PostForm.Get("biography"),
		State:     "Active",
	}
	address, parseErr := mail.ParseAddress(profile.Email)
	if parseErr != nil || address.Address != profile.Email || !strings.Contains(strings.SplitAfter(profile.Email, "@")[1], ".") {
		return SaveResult{}, skgo.Fail(http.StatusUnprocessableEntity, ValidationFailure{
			Name: profile.Name, Email: profile.Email, Biography: profile.Biography,
			EmailError: "Enter a valid email address",
		})
	}
	profiles.Lock()
	profiles.byWorkspace[id] = profile
	profiles.Unlock()
	if err := skgo.EventFrom(ctx).SetHeader("x-skgo-action-demo", "profile-saved"); err != nil {
		return SaveResult{}, err
	}
	return SaveResult{Receipt: "Saved " + profile.Name, Price: businesslogic.Money{Cents: 750}}, nil
}
```

The action's name is the Go function's name, so `<form method="POST" action="?/save">` and `use:enhance` stay as they were, and `form` is typed from the result and failure structs. See [Form actions](form-actions).

## +server routes

A `+server.ts` becomes `server.go` with one `net/http` handler per method, registered with `skgo.GET`, `skgo.POST` and the others. The handler owns the status, headers and body, and reads locals from the request's context:

```go
/// file: src/routes/api/todos/server.go
// list answers `GET /api/todos` with the todos this visitor may see.
//
// Who the visitor is comes from the app's Handle hook, exactly as it does in a
// load or a remote function: the endpoint reads the session it stored rather
// than parsing the cookie again.
func list(w http.ResponseWriter, r *http.Request) {
	session := app.LocalsFrom(r.Context()).Session
	writeJSON(w, http.StatusOK, visitor.Store(r.Context()).Todos(session.User != ""))
}
```

```go
/// file: src/routes/api/todos/server.go
var (
	_ = skgo.GET(list)
	_ = skgo.POST(add)
	_ = skgo.QUERY(search)
)
```

See [+server](routing#server).

## Remote functions

Each export of a `.remote.ts` becomes a Go function in a `.remote.go` of the same name, declared with `skgo.Query`, `skgo.Command`, `skgo.Form`, `skgo.Prerender`, `skgo.BatchQuery` or `skgo.LiveQuery`:

```go
/// file: src/routes/todos/todos.remote.go
// getTodo looks one todo up by id.
func getTodo(ctx context.Context, id string) (businesslogic.Todo, error) {
	todo, ok := visitor.Store(ctx).Todo(id, signedIn(ctx))
	if !ok {
		return businesslogic.Todo{}, skgo.Errorf(404, "No todo with id %q", id)
	}
	return todo, nil
}
```

```go
/// file: src/routes/todos/todos.remote.go
var (
	_ = skgo.Query(getTodos)
	_ = skgo.Query(getTodo)
	_ = skgo.Command(addTodo)
	_ = skgo.Command(renameTodo)
	_ = skgo.Command(retitleTodo)
	_ = skgo.LiveQuery(watchCount)
)
```

The Standard Schema validator is replaced by the Go argument type: an argument that does not decode into it gets a 400, and your function validates the values. Components keep importing from `./todos.remote`. See [Remote functions](remote-functions).

## Hooks

- `handle` becomes `Handle` in `internal/serverhooks/handle.go`: `params.Handle(fn).Middleware()` for a before-only hook, `params.Sequence` to compose several. Write to `event.Locals` as you wrote to `event.locals`.
- `handleError` becomes a `HandleError` function set on the load, remote, endpoint and handle configurations in `server.go`.
- `handleFetch` becomes a `HandleFetch` on the fetch and renderer configurations.
- `init` becomes code in `cmd/main.go` that runs before the server listens.

See [Hooks](hooks) and [Errors](errors).

## Environment variables

Replace `$env/static/private`, `$env/dynamic/public` and the rest with one Go struct in `web/src/env.go` marked with `skgo.Environment`. `skgo generate` writes Kit's `src/env.ts` from it, so frontend imports of public variables keep working, and `skgo.LoadEnvironment` validates the values when the server starts. See [Environment variables](environment-variables).

## Cookies and sessions

`cookies.get`, `cookies.set` and `cookies.delete` become `Event.Cookie`, `SetCookie` and `DeleteCookie`, with options in `skgo.CookieOptions`; `Secure` follows the configured origin rather than the request. The session lookup moves into the Go `handle` hook, which stores the user in `Locals` for every load, action and remote function to read. See [Auth](auth) and [Cookies](cookies).
