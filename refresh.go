// Server-driven single-flight refreshes: the Go side of the `refresh()` a kit
// command calls on a query instance.
//
// A kit command handler writes `getTodo(id).refresh()`, and the query's new
// value rides back on the command's own response
// (`runtime/app/server/remote/query.js`, `collect_remote_data` in
// `runtime/server/remote-functions.js`). Go has no query instance to hang a
// method on, so the function value and its argument are named instead:
//
//	skgo.Refresh(ctx, getTodo, id)
//
// which is the same two facts kit's `getTodo(id)` carries, and the compiler
// checks the argument against the query's own parameter type.
package skgo

import (
	"context"
	"fmt"
	"reflect"
	"runtime"

	"github.com/tylergannon/skgo/internal/remotearg"
)

// Refresh asks for a query to be re-run and its new value returned on the
// response the current command or form is already producing. The client
// applies it to the query instance keyed by exactly this argument, so nothing
// goes back for more: the browser sees the change in a single flight.
//
// It names the Go function, not a string:
//
//	func renameTodo(ctx context.Context, arg Rename) (businesslogic.Todo, error) {
//		todo, ok := businesslogic.Default.Rename(arg.ID, arg.Text, signedIn(ctx))
//		if !ok {
//			return businesslogic.Todo{}, skgo.Errorf(404, "No todo with id %q", arg.ID)
//		}
//		return todo, skgo.Refresh(ctx, getTodo, arg.ID)
//	}
//
// getTodo's own parameter type fixes what the second argument may be, so a
// refresh that names the wrong kind of argument does not compile.
//
// The argument is the cache key, not a hint. Kit's client caches a query under
// the argument it was called with, and the key skgo computes here is the same
// string that client computed — kit's `create_remote_key(id, payload)` over
// `stringify_remote_arg`, both ported in internal/remotearg. Refreshing
// getTodo("t1") therefore leaves an open getTodo("t2") alone, which is the
// point: a command that changed one row must not silently republish another.
//
// Naming a query no open page is showing is not an error. Kit's client parks
// the value under its key and hands it to the next component that asks for
// exactly that query and argument
// (`runtime/client/remote-functions/shared.svelte.js`), so the refresh is a
// pre-seed rather than a mistake.
//
// The query does not run here. Kit stores the refresh and runs it after the
// handler body has finished, so it observes everything the command went on to
// change; skgo does the same, in the same request, on an event that shares the
// command's cookie jar. A refresh that fails does not fail the command: the
// error is reported on that query alone, exactly as kit reports it, and the
// command keeps its own answer.
//
// The error returned here is about the call itself — a function that is not a
// registered query, or an argument that cannot be encoded — and never about
// what the query goes on to do.
func Refresh[In, Out any](ctx context.Context, fn func(context.Context, In) (Out, error), arg In) error {
	set := refreshSetFrom(ctx)
	if set == nil {
		return fmt.Errorf("skgo: Refresh(%s): a refresh can only be requested from a command or a form, because it rides back on that call's response", funcName(fn))
	}

	target, err := set.rs.lookupFunc(fn)
	if err != nil {
		return err
	}
	if target.kind != kindQuery {
		return fmt.Errorf("skgo: Refresh(%s): %s is a %s, and only a query can be refreshed", funcName(fn), target.id, target.kind)
	}

	payload, present, err := queryPayload(arg)
	if err != nil {
		return fmt.Errorf("skgo: Refresh(%s): encoding the argument: %w", funcName(fn), err)
	}

	set.add(target.id+"/"+payload, refreshEntry{fn: target, arg: any(arg), present: present})
	return nil
}

// queryPayload builds the payload half of a refresh key: kit's
// `stringify_remote_arg`, which is what the client used to key its cache.
//
// skgo.None is the Go spelling of a query that takes no argument, and kit's
// client calls such a query with `undefined`, whose payload is the empty
// string. Encoding None as the empty object it looks like in Go would build a
// key nothing on the page is stored under, and the refresh would land nowhere
// and report nothing.
func queryPayload(arg any) (payload string, present bool, err error) {
	if _, none := arg.(None); none {
		return "", false, nil
	}
	tree, err := encodeValue(arg)
	if err != nil {
		return "", false, err
	}
	payload, err = remotearg.StringifyQueryArg(tree)
	if err != nil {
		return "", false, err
	}
	return payload, true, nil
}

// refreshEntry is one registered refresh, held until the handler returns.
type refreshEntry struct {
	fn      *Remote
	arg     any
	present bool
}

// refreshSet is the per-request record of what the handler asked to refresh —
// kit's `state.remote.explicit`. It is created for a command or a form and for
// nothing else, and the same one is reachable from every event derived from
// theirs, so a query that refreshes another query while it is itself being
// refreshed reaches the same set.
type refreshSet struct {
	rs *Remotes
	// order is registration order, so the response is built in the order the
	// handler asked rather than in Go's map order.
	order   []string
	entries map[string]refreshEntry
	// drained records the keys already run, so the re-entrant drain below
	// cannot loop on a query that refreshes itself.
	drained map[string]bool
}

func (s *refreshSet) add(key string, e refreshEntry) {
	if _, seen := s.entries[key]; !seen {
		s.order = append(s.order, key)
	}
	s.entries[key] = e
}

// take removes and returns everything registered since the last call. The
// drain loop calls it until it comes back empty, because running one refresh
// may register another.
func (s *refreshSet) take() []string {
	keys := s.order
	s.order = nil
	return keys
}

func refreshSetFrom(ctx context.Context) *refreshSet {
	ev := EventFrom(ctx)
	if ev == nil {
		return nil
	}
	return ev.refreshes
}

// resolveExplicit runs the refreshes the handler registered and returns them in
// the `q` shape kit's client reads.
//
// The queries run here rather than at the point of the Refresh call for kit's
// own reason: a command that refreshes a query and then goes on to change more
// state must publish what the client will see after the command, not a value
// read halfway through it. Running a refresh can register further refreshes —
// nothing stops a query from calling Refresh — so the loop drains until the
// set stops growing.
func (rs *Remotes) resolveExplicit(ctx context.Context, set *refreshSet) map[string]any {
	q := map[string]any{}
	if set == nil {
		return q
	}
	for keys := set.take(); len(keys) > 0; keys = set.take() {
		for _, key := range keys {
			if set.drained[key] {
				continue
			}
			set.drained[key] = true

			entry := set.entries[key]
			value, err := rs.call(ctx, entry.fn, entry.arg, entry.present)
			if err != nil {
				// A refresh runs after the command has already succeeded, so
				// its failure is that query's business and not the command's.
				// Kit reports it on the key and returns the command's own
				// result unchanged.
				q[key] = map[string]any{"e": errorNode(asHTTPError(err))}
				continue
			}
			q[key] = map[string]any{"v": value}
		}
	}
	return q
}

// lookupFunc finds the registration a Go function value belongs to.
//
// A remote function is declared by name — `skgo.Query(getTodo)` refuses
// anything but an identifier for a function declared in the same file — so the
// generator registers a top-level function and Refresh is handed that same
// top-level function. Two such functions never share a code pointer, which is
// what makes the function value usable as its own identity and lets the
// argument keep its Go type all the way to the compiler. NewRemotes refuses a
// registry where two of them do share one, so the ambiguity is a startup
// failure rather than a refresh that quietly updates the wrong query.
func (rs *Remotes) lookupFunc(fn any) (*Remote, error) {
	ptr := codePointer(fn)
	if ptr == 0 {
		return nil, fmt.Errorf("skgo: a nil function cannot be refreshed")
	}
	target, ok := rs.byFunc[ptr]
	if !ok {
		return nil, fmt.Errorf("skgo: %s is not a registered remote function: declare it with skgo.Query and re-run `skgo generate`", funcName(fn))
	}
	return target, nil
}

func codePointer(fn any) uintptr {
	v := reflect.ValueOf(fn)
	if v.Kind() != reflect.Func || v.IsNil() {
		return 0
	}
	return v.Pointer()
}

// funcName is the fully qualified name of a Go function, for error messages.
func funcName(fn any) string { return funcNameAt(codePointer(fn)) }

func funcNameAt(ptr uintptr) string {
	if ptr == 0 {
		return "<nil>"
	}
	if f := runtime.FuncForPC(ptr); f != nil {
		return f.Name()
	}
	return "<unknown>"
}

func (k remoteKind) String() string {
	switch k {
	case kindCommand:
		return "command"
	case kindLive:
		return "live query"
	case kindForm:
		return "form"
	}
	return "query"
}

func newRefreshSet(rs *Remotes) *refreshSet {
	return &refreshSet{
		rs:      rs,
		entries: map[string]refreshEntry{},
		drained: map[string]bool{},
	}
}

// collectRefreshes builds the `q` and `l` maps a command or form response
// carries, from both of the things that can ask for one.
//
// The handler's own Refresh calls are drained first and win any key the client
// also named. They are the server's instruction rather than a request, and the
// distinction is load-bearing for a value the handler computed itself: a
// client-requested re-run of the same key would replace it with whatever the
// query returns now.
//
// Both run on an event derived from the command's — immutable, sharing its
// cookie jar — so a query refreshed by a command that just signed the visitor
// in reads the new cookie, which is what kit gets by keeping them on one
// request.
func (rs *Remotes) collectRefreshes(ctx context.Context, ev *Event, requested []string) (q, l map[string]any) {
	derived := withEvent(ctx, ev.immutable())

	q = rs.resolveExplicit(derived, ev.refreshes)

	// A key the handler already answered is dropped from the client's list
	// rather than merged afterwards: running the query a second time to throw
	// the result away would run its side of the app twice per command.
	remaining := make([]string, 0, len(requested))
	for _, key := range requested {
		if _, done := q[key]; !done {
			remaining = append(remaining, key)
		}
	}

	asked, l := rs.resolveRefreshes(derived, remaining)
	for key, node := range asked {
		q[key] = node
	}
	return q, l
}
