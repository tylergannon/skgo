---
title: "skgo: endpoints"
summary: "+server routes in Go: GET, POST and the other method markers, Fallback and NewResponse."
---

The Go side of a `+server.ts` file. A `server.go` in a route directory holds
one Go function per HTTP method, marked with `GET`, `POST`, `PUT`, `PATCH`,
`DELETE`, `HEAD`, `OPTIONS` or `QUERY`, plus an optional `Fallback` for every
other method. Each handler is an ordinary `net/http` handler signature; the
server mounts it at the route Kit would have mounted the TypeScript one. Kit's
guide is [Routing](routing#server).

`NewResponse` builds an `*http.Response` for the composition points that
return one instead of writing to a `ResponseWriter`, such as hooks.

## GET

```go
func GET(fn http.HandlerFunc) Marker
```

GET declares fn as the handler for GET requests to the route it is written in. Write it beside the handler, in a file named `server.go` in the route's own directory:

```go
func get(w http.ResponseWriter, r *http.Request) { ... }

var _ = skgo.GET(get)
```

`skgo generate` emits the `+server.ts` kit compiles — one export per marker, named as kit names them — and the Go registration that answers the route.

The handler is an ordinary net/http handler: it owns the status, the headers and the body, and skgo does not encode, wrap or reinterpret any of them. What the request carries beyond the standard library is reachable with skgo.EventFrom(r.Context()) — the route id, the route parameters, and the application's selected locals through its generated LocalsFrom helper.

## POST

```go
func POST(fn http.HandlerFunc) Marker
```

POST declares fn as the handler for POST requests to this route. See GET.

## PUT

```go
func PUT(fn http.HandlerFunc) Marker
```

PUT declares fn as the handler for PUT requests to this route. See GET.

## PATCH

```go
func PATCH(fn http.HandlerFunc) Marker
```

PATCH declares fn as the handler for PATCH requests to this route. See GET.

## DELETE

```go
func DELETE(fn http.HandlerFunc) Marker
```

DELETE declares fn as the handler for DELETE requests to this route. See GET.

## HEAD

```go
func HEAD(fn http.HandlerFunc) Marker
```

HEAD declares fn as the handler for HEAD requests to this route.

It is rarely needed: kit answers HEAD with the GET handler when the module exports no HEAD of its own, and skgo does too. Declare it only to answer HEAD differently from GET.

## OPTIONS

```go
func OPTIONS(fn http.HandlerFunc) Marker
```

OPTIONS declares fn as the handler for OPTIONS requests to this route.

Kit synthesizes nothing here: an endpoint route answers OPTIONS only if its module exports one, and otherwise gets the same 405 any other undeclared method gets (`render_endpoint`). skgo does the same.

## QUERY

```go
func QUERY(fn http.HandlerFunc) Marker
```

QUERY declares fn as the handler for HTTP QUERY requests to this route. It is the HTTP method of that name, which kit lists among its endpoint methods; it has nothing to do with skgo.Query, which declares a remote function.

## Fallback

```go
func Fallback(fn http.HandlerFunc) Marker
```

Fallback declares fn as the handler for every method this route does not declare a handler for. It is kit's `fallback` export, and like kit's it answers before the 405 does.

## NewResponse

```go
func NewResponse(status int, header http.Header, body io.Reader) *http.Response
```

NewResponse builds a response a middleware answers with. A nil header or body is empty.

## Endpoint

```go
type Endpoint struct {
	// contains filtered or unexported fields
}
```

Endpoint is one registered method of one server route. Generated code builds these with NewEndpoint; application code declares the handlers and marks them with GET, POST and the rest.

### Method

```go
func (e *Endpoint) Method() string
```

Method is the HTTP method this endpoint answers, or "\*" for a fallback.

### RouteID

```go
func (e *Endpoint) RouteID() string
```

RouteID is kit's id for the route this endpoint answers.

## Endpoints

```go
type Endpoints struct {
	// contains filtered or unexported fields
}
```

Endpoints is a registry of server routes and the http.Handler that answers them.

It also issues kit's trailing-slash redirect, because kit issues it for every route it matches — page or endpoint — before it decides which of the two answers (`runtime/server/respond.js`). Doing it anywhere else would leave endpoint routes out of it or duplicate the route table.

### Intercept

```go
func (es *Endpoints) Intercept(next http.Handler) http.Handler
```

Intercept returns a handler that answers this app's server routes and passes everything else to next.

It must sit in front of the static handler so declared server routes reach their Go handlers.

### SetErrorTemplate

```go
func (es *Endpoints) SetErrorTemplate(template string)
```

SetErrorTemplate installs Kit's error.html after the renderer has read it. The renderer is built after the endpoint registry because its internal fetch dispatches through that registry.

## EndpointConfig

```go
type EndpointConfig struct {
	// AppDir is kit's appDir; empty means "_app".
	AppDir string
	// Base is kit's paths.base, without a trailing slash.
	Base string
	// Origin is the app's configured origin. It is the "self" origin of kit's
	// CSRF check on form-shaped mutations; leaving it empty turns that check
	// off.
	Origin string
	// TrustedOrigins are the extra origins kit's `csrf.trustedOrigins` allows
	// to submit forms to this app.
	TrustedOrigins []string
	// OnPanic is called when an endpoint handler panics, with the route id and
	// method, the recovered value, and the stack. Leaving it nil logs the same
	// three things, because a panicking handler that reports nowhere is a bug
	// that cannot be found.
	OnPanic func(routeID, method string, value any, stack []byte)
	// HandleError shapes a fatal endpoint error as Kit's App.Error.
	HandleError HandleError
	// ErrorTemplate is Kit's error.html, used for fatal endpoint errors when
	// the request accepts HTML. The application sets it from its renderer.
	ErrorTemplate string
	// Matchers contains the app route matchers.
	Matchers map[string]ParamMatcher
	// Routes is the route table, from the manifest.
	Routes []ManifestRoute
	// Prerendered is the build's list of prerendered pathnames. Kit's adapters
	// answer a GET of one from its file before any dynamic route is consulted,
	// so a server route that would also match it does not run for it.
	Prerendered []string
	// PrerenderedEndpoints records compiled methods for fully prerendered
	// endpoint routes. They participate in drift checks, never dynamic routing.
	PrerenderedEndpoints map[string][]string
	// contains filtered or unexported fields
}
```

EndpointConfig describes the app whose server routes a registry answers. Everything but Origin comes from the built manifest.

## NewEndpoint

```go
func NewEndpoint(routeID, method string, fn http.HandlerFunc) *Endpoint
```

NewEndpoint registers one method of one server route. Generated code calls this; application code uses GET, POST and the rest.

## NewEndpoints

```go
func NewEndpoints(cfg EndpointConfig, eps ...*Endpoint) (*Endpoints, error)
```

NewEndpoints builds a registry. Every endpoint the built frontend declares must be registered and no other, for the same reason a remote function or a server load must: kit's route table is what decides that a request to this path is an endpoint request at all, so a disagreement is a route nobody answers or a handler nothing reaches.
