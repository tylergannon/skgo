---
title: "skgo: endpoints"
summary: "+server routes in Go: GET, POST and the other method markers, Fallback and NewResponse."
---

The Go side of a `+server.ts` file. A `server.go` in a route directory holds
one Go function per HTTP method, marked with `GET`, `POST`, `PUT`, `PATCH`,
`DELETE`, `HEAD`, `OPTIONS` or `QUERY`, plus an optional `Fallback` for every
other method. Each handler is an ordinary `net/http` handler signature; the
server mounts it at the route Kit would have mounted the TypeScript one. Kit's
guide is [Routing](https://svelte.dev/docs/kit/routing#server).

`NewResponse` builds an `*http.Response` for the composition points that
return one instead of writing to a `ResponseWriter`, such as hooks.

> PACKAGE: github.com/tylergannon/skgo
> SYMBOLS: GET POST PUT PATCH DELETE HEAD OPTIONS QUERY Fallback NewResponse Endpoint Endpoints EndpointConfig NewEndpoint NewEndpoints
