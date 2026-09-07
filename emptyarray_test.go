package skgo

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/tylergannon/polytype/devalue"
	"github.com/tylergannon/skgo/internal/formdata"
	"time"
)

// report is the shape the issue describes: a result whose slice fields are left
// nil on some path through the handler. polytype projects a Go slice as
// `Array<T>`, never `Array<T> | null`, so a wire that says `null` here makes
// the generated declaration false and the page throws on `.map`.
type report struct {
	Title       string       `json:"title"`
	Diagnostics []diagnostic `json:"diagnostics"`
	Models      []string     `json:"models"`
}

type diagnostic struct {
	Message string `json:"message"`
}

// nilReport is what every surface below returns: the zero value of the two
// slices, with a title that proves the handler ran at all.
func nilReport() report { return report{Title: "parse failed"} }

// assertReport is the assertion the whole issue comes down to. Each half is
// anchored in the declaration polytype writes — an array with no elements —
// rather than in whatever this encoder happened to produce.
func assertReport(t *testing.T, value any) {
	t.Helper()
	if got := field(t, value, "title"); got != "parse failed" {
		t.Fatalf("title = %#v, want the handler's own value; the rest of this assertion would be about nothing", got)
	}
	assertEmptyArray(t, field(t, value, "diagnostics"), "diagnostics")
	assertEmptyArray(t, field(t, value, "models"), "models")
}

func assertEmptyArray(t *testing.T, v any, what string) {
	t.Helper()
	list, ok := v.([]any)
	if !ok {
		t.Errorf("%s is %#v, want an empty array — the declaration says Array<T>", what, v)
		return
	}
	if len(list) != 0 {
		t.Errorf("%s has %d elements, want none", what, len(list))
	}
}

// A null anywhere in the body is the bug itself, since nothing in these
// fixtures is nullable but the two slices.
func assertNoNull(t *testing.T, body string) {
	t.Helper()
	if strings.Contains(body, "null") {
		t.Errorf("the response carries a null:\n%s", body)
	}
}

func TestNilSliceIsAnEmptyArrayInAQueryResult(t *testing.T) {
	getReport := NewQueryNoArg(testModule, "getReport", func(ctx context.Context) (report, error) {
		return nilReport(), nil
	})
	rs := testRemotes(t, RemoteConfig{}, getReport)

	rec := httptest.NewRecorder()
	rs.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, rs.Prefix()+getReport.ID(), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	kind, data, _ := envelope(t, rec.Body.Bytes())
	if kind != "result" {
		t.Fatalf("type = %q, want result", kind)
	}
	assertReport(t, field(t, node(t, data, getReport.ID()+"/"), "v"))
	assertNoNull(t, rec.Body.String())
}

// A nil slice returned as the whole result, which is what `query((): Array<T>
// => ...)` declares and what the issue's own getTodos looks like.
func TestNilSliceIsAnEmptyArrayAsTheWholeResult(t *testing.T) {
	getTodos := NewQueryNoArg(testModule, "getTodos", func(ctx context.Context) ([]todo, error) {
		return nil, nil
	})
	rs := testRemotes(t, RemoteConfig{}, getTodos)

	rec := httptest.NewRecorder()
	rs.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, rs.Prefix()+getTodos.ID(), nil))

	_, data, _ := envelope(t, rec.Body.Bytes())
	assertEmptyArray(t, field(t, node(t, data, getTodos.ID()+"/"), "v"), "the result")
	assertNoNull(t, rec.Body.String())
}

func TestNilSliceIsAnEmptyArrayInACommandResult(t *testing.T) {
	reparse := NewCommandNoArg(testModule, "reparse", func(ctx context.Context) (report, error) {
		return nilReport(), nil
	})
	rs := testRemotes(t, RemoteConfig{}, reparse)

	req := httptest.NewRequest(http.MethodPost, rs.Prefix()+reparse.ID(), strings.NewReader(`{"payload":""}`))
	req.Header.Set("X-Sveltekit-Pathname", "/")
	rec := httptest.NewRecorder()
	rs.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	kind, data, _ := envelope(t, rec.Body.Bytes())
	if kind != "result" {
		t.Fatalf("type = %q, want result", kind)
	}
	// A command's own result is read out of `_`.
	assertReport(t, field(t, data, "_"))
	assertNoNull(t, rec.Body.String())
}

func TestNilSliceIsAnEmptyArrayInAFormResult(t *testing.T) {
	// The envelope is the one kit's own client posts; see remote_form_test.go.
	saveReport := NewForm(testModule, "saveReport", func(ctx context.Context, in draft) (report, error) {
		return nilReport(), nil
	})
	rs := testRemotes(t, RemoteConfig{}, saveReport)

	body, err := base64.StdEncoding.DecodeString(formGoldenSubmission)
	if err != nil {
		t.Fatalf("decoding golden: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, rs.Prefix()+saveReport.ID(), strings.NewReader(string(body)))
	req.Header.Set("Content-Type", formdata.ContentType)
	rec := httptest.NewRecorder()
	rs.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	kind, data, _ := envelope(t, rec.Body.Bytes())
	if kind != "result" {
		t.Fatalf("type = %q, want result: %s", kind, rec.Body.String())
	}
	// A form's result sits under `_`.result, beside the submission flag.
	assertReport(t, field(t, field(t, data, "_"), "result"))
}

func TestNilSliceIsAnEmptyArrayInALiveQueryFrame(t *testing.T) {
	watchReport := NewLiveQueryNoArg(testModule, "watchReport", func(ctx context.Context, yield func(report) error) error {
		if err := yield(nilReport()); err != nil {
			return err
		}
		<-ctx.Done()
		return nil
	})
	srv, _, _ := liveServer(t, watchReport)
	br, cancel := openLive(t, srv, watchReport)
	defer cancel()

	frame := readFrame(t, br)
	assertNoNull(t, frame)

	// The frame is `data: {"type":"result","result":"<devalue document>"}`.
	var payload struct {
		Type   string `json:"type"`
		Result string `json:"result"`
	}
	line := strings.TrimSuffix(strings.TrimPrefix(frame, "data: "), "\n\n")
	if err := json.Unmarshal([]byte(line), &payload); err != nil {
		t.Fatalf("decoding frame %q: %v", frame, err)
	}
	if payload.Type != "result" {
		t.Fatalf("frame type = %q, want result", payload.Type)
	}
	value, err := devalue.Parse(payload.Result, nil)
	if err != nil {
		t.Fatalf("parsing %q: %v", payload.Result, err)
	}
	assertReport(t, value)
}

func TestNilSliceIsAnEmptyArrayInLoadData(t *testing.T) {
	page := NewLoad("src/routes/a/+page.server.ts", func(ctx context.Context) (report, error) {
		return nilReport(), nil
	})
	ls := mustLoads(t, nil, page)

	rec := get(t, ls, "/a/__data.json?x-sveltekit-invalidated=111")
	// Hand-written from devalue's flat format: the document is a list of
	// values, index 0 is the root object mapping each property to the index
	// holding it, devalue sorts those keys, and an empty JavaScript array is
	// `[]`. So `diagnostics` is index 1 and `models` is index 2, and both of
	// them are the empty array the declaration promises. Node 0 has no server
	// file at all and node 1 is a layout with no load, so both are null.
	want := `{"type":"data","nodes":[null,null,{"type":"data","data":[{"diagnostics":1,"models":2,"title":3},[],[],"parse failed"],"uses":{}}]}`
	if got := recorded(rec); got != want {
		t.Errorf("body =\n%s\nwant\n%s", got, want)
	}
}

// encoding/json promotes an untagged embedded struct's fields into the same
// object, and when two promoted names collide it does not simply take the first
// one it walks past. The three shapes below are the three arms of that rule,
// each arranged so the field that *loses* is a slice and the field that wins is
// not: an encoder that resolves the collision the wrong way turns a null the
// declaration promises into an array it never promised.
type inheritedSlice struct {
	X []string `json:"x"`
}

// The shallower field wins, whatever the order they are declared in.
type shadowing struct {
	inheritedSlice
	X *string `json:"x"`
}

// At equal depth the tagged name wins. Declared first, so that an encoder
// taking the last writer would get it wrong.
type tieTagged struct {
	P *string `json:"Tie"`
}
type tieUntagged struct{ Tie []string }
type tiebreak struct {
	tieTagged
	tieUntagged
}

// At equal depth with the same taggedness nobody wins and encoding/json writes
// no such property at all. Kept is there so the object is not empty for a
// reason that has nothing to do with the rule.
type ambiguousA struct{ Z []string }
type ambiguousB struct{ Z *string }
type ambiguous struct {
	ambiguousA
	ambiguousB
	Kept []string `json:"kept"`
}

// The rewrite is not a blanket "null becomes an array". Everything below is a
// null the declaration also promises, and it has to survive.
func TestOnlyANilSliceBecomesAnArray(t *testing.T) {
	type inner struct {
		Tags []string `json:"tags"`
	}
	type wide struct {
		Ptr      *inner            `json:"ptr"`
		Map      map[string]string `json:"map"`
		Any      any               `json:"any"`
		Bytes    []byte            `json:"bytes"`
		Nested   []inner           `json:"nested"`
		Deep     *inner            `json:"deep"`
		ByKey    map[string]inner  `json:"byKey"`
		ByNumber map[int][]string  `json:"byNumber"`
		When     time.Time         `json:"when"`
		Omitted  []string          `json:"omitted,omitempty"`
		Fixed    [2][]string       `json:"fixed"`
	}

	for _, tc := range []struct {
		name  string
		value any
		want  string
	}{
		{
			name: "every other null survives",
			value: wide{
				Deep:     &inner{},
				ByKey:    map[string]inner{"a": {}},
				ByNumber: map[int][]string{7: nil},
				Nested:   []inner{{}},
				When:     time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC),
			},
			want: `{"any":null,"byKey":{"a":{"tags":[]}},"byNumber":{"7":[]},"bytes":null,"deep":{"tags":[]},"fixed":[[],[]],"map":null,"nested":[{"tags":[]}],"ptr":null,"when":"2026-09-07T00:00:00Z"}`,
		},
		// The three collision cases. Each `want` is what encoding/json's own
		// shadowing rule says the property is — a nil pointer, or nothing at
		// all — so a rewrite that credits the property to the shadowed slice
		// fails here rather than shipping an array the client cannot get a
		// string out of.
		{
			name:  "a shallower field shadows a promoted slice",
			value: shadowing{},
			want:  `{"x":null}`,
		},
		{
			name:  "a tagged name beats an untagged one at the same depth",
			value: tiebreak{},
			want:  `{"Tie":null}`,
		},
		{
			name:  "an ambiguous name is nobody's property",
			value: ambiguous{},
			want:  `{"kept":[]}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// encoding/json is the authority on which properties there are and
			// which field each one came from, so the rewrite is held to the
			// object it actually produced.
			plain, err := json.Marshal(tc.value)
			if err != nil {
				t.Fatalf("json.Marshal: %v", err)
			}
			if names := propertyNames(t, plain); names != propertyNamesOf(t, tc.want) {
				t.Fatalf("the case is wrong before the rewrite runs: encoding/json writes %s, and the expectation names %s", plain, tc.want)
			}

			tree, err := encodeValue(tc.value)
			if err != nil {
				t.Fatalf("encodeValue: %v", err)
			}
			raw, err := json.Marshal(tree)
			if err != nil {
				t.Fatalf("marshalling the tree: %v", err)
			}
			if string(raw) != tc.want {
				t.Errorf("tree =\n%s\nwant\n%s", raw, tc.want)
			}
		})
	}
}

// propertyNames is the sorted top-level property names of a JSON object, so a
// case can be checked against encoding/json before its expectation is trusted.
func propertyNames(t *testing.T, raw []byte) string {
	t.Helper()
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("decoding %s: %v", raw, err)
	}
	names := make([]string, 0, len(obj))
	for name := range obj {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

func propertyNamesOf(t *testing.T, raw string) string {
	t.Helper()
	return propertyNames(t, []byte(raw))
}

// The transport walk is a second encoder — it descends into the Go value so
// devalue's reducers can still see a transported type — and it has to make the
// same correction, or a result gains and loses its nulls depending on whether
// the app happens to declare a transport.
//
// Prices is the field that proves it. Its element type is transported, so
// reaches() keeps the whole field on the walk and it never reaches encodeValue:
// the walk itself is the only thing that can turn its null into an array. Tags
// is the control, and it is not proof of anything on its own — a []string
// cannot hold a money, so the walk hands it straight back to the round trip and
// it would come out as [] even if the walk made no correction at all.
func TestTheTransportWalkAgreesAboutNilSlices(t *testing.T) {
	type basket struct {
		Price  money    `json:"price"`
		Prices []money  `json:"prices"`
		Tags   []string `json:"tags"`
	}
	tr := Transport{"Money": {
		Type:   reflect.TypeFor[money](),
		Encode: func(v any) (any, error) { return []any{float64(v.(money).Cents)}, nil },
		Decode: func(any) (any, error) { return money{}, nil },
	}}

	tree, err := tr.encodeTree(basket{Price: money{Cents: 1250}})
	if err != nil {
		t.Fatalf("encodeTree: %v", err)
	}
	obj, ok := tree.(map[string]any)
	if !ok {
		t.Fatalf("tree is %#v, want an object", tree)
	}
	assertEmptyArray(t, obj["prices"], "prices")
	assertEmptyArray(t, obj["tags"], "tags")
	if obj["price"] != (money{Cents: 1250}) {
		t.Errorf("price = %#v, want the Go value the reducer is watching for", obj["price"])
	}
}
