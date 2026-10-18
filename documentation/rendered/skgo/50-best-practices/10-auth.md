---
title: "Auth"
summary: "Sessions in Go, decided once in the handle hook and read everywhere else through locals."
---

Auth in SKGo follows Kit's [auth guide](https://svelte.dev/docs/kit/auth): decide who the caller is once per request in the `handle` hook, put the answer in `locals`, and check it wherever data is read or changed. The session store is yours; any Go library or your own table will do. This page shows where each piece goes.

## Typed locals

`Locals` in `internal/app/locals.go` is the Go counterpart of `App.Locals`. `skgo new` creates it empty, and you add what a request should know about its caller. The example's holds the session its hook looked up:

```go
/// file: internal/app/locals.go
package app

import (
	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/businesslogic"
)

type Locals struct {
	Session           businesslogic.Session
	Visit             businesslogic.Visit
	Visitor           string
	RootLayoutFailure bool
	ScriptSafeProof   bool
}

type RequestEvent[P any] = skgo.RequestEvent[P, Locals]
```

Every request gets a fresh `Locals`, as in Kit. A load, a command or a form receives it on its typed event as `event.Locals`; a query, a page action or a `server.go` handler has only a `context.Context` and reads it with `app.LocalsFrom(ctx)`, which `skgo generate` writes into this package.

## Deciding in handle

The `handle` hook in `internal/serverhooks/handle.go` runs before anything answers the request. Read the cookie there, look the session up, and store the result:

```go
/// file: internal/serverhooks/handle.go
// Handle is the app's `handle` hook — the one place it decides who the caller
// is. It runs once per request, before any load or remote function, and puts
// the answer where all of them can read it with app.LocalsFrom.
//
// A guard is then a load that reads the session; see
// web/src/routes/account/layout.server.go, which turns a signed-out visitor
// away from every page under /account without any of those pages knowing.
func initialize(ctx context.Context, event params.RequestEvent) (params.RequestEvent, error) {
	request := event.Request()
	event.Locals.RootLayoutFailure = request.URL.Query().Get("boom") == "root-layout"
	event.Locals.ScriptSafeProof = request.URL.Query().Get("proof") == "script-safe"
	if request.Method == http.MethodPost && request.URL.Path == "/actions" {
		switch request.URL.Query().Get("hook") {
		case "sign-in":
			return event, &skgo.Redirect{Status: http.StatusSeeOther, Location: "/actions/signed-in?required=1"}
		case "forbidden":
			return event, skgo.Errorf(http.StatusForbidden, "Hook denied this edit")
		}
	}
	if run := request.URL.Query().Get("run"); run != "" && request.URL.Path != "/api/replay-count" {
		businesslogic.Replays.Record(run, request.Method+" "+request.URL.Path)
	}
	id, _ := event.Cookie(SessionCookie)
	event.Locals.Session = businesslogic.Default.Session(id)
	return event, nil
}
```

`params.Handle(fn).Middleware()` turns a before-only function into the hook, and `params.Sequence` composes it with middleware that needs to see the response. The hook can also refuse a request outright: returning a `*skgo.Redirect` sends the visitor elsewhere, and returning `skgo.Errorf(http.StatusForbidden, ...)` answers with that error, each shaped for the kind of request it was, a page, a `__data.json` or a remote call.

## Guarding pages

A guard is a layout load that reads the session and turns a signed-out visitor away. Every page below the layout is covered without knowing how:

```go
/// file: src/routes/account/layout.server.go
// layoutLoad is the section's guard and its shared data.
//
// The session comes from the request's locals, which the app's one `handle`
// hook filled in; nothing here reads a cookie or knows how a session is
// spelled. Turning a signed-out visitor away here turns them away from every
// page below, so adding another one needs no new guard.
func layoutLoad(event LayoutRequestEvent) (LayoutData, error) {
	ctx := event.Context()
	session := app.LocalsFrom(ctx).Session
	if session.User == "" {
		return LayoutData{}, &skgo.Redirect{Status: 307, Location: "/"}
	}

	// The visitor can ask for this section's data again without navigating;
	// `invalidate('app:account')` in the browser is what makes kit's client ask
	// for this node and no other.
	event.Depends("app:account")

	return LayoutData{
		AccountUser:   session.User,
		AccountSerial: nextSerial(ctx),
	}, nil
}

var _ = skgo.Load(layoutLoad)
```

Kit's caution applies unchanged: a layout load protects pages only. Remote functions, form actions and `server.go` routes never run it, so each of them checks `locals` for itself. See [Implications for authentication](load#Implications-for-authentication) for why a page load does not need to wait on its parent to be safe.

## Checking in remote functions and routes

A query has no event parameter, so it reaches the request through the context, exactly as Kit's `getRequestEvent()` does:

```go
/// file: src/routes/todos/todos.remote.go
// signedIn reports whether the caller has a live session. The request is
// reachable from the context, the way kit reaches it with `getRequestEvent()`.
// A query may read cookies; only a command may write them.
func signedIn(ctx context.Context) bool {
	id, _ := skgo.EventFrom(ctx).Cookie(sessionCookie)
	return businesslogic.Default.Session(id).User != ""
}
```

A `server.go` handler reads the same locals from the request's context:

```go
/// file: src/routes/api/todos/server.go
// list answers `GET /api/todos` with the todos this visitor may see.
//
// Who the visitor is comes from the app's Handle hook, exactly as it does in a
// load or a remote function: the endpoint reads the session it stored rather
// than parsing the cookie again.
func list(w http.ResponseWriter, r *http.Request) {
	session := app.LocalsFrom(r.Context()).Session
	writeJSON(w, http.StatusOK, visitor.Store(r.Context()).Todos(session.User != ""))
}
```

A command or form receives `params.RequestEvent`, so its check is a field read on `event.Locals`. Cross-site form posts and non-`GET` remote calls are refused before any of this runs, as Kit's CSRF check refuses them; see [Security](differences#Security).

## Signing in and out

Kit allows cookie writes from server loads, actions, commands, forms and `handle`, and never from a query. SKGo enforces the same rule: `SetCookie` from a query returns an error, and the `skgo advice SKGO001` analyzer check flags it before it runs. A sign-in form that updates the page without a reload is a command:

```go
/// file: src/lib/auth.remote.go
// signIn opens a session and stores its id in an HttpOnly cookie. Kit allows
// cookie writes in commands and forms and nowhere else, so declaring this as
// anything but a command would make SetCookie fail.
//
// The page writes `signIn(name).updates(whoami(), getTodos(), watchCount)`.
// Everything that request asks for is named below, and nothing else is: a
// request naming a fourth query would run nothing.
func signIn(ctx context.Context, event params.RequestEvent, user string) (businesslogic.Session, error) {
	if user == "" {
		return businesslogic.Session{}, skgo.Errorf(400, "Who are you?")
	}
	id := businesslogic.Default.SignIn(user)
	if err := skgo.EventFrom(ctx).SetCookie(sessionCookie, id, skgo.CookieOptions{MaxAge: 60 * 60}); err != nil {
		return businesslogic.Session{}, err
	}
	if err := skgo.RefreshRequestedNoArg(ctx, whoami); err != nil {
		return businesslogic.Session{}, err
	}
	return businesslogic.Session{User: user}, todos.AcceptSessionRefreshes(ctx)
}
```

The command writes the cookie, then names the queries whose answers the session changed with `skgo.RefreshRequestedNoArg`, so the page's `.updates(...)` is honoured and nothing else reruns. Signing out is the mirror image: `event.DeleteCookie` and the same refreshes. See [Single-flight mutations](remote-functions#Single-flight-mutations).

A sign-in that must work with JavaScript off is a page form action that writes the cookie and redirects:

```go
/// file: src/routes/actions/page.server.go
func signIn(ctx context.Context) error {
	request := skgo.EventFrom(ctx).Request()
	if err := request.ParseForm(); err != nil {
		return skgo.Errorf(http.StatusBadRequest, "Invalid form")
	}
	if request.PostForm.Get("username") != "ada" {
		return skgo.Errorf(http.StatusBadRequest, "Unknown demo user")
	}
	if err := skgo.EventFrom(ctx).SetCookie(signInCookie, "ada", skgo.CookieOptions{Path: "/actions"}); err != nil {
		return err
	}
	return &skgo.Redirect{Status: http.StatusSeeOther, Location: "/actions/signed-in"}
}
```

To report a bad password and keep the fields filled, return `skgo.Fail` from an action declared with `skgo.ActionWithFailure`; [Validation errors](form-actions#Anatomy-of-an-action-Validation-errors) shows the shape.

## Refusing an action

An action that the caller may not perform returns an error with the status, and Kit renders the nearest `+error.svelte`:

```go
/// file: src/routes/actions/page.server.go
func forbidden(context.Context) error {
	return skgo.Errorf(http.StatusForbidden, "You cannot edit this profile")
}
```

## Cookie defaults

`skgo.CookieOptions` takes Kit's defaults: path `/`, `HttpOnly`, `SameSite=Lax`, and `Secure` unless the application's origin is `http://localhost`. `MaxAge` zero makes a session cookie. The origin is the one the build was fixed to, not the request's URL, so a development server on `http://127.0.0.1` writes `Secure` cookies; see [Development](differences#Development).

## JavaScript auth libraries

A Kit auth library that installs `hooks.server.ts` or `+page.server.ts` handlers cannot run here, because SKGo generates those files and refuses authored ones. Use a Go library for sessions, OAuth and password hashing; the frontend half of such a library, if it has one, is ordinary Svelte.

> [!NOTE] Under `skgo dev`, a cookie set by Go on a render-time subrequest reaches server code but is not relayed into the page response. Check your sign-in flow in both development and the built binary.
