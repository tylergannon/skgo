---
title: "skgo: hooks"
summary: "Handle, HandleFetch, HandleError and the middleware sequences, in Go."
---

Kit's server hooks, in Go. `Handle` wraps every request and decides how it is
resolved; `HandleFetch` intercepts the `fetch` a load or remote function makes
on the server; `HandleError` shapes what an unexpected error reports.
`Sequence` composes several `Middleware` values into one `Handle`, as Kit's
`sequence` does, and `RequestSequence` is the same for the typed
`RequestMiddleware`. Kit's guide is [Hooks](https://svelte.dev/docs/kit/hooks).

## ServerInit

```go
type ServerInit func() error
```

ServerInit is the application's server init hook. It runs synchronously while NewSSR or NativeDevelopment.NewSSR constructs an application instance, before the application's page, endpoint or remote handlers are returned for serving. This also applies when every page has ssr = false.

The hook has no request event. It may perform application I/O, such as opening a database, and must finish before serving starts. Returning an error prevents construction and reports the cause to the caller. Each new application instance runs its own hook; requests and renderer-pool runtimes never rerun it. A replacement development application is a new instance and initializes again.

## Handle

```go
type Handle func(ctx context.Context) error
```

Handle is the simple, before-only form of kit's `handle` hook (`hooks.server.js`, whose contract lives in `runtime/server/respond.js`): it runs once per request before anything answers it, and the request carries on to the application when it returns nil.

```go
func handle(ctx context.Context) error {
	user := users.FromCookie(skgo.EventFrom(ctx))
	return nil // low-level hooks do not bind application locals
}
```

Returning a \*Redirect or an \*HTTPError refuses the request there and then, and skgo shapes the refusal for the kind of request it was: a data request or a remote call gets the JSON node kit's client expects, a page request a real HTTP redirect.

The event it reads is kit's hook event: the normalized URL, the matched route and its parameters, and the request-kind flags, all resolved before the hook runs. It may write cookies and response headers, which ride out on whatever answers, because kit's `event.cookies` and `event.setHeaders` are available to `handle` until the response exists. A hook that has to look at the response — or answer without the application at all — is a [Middleware](#Middleware).

### Intercept

```go
func (h Handle) Intercept(cfg HandleConfig, next http.Handler) http.Handler
```

Intercept wraps next so that h runs once before it, on every request kind this app answers, mirroring where kit runs `hooks.handle` (`runtime/server/respond.js`): before dispatch, no matter what next is made of. Build cfg directly from the app's manifest — an app that answers only remote functions has no reason to also build a Loads registry merely to reach this hook.

A nil Handle is a plain pass-through, so an app with no hook at all has no reason to call Intercept either.

### Middleware

```go
func (h Handle) Middleware() Middleware
```

Middleware is the hook as a Middleware that runs h before the application.

## HandleConfig

```go
type HandleConfig struct {

	// ClientAddress resolves the visitor address from the original HTTP request.
	// Nil uses the peer IP in RemoteAddr. Proxy headers require an explicit provider.
	// The first value or error is shared by derived events and internal fetches.
	ClientAddress func(*http.Request) (string, error)
	// AppDir is kit's appDir; empty means "_app".
	AppDir string
	// Base is kit's paths.base, without a trailing slash.
	Base string
	// Origin is the app's fixed origin. It makes the event's URL absolute and
	// decides the `Secure` default of the cookies the hook writes, as in kit.
	Origin string
	// Version, when non-empty, is sent as the `x-sveltekit-version` response
	// header on a request the hook refuses — the same header a successful
	// data or remote response carries, so a client watching for a new
	// deployment sees it either way.
	Version string
	// Matchers contains the app route matchers when Loads is unset.
	Matchers map[string]ParamMatcher
	// Loads, when set, is the route table the hook matches against. It is the
	// one `__data.json` already uses;
	// without it the hook matches the manifest routes HandleConfig was made
	// from.
	Loads *Loads
	// Static, when set, reports that a request is answered before Kit's server
	// runs — a file out of `static/` or a prerendered page.
	// Kit's adapters serve those ahead of `handle`, so the hook never
	// sees them.
	Static func(r *http.Request) bool
	// HandleError shapes errors raised by the handle hook as Kit's App.Error.
	HandleError HandleError
	// ErrorTemplate is Kit's error.html, used for a fatal native hook error.
	ErrorTemplate string

	// OnPanic is called when the hook panics, with "handle", the recovered
	// value, and the stack. The client is told nothing but an opaque 500, so
	// this is the only record the panic leaves; leaving it nil logs the same
	// three things to the standard logger, because a panicking hook that
	// reports nowhere is a bug that cannot be found.
	OnPanic func(id string, value any, stack []byte)
	// contains filtered or unexported fields
}
```

HandleConfig is what Intercept needs to recognise the requests kit's own `handle` never sees and to resolve what kit resolves before it runs. AppDir and Base come straight from the built manifest; an app has nothing else to decide.

## HandleError

```go
type HandleError func(ctx context.Context, caught CaughtError) map[string]any
```

HandleError is skgo's mirror of kit's `handleError` hook (`exports/hooks/public.d.ts`, `HandleServerError`): the one place an app decides what a failed request's visitor is told, beyond status and message.

RemoteConfig uses it for rejected schema arguments. It also runs for every error that reaches a page response except a redirect, which is not a failure and never reaches it, and an already-handled built-prerender error whose recorded App.Error body is reused without running the hook again. That includes an error the app raised on purpose with Errorf: kit's own type never exempts an "app"-kind error from the hook, only from having its message replaced by default.

Returning nil keeps kit's own defaults exactly: an "app" error's message survives untouched, and an "unknown" error's stays the generic "Internal Error" a real one never leaks through. Returning a map adds or overrides fields on the JSON object the error page and the client both see — "status" and "message" included, if the hook chooses to replace them — the same partial-override rule kit's own type comment states: "the hook returns only the properties it wants to override; anything it omits ... is inherited from the caught error." A "status" entry must be an int and a "message" entry a string; anything else under those two keys is ignored, the same as a value kit's own type checker would reject.

The hook must never panic — kit's own doc comment says so too. One that does is logged and treated as returning nothing, except that the message is forced back to the fallback's default even for an "app" error: kit's own catch does the same (`log_handle_error_hook_failure`), because a hook that failed to run is not a hook whose omissions can be trusted.

## HandleFetch

```go
type HandleFetch func(ctx context.Context, request *http.Request, fetch Fetch) (*http.Response, error)
```

HandleFetch mirrors Kit's server hook. It receives the normalized request before the dispatcher inherits the page's credentials. EventFrom(ctx) exposes the originating request and its locals. The supplied fetch dispatches directly rather than re-entering the hook, and retains the caller's cancellation. Configure it through SSROptions for render-time fetches and FetchConfig for Event.Fetch; it runs in Go, never in the browser.

## Middleware

```go
type Middleware func(ctx context.Context, event *Event, resolve Resolve) (*http.Response, error)
```

Middleware is skgo's mirror of kit's `handle` hook in full: it receives the request's event and a resolve function, and returns the response.

```go
func handle(ctx context.Context, e *skgo.Event, resolve skgo.Resolve) (*http.Response, error) {
	// before: the event is resolved, locals and cookies are writable
	res, err := resolve(ctx) // the application's real response
	// after: wrap the response's headers or body
	return res, err
}
```

Not calling resolve answers the request without the application: return a response of your own (see [NewResponse](/docs/skgo/skgo-endpoints#NewResponse)), or an error. A returned \*Redirect or \*HTTPError is kit's thrown `redirect()` / `error()`, shaped for the kind of request it was; a response the middleware builds is sent exactly as it is. The pin does not add the event's cookies or headers to a response the hook builds itself, so neither does this: only a resolved response carries them.

Compose several with [Sequence](#Middleware-Sequence).

### Sequence

```go
func Sequence(middleware ...Middleware) Middleware
```

Sequence composes middleware the way kit's `sequence` does: the first runs outermost, each one's resolve runs the next, and the last one's resolve runs the application. Their after-logic unwinds in reverse. The [ResolveOptions](#ResolveOptions) they pass compose as in kit: inner transforms run before outer ones, and the first defined outer filter or preload callback wins, so an inner one is never called once an outer one exists.

### Intercept

```go
func (m Middleware) Intercept(cfg HandleConfig, next http.Handler) http.Handler
```

Intercept wraps next with the middleware, mirroring where kit runs `hooks.handle` (`runtime/server/respond.js`): after kit has normalized the URL and matched the route, before anything answers, for every request kind the server answers. A nil Middleware is a plain pass-through.

## RequestHandle

```go
type RequestHandle[P, L any] func(context.Context, RequestEvent[P, L]) (RequestEvent[P, L], error)
```

RequestHandle returns the event a before-only hook wants to forward.

### Middleware

```go
func (h RequestHandle[P, L]) Middleware() RequestMiddleware[P, L]
```

## RequestMiddleware

```go
type RequestMiddleware[P, L any] func(context.Context, RequestEvent[P, L], RequestResolve[P, L]) (*http.Response, error)
```

RequestMiddleware is a value-event hook with explicit downstream forwarding.

### RequestSequence

```go
func RequestSequence[P, L any](hooks ...RequestMiddleware[P, L]) RequestMiddleware[P, L]
```

RequestSequence retains each hook's incoming context binding. Only resolve forwards replacements; mutations of a shared object follow Go pointer rules.

### Intercept

```go
func (h RequestMiddleware[P, L]) Intercept(cfg HandleConfig, makeParams func(*Event) (P, error), next http.Handler) http.Handler
```

Intercept is the application's single locals allocator, including when the selected hook is nil. Raw Intercept still owns response and refusal semantics.

## RequestResolve

```go
type RequestResolve[P, L any] func(context.Context, RequestEvent[P, L], ...ResolveOptions) (*http.Response, error)
```

RequestResolve forwards a value event to the next hook or application.

## Resolve

```go
type Resolve func(ctx context.Context, options ...ResolveOptions) (*http.Response, error)
```

Resolve is kit's `resolve(event, opts)`: it runs the application for the request and returns its response. The response is streamed — headers arrive when the application commits them and the body as it writes it, so wrapping it never waits for the whole body and closing it cancels the application. Resolve may be called once per request, with at most one [ResolveOptions](#ResolveOptions).

Pass the ctx the middleware was given, or one derived from it, so the application sees the same event and locals.

## ResolveOptions

```go
type ResolveOptions struct {
	// TransformPageChunk rewrites the assembled document exactly once, with
	// done true, before a finite document's ETag is computed. Values a page
	// streams afterwards are appended as they are and never transformed. It
	// applies to rendered pages, their error pages and the `ssr = false` shell,
	// and to nothing else: data, endpoints and remote responses are untouched.
	// Chunks are not required to stay well-formed — but removing comments can
	// break Svelte's hydration. The default is the identity.
	TransformPageChunk func(ctx context.Context, html string, done bool) (string, error)
	// FilterSerializedResponseHeaders decides which headers of a universal
	// load's `fetch` response the hydration data carries. Without it the
	// renderer's own [SSROptions.FilterSerializedResponseHeaders] applies, and
	// without that none are carried.
	FilterSerializedResponseHeaders func(name, value string) bool
	// Preload decides which files a document preloads. Without it JavaScript and
	// CSS are preloaded and fonts are not. Stylesheets are linked whatever it
	// answers.
	Preload func(input PreloadInput) bool
}
```

ResolveOptions is kit's `ResolveOptions` (`exports/hooks/public.d.ts`): how this one request's page document is rendered. Every field is optional, and an option is request-local — it never changes the renderer other requests share. Under [Sequence](#Middleware-Sequence) the options of nested middleware compose the way kit's `sequence` composes them.

## HookValues

```go
func HookValues(event *Event) (string, map[string]any)
```

HookValues returns the already converted matched route snapshot. It never invokes a matcher and is separate from remote caller permissions.
