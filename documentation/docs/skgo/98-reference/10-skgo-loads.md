---
title: "skgo: loads"
summary: "The server-load API: Load, Parent, Deferred and the parameter helpers."
---

The Go side of a `+page.server.ts` or `+layout.server.ts` load. You write a
function and mark it with `Load`; `skgo generate` writes the TypeScript stub
Kit compiles and the registration the server mounts. Kit's guide is
[Loading data](load); the Go side follows it.

A load receives a `context.Context` carrying the request. `EventFrom` on the
[request event](skgo-request-event) page gets you the URL, params, cookies and
`fetch`. `Parent` reads the parent layout's data, `Async` starts work that the
page streams in after the first response, and the `LoadParam` helpers read a
route parameter with the type its matcher produced.

> PACKAGE: github.com/tylergannon/skgo
> SYMBOLS: Entries RouteEntries NewRouteEntries Load Parent Deferred Async Resolved NewLoads NewServerLoad LoadParamValue OptionalLoadParamValue LoadRouteIDValue TrackLoadParam Prerender PreloadInput PrerenderLoadInput LoadConfig LoadSpec Loads ServerLoad
