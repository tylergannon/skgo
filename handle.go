package skgo

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"sync"
)

// Handle is skgo's mirror of kit's `handle` hook: the one place an app decides
// what a request is allowed to do, before anything answers it.
//
// It runs on every request the app itself answers — a page document, a
// `__data.json`, a remote-function call — after the route is known and before
// any load or remote function runs. What it is for is deciding who the caller
// is, once, and putting the answer where every handler can read it:
//
//	func handle(ctx context.Context) error {
//		user := users.FromCookie(skgo.EventFrom(ctx))
//		return skgo.SetLocal(ctx, user)
//	}
//
// Returning a *Redirect or an *HTTPError refuses the request there and then,
// and skgo shapes the refusal for the kind of request it was: a data request
// gets the JSON node kit's client expects, a page request a real HTTP redirect.
//
// Its event may read cookies and may not write them. A cookie a navigation
// writes belongs in a load, and one a mutation writes belongs in a command;
// both are places kit's own caching rules already say a write is safe.
type Handle func(ctx context.Context) error

type localsKey struct{}

// locals is the per-request scratch space kit calls `event.locals`, keyed by
// the type of what is stored so a reader gets back what a writer put in without
// a name to agree on.
type locals struct {
	mu     sync.Mutex
	values map[reflect.Type]any
}

// SetLocal stores v on the request, keyed by its type. Call it from the app's
// Handle hook; every load and remote function serving the same request can then
// read it with LocalOf.
func SetLocal[T any](ctx context.Context, v T) error {
	l, _ := ctx.Value(localsKey{}).(*locals)
	if l == nil {
		return Errorf(500, "skgo: there is no request here to store a local on")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.values[reflect.TypeOf((*T)(nil)).Elem()] = v
	return nil
}

// LocalOf returns the value of type T that the Handle hook stored for this
// request, and whether it stored one.
func LocalOf[T any](ctx context.Context) (T, bool) {
	var zero T
	l, _ := ctx.Value(localsKey{}).(*locals)
	if l == nil {
		return zero, false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	v, ok := l.values[reflect.TypeOf((*T)(nil)).Elem()]
	if !ok {
		return zero, false
	}
	typed, ok := v.(T)
	return typed, ok
}

// runHandle builds the hook's event, runs it, and returns the request carrying
// whatever it decided.
func (ls *Loads) runHandle(r *http.Request, isData bool) (*http.Request, error) {
	req := dataRequest{url: r.URL, routePath: r.URL.Path}
	if isData {
		parsed, ok := ls.parseDataRequest(r)
		if !ok {
			return r, Errorf(404, "Not Found")
		}
		req = parsed
	} else {
		routePath := strings.TrimPrefix(r.URL.Path, ls.base)
		if routePath == "" {
			routePath = "/"
		}
		req.routePath = routePath
	}

	routeID, params := "", map[string]string{}
	if route, matched, ok := ls.match(req.routePath); ok {
		routeID, params = route.id, matched
	}

	shared := &loadRequest{
		req:     r,
		jar:     newCookieJar(r, secureCookieDefault(ls.cfg.Origin, ls.cfg.Dev)),
		headers: http.Header{},
		url:     req.url,
		routeID: routeID,
		params:  params,
	}
	// The hook reads; it does not write. Nothing it records is tracked either:
	// `uses` describes one load's dependencies, and the hook is not a load.
	e := shared.event(0, nil)
	e.mutable = false
	e.load.uses.tracking = false

	ctx := withEvent(r.Context(), e)
	ctx = context.WithValue(ctx, localsKey{}, &locals{values: map[reflect.Type]any{}})

	if err := ls.handle(ctx); err != nil {
		return r, err
	}
	return r.WithContext(ctx), nil
}

// refuse writes the hook's refusal in the shape the request it refused expects.
// The three shapes are kit's own: a data request is answered at HTTP 200 with a
// redirect or error node, a remote call with the envelope its client parses,
// and a page request with a real HTTP redirect.
func (ls *Loads) refuse(w http.ResponseWriter, r *http.Request, err error, isData bool) {
	redirect := asRedirect(err)
	remote := ls.cfg.AppDir != "" && strings.HasPrefix(r.URL.Path, ls.base+"/"+ls.cfg.AppDir+"/remote/")

	if isData || remote {
		if redirect != nil {
			ls.writeRedirect(w, nil, redirect)
			return
		}
		e := asHTTPError(err)
		h := ls.header(w)
		h.Set("Content-Type", "application/json")
		if remote {
			w.WriteHeader(http.StatusOK)
			writeJSON(w, remoteResponse{Type: "error", Error: e})
			return
		}
		w.WriteHeader(e.Status)
		writeJSON(w, e)
		return
	}

	if redirect != nil {
		http.Redirect(w, r, redirect.Location, redirect.status())
		return
	}
	e := asHTTPError(err)
	h := ls.header(w)
	h.Set("Content-Type", "application/json")
	w.WriteHeader(e.Status)
	writeJSON(w, e)
}
