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
package skgo
