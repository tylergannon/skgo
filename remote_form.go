// The Go side of SvelteKit's `form` remote function.
//
// A form is the fourth remote kind and the only one whose argument is not a
// devalue payload in a URL or a JSON body. A hydrated kit page posts a form as
// `application/x-sveltekit-formdata`: kit's client turns the FormData into a
// POJO first — coercing types, nesting dotted names — and appends any uploaded
// file's bytes raw after the header. That is why a form cannot reuse
// the generated devalue decoder every other kind uses: a File is not a JSON
// value and polytype describes none, so the generated closure assigns the
// submission onto the handler's own argument type with skgo.DecodeForm.
//
// The two things a form does that a command does not are both kit's design,
// not additions here. Its handler may report validation failures against named
// fields, which kit's client hangs off `form.fields.<name>.issues()`, and a
// submission carrying issues suppresses single-flight refreshes entirely —
// kit's server returns before it collects them.
package skgo

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/tylergannon/polytype/devalue"
	"github.com/tylergannon/skgo/internal/formdata"
)

// File is a file uploaded through a form. Give a form's argument struct a File
// field and the bytes the visitor chose arrive in it.
type File = formdata.File

// maxFormBytes caps a form submission. Kit's client streams the body and never
// holds a whole upload in memory; Go reads it into memory to resolve the file
// offset table, so it needs a limit or a single request could exhaust the
// process. 32 MiB is generous for a form and small enough that it cannot be
// used as a denial of service.
const maxFormBytes = 32 << 20

// Issue is one validation failure, reported against the field it is about.
//
// Field is the field's path as the form names it: "email" for a top-level
// field, "author.name" for a nested one, "tags[0]" for an element of an array
// field. It must match the path the page used in `form.fields...`, because
// kit's client matches issues to fields by exactly that string. An empty Field
// puts the issue on the form as a whole, where `form.fields.allIssues()`
// finds it.
type Issue struct {
	Field   string
	Message string
}

// Invalid is the error a form handler returns to report validation failures.
// Kit treats a submission with issues as neither a success nor a server error:
// the client keeps the page as it is, hangs each message off its field, and —
// because kit's server returns before it collects them — runs none of the
// single-flight refreshes the submission asked for.
//
// The visitor's typed input is not sent back and does not need to be. A
// progressively-enhanced submission never navigates, so the values are still
// in the inputs; kit's own server skips the input entirely for exactly this
// case ("we don't need to return the input — it's already there").
type Invalid struct {
	Issues []Issue
}

// Invalidf reports one field invalid. It is the common case, and reads at the
// call site like the check it is:
//
//	if !strings.Contains(arg.Email, "@") {
//	    return Contact{}, skgo.Invalidf("email", "%q is not an email address", arg.Email)
//	}
func Invalidf(field, format string, args ...any) *Invalid {
	return &Invalid{Issues: []Issue{{Field: field, Message: fmt.Sprintf(format, args...)}}}
}

// Add appends another issue, so a handler can collect every problem with a
// submission rather than reporting them one round-trip at a time.
func (v *Invalid) Add(field, format string, args ...any) *Invalid {
	v.Issues = append(v.Issues, Issue{Field: field, Message: fmt.Sprintf(format, args...)})
	return v
}

// Err returns v, or nil when nothing was found wrong. It exists so a handler
// that accumulates issues can end with `return out, issues.Err()` rather than
// with a length check that is easy to get backwards.
func (v *Invalid) Err() error {
	if v == nil || len(v.Issues) == 0 {
		return nil
	}
	return v
}

func (v *Invalid) Error() string {
	if len(v.Issues) == 0 {
		return "invalid form submission"
	}
	parts := make([]string, 0, len(v.Issues))
	for _, issue := range v.Issues {
		if issue.Field == "" {
			parts = append(parts, issue.Message)
			continue
		}
		parts = append(parts, issue.Field+": "+issue.Message)
	}
	return "invalid form submission: " + strings.Join(parts, "; ")
}

// Form declares fn as a SvelteKit `form`. Write it beside the function, in a
// file named `*.remote.go`:
//
//	func subscribe(ctx context.Context, arg Signup) (Result, error) { ... }
//
//	var _ = skgo.Form(subscribe)
//
// A form's event may write cookies, which kit permits in forms and commands
// and nowhere else. Return a *Invalid to put messages on specific fields.
func Form[In, Out any](fn func(context.Context, In) (Out, error)) Marker { _ = fn; return Marker{} }

func (rs *Remotes) serveForm(w http.ResponseWriter, r *http.Request, fn *Remote) {
	if r.Method != http.MethodPost {
		rs.writeErrorStatus(w, &HTTPError{
			Status:  405,
			Message: "`form` functions must be invoked via POST request, not " + r.Method,
		}, http.StatusMethodNotAllowed)
		return
	}

	// Kit refuses a non-form content type with 415 before it reads a byte,
	// because the form content types are the ones a cross-origin <form> can
	// send and so the ones its CSRF protection is written around.
	media := mediaType(r.Header.Get("Content-Type"))
	if !isFormContentType(media) {
		rs.writeErrorStatus(w, &HTTPError{
			Status:  415,
			Message: "`form` functions expect form-encoded data — received " + r.Header.Get("Content-Type"),
		}, http.StatusUnsupportedMediaType)
		return
	}

	// Of those, only kit's binary envelope can be answered here. The rest
	// belong to a submission kit did not enhance, which posts to the page's
	// own URL with `?/remote=<id>` and is answered by rendering the page —
	// something a CSR-only app has no server-side renderer to do.
	if media != formdata.ContentType {
		rs.writeErrorStatus(w, &HTTPError{
			Status:  415,
			Message: "`form` functions need an enhanced submission; this app renders on the client only",
		}, http.StatusUnsupportedMediaType)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxFormBytes))
	if err != nil {
		// The only error a MaxBytesReader adds is the cap, and a body that
		// large is the client's problem rather than a fault here.
		rs.writeErrorStatus(w, &HTTPError{Status: 413, Message: "Payload Too Large"}, http.StatusRequestEntityTooLarge)
		return
	}

	arg, meta, err := formdata.ParseWith(body, rs.cfg.Transport.revivers())
	if err != nil {
		rs.writeError(w, &HTTPError{Status: 400, Message: "Bad Request"})
		return
	}

	// `validate_only` asks for the declarative schema's verdict without
	// running the handler. skgo's stubs are declared `'unchecked'` — the
	// validation is the Go handler's own work — so there is no schema to
	// consult and the honest answer is the one kit gives for an unchecked
	// form: no issues. Running the handler here instead would execute its
	// side effects on every keystroke.
	if meta.ValidateOnly {
		rs.writeResult(w, rs.newEvent(r, false), map[string]any{"_": []any{}})
		return
	}

	ev := rs.newEvent(r, true)
	// Indexed, not obeyed — see requested.go.
	ev.refreshes = newRefreshSet(rs, meta.RemoteRefreshes)
	ctx := withEvent(r.Context(), ev)

	value, err := rs.call(ctx, fn, rs.newCall(arg, true))
	if err != nil {
		var invalid *Invalid
		if errors.As(err, &invalid) {
			// A submission with issues stops here. Kit's server returns before
			// it collects refreshes, so the client sees no `q`, no `l` and no
			// `r` — and therefore leaves the page exactly as it is, which is
			// what keeps the visitor's typed input on screen.
			rs.writeResult(w, ev, map[string]any{"_": devalue.NewObject(
				"submission", true,
				"issues", issueNodes(invalid.Issues),
			)})
			return
		}
		if redirect := asRedirect(err); redirect != nil {
			// Unlike a command, a form redirect is honoured: kit's form client
			// reads `redirect` off the response and navigates.
			rs.writeResult(w, ev, map[string]any{"redirect": redirect.Location})
			return
		}
		rs.writeError(w, asHTTPError(err))
		return
	}

	data := map[string]any{"_": devalue.NewObject("submission", true, "result", value)}
	q, l := rs.collectRefreshes(r.Context(), ev)
	if len(q) > 0 {
		data["q"] = q
	}
	if len(l) > 0 {
		data["l"] = l
	}
	if len(q) > 0 || len(l) > 0 {
		// `r` is the one flag only a form reads. It tells `form.svelte.js`
		// that the server already performed the single-flight updates, so the
		// blanket `refreshAll()` an unenhanced submission would need is not
		// wanted. Without it every successful submission re-runs every query
		// on the page.
		data["r"] = true
	}
	rs.writeResult(w, ev, data)
}

// issueNodes renders the issues in the shape kit's client expects. Each issue
// carries both the segment path and the joined name, because the client uses
// them differently: `flatten_issues` walks `path` to index an issue under every
// prefix of its field, while the field proxy matches `name` exactly to decide
// which messages belong to one input.
func issueNodes(issues []Issue) []any {
	nodes := make([]any, 0, len(issues))
	for _, issue := range issues {
		path := splitFieldPath(issue.Field)
		nodes = append(nodes, devalue.NewObject(
			"name", buildFieldPath(path),
			"path", path,
			"message", issue.Message,
			// `server` marks an issue the server raised. Kit's client keeps
			// these across a re-validation that produces no client-side issue
			// for the same field, so a message the server alone can produce —
			// "that address is already registered" — does not vanish the
			// moment the visitor touches the input.
			"server", true,
		))
	}
	return nodes
}

// splitFieldPath turns "author.name" into ["author", "name"] and "tags[0]"
// into ["tags", 0], mirroring kit's `split_path`. A numeric segment has to be
// a number on the wire, not a string, because kit's client renders it back as
// `[0]` rather than `.0`.
func splitFieldPath(field string) []any {
	if field == "" {
		return []any{}
	}
	var path []any
	for _, segment := range strings.FieldsFunc(field, func(r rune) bool {
		return r == '.' || r == '[' || r == ']'
	}) {
		if n, err := strconv.Atoi(segment); err == nil {
			// devalue serialises the JSON value set, so an index is a float.
			path = append(path, float64(n))
			continue
		}
		path = append(path, segment)
	}
	if path == nil {
		return []any{}
	}
	return path
}

// buildFieldPath is kit's `build_path_string`: the name an input carries.
func buildFieldPath(path []any) string {
	var b strings.Builder
	for _, segment := range path {
		if n, ok := segment.(float64); ok {
			fmt.Fprintf(&b, "[%d]", int(n))
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('.')
		}
		b.WriteString(segment.(string))
	}
	return b.String()
}

// mediaType is the Content-Type header without its parameters, lower-cased —
// `multipart/form-data; boundary=...` is `multipart/form-data`.
func mediaType(header string) string {
	media, _, _ := strings.Cut(header, ";")
	return strings.ToLower(strings.TrimSpace(media))
}

// isFormContentType mirrors kit's check of the same name: the media types a
// plain cross-origin <form> can send, plus kit's own binary envelope. These are
// exactly the types kit's CSRF protection has to cover, which is why the set is
// kit's and not a convenience.
func isFormContentType(media string) bool {
	switch media {
	case "application/x-www-form-urlencoded", "multipart/form-data", "text/plain", formdata.ContentType:
		return true
	}
	return false
}
