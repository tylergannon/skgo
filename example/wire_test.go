package example_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The wire, both ways, through the codecs `skgo generate` emits.
//
// Every payload here is written out as the devalue document kit's client would
// have sent — the bytes are the fixture, not something read back off the
// server — and every expected answer is the devalue document the browser is
// meant to receive. Nothing in this file computes an expectation by calling
// the thing it is testing.
//
// It exists because the codecs are the whole contract with the browser and
// nothing else in Go looks at them: the runtime hands a generated closure a
// tree and takes back a tree, and what happens in between is polytype's
// output. Corrupt one encoder and this file is what says so.

// payload is a devalue document as kit's client would send it: base64url, no
// padding. Kit's own `stringify_remote_arg` produces exactly this.
func payload(document string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(document))
}

// callQuery asks a query the way kit's client does.
func callQuery(t *testing.T, h http.Handler, id, document string) *httptest.ResponseRecorder {
	t.Helper()
	url := "/_app/remote/" + id
	if document != "" {
		url += "?payload=" + payload(document)
	}
	return get(t, h, url)
}

// callCommand posts a command the way kit's client does.
func callCommand(t *testing.T, h http.Handler, id, document string) *httptest.ResponseRecorder {
	t.Helper()
	body := map[string]any{"payload": payload(document), "refreshes": []string{}}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("encoding the command body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/_app/remote/"+id, strings.NewReader(string(raw)))
	req.Header.Set("Origin", prodOrigin)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// callBatch posts a whole `query.batch` the way kit's client does.
func callBatch(t *testing.T, h http.Handler, id string, documents ...string) *httptest.ResponseRecorder {
	t.Helper()
	payloads := make([]string, len(documents))
	for i, document := range documents {
		payloads[i] = payload(document)
	}
	raw, err := json.Marshal(map[string]any{"payloads": payloads})
	if err != nil {
		t.Fatalf("encoding the batch body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/_app/remote/"+id, strings.NewReader(string(raw)))
	req.Header.Set("Origin", prodOrigin)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// envelopeData reads the `data` field out of kit's remote-function envelope:
// the devalue document the browser parses.
func envelopeData(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Type string `json:"type"`
		Data string `json:"data"`
		Err  *struct {
			Status  int    `json:"status"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("parsing the envelope %q: %v", rec.Body.String(), err)
	}
	if body.Type != "result" {
		t.Fatalf("envelope type = %q (%v), want a result", body.Type, body.Err)
	}
	return body.Data
}

// requireBadRequest asserts kit's own refusal: a 200 carrying an error
// envelope, because that is what kit answers a runtime remote error with, and
// status 400 inside it.
func requireBadRequest(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("HTTP status = %d, want 200 carrying an error envelope", rec.Code)
	}
	var body struct {
		Type  string `json:"type"`
		Error struct {
			Status  int    `json:"status"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("parsing the envelope %q: %v", rec.Body.String(), err)
	}
	if body.Type != "error" || body.Error.Status != 400 {
		t.Fatalf("envelope = %s, want an error with status 400", rec.Body.String())
	}
	if body.Error.Message != "Bad Request" {
		t.Errorf("message = %q, want kit's opaque %q", body.Error.Message, "Bad Request")
	}
}

// TestEveryShapeTheExampleSendsRoundTripsThroughItsGeneratedCodec is the wire
// claim, one row per shape.
//
// `want` is the devalue document the browser receives, written out here. It is
// not the whole envelope: kit's client reads the value out of `_`, and out of
// `q[<id>/<payload>]` for a query it will cache, so both appear.
func TestEveryShapeTheExampleSendsRoundTripsThroughItsGeneratedCodec(t *testing.T) {
	h := newProdHandler(t)

	cases := []struct {
		name string
		// what the browser sends
		fn       string
		document string
		// what the browser receives. `{key}` stands for the cache key kit's
		// client will look the value up by — `<hash>/<name>/<payload>`, which
		// is `create_remote_key` — so that the shape of the document is what
		// this test pins and the module hash is not copied into it.
		want string
	}{
		{
			name: "a struct result, no argument",
			fn:   "getSite",
			want: `[{"_":1,"q":4},{"name":2,"colocated":3},"skgo","src/routes/site.remote.go",{"{key}":5},{"v":1}]`,
		},
		{
			name:     "a string argument and a struct result",
			fn:       "getItem",
			document: `["93"]`,
			want:     `[{"_":1,"q":5},{"id":2,"name":3,"colocated":4},"93","Widget 93","src/routes/items/[id]/item.remote.go",{"{key}":6},{"v":1}]`,
		},
		{
			name: "a nil slice inside a struct, which the browser is promised is an array",
			fn:   "getReport",
			want: `[{"_":1,"q":5},{"title":2,"diagnostics":3,"models":4},"Parse failed",[],[],{"{key}":6},{"v":1}]`,
		},
		{
			name: "a nil slice as the whole result",
			fn:   "getModels",
			want: `[{"_":1,"q":2},[],{"{key}":3},{"v":4},[]]`,
		},
		{
			name: "a populated struct with two slices in it",
			fn:   "getKnownReport",
			want: `[{"_":1,"q":11},{"title":2,"diagnostics":3,"models":8},"Parsed",[4,6],{"message":5},"unused import fmt",{"message":7},"missing return",[9,10],"opus","haiku",{"{key}":12},{"v":1}]`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := remoteID(t, tc.fn)
			key := id + "/"
			if tc.document != "" {
				key += payload(tc.document)
			}
			want := strings.Replace(tc.want, "{key}", key, 1)
			rec := callQuery(t, h, id, tc.document)
			if got := envelopeData(t, rec); got != want {
				t.Errorf("the wire document for %s is\n  %s\nand the browser was promised\n  %s", tc.fn, got, want)
			}
		})
	}
}

// TestABatchQueryAnswersEveryArgumentThroughItsGeneratedCodec is the same
// claim for the one call shape that carries several arguments at once.
func TestABatchQueryAnswersEveryArgumentThroughItsGeneratedCodec(t *testing.T) {
	h := newProdHandler(t)

	// Two symbols, so BatchSize is 2 and a server that answered them one at a
	// time would say 1.
	rec := callBatch(t, h, remoteID(t, "getQuotes"), `["SKGO"]`, `["KITX"]`)
	const want = `[{"_":1},[2,8],{"data":3,"type":7},{"symbol":4,"cents":5,"batchSize":6},"SKGO",1275,2,"result",{"data":9,"type":7},{"symbol":10,"cents":11,"batchSize":6},"KITX",5610]`
	if got := envelopeData(t, rec); got != want {
		t.Errorf("the wire document for getQuotes is\n  %s\nand the browser was promised\n  %s", got, want)
	}
}

// TestATransportedArgumentReachesGoAsItsOwnType is the round trip for the one
// argument that is not plain data: kit's `transport` hook tags it, and the Go
// half of that hook is the codec polytype generated for the type.
func TestATransportedArgumentReachesGoAsItsOwnType(t *testing.T) {
	h := newProdHandler(t)

	// `["Money", <payload>]` is what src/hooks.ts encodes a Money as, and 4500
	// cents is $45.00 — a number that appears nowhere else in this test.
	rec := callCommand(t, h, remoteID(t, "quoteFor"), `[["Money",1],{"cents":2},4500]`)
	const want = `[{"_":1},{"heard":2,"doubled":3},"$45.00","$90.00"]`
	if got := envelopeData(t, rec); got != want {
		t.Errorf("the wire document for quoteFor is\n  %s\nand the browser was promised\n  %s", got, want)
	}
}

// TestACommandMissingARequiredFieldIsRefusedAndChangesNothing is the defect
// this whole change exists to close.
//
// A JSON round trip zero-fills: `{}` reached renameTodo with an empty ID and
// the command answered 404 for a todo nobody named. The generated decoder is a
// schema, so the missing field is a 400 before the handler runs.
func TestACommandMissingARequiredFieldIsRefusedAndChangesNothing(t *testing.T) {
	h := newProdHandler(t)

	// The fixture is created here, so nothing else in the suite can have
	// touched it and the text below is the only place this string appears.
	const text = "a todo the wire test made"
	id := addFixtureTodo(t, h, text)

	// `text` is missing. Kit's own answer for a schema-validated function is a
	// validation error; skgo's schema is the generated decoder.
	requireBadRequest(t, callCommand(t, h, remoteID(t, "renameTodo"),
		fmt.Sprintf(`[{"id":1},%q]`, id)))

	requireTodoText(t, h, id, text)
}

// TestACommandGivenAFieldOfTheWrongKindIsRefusedAndChangesNothing is the same
// claim for a value of the wrong type rather than a missing one.
func TestACommandGivenAFieldOfTheWrongKindIsRefusedAndChangesNothing(t *testing.T) {
	h := newProdHandler(t)

	const text = "a todo the wrong-kind test made"
	id := addFixtureTodo(t, h, text)

	// `text` is a number. Rename's Text is a Go string, and the generated
	// decoder admits nothing else.
	requireBadRequest(t, callCommand(t, h, remoteID(t, "renameTodo"),
		fmt.Sprintf(`[{"id":1,"text":2},%q,7]`, id)))

	requireTodoText(t, h, id, text)
}

// TestACommandGivenAnUnknownFieldIsRefused: the decoder is strict in both
// directions, which is what stops a client's typo from silently doing nothing.
func TestACommandGivenAnUnknownFieldIsRefused(t *testing.T) {
	h := newProdHandler(t)

	const text = "a todo the unknown-field test made"
	id := addFixtureTodo(t, h, text)

	requireBadRequest(t, callCommand(t, h, remoteID(t, "renameTodo"),
		fmt.Sprintf(`[{"id":1,"text":2,"colour":3},%q,"new text","green"]`, id)))

	requireTodoText(t, h, id, text)
}

// TestAQueryDeclaredWithoutAnArgumentRefusesOne is kit's own rule, verbatim:
// `create_validator` with no validator answers 400 to any argument that is not
// `undefined`.
func TestAQueryDeclaredWithoutAnArgumentRefusesOne(t *testing.T) {
	h := newProdHandler(t)

	// `null` is a defined argument. Kit refuses it, and so does this.
	requireBadRequest(t, callQuery(t, h, remoteID(t, "getSite"), `[null]`))
	requireBadRequest(t, callQuery(t, h, remoteID(t, "getSite"), `["surprise"]`))

	// With no argument at all it answers, which is what makes the refusal
	// above a refusal rather than the function being broken.
	if got := envelopeData(t, callQuery(t, h, remoteID(t, "getSite"), "")); !strings.Contains(got, `"skgo"`) {
		t.Errorf("getSite called with no argument answered %s", got)
	}
}

// TestAQueryDeclaredWithAnArgumentRefusesACallWithoutOne is the other half.
// There is no Go value that means `undefined`, and inventing a zero one is the
// same silent wrong answer the round trip used to give.
func TestAQueryDeclaredWithAnArgumentRefusesACallWithoutOne(t *testing.T) {
	h := newProdHandler(t)
	requireBadRequest(t, callQuery(t, h, remoteID(t, "getItem"), ""))
}

// addFixtureTodo creates a todo through the app's own command and returns its
// id, so a test that must show a store *unchanged* owns the row it looks at.
func addFixtureTodo(t *testing.T, h http.Handler, text string) string {
	t.Helper()
	rec := callCommand(t, h, remoteID(t, "addTodo"), fmt.Sprintf(`[%q]`, text))
	data := envelopeData(t, rec)
	// The id is the app's to choose; the text is the test's. Read the id back
	// out of the document, which is the only thing here that could not have
	// been written down in advance.
	var flat []json.RawMessage
	if err := json.Unmarshal([]byte(data), &flat); err != nil {
		t.Fatalf("parsing %s: %v", data, err)
	}
	var root struct {
		Underscore int `json:"_"`
	}
	if err := json.Unmarshal(flat[0], &root); err != nil {
		t.Fatalf("parsing the envelope root of %s: %v", data, err)
	}
	var todo struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(flat[root.Underscore], &todo); err != nil {
		t.Fatalf("parsing the todo in %s: %v", data, err)
	}
	var id string
	if err := json.Unmarshal(flat[todo.ID], &id); err != nil {
		t.Fatalf("parsing the todo's id in %s: %v", data, err)
	}
	if id == "" {
		t.Fatalf("addTodo answered with no id: %s", data)
	}
	return id
}

// requireTodoText asks the app what one todo says, and requires it to be the
// string the test supplied. It is the "and the store is unchanged" half of a
// refusal: a decoder that let a malformed rename through would have replaced
// this text with whatever the malformed argument carried.
func requireTodoText(t *testing.T, h http.Handler, id, want string) {
	t.Helper()
	data := envelopeData(t, callQuery(t, h, remoteID(t, "getTodo"), fmt.Sprintf(`[%q]`, id)))
	if !strings.Contains(data, `"`+want+`"`) {
		t.Fatalf("todo %s reads %s, and the test put %q in it", id, data, want)
	}
}

// TestTheRuntimeHasNoGenericArgumentDecoder is the guard on the way back.
//
// The reflective codec this change removed was not one function: it was
// `decodeArg[In]`, the per-arity generic constructors that installed it, and
// the adapters between. Any of them reappearing would work — every test in
// this suite would still pass — and the strictness above would quietly be gone
// again, because a JSON round trip zero-fills.
//
// So the names are named. A runtime that decodes a client's argument into a Go
// type without a generated codec has to be spelled somehow, and every spelling
// this project has used is refused here.
func TestTheRuntimeHasNoGenericArgumentDecoder(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("locating the repository root: %v", err)
	}

	// The symbols the reflective path was made of, and the shape of any
	// replacement: a registration generic in the argument's type decodes that
	// argument somewhere, and the somewhere is not generated code.
	banned := []string{
		"decodeArg",
		"callAdapter",
		"formAdapter",
		"func NewQuery[",
		"func NewQueryNoArg[",
		"func NewCommand[",
		"func NewCommandNoArg[",
		"func NewLiveQuery[",
		"func NewLiveQueryNoArg[",
		"func NewBatchQuery[",
		"func NewForm[",
		"func NewLoad[",
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("reading %s: %v", root, err)
	}
	var checked int
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		checked++
		source := string(raw)
		for _, symbol := range banned {
			if strings.Contains(source, symbol) {
				t.Errorf("%s contains %q. A remote function's argument is decoded by the codec "+
					"`skgo generate` emitted for its own type; a generic decoder in the runtime is "+
					"the reflective path this was replaced to remove, and it zero-fills.", name, symbol)
			}
		}
	}
	// A glob that matched nothing would pass every assertion above.
	if checked < 20 {
		t.Fatalf("only %d files in %s were checked; this test is not looking at the runtime", checked, root)
	}
}
