---
title: Cookies
summary: Read, set and delete cookies from Go, and where each kind of handler may write them.
proof:
  - example/e2e/features/auth.feature
  - TestCommandSetsCookieWithKitDefaults
  - TestCommandDeleteCookieExpiresIt
  - TestDeletedCookieReadsAsAbsent
  - TestRelativeCookiePathIsRefused
  - TestQueryCannotWriteCookies
  - TestSecureCookieDefaultMirrorsKit
  - TestAnEndpointsEventRefusesToWrite
  - TestCookiesAFetchSetsReachTheNextFetchAndTheDocument
---

Kit gives server code a [`cookies`](https://svelte.dev/docs/kit/@sveltejs-kit#Cookies) object on the request event. In SKGo the same cookie jar is on the Go `*skgo.Event`, with three methods: `Cookie`, `SetCookie` and `DeleteCookie`. Loads, form actions, `handle` and remote functions all reach it, either through their typed event or with `skgo.EventFrom(ctx)`.

## Reading cookies

`Cookie` returns a cookie's value and whether it was present. The example application's root layout calls this on every request to give a browser a visitor id:

```go
/// file: businesslogic/visitor/visitor.go
/// include: example/businesslogic/visitor/visitor.go#start
```

As in Kit, `Cookie` sees what this request has already written. A cookie set earlier in the same request is returned in place of the one the browser sent, and a deleted cookie reads as absent. When the browser sent several cookies with the same name, the one with the most specific path wins.

## Setting and deleting cookies

`SetCookie` and `DeleteCookie` return an error. Return it, because a cookie that could not be written is a request that did not do what it says:

```go
/// file: src/lib/auth.remote.go
/// include: example/web/src/lib/auth.remote.go#sign-in
```

[Auth](auth) has the full sign-in and sign-out pair.

`DeleteCookie` sets the cookie to an empty value with `Max-Age=0`, as Kit's `cookies.delete` does. Pass the same `Path` and `Domain` you set it with, or the browser keeps the original. [skgo check](/docs/cli/check) reports a discarded cookie error as `SKGO004`.

### CookieOptions

The zero value of `skgo.CookieOptions` gives Kit's defaults:

| Field      | Default                                         |
| ---------- | ----------------------------------------------- |
| `Path`     | `/`                                             |
| `Domain`   | none, so the cookie belongs to the request host |
| `MaxAge`   | none, so it is a session cookie                 |
| `Expires`  | none                                            |
| `SameSite` | `http.SameSiteLaxMode`                          |
| `HTTPOnly` | `true`                                          |
| `Secure`   | `true`, unless the origin is `http://localhost` |

`HTTPOnly` and `Secure` are `*bool`, so that `nil` means the default. Opt out with a pointer to `false`.

`Path` must be absolute and the name must not be empty. Kit resolves a relative path against the current URL outside remote functions; SKGo refuses it everywhere, and `SetCookie` returns an error.

The `Secure` default looks at the origin the server was given, not the request. Only an `http` origin whose host is `localhost` turns it off, so an origin of `http://127.0.0.1:8080` still sets `Secure` cookies. Use `http://localhost:8080` as the development origin, or set `Secure` explicitly, if your browser drops them.

## Where cookies can be written

| Handler                                                     | Read | Write                                        |
| ----------------------------------------------------------- | ---- | -------------------------------------------- |
| `handle`                                                    | yes  | yes, until the response exists               |
| server load (`page.server.go`, `layout.server.go`)          | yes  | yes                                          |
| form action                                                 | yes  | yes                                          |
| remote `command` and `form`                                 | yes  | yes                                          |
| remote `query`, `query.live`, `query.batch` and `prerender` | yes  | no                                           |
| `+server` route (`server.go`)                               | yes  | with `http.SetCookie` on the response writer |
| `handleError`                                               | yes  | no                                           |

A query's result is cached by its argument and can be served again without running, so Kit refuses cookie writes there, and so does SKGo: `SetCookie` returns `cannot set cookies in a query; only a command may write them`. A query that a command calls directly from Go runs with the command's event and can still write ([#271](https://github.com/tylergannon/skgo/issues/271)). `skgo check` reports the attempt as `SKGO001`, and [skgo advice](/docs/cli/advice) explains the fix, which is to move the write to a command or form.

A `+server` route writes its own response, so its event refuses `SetCookie` with `a server route writes its own response — use http.SetCookie`. `http.Cookie` has no defaults, so set the attributes you want yourself: `HttpOnly`, `Secure` and `SameSite` are all off unless you name them. This route sets two cookies when asked, and names only `Path`:

```go
/// file: src/routes/api/replay/[kind]/server.go
/// include: example/web/src/routes/api/replay/[kind]/server.go#cookie
```

The `handle` hook can write cookies before it calls `resolve`. Once the response exists, a write returns `cannot set cookies after the response has been generated`. Cookies the hook writes are added to the response `resolve` produced, and to a redirect the hook returns, but not to a response the hook builds itself. See [Hooks](hooks#handle).

A prerendered load cannot write a cookie; the build fails. See [Page options](page-options).

## Cookies and server fetch

`event.Fetch` from Go, and `fetch` in a universal load during server rendering, pass cookies on as [Kit's `event.fetch`](https://svelte.dev/docs/kit/load#Making-fetch-requests) does:

- A request to the app's own routes is answered in-process, with fresh locals. It carries the page's cookies, with cookies written earlier in this request taking precedence over the ones the browser sent, and an explicit `Cookie` header taking precedence over both.
- A request that leaves the app carries cookies only to the app's host and its subdomains, not to a sibling or parent domain. Use [`handleFetch`](hooks#handleFetch) to add them there.
- `skgo.WithFetchCredentials(skgo.FetchOmit)` sends none.
- A `Set-Cookie` header on an in-process response becomes a cookie of this request. It is sent on later fetches and to the browser. A response from `handleFetch` or from an HTTP request is not harvested this way.
- A path the build prerendered is fetched without the page's cookies.

Under `skgo dev`, a `Set-Cookie` on a response Go fetched while Kit rendered the page does not reach the browser; see the [development server](development-server#Differences-from-production).

## Not provided

Kit's `cookies.getAll` and `cookies.serialize`, and the `encode` and `decode` options, have no Go counterparts. `event.Request().Cookies()` lists the cookies the browser sent, without this request's writes, and `net/http` encodes values with `http.Cookie`.
