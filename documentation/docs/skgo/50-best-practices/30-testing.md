---
title: Testing
summary: Go tests against the production handler, generated form clients, component tests with Vitest, and browser tests against the binary.
proof:
  - TestTheEmbeddedBuildProducesAWorkingServer
  - TestTheServerAnswersARealRemoteCall
  - TestGeneratedFormClientOverHTTPAndUDS
---

An SKGo application has three layers to test. Components are tested as in any Svelte project. The server is Go, and you test it with `go test` against the same `http.Handler` the binary serves. Browser tests run against the built binary, because that is what you deploy.

## Testing the Go server

`NewHandler` in `server.go` is the production stack: hooks, loads, actions, remote functions, `server.go` routes and the renderer, over the embedded build. A test builds it and sends requests without opening a socket. The example's helpers do both, over the embedded build:

```go
/// file: server_test.go
/// include: example/server_test.go#prod-handler
```

`web.Build` embeds `web/build`, so build the frontend before `go test`: `just build` does it, or `go generate ./...` followed by `vp build` in `web`. On a fresh checkout the directory holds only a placeholder, and `NewHandler` fails because there is no manifest to read. A test through this handler renders real pages, so it is also the cheapest place to catch a component that breaks in the embedded renderer.

Requests a browser would make need the same shape in a test:

- a page's data is a `GET` of `<path>/__data.json`
- a form action is a `POST` to `<path>?/<name>` with a form content type and an `Origin` header equal to the origin the handler was built with; without it SKGo refuses the submission as cross-site, as Kit does
- cookies go on the request with `req.AddCookie` and come back in `rec.Result().Cookies()`

Route directories under `web/src/routes` sit behind a module boundary, so a test cannot import a route package by its path. `skgo generate` copies each route directory's Go files, tests included, into its link package under `internal/skgo/links/`, and `go test ./...` compiles them there; `internal/skgo/links.json` maps each copy to its route. Import a route's types through the link package.

## Testing remote functions

A remote function is a Go function, so when it needs nothing from the request, call it. One that reads cookies, locals or `skgo.EventFrom(ctx)` needs a request, so call it through the handler the way Kit's client does.

The id comes from the generated registry, where `generated` is your bindings package `internal/skgo`:

```go
/// file: server_test.go
/// include: example/server_test.go#remote-id
```

A query is `GET /_app/remote/<id>`, with `?payload=` carrying the argument as a devalue document, base64url-encoded without padding. A command is `POST /_app/remote/<id>` with `Content-Type: application/json`, the matching `Origin`, and the body `{"payload": "<base64url devalue>", "refreshes": []}`. The answer is Kit's remote envelope, whose `data` is the devalue document the browser would parse. The example sends both shapes:

```go
/// file: wire_test.go
/// include: example/wire_test.go#call-query
```

```go
/// file: wire_test.go
/// include: example/wire_test.go#call-command
```

## Submitting forms from Go

For a remote `form` whose input is a flat struct of scalar fields, the generator also writes a typed Go client in `internal/skgo/client`, with one method per form named after the Go function. It submits what Kit's enhanced form submits, against a running server:

```go
/// file: client_test.go
/// include: example/client_test.go#invalid
```

`optional` is the route's link package, and `Input` the form's argument type; the client `c` was built over a test server. A validation failure comes back as `*skgo.Invalid` with one `skgo.Issue{Field, Message}` per problem. The client sends an `Origin` header derived from `BaseURL`, so build the handler with the test server's own address or with an empty origin, which turns the check off. Forms outside the flat-scalar shape are tested with a form `POST` through the handler.

## Component tests

Component tests are unchanged from Svelte. `skgo new` installs Vitest and Storybook; `just test` runs the Vitest suite (`vp run test` in `web`) and `just storybook` opens Storybook. Svelte's [testing guide](https://svelte.dev/docs/svelte/testing) covers writing them.

A component test runs no Go. When a component calls a remote function, mock the generated `.remote.ts` module in the test, or move the behaviour up to a page you test through the handler or a browser.

## Browser tests

Run Playwright against the built binary. Kit's Playwright setup starts Vite's preview server; point it at `just serve`, which builds and runs `bin/<app>` on the project's origin:

```ts
/// file: web/playwright.config.ts
import { defineConfig } from '@playwright/test';

export default defineConfig({
	webServer: {
		command: 'cd .. && just serve',
		url: 'http://127.0.0.1:8080',
		reuseExistingServer: !process.env.CI
	},
	use: { baseURL: 'http://127.0.0.1:8080' }
});
```

Because the server owns forms and redirects, run the flows that matter once more with Playwright's `javaScriptEnabled: false`, which is how a visitor without JavaScript sees them. Run them under `skgo dev` as well: development renders in Node and production in the embedded engine, and the [development server](development-server) page lists where they differ.

## Checking the project

[`skgo check`](/docs/cli/check) runs every analyzer SKGo ships, `svelte-check`, lint and formatting without editing a file. Run it in CI beside `go test ./...` and `just test`.
