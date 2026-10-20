package skgo

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tylergannon/polytype"
	"github.com/tylergannon/skgo/internal/formdata"
)

// The bodies below are real envelopes, produced by running kit's own
// `serialize_binary_form` against the pinned devalue — the same recipe as the
// goldens in internal/formdata. Building them with skgo's own encoder would
// prove nothing about whether skgo agrees with kit.
const (
	// {from: 'Ada', body: 'hello there'} asking for `worolc/getMessages/` to be
	// refreshed in the same flight. `worolc` is kit's hash of
	// "src/lib/todos.remote.ts", which is what testModule is.
	formGoldenSubmission = "AGAAAAAAAFtbMSw0XSx7ImZyb20iOjIsImJvZHkiOjN9LCJBZGEiLCJoZWxsbyB0aGVyZSIseyJyZW1vdGVfcmVmcmVzaGVzIjo1fSxbNl0sIndvcm9sYy9nZXRNZXNzYWdlcy8iXQ=="

	// {from: 'Ada', attachment: File('note.txt', 'haiku bytes')}.
	formGoldenWithFile = "AIUAAAADAFtbMSwxMF0seyJmcm9tIjoyLCJhdHRhY2htZW50IjozfSwiQWRhIixbIkZpbGUiLDRdLFs1LDYsNyw4LDldLCJub3RlLnR4dCIsInRleHQvcGxhaW4iLDExLDE3MDAwMDAwMDAwMDAsMCx7InJlbW90ZV9yZWZyZXNoZXMiOjExfSxbXV1bMF1oYWlrdSBieXRlcw=="

	// What `form.validate()` posts as the visitor types.
	formGoldenValidateOnly = "AC4AAAAAAFtbMSwzXSx7ImZyb20iOjJ9LCIiLHsidmFsaWRhdGVfb25seSI6NH0sdHJ1ZV0="

	// Produced with the pinned Kit 3.0.0-next.27 convert_formdata and
	// serialize_binary_form, using controls named name/id and n:count/id. The
	// expectations below are literal fixture values, independent of the parsed
	// submission.
	formGoldenOptionalInvalid  = "ACoAAAAAAFtbMSwzXSx7Im5hbWUiOjIsImNvdW50IjotM30sImZpeHR1cmUiLHt9XQ=="
	formGoldenOptionalValidate = "AEEAAAAAAFtbMSw0XSx7Im5hbWUiOjIsImNvdW50IjozfSwiZml4dHVyZSIsMCx7InZhbGlkYXRlX29ubHkiOjV9LHRydWVd"
)

// Envelopes the pinned Kit's convert_formdata and serialize_binary_form build
// from the FormData in TestKitSerializesTheOptionalFormFixtures, which checks
// Kit still produces exactly these bytes. TestFormOptionalScalarsFromKit
// decodes them at the form handler.
const (
	formFixtureOmitted  = "internal/formdata/testdata/kit-optional-omitted.bin"
	formFixtureExplicit = "internal/formdata/testdata/kit-optional-explicit.bin"
)

type draft struct {
	From       string `json:"from"`
	Body       string `json:"body"`
	Attachment File   `json:"attachment"`
}

type receipt struct {
	ID string `json:"id"`
}

func formRequest(t *testing.T, rs *Remotes, fn *Remote, golden string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := base64.StdEncoding.DecodeString(golden)
	if err != nil {
		t.Fatalf("decoding golden: %v", err)
	}
	return postForm(rs, fn, body)
}

func postForm(rs *Remotes, fn *Remote, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, rs.Prefix()+fn.ID(), bytes.NewReader(body))
	req.Header.Set("Content-Type", formdata.ContentType)
	rec := httptest.NewRecorder()
	rs.ServeHTTP(rec, req)
	return rec
}

func TestFormSubmissionAndSingleFlightRefresh(t *testing.T) {
	var received draft
	var sent []string

	getMessagesFn := func(ctx context.Context) ([]string, error) {
		out := make([]string, len(sent))
		copy(out, sent)
		return out, nil
	}
	getMessages := NewQueryNoArg(testModule, "getMessages", getMessagesFn)
	sendMessage := NewForm(testModule, "sendMessage", func(ctx context.Context, in draft) (receipt, error) {
		received = in
		sent = append(sent, in.Body)
		return receipt{ID: "m1"}, RefreshRequestedNoArg(ctx, getMessagesFn)
	})
	rs := testRemotes(t, RemoteConfig{}, getMessages, sendMessage)

	rec := formRequest(t, rs, sendMessage, formGoldenSubmission)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	// The handler saw what kit's client put in the envelope.
	if received.From != "Ada" || received.Body != "hello there" {
		t.Errorf("handler received %+v, want {Ada hello there}", received)
	}

	kind, data, _ := envelope(t, rec.Body.Bytes())
	if kind != "result" {
		t.Fatalf("type = %q, want result", kind)
	}
	if got := field(t, field(t, data, "_"), "result"); field(t, got, "id") != "m1" {
		t.Errorf("result = %#v, want the receipt", got)
	}

	// The refresh the client asked for ran after the handler, so the list it
	// carries back already has the new message in it.
	refreshed := field(t, node(t, data, getMessages.ID()+"/"), "v")
	list, ok := refreshed.([]any)
	if !ok || len(list) != 1 || list[0] != "hello there" {
		t.Errorf("refreshed query = %#v, want [hello there]", refreshed)
	}
	if field(t, data, "r") != true {
		t.Error("r is not set, so kit's client would run refreshAll on top of the single-flight update")
	}
}

func TestFormReceivesFileBytes(t *testing.T) {
	var received draft
	sendMessage := NewForm(testModule, "sendMessage", func(ctx context.Context, in draft) (receipt, error) {
		received = in
		return receipt{ID: "m1"}, nil
	})
	rs := testRemotes(t, RemoteConfig{}, sendMessage)

	if rec := formRequest(t, rs, sendMessage, formGoldenWithFile); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	if received.Attachment.Name != "note.txt" {
		t.Errorf("Attachment.Name = %q, want note.txt", received.Attachment.Name)
	}
	if received.Attachment.Type != "text/plain" {
		t.Errorf("Attachment.Type = %q", received.Attachment.Type)
	}
	// The literal bytes the golden was built from.
	if string(received.Attachment.Data) != "haiku bytes" {
		t.Errorf("Attachment.Data = %q, want %q", received.Attachment.Data, "haiku bytes")
	}
}

// A submission the handler rejects must come back as issues and nothing else.
// Kit's server returns before it collects refreshes precisely so the client
// leaves the page — and the visitor's input — alone.
func TestFormIssuesSuppressRefreshes(t *testing.T) {
	getMessagesFn := func(ctx context.Context) ([]string, error) {
		t.Error("a rejected submission must not run the refreshes it asked for")
		return nil, nil
	}
	getMessages := NewQueryNoArg(testModule, "getMessages", getMessagesFn)
	sendMessage := NewForm(testModule, "sendMessage", func(ctx context.Context, in draft) (receipt, error) {
		// Accepted first, so what suppresses the refresh is the submission
		// having issues rather than the handler never having asked.
		if err := RefreshRequestedNoArg(ctx, getMessagesFn); err != nil {
			return receipt{}, err
		}
		return receipt{}, Invalidf("from", "Tell us who you are")
	})
	rs := testRemotes(t, RemoteConfig{}, getMessages, sendMessage)

	rec := formRequest(t, rs, sendMessage, formGoldenSubmission)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	kind, data, _ := envelope(t, rec.Body.Bytes())
	if kind != "result" {
		t.Fatalf("type = %q, want result", kind)
	}
	for _, key := range []string{"q", "l", "r"} {
		if has(t, data, key) {
			t.Errorf("a rejected submission carried %q; kit's client would apply updates or reload", key)
		}
	}

	issues, ok := field(t, field(t, data, "_"), "issues").([]any)
	if !ok || len(issues) != 1 {
		t.Fatalf("issues = %#v, want one", field(t, field(t, data, "_"), "issues"))
	}
	issue := issues[0]
	if got := field(t, issue, "message"); got != "Tell us who you are" {
		t.Errorf("message = %#v", got)
	}
	// `name` is what the field proxy matches on and `path` is what
	// `flatten_issues` walks; kit's client needs both.
	if got := field(t, issue, "name"); got != "from" {
		t.Errorf("name = %#v, want %q", got, "from")
	}
	path, ok := field(t, issue, "path").([]any)
	if !ok || len(path) != 1 || path[0] != "from" {
		t.Errorf("path = %#v, want [from]", field(t, issue, "path"))
	}
	if got := field(t, issue, "server"); got != true {
		t.Errorf("server = %#v, want true", got)
	}
	// A rejected submission does not send the input back. It never navigated,
	// so the values are still in the controls; kit's own server skips this for
	// the same reason.
	if has(t, field(t, data, "_"), "input") {
		t.Error("issues carried an input; an enhanced submission does not need one")
	}
}

func TestFormIssuePathsMatchKitsFieldNames(t *testing.T) {
	cases := []struct {
		field string
		name  string
		path  []any
	}{
		{"email", "email", []any{"email"}},
		{"author.name", "author.name", []any{"author", "name"}},
		{"tags[0]", "tags[0]", []any{"tags", float64(0)}},
		{"rows[2].label", "rows[2].label", []any{"rows", float64(2), "label"}},
		{"", "", []any{}},
	}
	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			path := splitFieldPath(tc.field)
			if len(path) != len(tc.path) {
				t.Fatalf("path = %#v, want %#v", path, tc.path)
			}
			for i := range path {
				if path[i] != tc.path[i] {
					t.Fatalf("path = %#v, want %#v", path, tc.path)
				}
			}
			if got := buildFieldPath(path); got != tc.name {
				t.Errorf("name = %q, want %q", got, tc.name)
			}
		})
	}
}

// `form.validate()` asks for the schema's verdict without running the handler.
// skgo's forms are declared `'unchecked'`, so there is no schema and the answer
// is an empty issue list — running the handler here would fire its side effects
// on every keystroke.
func TestFormValidateOnlyDoesNotRunTheHandler(t *testing.T) {
	ran := false
	sendMessage := NewForm(testModule, "sendMessage", func(ctx context.Context, in draft) (receipt, error) {
		ran = true
		return receipt{}, nil
	})
	rs := testRemotes(t, RemoteConfig{}, sendMessage)

	rec := formRequest(t, rs, sendMessage, formGoldenValidateOnly)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ran {
		t.Error("validate_only ran the handler")
	}

	_, data, _ := envelope(t, rec.Body.Bytes())
	list, ok := field(t, data, "_").([]any)
	if !ok || len(list) != 0 {
		t.Errorf("_ = %#v, want an empty issue list", field(t, data, "_"))
	}
}

// Kit's client, not skgo, encodes an enhanced form submission, so the bytes
// skgo decodes must be the bytes the pinned Kit writes. In the omitted
// submission a blank number input posts "", which Kit turns into undefined,
// and the other optional fields post nothing. The file's type and
// lastModified are fixed so the bytes are too.
func TestKitSerializesTheOptionalFormFixtures(t *testing.T) {
	module, err := filepath.Abs("example/web/node_modules/@sveltejs/kit/src/runtime/form-utils.js")
	if err != nil {
		t.Fatal(err)
	}
	const program = `import {pathToFileURL} from 'node:url';
const {convert_formdata,serialize_binary_form,BINARY_FORM_CONTENT_TYPE}=await import(pathToFileURL(process.argv[1]).href);
const id='worolc/optional';
const omitted=new FormData();
omitted.append('name/'+id,'fixture');
omitted.append('n:count/'+id,'');
const explicit=new FormData();
explicit.append('name/'+id,'fixture');
explicit.append('n:count/'+id,'0');
explicit.append('b:enabled/'+id,'off');
explicit.append('label/'+id,'');
explicit.append('attachment/'+id,new File(['literal-upload-73'],'haiku.txt',{type:'text/plain',lastModified:1700000000000}));
const envelope=async form=>{
 const {blob}=serialize_binary_form(convert_formdata(id,form),{remote_refreshes:[]});
 return Buffer.from(await blob.arrayBuffer()).toString('base64');
};
process.stdout.write(JSON.stringify({contentType:BINARY_FORM_CONTENT_TYPE,omitted:await envelope(omitted),explicit:await envelope(explicit)}));`
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "node", "--input-type=module", "-e", program, module).Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			t.Fatalf("Kit's form serializer: %v\n%s", err, exit.Stderr)
		}
		t.Fatalf("Kit's form serializer: %v", err)
	}
	var kit struct {
		ContentType string `json:"contentType"`
		Omitted     []byte `json:"omitted"`
		Explicit    []byte `json:"explicit"`
	}
	if err := json.Unmarshal(out, &kit); err != nil {
		t.Fatalf("Kit's form serializer printed %q: %v", out, err)
	}
	if kit.ContentType != formdata.ContentType {
		t.Errorf("Kit posts forms as %q; skgo's form handler accepts %q", kit.ContentType, formdata.ContentType)
	}
	for _, fixture := range []struct {
		path string
		kit  []byte
	}{{formFixtureOmitted, kit.Omitted}, {formFixtureExplicit, kit.Explicit}} {
		want, err := os.ReadFile(fixture.path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(fixture.kit, want) {
			t.Errorf("Kit's serializer wrote\n%q\nthe committed %s is\n%q", fixture.kit, fixture.path, want)
		}
	}
}

func TestFormOptionalScalarsFromKit(t *testing.T) {
	type input struct {
		Name       string                    `json:"name"`
		Count      polytype.Optional[int]    `json:"count,omitzero"`
		Enabled    polytype.Optional[bool]   `json:"enabled,omitzero"`
		Label      polytype.Optional[string] `json:"label,omitzero"`
		Attachment File                      `json:"attachment"`
	}
	var received []input
	form := NewForm(testModule, "optional", func(_ context.Context, in input) (receipt, error) {
		received = append(received, in)
		return receipt{ID: "accepted"}, nil
	})
	rs := testRemotes(t, RemoteConfig{}, form)

	for _, tc := range []struct {
		name    string
		fixture string
		want    input
	}{
		{"omitted", formFixtureOmitted, input{Name: "fixture"}},
		{"explicit zeroes", formFixtureExplicit, input{
			Name:       "fixture",
			Count:      polytype.Optional[int]{Present: true, Value: 0},
			Enabled:    polytype.Optional[bool]{Present: true, Value: false},
			Label:      polytype.Optional[string]{Present: true, Value: ""},
			Attachment: File{Name: "haiku.txt", Type: "text/plain", LastModified: 1700000000000, Data: []byte("literal-upload-73")},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := os.ReadFile(tc.fixture)
			if err != nil {
				t.Fatal(err)
			}
			rec := postForm(rs, form, body)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			if len(received) == 0 {
				t.Fatal("the handler did not run")
			}
			if got := received[len(received)-1]; !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("handler received %+v with file bytes %q, want %+v with %q", got, got.Attachment.Data, tc.want, tc.want.Attachment.Data)
			}
			kind, data, _ := envelope(t, rec.Body.Bytes())
			if kind != "result" || field(t, field(t, field(t, data, "_"), "result"), "id") != "accepted" {
				t.Fatalf("response = %s, want accepted result", rec.Body.String())
			}
		})
	}

	before := len(received)
	rec := formRequest(t, rs, form, formGoldenOptionalInvalid)
	if rec.Code != http.StatusOK {
		t.Fatalf("invalid status = %d, want Kit's 200 error envelope", rec.Code)
	}
	kind, _, httpErr := envelope(t, rec.Body.Bytes())
	if kind != "error" || httpErr["status"] != float64(400) {
		t.Fatalf("invalid response = %s, want a 400 error envelope", rec.Body.String())
	}
	if len(received) != before {
		t.Fatal("invalid scalar invoked the operation")
	}

	rec = formRequest(t, rs, form, formGoldenOptionalValidate)
	if rec.Code != http.StatusOK {
		t.Fatalf("validation status = %d, want 200", rec.Code)
	}
	_, data, _ := envelope(t, rec.Body.Bytes())
	if list, ok := field(t, data, "_").([]any); !ok || len(list) != 0 {
		t.Fatalf("validation response = %s, want empty issues", rec.Body.String())
	}
	if len(received) != before {
		t.Fatal("validation-only request invoked the operation")
	}
}

func TestFormRejectsWrongMethodAndContentType(t *testing.T) {
	sendMessage := NewForm(testModule, "sendMessage", func(ctx context.Context, in draft) (receipt, error) {
		t.Error("the handler must not run")
		return receipt{}, nil
	})
	rs := testRemotes(t, RemoteConfig{}, sendMessage)

	t.Run("GET", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, rs.Prefix()+sendMessage.ID(), nil)
		rec := httptest.NewRecorder()
		rs.ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status = %d, want 405", rec.Code)
		}
	})

	t.Run("JSON body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, rs.Prefix()+sendMessage.ID(), strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		rs.ServeHTTP(rec, req)
		// Kit refuses a non-form content type outright: these are the media
		// types a cross-origin <form> can send, and its CSRF protection is
		// written around exactly that set.
		if rec.Code != http.StatusUnsupportedMediaType {
			t.Fatalf("status = %d, want 415", rec.Code)
		}
	})
}

func TestFormPanicIsAnOpaque500(t *testing.T) {
	sendMessage := NewForm(testModule, "sendMessage", func(ctx context.Context, in draft) (receipt, error) {
		panic("boom")
	})
	var reported string
	rs := testRemotes(t, RemoteConfig{OnPanic: func(id string, value any, stack []byte) {
		reported = id
	}}, sendMessage)

	rec := formRequest(t, rs, sendMessage, formGoldenSubmission)
	kind, _, httpErr := envelope(t, rec.Body.Bytes())
	if kind != "error" {
		t.Fatalf("type = %q, want error", kind)
	}
	if httpErr["status"] != float64(500) || httpErr["message"] != "Internal Error" {
		t.Errorf("error = %#v, want an opaque 500", httpErr)
	}
	if reported != sendMessage.ID() {
		t.Errorf("OnPanic saw %q, want %q", reported, sendMessage.ID())
	}
}

func TestInvalidCollectsIssues(t *testing.T) {
	var v *Invalid
	if v.Err() != nil {
		t.Error("a nil Invalid reports an error")
	}

	v = &Invalid{}
	if v.Err() != nil {
		t.Error("an empty Invalid reports an error")
	}

	v.Add("email", "%q is not an address", "nope").Add("body", "too short")
	err := v.Err()
	if err == nil {
		t.Fatal("Err() returned nil after two issues were added")
	}

	var target *Invalid
	if !errors.As(err, &target) || len(target.Issues) != 2 {
		t.Fatalf("errors.As gave %#v", target)
	}
	if target.Issues[0].Message != `"nope" is not an address` {
		t.Errorf("first message = %q", target.Issues[0].Message)
	}
	if !strings.Contains(err.Error(), "email:") || !strings.Contains(err.Error(), "body:") {
		t.Errorf("Error() = %q, want both fields named", err.Error())
	}
}
