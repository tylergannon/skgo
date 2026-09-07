package skgo

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"
)

// Deferred is a value a load has promised but does not have yet. It reaches the
// browser as a JavaScript `Promise`: the page renders everything else
// immediately, and the value arrives on the same response as an extra line,
// without a second request.
//
// It is what kit's own loads do by putting a promise in the returned object,
// and it may sit wherever kit lets a promise sit: anywhere in the value a load
// returns, at any depth, including inside a value another Deferred promised.
// Kit reaches promises with a devalue reducer and its client reads them back
// with a devalue reviver, and both walk the whole tree.
type Deferred[T any] struct{ d *deferred }

// Async starts fn and returns a Deferred for its result. fn runs on its own
// goroutine, so the load can return before fn has finished; ctx is the load's
// context and stays alive until the response is complete.
//
//	func load(ctx context.Context) (Data, error) {
//		return Data{
//			Total:  store.Total(),
//			Orders: skgo.Async(ctx, store.Orders),
//		}, nil
//	}
func Async[T any](ctx context.Context, fn func(context.Context) (T, error)) Deferred[T] {
	d := &deferred{done: make(chan struct{})}
	go func() {
		defer close(d.done)
		defer func() {
			if r := recover(); r != nil {
				d.err = Errorf(500, "Internal Error")
			}
		}()
		value, err := fn(ctx)
		if err != nil {
			d.err = err
			return
		}
		d.value = value
	}()
	return Deferred[T]{d: d}
}

// Resolved is a Deferred that already holds its value. It exists so a load can
// return the same shape whether or not the work was worth deferring.
func Resolved[T any](value T) Deferred[T] {
	d := &deferred{done: make(chan struct{})}
	d.value = value
	close(d.done)
	return Deferred[T]{d: d}
}

// MarshalJSON writes null. A Deferred has no JSON form: the serializer replaces
// it with the promise placeholder kit's client understands before the value
// reaches the wire, and this exists only so that encoding a load's result never
// fails on the field.
func (d Deferred[T]) MarshalJSON() ([]byte, error) { return []byte("null"), nil }

func (d Deferred[T]) deferredValue() *deferred { return d.d }

// deferredHolder is how the serializer recognises a Deferred without knowing
// its type argument.
type deferredHolder interface{ deferredValue() *deferred }

var deferredHolderType = reflect.TypeOf((*deferredHolder)(nil)).Elem()

// deferred is the untyped half: a value that is being computed, plus the
// channel that says when it is not.
type deferred struct {
	done chan struct{}
	// value is the raw Go value the load produced, not a tree: it is encoded
	// at serialization time, which is the only place the app's transport hook
	// is known. Encoding it here would flatten a transported value before any
	// reducer could see it.
	value any
	err   error

	once sync.Once
}

// wait blocks until the value is settled or ctx is done.
func (d *deferred) wait(ctx context.Context) (any, error) {
	select {
	case <-d.done:
		return d.value, d.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// encodeLoadValue turns a load's result into the tree the serializer walks.
//
// It is encodeTree with one addition: a Deferred survives the walk instead of
// being flattened, so the devalue reducer can put kit's promise placeholder
// where the load left it.
//
// Kit's rule is that a promise may sit anywhere in the object a load returns.
// Its serializer reaches one through a devalue reducer — `Promise` in
// `server_data_serializer_json`, the `thing?.then` arm of `get_replacer` in
// `server_data_serializer` — and a reducer runs over every value in the tree,
// at every depth. The client agrees: `process_stream` deserializes with
// `devalue.unflatten(data, { ...app.decoders, Promise })`, which walks the
// same way. A promised value that itself holds a promise works too, because
// each chunk is written with the same reducers that wrote the first one.
//
// A Deferred is not encoded here. It holds the raw Go value so that the value
// is encoded at the moment it settles, which is the only place the app's
// transport hook is known; encoding it now would flatten a transported value
// before any reducer could see it.
func (t Transport) encodeLoadValue(v any) (any, error) {
	return treeEncoder{t: t, promises: true}.walk(reflect.ValueOf(v))
}

// isDeferred reports whether rt is a Deferred.
func isDeferred(rt reflect.Type) bool {
	return rt.Implements(deferredHolderType) || reflect.PointerTo(rt).Implements(deferredHolderType)
}

var _ json.Marshaler = Deferred[int]{}
