package skgo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// loadRequest is the part of a load's event that every node of one branch
// shares: the request, the cookie jar, the response headers, and the page the
// visitor asked for.
type loadRequest struct {
	req     *http.Request
	jar     *cookieJar
	url     *url.URL
	routeID string
	params  map[string]string

	mu      sync.Mutex
	headers http.Header
	sealed  bool
}

// event derives the per-node event. Each node gets its own `uses` record,
// because the client decides per node whether the load has to run again.
func (lr *loadRequest) event(index int, fns []func() dataNode) *Event {
	return &Event{
		req: lr.req,
		jar: lr.jar,
		// A load may write cookies. Kit allows it there and forbids it in a
		// query, because a query's result is cached by its argument and
		// replayed; a load's result is not.
		mutable: true,
		load: &loadState{
			shared: lr,
			index:  index,
			fns:    fns,
			uses:   &uses{tracking: true},
		},
	}
}

// applyTo writes the cookies and the headers the loads asked for onto the
// response, and stops accepting more. Kit does the same once the response
// exists.
func (lr *loadRequest) applyTo(h http.Header) {
	lr.mu.Lock()
	lr.sealed = true
	headers := lr.headers
	lr.mu.Unlock()

	for name, values := range headers {
		for _, v := range values {
			h.Set(name, v)
		}
	}
	lr.jar.writeTo(h)
}

// loadState is what an Event carries only inside a server load.
type loadState struct {
	shared *loadRequest
	index  int
	fns    []func() dataNode
	uses   *uses
}

// URL is the page the visitor asked for — not the `__data.json` address the
// browser used, and with kit's two private query parameters already removed.
//
// Reading it makes this load depend on the whole URL, so kit's client re-runs
// it on any navigation that changes the path or the query. When only one
// parameter matters, SearchParam is cheaper: it records only that parameter,
// exactly as kit's own `url.searchParams.get` does.
func (e *Event) URL() *url.URL {
	if e == nil || e.load == nil {
		return nil
	}
	e.load.uses.flag(&e.load.uses.url)
	copied := *e.load.shared.url
	return &copied
}

// Param returns a route parameter, and records that this load depends on it.
func (e *Event) Param(name string) string {
	if e == nil || e.load == nil {
		return ""
	}
	e.load.uses.add(&e.load.uses.params, name)
	return e.load.shared.params[name]
}

// SearchParam returns a query parameter, and records that this load depends on
// that one parameter and no other.
func (e *Event) SearchParam(name string) (string, bool) {
	if e == nil || e.load == nil {
		return "", false
	}
	e.load.uses.add(&e.load.uses.searchParams, name)
	values, ok := e.load.shared.url.Query()[name]
	if !ok || len(values) == 0 {
		return "", false
	}
	return values[0], true
}

// RouteID is the id of the route being served, e.g. "/account/orders", and
// records that this load depends on which route it is.
func (e *Event) RouteID() string {
	if e == nil || e.load == nil {
		return ""
	}
	e.load.uses.flag(&e.load.uses.route)
	return e.load.shared.routeID
}

// Depends declares that this load's result goes stale when any of these
// identifiers is invalidated from the browser with `invalidate(...)`.
//
// An identifier is a URL: a bare path is resolved against the page's own
// address, and a custom scheme such as "app:account" travels as written. It is
// the string the client matches on, so it has to be the same string on both
// sides.
//
// Unlike everything else, a dependency is recorded even inside Untrack: kit
// turns off the implicit tracking there, never the explicit call.
func (e *Event) Depends(deps ...string) {
	if e == nil || e.load == nil {
		return
	}
	for _, dep := range deps {
		href := dep
		if resolved, err := e.load.shared.url.Parse(dep); err == nil {
			href = resolved.String()
		}
		e.load.uses.depend(href)
	}
}

// Untrack runs fn without recording what it reads, so a load can consult the
// URL or a parameter without making its result depend on it.
func (e *Event) Untrack(fn func()) {
	if e == nil || e.load == nil {
		fn()
		return
	}
	u := e.load.uses
	u.mu.Lock()
	was := u.tracking
	u.tracking = false
	u.mu.Unlock()
	defer func() {
		u.mu.Lock()
		u.tracking = was
		u.mu.Unlock()
	}()
	fn()
}

// SetHeader puts a header on the response the whole navigation shares. It is
// kit's `setHeaders`, and it enforces kit's rules: cookies go through
// SetCookie, and a header may not be set twice — except `server-timing`, which
// accumulates.
func (e *Event) SetHeader(name, value string) error {
	if e == nil || e.load == nil {
		return Errorf(500, "skgo: response headers can only be set from a server load")
	}
	if e.endpoint {
		return Errorf(500, "skgo: a server route writes its own response — set headers on the http.ResponseWriter")
	}
	lower := strings.ToLower(name)
	if lower == "set-cookie" {
		return Errorf(500, "skgo: use SetCookie to set cookies, not SetHeader")
	}

	lr := e.load.shared
	lr.mu.Lock()
	defer lr.mu.Unlock()
	if lr.sealed {
		return Errorf(500, "skgo: headers can no longer be set; the response has already been generated")
	}
	if existing := lr.headers.Get(lower); existing != "" {
		if lower != "server-timing" {
			return Errorf(500, "skgo: the %q header is already set", lower)
		}
		lr.headers.Set(lower, existing+", "+value)
		return nil
	}
	lr.headers.Set(lower, value)
	return nil
}

// Parent returns what this route's outer loads produced, decoded into T.
//
// It is kit's `await parent()`: a shallow merge of every preceding node's data,
// outermost first, so a deeper layout's field wins. Calling it records that
// this load depends on its parents, which is what makes kit's client re-run it
// when one of them re-runs.
//
// A load whose parent failed fails with the same error, and one whose parent
// redirected redirects — the visitor never reaches a page whose layout turned
// them away.
func Parent[T any](ctx context.Context) (T, error) {
	var out T
	e := EventFrom(ctx)
	// fns is nil on the events skgo builds for the `handle` hook and for a
	// server route: neither sits in a branch, so neither has a parent. Without
	// this they would both read as a load whose branch happens to be empty and
	// get a zero value back.
	if e == nil || e.load == nil || e.load.fns == nil {
		return out, Errorf(500, "skgo: Parent can only be called from a server load")
	}

	state := e.load
	state.uses.flag(&state.uses.parent)

	merged := map[string]any{}
	for j := 0; j < state.index; j++ {
		n := state.fns[j]()
		if n.redir != nil {
			return out, n.redir
		}
		if n.err != nil {
			return out, n.err
		}
		// A load returns its raw Go value now — it is not encoded until the
		// serializer, which is the only place the transport hook is known — so
		// the merge has to flatten it here. encodeValue rather than the
		// transport walk on purpose: this data is not going to the browser, it
		// is going into T through encoding/json, which understands a custom
		// type perfectly well without any transport.
		tree, err := encodeValue(n.data)
		if err != nil {
			return out, Errorf(500, "skgo: encoding parent data: %v", err)
		}
		fields, ok := tree.(map[string]any)
		if !ok {
			continue
		}
		for k, v := range fields {
			merged[k] = v
		}
	}

	raw, err := json.Marshal(merged)
	if err != nil {
		return out, Errorf(500, "skgo: encoding parent data: %v", err)
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, Errorf(500, "skgo: the parent's data does not fit %T: %v", out, err)
	}
	return out, nil
}
