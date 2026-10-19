// Package skgo is a Go backend for SvelteKit.
//
// It lets you write the server side of a SvelteKit app in Go. Remote
// functions, server loads and API routes sit next to the Svelte files that use
// them, and skgo generate writes the TypeScript types and bindings that
// SvelteKit's client calls. Pages are rendered by SvelteKit's own renderer
// inside the Go process, so the app ships as one Go binary.
//
// A remote function is an ordinary Go function passed to a marker:
//
//	func todos(ctx context.Context) ([]Todo, error) {
//		return store.List(ctx)
//	}
//
//	var _ = skgo.Query(todos)
//
// The markers are [Query], [Command], [Form], [LiveQuery] and [BatchQuery].
// Request state is available from [EventFrom]. The example application in the
// repository, https://github.com/tylergannon/skgo, shows a complete server.
//
// Loads, actions, remote functions and endpoints are each declared by passing
// the function to a marker whose result is discarded; skgo generate reads the
// marker and writes the TypeScript file Kit compiles and the Go registration
// the server mounts.
//
// # Loads
//
// The Go side of a +page.server.ts or +layout.server.ts load is a function
// marked with [Load]. [Parent] reads what the outer layouts' loads produced,
// and [Async] starts work whose [Deferred] result the page receives later on
// the same response. [LoadParamValue] and [TrackLoadParam] are what the
// generated route parameter accessors use.
//
// # Form actions
//
// The Go side of a +page.server.ts actions export is a function marked with
// [Action], [DefaultAction], [ActionWithFailure] or [ActionNoData]. [Fail]
// returns validation data with an error status instead of an error page.
//
// # Remote functions
//
// A function in a *.remote.go file marked with [Query], [Command], [Form],
// [LiveQuery] or [BatchQuery] becomes an export of the generated .remote.ts
// module beside it. A command or form names the queries to re-run on the same
// response with [Refresh] and its relatives; [Requested] returns the instances
// the client asked it to refresh. [SubmitForm] and [FormClient] submit a Form
// from Go to the endpoint Kit's client uses. [Call], [Kind] and [Remote] are
// what the generated registration hands the server, and [RequireArgument] and
// [RefuseArgument] are the decode steps it uses for functions declared with
// and without an argument; application code rarely touches them.
//
// # Endpoints
//
// The Go side of a +server.ts file is a server.go in the route directory with
// one ordinary net/http handler per method, marked with [GET], [POST], [PUT],
// [PATCH], [DELETE], [HEAD], [OPTIONS] or [QUERY], plus an optional
// [Fallback] for every other method.
//
// # Request event
//
// Kit's RequestEvent is an [Event]. [EventFrom] returns it from the
// context.Context a remote function, action or hook runs with, and it holds
// the URL, the route id and params, the cookies, the request-scoped fetch,
// the client address and the response headers. [RequestEvent] is the typed
// form generated code hands a load or a handler that declared route
// parameters and locals, and [RequestLocals] returns the application's own
// locals struct, the one named by --locals-type at generation.
//
// # Hooks
//
// [Handle] runs before anything answers a request, [HandleFetch] intercepts
// the fetches the server makes during a render or through Event.Fetch, and
// [HandleError] shapes what an unexpected error reports. [Sequence] composes
// several [Middleware] values into one, as Kit's sequence does, and
// [RequestSequence] does the same for the typed [RequestMiddleware].
// [NewResponse] builds the response a middleware answers with.
//
// # Errors and redirects
//
// [Errorf] returns an [HTTPError], Kit's error(status, message). [Invalid]
// carries field-level validation [Issue] values back to a form, and
// [BadRequest] is an opaque 400 that keeps its detail on the server.
// [NewRedirect] builds a [Redirect]. [CaughtError] is what HandleError
// receives.
//
// # Server
//
// [NewSSR] renders pages with Kit's own renderer over the adapter's build
// output; [NewStaticHandler] serves the client assets and prerendered pages
// and hands everything else to the renderer; [ReadManifest] reads the routes,
// pages and assets the adapter recorded. [Transport] declares the domain
// types that cross between Go and the browser with their own encoding.
// [RunPrerenderService] is the Go side of the build-time prerender.
//
// # Environment
//
// Kit's $env modules are declared in Go: a struct marked with [Environment]
// names the variables the application needs and their types, and
// [LoadEnvironment] validates the values at startup and returns the
// [EnvironmentSnapshot] the server keeps.
package skgo
