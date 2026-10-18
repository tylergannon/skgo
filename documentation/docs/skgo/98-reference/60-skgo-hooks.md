---
title: "skgo: hooks"
summary: Handle, HandleFetch, HandleError and the middleware sequences, in Go.
---

Kit's server hooks, in Go. `Handle` wraps every request and decides how it is
resolved; `HandleFetch` intercepts the `fetch` a load or remote function makes
on the server; `HandleError` shapes what an unexpected error reports.
`Sequence` composes several `Middleware` values into one `Handle`, as Kit's
`sequence` does, and `RequestSequence` is the same for the typed
`RequestMiddleware`. Kit's guide is [Hooks](https://svelte.dev/docs/kit/hooks).

> PACKAGE: github.com/tylergannon/skgo
> SYMBOLS: Handle HandleConfig HandleError HandleFetch Middleware RequestHandle RequestMiddleware RequestResolve Resolve ResolveOptions HookValues
