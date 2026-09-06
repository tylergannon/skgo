// Package remotearg encodes and decodes the argument of a SvelteKit remote
// function.
//
// It is a port of `stringify_remote_arg`, `stringify_command_arg` and
// `parse_remote_arg` from kit's `src/runtime/shared.js`. The payload is the
// base64url form (no padding) of a devalue string, and it doubles as a URL
// segment and a filename, so query/live/prerender arguments are canonicalized:
// object keys, Map entries and Set items are sorted, and two arguments that
// differ only in ordering produce the same payload. Command arguments are not
// sorted, because a command's argument is not a cache key.
package remotearg

import (
	"encoding/base64"
	"errors"
	"reflect"
	"sort"
	"strings"

	"github.com/tylergannon/skgo/internal/devalue"
)

// Reducer/reviver keys, spelled "sveltekit remote arg" in kit.
const (
	tagObject = "__skrao"
	tagMap    = "__skram"
	tagSet    = "__skras"
	tagFile   = "__skraf"
	tagRegExp = "__skrag"
)

// ErrRegExp is returned when a regular expression appears in an argument.
var ErrRegExp = errors.New("Regular expressions are not valid remote function arguments")

// File is a JavaScript File, the one non-JSON value a command argument may
// carry. Size is derived from Data.
type File struct {
	Name         string
	Type         string
	LastModified int64
	Data         []byte
}

// ParsePayload decodes a base64url (no padding) devalue payload from a remote
// request. present is false when payload is "" — kit's `undefined` — and when
// the payload decodes to undefined.
func ParsePayload(payload string) (value any, present bool, err error) {
	if payload == "" {
		return nil, false, nil
	}

	data, err := decodeBase64URL(payload)
	if err != nil {
		return nil, false, err
	}

	v, err := devalue.Parse(string(data), revivers())
	if err != nil {
		return nil, false, err
	}
	if _, undefined := v.(devalue.UndefinedValue); undefined {
		return nil, false, nil
	}

	return v, true, nil
}

// StringifyQueryArg encodes a value the way kit's stringify_remote_arg does
// (sorted reducers) and returns the base64url payload. devalue.Undefined
// encodes to the empty payload.
func StringifyQueryArg(v any) (string, error) {
	return stringifyArg(v, queryReducers())
}

// StringifyCommandArg mirrors kit's stringify_command_arg: the same guards, but
// no canonical ordering, and File values are supported.
func StringifyCommandArg(v any) (string, error) {
	return stringifyArg(v, commandReducers())
}

func stringifyArg(v any, reducers []devalue.Reducer) (string, error) {
	if _, undefined := v.(devalue.UndefinedValue); undefined {
		return "", nil
	}

	json, err := devalue.StringifyWith(v, reducers)
	if err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString([]byte(json)), nil
}

func decodeBase64URL(payload string) ([]byte, error) {
	// kit strips padding on the way out; atob tolerates it on the way back in.
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(payload, "="))
}

// regexpGuard rejects regular expressions, which cannot round-trip through a
// remote function argument.
func regexpGuard() devalue.Reducer {
	return devalue.Reducer{
		Key: tagRegExp,
		Fn: func(v any) (any, bool, error) {
			switch v.(type) {
			case devalue.RegExp, *devalue.RegExp:
				return nil, false, ErrRegExp
			}
			return nil, false, nil
		},
	}
}

func fileReducer() devalue.Reducer {
	return devalue.Reducer{
		Key: tagFile,
		Fn: func(v any) (any, bool, error) {
			f, ok := asFile(v)
			if !ok {
				return nil, false, nil
			}
			// Key order matches kit's object literal.
			return devalue.NewObject(
				"data", devalue.ArrayBuffer(f.Data),
				"lastModified", float64(f.LastModified),
				"name", f.Name,
				"size", float64(len(f.Data)),
				"type", f.Type,
			), true, nil
		},
	}
}

func asFile(v any) (File, bool) {
	switch t := v.(type) {
	case File:
		return t, true
	case *File:
		if t == nil {
			return File{}, false
		}
		return *t, true
	}
	return File{}, false
}

func commandReducers() []devalue.Reducer {
	return []devalue.Reducer{regexpGuard(), fileReducer()}
}

// queryReducers builds the canonicalizing reducers. They share one clone table,
// exactly as kit's create_remote_arg_reducers(true) does, so a cyclical object
// graph clones each node once.
func queryReducers() []devalue.Reducer {
	clones := make(map[any]*devalue.Object)
	cloned := make(map[*devalue.Object]bool)

	var reducers []devalue.Reducer

	// Map entries and Set items are compared as nested devalue strings, which
	// is what makes their ordering canonical.
	stringify := func(v any) (string, error) {
		return devalue.StringifyWith(v, reducers)
	}

	reducers = []devalue.Reducer{
		regexpGuard(),
		{
			Key: tagMap,
			Fn: func(v any) (any, bool, error) {
				m, ok := v.(*devalue.Map)
				if !ok {
					return nil, false, nil
				}

				entries := make([][2]string, 0, m.Len())
				for _, e := range m.Entries() {
					key, err := stringify(e.Key)
					if err != nil {
						return nil, false, err
					}
					value, err := stringify(e.Value)
					if err != nil {
						return nil, false, err
					}
					entries = append(entries, [2]string{key, value})
				}

				sort.Slice(entries, func(i, j int) bool {
					if entries[i][0] != entries[j][0] {
						return entries[i][0] < entries[j][0]
					}
					return entries[i][1] < entries[j][1]
				})

				out := make([]any, len(entries))
				for i, e := range entries {
					out[i] = []any{e[0], e[1]}
				}

				return out, true, nil
			},
		},
		{
			Key: tagSet,
			Fn: func(v any) (any, bool, error) {
				s, ok := v.(*devalue.Set)
				if !ok {
					return nil, false, nil
				}

				items := make([]string, 0, s.Len())
				for _, item := range s.Items() {
					str, err := stringify(item)
					if err != nil {
						return nil, false, err
					}
					items = append(items, str)
				}
				sort.Strings(items)

				out := make([]any, len(items))
				for i, item := range items {
					out[i] = item
				}

				return out, true, nil
			},
		},
		{
			Key: tagObject,
			Fn: func(v any) (any, bool, error) {
				if obj, ok := v.(*devalue.Object); ok && cloned[obj] {
					// Already canonical; let devalue serialize it plainly.
					return nil, false, nil
				}

				keys, get, nullProto, ok := plainView(v)
				if !ok {
					return nil, false, nil
				}

				id, hasID := identity(v)
				if hasID {
					if clone, ok := clones[id]; ok {
						return clone, true, nil
					}
				}

				var clone *devalue.Object
				if nullProto {
					clone = devalue.NewNullProtoObject()
				} else {
					clone = devalue.NewObject()
				}
				if hasID {
					clones[id] = clone
				}
				cloned[clone] = true

				sorted := append([]string(nil), keys...)
				sort.Strings(sorted)

				for _, k := range sorted {
					property := get(k)
					if pid, ok := identity(property); ok {
						if c, ok := clones[pid]; ok {
							property = c
						}
					}
					clone.Set(k, property)
				}

				return clone, true, nil
			},
		},
	}

	return reducers
}

// plainView exposes the keys and properties of a plain object, whether it
// arrived as a *devalue.Object or a Go map.
func plainView(v any) (keys []string, get func(string) any, nullProto bool, ok bool) {
	switch t := v.(type) {
	case *devalue.Object:
		return t.Keys(), func(k string) any {
			value, _ := t.Get(k)
			return value
		}, t.NullProto, true
	case map[string]any:
		keys = make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		return keys, func(k string) any { return t[k] }, false, true
	}
	return nil, nil, false, false
}

// identity is a comparable stand-in for object identity, so that a value
// cloned once is not cloned again.
func identity(v any) (any, bool) {
	switch t := v.(type) {
	case *devalue.Object:
		return t, true
	case map[string]any:
		if len(t) == 0 {
			return nil, false
		}
		return reflect.ValueOf(t).Pointer(), true
	}
	return nil, false
}

func revivers() map[string]func(any) (any, error) {
	var all map[string]func(any) (any, error)

	parse := func(s string) (any, error) {
		return devalue.Parse(s, all)
	}

	all = map[string]func(any) (any, error){
		tagObject: func(v any) (any, error) { return v, nil },

		tagMap: func(v any) (any, error) {
			items, ok := v.([]any)
			if !ok {
				return nil, errors.New("Invalid data for Map reviver")
			}

			m := devalue.NewMap()
			for _, item := range items {
				pair, ok := item.([]any)
				if !ok || len(pair) != 2 {
					return nil, errors.New("Invalid data for Map reviver")
				}
				key, keyOK := pair[0].(string)
				value, valueOK := pair[1].(string)
				if !keyOK || !valueOK {
					return nil, errors.New("Invalid data for Map reviver")
				}
				k, err := parse(key)
				if err != nil {
					return nil, err
				}
				val, err := parse(value)
				if err != nil {
					return nil, err
				}
				m.Set(k, val)
			}

			return m, nil
		},

		tagSet: func(v any) (any, error) {
			items, ok := v.([]any)
			if !ok {
				return nil, errors.New("Invalid data for Set reviver")
			}

			s := devalue.NewSet()
			for _, item := range items {
				str, ok := item.(string)
				if !ok {
					return nil, errors.New("Invalid data for Set reviver")
				}
				value, err := parse(str)
				if err != nil {
					return nil, err
				}
				s.Add(value)
			}

			return s, nil
		},

		tagFile: func(v any) (any, error) {
			obj, ok := v.(*devalue.Object)
			if !ok {
				return nil, errors.New("Invalid data for File reviver")
			}

			name, nameOK := property[string](obj, "name")
			mime, mimeOK := property[string](obj, "type")
			modified, modifiedOK := property[float64](obj, "lastModified")
			_, sizeOK := property[float64](obj, "size")
			data, dataOK := property[devalue.ArrayBuffer](obj, "data")

			if !nameOK || !mimeOK || !modifiedOK || !sizeOK || !dataOK {
				return nil, errors.New("Invalid data for File reviver")
			}

			return File{
				Name:         name,
				Type:         mime,
				LastModified: int64(modified),
				Data:         []byte(data),
			}, nil
		},
	}

	return all
}

func property[T any](obj *devalue.Object, key string) (T, bool) {
	var zero T
	v, ok := obj.Get(key)
	if !ok {
		return zero, false
	}
	t, ok := v.(T)
	if !ok {
		return zero, false
	}
	return t, true
}
