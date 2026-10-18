---
title: Advanced routing
description: Rest and optional parameters, Go parameter matchers, typed parameter values and layout groups.
division: skgo
section: Advanced
order: 4010
---

Route syntax is Kit's. Rest parameters, optional parameters, matchers, sorting, encoding and layout groups all mean what [Kit's advanced routing](https://svelte.dev/docs/kit/advanced-routing) says they mean, and the browser's router is Kit's own. The difference is on the server: Go matches the request, using the route patterns and order Kit compiled, and hands your Go code typed parameter values. This page covers what that adds. The basics of parameters and matchers are on [Routing](/docs/skgo/routing#Route-parameters).

## Rest parameters

A `[...rest]` segment matches any number of path segments, as in [Kit](https://svelte.dev/docs/kit/advanced-routing#Rest-parameters). In a Go load, endpoint or remote function, the parameter is one `string` holding the matched segments joined by `/`, and the empty string when nothing matched:

```go example/web/src/routes/shadow/[...rest]/server.go
```

A load on a rest route gets the same value from its generated accessor, `event.Params.Rest()`.

### 404 pages

Kit's pattern for a [custom 404 inside a subtree](https://svelte.dev/docs/kit/advanced-routing#Rest-parameters-404-pages) works with a Go load. Add a `[...path]` route under the subtree and return a 404 from its load:

```go example/web/src/routes/missing/[...path]/page.server.go
```

The nearest `+error.svelte` above the route renders the error, as [Errors](/docs/skgo/errors#App-errors) describes.

## Optional parameters

A `[[lang]]` segment may be absent, as in [Kit](https://svelte.dev/docs/kit/advanced-routing#Optional-parameters). Its Go accessor returns a pointer, `nil` when the segment is missing:

```go example/web/src/routes/lang/[[lang]]/page.server.go
```

An optional parameter with a matcher, such as `[[n=Order]]`, returns a pointer to the matcher's type. When the matcher rejects a segment, the segment is not consumed and the next parameter or route gets a chance at it, as in Kit.

## Matching

Kit declares matchers in `src/params.ts` with [`defineParams`](https://svelte.dev/docs/kit/@sveltejs-kit-params), and the browser runs them. SKGo matches on the server in Go, so every matcher has two implementations: the Kit one in `src/params.ts` and a Go one in `src/params.go`. You write both; neither is generated from the other. [Routing](/docs/skgo/routing#Route-parameters-Matching) shows the pair for the `Order` matcher. `skgo generate` stops with `matcher "Order" requires a Go matcher in src/params.go` when the Go half is missing, and refuses a route that names a matcher declared on only one side.

### Go matchers

A Go matcher is an exported function in `src/params.go` with the same name as the Kit matcher. It takes the decoded segment and returns the parameter's value and whether the segment matched. Its signature must be `func(string) (T, bool)`; `skgo generate` stops with `matcher Order must be func(string) (T, bool)` otherwise.

`T` can be any exported type, but neither `src/params.go` nor the type's package may import your generated bindings, your route packages or a package with a `.remote.go` file, because the bindings import the matcher. A type declared in `src/params.go` itself qualifies. Otherwise `skgo generate` stops with `move matcher domain types to a leaf package`.

The two halves must agree. When Go accepts a segment the browser rejects, a client-side navigation goes to a different route than a full page load does. Test the boundaries of the Go matcher with an ordinary Go test beside it, and keep the TypeScript matcher's rules the same:

```go example/web/src/params_test.go
```

When the Go matcher rejects a segment, Go tries the next route in Kit's order, the same fallback Kit's router applies. A route with a matcher and no Go load still runs the Go matcher, so a `+page.svelte` with only a universal load or a `server.go` under `[number=Order]` gets the same fallback.

### Typed parameter values

The value a Go matcher returns is the parameter's Go type. On `typed-load/[number=Order]`, the page's accessor returns an `OrderNumber`, with its methods:

```go example/web/src/routes/typed-load/[number=Order]/page.server.go
```

Kit's own matcher return value still becomes the parameter in the browser, so `page.params.number` is what `params.ts` returned, not the Go type.

Where the accessor's type comes from depends on where you read it:

- A page reads its own parameters as plain values: `string`, or the matcher's type.
- A layout reads its own parameters as values, and parameters that only a child page has as pointers, `nil` when the current page lacks them. When two child pages give the same name different types, the accessor returns an interface with one variant per type, such as `NumberParam_OrderNumber`.
- Hooks, commands and forms run for every route, so they see the app-wide `params.Params`, where every accessor returns such an interface: `Slug()` returns a `Key_Slug`, and a type assertion to `params.SlugParam_String` gives the string.

A layout above `[section]/[item]` reads `Section()` as a value and `Item()` as a pointer:

```go example/web/src/routes/event-layout/[section]/layout.server.go
```

Every accessor records that the load read the parameter, so Kit reruns the load when it changes. Read a parameter inside `event.Untrack`, as the layout above does, to avoid that; see [Loading data](/docs/skgo/load#Rerunning-load-functions-Untracking-dependencies).

## Sorting

Kit's [sorting rules](https://svelte.dev/docs/kit/advanced-routing#Sorting) apply. Go reads Kit's compiled route order from the build manifest and tries routes in that order, so the server and the browser pick the same route.

## Encoding

Kit's [encoded characters](https://svelte.dev/docs/kit/advanced-routing#Encoding) in route directory names, such as `[x+3a]`, are compiled by Kit into the route pattern, and Go matches against that pattern. Parameter values reach Go decoded the way Kit decodes them: `/items/a%2Fb` gives `a/b` to `[id]`, and a malformed escape such as `%FF` is a `400 Bad Request` before any load or matcher runs.

Directory names like `[id]` and `(marketing)` are not legal Go import paths; see [Routing](/docs/skgo/routing#Go-packages) for how code outside the route tree imports them.

## Advanced layouts

[Layout groups](https://svelte.dev/docs/kit/advanced-routing#Advanced-layouts-(group)) and `+page@` and `+layout@` [resets](https://svelte.dev/docs/kit/advanced-routing#Advanced-layouts-Breaking-out-of-layouts) are decided by Kit's Svelte files, and Go code is not involved. A Go load in a group is an ordinary route package whose directory name has parentheses: `src/routes/(marketing)/pricing/page.server.go` is `package pricing`, its URL is `/pricing`, and its generated event type is the same `PageRequestEvent` any page gets.
