---
title: State management
summary: What Kit's rules about server state mean when the server is one long-running Go process.
proof:
  - example/e2e/features/loads.feature
  - example/e2e/features/gate.feature
  - TestTypedDependencySerialsBelongToEachCaller
---

Kit's [State management](https://svelte.dev/docs/kit/state-management) page is written for a server that may be a serverless function, a Node process or an edge worker. An SKGo application is one Go process that serves every visitor for as long as it runs, with every request on its own goroutine. That makes Kit's rules easier to break and more important to keep.

## Avoid shared state on the server

A package-level variable in Go is shared by every request the process ever serves. This is wrong:

```go
// Do not do this. `user` is one variable for every visitor at once.
var user businesslogic.Session

func load(event PageRequestEvent) (PageData, error) {
	user = sessionFor(event)
	return PageData{Name: user.User}, nil
}
```

Two visitors loading the page at the same moment overwrite each other, and a goroutine race is the kindest failure available. State that belongs to a request belongs in one of three places.

**Locals.** `internal/app/locals.go` declares the Go counterpart of `App.Locals`, a struct the `handle` hook fills once per request and every load, action, remote function and `server.go` handler reads back:

```go
/// file: internal/app/locals.go
/// include: example/internal/app/locals.go
```

```go
/// file: internal/serverhooks/handle.go
/// include: example/internal/serverhooks/handle.go#initialize
```

**Cookies.** A value that must survive between requests, such as a session id, travels in a cookie and is looked up on each request; the `handle` hook above does exactly that. See [Cookies](load#Cookies).

**Per-visitor storage.** State the server keeps for a visitor between requests is keyed by who they are. The example keeps a todo list per visitor id and looks it up from the request's context:

```go
/// file: businesslogic/visitor/visitor.go
/// include: example/businesslogic/visitor/visitor.go#store
```

Whatever the storage is, a map, a database or a cache, it is reached from several goroutines at once. Guard in-process state with a mutex, as `businesslogic.For` does behind that call, or use types that are safe for concurrent use.

## No side-effects in load

Kit's rule, for the same reasons: a load runs on every render and every navigation that depends on it, at build time if the page is prerendered, and concurrently with the other loads of the page. A load reads; a [command](remote-functions#command), a [form](remote-functions#form) or a [page action](form-actions) writes. The `skgo advice SKGO001` check catches the most common slip, a cookie written from a `query`.

## Using state and stores with context

Unchanged. Svelte's `setContext` and `getContext` are the way to share state between components during a render, and Kit's advice about not using module-level stores on the server applies to your Svelte code exactly as it does in any Kit application.

## Component and page state is preserved

Unchanged. When a Go load reruns, Kit updates `data` in place and keeps the component; see [When do load functions rerun?](load#Rerunning-load-functions-When-do-load-functions-rerun). Derive from `data` with `$derived`, or reset in `afterNavigate` or with a `{#key}` block, as Kit describes.

## Storing state in the URL

Unchanged. State that should survive a reload, such as a filter or a sort order, goes in the URL, and a Go load reads it with `event.SearchParam(name)`, rerunning when that parameter changes; see [Using URL data](load#Using-URL-data).

## Storing ephemeral state in snapshots

Unchanged. Snapshots are Kit's client feature and need nothing from the server.

## When the server's state changes

A long-running Go server knows when its state changes, and can tell the page. A [`query.live`](remote-functions#query.live) pushes new values to every open page as they happen; a command's [single-flight refresh](remote-functions#Single-flight-mutations) returns the queries a write changed on the write's own response; and `event.Depends` with `invalidate` reruns a load on demand. None of these replaces the rules above: each one reads state the request can see, and none of them stores it in a package variable.
