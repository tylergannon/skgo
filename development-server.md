---
title: Development server
description: How skgo dev runs native Vite and Kit rendering alongside a hot-updated Go application.
division: skgo
section: Advanced
order: 4070
---

In development, SKGo does not run the production renderer. `go tool skgo dev` starts Vite with SvelteKit in Node, which renders pages, serves modules and drives HMR as in any Kit project. Your Go application runs next to it and answers everything that is Go: loads, form actions, `+server` routes, remote functions and the `handle` hook. Production is still one Go binary with goja rendering and no Node. Native Kit rendering is used only in development.

Run it from the application root:

```sh
go tool skgo dev
```

Open the address it prints. Flags are listed on the [skgo dev](/docs/cli/dev) command page.

## Composition

`skgo dev` listens on `127.0.0.1:8080`, or on the next free port when that one is taken, unless you pass `--strict-port`. Every response it serves carries an `X-Skgo-Mode: dev` header.

One listener fronts two processes. A remote function call, under `/_app/remote/`, goes straight to the Go application. Everything else goes to Vite, where Kit renders the page, answers `__data.json`, and calls back into Go for loads, actions, `+server` routes and the `handle` hook.

The Go application serves through the same `newHandler` as production. The only difference is the `*skgo.NativeDevelopment` value it receives: its `NewSSR` and `NewStaticHandler` methods replace the goja renderer and the static handler with calls into Kit, and a nil receiver builds the production ones. Hooks, locals, cookies, actions and remote functions therefore run the same Go code in both modes.

## Hot updates

What happens after an edit depends on the file:

- **Svelte, TypeScript and CSS** go through Vite HMR, as in any Kit project.
- **A compatible Go edit** is installed into the running application without a restart. The body of a top-level function in any package of your module hot-installs, as long as the function is not a method, not generic, not `init` and does not start a goroutine with `go`. Generated files are skipped. Kit's client then refreshes its queries and loads, so the page shows the new result.
- **A structural Go change** makes `skgo dev` build and start a new development host, then switch traffic to it. In-memory Go state does not survive the switch.
- **A compatible edit the native loader rejects** with an error, such as a relocation failure, takes the structural path. The running application keeps answering with the previous code until the new host takes over, and later edits, including hot installs, apply to the new host.

`skgo dev` reports why an edit needed a complete build. The reasons are a changed declaration, initialization, import or build selection; a `go` statement in the edited function; a generic function; a call from a function the hot loader could not instrument; and a failed native installation, which the terminal shows as `rebuilding revision N after install admission` followed by the loader's error.

If the development host is restarting when a request arrives, `skgo dev` holds the request for up to 30 seconds. After that it answers `503 Development application restarting` with `Retry-After: 1`.

## Errors in development

A Go build error appears in the browser through Vite's error overlay, with the file and line you wrote. The running application keeps serving until the build succeeds, and the next successful build closes the overlay and refreshes the page's data. Svelte compile errors use Vite's overlay as usual.

## Requirements

`skgo dev` builds with the exact Go release the application pins, on `darwin/arm64` or `linux/amd64`, with `CGO_ENABLED=0` and no `GOEXPERIMENT`, and reports a failing check at startup. It also needs a Node release in the range the [prerequisites](/docs/skgo/creating-a-project#Prerequisites) give, which it does not check itself. Production builds have none of these limits.

## The development handler

`skgo dev` builds your application from exactly one function, `NewDevelopmentHandler`, with the signature this composition shows:

```go ExampleNativeDevelopment_developmentHandler
```

Applications created by `skgo new` have it in `server.go`; [Server composition](/docs/skgo/server-composition) shows the file.

## Differences from production

- **Rendering.** Development renders in Node. Production renders in goja, inside the Go process. Module-level state in `.svelte` and `.ts` files behaves like one Node process in development, and like one copy per renderer runtime in production.
- **Cookies from subfetches.** A `Set-Cookie` on a response to one of Kit's server-render subrequests is visible to your Go code, but in development it is not relayed into the page response. Test sign-in and other cookie flows in both modes.
- **Tracing.** Kit's framework spans come from Node in development, and Go annotations are relayed to them with a size budget. See [Observability](/docs/skgo/observability#In-development).

## Known limits

- Browser coverage of `skgo dev` is one check: Kit's client adopts a server-rendered page and sees a Go edit arrive. Preservation of browser state such as unsent form drafts across edits, and full platform qualification, are not established. Check critical flows, including authentication, in both modes.
