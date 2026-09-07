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
	"strings"

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
	set, target, err := refreshTarget(ctx, "Refresh", fn)
	if err != nil {
		return err
	}

	payload, present, err := queryPayload(arg)
	if err != nil {
		return fmt.Errorf("skgo: Refresh(%s): encoding the argument: %w", funcName(fn), err)
	}

	set.add(target.id+"/"+payload, refreshEntry{fn: target, arg: any(arg), present: present})
	return nil
}

// RefreshNoArg is Refresh for a query that takes no argument:
//
//	return todo, skgo.RefreshNoArg(ctx, getTodos)
//
// It exists because there is no argument to infer the query's parameter type
// from, and it is not a variant of the key: kit's client calls a no-argument
// query with `undefined`, whose payload is the empty string, so the key is the
// query's id and nothing after the slash. That is the same key the client
// stored the query under, which is what makes the value land on the open page.
func RefreshNoArg[Out any](ctx context.Context, fn func(context.Context) (Out, error)) error {
	set, target, err := refreshTarget(ctx, "RefreshNoArg", fn)
	if err != nil {
		return err
	}

	set.add(target.id+"/", refreshEntry{fn: target})
	return nil
}

// refreshTarget resolves the registration a refresh names, and the set it will
// be recorded in.
func refreshTarget(ctx context.Context, who string, fn any) (*refreshSet, *Remote, error) {
	set := refreshSetFrom(ctx)
	if set == nil {
		return nil, nil, fmt.Errorf("skgo: %s(%s): a refresh can only be requested from a command or a form, because it rides back on that call's response", who, funcName(fn))
	}

	target, err := set.rs.lookupFunc(fn)
	if err != nil {
		return nil, nil, err
	}
	if target.kind != kindQuery {
		return nil, nil, fmt.Errorf("skgo: %s(%s): %s is a %s, and only a query can be refreshed", who, funcName(fn), target.id, target.kind)
	}
	return set, target, nil
}

// queryPayload builds the payload half of a refresh key: kit's
// `stringify_remote_arg`, which is what the client used to key its cache.
func queryPayload(arg any) (payload string, present bool, err error) {
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
	// err, when set, is this key's whole answer and the query is never run.
	// It is how a refresh the handler would not accept reaches the client as
	// that query's own error rather than as silence — see requested.go.
	err error
}

// refreshSet is the per-request record of what the handler asked to refresh —
// kit's `state.remote.explicit`. It is created for a command or a form and for
// nothing else, and the same one is reachable from every event derived from
// theirs, so a query that refreshes another query while it is itself being
// refreshed reaches the same set.
type refreshSet struct {
	rs *Remotes
	// requested is what the client asked for, grouped by query id and kept in
	// the order it arrived — kit's `state.remote.requested`, built by
	// `create_requested_map`. Nothing in here runs on its own: a handler has
	// to name a query through RefreshRequested or ReconnectRequested before
	// any of its payloads becomes an entry below.
	requested map[string][]string
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

// resolveExplicit runs everything the handler registered — its own Refresh
// calls and the client requests it accepted — and returns them in the two
// shapes kit's client reads.
//
// Kit keeps one such record, `state.remote.explicit`, for both, and decides
// between `q` and `l` from the registration's kind rather than from who asked;
// so does this. The client reads them differently: a `q` entry replaces a
// query's value, while an `l` entry seeds a live query's value and then tears
// the stream down and reopens it
// (`runtime/client/remote-functions/shared.svelte.js`). Reconnecting is the
// only way a live query can pick up a cookie the command just wrote — its event
// is a snapshot of the request that opened the stream and never refreshes — and
// it is what kit documents for exactly that case.
//
// The queries run here rather than at the point of the Refresh call for kit's
// own reason: a command that refreshes a query and then goes on to change more
// state must publish what the client will see after the command, not a value
// read halfway through it. Running a refresh can register further refreshes —
// nothing stops a query from calling Refresh — so the loop drains until the
// set stops growing.
func (rs *Remotes) resolveExplicit(ctx context.Context, set *refreshSet) (q, l map[string]any) {
	q, l = map[string]any{}, map[string]any{}
	if set == nil {
		return q, l
	}
	for keys := set.take(); len(keys) > 0; keys = set.take() {
		for _, key := range keys {
			if set.drained[key] {
				continue
			}
			set.drained[key] = true

			entry := set.entries[key]
			into := q
			if entry.fn.kind == kindLive {
				into = l
			}

			// A refusal is already this key's answer; nothing runs.
			if entry.err != nil {
				into[key] = map[string]any{"e": errorNode(asHTTPError(entry.err))}
				continue
			}

			// A refresh runs after the command has already succeeded — and may
			// already have written a cookie — so a panic in one of them becomes
			// that entry's error and nothing more. The command keeps its answer:
			// kit reports the failure on the key and returns the command's own
			// result unchanged.
			var (
				value any
				err   error
			)
			if entry.fn.kind == kindLive {
				value, err = rs.firstValue(ctx, entry.fn, entry.arg, entry.present)
			} else {
				value, err = rs.call(ctx, entry.fn, entry.arg, entry.present)
			}
			if err != nil {
				into[key] = map[string]any{"e": errorNode(asHTTPError(err))}
				continue
			}
			into[key] = map[string]any{"v": value}
		}
	}
	return q, l
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

// newRefreshSet opens the record for one command or form. requested is the
// list of keys the client posted, which is client input and is treated as
// such: it is indexed here and read nowhere else but requested.go, where a
// handler has to name a query before any of it runs.
func newRefreshSet(rs *Remotes, requested []string) *refreshSet {
	byID := map[string][]string{}
	for _, key := range requested {
		// The payload can itself contain no slash, but the id always holds
		// exactly one, so the split is on the LAST slash.
		i := strings.LastIndex(key, "/")
		if i < 0 {
			continue
		}
		id, payload := key[:i], key[i+1:]
		byID[id] = append(byID[id], payload)
	}
	return &refreshSet{
		rs:        rs,
		requested: byID,
		entries:   map[string]refreshEntry{},
		drained:   map[string]bool{},
	}
}

// collectRefreshes builds the `q` and `l` maps a command or form response
// carries.
//
// There is one source now: what the handler registered. Its own Refresh calls
// and the client requests it accepted go into the same set under the same
// keys, so a handler that computed a value itself and then accepted the
// client's request for the same instance publishes one of them — the later
// registration — rather than running the query twice.
//
// They run on an event derived from the command's — immutable, sharing its
// cookie jar — so a query refreshed by a command that just signed the visitor
// in reads the new cookie, which is what kit gets by keeping them on one
// request.
func (rs *Remotes) collectRefreshes(ctx context.Context, ev *Event) (q, l map[string]any) {
	return rs.resolveExplicit(withEvent(ctx, ev.immutable()), ev.refreshes)
}
