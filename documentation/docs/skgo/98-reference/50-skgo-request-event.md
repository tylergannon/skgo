---
title: "skgo: request event"
summary: Event and EventFrom, the request's URL, params, cookies, locals, fetch and tracing.
---

Kit's `RequestEvent`, in Go. Every load, action, remote function and hook
runs with a `context.Context` that carries the event; `EventFrom` returns it.
The event is where the URL, the route id and params, the cookies, the
request-scoped `fetch`, the client address and the response headers live.
Locals are typed: `RequestLocals` returns the application's own struct, the
one named by `--locals-type` at generation.

`RequestEvent` is the typed form the generated code hands to a handler that
declared route parameters and locals; `Event` is the untyped core both share.

> PACKAGE: github.com/tylergannon/skgo
> SYMBOLS: Event RequestEvent RequestLocals CookieOptions File Fetch FetchConfig FetchCredentials FetchMode FetchOption ParamMatcher CallerMatchers CallerRoute CallerRoutes Tracing
