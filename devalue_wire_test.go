package skgo

import (
	"math"
	"testing"
	"time"

	"github.com/tylergannon/polytype/devalue"
)

// The wire skgo speaks is devalue's, and skgo does not implement it: the codec
// is github.com/tylergannon/polytype/devalue. This file is the anchor that says
// the codec skgo compiles against agrees with the real thing on every shape
// skgo emits or accepts.
//
// The `wire` bytes below were produced by the pinned JavaScript devalue 5.9.2 —
// ephemeral/inspiration/reference/devalue/index.js, the version kit's ^5.9.0
// resolves to — by calling its own `stringify` on the value each `name`
// describes, on the Node that `mise x -- node` provides inside example/. They
// are not this codec's output, and re-recording them by asking Go what it
// produces would empty the test out: the whole point is an expectation that
// comes from somewhere else. The generator script stays out of the tree
// (AGENTS.md: no `.mjs` harnesses); the recipe is in the worklog.
//
// internal/remotearg/kitgolden_test.go is the sibling anchor one layer up: it
// pins bytes produced by kit's own remote-argument reducers rather than by
// devalue alone.
type wireCase struct {
	name string
	// value is the Go value skgo would hand to the codec, written out here by
	// hand from the JavaScript value the generator serialized.
	value any
	// wire is what devalue 5.9.2 printed for that value.
	wire string
}

func wireCases() []wireCase {
	shared := devalue.NewObject("z", 1, "a", 2)
	cycle := devalue.NewObject("items", []any{shared, shared})
	cycle.Set("self", cycle)

	sparse := make([]any, 11)
	for i := range sparse {
		sparse[i] = devalue.Hole
	}
	sparse[10] = devalue.NewObject("b", 2, "a", 1)

	holes := []any{1, devalue.Undefined, 3}

	return []wireCase{
		// Objects. Property order is the caller's, and devalue hoists
		// canonical array indices to the front regardless of it.
		{"object", devalue.NewObject("limit", 10, "offset", 20), `[{"limit":1,"offset":2},10,20]`},
		{"object reordered", devalue.NewObject("offset", 20, "limit", 10), `[{"offset":1,"limit":2},20,10]`},
		{"empty object", devalue.NewObject(), `[{}]`},
		{"null prototype object", devalue.NewNullProtoObject("b", 2, "a", 1), `[["null","b",1,"a",2],2,1]`},
		{
			"nested object",
			devalue.NewObject("page", devalue.NewObject("limit", 10, "cursor", "abc"), "tag", "x"),
			`[{"page":1,"tag":4},{"limit":2,"cursor":3},10,"abc","x"]`,
		},
		{
			"integer-like keys",
			devalue.NewObject("10", "a", "2", "b", "B", "c", "a", "d"),
			`[{"2":1,"10":2,"B":3,"a":4},"b","a","c","d"]`,
		},

		// Arrays, including the two different absences: a hole in a sparse
		// array and an explicit undefined element.
		{"array", []any{1, "two", true, nil}, `[[1,2,3,4],1,"two",true,null]`},
		{"empty array", []any{}, `[[]]`},
		{"sparse array", sparse, `[[-7,11,10,1],{"b":2,"a":3},2,1]`},
		{"array with undefined", holes, `[[1,-1,2],1,3]`},
		{
			"array of objects",
			[]any{devalue.NewObject("a", 1), devalue.NewObject("a", 2)},
			`[[1,3],{"a":2},1,{"a":4},2]`,
		},

		// Map and Set keep insertion order and may hold any value as a key.
		{"map", devalue.NewMap("a", 1, "b", devalue.NewObject("c", 2)), `[["Map",1,2,3,4],"a",1,"b",{"c":5},2]`},
		{"empty map", devalue.NewMap(), `[["Map"]]`},
		{"map with object keys", devalue.NewMap(devalue.NewObject("k", 1), "v"), `[["Map",1,3],{"k":2},1,"v"]`},
		{"set", devalue.NewSet(3, 1, 2), `[["Set",1,2,3],3,1,2]`},
		{"empty set", devalue.NewSet(), `[["Set"]]`},
		{"set of strings", devalue.NewSet("b", "a"), `[["Set",1,2],"b","a"]`},

		// Tagged scalars.
		{
			"date",
			devalue.Date(time.Date(2024, 1, 2, 3, 4, 5, 678000000, time.UTC)),
			`[["Date","2024-01-02T03:04:05.678Z"]]`,
		},
		{"date epoch", devalue.Date(time.Unix(0, 0).UTC()), `[["Date","1970-01-01T00:00:00.000Z"]]`},
		{"bigint", devalue.BigInt("12345678901234567890"), `[["BigInt","12345678901234567890"]]`},
		{"bigint negative", devalue.BigInt("-42"), `[["BigInt","-42"]]`},
		{"bigint zero", devalue.BigInt("0"), `[["BigInt","0"]]`},
		{"regexp", devalue.RegExp{Source: "ab+c", Flags: "gi"}, `[["RegExp","ab+c","gi"]]`},
		{"regexp no flags", devalue.RegExp{Source: "^x$"}, `[["RegExp","^x$"]]`},
		{
			"arraybuffer",
			devalue.ArrayBuffer([]byte{0, 1, 2, 253, 254, 255}),
			`[["ArrayBuffer","AAEC/f7/"]]`,
		},
		{"arraybuffer empty", devalue.ArrayBuffer(nil), `[["ArrayBuffer",""]]`},

		// Boxed primitives.
		{"boxed number", devalue.NewBoxed(42), `[["Object",1],42]`},
		{"boxed string", devalue.NewBoxed("hi"), `[["Object",1],"hi"]`},
		{"boxed boolean", devalue.NewBoxed(true), `[["Object",1],true]`},

		// The out-of-band scalars devalue spells as negative slot numbers.
		{"undefined", devalue.Undefined, `-1`},
		{"null", nil, `[null]`},
		{"nan", math.NaN(), `-3`},
		{"infinity", math.Inf(1), `-4`},
		{"negative infinity", math.Inf(-1), `-5`},
		{"negative zero", math.Copysign(0, -1), `-6`},

		// Strings. devalue escapes control characters and quotes but writes
		// non-ASCII runes raw, which is why UTF-16 ordering reaches nested
		// strings and not only object keys.
		{"string with escapes", "a\"b\\c\nd\te f", `["a\"b\\c\nd\te f"]`},
		{"string astral", "\U0001F600￿", "[\"\U0001F600￿\"]"},

		// Sharing and cycles: `items` holds the same object twice and `self`
		// points back at the root, so both must resolve to slots, not copies.
		{
			"cycle and repeated reference",
			cycle,
			`[{"items":1,"self":0},[2,2],{"z":3,"a":4},1,2]`,
		},

		// One value carrying every tagged shape at once, to catch a codec that
		// handles each in isolation but miscounts slots when they are mixed.
		{
			"everything nested",
			devalue.NewObject(
				"when", devalue.Date(time.Date(2020, 6, 6, 0, 0, 0, 0, time.UTC)),
				"big", devalue.BigInt("7"),
				"re", devalue.RegExp{Source: "z", Flags: "m"},
				"buf", devalue.ArrayBuffer([]byte{9, 8}),
				"m", devalue.NewMap("k", devalue.NewSet(1)),
				"holes", holes,
				"boxed", devalue.NewBoxed("b"),
			),
			`[{"when":1,"big":2,"re":3,"buf":4,"m":5,"holes":9,"boxed":11},` +
				`["Date","2020-06-06T00:00:00.000Z"],["BigInt","7"],["RegExp","z","m"],` +
				`["ArrayBuffer","CQg="],["Map",6,7],"k",["Set",8],1,[8,-1,10],3,["Object",12],"b"]`,
		},
	}
}

// TestWireEmitsDevalueBytes is the emit half: the Go value written into the
// case has to serialize to the bytes JavaScript devalue printed for the value
// it was transcribed from. Nothing here is read back off the codec, so a codec
// that is merely self-consistent fails.
func TestWireEmitsDevalueBytes(t *testing.T) {
	t.Parallel()

	for _, c := range wireCases() {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got, err := devalue.Stringify(c.value)
			if err != nil {
				t.Fatalf("stringify: %v", err)
			}
			if got != c.wire {
				t.Fatalf("devalue 5.9.2 wrote\n  %s\nthis codec wrote\n  %s", c.wire, got)
			}
		})
	}
}

// TestWireAcceptsDevalueBytes is the accept half: every document JavaScript
// devalue produced must parse, and what comes back must serialize to the same
// bytes. A shape the codec silently flattened — a Map read as an object, a hole
// read as null, a cycle copied instead of shared — changes the bytes.
func TestWireAcceptsDevalueBytes(t *testing.T) {
	t.Parallel()

	for _, c := range wireCases() {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			parsed, err := devalue.Parse(c.wire, nil)
			if err != nil {
				t.Fatalf("parse %s: %v", c.wire, err)
			}
			got, err := devalue.Stringify(parsed)
			if err != nil {
				t.Fatalf("stringify what we parsed: %v", err)
			}
			if got != c.wire {
				t.Fatalf("round trip changed the document\n want: %s\n  got: %s", c.wire, got)
			}
		})
	}
}
