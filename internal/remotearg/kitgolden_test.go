package remotearg

import (
	"errors"
	"testing"

	"github.com/tylergannon/skgo/internal/devalue"
)

// The payloads in this file are goldens taken from kit itself, not derived by
// hand. They were produced by running kit's `stringify_remote_arg` and
// `stringify_command_arg` — the reducer code copied verbatim out of the pinned
// ephemeral/inspiration/reference/kit/packages/kit/src/runtime/shared.js, with
// the transport `encoders` map empty — against the pinned devalue 5.9.2 in
// ephemeral/inspiration/reference/devalue, on the Node that `mise x -- node`
// provides inside example/. The recipe is in the worklog.
//
// kit's own shared.spec.js asserts only that two orderings of the same value
// agree; it never pins the bytes. Those equivalence cases are ported in
// remotearg_test.go. These goldens pin the bytes, which is what a refresh key
// built in Go actually has to match.

const (
	// Astral: UTF-16 D83D DE00, UTF-8 F0 9F 98 80.
	emoji = "\U0001F600"
	// BMP private use: UTF-16 E000, UTF-8 EE 80 80. Sorts *after* emoji in
	// UTF-16 (JavaScript) and *before* it in UTF-8 bytes (Go).
	privateUse = "\uE000"
	// Last BMP code point: UTF-16 FFFF, UTF-8 EF BF BF.
	lastBMP = "\uFFFF"
)

func TestKitQueryArgGoldens(t *testing.T) {
	shared := devalue.NewObject("z", 1, "a", 2)
	cycle := devalue.NewObject("items", []any{shared, shared})
	cycle.Set("self", cycle)

	selfRef := devalue.NewObject("z", 1, "a", 2)
	selfRef.Set("self", selfRef)

	sparse := make([]any, 11)
	for i := range sparse {
		sparse[i] = devalue.Hole
	}
	sparse[10] = devalue.NewObject("b", 2, "a", 1)

	cases := []struct {
		name    string
		value   any
		json    string
		payload string
	}{
		{
			name:    "null",
			value:   nil,
			json:    `[null]`,
			payload: "W251bGxd",
		},
		{
			name:    "empty object",
			value:   devalue.NewObject(),
			json:    `[["__skrao",1],{}]`,
			payload: "W1siX19za3JhbyIsMV0se31d",
		},
		{
			name:    "set of numbers",
			value:   devalue.NewSet(3, 1, 2),
			json:    `[["__skras",1],[2,3,4],"[1]","[2]","[3]"]`,
			payload: "W1siX19za3JhcyIsMV0sWzIsMyw0XSwiWzFdIiwiWzJdIiwiWzNdIl0",
		},
		{
			name:    "flat object",
			value:   devalue.NewObject("limit", 10, "offset", 20),
			json:    `[["__skrao",1],{"limit":2,"offset":3},10,20]`,
			payload: "W1siX19za3JhbyIsMV0seyJsaW1pdCI6Miwib2Zmc2V0IjozfSwxMCwyMF0",
		},
		{
			name:    "flat object, reordered",
			value:   devalue.NewObject("offset", 20, "limit", 10),
			json:    `[["__skrao",1],{"limit":2,"offset":3},10,20]`,
			payload: "W1siX19za3JhbyIsMV0seyJsaW1pdCI6Miwib2Zmc2V0IjozfSwxMCwyMF0",
		},
		{
			name: "go map matches the sorted object",
			// Query arguments are canonicalized, so an unordered Go map is a
			// legitimate way to build one.
			value:   map[string]any{"offset": 20, "limit": 10},
			json:    `[["__skrao",1],{"limit":2,"offset":3},10,20]`,
			payload: "W1siX19za3JhbyIsMV0seyJsaW1pdCI6Miwib2Zmc2V0IjozfSwxMCwyMF0",
		},
		{
			name: "nested object",
			value: devalue.NewObject("filter", devalue.NewObject(
				"range", devalue.NewObject("min", 1, "max", 5),
				"tags", []any{"a", "b"},
			)),
			json: `[["__skrao",1],{"filter":2},["__skrao",3],{"range":4,"tags":8},` +
				`["__skrao",5],{"max":6,"min":7},5,1,[9,10],"a","b"]`,
			payload: "W1siX19za3JhbyIsMV0seyJmaWx0ZXIiOjJ9LFsiX19za3JhbyIsM10seyJyYW5nZSI6NCwidGFncyI6OH0s" +
				"WyJfX3NrcmFvIiw1XSx7Im1heCI6NiwibWluIjo3fSw1LDEsWzksMTBdLCJhIiwiYiJd",
		},
		{
			name:    "null-prototype object",
			value:   devalue.NewNullProtoObject("offset", 20, "limit", 10),
			json:    `[["__skrao",1],["null","limit",2,"offset",3],10,20]`,
			payload: "W1siX19za3JhbyIsMV0sWyJudWxsIiwibGltaXQiLDIsIm9mZnNldCIsM10sMTAsMjBd",
		},
		{
			name: "nested maps",
			value: devalue.NewMap(
				"second", devalue.NewMap(
					"y", devalue.NewObject("d", 4, "c", 3),
					"x", devalue.NewObject("b", 2, "a", 1),
				),
				"first", devalue.NewObject("nested", devalue.NewObject("z", 1, "a", 2)),
			),
			json: `[["__skram",1],[2,5],[3,4],"[\"first\"]",` +
				`"[[\"__skrao\",1],{\"nested\":2},[\"__skrao\",3],{\"a\":4,\"z\":5},2,1]",` +
				`[6,7],"[\"second\"]",` +
				`"[[\"__skram\",1],[2,5],[3,4],\"[\\\"x\\\"]\",` +
				`\"[[\\\"__skrao\\\",1],{\\\"a\\\":2,\\\"b\\\":3},1,2]\",[6,7],\"[\\\"y\\\"]\",` +
				`\"[[\\\"__skrao\\\",1],{\\\"c\\\":2,\\\"d\\\":3},3,4]\"]"]`,
			payload: "W1siX19za3JhbSIsMV0sWzIsNV0sWzMsNF0sIltcImZpcnN0XCJdIiwiW1tcIl9fc2tyYW9cIiwxXSx7XCJu" +
				"ZXN0ZWRcIjoyfSxbXCJfX3NrcmFvXCIsM10se1wiYVwiOjQsXCJ6XCI6NX0sMiwxXSIsWzYsN10sIltcInNl" +
				"Y29uZFwiXSIsIltbXCJfX3NrcmFtXCIsMV0sWzIsNV0sWzMsNF0sXCJbXFxcInhcXFwiXVwiLFwiW1tcXFwi" +
				"X19za3Jhb1xcXCIsMV0se1xcXCJhXFxcIjoyLFxcXCJiXFxcIjozfSwxLDJdXCIsWzYsN10sXCJbXFxcInlc" +
				"XFwiXVwiLFwiW1tcXFwiX19za3Jhb1xcXCIsMV0se1xcXCJjXFxcIjoyLFxcXCJkXFxcIjozfSwzLDRdXCJd" +
				"Il0",
		},
		{
			name:    "cycle and repeated reference",
			value:   cycle,
			json:    `[["__skrao",1],{"items":2,"self":1},[3,3],["__skrao",4],{"a":5,"z":6},2,1]`,
			payload: "W1siX19za3JhbyIsMV0seyJpdGVtcyI6Miwic2VsZiI6MX0sWzMsM10sWyJfX3NrcmFvIiw0XSx7ImEiOjUsInoiOjZ9LDIsMV0",
		},
		{
			name:    "self reference",
			value:   selfRef,
			json:    `[["__skrao",1],{"a":2,"self":1,"z":3},2,1]`,
			payload: "W1siX19za3JhbyIsMV0seyJhIjoyLCJzZWxmIjoxLCJ6IjozfSwyLDFd",
		},
		{
			name:    "date",
			value:   devalue.NewObject("date", devalue.Date(mustTime(t, "2024-01-01T00:00:00.000Z"))),
			json:    `[["__skrao",1],{"date":2},["Date","2024-01-01T00:00:00.000Z"]]`,
			payload: "W1siX19za3JhbyIsMV0seyJkYXRlIjoyfSxbIkRhdGUiLCIyMDI0LTAxLTAxVDAwOjAwOjAwLjAwMFoiXV0",
		},
		{
			name:    "sparse array",
			value:   sparse,
			json:    `[[-7,11,10,1],["__skrao",2],{"a":3,"b":4},1,2]`,
			payload: "W1stNywxMSwxMCwxXSxbIl9fc2tyYW8iLDJdLHsiYSI6MywiYiI6NH0sMSwyXQ",
		},
		{
			name:    "array with undefined",
			value:   []any{devalue.Undefined, 1},
			json:    `[[-1,1],1]`,
			payload: "W1stMSwxXSwxXQ",
		},
		{
			name:  "array of objects",
			value: []any{devalue.NewObject("b", 1, "a", 2), devalue.NewObject("d", 3, "c", 4)},
			json: `[[1,5],["__skrao",2],{"a":3,"b":4},2,1,["__skrao",6],` +
				`{"c":7,"d":8},4,3]`,
			payload: "W1sxLDVdLFsiX19za3JhbyIsMl0seyJhIjozLCJiIjo0fSwyLDEsWyJfX3NrcmFvIiw2XSx7ImMiOjcsImQiOjh9LDQsM10",
		},
		{
			// Sorting comes first, but JavaScript then hoists canonical array
			// indices ahead of string keys in ascending numeric order, so the
			// serialized order is 2, 10, B, a — not the sorted order 10, 2,
			// B, a.
			name:    "integer-like keys",
			value:   devalue.NewObject("10", "a", "2", "b", "a", "c", "B", "d"),
			json:    `[["__skrao",1],{"2":2,"10":3,"B":4,"a":5},"b","a","d","c"]`,
			payload: "W1siX19za3JhbyIsMV0seyIyIjoyLCIxMCI6MywiQiI6NCwiYSI6NX0sImIiLCJhIiwiZCIsImMiXQ",
		},
		{
			name:    "non-ascii keys below U+E000",
			value:   devalue.NewObject("é", 1, "z", 2, "A", 3),
			json:    `[["__skrao",1],{"A":2,"z":3,"é":4},3,2,1]`,
			payload: "W1siX19za3JhbyIsMV0seyJBIjoyLCJ6IjozLCLDqSI6NH0sMywyLDFd",
		},

		// The four cases below are the ones issue #2 is about: UTF-8 byte
		// order would put privateUse before emoji, JavaScript does not.
		{
			name:    "astral vs private-use object keys",
			value:   devalue.NewObject(emoji, 1, privateUse, 2),
			json:    `[["__skrao",1],{"` + emoji + `":2,"` + privateUse + `":3},1,2]`,
			payload: "W1siX19za3JhbyIsMV0seyLwn5iAIjoyLCLugIAiOjN9LDEsMl0",
		},
		{
			name:    "astral vs private-use object keys, reordered",
			value:   devalue.NewObject(privateUse, 2, emoji, 1),
			json:    `[["__skrao",1],{"` + emoji + `":2,"` + privateUse + `":3},1,2]`,
			payload: "W1siX19za3JhbyIsMV0seyLwn5iAIjoyLCLugIAiOjN9LDEsMl0",
		},
		{
			name:  "astral vs private-use set items",
			value: devalue.NewSet(privateUse, emoji, "z"),
			json: `[["__skras",1],[2,3,4],"[\"z\"]",` +
				`"[\"` + emoji + `\"]","[\"` + privateUse + `\"]"]`,
			payload: "W1siX19za3JhcyIsMV0sWzIsMyw0XSwiW1wielwiXSIsIltcIvCfmIBcIl0iLCJbXCLugIBcIl0iXQ",
		},
		{
			name:  "astral vs private-use map keys",
			value: devalue.NewMap(privateUse, 1, emoji, 2, "z", 3),
			json: `[["__skram",1],[2,5,8],[3,4],"[\"z\"]","[3]",` +
				`[6,7],"[\"` + emoji + `\"]","[2]",` +
				`[9,10],"[\"` + privateUse + `\"]","[1]"]`,
			payload: "W1siX19za3JhbSIsMV0sWzIsNSw4XSxbMyw0XSwiW1wielwiXSIsIlszXSIsWzYsN10sIltcIvCfmIBcIl0i" +
				"LCJbMl0iLFs5LDEwXSwiW1wi7oCAXCJdIiwiWzFdIl0",
		},
		{
			// Same divergence, but not at the first character, and with U+FFFF
			// (the largest BMP code point) in the mix.
			name:  "astral divergence after a common prefix",
			value: devalue.NewObject("a"+emoji, 1, "a"+privateUse, 2, "a"+lastBMP, 3, "az", 4),
			json: `[["__skrao",1],{"az":2,"a` + emoji + `":3,"a` + privateUse + `":4,` +
				`"a` + lastBMP + `":5},4,1,2,3]`,
			payload: "W1siX19za3JhbyIsMV0seyJheiI6MiwiYfCfmIAiOjMsImHugIAiOjQsImHvv78iOjV9LDQsMSwyLDNd",
		},
		{
			// Astral against astral: UTF-8 and UTF-16 agree here, including
			// when the lead surrogate is shared, so this case must not move.
			name:  "astral against astral",
			value: devalue.NewObject(emoji, 1, "\U0001F601", 2, "\U00010000", 3, "\U0010FFFF", 4),
			json: `[["__skrao",1],{"` + "\U00010000" + `":2,"` + emoji + `":3,"` +
				"\U0001F601" + `":4,"` + "\U0010FFFF" + `":5},3,1,2,4]`,
			payload: "W1siX19za3JhbyIsMV0seyLwkICAIjoyLCLwn5iAIjozLCLwn5iBIjo0LCL0j7-_Ijo1fSwzLDEsMiw0XQ",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			payload := queryArg(t, c.value)
			if got := devalueString(t, payload); got != c.json {
				t.Errorf("devalue = %s\n    want %s", got, c.json)
			}
			if payload != c.payload {
				t.Errorf("payload = %s\n    want %s", payload, c.payload)
			}
		})
	}
}

func TestKitCommandArgGoldens(t *testing.T) {
	cases := []struct {
		name    string
		value   any
		json    string
		payload string
	}{
		{
			name:    "flat object",
			value:   devalue.NewObject("limit", 10, "offset", 20),
			json:    `[{"limit":1,"offset":2},10,20]`,
			payload: "W3sibGltaXQiOjEsIm9mZnNldCI6Mn0sMTAsMjBd",
		},
		{
			// Not canonicalized: a different property order is a different
			// payload.
			name:    "flat object, reordered",
			value:   devalue.NewObject("offset", 20, "limit", 10),
			json:    `[{"offset":1,"limit":2},20,10]`,
			payload: "W3sib2Zmc2V0IjoxLCJsaW1pdCI6Mn0sMjAsMTBd",
		},
		{
			name:    "nested object keeps its order",
			value:   devalue.NewObject("a", devalue.NewObject("z", 1, "a", 2)),
			json:    `[{"a":1},{"z":2,"a":3},1,2]`,
			payload: "W3siYSI6MX0seyJ6IjoyLCJhIjozfSwxLDJd",
		},
		{
			name: "file",
			value: devalue.NewObject("myfile", File{
				Name:         "hello.md",
				Type:         "text/markdown",
				LastModified: -1,
				Data:         []byte("hello"),
			}),
			json: `[{"myfile":1},["__skraf",2],{"data":3,"lastModified":4,"name":5,"size":6,"type":7},` +
				`["ArrayBuffer","aGVsbG8="],-1,"hello.md",5,"text/markdown"]`,
			payload: "W3sibXlmaWxlIjoxfSxbIl9fc2tyYWYiLDJdLHsiZGF0YSI6MywibGFzdE1vZGlmaWVkIjo0LCJuYW1lIjo1" +
				"LCJzaXplIjo2LCJ0eXBlIjo3fSxbIkFycmF5QnVmZmVyIiwiYUdWc2JHOD0iXSwtMSwiaGVsbG8ubWQiLDUs" +
				"InRleHQvbWFya2Rvd24iXQ",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			payload, err := StringifyCommandArg(c.value)
			if err != nil {
				t.Fatalf("StringifyCommandArg: %v", err)
			}
			if got := devalueString(t, payload); got != c.json {
				t.Errorf("devalue = %s\n    want %s", got, c.json)
			}
			if payload != c.payload {
				t.Errorf("payload = %s\n    want %s", payload, c.payload)
			}
		})
	}
}

// A command argument's property order is the caller's, and a Go map has none.
// Sorting it would produce bytes the client never produces, silently, so the
// map is refused instead — issue #3.
func TestCommandArgRejectsGoMaps(t *testing.T) {
	nonEmpty := map[string]any{"z": 1, "a": 2}

	cases := []struct {
		name  string
		value any
	}{
		{"at the root", nonEmpty},
		{"empty at the root", map[string]any{}},
		{"inside an object", devalue.NewObject("filter", nonEmpty)},
		{"inside an array", []any{1, nonEmpty}},
		{"inside a set", devalue.NewSet(nonEmpty)},
		{"as a map value", devalue.NewMap("k", nonEmpty)},
		{"as a map key", devalue.NewMap(nonEmpty, "v")},
		{"two levels down", devalue.NewObject("a", devalue.NewObject("b", []any{nonEmpty}))},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			payload, err := StringifyCommandArg(c.value)
			if err == nil {
				t.Fatalf("expected an error, got payload %q", payload)
			}
			if !errors.Is(err, ErrCommandMap) {
				t.Errorf("error = %v, want ErrCommandMap", err)
			}
		})
	}
}

func TestQueryArgAcceptsGoMaps(t *testing.T) {
	// The query reducers sort, so a Go map is fine there and must stay fine:
	// this is the asymmetry the guard on command arguments creates.
	nested := map[string]any{"z": 1, "a": 2}

	a := queryArg(t, devalue.NewObject("filter", nested))
	b := queryArg(t, devalue.NewObject("filter", devalue.NewObject("a", 2, "z", 1)))

	if a != b {
		t.Errorf("%s != %s", devalueString(t, a), devalueString(t, b))
	}
}

// devalue.Undefined is "no argument"; Go nil is JavaScript `null`. They must
// not collapse into each other in either direction.
func TestUndefinedIsNotNil(t *testing.T) {
	undefined, err := StringifyQueryArg(devalue.Undefined)
	if err != nil {
		t.Fatal(err)
	}
	if undefined != "" {
		t.Errorf("undefined payload = %q, want empty", undefined)
	}

	null := queryArg(t, nil)
	if null == "" {
		t.Fatal("nil must not produce the empty payload")
	}
	if got := devalueString(t, null); got != "[null]" {
		t.Errorf("devalue = %s, want [null]", got)
	}

	command, err := StringifyCommandArg(devalue.Undefined)
	if err != nil {
		t.Fatal(err)
	}
	if command != "" {
		t.Errorf("undefined command payload = %q, want empty", command)
	}

	value, present, err := ParsePayload("")
	if err != nil {
		t.Fatal(err)
	}
	if present || value != nil {
		t.Errorf("ParsePayload(%q) = %#v, %v", "", value, present)
	}

	value, present, err = ParsePayload(null)
	if err != nil {
		t.Fatal(err)
	}
	if !present {
		t.Error("null is a present argument")
	}
	if value != nil {
		t.Errorf("value = %#v, want nil", value)
	}
}
