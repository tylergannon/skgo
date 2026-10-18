---
title: Debugging
summary: Delve for the Go server, browser devtools for Svelte, and where rendering and generation failures are reported.
---

An SKGo application runs code in three places: Go handlers in the Go process, Svelte components in the browser, and SvelteKit's server renderer, which is Node under `skgo dev` and goja inside the binary.

## Go code

Loads, actions, remote functions, `server.go` routes and hooks are ordinary Go, so [Delve](https://github.com/go-delve/delve) and any editor built on it work. Build the binary and start it under the debugger:

```sh
just build
dlv exec ./bin/my-app
```

Tests run the same way with `dlv test` on their package; a test that serves requests through `NewHandler` exercises the production stack, rendering included.

Go compiles route packages from the generated copies under `internal/skgo/links/`, not from `web/src/routes/`, because Kit's directory names are not legal import paths. A breakpoint in `web/src/routes/blog/page.server.go` belongs on the same line of its copy; `internal/skgo/links.json` maps each copy to its route directory. The copies are refreshed by every `go generate ./...`, so regenerate before you build for debugging.

## Svelte code in the browser

Client code is unchanged SvelteKit, so browser devtools work as Kit's [debugging](https://svelte.dev/docs/kit/debugging) guide describes.

## Server rendering failures

When a render fails in production, the visitor gets Kit's error page or `error.html`, which says nothing about the cause. The cause goes to `SSROptions.OnError(routeID, err)`; the generated `server.go` leaves it nil, which logs. Set it to send render failures to your own logger or error tracker.

The production renderer's bundle is unminified and has no source map. A stack trace from goja points at lines of `ssr/bundle.js` in the build output, not at your `.svelte` files. Reproduce the failure under `skgo dev`, where Vite renders with source maps, or read the bundle around the reported line. Anything the renderer writes to `console` reaches the Go log, prefixed with the route being rendered.

An unexpected error from a Go load, action or the `handle` hook reaches your `HandleError` hook with the original error in `caught.Err`; [Errors](errors) lists where the hook does not run.

## Generator and analyzer errors

[`skgo check`](/docs/cli/check) runs every analyzer SKGo ships, `svelte-check`, lint and formatting without editing files. Each analyzer finding has a code, `SKGO001` through `SKGO008`, and [`skgo advice`](/docs/cli/advice) prints the rule, its consequence, a repair and an example for each.

If generation fails, `go generate ./...` prints the generator's error with the declaration it refers to. The usual causes are a remote function whose argument or result is a pointer, a map, an interface or a variadic parameter, a Kit matcher with no Go matcher of the same name in `src/params.go`, and an authored file where a generated one belongs.

## Development server errors

`skgo dev` prints Go build errors and runtime output in its terminal. When a Go edit fails to build, the error is also sent to the browser as Vite's error overlay, and the overlay closes by itself once a later edit builds; the running application keeps serving meanwhile. If a change seems to have no effect, read the terminal; [Development server](development-server) lists the known failure modes.

To debug server-side Svelte code in development, use Node's inspector. The Vite child inherits your environment, so `NODE_OPTIONS=--inspect=0 go tool skgo dev` starts it with the inspector on a free port, and attaching to the URL it prints works as it does for any Node process.
