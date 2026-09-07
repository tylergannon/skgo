package formdata

import (
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/tylergannon/polytype/devalue"
)

// The cases below are kit's own, from
// `packages/kit/src/runtime/form-utils.spec.js` at @sveltejs/kit
// 3.0.0-next.25. A submission a browser makes on its own has to arrive at a Go
// handler as the object kit's client would have built from the same controls,
// or the handler answers two different arguments depending on whether the
// visitor had scripting on.

func text(name, value string) Entry { return Entry{Name: name, Value: value} }

func TestParseFormKeyNormalizesPrefixesAndSuffixes(t *testing.T) {
	field, err := ParseFormKey("form", "n:items[]/form")
	if err != nil {
		t.Fatalf("ParseFormKey: %v", err)
	}
	if want := (Field{Name: "items", Type: "number", IsArray: true}); field != want {
		t.Errorf("got %+v, want %+v", field, want)
	}

	field, err = ParseFormKey("form", "b:enabled/form")
	if err != nil {
		t.Fatalf("ParseFormKey: %v", err)
	}
	if want := (Field{Name: "enabled", Type: "boolean"}); field != want {
		t.Errorf("got %+v, want %+v", field, want)
	}
}

func TestParseFormKeyRejectsAFieldOfAnotherForm(t *testing.T) {
	if _, err := ParseFormKey("form", "foo/other"); !errors.Is(err, ErrBadRequest) {
		t.Fatalf("a field named for another form was accepted: %v", err)
	}
}

func TestConvertCoercesTypedValues(t *testing.T) {
	got := convert(t, "form",
		text("n:count/form", "42"),
		text("n:items[]/form", "1"),
		text("n:items[]/form", "2"),
		text("b:enabled/form", "on"),
	)

	assertField(t, got, "count", float64(42))
	assertField(t, got, "items", []any{float64(1), float64(2)})
	assertField(t, got, "enabled", true)
}

func TestConvertNestsAndGroups(t *testing.T) {
	got := convert(t, "form",
		text("foo/form", "foo"),
		text("object.nested.property/form", "property"),
		text("array[]/form", "a"),
		text("array[]/form", "b"),
		text("array[]/form", "c"),
		text("user.name.first/form", "first"),
		text("user.name.last/form", "last"),
	)

	assertField(t, got, "foo", "foo")
	assertField(t, got, "array", []any{"a", "b", "c"})

	object := prop(t, got, "object").(*devalue.Object)
	nested := prop(t, object, "nested").(*devalue.Object)
	assertField(t, nested, "property", "property")

	user := prop(t, got, "user").(*devalue.Object)
	name := prop(t, user, "name").(*devalue.Object)
	assertField(t, name, "first", "first")
	assertField(t, name, "last", "last")
}

// An `<input type="file">` the visitor left alone submits a nameless zero-byte
// file. Kit drops it; a real zero-byte file with a name is kept, because the
// visitor chose it.
func TestConvertOmitsAnEmptyFileInputAndKeepsARealEmptyFile(t *testing.T) {
	got := convert(t, "form", Entry{Name: "file/form", File: &File{}})
	if got.Len() != 0 {
		t.Fatalf("an empty file input reached the handler: %v", got.Keys())
	}

	got = convert(t, "form", Entry{Name: "file/form", File: &File{Name: "empty.txt"}})
	if file, ok := prop(t, got, "file").(File); !ok || file.Name != "empty.txt" {
		t.Fatalf("a chosen zero-byte file did not reach the handler: %#v", prop(t, got, "file"))
	}
}

func TestConvertRefusesDuplicatesOfAFieldThatIsNotAnArray(t *testing.T) {
	_, err := Convert("form", []Entry{text("foo/form", "a"), text("foo/form", "b")})
	if !errors.Is(err, ErrBadRequest) {
		t.Fatalf("two values under one non-array name were accepted: %v", err)
	}
}

func TestConvertRefusesPrototypePollution(t *testing.T) {
	for _, attack := range []string{
		"__proto__.polluted",
		"constructor.polluted",
		"prototype.polluted",
		"user.__proto__.polluted",
		"user.constructor.polluted",
	} {
		if _, err := Convert("form", []Entry{text(attack+"/form", "bad")}); !errors.Is(err, ErrBadRequest) {
			t.Errorf("%s was accepted: %v", attack, err)
		}
	}
}

func TestConvertRefusesAPathTheFieldProxyCouldNotHaveMade(t *testing.T) {
	for _, path := range []string{"[0]", "foo.0", "foo[bar]"} {
		if _, err := Convert("form", []Entry{text(path+"/form", "x")}); !errors.Is(err, ErrBadRequest) {
			t.Errorf("%s was accepted: %v", path, err)
		}
	}
}

// An empty number input is absent rather than zero, and an unparseable one is
// NaN — `parseFloat(”)` and `parseFloat('nope')` in kit's `coerce_form_value`.
func TestConvertAnEmptyNumberIsAbsentAndAnUnparseableOneIsNotANumber(t *testing.T) {
	got := convert(t, "form", text("n:age/form", ""), text("n:weight/form", "nope"))

	if _, undefined := prop(t, got, "age").(devalue.UndefinedValue); !undefined {
		t.Errorf("an empty number input arrived as %#v, want undefined", prop(t, got, "age"))
	}
	if n, ok := prop(t, got, "weight").(float64); !ok || !math.IsNaN(n) {
		t.Errorf("an unparseable number arrived as %#v, want NaN", prop(t, got, "weight"))
	}
}

// The `input` a refused submission carries back is not coerced and is not
// deduplicated: it refills controls, which hold text.
func TestConvertRawKeepsTextAsText(t *testing.T) {
	got, err := ConvertRaw("form", []Entry{
		text("n:count/form", "42"),
		text("b:enabled/form", "on"),
		Entry{Name: "file/form", File: &File{Name: "haiku.txt", Data: []byte("x")}},
	})
	if err != nil {
		t.Fatalf("ConvertRaw: %v", err)
	}

	assertField(t, got, "count", "42")
	assertField(t, got, "enabled", "on")
	if _, undefined := prop(t, got, "file").(devalue.UndefinedValue); !undefined {
		t.Errorf("a file was echoed back into the controls as %#v", prop(t, got, "file"))
	}
}

func convert(t *testing.T, formID string, entries ...Entry) *devalue.Object {
	t.Helper()
	got, err := Convert(formID, entries)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	object, ok := got.(*devalue.Object)
	if !ok {
		t.Fatalf("Convert produced %T, want an object", got)
	}
	return object
}

func prop(t *testing.T, object *devalue.Object, key string) any {
	t.Helper()
	value, ok := object.Get(key)
	if !ok {
		t.Fatalf("no %q in %v", key, object.Keys())
	}
	return value
}

func assertField(t *testing.T, object *devalue.Object, key string, want any) {
	t.Helper()
	if got := prop(t, object, key); !reflect.DeepEqual(got, want) {
		t.Errorf("%s = %#v, want %#v", key, got, want)
	}
}
