---
title: Testing
description: Go tests against the production handler, generated form clients, component tests with Vitest, and browser tests against the binary.
division: skgo
section: Best practices
order: 5030
---

An SKGo application has three layers to test. Components are tested as in any Svelte project. The server is Go, and you test it with `go test` against the same `http.Handler` the binary serves. Browser tests run against the built binary, because that is what you deploy.

## Testing the Go server

`NewHandler` in `server.go` is the production stack: hooks, loads, actions, remote functions, `server.go` routes and the renderer, over the embedded build. A test builds it and sends requests without opening a socket. The example's helpers do both, over the embedded build:

```go example/testing_handler_test.go
```

`web.Build` embeds `web/build`, so build the frontend before `go test`: `just build` does it, or `go generate ./...` followed by `vp build` in `web`. On a fresh checkout the directory holds only a placeholder, and `NewHandler` fails because there is no manifest to read. A test through this handler renders real pages, so it is also the cheapest place to catch a component that breaks in the embedded renderer.

Requests a browser would make need the same shape in a test:

- a page's data is a `GET` of `<path>/__data.json`
- a form action is a `POST` to `<path>?/<name>` with a form content type and an `Origin` header equal to the origin the handler was built with; without it SKGo refuses the submission as cross-site, as Kit does
- cookies go on the request with `req.AddCookie` and come back in `rec.Result().Cookies()`

Go cannot spell most route directory paths, and the application's `go.mod` ignores `./web/src/routes`, so a test cannot import a route package by its path. `skgo generate` links each route directory that holds Go files, tests included, to its link package under `internal/skgo/_links/`; `internal/skgo/links.json` maps each link package to its route. Import a route's types through the link package.

`go test ./...` skips `_links/`, as it skips every directory that starts with `_`, so it does not run tests in route directories. Name the link packages to run them:

```bash
go test $(go list -deps ./internal/skgo | grep /_links/)
```

`skgo check` names them itself, so `go vet` and Staticcheck cover route code.

## Testing remote functions

A remote function is a Go function, so when it needs nothing from the request, call it. One that reads cookies, locals or `skgo.EventFrom(ctx)` needs a request, so call it through the handler the way Kit's client does. The tests below call the remote functions of this route:

```go example/web/src/routes/samples/testing/testing.remote.go
```

The id comes from the generated registry, where `generated` is your bindings package `internal/skgo`. A query is `GET /_app/remote/<id>`, with `?payload=` carrying the argument as a devalue document, base64url-encoded without padding. A command is `POST /_app/remote/<id>` with `Content-Type: application/json`, the matching `Origin`, and the body `{"payload": "<base64url devalue>", "refreshes": []}`. The answer is Kit's remote envelope, whose `data` is the devalue document the browser would parse. The example finds the id and sends both shapes:

```go example/testing_remote_test.go
```

## Submitting forms from Go

For a remote `form` whose input is a flat struct of scalar fields, the generator also writes a typed Go client in `internal/skgo/client`, with one method per form named after the Go function. It submits what Kit's enhanced form submits, against a running server:

```go example/testing_client_test.go
```

`signup` is the route's link package, and `Signup` the form's argument type; the client `c` is built over a test server. A validation failure comes back as `*skgo.Invalid` with one `skgo.Issue{Field, Message}` per problem. The client sends an `Origin` header derived from `BaseURL`, so build the handler with the test server's own address or with an empty origin, which turns the check off. Forms outside the flat-scalar shape are tested with a form `POST` through the handler.

## Component tests

Component tests are unchanged from Svelte. `skgo new` installs Vitest and Storybook; `just test` runs the Vitest suite (`vp run test` in `web`) and `just storybook` opens Storybook. Svelte's [testing guide](https://svelte.dev/docs/svelte/testing) covers writing them.

A component test runs no Go. When a component calls a remote function, mock the generated `.remote.ts` module in the test, or move the behaviour up to a page you test through the handler or a browser.

## Browser tests

Run Playwright against the built binary. Kit's Playwright setup starts Vite's preview server; point `web/playwright.config.ts` at `just serve` instead, which builds and runs `bin/<app>` on the project's origin:

```ts example/web/src/routes/samples/testing/playwright.config.ts
```

Because the server owns forms and redirects, run the flows that matter once more with Playwright's `javaScriptEnabled: false`, which is how a visitor without JavaScript sees them. Run them under `skgo dev` as well: development renders in Node and production in the embedded engine, and the [development server](/docs/skgo/development-server) page lists where they differ.

## Checking the project

[`skgo check`](/docs/cli/check) runs every analyzer SKGo ships, `svelte-check`, lint and formatting without editing a file. Run it in CI beside `go test ./...` and `just test`.
