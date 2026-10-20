---
title: Remote functions
description: Queries, commands, forms, live and batch queries written in Go, and how they refresh each other.
division: skgo
section: Core concepts
order: 2060
---

[Remote functions](https://svelte.dev/docs/kit/remote-functions) are functions a component calls as if they were local and that run on the server. In SKGo they are Go functions in a `*.remote.go` file, and `skgo generate` writes the `*.remote.ts` beside it that your components import. The generated bodies throw; the Go function is the only thing that ever answers.

A `var` block at the end of the file says which functions are exposed and as what:

```go example/web/src/routes/samples/remote-functions/notes.remote.go
```

| Marker              | Kit's kind    | Go signature                                                              |
| ------------------- | ------------- | ------------------------------------------------------------------------- |
| `skgo.Query`        | `query`       | `func(ctx) (T, error)` or `func(ctx, arg A) (T, error)`                   |
| `skgo.BatchQuery`   | `query.batch` | `func(ctx, args []A) ([]T, error)`                                        |
| `skgo.LiveQuery`    | `query.live`  | `func(ctx, yield func(T) error) error`                                    |
| `skgo.Command`      | `command`     | `func(ctx, event params.RequestEvent) (T, error)` or with `arg A` last    |
| `skgo.Form`         | `form`        | `func(ctx, event params.RequestEvent, fields F) (T, error)`               |
| `skgo.Prerender`    | `prerender`   | as `Query`, with `skgo.PrerenderOptions`                                  |

The argument and result types are ordinary Go types, and the generator writes their TypeScript; see [Types and transport](/docs/skgo/transport).

## query

A `query` reads data. It takes the request's context and, optionally, one argument, as `getNotes` and `getNote` above do:

```svelte example/web/src/routes/samples/remote-functions/+page.svelte
```

Everything Kit says about calling a query holds: `await` it inside a `<svelte:boundary>`, or read `.loading`, `.error` and `.current`; call `.refresh()` to ask for it again; two calls with the same argument in one render are one request. During server rendering, the renderer running inside your Go process calls the Go function directly, so the first value is in the document.

A query may read cookies through `skgo.EventFrom(ctx)` but not write them; a write is refused, and `skgo advice SKGO001` catches it statically. `event.URL()` panics in a query, as `event.url` throws in Kit, because a query is not tied to a page: anything about the page belongs in the argument. See [Using getRequestEvent](#Using-getRequestEvent).

A query that fails returns an error. `skgo.Errorf(status, format, ...)` is an expected error with a status; any other error is a 500 whose message the `handleError` hook decides, as in [Errors](/docs/skgo/load#Errors).

## query.batch

A `query.batch` answers several calls made in the same turn with one invocation. The Go function receives every argument and returns one result per argument, in the same order:

```go example/web/src/routes/batch/quotes.remote.go
```

A component still calls `getQuotes('SKGO')` with one symbol; Kit collects the calls, and an error for the batch fails every call in it.

## query.live

A `query.live` pushes values to the client as they change. The Go function receives a `yield` and calls it for every value until it returns or its context is cancelled, which happens when the client disconnects:

```go example/web/src/routes/samples/remote-functions/count.remote.go
```

The first value is rendered into the document like any query; the ones after it arrive over the open connection. The event a live query sees is a snapshot of the request that opened it: read the cookie once, at the top, and when it changes have the command that changed it [reconnect the stream](#Single-flight-mutations-Reconnecting-live-queries).

## form

A `form` receives what the browser posted, decoded into a Go struct whose JSON tags match the field paths the page uses:

```go example/web/src/routes/samples/remote-functions/form/message.remote.go
```

```svelte example/web/src/routes/samples/remote-functions/form/+page.svelte
```

The page side is Kit's: `fields`, `.as(...)`, `issues()`, `enhance`, `for(key)` and multiple submit buttons all work as documented, and a form works with JavaScript off, because the browser posts it and Go answers with the re-rendered document.

A `skgo.File` field receives an uploaded file with its name, type and bytes already read. A keyed instance made with `form.for(key)` delivers its key as the `id` field of the argument, which is what `Draft.ForKey` above is for.

### Validation

The Go struct is the schema: a submission whose values cannot be decoded into it is rejected before the function runs. For rules the type cannot express, return a `*skgo.Invalid`. `invalid.Add(path, format, ...)` attaches a message to a field, `invalid.Err()` returns the error when any were added, and `skgo.Invalidf(path, format, ...)` does both for a single issue. Kit's client puts each message on `fields.<name>.issues()` and leaves the form as the visitor typed it.

### Returns and redirects

What the function returns is `sendMessage.result` on the page. Return a `*skgo.Redirect` to send the visitor elsewhere after a successful submission; Kit's client follows it, and a native submission gets the HTTP redirect.

## command

A `command` writes. It takes the context, the request event and, optionally, one argument, and it may set cookies:

```go example/web/src/routes/samples/remote-functions/add.remote.go
```

```svelte example/web/src/routes/samples/remote-functions/AddNote.svelte
```

Kit refuses to run a command while a page renders on the server, so a command is called from an event handler, never from markup. The `event` argument is the generated `params.RequestEvent`, with `Locals` and typed `Params`; see [Using getRequestEvent](#Using-getRequestEvent).

## Single-flight mutations

After a command or form changes something, the queries that showed the old value need new ones. Kit's answer is to return them on the same response, and in SKGo the Go function decides which.

### Server-driven

`skgo.Refresh(ctx, query, arg)` names a query function and the argument of the instance to refresh, in Go, with the query's own parameter type checking the argument:

```go example/web/src/routes/samples/remote-functions/rename.remote.go
```

```svelte example/web/src/routes/samples/remote-functions/EditNote.svelte
```

The page that calls `renameNote` passes no `.updates(...)`; the refreshed value is on the response because the server put it there.

### Client-requested

When the page writes `addNote(text).updates(getNotes())`, Kit's client posts the keys of the query instances it wants back. That list is client input, so the server does not act on it unasked: a command runs the requested queries its handler names, and no others.

- `skgo.RefreshRequestedNoArg(ctx, getNotes)` runs the requested instance of a query that takes no argument
- `skgo.RefreshRequested(ctx, getNote, 1)` runs up to one requested instance of a query that takes an argument; a request for more comes back refused
- `skgo.IgnoreRequested` and `skgo.IgnoreRequestedNoArg` acknowledge a requested query and deliberately leave it stale, which Kit's client requires for any requested query the command does not run

```go example/web/src/routes/todos/gate/gate.remote.go
```

### Reconnecting live queries

A live query's event is a snapshot of the request that opened it, so a command that changes what it should see, such as signing in, reconnects it with `skgo.ReconnectRequestedNoArg(ctx, watchCount)` so that the new stream reads the new cookie:

```go example/web/src/lib/auth.remote.go
```

A command in one route package can name a query in another only through the generated import path for that directory; see [Route parameters](/docs/skgo/routing#Route-parameters).

## prerender

A `skgo.Prerender` is a query answered at build time and served as a file. `PrerenderOptions.Inputs` lists the arguments to render; see [Page options](/docs/skgo/page-options#prerender-Prerendered-remote-functions). It is distinct from `query`: Kit throws when a `query`, `query.batch` or `query.live` is called while a page is prerendered, so data a prerendered page reads through a remote function comes from a `skgo.Prerender`. See [Prerendering](/docs/skgo/prerendering#Remote-prerender-functions).

## Validation

Argument types are declared in Go and the generator writes them to TypeScript, so a call site that passes the wrong shape fails to type-check, and a request that does not decode into the Go type is rejected before your function runs, as a schema rejects it in Kit. There is no schema library to add. A field that may be absent is a `polytype.Optional[T]`, which tells absence from the zero value:

```go example/web/src/routes/optional/optional.remote.go
```

## Using getRequestEvent

`skgo.EventFrom(ctx)` is Kit's `getRequestEvent`, available in every kind of remote function, and [Go and web standards](/docs/skgo/go-standards#context.Context) shows the usual helper written with it. Commands and forms also receive the generated `params.RequestEvent` as their second argument, whose `URL()` is the calling page's URL and whose `Params` are the page's route parameters, typed: `event.Params.ID()` yields an interface with one concrete type per matcher that can produce the segment, so a `switch` over it tells a string `[id]` from an `[id=Order]`. In a query, `URL()` panics and `Params` is empty, as Kit's query has neither.

## Redirects

A query that returns a `*skgo.Redirect` while a page renders redirects the whole document, as a thrown `redirect` does in Kit:

```go example/web/src/routes/error/redirect/redirect.remote.go
```

A form may redirect after a successful submission, as above. A command may not: Kit's wire format for a command has no room for one, so a `Redirect` returned from a command is reported as an error.
