package skgo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/tylergannon/skgo/internal/devalue"
	"github.com/tylergannon/skgo/internal/formdata"
)

// The strings below were written out of kit's own source — `render.js`'s boot
// script, `collect_remote_data`'s `f` bucket and `form.svelte.js`'s read of it
// (@sveltejs/kit 3.0.0-next.27) — rather than read back off skgo's output. A
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
	csp := newDocumentCSPWithNonce(s.info.CSP, "test-nonce")
	document, promises, _, err := s.assemble(req, plan, result, answers, csp)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	return document, promises
}

// splitActionID is kit's own `[hash, name, ...rest]` split
// (`handle_remote_form_post_internal`), and this is the fixture it was
// written out of: a hash and a name never contain a "/", but the JSON text of
// a `form.for(key)` instance's key can — restored by url-decoding a "%2F" the
// browser's `encodeURIComponent` put there — so the remaining segments have to
// be rejoined rather than the third one taken alone.
func TestSplitActionIDIsKitsHashNameRestSplit(t *testing.T) {
	for _, test := range []struct {
		name       string
		id         string
		wantBase   string
		wantKey    string
		wantHasKey bool
	}{
		{
			name:     "no third segment is an unkeyed form",
			id:       "14q4me9/sendMessage",
			wantBase: "14q4me9/sendMessage",
		},
		{
			name:       "a string key",
			id:         `14q4me9/sendMessage/"k1"`,
			wantBase:   "14q4me9/sendMessage",
			wantKey:    `"k1"`,
			wantHasKey: true,
		},
		{
			name:       "a numeric key",
			id:         "14q4me9/sendMessage/1",
			wantBase:   "14q4me9/sendMessage",
			wantKey:    "1",
			wantHasKey: true,
		},
		{
			name: "an empty trailing segment is not a key",
			// `rest.join('/')` of a lone "" is "", and kit's own
			// `if (action_id)` treats that as no key at all.
			id:       "14q4me9/sendMessage/",
			wantBase: "14q4me9/sendMessage",
		},
		{
			name: "a key whose JSON text itself contains a slash",
			// JSON.stringify("a/b") is `"a/b"`; url-decoding what
			// encodeURIComponent did to it puts the slash back before this
			// function ever sees the string, so the rejoin has to recover it.
			id:         `14q4me9/sendMessage/"a/b"`,
			wantBase:   "14q4me9/sendMessage",
			wantKey:    `"a/b"`,
			wantHasKey: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			base, key, hasKey := splitActionID(test.id)
			if base != test.wantBase || key != test.wantKey || hasKey != test.wantHasKey {
				t.Errorf("splitActionID(%q) = (%q, %q, %v), want (%q, %q, %v)",
					test.id, base, key, hasKey, test.wantBase, test.wantKey, test.wantHasKey)
			}
		})
	}
}

// setFormKey is kit's own line, once a keyed submission's form is resolved:
// `if (action_id && !('id' in data)) data.id = JSON.parse(decodeURIComponent(action_id))`.
func TestSetFormKeyMirrorsKitsIDInjection(t *testing.T) {
	t.Run("a string key becomes the id field", func(t *testing.T) {
		arg := devalue.NewObject("body", "hello")
		if err := setFormKey(arg, `"k1"`); err != nil {
			t.Fatalf("setFormKey: %v", err)
		}
		got, ok := arg.Get("id")
		if !ok || got != "k1" {
			t.Fatalf("id = %#v, %v; want \"k1\", true", got, ok)
		}
	})

	t.Run("a numeric key parses as a number, not a string", func(t *testing.T) {
		arg := devalue.NewObject("body", "hello")
		if err := setFormKey(arg, "42"); err != nil {
			t.Fatalf("setFormKey: %v", err)
		}
		got, _ := arg.Get("id")
		if got != float64(42) {
			t.Fatalf("id = %#v (%T), want float64(42)", got, got)
		}
	})

	t.Run("a form that already declared its own id field is left alone", func(t *testing.T) {
		// Kit's `!('id' in data)` guard: a form the app itself gave an `id`
		// control keeps what the visitor typed over what the URL carried.
		arg := devalue.NewObject("id", "explicit-value")
		if err := setFormKey(arg, `"k1"`); err != nil {
			t.Fatalf("setFormKey: %v", err)
		}
		got, _ := arg.Get("id")
		if got != "explicit-value" {
			t.Fatalf("id = %#v, want \"explicit-value\" (untouched)", got)
		}
	})
}

// TestAKeyedFormSubmissionDeliversItsKeyAsTheIDField is the end-to-end proof
// that a `form.for(key)` submission the browser posted itself — no kit client
// in the loop — reaches the Go handler with the key kit's own client encoded
// into the URL, delivered exactly the way kit delivers it: as the `id` field
// of the form's argument.
//
// The fixture is the literal bytes kit's client puts on the wire
// (`runtime/app/server/remote/form.js:163`,
// `action_id = id + (key != undefined ? \`/${JSON.stringify(key)}\` : '')`,
// `action = '/remote=' + encodeURIComponent(action_id)`) for the key "k1" —
// not a string this test or the code under test invented — decoded by
// `actionID`, kit's own `get_remote_action`, the same as a real request would
// be.
func TestAKeyedFormSubmissionDeliversItsKeyAsTheIDField(t *testing.T) {
	type keyedDraft struct {
		ID   string `json:"id"`
		Body string `json:"body"`
	}
	type receipt struct {
		ID string `json:"id"`
	}

	var received keyedDraft
	sendMessage := NewForm(testModule, "sendMessage", func(_ context.Context, in keyedDraft) (receipt, error) {
		received = in
		return receipt{ID: "m1"}, nil
	})
	rs := testRemotes(t, RemoteConfig{}, sendMessage)
	s := &SSR{loads: &Loads{}, remotes: rs}

	// JSON.stringify("k1") is `"k1"`; encodeURIComponent of that is
	// `%22k1%22` — the exact string the issue this fixes reproduced kit's
	// client sending (`sendMessage/%22k1%22`).
	rawAction := sendMessage.ID() + `/%22k1%22`
	id := actionID(mustURL(t, "http://127.0.0.1/contact?/remote="+rawAction))

	body := url.Values{"body/" + sendMessage.ID(): {"Two elements, one Nobel each."}}.Encode()
	req := httptest.NewRequest(http.MethodPost, "/contact?/remote="+rawAction, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	action, redirect, herr := s.runFormAction(req, id)
	if herr != nil {
		t.Fatalf("runFormAction: %+v", herr)
	}
	if redirect != nil {
		t.Fatalf("runFormAction redirected unexpectedly: %+v", redirect)
	}
	if received.ID != "k1" {
		t.Fatalf("the handler received id %q, want \"k1\" — the key never reached it", received.ID)
	}

	// The composite id this submission's outcome has to travel under: kit's
	// own `${__.id}/${__.key}`, unencoded, which is both the `<global>.data.f`
	// key kit's real client reads on hydration and the id the render engine's
	// `seed_form` looks the keyed instance up by.
	wantActionID := sendMessage.ID() + `/"k1"`
	if action.id != wantActionID {
		t.Fatalf("action.id = %q, want %q", action.id, wantActionID)
	}
}
