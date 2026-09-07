package example_test

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A form submitted by a browser with scripting off, against the production
// stack: the POST kit's `form` instance renders as the form's `action`, and the
// document Go answers it with.
//
// The values asserted here are the app's own — `sendMessage` in
// `src/routes/contact/contact.remote.go` writes them and the generated
// `contact.remote.ts` throws in every body — so markup carrying them is proof
// the Go handler ran and its outcome reached the render.

func TestTheFormRendersItsActionSoABrowserCanPostItWithoutAScript(t *testing.T) {
	h := newProdHandler(t)
	body := get(t, h, "/contact").Body.String()

	id := remoteID(t, "sendMessage")
	want := `action="?/remote=` + id + `" method="POST"`
	if !strings.Contains(body, want) {
		t.Fatalf("the contact form does not carry %s; nothing tells the browser where to post it", want)
	}
}

func TestARefusedSubmissionComesBackAsThePageWithItsIssues(t *testing.T) {
	h := newProdHandler(t)
	id := remoteID(t, "sendMessage")

	rec := submit(t, h, id, map[string]string{
		"from":  "Grace Hopper",
		"email": "grace-at-example",
		"body":  "too short",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 — kit answers a refused submission with the page", rec.Code)
	}
	body := rec.Body.String()

	// The messages are the Go handler's own sentences, with the values the
	// submission carried interpolated into them.
	// Anchored on the test id and the sentence, not on the whole tag: Svelte's
	// scoped class hash is part of the markup and changes whenever the page's
	// <style> does, which has nothing to do with what this test is about.
	for _, want := range []string{
		`data-testid="issue-email">"grace-at-example" is not an email address</p>`,
		`data-testid="issue-body">A message needs at least 10 characters; this one has 9</p>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the re-rendered page does not show %s", want)
		}
	}
	if strings.Contains(body, `data-testid="issue-from"`) {
		t.Error("the re-rendered page puts a message on a field the handler did not name")
	}

	// The controls the visitor typed into come back holding what they typed.
	if !strings.Contains(body, `data-testid="field-email" name="email/`+id+`" aria-invalid="true" value="grace-at-example"`) {
		t.Error("the re-rendered email input does not hold what was typed into it")
	}

	// And the same output travels to the client, so hydration starts the form
	// where the document left it rather than empty.
	if !strings.Contains(body, `f:{"`+id+`":{v:{submission:true,issues:[`) {
		t.Errorf("the boot payload carries no `f` entry for %s", id)
	}
	if !strings.Contains(body, "form: null") {
		t.Error("the boot object does not carry `form: null`; a remote form never fills that slot")
	}
}

func TestASuccessfulSubmissionComesBackAsThePageWithItsResult(t *testing.T) {
	h := newProdHandler(t)
	id := remoteID(t, "sendMessage")

	rec := submit(t, h, id, map[string]string{
		"from":  "Ada Lovelace",
		"email": "ada@example.com",
		"body":  "The analytical engine weaves algebraic patterns.",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()

	// The receipt's sentence is built in Go out of the name the submission
	// carried; only the handler can produce it.
	if !strings.Contains(body, `<p data-testid="receipt">Thanks, Ada Lovelace — message m`) {
		t.Error("the re-rendered page does not show the receipt the Go handler returned")
	}
	if strings.Contains(body, `data-testid="rejected"`) {
		t.Error("the re-rendered page says a successful submission was not sent")
	}
	if !strings.Contains(body, `f:{"`+id+`":{v:{submission:true,result:{id:"m`) {
		t.Errorf("the boot payload carries no result for %s", id)
	}
}

// Kit refuses a cross-site form submission before it looks the form up
// (`runtime/server/csrf.js`), and a browser that sends no Origin at all is
// refused by the same rule — `!request_origin` is one of the ways
// `is_csrf_forbidden` becomes true.
func TestASubmissionFromAnotherSiteIsRefused(t *testing.T) {
	h := newProdHandler(t)
	id := remoteID(t, "sendMessage")

	for name, origin := range map[string]string{
		"another origin": "http://evil.example",
		"no origin":      "",
	} {
		req := multipartRequest(t, id, map[string]string{"from": "Mallory", "email": "m@example.com", "body": "a long enough message"})
		if origin != "" {
			req.Header.Set("Origin", origin)
		} else {
			req.Header.Del("Origin")
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("%s: status = %d, want 403", name, rec.Code)
		}
	}
}

// A POST to a page that names no form is kit's `method_not_allowed_result`.
func TestAPostToAPageThatNamesNoFormIsNotAllowed(t *testing.T) {
	h := newProdHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/contact", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", prodOrigin)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

// submit is the POST a browser makes when the visitor presses the submit
// button of `<form action="?/remote=<id>" method="POST"
// enctype="multipart/form-data">` — the same encoding, the same field names,
// and the Origin header a browser sends with it.
func submit(t *testing.T, h http.Handler, id string, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, multipartRequest(t, id, fields))
	return rec
}

func multipartRequest(t *testing.T, id string, fields map[string]string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	// The controls in the order the document declares them, so that the object
	// Go builds has the property order a browser's FormData would give it.
	for _, name := range []string{"from", "email", "body"} {
		if value, ok := fields[name]; ok {
			if err := form.WriteField(name+"/"+id, value); err != nil {
				t.Fatalf("writing the %s field: %v", name, err)
			}
		}
	}
	// An untouched `<input type="file">` still submits a part, with an empty
	// filename and no bytes. Kit drops it; a File field that received the empty
	// string instead would be a type error, and this is the shape that finds it.
	if _, err := form.CreateFormFile("attachment/"+id, ""); err != nil {
		t.Fatalf("writing the empty file part: %v", err)
	}
	if err := form.Close(); err != nil {
		t.Fatalf("closing the form: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/contact?/remote="+id, &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	req.Header.Set("Origin", prodOrigin)
	return req
}
