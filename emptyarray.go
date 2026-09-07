package skgo

import (
	"encoding"
	"encoding/json"
	"reflect"
	"strconv"
)

// emptyArrays rewrites the `null` encoding/json writes for a nil slice into the
// empty array the generated TypeScript promises.
//
// polytype projects a Go slice as `Array<T>`, never `Array<T> | null`, and its
// own devalue encoder already says so out loud — "a nil required slice encodes
// as an empty array, never as null" — but only the types the app transports go
// through that encoder. Every other result reaches the wire through
// encodeValue's `json.Marshal`/`json.Unmarshal` round trip, and encoding/json
// writes `null` for a nil slice. So the declaration was false for the zero
// value of the most common Go container, and a page that called `.map` on it
// threw.
//
// The rewrite runs over the tree the round trip produced, with the Go value
// beside it to say where a slice was: a null the tree carries becomes `[]`
// exactly where the Go type is a slice, and nothing else changes. Nulls that
// mean something — a nil pointer, a nil map, a nil interface — are left alone,
// because their declarations say so too.
//
// It never descends into a value that marshals itself: what a json.Marshaler or
// an encoding.TextMarshaler writes is its own business and need not look like
// its fields at all.
func emptyArrays(rv reflect.Value, tree any) any {
	if !rv.IsValid() || marshalsItself(rv.Type()) {
		return tree
	}
	switch rv.Kind() {
	case reflect.Pointer, reflect.Interface:
		if rv.IsNil() {
			return tree
		}
		return emptyArrays(rv.Elem(), tree)

	case reflect.Slice, reflect.Array:
		if rv.Kind() == reflect.Slice {
			// A byte slice is not an array on the wire: encoding/json writes it
			// as a base64 string, and polytype refuses it outright rather than
			// project one. Turning its null into `[]` would invent an array
			// where the declaration never promised one.
			if rv.Type().Elem().Kind() == reflect.Uint8 {
				return tree
			}
			if tree == nil && rv.IsNil() {
				return []any{}
			}
		}
		items, ok := tree.([]any)
		if !ok || len(items) != rv.Len() {
			return tree
		}
		for i := range items {
			items[i] = emptyArrays(rv.Index(i), items[i])
		}
		return items

	case reflect.Map:
		obj, ok := tree.(map[string]any)
		if !ok {
			return tree
		}
		iter := rv.MapRange()
		for iter.Next() {
			key, ok := jsonMapKey(iter.Key())
			if !ok {
				return tree
			}
			if value, present := obj[key]; present {
				obj[key] = emptyArrays(iter.Value(), value)
			}
		}
		return obj

	case reflect.Struct:
		obj, ok := tree.(map[string]any)
		if !ok {
			return tree
		}
		emptyArrayFields(rv, obj)
		return obj
	}
	return tree
}

// emptyArrayFields rewrites the properties of one struct, asking jsonFields
// which Go value produced each one. That question has a real answer only
// because encoding/json's shadowing rule is ported there rather than guessed
// at: an embedded struct's fields are promoted into this same object, and a
// promoted `[]string` that lost a name collision to a `*string` must not be
// the field this rewrite consults — that would turn a nil pointer into an
// empty array, which is the same lie in the other direction.
func emptyArrayFields(rv reflect.Value, obj map[string]any) {
	for _, f := range jsonFields(rv.Type()) {
		// An absent property is one `omitempty` dropped, or one an ambiguous
		// name meant nobody wrote; dropping it is what makes the declaration
		// optional, and it is not a null to rewrite.
		tree, present := obj[f.name]
		if !present {
			continue
		}
		value, reached := f.value(rv)
		if !reached {
			continue
		}
		obj[f.name] = emptyArrays(value, tree)
	}
}

// jsonMapKey spells a map key the way encoding/json spells it, so a rewrite can
// find the property the key produced. It reports false for a key encoding/json
// itself refuses, which is a value that never reached the tree at all.
func jsonMapKey(rv reflect.Value) (string, bool) {
	if tm, ok := rv.Interface().(encoding.TextMarshaler); ok {
		if rv.Kind() == reflect.Pointer && rv.IsNil() {
			return "", false
		}
		text, err := tm.MarshalText()
		if err != nil {
			return "", false
		}
		return string(text), true
	}
	switch rv.Kind() {
	case reflect.String:
		return rv.String(), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(rv.Int(), 10), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return strconv.FormatUint(rv.Uint(), 10), true
	}
	return "", false
}

// marshalsItself reports whether a value of rt writes its own JSON, in which
// case the tree beneath it is not derived from its fields and must be left
// exactly as it came.
func marshalsItself(rt reflect.Type) bool {
	if rt.Implements(jsonMarshaler) || rt.Implements(textMarshaler) {
		return true
	}
	// encoding/json takes the address of an addressable value to find a
	// pointer-receiver marshaler, so a struct field of such a type marshals
	// itself too.
	if rt.Kind() != reflect.Pointer {
		p := reflect.PointerTo(rt)
		return p.Implements(jsonMarshaler) || p.Implements(textMarshaler)
	}
	return false
}

var (
	jsonMarshaler = reflect.TypeFor[json.Marshaler]()
	textMarshaler = reflect.TypeFor[encoding.TextMarshaler]()
)
