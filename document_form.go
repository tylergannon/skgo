// A form submitted without JavaScript.
//
// Kit's client normally intercepts a form's submit event, posts the controls
// as its own binary envelope to `/_app/remote/<id>`, and applies the answer to
// the page it is already on. With scripting off there is no client: the browser
// performs the form's own POST to `action`, which kit's `form` instance renders
// as `?/remote=<id>` on the page's own URL, and the answer has to be the whole
// page again with the submission's outcome already in it.
//
// That is what this file does, and it is `render_page`'s action arm
// (`packages/kit/src/runtime/server/page/index.js`) together with
// `handle_remote_form_post_internal` (`runtime/server/remote-functions.js`).
// The one thing worth knowing before reading it: for a *remote* form there is
// no `form:` in the boot object. `handle_remote_form_post_internal` returns
// `{type: 'success', status, location}` with no `data`, so kit's `render.js`
// computes `form_value = null` and never reaches `uneval_action_response` —
// that slot belongs to the classic `+page.server.js` actions kit also has and
// skgo has no equivalent of. The submission travels in `<global>.data.f`
// instead, keyed by the action id, which is where `_start` copies it from into
// the query cache and where `form.svelte.js` reads `result`, `issues` and
// `input` out of it as the form's initial state.
package skgo

import (
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"

	"github.com/tylergannon/skgo/internal/devalue"
	"github.com/tylergannon/skgo/internal/formdata"
	"github.com/tylergannon/skgo/internal/ssr"
)

// formAction is one non-enhanced submission, once its Go handler has run.
type formAction struct {
	// id is the client-side action id — `<hash>/<name>` — which is both the
	// key `<global>.data.f` uses and the id the engine looks the form instance
	// up by when it seeds the render.
	id string
	// output is kit's form output object: `{submission, result}` for a
	// submission that succeeded and `{submission, issues, input}` for one the
	// handler rejected. The property order is kit's own, because these bytes
	// are what the browser parses.
	output *devalue.Object
	// jar carries the cookies the handler wrote. A form may write them — kit
	// permits it in forms and commands and nowhere else — and they belong on
	// the document this submission is answered with.
	jar *cookieJar
}

// actionID is kit's `get_remote_action`: the `?/remote=<id>` a form's action
// attribute carries, or "" for a POST that is not one.
func actionID(u *url.URL) string { return u.Query().Get("/remote") }

// runFormAction runs the form the submission names and shapes its outcome the
// way kit's own server shapes it.
//
// Every return other than the first is an answer for the whole document: a
// redirect thrown by the handler becomes a bare 3xx, and anything else becomes
// the error page at the status the error carried, which is what kit does by
// rethrowing `action_result.error` at the leaf node.
func (s *SSR) runFormAction(r *http.Request, id string) (*formAction, *Redirect, *HTTPError) {
	// Kit refuses a cross-site form submission before it looks anything up
	// (`runtime/server/csrf.js`, `is_csrf_forbidden`): a mutating method with
	// a form content type whose Origin is not the app's own. The remote
	// endpoint's own check does not cover this request — that one is keyed on
	// the `/_app/remote/` pathname, and this arrives on the page's URL.
	if s.loads.origin != nil && r.Header.Get("Origin") != s.loads.origin.String() {
		media := mediaType(r.Header.Get("Content-Type"))
		if media == "" || isFormContentType(media) {
			return nil, nil, &HTTPError{Status: 403, Message: "Cross-site POST form submissions are forbidden"}
		}
	}

	fn, ok := s.remotes.Lookup(id)
	if !ok || fn.kind != kindForm {
		// Kit's `method_not_allowed_result`: an id that names no form is a 405
		// and the page renders its error boundary at that status. The Allow
		// header a 405 must carry is set by the caller, which holds the
		// ResponseWriter; this is the only 405 this function returns.
		return nil, nil, &HTTPError{Status: http.StatusMethodNotAllowed, Message: "POST method not allowed. No form actions exist for this page"}
	}

	entries, err := readFormEntries(r)
	if err != nil {
		return nil, nil, asHTTPError(err)
	}
	arg, err := formdata.Convert(id, entries)
	if err != nil {
		return nil, nil, &HTTPError{Status: 400, Message: "Bad Request"}
	}

	ev := s.remotes.newEvent(r, true)
	// An empty set rather than none. A handler that ends
	// `return out, skgo.RefreshRequestedNoArg(ctx, q)` is refusing to refresh
	// anything the client did not ask for, and a submission the browser made
	// on its own cannot ask — so the answer is "nothing was requested", not
	// "you are not in a form". Nothing is lost by it: the page is being
	// rendered again from the top, so every query on it runs regardless, which
	// is what single-flight refreshes exist to avoid having to do.
	ev.refreshes = newRefreshSet(s.remotes, nil)
	action := &formAction{id: id, jar: ev.jar}

	value, err := s.remotes.call(withEvent(r.Context(), ev), fn, arg, true)
	if err != nil {
		var invalid *Invalid
		if errors.As(err, &invalid) {
			// Kit's `handle_issues`: the issues, and then the input, because a
			// submission that navigated has no controls left holding what the
			// visitor typed. `output.submission` is set before either.
			action.output = devalue.NewObject(
				"submission", true,
				"issues", issueNodes(invalid.Issues),
				"input", submittedInput(id, entries),
			)
			return action, nil, nil
		}
		if redirect := asRedirect(err); redirect != nil {
			return nil, redirect, nil
		}
		return nil, nil, asHTTPError(err)
	}

	tree, terr := s.remotes.cfg.Transport.encodeTree(value)
	if terr != nil {
		return nil, nil, asHTTPError(terr)
	}
	action.output = devalue.NewObject("submission", true, "result", tree)
	return action, nil, nil
}

// submittedInput is the `input` half of kit's `handle_issues`: the strings the
// browser sent, nested by the path each control's name encodes, so the
// re-rendered form can put the visitor's words back into the controls.
//
// Two details are kit's and look like mistakes until you check: the values are
// *not* coerced — `n:age` comes back as the text that was typed, not a number —
// and the redaction test is applied to the raw key rather than to the field's
// path, so it hides a top-level `_secret` and nothing deeper.
func submittedInput(formID string, entries []formdata.Entry) *devalue.Object {
	visible := make([]formdata.Entry, 0, len(entries))
	for _, entry := range entries {
		if redactedFormKey(entry.Name) {
			continue
		}
		// A file entry passes through unchanged. Kit's `form_data.getAll`
		// still yields the File, and it is `handle_issues`'s own
		// `.filter((value) => typeof value === 'string')` that drops it
		// before `values[0]` is taken — an untouched file input answers
		// undefined, not "". ConvertRaw's own File check reproduces exactly
		// that filtering, so rewriting the entry here would only defeat it.
		visible = append(visible, entry)
	}

	input, err := formdata.ConvertRaw(formID, visible)
	if err != nil {
		// The same entries already went through Convert to build the argument,
		// so a failure here cannot be about the shape of the form; an empty
		// input is closer to kit than a 500 would be.
		return devalue.NewObject()
	}
	return input
}

// redactedFormKey is kit's `/^[.\]]?_/` over the raw field name: a control
// whose name starts with an underscore is not echoed back.
func redactedFormKey(key string) bool {
	rest := key
	if len(rest) > 0 && (rest[0] == '.' || rest[0] == ']') {
		rest = rest[1:]
	}
	return strings.HasPrefix(rest, "_")
}

// readFormEntries reads the controls of an ordinary form POST in the order the
// document declared them.
//
// Order is preserved on purpose: it is the order `FormData` iterates in, and
// therefore the property order of the object kit builds, which is part of the
// bytes the browser parses back.
func readFormEntries(r *http.Request) ([]formdata.Entry, error) {
	media, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		return nil, &HTTPError{Status: 415, Message: "Unsupported Media Type"}
	}

	switch media {
	case "application/x-www-form-urlencoded":
		body, err := io.ReadAll(io.LimitReader(r.Body, maxFormBytes+1))
		if err != nil || len(body) > maxFormBytes {
			return nil, &HTTPError{Status: 413, Message: "Payload Too Large"}
		}
		return parseURLEncoded(string(body))

	case "multipart/form-data":
		reader := multipart.NewReader(io.LimitReader(r.Body, maxFormBytes), params["boundary"])
		var entries []formdata.Entry
		for {
			part, err := reader.NextPart()
			if errors.Is(err, io.EOF) {
				return entries, nil
			}
			if err != nil {
				return nil, &HTTPError{Status: 400, Message: "Bad Request"}
			}
			data, err := io.ReadAll(part)
			if err != nil {
				return nil, &HTTPError{Status: 400, Message: "Bad Request"}
			}
			// A file input the visitor left alone still submits a part, with
			// an empty filename and no bytes — `part.FileName()` cannot tell
			// that from a text control, and a "" assigned to a File field is a
			// type error rather than the absent file it really is. The
			// `filename` parameter's presence is what says the control was a
			// file input; `Convert` then drops the empty one, as kit does.
			_, cdParams, cdErr := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
			if _, isFile := cdParams["filename"]; cdErr == nil && isFile {
				entries = append(entries, formdata.Entry{
					Name: part.FormName(),
					File: &formdata.File{
						Name: part.FileName(),
						Type: part.Header.Get("Content-Type"),
						Data: data,
					},
				})
				continue
			}
			entries = append(entries, formdata.Entry{Name: part.FormName(), Value: string(data)})
		}

	default:
		return nil, &HTTPError{Status: 415, Message: "Unsupported Media Type"}
	}
}

// parseURLEncoded is `application/x-www-form-urlencoded` with the order kept.
// net/url's own parser answers with a map, and a map has no order.
func parseURLEncoded(body string) ([]formdata.Entry, error) {
	var entries []formdata.Entry
	for _, pair := range strings.Split(body, "&") {
		if pair == "" {
			continue
		}
		key, value, _ := strings.Cut(pair, "=")
		name, err := url.QueryUnescape(key)
		if err != nil {
			return nil, &HTTPError{Status: 400, Message: "Bad Request"}
		}
		decoded, err := url.QueryUnescape(value)
		if err != nil {
			return nil, &HTTPError{Status: 400, Message: "Bad Request"}
		}
		entries = append(entries, formdata.Entry{Name: name, Value: decoded})
	}
	return entries, nil
}

// seed is what the render is given so that the page can show the submission.
//
// The output goes into the engine as devalue's flat form — the same bytes a
// remote call's answer travels in — because `result` may hold a type the app's
// transport hook owns, and the component that renders it expects the instance
// rather than the object its fields travelled in.
func (s *SSR) seed(action *formAction) (*ssr.Form, error) {
	if action == nil {
		return nil, nil
	}
	serialized, err := devalue.StringifyWith(action.output, s.remotes.cfg.Transport.reducers())
	if err != nil {
		return nil, err
	}
	return &ssr.Form{ID: action.id, Output: serialized}, nil
}
