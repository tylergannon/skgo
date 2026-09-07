package skgo

import (
	"cmp"
	"reflect"
	"slices"
	"strings"
	"sync"
	"unicode"
)

// jsonFields answers, for a struct type, the one question two encoders in this
// package both have to ask: which properties does encoding/json write for it,
// and which Go value is behind each one.
//
// The answer is not "the exported fields in declaration order". An untagged
// embedded struct's fields are promoted into the same object, and when a
// promoted name collides with another, encoding/json does not take the first it
// walks past: the shallower field wins, a tagged field beats an untagged one at
// the same depth, and a tie at the same depth with the same taggedness is
// dropped from the object entirely.
//
// Getting that wrong is not cosmetic here. emptyArrays rewrites a null into
// `[]` wherever the Go type is a slice, so an encoder that thinks a shadowed
// `[]string` produced the property rewrites the *shadowing* field's null — a
// nil pointer reaching the client as an empty array, which is the exact class
// of lie this package exists to stop. So this is a port of `typeFields` from
// $GOROOT/src/encoding/json/encode.go rather than a resemblance to it, minus
// only the parts that are about writing bytes: HTML escaping, the `,string`
// option, case-folded name lookup, and the per-field encoder.
type jsonField struct {
	// name is the property encoding/json writes.
	name string
	// tagged records that name came from a `json:"..."` tag, which is the
	// tiebreak between two fields at the same depth.
	tagged bool
	// index is the chain of field indexes reaching the value, through any
	// embedded structs it was promoted out of.
	index []int
	// omitEmpty and omitZero are the tag options that can drop the property.
	omitEmpty bool
	omitZero  bool
	// isZero is `omitzero`'s test when the field's type has an IsZero method.
	isZero func(reflect.Value) bool
}

// value finds the Go value behind the property, following the index chain the
// way encoding/json's struct encoder does. It reports false when an embedded
// pointer along the way is nil, which is a property encoding/json skips.
func (f jsonField) value(rv reflect.Value) (reflect.Value, bool) {
	for _, i := range f.index {
		if rv.Kind() == reflect.Pointer {
			if rv.IsNil() {
				return reflect.Value{}, false
			}
			rv = rv.Elem()
		}
		rv = rv.Field(i)
	}
	return rv, true
}

// omit reports whether encoding/json drops the property for this value.
func (f jsonField) omit(rv reflect.Value) bool {
	if f.omitEmpty && isEmptyValue(rv) {
		return true
	}
	if f.omitZero {
		if f.isZero != nil {
			return f.isZero(rv)
		}
		return rv.IsZero()
	}
	return false
}

// jsonFields is typeFields with encoding/json's own cache in front of it: the
// answer is a property of the type alone, and a result with a thousand rows
// asks for it a thousand times.
func jsonFields(rt reflect.Type) []jsonField {
	if cached, ok := jsonFieldCache.Load(rt); ok {
		return cached.([]jsonField)
	}
	fields := typeJSONFields(rt)
	jsonFieldCache.Store(rt, fields)
	return fields
}

var jsonFieldCache sync.Map // map[reflect.Type][]jsonField

// typeJSONFields is encoding/json's typeFields: a breadth-first sweep of the
// embedding tree, so that everything found in one round is at the same depth,
// followed by the annihilation pass that resolves collisions.
func typeJSONFields(rt reflect.Type) []jsonField {
	// An embedded struct queued to be explored, and the index chain that
	// reached it.
	type embedded struct {
		index []int
		typ   reflect.Type
	}

	current := []embedded{}
	next := []embedded{{typ: rt}}

	// Count of queued names for the current level and the next.
	var count, nextCount map[reflect.Type]int

	visited := map[reflect.Type]bool{}
	var fields []jsonField

	for len(next) > 0 {
		current, next = next, current[:0]
		count, nextCount = nextCount, map[reflect.Type]int{}

		for _, e := range current {
			if visited[e.typ] {
				continue
			}
			visited[e.typ] = true

			for i := range e.typ.NumField() {
				sf := e.typ.Field(i)
				if sf.Anonymous {
					t := sf.Type
					if t.Kind() == reflect.Pointer {
						t = t.Elem()
					}
					if !sf.IsExported() && t.Kind() != reflect.Struct {
						// An embedded field of an unexported non-struct type is
						// ignored; an unexported struct type is not, because it
						// may have exported fields.
						continue
					}
				} else if !sf.IsExported() {
					continue
				}
				tag := sf.Tag.Get("json")
				if tag == "-" {
					continue
				}
				name, opts, _ := strings.Cut(tag, ",")
				if !validJSONTag(name) {
					name = ""
				}
				index := make([]int, len(e.index)+1)
				copy(index, e.index)
				index[len(e.index)] = i

				ft := sf.Type
				if ft.Name() == "" && ft.Kind() == reflect.Pointer {
					ft = ft.Elem()
				}

				if name != "" || !sf.Anonymous || ft.Kind() != reflect.Struct {
					tagged := name != ""
					if name == "" {
						name = sf.Name
					}
					f := jsonField{
						name:      name,
						tagged:    tagged,
						index:     index,
						omitEmpty: hasTagOption(opts, "omitempty"),
						omitZero:  hasTagOption(opts, "omitzero"),
					}
					if f.omitZero {
						f.isZero = isZeroFunc(sf.Type)
					}
					fields = append(fields, f)
					if count[e.typ] > 1 {
						// There were multiple instances of this embedded type,
						// so add a second copy for the annihilation pass to see
						// as a duplicate. It only cares about 1 versus 2.
						fields = append(fields, fields[len(fields)-1])
					}
					continue
				}

				// An untagged embedded struct: explore it in the next round.
				nextCount[ft]++
				if nextCount[ft] == 1 {
					next = append(next, embedded{index: index, typ: ft})
				}
			}
		}
	}

	slices.SortFunc(fields, func(a, b jsonField) int {
		// By name, then depth, then "the name came from a tag", then the index
		// sequence — which is the order dominantField reads.
		if c := strings.Compare(a.name, b.name); c != 0 {
			return c
		}
		if c := cmp.Compare(len(a.index), len(b.index)); c != 0 {
			return c
		}
		if a.tagged != b.tagged {
			if a.tagged {
				return -1
			}
			return +1
		}
		return slices.Compare(a.index, b.index)
	})

	// Delete the fields hidden by Go's embedding rules, except that a tagged
	// field is promoted over an untagged one at the same depth.
	out := fields[:0]
	for advance, i := 0, 0; i < len(fields); i += advance {
		fi := fields[i]
		name := fi.name
		for advance = 1; i+advance < len(fields); advance++ {
			if fields[i+advance].name != name {
				break
			}
		}
		if advance == 1 {
			out = append(out, fi)
			continue
		}
		if dominant, ok := dominantJSONField(fields[i : i+advance]); ok {
			out = append(out, dominant)
		}
	}
	fields = out

	// Back into the order the struct declares, so an object built by walking
	// this list is built in the order encoding/json writes it.
	slices.SortFunc(fields, func(a, b jsonField) int {
		return slices.Compare(a.index, b.index)
	})
	return fields
}

// dominantJSONField picks the single field that survives among fields that all
// share a name. They arrive sorted by depth and then by taggedness, so the
// first is the winner unless the first two tie on both, which is an error in Go
// and drops every one of them.
func dominantJSONField(fields []jsonField) (jsonField, bool) {
	if len(fields) > 1 && len(fields[0].index) == len(fields[1].index) && fields[0].tagged == fields[1].tagged {
		return jsonField{}, false
	}
	return fields[0], true
}

// validJSONTag is encoding/json's isValidTag: a tag name it refuses falls back
// to the Go field name rather than naming the property.
func validJSONTag(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		switch {
		case strings.ContainsRune("!#$%&()*+-./:;<=>?@[]^_{|}~ ", c):
			// Punctuation is allowed; backslash and quote are reserved.
		case !unicode.IsLetter(c) && !unicode.IsDigit(c):
			return false
		}
	}
	return true
}

// hasTagOption is encoding/json's tagOptions.Contains.
func hasTagOption(opts, name string) bool {
	for opts != "" {
		var opt string
		opt, opts, _ = strings.Cut(opts, ",")
		if opt == name {
			return true
		}
	}
	return false
}

// isZeroer is the interface `omitzero` consults, and isZeroFunc is the switch
// encoding/json builds around it.
type isZeroer interface{ IsZero() bool }

var isZeroerType = reflect.TypeFor[isZeroer]()

func isZeroFunc(rt reflect.Type) func(reflect.Value) bool {
	switch {
	case rt.Kind() == reflect.Interface && rt.Implements(isZeroerType):
		return func(v reflect.Value) bool {
			// A nil interface, or a non-nil one holding a nil pointer, would
			// panic on the call.
			return v.IsNil() ||
				(v.Elem().Kind() == reflect.Pointer && v.Elem().IsNil()) ||
				v.Interface().(isZeroer).IsZero()
		}
	case rt.Kind() == reflect.Pointer && rt.Implements(isZeroerType):
		return func(v reflect.Value) bool {
			return v.IsNil() || v.Interface().(isZeroer).IsZero()
		}
	case rt.Implements(isZeroerType):
		return func(v reflect.Value) bool {
			return v.Interface().(isZeroer).IsZero()
		}
	case reflect.PointerTo(rt).Implements(isZeroerType):
		return func(v reflect.Value) bool {
			if !v.CanAddr() {
				v2 := reflect.New(v.Type()).Elem()
				v2.Set(v)
				v = v2
			}
			return v.Addr().Interface().(isZeroer).IsZero()
		}
	}
	return nil
}

// isEmptyValue is encoding/json's `omitempty` test.
func isEmptyValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64,
		reflect.Interface, reflect.Pointer:
		return v.IsZero()
	}
	return false
}
