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
	"encoding/json"
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
	// id is the client-side action id: `<hash>/<name>`, or
	// `<hash>/<name>/<key>` for a submission to a `form.for(key)` instance,
	// with `<key>` the JSON text of the key exactly as `JSON.stringify(key)`
	// produced it — no URL encoding. That is kit's own format
	// (`runtime/app/server/remote/form.js`: `__.key ? \`${__.id}/${__.key}\` :
	// __.id`) for two things this one string has to be, at once: the key
	// `<global>.data.f` carries the submission under for kit's real client to
	// find on hydration, and the id the engine's `seed_form` looks the form
	// instance up by — calling `.for(key)` itself when there is one — so that
	// `sendMessage.for(key)` reads the same cache entry during the render.
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

// splitActionID is kit's own split, from `handle_remote_form_post_internal`:
//
//	const [hash, name, ...rest] = id.split('/');
//	const action_id = rest.join('/');
//
// A hash and a name never contain a "/", so the first two segments are the
// form's own id; a `form.for(key)` instance's key can — its JSON text may
// itself contain one, restored by url-decoding a "%2F" the browser's
// `encodeURIComponent` put there — so whatever segments remain are rejoined
// rather than the third one taken alone. hasKey is false for an id with no
// third segment and for one whose third segment is empty, mirroring kit's own
// `if (action_id)` on the rejoined (possibly "") string.
func splitActionID(id string) (base, key string, hasKey bool) {
	hash, rest, ok := strings.Cut(id, "/")
	if !ok {
		return id, "", false
	}
	name, tail, hasTail := strings.Cut(rest, "/")
	base = hash + "/" + name
	if !hasTail || tail == "" {
		return base, "", false
	}
	return base, tail, true
}

// setFormKey is kit's own line, once a keyed submission's form is resolved:
//
//	if (action_id && !('id' in data)) data.id = JSON.parse(decodeURIComponent(action_id));
//
// keyJSON is JSON text — the decoded action id — so this is the `JSON.parse`
// half; the `decodeURIComponent` half is already done by the time a caller
// reads a query parameter's value in Go, the same as it is by the time kit
// reads `URLSearchParams.get(...)` in JavaScript.
func setFormKey(arg any, keyJSON string) error {
	obj, ok := arg.(*devalue.Object)
	if !ok {
		return nil
	}
	if _, has := obj.Get("id"); has {
		return nil
	}
	var key any
	if err := json.Unmarshal([]byte(keyJSON), &key); err != nil {
		return err
	}
	obj.Set("id", key)
	return nil
}

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

	// `id` is kit's own `[hash, name, ...rest]` split
	// (`handle_remote_form_post_internal`): hash and name can never contain a
	// "/", but the JSON-stringified key of a `form.for(key)` instance can —
	// url-decoding a slash back out of `%2F` is exactly how it gets here — so
	// the remaining segments are rejoined rather than the third one taken
	// alone. baseID is what the form is registered under and what every field
	// name is suffixed with, keyed or not; kit's client builds both a keyed
	// and an unkeyed instance's controls with `action_id_without_key`.
	baseID, actionKey, hasKey := splitActionID(id)

	fn, ok := s.remotes.Lookup(baseID)
	if !ok {
		// Kit's `method_not_allowed_result`
		// (`handle_remote_form_post_internal`, `if (!form)`): an id that
		// resolves to nothing is a 405 and the page renders its error
		// boundary at that status. The Allow header a 405 must carry is set
		// by the caller, which holds the ResponseWriter; this is the only
		// 405 this function returns.
		return nil, nil, &HTTPError{Status: http.StatusMethodNotAllowed, Message: "POST method not allowed. No form actions exist for this page"}
	}
	if fn.kind != KindForm {
		// Not a 405. Kit only checks that the name resolves; a query or
		// command named here has no `__.fn`, the call throws, and
		// `action_error_result` turns the throw into kit's opaque 500
		// (`get_status` of anything but an HttpError). Mirror the status,
		// not the accident: refuse before touching the body.
		return nil, nil, &HTTPError{Status: http.StatusInternalServerError, Message: "Internal Error"}
	}

	entries, err := readFormEntries(r)
	if err != nil {
		return nil, nil, asHTTPError(err)
	}
	arg, err := formdata.Convert(baseID, entries)
	if err != nil {
		return nil, nil, &HTTPError{Status: 400, Message: "Bad Request"}
	}
	if hasKey {
		// Kit's own line, once the form is resolved: `if (action_id &&
		// !('id' in data)) data.id = JSON.parse(decodeURIComponent(action_id))`.
		// `actionKey` already went through the one round of percent-decoding
		// `url.Query()` does — the same job `decodeURIComponent` does there —
		// so it is JSON text, ready to parse.
		if err := setFormKey(arg, actionKey); err != nil {
			return nil, nil, &HTTPError{Status: 400, Message: "Bad Request"}
		}
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
	// The composite id kit's own key format uses — see formAction.id — built
	// once, here, rather than recomputed at every place that needs it.
	compositeID := baseID
	if hasKey {
		compositeID = baseID + "/" + actionKey
	}
	action := &formAction{id: compositeID, jar: ev.jar}

	value, err := s.remotes.call(withEvent(r.Context(), ev), fn, s.remotes.newCall(arg, true))
	if err != nil {
		var invalid *Invalid
		if errors.As(err, &invalid) {
			// Kit's `handle_issues`: the issues, and then the input, because a
			// submission that navigated has no controls left holding what the
			// visitor typed. `output.submission` is set before either.
			action.output = devalue.NewObject(
				"submission", true,
				"issues", issueNodes(invalid.Issues),
				"input", submittedInput(baseID, entries),
			)
			return action, nil, nil
		}
		if redirect := asRedirect(err); redirect != nil {
			return nil, redirect, nil
		}
		return nil, nil, asHTTPError(err)
	}

	// value is already the tree the form's generated encoder produced.
	action.output = devalue.NewObject("submission", true, "result", value)
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
