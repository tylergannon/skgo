---
title: Project types
summary: Which kinds of application SKGo builds today, and which it does not target.
proof:
  - TestAPrerenderedPageCanHaveAGoLayoutLoadInItsBranch
  - example/e2e/features/actions-options.feature
---

SvelteKit's [project types](https://svelte.dev/docs/kit/project-types) depend on its adapters. SKGo has one adapter, `@skgo/sveltekit-adapter`, and one output: a Go binary that serves your app. Most of Kit's project types are page options or deployment choices around that binary. The [project structure](project-structure) and routing are the same whichever you choose.

## Default rendering

Every page is rendered on the server by the Go binary and then hydrated in the browser, as in [Kit](https://svelte.dev/docs/kit/project-types#Default-rendering). Your Go loads run first; their results go to SvelteKit's renderer, which runs inside the process on [goja](https://github.com/dop251/goja), a JavaScript engine written in Go. Subsequent navigations are client-side, and Kit's client asks Go for each page's data.

## Static site generation

Pages with `prerender = true` are rendered at build time, including pages whose branch has a Go server load, and the binary serves them as files. See [Prerendering](https://svelte.dev/docs/kit/page-options#prerender).

## Single-page app

A route with `ssr = false` is still served by the Go binary: Go answers with Kit's shell document, which boots the client, and your Go loads, remote functions and endpoints still serve the data. Set it in the root layout to make the whole app client-rendered. See [Page options](https://svelte.dev/docs/kit/page-options).

## Multi-page app

`csr = false` sends no JavaScript for that page, as in [Kit](https://svelte.dev/docs/kit/page-options#csr). Form actions and remote `form` functions keep working with native form submissions, because Go answers the POST.

## Your own server

This is the normal case. `NewHandler` in the generated `server.go` builds the app's `http.Handler`, which you can mount in your own `net/http` server, wrap with middleware, or serve beside other routes. See Server composition.

## Container

Build the binary in a Docker image and run it anywhere containers run. Deploying walks through it.

## Separate backend

Your Go code is the backend, so there is usually no separate API server. If one exists, call it from Go loads and remote functions with any Go HTTP client, or with `Event.Fetch` when you want Kit's `event.fetch` behaviour for the app's own routes. See [Go and web standards](go-standards#Fetch).

## Serverless app

Not supported. SKGo is a long-running server with a pool of warm JavaScript runtimes, which does not fit per-request function platforms. Vercel, Netlify and Cloudflare deployments are listed on [/status](/status) under "Not currently planned".

## Library

Not supported. `skgo new` refuses `sv`'s `library` and `addon` templates, because a component package is not an application Go can serve. Build libraries with Kit's [packaging](https://svelte.dev/docs/kit/packaging) tools in a separate project.

## Offline app

Kit's service workers have not been verified under SKGo's binary, and no cell of the compatibility matrix measures them yet. Treat an offline app as unsupported until one does.

## Desktop and mobile app

Generated Swift clients for your Go queries and commands are released; [`skgo native build`](/docs/cli/native-build) prepares bindings, native libraries and an Xcode project and compiles a Debug application for macOS, iPhone or the simulator. Hosting the application natively, with bundled or hosted Go, including lifecycle, WebView coordination and distribution, is not finished, so a compiled Debug build is not a deployed app. See Native targets.

## Browser extension and embedded device

Not targets. SKGo has no browser-extension output, and its binary is meant for a server, not a microcontroller.
