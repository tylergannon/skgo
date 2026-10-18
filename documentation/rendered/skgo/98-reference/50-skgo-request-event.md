---
title: "skgo: request event"
summary: "Event and EventFrom, the request's URL, params, cookies, locals, fetch and tracing."
---

Kit's `RequestEvent`, in Go. Every load, action, remote function and hook
runs with a `context.Context` that carries the event; `EventFrom` returns it.
The event is where the URL, the route id and params, the cookies, the
request-scoped `fetch`, the client address and the response headers live.
Locals are typed: `RequestLocals` returns the application's own struct, the
one named by `--locals-type` at generation.

`RequestEvent` is the typed form the generated code hands to a handler that
declared route parameters and locals; `Event` is the untyped core both share.

## Event

```go
type Event struct {
	// Tracing provides the framework root and current OpenTelemetry spans.
	Tracing Tracing
	// contains filtered or unexported fields
}
```

Event is the per-call handle a remote function reaches through its context. It is skgo's equivalent of the `RequestEvent` kit hands to a remote function through `getRequestEvent()`, narrowed the same way kit narrows it.

Kit derives a restricted event for every remote function (`runtime/app/server/remote/shared.js`): `setHeaders` throws, and inside a `query`, `query.live`, `query.batch` or `prerender` so do `cookies.set` and `cookies.delete`. skgo enforces the same rules; the difference is that a Go handler returns the refusal rather than throwing it.

### EventFrom

```go
func EventFrom(ctx context.Context) *Event
```

EventFrom returns the request event bound to ctx, mirroring Kit's `getRequestEvent()`. Remote functions, server loads, page actions, server routes and `handle` all have an event. With no event it returns nil, and every method below is safe on a nil \*Event.

### ClientAddress

```go
func (e *Event) ClientAddress() (string, error)
```

ClientAddress returns the hosting boundary's visitor address. The provider's first value or error is retained for the request, derived events and internal fetches. Build-only and requestless contexts have no address and return an error.

### Cookie

```go
func (e *Event) Cookie(name string) (string, bool)
```

Cookie returns the value of a request cookie. A cookie written earlier in the same call shadows the one the browser sent, and a cookie deleted earlier reads as absent — the same precedence kit's `cookies.get` applies.

### DeleteCookie

```go
func (e *Event) DeleteCookie(name string, opts CookieOptions) error
```

DeleteCookie removes a cookie. Kit implements `cookies.delete` as a `set` with an empty value and `maxAge: 0`, and so does this; the options must name the same Path and Domain the cookie was set with or the browser keeps it.

### Depends

```go
func (e *Event) Depends(deps ...string)
```

Depends declares that this load's result goes stale when any of these identifiers is invalidated from the browser with `invalidate(...)`.

An identifier is a URL: a bare path is resolved against the page's own address, and a custom scheme such as "app:account" travels as written. It is the string the client matches on, so it has to be the same string on both sides.

Unlike everything else, a dependency is recorded even inside Untrack: kit turns off the implicit tracking there, never the explicit call.

### Fetch

```go
func (e *Event) Fetch(ctx context.Context, request *http.Request, opts ...FetchOption) (*http.Response, error)
```

Fetch is skgo's `event.fetch`. A relative URL resolves against the event's URL. A request for this app's own routes is answered in-process with fresh locals; the page's cookies, Authorization, Accept-Language and Origin are inherited as Kit inherits them, unless WithFetchCredentials(FetchOmit). Any other request goes out over HTTP, and carries cookies only to this host and its subdomains. The Set-Cookie headers of an in-process answer are taken as cookies this request wrote, so the next fetch sends them and the response to the visitor carries them, exactly as Kit's `event.fetch` does with them; an answer that came from HandleFetch or over HTTP is not harvested.

The response is returned once the answering handler has committed its status and headers, and its body is read as the handler writes it, so a handler that streams is seen streaming. As with net/http, read the body to the end or Close it: Close, or cancelling ctx, ends the handler's context, and a read that outlives a cancelled handler fails with the context's error rather than ending as if the answer were complete.

ctx and request's own context both cancel it. HandleFetch sees ctx's values, so EventFrom(ctx) and the locals are the originating request's. Fetching records no dependency: a server load that fetches is not re-run by `invalidate`, exactly as in Kit.

### IsDataRequest

```go
func (e *Event) IsDataRequest() bool
```

IsDataRequest reports a client \_\_data.json request in every request context.

### IsRemoteRequest

```go
func (e *Event) IsRemoteRequest() bool
```

IsRemoteRequest reports an incoming remote transport call. Calling a query during an ordinary page render does not change the kind of that request.

### IsSubRequest

```go
func (e *Event) IsSubRequest() bool
```

IsSubRequest reports an in-process fetch request, rather than a direct nested function invocation within the same request.

### Param

```go
func (e *Event) Param(name string) string
```

Param returns a route parameter, and records that this load depends on it.

### Params

```go
func (e *Event) Params() map[string]string
```

Params is a copy of the matched route's parameters, or nil when no route matched. It is only available on the event a Middleware receives; inside a query kit forbids reading it, and so does this.

### Request

```go
func (e *Event) Request() *http.Request
```

Request is the HTTP request that carried this event, or nil when the event has no request.

Inside a remote function, its URL is the remote-function endpoint — `/_app/remote/<hash>/<name>` — not the page the user is looking at. Kit is stricter still: reading `event.url`, `event.params` or `event.route` inside a query throws, because a query's result is cached by its argument and would go stale, so the client would keep showing an answer computed for a page it has since left. Anything a remote function needs about the page belongs in its argument.

### RouteID

```go
func (e *Event) RouteID() string
```

RouteID is the id of the route being served, e.g. "/account/orders", and records that this load depends on which route it is.

### SearchParam

```go
func (e *Event) SearchParam(name string) (string, bool)
```

SearchParam returns a query parameter, and records that this load depends on that one parameter and no other.

### SetCookie

```go
func (e *Event) SetCookie(name, value string, opts CookieOptions) error
```

SetCookie writes a cookie on the response.

Commands, forms, server loads, page actions and `handle` may set cookies. Queries and prerender functions cannot, because a query's result is cached by its argument and replayed from that cache, so a cookie it wrote would be written once and then silently skipped. Server routes write cookies on their `http.ResponseWriter`. SetCookie also returns an error when called from a sealed `handle` response.

### SetHeader

```go
func (e *Event) SetHeader(name, value string) error
```

SetHeader sets a header on the response for the current page or `__data.json` request. It is Kit's `setHeaders` and works in server loads, page actions and `handle`. Cookies go through SetCookie, and a header may not be set twice except `server-timing`, which accumulates. In a remote function or server route it returns an error; a server route sets headers on its `http.ResponseWriter`.

### URL

```go
func (e *Event) URL() *url.URL
```

URL is the page the visitor asked for — not the `__data.json` address the browser used, and with kit's two private query parameters already removed.

Reading it makes this load depend on the whole URL, so kit's client re-runs it on any navigation that changes the path or the query. When only one parameter matters, SearchParam is cheaper: it records only that parameter, exactly as kit's own `url.searchParams.get` does.

### Untrack

```go
func (e *Event) Untrack(fn func())
```

Untrack runs fn without recording what it reads, so a load can consult the URL or a parameter without making its result depend on it.

## RequestEvent

```go
type RequestEvent[Params, Locals any] struct {
	*Event
	Params Params
	Locals *Locals
}
```

RequestEvent pairs a request with generated typed params and application locals. Page and layout loads use their generated RouteParams and LayoutParams domains. Commands and forms use the generated application-wide params.Params.

### Context

```go
func (e RequestEvent[P, L]) Context() context.Context
```

Context carries cancellation and the existing request-scoped helpers.

## RequestLocals

```go
func RequestLocals[L any](ctx context.Context) *L
```

RequestLocals returns the application's selected object, or nil outside a bound request. A mismatched type is a programming error, never a new store.

## CookieOptions

```go
type CookieOptions struct {
	// Path defaults to "/". A non-empty path must start with "/"; SetCookie
	// returns an error otherwise.
	Path string
	// Domain is empty by default, so the cookie has no Domain attribute and is
	// host-only.
	Domain string
	// MaxAge is the cookie's lifetime in seconds. Zero omits the attribute,
	// making a session cookie, exactly as kit does.
	MaxAge int
	// Expires, when non-zero, sets an absolute expiry.
	Expires time.Time
	// SameSite defaults to http.SameSiteLaxMode.
	SameSite http.SameSite
	// HTTPOnly defaults to true. Set it to a pointer to false to opt out.
	HTTPOnly *bool
	// Secure defaults to true unless the app's origin is http://localhost.
	// Set it to a pointer to false to opt out.
	Secure *bool
}
```

CookieOptions mirrors the options kit's `cookies.set` accepts. A zero field takes kit's default: Path "/", HttpOnly, SameSite=Lax, and Secure everywhere except an app served over plain HTTP from localhost.

## File

```go
type File = formdata.File
```

File is a file uploaded through a form. Give a form's argument struct a File field and the bytes the visitor chose arrive in it.

## Fetch

```go
type Fetch func(request *http.Request) (*http.Response, error)
```

Fetch dispatches a server-side fetch. A HandleFetch hook may change the request or return its own response without calling the dispatcher.

## FetchConfig

```go
type FetchConfig struct {
	// Origin is the app's configured origin, used to complete the URL of an
	// event that does not carry one. Empty falls back to the request's host.
	Origin string
	// Base is kit's paths.base. A same-origin URL outside it is not one of this
	// app's routes and goes over the network, as in Kit.
	Base string
	// Handler answers requests for this app's own routes in-process, with no
	// socket. Pass the handler the app serves with, from Handle inwards: every
	// page, `__data.json`, remote function, server route, asset and prerendered
	// file a fetch can name, as the real server would answer it. It sees every
	// subrequest with fresh locals and a fresh event. The one Intercept returns
	// is not it — that mounts this very fetcher — so assemble the handler first
	// and hand it over by reference. Nil refuses same-origin fetches.
	Handler http.Handler
	// CarryContext preserves an owning host's execution lifetime on an internal
	// fetch. It receives a fresh subrequest context and the originating context;
	// return the fresh context with only that host token added. Request events,
	// locals and arbitrary request values must remain isolated.
	CarryContext func(subrequest, originating context.Context) context.Context
	// HandleFetch intercepts every fetch before credential inheritance.
	HandleFetch HandleFetch
	// Client sends requests that leave the app. Nil uses http.DefaultClient.
	Client *http.Client
	// Prerendered is the build manifest's Prerendered list. A fetch of a path
	// on it takes nothing from the page's request, as in Kit, and nothing a
	// dynamic route might match is consulted in its place.
	Prerendered []string
}
```

FetchConfig configures Event.Fetch, skgo's `event.fetch`: the request a load, action, server route, Handle hook or error hook makes while serving a request. The rules are Kit's (`runtime/server/fetch.js`).

### Intercept

```go
func (c FetchConfig) Intercept(next http.Handler) http.Handler
```

Intercept makes Event.Fetch available to everything beneath next. Mount it outermost, so Handle's own event can fetch as well.

## FetchCredentials

```go
type FetchCredentials string
```

FetchCredentials is the `credentials` option of a Kit fetch.

### FetchSameOrigin, FetchInclude, FetchOmit

```go
const (
	// FetchSameOrigin is the default: the request inherits the page's cookies
	// and Authorization header when it targets this app.
	FetchSameOrigin FetchCredentials = "same-origin"
	// FetchInclude behaves like FetchSameOrigin on the server, as in Kit.
	FetchInclude FetchCredentials = "include"
	// FetchOmit inherits nothing from the originating request.
	FetchOmit FetchCredentials = "omit"
)
```

## FetchMode

```go
type FetchMode string
```

FetchMode is the `mode` option of a Kit fetch. Only "no-cors" changes what the server sends: it drops Origin from a cross-origin GET or HEAD.

### FetchCORS, FetchNoCORS

```go
const (
	FetchCORS   FetchMode = "cors"
	FetchNoCORS FetchMode = "no-cors"
)
```

## FetchOption

```go
type FetchOption func(*fetchOptions)
```

FetchOption adjusts one Event.Fetch call.

### WithFetchCredentials

```go
func WithFetchCredentials(c FetchCredentials) FetchOption
```

WithFetchCredentials sets the request's `credentials` mode.

### WithFetchMode

```go
func WithFetchMode(m FetchMode) FetchOption
```

WithFetchMode sets the request's `mode`.

## ParamMatcher

```go
type ParamMatcher func(string) (any, bool)
```

ParamMatcher is a generated adapter for a plain Go func(string) (T, bool). Only the bool decides whether a route candidate is accepted.

## CallerMatchers

```go
type CallerMatchers map[string]ParamMatcher
```

CallerMatchers is emitted alongside remote bindings, independently of loads. The manifest owns candidate ordering; only the Go matchers convert captures.

## CallerRoute

```go
type CallerRoute struct {
	Params    []ManifestParam
	NewParams func(map[string]any) (any, error)
}
```

CallerRoute is generated from Kit's complete caller metadata, including routes without Go loads. NewParams must construct the shared Params for it.

## CallerRoutes

```go
type CallerRoutes map[string]CallerRoute
```

## Tracing

```go
type Tracing struct {
	Enabled bool
	Root    trace.Span
	Current trace.Span
}
```

Tracing mirrors SvelteKit's event.tracing. Root identifies the outermost handle span; Current identifies the operation invoking the application. Disabled tracing exposes non-recording spans, so annotations are always safe. Configure the application's OpenTelemetry provider before serving requests.
