package remotearg

import (
	"encoding/base64"
	"reflect"
	"strings"
	"testing"

	"github.com/tylergannon/skgo/internal/devalue"
)

func devalueString(t *testing.T, payload string) string {
	t.Helper()
	data, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		t.Fatalf("decoding %q: %v", payload, err)
	}
	return string(data)
}

func queryArg(t *testing.T, v any) string {
	t.Helper()
	payload, err := StringifyQueryArg(v)
	if err != nil {
		t.Fatalf("StringifyQueryArg: %v", err)
	}
	return payload
}

func TestQueryArgGoldens(t *testing.T) {
	cases := []struct {
		name    string
		value   any
		json    string
		payload string
	}{
		{
			name:    "number",
			value:   1,
			json:    "[1]",
			payload: "WzFd",
		},
		{
			name:    "string",
			value:   "hi",
			json:    `["hi"]`,
			payload: "WyJoaSJd",
		},
		{
			name:    "object",
			value:   devalue.NewObject("filter", "author:santa"),
			json:    `[["__skrao",1],{"filter":2},"author:santa"]`,
			payload: "W1siX19za3JhbyIsMV0seyJmaWx0ZXIiOjJ9LCJhdXRob3I6c2FudGEiXQ",
		},
		{
			name:    "set",
			value:   devalue.NewSet(3, 1, 2),
			json:    `[["__skras",1],[2,3,4],"[1]","[2]","[3]"]`,
			payload: "",
		},
	}

	for _, c := range cases {
		payload := queryArg(t, c.value)
		if got := devalueString(t, payload); got != c.json {
			t.Errorf("%s: devalue = %s, want %s", c.name, got, c.json)
		}
		if c.payload != "" && payload != c.payload {
			t.Errorf("%s: payload = %s, want %s", c.name, payload, c.payload)
		}
	}
}

func TestQueryArgUndefinedIsEmpty(t *testing.T) {
	if got := queryArg(t, devalue.Undefined); got != "" {
		t.Errorf("payload = %q, want empty", got)
	}
}

// The tests below are ported from kit's src/runtime/shared.spec.js.

func TestReorderedPlainObjectProperties(t *testing.T) {
	a := queryArg(t, devalue.NewObject("limit", 10, "offset", 20))
	b := queryArg(t, devalue.NewObject("offset", 20, "limit", 10))

	if a != b {
		t.Errorf("%s != %s", a, b)
	}
}

func TestReorderedNestedPlainObjectProperties(t *testing.T) {
	a := queryArg(t, devalue.NewObject("filter", devalue.NewObject(
		"range", devalue.NewObject("min", 1, "max", 5),
		"tags", []any{"a", "b"},
	)))
	b := queryArg(t, devalue.NewObject("filter", devalue.NewObject(
		"tags", []any{"a", "b"},
		"range", devalue.NewObject("max", 5, "min", 1),
	)))

	if a != b {
		t.Errorf("%s != %s", a, b)
	}
}

func TestReorderedNullPrototypeObjectProperties(t *testing.T) {
	a := queryArg(t, devalue.NewNullProtoObject("limit", 10, "offset", 20))
	b := queryArg(t, devalue.NewNullProtoObject("offset", 20, "limit", 10))

	if a != b {
		t.Errorf("%s != %s", a, b)
	}
}

func TestGoMapMatchesSortedObject(t *testing.T) {
	a := queryArg(t, map[string]any{"offset": 20, "limit": 10})
	b := queryArg(t, devalue.NewObject("limit", 10, "offset", 20))

	if a != b {
		t.Errorf("%s != %s", a, b)
	}
}

func TestReorderedMapEntries(t *testing.T) {
	a := queryArg(t, devalue.NewMap(
		"second", devalue.NewMap(
			"y", devalue.NewObject("d", 4, "c", 3),
			"x", devalue.NewObject("b", 2, "a", 1),
		),
		"first", devalue.NewObject("nested", devalue.NewObject("z", 1, "a", 2)),
	))
	b := queryArg(t, devalue.NewMap(
		"first", devalue.NewObject("nested", devalue.NewObject("a", 2, "z", 1)),
		"second", devalue.NewMap(
			"x", devalue.NewObject("a", 1, "b", 2),
			"y", devalue.NewObject("c", 3, "d", 4),
		),
	))

	if a != b {
		t.Errorf("%s != %s", devalueString(t, a), devalueString(t, b))
	}
}

func TestReorderedSetItems(t *testing.T) {
	a := queryArg(t, devalue.NewSet(
		devalue.NewMap(
			"b", devalue.NewObject("y", 2, "x", 1),
			"a", devalue.NewObject("b", 2, "a", 1),
		),
		devalue.NewMap(
			"d", devalue.NewSet(
				devalue.NewObject("d", 4, "c", 3),
				devalue.NewObject("b", 2, "a", 1),
			),
			"c", devalue.NewObject("z", 1, "y", 2),
		),
	))
	b := queryArg(t, devalue.NewSet(
		devalue.NewMap(
			"c", devalue.NewObject("y", 2, "z", 1),
			"d", devalue.NewSet(
				devalue.NewObject("a", 1, "b", 2),
				devalue.NewObject("c", 3, "d", 4),
			),
		),
		devalue.NewMap(
			"a", devalue.NewObject("a", 1, "b", 2),
			"b", devalue.NewObject("x", 1, "y", 2),
		),
	))

	if a != b {
		t.Errorf("%s != %s", devalueString(t, a), devalueString(t, b))
	}
}

func TestDoesNotMutateInput(t *testing.T) {
	nested := devalue.NewObject("b", 2, "a", 1)
	value := devalue.NewObject("z", 1, "nested", nested)

	queryArg(t, value)

	if got, want := value.Keys(), []string{"z", "nested"}; !reflect.DeepEqual(got, want) {
		t.Errorf("keys = %v, want %v", got, want)
	}
	if got, want := nested.Keys(), []string{"b", "a"}; !reflect.DeepEqual(got, want) {
		t.Errorf("nested keys = %v, want %v", got, want)
	}
}

func TestRoundTripsCyclesAndRepeatedReferences(t *testing.T) {
	shared := devalue.NewObject("z", 1, "a", 2)
	value := devalue.NewObject("items", []any{shared, shared})
	value.Set("self", value)

	parsed, present, err := ParsePayload(queryArg(t, value))
	if err != nil {
		t.Fatal(err)
	}
	if !present {
		t.Fatal("expected a value")
	}

	obj := parsed.(*devalue.Object)
	self, _ := obj.Get("self")
	if self != any(obj) {
		t.Error("parsed.self is not parsed")
	}

	items, _ := obj.Get("items")
	array := items.([]any)
	if array[0] != array[1] {
		t.Error("repeated reference was not shared")
	}
	if got, want := array[0].(*devalue.Object).Keys(), []string{"a", "z"}; !reflect.DeepEqual(got, want) {
		t.Errorf("keys = %v, want %v", got, want)
	}
}

func TestRoundTripsBuiltins(t *testing.T) {
	value := devalue.NewObject(
		"date", devalue.Date(mustTime(t, "2024-01-01T00:00:00.000Z")),
		"buffer", devalue.ArrayBuffer([]byte{3, 1, 2}),
		"bigint", devalue.BigInt("12345678901234567890"),
	)

	parsed, _, err := ParsePayload(queryArg(t, value))
	if err != nil {
		t.Fatal(err)
	}
	obj := parsed.(*devalue.Object)

	date, _ := obj.Get("date")
	if got := date.(devalue.Date).Time().UTC().Format("2006-01-02T15:04:05.000Z"); got != "2024-01-01T00:00:00.000Z" {
		t.Errorf("date = %s", got)
	}

	buffer, _ := obj.Get("buffer")
	if got, want := []byte(buffer.(devalue.ArrayBuffer)), []byte{3, 1, 2}; !reflect.DeepEqual(got, want) {
		t.Errorf("buffer = %v, want %v", got, want)
	}

	bigint, _ := obj.Get("bigint")
	if bigint != devalue.BigInt("12345678901234567890") {
		t.Errorf("bigint = %#v", bigint)
	}
}

func TestRejectsRegExp(t *testing.T) {
	_, err := StringifyQueryArg(devalue.RegExp{Source: "a"})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "Regular expressions are not valid remote function arguments") {
		t.Errorf("error = %q", err)
	}

	if _, err := StringifyCommandArg(devalue.RegExp{Source: "a"}); err == nil {
		t.Error("expected an error from StringifyCommandArg")
	}

	// Nested inside a container the guard still fires, including through the
	// nested stringify a Set item goes through.
	if _, err := StringifyQueryArg(devalue.NewObject("re", devalue.RegExp{Source: "a"})); err == nil {
		t.Error("expected an error for a nested regular expression")
	}
	if _, err := StringifyQueryArg(devalue.NewSet(devalue.RegExp{Source: "a"})); err == nil {
		t.Error("expected an error for a regular expression inside a Set")
	}
}

func TestRejectsClassInstances(t *testing.T) {
	type thing struct{ A int }

	_, err := StringifyQueryArg(thing{A: 1})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "Cannot stringify arbitrary non-POJOs") {
		t.Errorf("error = %q", err)
	}
}

func TestRoundTripsSparseArrays(t *testing.T) {
	value := make([]any, 1000001)
	for i := range value {
		value[i] = devalue.Hole
	}
	value[1000000] = devalue.NewObject("b", 2, "a", 1)

	parsed, _, err := ParsePayload(queryArg(t, value))
	if err != nil {
		t.Fatal(err)
	}

	array := parsed.([]any)
	if len(array) != 1000001 {
		t.Fatalf("length = %d, want 1000001", len(array))
	}
	if array[0] != any(devalue.Hole) {
		t.Error("index 0 should be a hole")
	}
	if got, want := array[1000000].(*devalue.Object).Keys(), []string{"a", "b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("keys = %v, want %v", got, want)
	}
}

func TestCommandArgPreservesOrdering(t *testing.T) {
	a, err := StringifyCommandArg(devalue.NewObject("limit", 10, "offset", 20))
	if err != nil {
		t.Fatal(err)
	}
	b, err := StringifyCommandArg(devalue.NewObject("offset", 20, "limit", 10))
	if err != nil {
		t.Fatal(err)
	}

	if a == b {
		t.Error("command arguments should not be canonicalized")
	}
	if got, want := devalueString(t, a), `[{"limit":1,"offset":2},10,20]`; got != want {
		t.Errorf("devalue = %s, want %s", got, want)
	}
}

func TestCommandArgSerializesFiles(t *testing.T) {
	payload, err := StringifyCommandArg(devalue.NewObject("myfile", File{
		Name:         "hello.md",
		Type:         "text/markdown",
		LastModified: -1,
		Data:         []byte("hello"),
	}))
	if err != nil {
		t.Fatal(err)
	}

	want := `[{"myfile":1},["__skraf",2],{"data":3,"lastModified":4,"name":5,"size":6,"type":7},` +
		`["ArrayBuffer","aGVsbG8="],-1,"hello.md",5,"text/markdown"]`
	if got := devalueString(t, payload); got != want {
		t.Errorf("devalue = %s, want %s", got, want)
	}

	parsed, _, err := ParsePayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	file, ok := parsed.(*devalue.Object).Get("myfile")
	if !ok {
		t.Fatal("no myfile property")
	}
	got, ok := file.(File)
	if !ok {
		t.Fatalf("myfile is %T, want File", file)
	}
	if got.Name != "hello.md" || got.Type != "text/markdown" || string(got.Data) != "hello" || got.LastModified != -1 {
		t.Errorf("file = %#v", got)
	}
}

func TestParsePayloadEmpty(t *testing.T) {
	value, present, err := ParsePayload("")
	if err != nil {
		t.Fatal(err)
	}
	if present {
		t.Error("present should be false")
	}
	if value != nil {
		t.Errorf("value = %#v, want nil", value)
	}
}

func TestParsePayloadSelfReferential(t *testing.T) {
	value := devalue.NewObject("z", 1, "a", 2)
	value.Set("self", value)

	parsed, _, err := ParsePayload(queryArg(t, value))
	if err != nil {
		t.Fatal(err)
	}

	obj := parsed.(*devalue.Object)
	self, _ := obj.Get("self")
	if self != any(obj) {
		t.Error("parsed.self is not parsed")
	}
	if got, want := obj.Keys(), []string{"a", "self", "z"}; !reflect.DeepEqual(got, want) {
		t.Errorf("keys = %v, want %v", got, want)
	}
}

func TestParsePayloadRestoresNullPrototypeObjects(t *testing.T) {
	value := devalue.NewNullProtoObject(
		"z", 1,
		"nested", devalue.NewNullProtoObject("b", 2, "a", 1),
	)

	parsed, _, err := ParsePayload(queryArg(t, value))
	if err != nil {
		t.Fatal(err)
	}

	obj := parsed.(*devalue.Object)
	if !obj.NullProto {
		t.Error("expected a null-prototype object")
	}
	if got, want := obj.Keys(), []string{"nested", "z"}; !reflect.DeepEqual(got, want) {
		t.Errorf("keys = %v, want %v", got, want)
	}

	nested, _ := obj.Get("nested")
	nestedObj := nested.(*devalue.Object)
	if !nestedObj.NullProto {
		t.Error("expected a nested null-prototype object")
	}
	if got, want := nestedObj.Keys(), []string{"a", "b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("nested keys = %v, want %v", got, want)
	}
}

func TestParsePayloadMapOrdering(t *testing.T) {
	value := devalue.NewMap(
		"second", devalue.NewMap(
			"y", devalue.NewObject("d", 4, "c", 3),
			"x", devalue.NewSet(
				devalue.NewObject("d", 4, "c", 3),
				devalue.NewObject("b", 2, "a", 1),
			),
		),
		"first", devalue.NewObject("nested", devalue.NewObject("z", 1, "a", 2)),
	)

	parsed, _, err := ParsePayload(queryArg(t, value))
	if err != nil {
		t.Fatal(err)
	}

	m, ok := parsed.(*devalue.Map)
	if !ok {
		t.Fatalf("parsed is %T, want *devalue.Map", parsed)
	}
	if got, want := mapKeys(m), []string{"first", "second"}; !reflect.DeepEqual(got, want) {
		t.Errorf("keys = %v, want %v", got, want)
	}

	first, _ := m.Get("first")
	firstObj := first.(*devalue.Object)
	if got, want := firstObj.Keys(), []string{"nested"}; !reflect.DeepEqual(got, want) {
		t.Errorf("first keys = %v, want %v", got, want)
	}
	nested, _ := firstObj.Get("nested")
	if got, want := nested.(*devalue.Object).Keys(), []string{"a", "z"}; !reflect.DeepEqual(got, want) {
		t.Errorf("nested keys = %v, want %v", got, want)
	}

	second, _ := m.Get("second")
	nestedMap := second.(*devalue.Map)
	if got, want := mapKeys(nestedMap), []string{"x", "y"}; !reflect.DeepEqual(got, want) {
		t.Errorf("nested map keys = %v, want %v", got, want)
	}

	x, _ := nestedMap.Get("x")
	set := x.(*devalue.Set)
	if got, want := objectKeysOf(set.Items()), [][]string{{"a", "b"}, {"c", "d"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("set items = %v, want %v", got, want)
	}
}

func TestParsePayloadSetOrdering(t *testing.T) {
	value := devalue.NewSet(
		devalue.NewMap(
			"b", devalue.NewObject("y", 2, "x", 1),
			"a", devalue.NewObject("b", 2, "a", 1),
		),
		devalue.NewMap(
			"d", devalue.NewSet(
				devalue.NewObject("d", 4, "c", 3),
				devalue.NewObject("b", 2, "a", 1),
			),
			"c", devalue.NewObject("z", 1, "y", 2),
		),
	)

	parsed, _, err := ParsePayload(queryArg(t, value))
	if err != nil {
		t.Fatal(err)
	}

	set := parsed.(*devalue.Set)
	items := set.Items()
	if len(items) != 2 {
		t.Fatalf("%d items, want 2", len(items))
	}

	first := items[0].(*devalue.Map)
	if got, want := mapKeys(first), []string{"a", "b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("first keys = %v, want %v", got, want)
	}

	second := items[1].(*devalue.Map)
	if got, want := mapKeys(second), []string{"c", "d"}; !reflect.DeepEqual(got, want) {
		t.Errorf("second keys = %v, want %v", got, want)
	}
}

func TestPayloadIsURLAndFilenameSafe(t *testing.T) {
	// The payload is used as both a URL segment and a filename, so the base64
	// alphabet has to be the URL-safe one and the padding has to be gone.
	for i := 0; i < 256; i++ {
		value := devalue.NewObject("a", strings.Repeat(string(rune(i)), 3))

		payload := queryArg(t, value)
		if strings.ContainsAny(payload, "+/=") {
			t.Fatalf("payload %q for rune %d is not URL safe", payload, i)
		}

		if _, _, err := ParsePayload(payload); err != nil {
			t.Fatalf("rune %d: %v", i, err)
		}
	}
}

func mapKeys(m *devalue.Map) []string {
	out := make([]string, 0, m.Len())
	for _, e := range m.Entries() {
		out = append(out, e.Key.(string))
	}
	return out
}

func objectKeysOf(items []any) [][]string {
	out := make([][]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.(*devalue.Object).Keys())
	}
	return out
}
