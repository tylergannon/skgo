package skgo

import (
	"encoding/json"
	"reflect"
	"testing"
)

// A transport switches results onto a hand-written walk instead of
// encoding/json's round trip, so the walk has to agree with encoding/json about
// everything that is not a transported value: which properties appear, what
// they are called, and what a nil becomes.
//
// The expectation here is not written out by hand and is not read back off the
// walk. It is what `json.Marshal` then `json.Unmarshal` produces — the same
// encodeValue every result went through before this feature existed — so a walk
// that invents a name, drops a field, or disagrees about `omitempty` fails
// against the standard library rather than against a fixture somebody typed.
type notTransported struct{ N int }

type tagged struct {
	Renamed  string `json:"renamed"`
	Omitted  string `json:"-"`
	Dash     string `json:"-,"`
	Empty    string `json:"empty,omitempty"`
	Kept     string `json:"kept,omitempty"`
	Untagged int
	unseen   string //nolint:unused // the point is that encoding/json skips it
}

type embedded struct{ Inner string }

type outer struct {
	embedded
	Own  string
	Ptr  *notTransported
	Nil  *notTransported
	List []notTransported
	None []notTransported
	Map  map[string]notTransported
	Deep [][]notTransported
}

// forceWalk is a transport whose type nothing in the fixtures holds, but which
// claims the fixture type itself so that reaches() says yes and the walk runs
// over the whole value. Without it every fixture would take encodeTree's
// encoding/json fast path and the test would be comparing encoding/json with
// itself.
func forceWalk(rt reflect.Type) Transport {
	return Transport{"Forced": {
		Type:   rt,
		Encode: func(any) (any, error) { return nil, nil },
		Decode: func(any) (any, error) { return nil, nil },
	}}
}

func TestTransportWalkAgreesWithEncodingJSON(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		value any
	}{
		{"tags, omitempty and unexported", tagged{Renamed: "r", Omitted: "o", Dash: "d", Kept: "k", Untagged: 3}},
		{"embedded promotion and nils", outer{
			embedded: embedded{Inner: "in"},
			Own:      "own",
			Ptr:      &notTransported{N: 1},
			List:     []notTransported{{N: 2}, {N: 3}},
			Map:      map[string]notTransported{"a": {N: 4}},
			Deep:     [][]notTransported{{{N: 5}}},
		}},
		{"zero value", outer{}},
		{"slice of structs", []tagged{{Renamed: "one"}, {Renamed: "two", Kept: "yes"}}},
		{"map of structs", map[string]tagged{"k": {Renamed: "v"}}},
		{"pointer to struct", &tagged{Renamed: "p"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// The reference: exactly what a result travelled through before
			// there was a transport hook.
			want, err := encodeValue(tc.value)
			if err != nil {
				t.Fatalf("encodeValue: %v", err)
			}

			// reflect.TypeOf on the fixture, so reaches() forces the walk over
			// the value's own type rather than a nested one.
			got, err := forceWalk(reflect.TypeOf(struct{ never int }{})).walk(reflect.ValueOf(tc.value))
			if err != nil {
				t.Fatalf("walk: %v", err)
			}

			// Compared as JSON so that a *devalue.Object and a map, or an int
			// and a float64, are not called different by reflect.DeepEqual for
			// reasons the wire does not care about.
			wantJSON, _ := json.Marshal(want)
			gotJSON, _ := json.Marshal(got)
			if string(wantJSON) != string(gotJSON) {
				t.Fatalf("the walk disagrees with encoding/json\n json: %s\n walk: %s", wantJSON, gotJSON)
			}
		})
	}
}

// The walk exists to keep a transported value out of encoding/json's hands, so
// the value that comes back must still be the Go value its reducer is watching
// for — not a map that looks like it.
type money struct{ Cents int }

type priced struct {
	Name  string
	Price money
	Also  []money
}

func TestTransportWalkLeavesTransportedValuesAlone(t *testing.T) {
	t.Parallel()

	tr := Transport{"Money": {
		Type:   reflect.TypeFor[money](),
		Encode: func(v any) (any, error) { return []any{float64(v.(money).Cents)}, nil },
		Decode: func(any) (any, error) { return money{}, nil },
	}}

	tree, err := tr.encodeTree(priced{Name: "hat", Price: money{Cents: 1250}, Also: []money{{Cents: 1}}})
	if err != nil {
		t.Fatalf("encodeTree: %v", err)
	}

	obj, ok := tree.(map[string]any)
	if !ok {
		t.Fatalf("want a map, got %T", tree)
	}
	if obj["Name"] != "hat" {
		t.Fatalf("Name is %v, want hat", obj["Name"])
	}
	if got, want := obj["Price"], (money{Cents: 1250}); got != want {
		t.Fatalf("Price reached devalue as %#v, want the Go value %#v — a reducer cannot recognise anything else", got, want)
	}
	list, ok := obj["Also"].([]any)
	if !ok || len(list) != 1 {
		t.Fatalf("Also is %#v", obj["Also"])
	}
	if got, want := list[0], (money{Cents: 1}); got != want {
		t.Fatalf("Also[0] reached devalue as %#v, want %#v", got, want)
	}
}

// Without a transport nothing changes: encodeTree is encodeValue, so an app
// that declares no custom types cannot be affected by any of this.
func TestTransportEncodeTreeIsEncodeValueWithoutATransport(t *testing.T) {
	t.Parallel()

	value := priced{Name: "hat", Price: money{Cents: 1250}}
	want, err := encodeValue(value)
	if err != nil {
		t.Fatalf("encodeValue: %v", err)
	}
	got, err := Transport(nil).encodeTree(value)
	if err != nil {
		t.Fatalf("encodeTree: %v", err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("an app with no transport must encode exactly as before\n want: %#v\n  got: %#v", want, got)
	}
}
