---
title: "Integrations"
summary: "Which Svelte, Vite and Go integrations work in an SKGo project, and the limits of the embedded renderer."
---

The frontend of an SKGo project is a SvelteKit project built by Vite, so frontend integrations work as Kit's [integrations](https://svelte.dev/docs/kit/integrations) page describes, with two limits: nothing may add JavaScript server code, and code that runs during server rendering runs on goja in production, not Node. Server-side integrations are Go libraries.

## Svelte preprocessing

`vitePreprocess` and `svelte-preprocess` work unchanged; configure them as in [Kit](https://svelte.dev/docs/kit/integrations#vitePreprocess).

## sv add-ons

`skgo new` hands frontend creation to `sv create`, so you pick add-ons there. Frontend-only add-ons work. An add-on that writes JavaScript server code, such as a `hooks.server.ts`, a `+server.ts` or `src/lib/server`, is refused by name, because Go serves every server file; see [Creating a project](creating-a-project#Creating-a-project).

## Vite plugins

Vite plugins run in the frontend build as in Kit. A plugin's development-server middleware exists only under `skgo dev`; in production the Go binary answers every request.

## UI libraries

A component library works if the code it runs during server rendering runs without Node. The production renderer is a bare ECMAScript engine with the globals Kit and Svelte need and nothing more: `URL`, `URLSearchParams`, `TextEncoder`, `TextDecoder`, `btoa`, `atob`, `console`, `Headers`, `Request`, `Response`, `Blob`, `File` and `ReadableStream`. There is no `crypto`, no `FormData`, no global `fetch`, and `node:async_hooks` throws, so nothing built on `AsyncLocalStorage` can run there. `setTimeout` is a queue drained between microtask turns, not a timer.

The server bundle is built with every dependency included and no Node built-ins, resolved under the `production`, `module`, `import` and `default` conditions and neither `node` nor `browser`. Nothing is loaded at run time, so a library that imports `node:fs` or similar during render fails the build or the render. A library that touches `window` or `document` only in the browser is fine.

A library that uses a missing API during render works under `skgo dev`, which renders in Node, and fails only in the binary. The cheapest guard is a Go test that renders the page through `NewHandler`; see [Testing](testing#Testing-the-Go-server).

## Go libraries

Everything on the server is Go, so databases, authentication, logging, metrics, queues and HTTP clients are ordinary Go modules used from loads, actions, remote functions and `server.go` routes. Prefer pure-Go libraries to keep the binary cgo-free. [Observability](observability) covers OpenTelemetry tracing and [Auth](auth) covers sessions and cookies.
