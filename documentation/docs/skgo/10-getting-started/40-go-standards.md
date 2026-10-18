---
title: Go and web standards
summary: How Kit's web-standard APIs map onto Go's context, net/http and standard library types.
proof:
  - example/e2e/features/request-fetch.feature
  - example/e2e/features/stream.feature
  - example/e2e/features/form.feature
  - example/e2e/features/actions-form-data.feature
---

SvelteKit's server code is written against [web standards](https://svelte.dev/docs/kit/web-standards): `Request`, `Response`, `Headers`, `FormData`, streams, `URL` and Web Crypto. In SKGo the server is Go, so the same jobs are done with `context.Context`, `net/http` and the Go standard library. Your Svelte components still use the web APIs, and in the browser they work as Kit describes; on the server, SvelteKit's renderer runs on goja inside the Go binary, which is not a browser.

## context.Context

Every Go handler receives the request's context: remote functions and actions as their first argument, loads through `event.Context()`, and `+server` handlers through `r.Context()`. `skgo.EventFrom(ctx)` returns the request event from that context. It is the Go counterpart of [`getRequestEvent`](https://svelte.dev/docs/kit/load#Using-getRequestEvent), so a helper several calls deep can read a cookie without the event being passed down:

```go
/// file: src/routes/todos/todos.remote.go
/// include: example/web/src/routes/todos/todos.remote.go#event-from
```

Outside a request, `EventFrom` returns nil, and every method is safe on a nil event: a cookie reads as absent and a write fails. A `context.Background()` is such a context, so a handler that starts one loses its request; `skgo advice SKGO003` describes the analyzer check that catches it. See [`skgo advice`](/docs/cli/advice).

The context also carries what the loads above a page produced. `skgo.Parent[T](ctx)` is Kit's `await parent()`, decoded into the layout's data type:

```go
/// file: src/routes/account/page.server.go
/// include: example/web/src/routes/account/page.server.go#parent
```

## net/http

Kit's Fetch API objects are `net/http` types in Go.

### Request

A `+server` handler is an `http.HandlerFunc` and receives `*http.Request` directly. In actions and remote functions, `skgo.EventFrom(ctx).Request()` returns the request that carried the call. For a remote function its URL is the `/_app/remote/...` endpoint, not the page: in a `command` or `form`, the `event` argument's `URL()` is the calling page's URL, while `skgo.EventFrom(ctx).URL()` panics there and in every `query`, as Kit throws on `event.url` in a query. Anything a remote function needs about the page belongs in its argument.

### Response

A `+server` handler writes its own response through `http.ResponseWriter`, and SKGo does not wrap or re-encode it:

```go
/// file: src/routes/api/replay-count/server.go
/// include: example/web/src/routes/api/replay-count/server.go#handler
```

A `handle` hook written as a `Middleware` returns a `*http.Response`; build one with `skgo.NewResponse(status, header, body)` when the hook answers without resolving. Loads, actions and remote functions return Go values, and SKGo encodes them for Kit.

### Headers

Headers are `http.Header`. In an endpoint, set them on the `http.ResponseWriter`, as above. In a server load, a page action or the `handle` hook before `resolve`, `Event.SetHeader(name, value)` is Kit's [`setHeaders`](https://svelte.dev/docs/kit/load#Headers) with Kit's rules: it refuses `set-cookie`, and a header set twice is an error except `server-timing`, which accumulates. Cookies go through `Event.Cookie`, `SetCookie` and `DeleteCookie`.

### Beyond the standard library

What the request carries beyond `net/http` is reachable from the context: the route id, the route parameters and the application's locals, through the `LocalsFrom` helper generated in `internal/app`:

```go
/// file: src/routes/api/request-fetch/server.go
/// include: example/web/src/routes/api/request-fetch/server.go#locals
```

## Form data and files

A page action reads the posted form with the standard library: call `ParseForm` or `ParseMultipartForm` on `skgo.EventFrom(ctx).Request()` and read `PostForm`, or `FormFile` for an upload:

```go
/// file: src/routes/actions/page.server.go
/// include: example/web/src/routes/actions/page.server.go#upload
```

A remote `form` function receives a typed Go struct instead of `FormData`. Each field's JSON name matches the field name the page uses, and a `skgo.File` field receives an uploaded file with its `Name`, `Type`, `LastModified` and `Data` bytes already read:

```go
/// file: src/routes/contact/contact.remote.go
/// include: example/web/src/routes/contact/contact.remote.go#draft
```

A remote form body is limited to 32 MiB. See [Form actions](https://svelte.dev/docs/kit/form-actions) and [Remote functions](https://svelte.dev/docs/kit/remote-functions).

## Fetch

Kit gives loads, hooks and server routes a special `fetch` that calls the app's own endpoints in-process, with the visitor's credentials. `Event.Fetch` is that fetch: a relative URL resolves against the event's URL, a request for the app's own routes is answered in-process with fresh locals, and the page's cookies and `Authorization` travel with it, unless you pass `WithFetchCredentials(FetchOmit)`. Any other URL goes out over HTTP.

```go
/// file: src/routes/request-fetch/page.server.go
/// include: example/web/src/routes/request-fetch/page.server.go#fetch
```

Read the body to the end or close it, as with any `net/http` response. Fetching records no dependency, so `invalidate` does not re-run a load because it fetched, exactly as in Kit.

## Streaming

Instead of `ReadableStream` and promises, SKGo uses goroutines.

A load streams a value by returning a `skgo.Deferred[T]` field, created with `skgo.Async(ctx, fn)`. `fn` runs on its own goroutine, the page renders before it finishes, and the value arrives on the same response, as [streaming with promises](https://svelte.dev/docs/kit/load#Streaming-with-promises) does in Kit:

```go
/// file: src/routes/account/orders/page.server.go
/// include: example/web/src/routes/account/orders/page.server.go#deferred
```

A `query.live` is a `skgo.LiveQuery` whose function pushes values with `yield` until it returns or its context is cancelled, which happens when the client disconnects:

```go
/// file: src/routes/todos/todos.remote.go
/// include: example/web/src/routes/todos/todos.remote.go#live
```

A `+server` handler streams by writing and calling `Flush` through `http.Flusher`, as any Go handler does.

## URLs

`Event.URL()` returns the page's `*url.URL`, with Kit's private query parameters removed. Reading it makes a load depend on the whole URL. When one parameter matters, `Event.SearchParam(name)` records only that parameter, so the load reruns only when it changes, as Kit's own `url.searchParams.get` does:

```go
/// file: src/routes/actions/signed-in/page.server.go
/// include: example/web/src/routes/actions/signed-in/page.server.go#search-param
```

In an endpoint, `r.URL` is the ordinary request URL.

## Crypto

Use Go's `crypto` packages, `crypto/rand`, `crypto/sha256`, `crypto/hmac` and the rest, in server code where Kit code would use Web Crypto. Browser code keeps using `crypto.subtle` and `crypto.randomUUID()`.
