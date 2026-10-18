---
title: "skgo: loads"
summary: "The server-load API: Load, Parent, Deferred and the parameter helpers."
---

The Go side of a `+page.server.ts` or `+layout.server.ts` load. You write a
function and mark it with `Load`; `skgo generate` writes the TypeScript stub
Kit compiles and the registration the server mounts. Kit's guide is
[Loading data](load); the Go side follows it.

A load receives a `context.Context` carrying the request. `EventFrom` on the
[request event](skgo-request-event) page gets you the URL, params, cookies and
`fetch`. `Parent` reads the parent layout's data, `Async` starts work that the
page streams in after the first response, and the `LoadParam` helpers read a
route parameter with the type its matcher produced.

## Load

```go
func Load[P, L, Out any](fn func(RequestEvent[P, L]) (Out, error)) Marker
```

Load declares fn as a SvelteKit server load. Write it beside the function, in a file named `page.server.go` or `layout.server.go` in the route's own directory:

```go
func load(event PageRequestEvent) (Data, error) { ... }

var _ = skgo.Load(load)
```

`skgo generate` emits the `+page.server.ts` or `+layout.server.ts` kit compiles — the file name says which — and the Go registration that answers the route's `__data.json`. PageRequestEvent and LayoutRequestEvent are the generated aliases with RouteParams and LayoutParams respectively; both bind the application Locals.

Out must be a struct: kit requires a load to return a plain object, and refuses anything else.

## Parent

```go
func Parent[T any](ctx context.Context) (T, error)
```

Parent returns what this route's outer loads produced, decoded into T.

It is kit's `await parent()`: a shallow merge of every preceding node's data, outermost first, so a deeper layout's field wins. Calling it records that this load depends on its parents, which is what makes kit's client re-run it when one of them re-runs.

A load whose parent failed fails with the same error, and one whose parent redirected redirects — the visitor never reaches a page whose layout turned them away.

## Deferred

```go
type Deferred[T any] struct {
	// contains filtered or unexported fields
}
```

Deferred is a value a load has promised but does not have yet. It reaches the browser as a JavaScript `Promise`: the page renders everything else immediately, and the value arrives on the same response as an extra line, without a second request.

It is what kit's own loads do by putting a promise in the returned object, and it may sit wherever kit lets a promise sit: anywhere in the value a load returns, at any depth, including inside a value another Deferred promised. Kit reaches promises with a devalue reducer and its client reads them back with a devalue reviver, and both walk the whole tree.

### MarshalJSON

```go
func (d Deferred[T]) MarshalJSON() ([]byte, error)
```

MarshalJSON writes null. A Deferred has no JSON form: the serializer replaces it with the promise placeholder kit's client understands before the value reaches the wire, and this exists only so that encoding a load's result never fails on the field.

## Async

```go
func Async[T any](ctx context.Context, fn func(context.Context) (T, error)) Deferred[T]
```

Async starts fn and returns a Deferred for its result. fn runs on its own goroutine, so the load can return before fn has finished; ctx is the load's context and stays alive until the response is complete.

```go
func load(event PageRequestEvent) (Data, error) {
	ctx := event.Context()
	return Data{
		Total:  store.Total(),
		Orders: skgo.Async(ctx, store.Orders),
	}, nil
}
```

## Resolved

```go
func Resolved[T any](value T) Deferred[T]
```

Resolved is a Deferred that already holds its value. It exists so a load can return the same shape whether or not the work was worth deferring.

## NewLoads

```go
func NewLoads(cfg LoadConfig, loads ...*ServerLoad) (*Loads, error)
```

NewLoads builds a registry. Every load the built frontend declares must be registered and no other, for the same reason a remote function must: kit decides whether its client asks for `__data.json` from the modules it compiled, so a disagreement is a page whose data nobody answers.

## NewServerLoad

```go
func NewServerLoad(spec LoadSpec) *ServerLoad
```

NewServerLoad builds a load registration from a generated spec.

## LoadParamValue

```go
func LoadParamValue[T any](e *Event, name string) T
```

LoadParamValue populates a generated RouteParams field without recording a dependency. Matching has already converted the value to the matcher's type.

## OptionalLoadParamValue

```go
func OptionalLoadParamValue[T any](e *Event, name string) *T
```

OptionalLoadParamValue populates an optional field without tracking, preserving the distinction between absence and an accepted zero, false or empty value.

## LoadRouteIDValue

```go
func LoadRouteIDValue(e *Event) string
```

LoadRouteIDValue selects a generated layout alternative without recording a route dependency. A layout's public RouteID still tracks the matched page.

## TrackLoadParam

```go
func TrackLoadParam(e *Event, name string)
```

TrackLoadParam records a generated accessor's read on this load invocation.

## Prerender

```go
func Prerender(fn any, options ...PrerenderOptions) Marker
```

Prerender declares a remote function whose result Kit writes during its build. Its Go body runs through the adapter's build-time Go bridge. Inputs, when supplied, names a local `func() ([]In, error)` producer where In is the body's existing argument type.

## PreloadInput

```go
type PreloadInput struct {
	Type     string
	Path     string
	Filename string
}
```

PreloadInput names a file ResolveOptions.Preload is asked about. Type is "js", "css", "asset" or "font"; Filename is only set for a font, and is the source file's pathname relative to the project root.

## PrerenderLoadInput

```go
type PrerenderLoadInput struct {
	Module       string            `json:"module"`
	URL          string            `json:"url"`
	RouteID      string            `json:"routeId"`
	Params       map[string]string `json:"params"`
	Headers      http.Header       `json:"headers"`
	Parent       map[string]any    `json:"parent"`
	ParentData   string            `json:"parentData,omitempty"`
	RoutePattern string            `json:"routePattern,omitempty"`
	RouteParams  []ManifestParam   `json:"routeParams,omitempty"`
	RoutePath    string            `json:"routePath,omitempty"`
}
```

PrerenderLoadInput carries one Kit build-time server-load invocation from the adapter's prerender process to the application's compiled Go load registry.

## LoadConfig

```go
type LoadConfig struct {
	// AppDir is kit's appDir; empty means "_app".
	AppDir string
	// Base is kit's paths.base, without a trailing slash.
	Base string
	// Version, when non-empty, is sent as the `x-sveltekit-version` response
	// header. It must equal the version baked into the client — a different
	// value makes the client believe a new deployment has landed and reload —
	// so leave it empty whenever the client did not come from this build.
	Version string
	// Origin is the app's configured origin. A load's `depends("/some/path")`
	// is resolved against it, exactly as kit resolves it against the request
	// URL, so that the identifier the client matches on is the same one.
	Origin string
	// TrustedOrigins are the extra origins Kit permits for form-shaped page
	// submissions. They follow the same CSRF rule as endpoint submissions.
	TrustedOrigins []string
	// Transport is the app's `transport` hook: the Go half of the encode/decode
	// pairs `src/hooks.ts` declares. It is optional; without it a custom type
	// goes out as whatever encoding/json makes of it, which is a plain object
	// with no methods on the other side. See the Transport type.
	Transport Transport
	// HandleError is the app's `handleError` hook: the one place it decides
	// what a failed load's visitor is told beyond status and message. The loads
	// registry owns it because it answers the error on `__data.json`; the page
	// renderer reads the same hook from this registry when the same load fails
	// while rendering a document. It is optional; see the HandleError type.
	HandleError HandleError
	// OnPanic is called when a load panics, with the load's `+*.server.ts`
	// module, the recovered value, and the stack. The client is told nothing
	// but an opaque 500, so this is the only record the panic leaves; leaving
	// it nil logs the same three things to the standard logger, because a
	// panicking handler that reports nowhere is a bug that cannot be found.
	//
	// The `handle` hook has its own OnPanic, on HandleConfig: it is not a load
	// concern and does not run through this registry. See Handle.
	OnPanic func(id string, value any, stack []byte)
	// Matchers contains the app route matchers, including routes without loads.
	Matchers map[string]ParamMatcher
	// Nodes and Routes come from the manifest.
	Nodes       []string
	LoadModules []string
	Prerendered []string
	Routes      []ManifestRoute
	// contains filtered or unexported fields
}
```

LoadConfig describes the app whose loads a registry answers. Manifest.LoadConfig fills AppDir, Base, Version, TrustedOrigins, Nodes, LoadModules, Prerendered and Routes. The caller supplies Origin, Transport, HandleError and Matchers.

## LoadSpec

```go
type LoadSpec struct {
	// Module is the vite-root-relative path of the `+*.server.ts` kit
	// compiled, which is the key kit itself records for the node.
	Module string
	// Run is the generated closure that calls the app's load.
	//
	// It answers with the load's own Go value rather than a devalue tree, and
	// that is the one place a generated encoder does not appear. A load's
	// result may hold a skgo.Deferred — kit lets a promise sit anywhere in the
	// object a load returns, and each one becomes a chunk on the response as
	// it settles — and there is no Go type polytype could project to
	// `Promise<T>`, so nothing it could generate would describe the shape.
	// The value is encoded where a promise can still be recognised: the
	// serializer in data.go, which is also the only place the app's transport
	// hook is known.
	Run func(ctx context.Context) (any, error)
	// Matchers are the app's Go parameter matchers, emitted by generation.
	Matchers map[string]ParamMatcher
}
```

LoadSpec is one generated load registration. `skgo generate` writes these; an application declares its loads with Load and writes none of this by hand.

## Loads

```go
type Loads struct {
	// contains filtered or unexported fields
}
```

Loads is a registry of server loads and the http.Handler that answers `__data.json`.

### Intercept

```go
func (ls *Loads) Intercept(next http.Handler) http.Handler
```

Intercept returns a handler that answers `__data.json` itself and passes everything else to next.

It must sit in front of the static handler so that `__data.json` never reaches it, and in front of the remote registry too, since a `__data` suffix and a remote call never share a path. Running the app's `handle` hook is not this registry's business; see Handle, which mounts outermost over this and everything else.

### ServeHTTP

```go
func (ls *Loads) ServeHTTP(w http.ResponseWriter, r *http.Request)
```

ServeHTTP answers one `__data.json` request.

## ServerLoad

```go
type ServerLoad struct {
	// contains filtered or unexported fields
}
```

ServerLoad is one registered server load. Generated code builds these with NewLoad; application code declares the functions and marks them with Load.

### Module

```go
func (l *ServerLoad) Module() string
```

Module is the vite-root-relative path of the `+*.server.ts` this load answers for.
