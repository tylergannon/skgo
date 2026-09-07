package skgo

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
)

// encodeTree turns a typed Go value into the tree devalue serializes, leaving
// values the transport claims in place so that its reducers can still see them.
//
// With no transport it is encodeValue: one `json.Marshal`/`json.Unmarshal`
// round trip, which is how every result reached the wire before there was a
// transport hook and is still how the great majority of one reaches it now. The
// walk below is entered only for a value whose *type* can reach a transported
// type, and it hands every subtree that cannot back to encodeValue. So the
// rules that decide a property's name and whether it appears at all stay
// encoding/json's for every value in the app, transported or not; this file
// only has to agree with encoding/json along the spine.
func (t Transport) encodeTree(v any) (any, error) {
	if len(t) == 0 {
		return encodeValue(v)
	}
	return t.walk(reflect.ValueOf(v))
}

// walk is encodeTree's recursion. An invalid reflect.Value is an untyped nil.
func (t Transport) walk(rv reflect.Value) (any, error) {
	if !rv.IsValid() {
		return nil, nil
	}
	if !t.reaches(rv.Type()) {
		return encodeValue(rv.Interface())
	}
	// The value itself is transported: hand devalue the Go value, whose type
	// its reducer is watching for.
	if _, ok := t.byType(rv.Type()); ok {
		return rv.Interface(), nil
	}

	switch rv.Kind() {
	case reflect.Pointer, reflect.Interface:
		if rv.IsNil() {
			return nil, nil
		}
		return t.walk(rv.Elem())

	case reflect.Slice, reflect.Array:
		if rv.Kind() == reflect.Slice && rv.IsNil() {
			// encoding/json writes null for a nil slice, and encodeValue would
			// have produced nil here too.
			return nil, nil
		}
		out := make([]any, rv.Len())
		for i := range out {
			item, err := t.walk(rv.Index(i))
			if err != nil {
				return nil, err
			}
			out[i] = item
		}
		return out, nil

	case reflect.Map:
		if rv.IsNil() {
			return nil, nil
		}
		if rv.Type().Key().Kind() != reflect.String {
			return nil, fmt.Errorf("skgo: encoding a result: %s has a non-string key and reaches a transported type", rv.Type())
		}
		out := make(map[string]any, rv.Len())
		iter := rv.MapRange()
		for iter.Next() {
			value, err := t.walk(iter.Value())
			if err != nil {
				return nil, err
			}
			out[iter.Key().String()] = value
		}
		return out, nil

	case reflect.Struct:
		// A map, not a *devalue.Object, because that is what encodeValue's
		// round trip produces and devalue sorts a map's keys. Building an
		// ordered object here would put a transported result's properties in
		// Go field order while every other result stayed sorted, and the two
		// would disagree about the same type.
		obj := map[string]any{}
		if err := t.structFields(rv, obj); err != nil {
			return nil, err
		}
		return obj, nil
	}

	// reaches only says true for the kinds above, so this is unreachable for a
	// well-formed type; falling back keeps it honest rather than silent.
	return encodeValue(rv.Interface())
}

// structFields writes rv's JSON properties into obj under the names
// encoding/json would give them, promoting an untagged embedded struct's fields
// rather than nesting them.
func (t Transport) structFields(rv reflect.Value, obj map[string]any) error {
	rt := rv.Type()
	for i := range rt.NumField() {
		field := rt.Field(i)
		tag, hasTag := field.Tag.Lookup("json")
		// `json:"-"` skips the field; `json:"-,"` names it "-".
		if tag == "-" {
			continue
		}
		name, opts, _ := strings.Cut(tag, ",")

		if field.Anonymous && !hasTag && embeddedStruct(field.Type) {
			// An embedded struct with no tag promotes its fields into this
			// object rather than nesting under its type name.
			value := rv.Field(i)
			if field.Type.Kind() == reflect.Pointer {
				if value.IsNil() {
					continue
				}
				value = value.Elem()
			}
			if err := t.structFields(value, obj); err != nil {
				return err
			}
			continue
		}

		if field.PkgPath != "" {
			// Unexported, and encoding/json does not write it.
			continue
		}
		if name == "" {
			name = field.Name
		}

		value := rv.Field(i)
		if strings.Contains(","+opts+",", ",omitempty,") && isEmptyValue(value) {
			continue
		}
		encoded, err := t.walk(value)
		if err != nil {
			return err
		}
		obj[name] = encoded
	}
	return nil
}

// embeddedStruct reports whether an anonymous field is one encoding/json
// promotes: a struct, or a pointer to one.
func embeddedStruct(rt reflect.Type) bool {
	if rt.Kind() == reflect.Pointer {
		rt = rt.Elem()
	}
	return rt.Kind() == reflect.Struct
}

// isEmptyValue is encoding/json's `omitempty` test.
func isEmptyValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Pointer:
		return v.IsNil()
	}
	return false
}

// byType finds the transporter that claims rt, if any.
func (t Transport) byType(rt reflect.Type) (Transporter, bool) {
	for _, key := range t.keys() {
		if t[key].Type == rt {
			return t[key], true
		}
	}
	return Transporter{}, false
}

// reachCache memoizes reaches per transport. The answer is a property of a
// (transport, type) pair and a walk asks it once per value, so a result type
// with a thousand rows asks it a thousand times.
var reachCache sync.Map // map[reachKey]bool

type reachKey struct {
	transport any // the map's identity, via a pointer-shaped key
	rt        reflect.Type
}

// reaches reports whether a value of rt can hold a transported value anywhere
// inside it. It is the whole reason the walk is cheap: a type that cannot reach
// one is handed to encoding/json untouched.
//
// A type it cannot see through — an interface, whose dynamic type is not known
// until there is a value — is reachable by assumption. Being wrong that way
// costs a walk; being wrong the other way would silently drop a transported
// value back to a plain object, which is the bug this whole file exists to fix.
func (t Transport) reaches(rt reflect.Type) bool {
	key := reachKey{transport: reflect.ValueOf(t).UnsafePointer(), rt: rt}
	if cached, ok := reachCache.Load(key); ok {
		return cached.(bool)
	}
	result := t.reachesWith(rt, map[reflect.Type]bool{})
	reachCache.Store(key, result)
	return result
}

// reachesWith is reaches, carrying the set of types already on the stack so a
// recursive type terminates. A cycle is not a reason on its own: the answer for
// a type already being decided is false, and any real reachability is found on
// another branch.
func (t Transport) reachesWith(rt reflect.Type, seen map[reflect.Type]bool) bool {
	if rt == nil {
		return false
	}
	if _, ok := t.byType(rt); ok {
		return true
	}
	if seen[rt] {
		return false
	}
	seen[rt] = true
	defer delete(seen, rt)

	switch rt.Kind() {
	case reflect.Interface:
		return true
	case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
		return t.reachesWith(rt.Elem(), seen)
	case reflect.Struct:
		for i := range rt.NumField() {
			field := rt.Field(i)
			if field.PkgPath != "" && !field.Anonymous {
				continue
			}
			if field.Tag.Get("json") == "-" {
				continue
			}
			if t.reachesWith(field.Type, seen) {
				return true
			}
		}
	}
	return false
}
