---
title: "skgo: remote functions"
summary: "Query, Command, Form, LiveQuery and BatchQuery, with refresh and caller helpers."
---

The Go side of a `.remote.ts` module. A Go function in a `*.remote.go` file
marked with `Query`, `Command`, `Form`, `LiveQuery` or `BatchQuery` becomes an
export of the generated `.remote.ts` module next to it, with the argument and
result types Kit's client expects. Kit's guide is
[Remote functions](remote-functions).

A command that changes data tells the client which queries to refresh with
`Refresh` and its relatives; `Requested` inspects which queries the caller
declared it depends on. `RequireArgument` and `RefuseArgument` are the
guards a query uses to say whether it takes an argument when the generated
signature cannot. The `Call`, `Kind` and `Remote` types are what the
generated registration hands the server; application code rarely touches
them.

## Query

```go
func Query(fn any) Marker
```

Query declares fn as a SvelteKit `query`. Write it beside the function, in a file named `*.remote.go`:

```go
func getTodos(ctx context.Context) ([]Todo, error) { ... }

var _ = skgo.Query(getTodos)
```

A query that takes an argument declares it as an ordinary second parameter:

```go
func getTodo(ctx context.Context, id string) (Todo, error) { ... }
```

Both are ordinary Go functions, which is kit's own rule rather than a liberty taken here: `query(fn)` accepts `(arg?) => Output`, so the argument is optional and there is no placeholder to name in its place.

fn is `any` because the marker checks nothing. `skgo generate` reads the function's real signature through go/types — it has to, since it projects the argument and the result to TypeScript — and reports a shape it cannot publish against the declaration's own position. What checks the function is the generated closure that calls it, which names the argument's type and the result's and is compiled beside the codecs polytype emitted for exactly those two types.

`skgo generate` emits the sibling `.remote.ts` kit compiles and the Go registration that answers the calls. The request is reachable with skgo.EventFrom(ctx); a query may read cookies but not write them, which is the restriction kit places on its own queries.

## Command

```go
func Command(fn any) Marker
```

Command declares fn as a SvelteKit `command`. A command's event may write cookies; kit allows that in commands and forms and nowhere else.

It takes context.Context, params.RequestEvent, and an optional separate input. The context preserves helpers while restricting caller state. The scanner validates the shared Params identity and reports old ctx shapes.

## Form

```go
func Form(fn any) Marker
```

Form declares fn as a SvelteKit `form`. Write it beside the function, in a file named `*.remote.go`:

```go
func subscribe(ctx context.Context, event params.RequestEvent, arg Signup) (Result, error) { ... }

var _ = skgo.Form(subscribe)
```

A form's event may write cookies, which kit permits in forms and commands and nowhere else. Return a \*Invalid to put messages on specific fields.

## LiveQuery

```go
func LiveQuery(fn any) Marker
```

LiveQuery declares fn as a SvelteKit `query.live`. fn pushes values with yield and returns when the subscription ends; its context is cancelled when the client disconnects:

```go
func watchCount(ctx context.Context, yield func(int) error) error { ... }
```

An argument, when there is one, sits between the two. See Query for why fn is any.

## BatchQuery

```go
func BatchQuery(fn any) Marker
```

BatchQuery declares fn as a SvelteKit `query.batch`. fn takes every argument collected in one macrotask and answers them all in one call:

```go
func getQuotes(ctx context.Context, symbols []string) ([]Quote, error) { ... }
```

The page still calls it one argument at a time — `getQuotes('SKGO')` — and kit's client collects those calls into a single POST, exactly as kit's own `query.batch` does. fn must answer with one result per argument, in the same order.

A batch query always takes an argument: kit's `batch(fn)` with no validator refuses any call that passes one, which leaves nothing to batch. See Query for why fn is any.

## RequestedQuery

```go
type RequestedQuery[In any] struct {
	// Arg is the argument the client's instance was called with, decoded into
	// the query's own parameter type. A handler that only wants to refresh
	// some of what was asked for reads this to decide.
	Arg In
	// contains filtered or unexported fields
}
```

RequestedQuery is one instance of a query that the client asked the current command or form to refresh — one entry of kit's `requested(...)` iterable.

It is an instance rather than a function: the browser caches a query under the argument it was called with, so `getTodo("t1")` and `getTodo("t2")` are two of these, and refreshing one leaves the other alone.

### Requested

```go
func Requested[In, Out any](ctx context.Context, fn func(context.Context, In) (Out, error), limit int) ([]RequestedQuery[In], error)
```

Requested returns the instances of fn that the client asked this command or form to refresh, at most limit of them, and refreshes none of them: calling Refresh on an entry is what accepts it.

Use it when the handler wants a say in which instances it will run:

```go
requests, err := skgo.Requested(ctx, getTodo, 4)
if err != nil {
	return todo, err
}
for _, request := range requests {
	if request.Arg == arg.ID {
		request.Refresh()
	}
}
```

When the handler will take everything the client asked for, RefreshRequested says exactly that in one line.

Anything the client asked for beyond limit is not returned and not run. It is failed instead, onto its own key, so the query the page is showing goes visibly into an error state rather than quietly stale — kit's choice, and the reason a page that outgrows its handler's limit shows up as a broken panel rather than as a bug report about data that stopped updating. An instance whose payload does not decode into In is failed the same way.

The error returned is about the call itself: a function that is not a registered query, a negative limit, or a call from somewhere that has no response for a refresh to ride back on. It is never about what a query goes on to do.

### Ignore

```go
func (q RequestedQuery[In]) Ignore()
```

Ignore explicitly declines this instance. Its current client-side value is left alone, and the instance's key is returned in Kit's `i` response field so the client knows the handler handled the request deliberately.

Kit 3.0.0-next.27 made this distinction load-bearing: a requested key that is neither refreshed, reconnected nor ignored makes the mutation fail.

### Refresh

```go
func (q RequestedQuery[In]) Refresh()
```

Refresh accepts this instance: the query is re-run after the handler returns and its new value rides back on the command's own response, exactly as it does for skgo.Refresh.

The key is the one the client sent, not one recomputed from Arg, so the value lands on the instance the browser is actually showing.

## Refresh

```go
func Refresh[In, Out any](ctx context.Context, fn func(context.Context, In) (Out, error), arg In) error
```

Refresh asks for a query to be re-run and its new value returned on the response the current command or form is already producing. The client applies it to the query instance keyed by exactly this argument, so nothing goes back for more: the browser sees the change in a single flight.

It names the Go function, not a string:

```go
func renameTodo(ctx context.Context, event params.RequestEvent, arg Rename) (businesslogic.Todo, error) {
	todo, ok := businesslogic.Default.Rename(arg.ID, arg.Text, signedIn(ctx))
	if !ok {
		return businesslogic.Todo{}, skgo.Errorf(404, "No todo with id %q", arg.ID)
	}
	return todo, skgo.Refresh(ctx, getTodo, arg.ID)
}
```

getTodo's own parameter type fixes what the second argument may be, so a refresh that names the wrong kind of argument does not compile.

The argument is the cache key, not a hint. Kit's client caches a query under the argument it was called with, and the key skgo computes here is the same string that client computed — kit's `create_remote_key(id, payload)` over `stringify_remote_arg`, both ported in internal/remotearg. Refreshing getTodo("t1") therefore leaves an open getTodo("t2") alone, which is the point: a command that changed one row must not silently republish another.

Naming a query no open page is showing is not an error. Kit's client parks the value under its key and hands it to the next component that asks for exactly that query and argument (`runtime/client/remote-functions/shared.svelte.js`), so the refresh is a pre-seed rather than a mistake.

The query does not run here. Kit stores the refresh and runs it after the handler body has finished, so it observes everything the command went on to change; skgo does the same, in the same request, on an event that shares the command's cookie jar. A refresh that fails does not fail the command: the error is reported on that query alone, exactly as kit reports it, and the command keeps its own answer.

The error returned here is about the call itself — a function that is not a registered query, or an argument that cannot be encoded — and never about what the query goes on to do.

## RefreshNoArg

```go
func RefreshNoArg[Out any](ctx context.Context, fn func(context.Context) (Out, error)) error
```

RefreshNoArg is Refresh for a query that takes no argument:

```go
return todo, skgo.RefreshNoArg(ctx, getTodos)
```

It exists because there is no argument to infer the query's parameter type from, and it is not a variant of the key: kit's client calls a no-argument query with `undefined`, whose payload is the empty string, so the key is the query's id and nothing after the slash. That is the same key the client stored the query under, which is what makes the value land on the open page.

## RefreshRequested

```go
func RefreshRequested[In, Out any](ctx context.Context, fn func(context.Context, In) (Out, error), limit int) error
```

RefreshRequested accepts up to limit of the instances of fn the client asked this command or form to refresh:

```go
func addTodo(ctx context.Context, event params.RequestEvent, text string) (businesslogic.Todo, error) {
	return businesslogic.Default.Add(text), skgo.RefreshRequested(ctx, getTodos, 1)
}
```

It is kit's `await requested(getTodos, 1).refreshAll()`, and it is the whole of what a handler has to write to honour the `updates(...)` its page asks for. A handler that never mentions a query will not run it however loudly the request asks.

## RefreshRequestedNoArg

```go
func RefreshRequestedNoArg[Out any](ctx context.Context, fn func(context.Context) (Out, error)) error
```

RefreshRequestedNoArg is RefreshRequested for a query that takes no argument:

```go
func addTodo(ctx context.Context, event params.RequestEvent, text string) (businesslogic.Todo, error) {
	return businesslogic.Default.Add(text), skgo.RefreshRequestedNoArg(ctx, getTodos)
}
```

It is the same gate — a handler that never names getTodos does not refresh it, however loudly the request asks — and it takes no limit because there is nothing left for one to bound: the client can only ever have asked for the one instance. See the package comment above.

Anything else the client sent under this query's id is not an instance of it. Kit's validator for a remote function declared without a schema refuses an argument outright, so a payload that carries one comes back refused on its own key, and so does a second copy of the instance itself.

There is no RequestedNoArg beside this. Requested exists so a handler can pick between instances by their arguments; a query with no argument has one instance and nothing to pick by, and a handler that wants a say writes this call under an `if`.

## IgnoreRequested

```go
func IgnoreRequested[In, Out any](ctx context.Context, fn func(context.Context, In) (Out, error), limit int) error
```

IgnoreRequested explicitly declines up to limit of the instances of fn the client asked this command or form to refresh. Kit returns those keys in `i`, leaving their current client-side values untouched. A requested key that is neither accepted nor ignored makes Kit 3.0.0-next.27 reject the mutation.

## IgnoreRequestedNoArg

```go
func IgnoreRequestedNoArg[Out any](ctx context.Context, fn func(context.Context) (Out, error)) error
```

IgnoreRequestedNoArg is IgnoreRequested for the sole instance of a query that takes no argument. It mirrors Kit's `requested(fn, 1).ignoreAll()`.

## ReconnectRequested

```go
func ReconnectRequested[In, Out any](ctx context.Context, fn func(context.Context, In, func(Out) error) error, limit int) error
```

ReconnectRequested is RefreshRequested for a live query, and it reconnects rather than refreshes because that is the only thing a live query can be told to do.

A live query's event is a snapshot of the request that opened its stream, so it can never see a cookie a later command wrote; kit's answer is to tear the stream down and open it again on the command's own request, which is why signing in reconnects the count rather than refreshing it.

## ReconnectRequestedNoArg

```go
func ReconnectRequestedNoArg[Out any](ctx context.Context, fn func(context.Context, func(Out) error) error) error
```

ReconnectRequestedNoArg is ReconnectRequested for a live query that takes no argument:

```go
return session, skgo.ReconnectRequestedNoArg(ctx, watchCount)
```

See RefreshRequestedNoArg for why there is no limit, and ReconnectRequested for why a live query reconnects rather than refreshes.

## RequireArgument

```go
func RequireArgument(call Call) error
```

RequireArgument is the other half of RefuseArgument: a function declared \*with\* an argument was called without one, so there is nothing for its decoder to admit and no zero value it would be honest to invent. Kit's schema-validated functions refuse the same call.

## RefuseArgument

```go
func RefuseArgument(call Call) error
```

RefuseArgument is the decode step for a function declared without an argument, and it is kit's own `create_validator` with no schema: such a function answers 400 to any argument at all. Generated code calls it so that the refusal is written in the same place every other decode is.

## RemoteCallerValues

```go
func RemoteCallerValues(e *Event) (string, map[string]any)
```

RemoteCallerValues is the generated command/form constructor's input. A query event cannot reconstruct a caller's shared Params through this helper.

## Marker

```go
type Marker struct{}
```

Marker is what the declaration helpers return. It carries nothing: Query, Command, LiveQuery and Form exist to be read by `skgo generate`.

## Call

```go
type Call struct {
	// Arg is the devalue value model tree the client sent — devalue's own
	// shapes: nil, bool, float64, string, []any, *devalue.Object, and for a
	// form submission the uploaded files kit's binary envelope carried.
	Arg any
	// Present reports that the client sent an argument at all. Kit's client
	// calls a function declared without one with `undefined`, whose payload is
	// the empty string, so absence is a value in its own right: kit's own
	// `create_validator` answers 400 to an argument a no-argument function was
	// never declared to take.
	Present bool
	// contains filtered or unexported fields
}
```

Call is one remote call as it reaches the generated closure that answers it.

Arg is the argument exactly as it came off the wire: the devalue value model tree kit's client sent, before anything has decided what Go type it is. Deciding that is the job of the strict decoder polytype generated for this function's own parameter type, and of nothing else. That decoder lives in the generated bindings package with the closure that calls it, which is why there is no type parameter here and no Go value of the app's own types passing through this struct.

### Transported

```go
func (c Call) Transported(v any) (any, error)
```

Transported encodes a result whose type can reach a type the app transports.

It is not a fallback but kit's own rule: a transported value has to reach devalue \*as itself\*, so that the reducer the `transport` hook installs still recognises it and writes `["Money", …]`. polytype's generated encoder would already have flattened it into an object, and the browser would be handed a plain object with no methods — which is the bug the hook exists to prevent. So a value that can hold one travels unflattened and devalue finishes it.

`skgo generate` emits this call for exactly the functions whose result type reaches a transported type, and a generated encoder for every other one.

## Kind

```go
type Kind int
```

Kind is which of kit's four remote-function kinds a registration publishes. `query.batch` is a fifth constant rather than a query because the call shape differs — one POST carrying every payload — even though everything downstream files its results in the browser's ordinary query cache.

### KindQuery, …

```go
const (
	KindQuery Kind = iota
	KindCommand
	KindLive
	KindBatch
	KindForm
	KindPrerender
)
```

### String

```go
func (k Kind) String() string
```

## Remote

```go
type Remote struct {
	// contains filtered or unexported fields
}
```

Remote is one registered remote function. `skgo generate` builds these with NewRemote; application code declares the functions and marks them with Query, Command, LiveQuery, BatchQuery and Form.

### ID

```go
func (r *Remote) ID() string
```

ID is the `<hash>/<name>` pair the client addresses this function by.

### Kind

```go
func (r *Remote) Kind() Kind
```

Kind is which of kit's remote-function kinds this registration publishes.

### Module

```go
func (r *Remote) Module() string
```

Module is the vite-root-relative path of the `.remote.ts` module.

### Name

```go
func (r *Remote) Name() string
```

Name is the export name within that module.

## Remotes

```go
type Remotes struct {
	// contains filtered or unexported fields
}
```

Remotes is a registry of remote functions and the http.Handler that answers calls to them.

### Intercept

```go
func (rs *Remotes) Intercept(next http.Handler) http.Handler
```

Intercept returns a handler that answers remote calls itself and passes everything else to next. It must sit in front of the static handler.

### Lookup

```go
func (rs *Remotes) Lookup(id string) (*Remote, bool)
```

Lookup returns the function registered under `<hash>/<name>`.

### Prefix

```go
func (rs *Remotes) Prefix() string
```

Prefix is the URL prefix every remote call lives under, including the trailing slash — normally "/\_app/remote/".

### ServeHTTP

```go
func (rs *Remotes) ServeHTTP(w http.ResponseWriter, r *http.Request)
```

## NewRemote

```go
func NewRemote(spec RemoteSpec) *Remote
```

NewRemote builds a registration from a generated spec.

It panics on a spec that cannot answer a call, because a spec is generated code: a missing closure is a bug in the generator, it is the same bug on every request, and the first thing an app does at startup is build its registry. Failing there names the function; failing later would be a nil call in a handler.

## NewRemotes

```go
func NewRemotes(cfg RemoteConfig, fns ...*Remote) (*Remotes, error)
```

NewRemotes builds a registry. Two functions with the same module and export name are a programming error and are reported as such.

## RemoteConfig

```go
type RemoteConfig struct {
	// Development mirrors Kit's development dispatch for prerender remotes.
	// Production registries serve only their recorded static artifacts.
	Development bool

	// AppDir is kit's appDir; empty means "_app".
	AppDir string
	// Base is kit's paths.base, without a trailing slash.
	Base string
	// Version, when non-empty, is sent as the `x-sveltekit-version` response
	// header. It must equal the version baked into the client — a different
	// value makes the client force a reload — so leave it empty whenever the
	// client did not come from this build.
	Version string
	// Origin is the app's configured origin, e.g. "http://127.0.0.1:8080".
	// When non-empty, a non-GET remote request from a different origin is
	// refused with 403. Empty disables the check.
	Origin string
	// Transport is the app's `transport` hook: the Go half of the encode/decode
	// pairs `src/hooks.ts` declares. It is optional; without it a custom type
	// goes out as whatever encoding/json makes of it, which is a plain object
	// with no methods on the other side. See the Transport type.
	Transport Transport
	// Remotes is the set of `<hash>/<name>` ids the built frontend calls, as
	// recorded in the manifest. NewRemotes refuses to build a registry that
	// does not answer exactly these.
	Remotes []string
	// Routes is Kit's ordered caller route table, including routes without loads.
	Routes []ManifestRoute
	// OnPanic is called when a remote function panics, with the function's
	// `<hash>/<name>` id, the recovered value, and the stack. The client is
	// told nothing but an opaque 500, so this is the only record the panic
	// leaves; leaving it nil logs the same three things to the standard
	// logger, because a panicking handler that reports nowhere is a bug that
	// cannot be found.
	OnPanic func(id string, value any, stack []byte)
	// contains filtered or unexported fields
}
```

RemoteConfig describes the app the registry is serving. Everything but Origin comes straight from the built manifest.

## RemoteSpec

```go
type RemoteSpec struct {
	// CallerMatchers are generated for commands and forms, including loadless routes.
	CallerMatchers CallerMatchers
	CallerRoutes   CallerRoutes
	// Kind is which of kit's remote-function kinds this publishes.
	Kind Kind
	// Module is the vite-root-relative path of the `.remote.ts` kit compiles,
	// for example "src/routes/todos/todos.remote.ts". Kit hashes it into the
	// id its client addresses the function by.
	Module string
	// Name is the export name within that module.
	Name string
	// Fn is the app's own function value. Nothing calls it through this field:
	// it is here for its code pointer, which is the identity skgo.Refresh
	// looks a query up by, and it is the same function value the closure below
	// calls. That is what lets a refresh name `getTodo` rather than a string.
	Fn any
	// Call answers a query, command, batch-of-one or form. Required for every
	// kind but KindLive.
	Call RemoteFunc
	// Live answers a `query.live`. Required for KindLive and nothing else.
	Live LiveFunc
	// Batch answers a `query.batch`. Required for KindBatch and nothing else.
	Batch BatchFunc
	// Inputs returns the arguments Kit should enqueue during a prerender build.
	// It is set only for KindPrerender when the declaration supplied Inputs.
	Inputs RemoteInputsFunc
	// DecodeArg decodes one argument into the function's own parameter type,
	// for skgo.Requested — the one place a handler is handed a client's
	// argument rather than the server running it. The value it returns is of
	// the function's parameter type; it is typed `any` because the caller's
	// type parameter comes from the app's own function signature and this
	// closure is generated beside the function, not here. It is nil for a
	// function declared without an argument.
	DecodeArg func(arg any) (any, error)
}
```

RemoteSpec is one generated registration. `skgo generate` writes these; an application declares its functions with Query, Command, LiveQuery, BatchQuery and Form and writes none of this by hand.

## RemoteFunc

```go
type RemoteFunc func(ctx context.Context, call Call) (any, error)
```

RemoteFunc is the generated closure that answers one query, command or form: it decodes call.Arg with the decoder polytype generated for the function's own parameter type, calls the app's Go function, and returns the devalue value model tree its generated encoder produced.

## RemoteInputsFunc

```go
type RemoteInputsFunc func(ctx context.Context, call Call) ([]any, error)
```

RemoteInputsFunc returns the build-time inputs for a prerender remote. The context is framework-owned and has no request event; Call provides the application's transport encoder when an input is a transported type.

## BatchFunc

```go
type BatchFunc func(ctx context.Context, calls []Call) ([]any, error)
```

BatchFunc is RemoteFunc for a `query.batch`: every argument kit's client collected in one macrotask, answered in one call, in order.

## LiveFunc

```go
type LiveFunc func(ctx context.Context, call Call, yield func(any) error) error
```

LiveFunc is RemoteFunc for a `query.live`. Each value the app's function yields is encoded by the generated encoder before it reaches yield, so the frames this produces are the same bytes a query's response carries.
