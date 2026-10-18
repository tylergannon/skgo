---
title: Auth
summary: Sessions in Go, decided once in the handle hook and read everywhere else through locals.
proof:
  - example/e2e/features/auth.feature
  - TestTheAccountSectionTurnsASignedOutVisitorAway
  - TestACommandRefusesTheTodoTheQueryRefuses
---

Auth in SKGo follows Kit's [auth guide](https://svelte.dev/docs/kit/auth): decide who the caller is once per request in the `handle` hook, put the answer in `locals`, and check it wherever data is read or changed. The session store is yours; any Go library or your own table will do. This page shows where each piece goes.

## Typed locals

`Locals` in `internal/app/locals.go` is the Go counterpart of `App.Locals`. `skgo new` creates it empty, and you add what a request should know about its caller. The example's holds the session its hook looked up:

```go
/// file: internal/app/locals.go
/// include: example/internal/app/locals.go
```

Every request gets a fresh `Locals`, as in Kit. A load, a command or a form receives it on its typed event as `event.Locals`; a query, a page action or a `server.go` handler has only a `context.Context` and reads it with `app.LocalsFrom(ctx)`, which `skgo generate` writes into this package.

## Deciding in handle

The `handle` hook in `internal/serverhooks/handle.go` runs before anything answers the request. Read the cookie there, look the session up, and store the result:

```go
/// file: internal/serverhooks/handle.go
/// include: example/internal/serverhooks/handle.go#initialize
```

`params.Handle(fn).Middleware()` turns a before-only function into the hook, and `params.Sequence` composes it with middleware that needs to see the response. The hook can also refuse a request outright: returning a `*skgo.Redirect` sends the visitor elsewhere, and returning `skgo.Errorf(http.StatusForbidden, ...)` answers with that error, each shaped for the kind of request it was, a page, a `__data.json` or a remote call.

## Guarding pages

A guard is a layout load that reads the session and turns a signed-out visitor away. Every page below the layout is covered without knowing how:

```go
/// file: src/routes/account/layout.server.go
/// include: example/web/src/routes/account/layout.server.go#guard
```

Kit's caution applies unchanged: a layout load protects pages only. Remote functions, form actions and `server.go` routes never run it, so each of them checks `locals` for itself. See [Implications for authentication](load#Implications-for-authentication) for why a page load does not need to wait on its parent to be safe.

## Checking in remote functions and routes

A query has no event parameter, so it reaches the request through the context, exactly as Kit's `getRequestEvent()` does:

```go
/// file: src/routes/todos/todos.remote.go
/// include: example/web/src/routes/todos/todos.remote.go#event-from
```

A `server.go` handler reads the same locals from the request's context:

```go
/// file: src/routes/api/todos/server.go
/// include: example/web/src/routes/api/todos/server.go#get
```

A command or form receives `params.RequestEvent`, so its check is a field read on `event.Locals`. Cross-site form posts and non-`GET` remote calls are refused before any of this runs, as Kit's CSRF check refuses them; see [Security](differences#Security).

## Signing in and out

Kit allows cookie writes from server loads, actions, commands, forms and `handle`, and never from a query. SKGo enforces the same rule: `SetCookie` from a query returns an error, and the `skgo advice SKGO001` analyzer check flags it before it runs. A sign-in form that updates the page without a reload is a command:

```go
/// file: src/lib/auth.remote.go
/// include: example/web/src/lib/auth.remote.go#sign-in
```

The command writes the cookie, then names the queries whose answers the session changed with `skgo.RefreshRequestedNoArg`, so the page's `.updates(...)` is honoured and nothing else reruns. Signing out is the mirror image: `event.DeleteCookie` and the same refreshes. See [Single-flight mutations](remote-functions#Single-flight-mutations).

A sign-in that must work with JavaScript off is a page form action that writes the cookie and redirects:

```go
/// file: src/routes/actions/page.server.go
/// include: example/web/src/routes/actions/page.server.go#sign-in
```

To report a bad password and keep the fields filled, return `skgo.Fail` from an action declared with `skgo.ActionWithFailure`; [Validation errors](form-actions#Anatomy-of-an-action-Validation-errors) shows the shape.

## Refusing an action

An action that the caller may not perform returns an error with the status, and Kit renders the nearest `+error.svelte`:

```go
/// file: src/routes/actions/page.server.go
/// include: example/web/src/routes/actions/page.server.go#forbidden
```

## Cookie defaults

`skgo.CookieOptions` takes Kit's defaults: path `/`, `HttpOnly`, `SameSite=Lax`, and `Secure` unless the application's origin is `http://localhost`. `MaxAge` zero makes a session cookie. The origin is the one the build was fixed to, not the request's URL, so a development server on `http://127.0.0.1` writes `Secure` cookies; see [Development](differences#Development).

## JavaScript auth libraries

A Kit auth library that installs `hooks.server.ts` or `+page.server.ts` handlers cannot run here, because SKGo generates those files and refuses authored ones. Use a Go library for sessions, OAuth and password hashing; the frontend half of such a library, if it has one, is ordinary Svelte.

> [!NOTE] Under `skgo dev`, a cookie set by Go on a render-time subrequest reaches server code but is not relayed into the page response. Check your sign-in flow in both development and the built binary.
