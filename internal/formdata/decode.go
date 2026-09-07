package formdata

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/tylergannon/polytype/devalue"
)

// Decode assigns a parsed form submission onto a typed Go value.
//
// It exists because a form is the one remote kind whose argument cannot go
// through encoding/json: an uploaded file's bytes are a []byte that JSON would
// have to base64 twice, and a File is not a JSON value at all. So the tree is
// walked directly instead, and a File lands on a File field as itself.
//
// Field names follow encoding/json's rules — the `json` tag wins, otherwise
// the field name matches case-insensitively — so a Go struct that already
// describes a query's argument describes a form's the same way.
//
// A field the submission did not carry is left alone rather than zeroed. An
// unchecked checkbox and an empty number input are both simply absent from a
// browser's FormData, so "missing" is the normal case and not an error.
func Decode(data any, into any) error {
	target := reflect.ValueOf(into)
	if target.Kind() != reflect.Pointer || target.IsNil() {
		return fmt.Errorf("skgo: form decode target must be a non-nil pointer, got %T", into)
	}
	return assign(data, target.Elem(), "")
}

// assign writes one node of the tree into dst. path is the dotted field path,
// used only to make a type error say which field it was about.
func assign(node any, dst reflect.Value, path string) error {
	if node == nil {
		return nil
	}
	if _, undefined := node.(devalue.UndefinedValue); undefined {
		return nil
	}
	if _, hole := node.(devalue.HoleValue); hole {
		return nil
	}

	// A File is carried through whole. It is checked before the generic
	// handling below so that a File never falls into the struct branch and
	// gets taken apart field by field.
	if file, ok := node.(File); ok {
		return assignFile(file, dst, path)
	}

	// An interface target takes the tree as it is, which is what a handler
	// that wants the raw submission asks for.
	if dst.Kind() == reflect.Interface && dst.NumMethod() == 0 {
		dst.Set(reflect.ValueOf(node))
		return nil
	}

	if dst.Kind() == reflect.Pointer {
		if dst.IsNil() {
			dst.Set(reflect.New(dst.Type().Elem()))
		}
		return assign(node, dst.Elem(), path)
	}

	switch value := node.(type) {
	case string:
		return assignString(value, dst, path)
	case bool:
		if dst.Kind() != reflect.Bool {
			return typeError(path, "a boolean", dst.Type())
		}
		dst.SetBool(value)
		return nil
	case float64:
		return assignNumber(value, dst, path)
	case []any:
		return assignSlice(value, dst, path)
	case *devalue.Object:
		return assignObject(value, dst, path)
	default:
		return typeError(path, fmt.Sprintf("%T", node), dst.Type())
	}
}

var fileType = reflect.TypeOf(File{})

func assignFile(file File, dst reflect.Value, path string) error {
	if dst.Type() != fileType {
		return typeError(path, "an uploaded file", dst.Type())
	}
	dst.Set(reflect.ValueOf(file))
	return nil
}

// assignString also accepts a numeric or boolean destination, because a form
// control that was not declared with `as('number')` or `as('checkbox')` sends
// its value as text however the Go side has typed it. Refusing would make the
// server's answer depend on a detail of the markup.
func assignString(value string, dst reflect.Value, path string) error {
	switch dst.Kind() {
	case reflect.String:
		dst.SetString(value)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		if strings.TrimSpace(value) == "" {
			return nil
		}
		n, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return typeError(path, strconv.Quote(value), dst.Type())
		}
		return assignNumber(n, dst, path)
	case reflect.Bool:
		// `as('checkbox')` sends "on"; a bare checkbox with no type prefix
		// sends its `value` attribute, which defaults to "on" too.
		dst.SetBool(value == "on" || value == "true")
	default:
		return typeError(path, "text", dst.Type())
	}
	return nil
}

func assignNumber(value float64, dst reflect.Value, path string) error {
	switch dst.Kind() {
	case reflect.Float32, reflect.Float64:
		if dst.OverflowFloat(value) {
			return typeError(path, strconv.FormatFloat(value, 'g', -1, 64), dst.Type())
		}
		dst.SetFloat(value)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n := int64(value)
		if float64(n) != value || dst.OverflowInt(n) {
			return typeError(path, strconv.FormatFloat(value, 'g', -1, 64), dst.Type())
		}
		dst.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n := uint64(value)
		if value < 0 || float64(n) != value || dst.OverflowUint(n) {
			return typeError(path, strconv.FormatFloat(value, 'g', -1, 64), dst.Type())
		}
		dst.SetUint(n)
	case reflect.String:
		dst.SetString(strconv.FormatFloat(value, 'g', -1, 64))
	default:
		return typeError(path, "a number", dst.Type())
	}
	return nil
}

func assignSlice(items []any, dst reflect.Value, path string) error {
	switch dst.Kind() {
	case reflect.Slice:
		out := reflect.MakeSlice(dst.Type(), len(items), len(items))
		for i, item := range items {
			if err := assign(item, out.Index(i), fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
		dst.Set(out)
		return nil
	case reflect.Array:
		for i := 0; i < dst.Len() && i < len(items); i++ {
			if err := assign(items[i], dst.Index(i), fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
		return nil
	default:
		return typeError(path, "a list", dst.Type())
	}
}

func assignObject(obj *devalue.Object, dst reflect.Value, path string) error {
	switch dst.Kind() {
	case reflect.Struct:
		fields := fieldIndex(dst.Type())
		for _, key := range obj.Keys() {
			index, ok := fields[strings.ToLower(key)]
			if !ok {
				// An unknown field is ignored, as encoding/json ignores one.
				continue
			}
			value, _ := obj.Get(key)
			if err := assign(value, dst.FieldByIndex(index), join(path, key)); err != nil {
				return err
			}
		}
		return nil
	case reflect.Map:
		if dst.Type().Key().Kind() != reflect.String {
			return typeError(path, "an object", dst.Type())
		}
		if dst.IsNil() {
			dst.Set(reflect.MakeMap(dst.Type()))
		}
		for _, key := range obj.Keys() {
			value, _ := obj.Get(key)
			entry := reflect.New(dst.Type().Elem()).Elem()
			if err := assign(value, entry, join(path, key)); err != nil {
				return err
			}
			dst.SetMapIndex(reflect.ValueOf(key).Convert(dst.Type().Key()), entry)
		}
		return nil
	default:
		return typeError(path, "an object", dst.Type())
	}
}

// fieldIndex maps a lower-cased wire name to a struct field, following
// encoding/json: the `json` tag renames, "-" hides, and an embedded struct's
// fields are promoted.
func fieldIndex(t reflect.Type) map[string][]int {
	out := map[string][]int{}
	var walk func(reflect.Type, []int)
	walk = func(t reflect.Type, prefix []int) {
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			index := append(append([]int{}, prefix...), i)

			tag, tagged := f.Tag.Lookup("json")
			name, _, _ := strings.Cut(tag, ",")
			if name == "-" && !strings.Contains(tag, ",") {
				continue
			}

			if f.Anonymous && !tagged && f.Type.Kind() == reflect.Struct && f.Type != fileType {
				walk(f.Type, index)
				continue
			}
			if !f.IsExported() {
				continue
			}
			if name == "" {
				name = f.Name
			}
			key := strings.ToLower(name)
			// An outer field wins over a promoted one, as in encoding/json.
			if _, taken := out[key]; taken && len(prefix) > 0 {
				continue
			}
			out[key] = index
		}
	}
	walk(t, nil)
	return out
}

func join(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

func typeError(path, got string, want reflect.Type) error {
	where := "the form"
	if path != "" {
		where = "form field " + strconv.Quote(path)
	}
	return fmt.Errorf("%w: %s carried %s, which does not fit a %s", ErrBadRequest, where, got, want)
}
