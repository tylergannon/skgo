// Client-requested single-flight refreshes: the Go side of kit's
// `requested(query, limit)`.
//
// A page writes `addTodo(text).updates(getTodos())`, and kit's client posts the
// keys of the query instances it wants back with the command. That list is
// client input. Kit does not act on it: the keys sit in `state.remote.requested`
// (`create_requested_map` in `runtime/server/remote-functions.js`) and are read
// in exactly one place — `requested(fn, limit)` in
// `runtime/app/server/remote/requested.js` — which a command or form handler
// has to call, naming the query it is willing to run and how many instances of
// it. A handler that says nothing refreshes nothing.
//
// That gate is the whole point, and kit's own documentation says why: the list
// is in the network tab. Without a gate, a visitor who has seen the app once
// can post a command carrying thousands of refreshes and have the server run
// every one of them.
//
// `limit` is required rather than optional for the same reason. A default would
// have to be unbounded — anything else silently drops refreshes a working page
// asked for — and an unbounded default is the hole with an extra step. Making
// the number a parameter makes the developer say how many instances of this
// query they are prepared to run for one command, which is a question only they
// can answer.
//
// A query that takes no argument is the exception, and kit's key space is what
// makes it one rather than a preference. Kit's client calls such a query with
// `undefined`, whose payload is the empty string, so every page holding it
// holds the same instance under the same key: the query's id, a slash, and
// nothing after it. One key is one instance, so the only number a limit could
// hold is one however large the caller writes it, and a parameter that can only
// mean one thing is worse than no parameter. RefreshRequestedNoArg and
// ReconnectRequestedNoArg therefore take no limit; the bound is structural, and
// the gate is unchanged — a handler that names nothing still runs nothing.
package skgo

import (
	"context"
	"fmt"
)

// RequestedQuery is one instance of a query that the client asked the current
// command or form to refresh — one entry of kit's `requested(...)` iterable.
//
// It is an instance rather than a function: the browser caches a query under
// the argument it was called with, so `getTodo("t1")` and `getTodo("t2")` are
// two of these, and refreshing one leaves the other alone.
type RequestedQuery[In any] struct {
	// Arg is the argument the client's instance was called with, decoded into
	// the query's own parameter type. A handler that only wants to refresh
	// some of what was asked for reads this to decide.
	Arg In

	set  *refreshSet
	key  string
	fn   *Remote
	call Call
}

// Refresh accepts this instance: the query is re-run after the handler returns
// and its new value rides back on the command's own response, exactly as it
// does for skgo.Refresh.
//
// The key is the one the client sent, not one recomputed from Arg, so the value
// lands on the instance the browser is actually showing.
func (q RequestedQuery[In]) Refresh() { q.register() }

func (q RequestedQuery[In]) register() {
	if q.set == nil {
		return
	}
	q.set.add(q.key, refreshEntry{fn: q.fn, call: q.call})
}

// Requested returns the instances of fn that the client asked this command or
// form to refresh, at most limit of them, and refreshes none of them: calling
// Refresh on an entry is what accepts it.
//
// Use it when the handler wants a say in which instances it will run:
//
//	requests, err := skgo.Requested(ctx, getTodo, 4)
//	if err != nil {
//		return todo, err
//	}
//	for _, request := range requests {
//		if request.Arg == arg.ID {
//			request.Refresh()
//		}
//	}
//
// When the handler will take everything the client asked for, RefreshRequested
// says exactly that in one line.
//
// Anything the client asked for beyond limit is not returned and not run.
// It is failed instead, onto its own key, so the query the page is showing goes
// visibly into an error state rather than quietly stale — kit's choice, and the
// reason a page that outgrows its handler's limit shows up as a broken panel
// rather than as a bug report about data that stopped updating. An instance
// whose payload does not decode into In is failed the same way.
//
// The error returned is about the call itself: a function that is not a
// registered query, a negative limit, or a call from somewhere that has no
// response for a refresh to ride back on. It is never about what a query goes
// on to do.
func Requested[In, Out any](ctx context.Context, fn func(context.Context, In) (Out, error), limit int) ([]RequestedQuery[In], error) {
	return requestedInstances[In](ctx, fn, limit, KindQuery, "Requested", "refresh")
}

// RefreshRequested accepts up to limit of the instances of fn the client asked
// this command or form to refresh:
//
//	func addTodo(ctx context.Context, text string) (businesslogic.Todo, error) {
//		return businesslogic.Default.Add(text), skgo.RefreshRequested(ctx, getTodos, 1)
//	}
//
// It is kit's `await requested(getTodos, 1).refreshAll()`, and it is the whole
// of what a handler has to write to honour the `updates(...)` its page asks
// for. A handler that never mentions a query will not run it however loudly the
// request asks.
func RefreshRequested[In, Out any](ctx context.Context, fn func(context.Context, In) (Out, error), limit int) error {
	requests, err := requestedInstances[In](ctx, fn, limit, KindQuery, "RefreshRequested", "refresh")
	if err != nil {
		return err
	}
	for _, request := range requests {
		request.register()
	}
	return nil
}

// RefreshRequestedNoArg is RefreshRequested for a query that takes no argument:
//
//	func addTodo(ctx context.Context, text string) (businesslogic.Todo, error) {
//		return businesslogic.Default.Add(text), skgo.RefreshRequestedNoArg(ctx, getTodos)
//	}
//
// It is the same gate — a handler that never names getTodos does not refresh
// it, however loudly the request asks — and it takes no limit because there is
// nothing left for one to bound: the client can only ever have asked for the
// one instance. See the package comment above.
//
// Anything else the client sent under this query's id is not an instance of it.
// Kit's validator for a remote function declared without a schema refuses an
// argument outright, so a payload that carries one comes back refused on its
// own key, and so does a second copy of the instance itself.
//
// There is no RequestedNoArg beside this. Requested exists so a handler can
// pick between instances by their arguments; a query with no argument has one
// instance and nothing to pick by, and a handler that wants a say writes this
// call under an `if`.
func RefreshRequestedNoArg[Out any](ctx context.Context, fn func(context.Context) (Out, error)) error {
	return acceptTheOneInstance(ctx, fn, KindQuery, "RefreshRequestedNoArg", "refresh")
}

// ReconnectRequested is RefreshRequested for a live query, and it reconnects
// rather than refreshes because that is the only thing a live query can be
// told to do.
//
// A live query's event is a snapshot of the request that opened its stream, so
// it can never see a cookie a later command wrote; kit's answer is to tear the
// stream down and open it again on the command's own request, which is why
// signing in reconnects the count rather than refreshing it.
func ReconnectRequested[In, Out any](ctx context.Context, fn func(context.Context, In, func(Out) error) error, limit int) error {
	requests, err := requestedInstances[In](ctx, fn, limit, KindLive, "ReconnectRequested", "reconnect")
	if err != nil {
		return err
	}
	for _, request := range requests {
		request.register()
	}
	return nil
}

// ReconnectRequestedNoArg is ReconnectRequested for a live query that takes no
// argument:
//
//	return session, skgo.ReconnectRequestedNoArg(ctx, watchCount)
//
// See RefreshRequestedNoArg for why there is no limit, and ReconnectRequested
// for why a live query reconnects rather than refreshes.
func ReconnectRequestedNoArg[Out any](ctx context.Context, fn func(context.Context, func(Out) error) error) error {
	return acceptTheOneInstance(ctx, fn, KindLive, "ReconnectRequestedNoArg", "reconnect")
}

// acceptTheOneInstance is the body the two no-argument forms share. The limit
// is one because the key space is one: see the package comment.
func acceptTheOneInstance(ctx context.Context, fn any, want Kind, call, verb string) error {
	requests, err := requestedInstances[noArgument](ctx, fn, 1, want, call, verb)
	if err != nil {
		return err
	}
	for _, request := range requests {
		request.register()
	}
	return nil
}

// noArgument stands in for the argument of a query that has none. It never
// reaches the function: the generated closure for such a query refuses any
// argument and calls the app's function with none.
type noArgument struct{}

// decodeRequested turns one requested instance's argument into the query's own
// parameter type, using the strict decoder `skgo generate` emitted for that
// type and registered beside the function.
//
// The type assertion cannot be avoided and cannot be wrong: the decoder was
// generated from the same Go signature the caller's type parameter comes from,
// so In is the type it returns. It is checked rather than asserted blind
// because a registry built some other way would otherwise corrupt a handler's
// argument instead of failing.
func decodeRequested[In any](target *Remote, call Call) (In, error) {
	var zero In
	if target.argDecoder == nil {
		// Kit's `create_validator` with no schema: a remote function declared
		// without an argument errors 400 on any argument at all. Reaching that
		// here means the client sent a payload under a key its own copy of the
		// query could not have produced.
		return zero, RefuseArgument(call)
	}
	if !call.Present {
		// The other half of the same rule: a query declared *with* an argument
		// was not called with one, so there is nothing for its decoder to
		// admit.
		return zero, &HTTPError{Status: 400, Message: "Bad Request"}
	}
	decoded, err := target.argDecoder(call.Arg)
	if err != nil {
		return zero, err
	}
	in, ok := decoded.(In)
	if !ok {
		return zero, fmt.Errorf("skgo: %s decodes its argument as %T, not %T", target.id, decoded, zero)
	}
	return in, nil
}

// requestedInstances is the body they all share: find the registration, take
// the client's payloads for it, and split them at the limit. Each surviving
// payload is decoded by the registration's own generated decoder, which is
// where a query that takes no argument differs from one that does.
func requestedInstances[In any](ctx context.Context, fn any, limit int, want Kind, call, verb string) ([]RequestedQuery[In], error) {
	set := refreshSetFrom(ctx)
	if set == nil {
		return nil, fmt.Errorf("skgo: %s(%s): a client's %s request can only be accepted by a command or a form, because it rides back on that call's response", call, funcName(fn), verb)
	}
	if limit < 0 {
		return nil, fmt.Errorf("skgo: %s(%s): a limit of %d is not a number of instances", call, funcName(fn), limit)
	}

	target, err := set.rs.lookupFunc(fn)
	if err != nil {
		return nil, err
	}
	if target.kind != want {
		return nil, fmt.Errorf("skgo: %s(%s): %s is a %s, and only a %s can be told to %s", call, funcName(fn), target.id, target.kind, want, verb)
	}

	payloads := set.requested[target.id]
	out := make([]RequestedQuery[In], 0, min(len(payloads), limit))
	for i, payload := range payloads {
		key := target.id + "/" + payload

		// Beyond the limit. Kit fails these onto their own keys rather than
		// dropping them, so the client's query reports the refusal instead of
		// showing a value nobody refreshed. A page whose command asks for more
		// than its handler accepts is then a visible fault rather than a page
		// that has quietly stopped updating.
		if i >= limit {
			set.add(key, refreshEntry{fn: target, err: Errorf(400, "A requested %s of %s was refused: this handler accepts at most %d", verb, target.name, limit)})
			continue
		}

		instance, err := set.rs.parsePayload(payload)
		if err != nil {
			set.add(key, refreshEntry{fn: target, err: Errorf(400, "Bad Request")})
			continue
		}
		// Kit runs the query's own validator here and fails the entry when it
		// rejects. skgo's validation is the generated decoder for the Go
		// parameter type, which is the same check at the same moment and the
		// same answer: this instance is refused, and the rest are unaffected.
		arg, err := decodeRequested[In](target, instance)
		if err != nil {
			set.add(key, refreshEntry{fn: target, err: asHTTPError(err)})
			continue
		}

		out = append(out, RequestedQuery[In]{Arg: arg, set: set, key: key, fn: target, call: instance})
	}
	return out, nil
}
