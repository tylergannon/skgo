package skgo

import (
	"net/http"
	"strings"
	"testing"

	"github.com/tylergannon/skgo/internal/devalue"
	"github.com/tylergannon/skgo/internal/formdata"
)

// The strings below were written out of kit's own source — `render.js`'s boot
// script, `collect_remote_data`'s `f` bucket and `form.svelte.js`'s read of it
// (@sveltejs/kit 3.0.0-next.25) — rather than read back off skgo's output. A
// document that carries a form's outcome in a different slot, or under a
// different key, is a document kit's client will start the form empty over.

// submitted is a renderer whose document assembly can write remote data: the
// transport hook `remoteData` reads lives on the remote registry, which
// `streamer` leaves nil because a document with no answers never asks for it.
func submitted(t *testing.T) *SSR {
	t.Helper()
	s := streamer(nil)
	s.remotes = &Remotes{cfg: RemoteConfig{}}
	return s
}

func withForm(plan documentPlan, id string, output *devalue.Object) (documentPlan, map[string]map[string]answered) {
	plan.action = &formAction{id: id, output: output}
	return plan, map[string]map[string]answered{"f": {id: {tree: output}}}
}

// A remote form leaves `form:` null. Kit's `handle_remote_form_post_internal`
// answers `{type: 'success', status, location}` with no `data`, so
// `render.js` computes `form_value = null` and `uneval_action_response` is
// never reached on this path — it belongs to the classic `+page.server.js`
// actions kit also has. Writing the submission there instead would put it on
// `let { form } = $props()`, which kit's comment says explicitly is not where
// it goes.
func TestARemoteFormLeavesTheFormSlotNull(t *testing.T) {
	s := submitted(t)
	plan, answers := withForm(
		plannedPage(dataNode{}, dataNode{}),
		"14q4me9/sendMessage",
		devalue.NewObject("submission", true, "result", map[string]any{"id": "m1"}),
	)

	document, _ := assembledWith(t, s, plan, answers)

	if !strings.Contains(document, "form: null") {
		t.Fatalf("the boot object does not carry `form: null`:\n%s", document)
	}
}

// The submission travels in `<global>.data.f`, keyed by the action id itself.
// `collect_remote_data` files a form's output under the client-side action id
// directly — `create_remote_key(id, payload)` is for a function with an
// argument to key on — and `form.svelte.js` reads `query_responses[action_id]`
// under exactly that string.
func TestTheSubmissionIsInTheFormSlotOfTheRemoteData(t *testing.T) {
	s := submitted(t)
	plan, answers := withForm(
		plannedPage(dataNode{}, dataNode{}),
		"14q4me9/sendMessage",
		devalue.NewObject(
			"submission", true,
			"result", devalue.NewObject("id", "m1", "summary", "Thanks, Ada Lovelace — message m1 is in."),
		),
	)

	document, _ := assembledWith(t, s, plan, answers)

	want := `__sveltekit_test.data = {f:{"14q4me9/sendMessage":{v:{submission:true,result:{id:"m1",summary:"Thanks, Ada Lovelace — message m1 is in."}}}}};`
	if !strings.Contains(document, want) {
		t.Fatalf("the document does not carry the submission where kit's client reads it.\nwant: %s\ngot:\n%s", want, document)
	}
}

// A refused submission carries its issues and the visitor's input, in kit's own
// property order: `handle_issues` sets `issues` and then `input` onto an object
// whose `submission` was set first. The client reads `issues` to seed
// `raw_issues` and `input` to refill the controls, so both have to be there and
// neither may be an error node — kit's comment says form outputs are always
// value nodes.
func TestARefusedSubmissionCarriesItsIssuesAndInput(t *testing.T) {
	s := submitted(t)
	plan, answers := withForm(
		plannedPage(dataNode{}, dataNode{}),
		"14q4me9/sendMessage",
		devalue.NewObject(
			"submission", true,
			"issues", issueNodes([]Issue{{Field: "email", Message: "that is not an email address"}}),
			"input", devalue.NewObject("from", "Grace Hopper", "email", "grace-at-example"),
		),
	)

	document, _ := assembledWith(t, s, plan, answers)

	want := `{f:{"14q4me9/sendMessage":{v:{submission:true,` +
		`issues:[{name:"email",path:["email"],message:"that is not an email address",server:true}],` +
		`input:{from:"Grace Hopper",email:"grace-at-example"}}}}}`
	if !strings.Contains(document, want) {
		t.Fatalf("the document does not carry the issues where kit's client reads them.\nwant: %s\ngot:\n%s", want, document)
	}
}

// An untouched `<input type="file">` is not a string, so kit's `handle_issues`
// filters it out of `form_data.getAll(name)` before taking `values[0]` — the
// field is `undefined`, not the empty string a text control would have left.
// `submittedInput` used to rewrite a file entry into an empty text entry
// before handing it to `ConvertRaw`, which defeated `ConvertRaw`'s own File
// check and produced "" instead.
func TestSubmittedInputLeavesAnUntouchedFileFieldUndefined(t *testing.T) {
	got := submittedInput("14q4me9/sendMessage", []formdata.Entry{
		{Name: "from/14q4me9/sendMessage", Value: "Ada Lovelace"},
		{Name: "attachment/14q4me9/sendMessage", File: &formdata.File{}},
	})

	if from, _ := got.Get("from"); from != "Ada Lovelace" {
		t.Fatalf("from = %#v, want \"Ada Lovelace\"", from)
	}
	attachment, _ := got.Get("attachment")
	if _, undefined := attachment.(devalue.UndefinedValue); !undefined {
		t.Errorf("an untouched file control's input is %#v, want devalue.Undefined", attachment)
	}
}

// A document that is not answering a submission has no `f` bucket at all. Kit
// omits an empty one, and an `f` entry the client cannot match to a form is an
// entry it never deletes from its query cache.
func TestADocumentThatAnswersNoSubmissionHasNoFormSlot(t *testing.T) {
	s := submitted(t)
	document, _ := assembledWith(t, s, plannedPage(dataNode{}, dataNode{}), nil)

	if strings.Contains(document, ".data = {f:") {
		t.Fatalf("a document that answered no submission carries a form slot:\n%s", document)
	}
}

// `?/remote=<id>` is kit's `get_remote_action`, and a POST that carries no such
// parameter is a classic form action — which skgo has none of.
func TestTheActionIDIsTheRemoteSearchParameter(t *testing.T) {
	for _, test := range []struct {
		raw  string
		want string
	}{
		{"http://127.0.0.1/contact?/remote=14q4me9/sendMessage", "14q4me9/sendMessage"},
		{"http://127.0.0.1/contact?page=2&/remote=14q4me9/sendMessage", "14q4me9/sendMessage"},
		{"http://127.0.0.1/contact?/remote=14q4me9%2FsendMessage", "14q4me9/sendMessage"},
		{"http://127.0.0.1/contact", ""},
		{"http://127.0.0.1/contact?remote=14q4me9/sendMessage", ""},
	} {
		if got := actionID(mustURL(t, test.raw)); got != test.want {
			t.Errorf("actionID(%q) = %q, want %q", test.raw, got, test.want)
		}
	}
}

// Kit's redaction of a control whose name begins with an underscore, `/^[.\]]?_/`
// applied to the raw field name. It is narrower than it looks — a `n:_secret`
// is not caught, because the prefix comes first — and mirroring it is the
// point: a name kit echoes back and skgo hides, or the reverse, is a form that
// refills differently in the two.
func TestARedactedControlIsTheOneKitRedacts(t *testing.T) {
	for name, want := range map[string]bool{
		"_token/14q4me9/sendMessage":   true,
		"._token/14q4me9/sendMessage":  true,
		"]_token/14q4me9/sendMessage":  true,
		"email/14q4me9/sendMessage":    false,
		"n:_token/14q4me9/sendMessage": false,
		"a._token/14q4me9/sendMessage": false,
	} {
		if got := redactedFormKey(name); got != want {
			t.Errorf("redactedFormKey(%q) = %v, want %v", name, got, want)
		}
	}
}

// assembledWith is `assembled` with remote answers, which the streaming tests
// have no use for and this file has nothing but.
func assembledWith(t *testing.T, s *SSR, plan documentPlan, answers map[string]map[string]answered) (string, *promiseTable) {
	t.Helper()
	req := dataRequest{url: mustURL(t, "http://127.0.0.1/contact?/remote=14q4me9/sendMessage")}
	result := rendered()
	result.Status = http.StatusOK
	document, promises, err := s.assemble(req, plan, result, answers)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	return document, promises
}
