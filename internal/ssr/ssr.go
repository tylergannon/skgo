// Package ssr runs SvelteKit's own server renderer inside the Go process.
//
// The JavaScript it executes is the bundle the skgo adapter built: kit's `Root`
// component, kit's `Props`/`RenderNode`, kit's remote-function wrappers and
// Svelte's server renderer, with the app's own components. Nothing in it reads
// a file, opens a socket or sets a timer — every remote function's body is a
// call back out to Go — so the engine needs no host beyond a handful of web
// globals the bundle carries itself.
//
// One runtime renders one page at a time. That is not a tuning choice: Svelte's
// no-AsyncLocalStorage mode keeps its render context in a module global, and a
// render started while another is in flight on the same runtime silently
// returns an empty document. The pool is what makes concurrent requests safe,
// and Result.Done is what proves a render actually finished.
package ssr

import (
	"context"
	"encoding/json"
	"fmt"
)

// Host answers one remote-function call made during a render. id is
// `<hash>/<name>`, as kit's own vite plugin assigns it, and payload is
// `stringify_remote_arg(arg)`; together they are the key the browser's query
// cache looks the value up under. The returned bytes are the JSON envelope the
// bundle parses: `{"v": "<devalue>"}` or `{"e": {"status": n, "message": "..."}}`.
//
// `v` is a string rather than a value, and for the same reason Node.Data is:
// it is devalue's flat form, so the bundle can hand it to the app's own
// decoders and a custom type arrives in the render with its methods.
type Host func(ctx context.Context, id, payload string) ([]byte, error)

// Request is what a render is given. It is marshalled straight to the bundle's
// entry point, so the field names are the ones the entry reads.
type Request struct {
	// URL is the absolute URL of the page being rendered.
	URL    string `json:"url"`
	Method string `json:"method,omitempty"`
	// RouteID is kit's route id, or "" for a request that matched no route.
	RouteID string `json:"route_id"`
	// Params are the route's parameters.
	Params map[string]string `json:"params"`
	// Status is the status the document will be answered with.
	Status int `json:"status"`
	// Error is the error a page is rendering, or nil.
	Error *Error `json:"error"`
	// Form is the value of `page.form`.
	//
	// It is null for a remote form. Kit's `handle_remote_form_post_internal`
	// answers with `{type: 'success', status, location}` and no `data`, so
	// `render.js` computes `form_value = null`; `page.form` is the classic
	// `+page.server.js` actions slot, which skgo has no equivalent of. A
	// remote submission travels in FormAction instead.
	Form any `json:"form"`
	// FormAction is a form submission that was posted without JavaScript, if
	// this render is answering one.
	FormAction *Form `json:"form_action,omitempty"`
	// Branch is the route's nodes, outermost first, with the data each one's
	// load produced.
	Branch []Node `json:"branch"`
	// ErrorComponents is one node index per branch slot whose `+error.svelte`
	// should catch a throw from that slot, or nil for a slot with none.
	ErrorComponents []*int `json:"error_components"`
	// Cookies are the request's cookies, by name.
	Cookies map[string]string `json:"cookies"`
	Headers map[string]string `json:"headers"`
	CSR     bool              `json:"csr"`
	// Flags describe the incoming request, including an internal fetch.
	IsDataRequest   bool `json:"is_data_request"`
	IsRemoteRequest bool `json:"is_remote_request"`
	IsSubRequest    bool `json:"is_sub_request"`
	// CSP is this render's Svelte-facing csp option, kit's own
	// `csp.script_needs_nonce ? { nonce: csp.nonce } : { hash:
	// csp.script_needs_hash }` (`page/render.js:198`), passed to Svelte's
	// own `render(Root, options)` call. Go's csp.go decides both booleans
	// before the engine ever runs — they depend only on the build's
	// configured directives, never on what a render produces — so this is
	// always the same answer the boot script's own nonce/hash attribute
	// (assembled separately, in Go, after the render) will use. It governs
	// the one other inline `<script>` a rendered document can carry:
	// Svelte's own hydratable-async-block script
	// (`internal/server/renderer.js`'s `#hydratable_block`), emitted when a
	// component's own top-level `await` needs to hand the client a value it
	// already resolved during SSR.
	CSP RequestCSP `json:"csp"`
}

// RequestCSP is one render's Svelte-facing csp option. Svelte's own renderer
// checks `csp.nonce` first and falls back to `csp.hash` only when that is
// empty, so carrying both fields is harmless — but Nonce and Hash are never
// both meaningful at once, mirroring kit's own ternary: documentCSP's
// ScriptNeedsNonce and ScriptNeedsHash cannot both be true for the same
// request.
type RequestCSP struct {
	// Nonce is this request's nonce, set only when the build's csp needs
	// one. "" is Svelte's own default (`csp = { hash: false }`,
	// `internal/server/renderer.js`).
	Nonce string `json:"nonce,omitempty"`
	// Hash is kit's `csp.script_needs_hash`.
	Hash bool `json:"hash"`
}

// Node is one slot of a route's branch.
type Node struct {
	// Index is the node's index in the manifest's node table.
	Index int `json:"node"`
	// Data is what the node's server load returned, serialized exactly as it
	// is for `__data.json`: devalue's flat form, with the app's transport
	// encoders applied. The bundle parses it back with the app's own decoders,
	// so a component renders against the same instance the browser will hold —
	// a `Money` with its `format()`, not the object its fields travelled in.
	//
	// A plain tree would lose that: JSON has no way to say which class an
	// object belongs to, and a method call on the object that arrived instead
	// throws in the middle of a render.
	//
	// A value the load promised crosses as kit's own promise placeholder,
	// `["Promise", <id>]`, and the bundle reads it back with a `Promise`
	// reviver into a promise that never settles — so an `{#await}` over one
	// renders its pending branch and the value follows the document down as a
	// chunk. Kit hands its renderer the promise itself, which is the same
	// thing said in a language that has one.
	//
	// "" is a node with no load.
	Data string `json:"data"`
}

// Form is one non-enhanced form submission, on its way into the render.
//
// The engine puts Output into the request's remote cache under the form
// instance ID names, which is exactly what kit's own `form` wrapper does at the
// end of a submission (`runtime/app/server/remote/form.js`). From there the
// instance's `result`, `fields.<name>.issues()` and `fields.<name>.as(...)`
// values are kit's own code reading kit's own cache, so the page renders the
// submission without knowing one happened.
type Form struct {
	// ID is the client-side action id: `<hash>/<name>`.
	ID string `json:"id"`
	// Output is `{submission, result}` or `{submission, issues, input}` in
	// devalue's flat form, encoded with the app's transport so a `result`
	// carrying a custom type arrives as an instance of its class.
	Output string `json:"output"`
}

// Error is kit's `App.Error`: status and message, plus whatever extra
// properties the app's `handleError` hook added. Kit's own type has no fixed
// shape beyond the two fields every error carries — an app may augment
// `App.Error` with fields of its own — so this flattens to one JSON object
// either way, which is what lets a component read `$page.error.someField`
// without skgo committing to any field beyond the two kit's own type
// guarantees.
type Error struct {
	Status  int
	Message string
	// Extra is whatever else the hook returned: a merge target rather than a
	// fixed struct, because kit's own hook contract is "return only the
	// properties you want to override" against a shape the app itself defines.
	Extra map[string]any
}

// HandledError is a Kit remote-prerender error whose response body has already
// passed through Kit's error handling. Document handling must render that body
// without running the app's HandleError hook again.
type HandledError struct {
	Body *Error
}

func (e *HandledError) Error() string {
	if e == nil || e.Body == nil {
		return "skgo: handled remote error"
	}
	return fmt.Sprintf("skgo: handled remote error (%d %s)", e.Body.Status, e.Body.Message)
}

// MarshalJSON flattens Extra alongside status and message, so a value with
// no extra properties round-trips exactly as the plain `{status, message}`
// object kit itself writes.
func (e *Error) MarshalJSON() ([]byte, error) {
	if e == nil {
		return []byte("null"), nil
	}
	out := make(map[string]any, len(e.Extra)+2)
	for k, v := range e.Extra {
		out[k] = v
	}
	if e.Status != 0 {
		out["status"] = e.Status
	}
	out["message"] = e.Message
	return json.Marshal(out)
}

// UnmarshalJSON reads status and message out by name and keeps everything
// else as Extra, the inverse of MarshalJSON.
func (e *Error) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if v, ok := raw["status"]; ok {
		if err := json.Unmarshal(v, &e.Status); err != nil {
			return err
		}
		delete(raw, "status")
	}
	if v, ok := raw["message"]; ok {
		if err := json.Unmarshal(v, &e.Message); err != nil {
			return err
		}
		delete(raw, "message")
	}
	if len(raw) == 0 {
		return nil
	}
	e.Extra = make(map[string]any, len(raw))
	for k, v := range raw {
		var value any
		if err := json.Unmarshal(v, &value); err != nil {
			return err
		}
		e.Extra[k] = value
	}
	return nil
}

// Result is one render.
type Result struct {
	// Done reports that the render finished. A render that neither resolved
	// nor rejected leaves it false, which is the only symptom of a re-entrant
	// render on one runtime — there is no error to catch.
	Done bool
	// Err is the message a render that threw left behind.
	Err string
	// Redirect is set when the render threw a redirect rather than an error —
	// a remote function called from a component, say. Kit answers the whole
	// document with a bare 3xx when that happens (`page/index.js`, the
	// `Redirect` arm of render_page's catch), so this is not a failure.
	Redirect *Redirect
	// Status is the status the document should be answered with. It is the one
	// the caller asked for unless an error boundary caught something, in which
	// case kit's `transformError` has replaced it with the caught error's.
	Status int
	// Error is `page.error` as the render left it: the error the caller passed
	// in, or the one a boundary caught and transformError jsonified. It is
	// what the boot script's `error:` carries.
	Error *Error
	// HandledError is set only when a built remote error escaped every page
	// error boundary. Its body has already been handled by Kit.
	HandledError *Error `json:"handled_error"`
	// Head is what the components put in `<svelte:head>`.
	Head string
	// Body is the rendered markup.
	Body string
	// Fetched is Kit's serialized universal-fetch cache for hydration.
	Fetched string
}

// Redirect is a redirect thrown during a render.
type Redirect struct {
	Status   int    `json:"status"`
	Location string `json:"location"`
}

// MaxRenderDepth is how many renders may wait on one another. Kit gives up on
// a page that fetches pages beyond ten requests deep, so no render the app
// can legitimately make nests further.
const MaxRenderDepth = 12

type depthKey struct{}

// Depth is how many renders on this call chain are waiting for the one about
// to start.
func Depth(ctx context.Context) int {
	d, _ := ctx.Value(depthKey{}).(int)
	return d
}

// CarryDepth makes ctx's render depth the same as from's. A subrequest does not
// inherit anything else a render's context holds, but it has to know how many
// renders are waiting on it.
func CarryDepth(ctx, from context.Context) context.Context {
	if d := Depth(from); d > 0 {
		return context.WithValue(ctx, depthKey{}, d)
	}
	return ctx
}

// Call is one remote-function call a render made.
type Call struct {
	ID      string
	Payload string
}

// Fetch answers one render-time `event.fetch` of the app's own routes. The
// bytes in are a FetchRequest, JSON-encoded by the bundle's own polyfill; the
// bytes out are a FetchAnswer, JSON-encoded, which the bundle turns into a
// Response or throws as a TypeError. It is the only I/O a render's fetch ever
// performs: an in-process call into Go, never a socket the engine opens.
type Fetch func(ctx context.Context, request []byte) ([]byte, error)

// FetchRequest is one render-time `event.fetch` call, on its way into Go.
// Kit resolves a relative fetch against the page's own URL before this ever
// leaves the engine (`runtime/server/fetch.js`, `normalize_fetch_input`), so
// URL always carries an absolute, same-origin URL.
type FetchRequest struct {
	Credentials string `json:"credentials,omitempty"`
	Mode        string `json:"mode,omitempty"`
	Method      string `json:"method"`
	URL         string `json:"url"`
	// Headers is the request's headers, one value per name — the shape the
	// bundle's own `Headers` polyfill stores them in.
	Headers map[string]string `json:"headers,omitempty"`
	// Body is the request body as text, or "" for none; BodyBase64 is the same
	// for a body that is bytes. A render's fetch sends whichever it has — the
	// bundle always sends the bytes, since a Request's body may be either.
	Body       string `json:"body,omitempty"`
	BodyBase64 string `json:"bodyBase64,omitempty"`
}

// FetchAnswer is what Go gave a render-time fetch: a response, or the message
// of the TypeError a real `fetch` throws when it cannot reach the target.
type FetchAnswer struct {
	Response *FetchResponse `json:"response,omitempty"`
	Error    string         `json:"error,omitempty"`
}

// FetchResponse is one answer to a render-time fetch.
type FetchResponse struct {
	Status     int    `json:"status"`
	StatusText string `json:"statusText"`
	// Headers is every response header in the order the standard's Headers
	// iterates them: a repeated name joined with ", ", except Set-Cookie, which
	// is one entry per value.
	Headers    []FetchHeader `json:"headers,omitempty"`
	BodyBase64 string        `json:"bodyBase64,omitempty"`
	Body       string        `json:"body,omitempty"`
}

// FetchHeader is one response header and whether the app's
// FilterSerializedResponseHeaders lets a universal load's hydration data carry
// it. Go decides, so the filter stays an ordinary Go function.
type FetchHeader struct {
	Name, Value string
	Serialized  bool
}

// MarshalJSON writes the header as the [name, value, serialized] triple the
// bundle reads.
func (h FetchHeader) MarshalJSON() ([]byte, error) {
	return json.Marshal([]any{h.Name, h.Value, h.Serialized})
}

// Match answers one render-time `$app/paths` `match(pathname)` call. pathname
// is already decoded and stripped of the app's base, exactly as kit's own
// `match` prepares it before it asks the manifest (`runtime/app/paths/
// server.js`). Go answers from the same route table every page, load and
// endpoint request matches against, so a render's match is a call back into
// Go rather than a second router built for the engine — nil params means the
// route takes none, and ok false is kit's own answer for a pathname nothing
// matched: null.
type Match func(pathname string) (routeID string, params map[string]string, ok bool)

// Hosts is what Go answers a render's calls back out to it with. Each is
// optional; leaving one nil means a render that reaches it gets a well-defined
// refusal rather than a ReferenceError.
type Hosts struct {
	// HandleError applies Go application policy to a component-boundary error.
	HandleError func(context.Context, []byte) ([]byte, error)
	// ClientAddress answers getClientAddress lazily from the hosting boundary.
	ClientAddress func() (string, error)
	// Remote answers a remote-function call.
	Remote Host
	// Fetch answers a same-origin `event.fetch`.
	Fetch Fetch
	// Match answers a `$app/paths` `match()` lookup.
	Match Match
	// Tracing serves in-process span proxies; it never performs JavaScript I/O.
	Tracing      func(context.Context, []byte) ([]byte, error)
	TraceContext func(context.Context, string) context.Context
}

// Console is told what the engine wrote to `console`. level is the method that
// was called — "log", "warn", "error" and the rest — routeID is the route being
// rendered, or "" for something written while the bundle was being evaluated,
// and text is the arguments joined the way a browser's console would show them.
//
// It exists because the engine is a bare ECMAScript runtime: kit and Svelte
// both report through `console`, and with no such global those reports were a
// ReferenceError thrown in the middle of a render rather than a line an
// operator could read.
type Console func(routeID, level, text string)
