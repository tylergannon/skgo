package skgo

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
)

// Deferred is a value a load has promised but does not have yet. It reaches the
// browser as a JavaScript `Promise`: the page renders everything else
// immediately, and the value arrives on the same response as an extra line,
// without a second request.
//
// It is what kit's own loads do by putting a promise in the returned object,
// and it obeys kit's rule about where one may sit: a Deferred may only be a
// field of the value a load returns, because that is the object kit's client
// walks looking for promises to await.
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
// Everything that is not deferred travels exactly as a remote function's result
// does, through encoding/json, so the two boundaries never disagree about a
// type. The deferred fields are then put back: encoding/json wrote null for
// each of them, and each is replaced by the Deferred itself, which the devalue
// reducer turns into kit's promise placeholder.
func (t Transport) encodeLoadValue(v any) (any, error) {
	tree, err := t.encodeTree(v)
	if err != nil {
		return nil, err
	}

	fields, err := deferredFields(reflect.ValueOf(v))
	if err != nil {
		return nil, err
	}
	if len(fields) == 0 {
		return tree, nil
	}

	obj, ok := tree.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("skgo: a load that defers a value must return a struct")
	}
	for name, d := range fields {
		if _, present := obj[name]; !present {
			// The field was dropped by encoding/json — a `json:"-"` tag, most
			// likely — so nothing on the client is waiting for it.
			continue
		}
		obj[name] = d
	}
	return tree, nil
}

// deferredFields finds the Deferred fields of a load's result, keyed by the
// name encoding/json gave them. A Deferred anywhere else is refused rather than
// silently serialized as null.
func deferredFields(rv reflect.Value) (map[string]*deferred, error) {
	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return nil, nil
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		if containsDeferred(rv.Type()) {
			return nil, fmt.Errorf("skgo: a load that defers a value must return a struct, not %s", rv.Type())
		}
		return nil, nil
	}

	found := map[string]*deferred{}
	if err := collectDeferredFields(rv, found); err != nil {
		return nil, err
	}
	return found, nil
}

func collectDeferredFields(rv reflect.Value, into map[string]*deferred) error {
	rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		if !field.IsExported() {
			continue
		}
		tag, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if tag == "-" && !strings.Contains(field.Tag.Get("json"), ",") {
			continue
		}

		value := rv.Field(i)
		if holder, ok := value.Interface().(deferredHolder); ok {
			name := tag
			if name == "" {
				name = field.Name
			}
			d := holder.deferredValue()
			if d == nil {
				return fmt.Errorf("skgo: field %s is a zero Deferred; build one with skgo.Async or skgo.Resolved", field.Name)
			}
			into[name] = d
			continue
		}

		// An embedded struct's fields are promoted into the same JSON object,
		// so a Deferred inside one is still a field of the load's result.
		if field.Anonymous && tag == "" {
			embedded := value
			for embedded.Kind() == reflect.Pointer {
				if embedded.IsNil() {
					embedded = reflect.Value{}
					break
				}
				embedded = embedded.Elem()
			}
			if embedded.IsValid() && embedded.Kind() == reflect.Struct {
				if err := collectDeferredFields(embedded, into); err != nil {
					return err
				}
				continue
			}
		}

		if containsDeferred(field.Type) {
			return fmt.Errorf("skgo: %s holds a Deferred below the top level of the load's result; kit's client only awaits promises the load returns directly", field.Name)
		}
	}
	return nil
}

// containsDeferred reports whether a Deferred can occur anywhere inside t.
func containsDeferred(t reflect.Type) bool {
	return containsDeferredSeen(t, map[reflect.Type]bool{})
}

func containsDeferredSeen(t reflect.Type, seen map[reflect.Type]bool) bool {
	if t == nil || seen[t] {
		return false
	}
	seen[t] = true
	if t.Implements(deferredHolderType) || reflect.PointerTo(t).Implements(deferredHolderType) {
		return true
	}
	switch t.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
		return containsDeferredSeen(t.Elem(), seen)
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			if containsDeferredSeen(t.Field(i).Type, seen) {
				return true
			}
		}
	case reflect.Interface:
		// An interface could hold anything, so it is treated as opaque; a
		// remote function's result may not be an interface either.
		return false
	}
	return false
}

var _ json.Marshaler = Deferred[int]{}
