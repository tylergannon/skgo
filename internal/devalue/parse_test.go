package devalue

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

// TestParseRoundTrip re-serializes every stringify fixture, which pins parse
// against the same goldens: identity, ordering and hole placement all have to
// survive for the bytes to come back identical.
func TestParseRoundTrip(t *testing.T) {
	for _, f := range fixtures() {
		value, err := Parse(f.json, nil)
		if err != nil {
			t.Errorf("%s: Parse(%s): %v", f.name, f.json, err)
			continue
		}
		got, err := Stringify(value)
		if err != nil {
			t.Errorf("%s: Stringify after Parse: %v", f.name, err)
			continue
		}
		if got != f.json {
			t.Errorf("%s: round trip = %s, want %s", f.name, got, f.json)
		}
	}
}

func TestParsePrimitives(t *testing.T) {
	cases := []struct {
		json string
		want any
	}{
		{"-1", Undefined},
		{"[42]", 42.0},
		{"[null]", nil},
		{"[true]", true},
		{`["hi"]`, "hi"},
		{"[0.1]", 0.1},
	}

	for _, c := range cases {
		got, err := Parse(c.json, nil)
		if err != nil {
			t.Fatalf("Parse(%s): %v", c.json, err)
		}
		if got != c.want {
			t.Errorf("Parse(%s) = %#v, want %#v", c.json, got, c.want)
		}
	}

	nan, err := Parse("-3", nil)
	if err != nil {
		t.Fatal(err)
	}
	if f, ok := nan.(float64); !ok || !math.IsNaN(f) {
		t.Errorf("Parse(-3) = %#v, want NaN", nan)
	}

	negZero, err := Parse("-6", nil)
	if err != nil {
		t.Fatal(err)
	}
	if f, ok := negZero.(float64); !ok || f != 0 || !math.Signbit(f) {
		t.Errorf("Parse(-6) = %#v, want -0", negZero)
	}
}

func TestParsePreservesCycles(t *testing.T) {
	value, err := Parse(`[{"self":0}]`, nil)
	if err != nil {
		t.Fatal(err)
	}
	obj, ok := value.(*Object)
	if !ok {
		t.Fatalf("Parse returned %T, want *Object", value)
	}
	self, _ := obj.Get("self")
	if self != any(obj) {
		t.Error("obj.self is not obj")
	}

	value, err = Parse("[[0]]", nil)
	if err != nil {
		t.Fatal(err)
	}
	array, ok := value.([]any)
	if !ok {
		t.Fatalf("Parse returned %T, want []any", value)
	}
	inner, ok := array[0].([]any)
	if !ok {
		t.Fatalf("array[0] is %T, want []any", array[0])
	}
	if reflect.ValueOf(inner).Pointer() != reflect.ValueOf(array).Pointer() {
		t.Error("array[0] is not array")
	}
}

func TestParseSharesRepeatedReferences(t *testing.T) {
	value, err := Parse(`[[1,1],{}]`, nil)
	if err != nil {
		t.Fatal(err)
	}
	array := value.([]any)
	if array[0] != array[1] {
		t.Error("repeated references did not come back as one object")
	}
}

func TestParsePreservesKeyOrder(t *testing.T) {
	value, err := Parse(`[{"z":1,"a":1,"1":1,"0":1},"x"]`, nil)
	if err != nil {
		t.Fatal(err)
	}
	obj := value.(*Object)
	// Canonical array indices are hoisted, as they are in JavaScript.
	want := []string{"0", "1", "z", "a"}
	if got := obj.Keys(); !reflect.DeepEqual(got, want) {
		t.Errorf("keys = %v, want %v", got, want)
	}
}

func TestParseHoles(t *testing.T) {
	value, err := Parse(`[[-2,1,-2],"b"]`, nil)
	if err != nil {
		t.Fatal(err)
	}
	array := value.([]any)
	if len(array) != 3 {
		t.Fatalf("len = %d, want 3", len(array))
	}
	if array[0] != any(Hole) || array[2] != any(Hole) {
		t.Errorf("holes not preserved: %#v", array)
	}
	if array[1] != "b" {
		t.Errorf("array[1] = %#v, want b", array[1])
	}
}

func TestParseRevivers(t *testing.T) {
	revivers := map[string]func(any) (any, error){
		"Custom": func(v any) (any, error) { return []any{"custom", v}, nil },
	}

	value, err := Parse(`[["Custom",1],"payload"]`, revivers)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := value.([]any)
	if !ok || len(got) != 2 || got[0] != "custom" || got[1] != "payload" {
		t.Fatalf("Parse = %#v", value)
	}
}

// TestParseReviverInlinePayload covers devalue's munging of a payload that a
// built-in reducer wrote inline rather than as a slot reference.
func TestParseReviverInlinePayload(t *testing.T) {
	revivers := map[string]func(any) (any, error){
		"Custom": func(v any) (any, error) { return v, nil },
	}

	value, err := Parse(`[["Custom","inline"]]`, revivers)
	if err != nil {
		t.Fatal(err)
	}
	if value != "inline" {
		t.Errorf("Parse = %#v, want inline", value)
	}
}

func TestParseInvalid(t *testing.T) {
	cases := []struct {
		name     string
		json     string
		revivers map[string]func(any) (any, error)
		message  string
	}{
		{name: "ArrayBuffer with non-string value", json: `[["ArrayBuffer",{"length":100}]]`, message: "Invalid ArrayBuffer encoding"},
		{name: "hole", json: "-2", message: "Invalid input"},
		{name: "string", json: `"hello"`, message: "Invalid input"},
		{name: "number", json: "42", message: "Invalid input"},
		{name: "boolean", json: "true", message: "Invalid input"},
		{name: "null", json: "null", message: "Invalid input"},
		{name: "object", json: "{}", message: "Invalid input"},
		{name: "empty array", json: "[]", message: "Invalid input"},
		{name: "prototype pollution", json: `[{"__proto__":1},{}]`, message: "__proto__"},
		{name: "sparse array prototype pollution", json: `[[-7,1,"__proto__",{}]]`, message: "Invalid input"},
		{name: "sparse array non-integer index", json: `[[-7,5,"foo",1]]`, message: "Invalid input"},
		{name: "sparse array negative index", json: `[[-7,5,-1,1]]`, message: "Invalid input"},
		{name: "sparse array out-of-bounds index", json: `[[-7,2,5,1]]`, message: "Invalid input"},
		{name: "sparse array non-integer length", json: `[[-7,"abc"]]`, message: "Invalid input"},
		{name: "sparse array negative length", json: `[[-7,-3]]`, message: "Invalid input"},
		{name: "sparse array float length", json: `[[-7,1.5]]`, message: "Invalid input"},
		{name: "sparse array float index", json: `[[-7,5,1.5,1]]`, message: "Invalid input"},
		{name: "prototype pollution via null-prototype object", json: `[["null","__proto__",1],{}]`, message: "__proto__"},
		{name: "prototype pollution via Object wrapper", json: `[["Object",{"__proto__":1}],{}]`, message: "Invalid input"},
		{name: "bad index", json: `[{"0":1,"toString":"push"},"hello"]`, message: "Invalid input"},
		{name: "custom reviver self-reference", json: `[["Custom",0]]`, revivers: map[string]func(any) (any, error){"Custom": func(v any) (any, error) { return v, nil }}, message: "Invalid circular reference"},
		{name: "unknown type", json: `[["Uint8Array",1],["ArrayBuffer","AQID"]]`, message: "Unknown type Uint8Array"},
		{name: "oversized sparse array", json: `[[-7,4000000,0,1],"x"]`, message: "exceeds the limit"},
	}

	for _, c := range cases {
		_, err := Parse(c.json, c.revivers)
		if err == nil {
			t.Errorf("%s: expected an error", c.name)
			continue
		}
		if !strings.Contains(err.Error(), c.message) {
			t.Errorf("%s: error = %q, want it to contain %q", c.name, err, c.message)
		}
	}
}

func TestParseInvalidJSON(t *testing.T) {
	if _, err := Parse("", nil); err == nil {
		t.Error("expected an error for empty input")
	}
	if _, err := Parse("][", nil); err == nil {
		t.Error("expected an error for invalid JSON")
	}
}
