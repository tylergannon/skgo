package skgo

import (
	"context"
	"log"
	"net/http"
	"reflect"
	"runtime/debug"
	"strings"
	"sync"
)

// Handle is skgo's mirror of kit's `handle` hook (`hooks.server.js`), whose
// contract lives in `runtime/server/respond.js`: the one place an app decides
// what a request is allowed to do, before anything answers it.
//
// Kit runs it once per request, for every kind of request its own server
// answers — a page document, a `__data.json` request, a remote-function call,
// or a `+server.ts` route — before it dispatches to any of them. That is
// kit's own top-level dispatcher, not a load concern, so skgo mirrors it the
// same way: Intercept mounts outermost, over whatever the app has. An app
// with nothing but remote functions mounts it over Remotes alone; one with
// pages too adds Loads and Endpoints underneath. Either way the hook itself
// looks the same:
//
//	func handle(ctx context.Context) error {
//		user := users.FromCookie(skgo.EventFrom(ctx))
//		return skgo.SetLocal(ctx, user)
//	}
//
// Returning a *Redirect or an *HTTPError refuses the request there and then,
// and skgo shapes the refusal for the kind of request it was: a data request
// or a remote call gets the JSON node kit's client expects, a page request a
// real HTTP redirect.
//
// Its event may read cookies and may not write them. A cookie a navigation
// writes belongs in a load, and one a mutation writes belongs in a command;
// both are places kit's own caching rules already say a write is safe. Its
// event may not read the page's URL, route id or parameters either — kit
// resolves those from the route table before running the hook, and skgo's
// route table is Loads' own, built from the server loads a page app
// registers; an app that mounts Handle without a Loads registry has no route
// table to resolve against, so the restriction is the same everywhere rather
// than a surprise that shows up only sometimes.
type Handle func(ctx context.Context) error

// HandleConfig is what Intercept needs to recognise the requests kit's own
// `handle` never sees. AppDir and Base come straight from the built manifest;
// an app has nothing else to decide.
type HandleConfig struct {
	// AppDir is kit's appDir; empty means "_app".
	AppDir string
	// Base is kit's paths.base, without a trailing slash.
	Base string
	// Version, when non-empty, is sent as the `x-sveltekit-version` response
	// header on a request the hook refuses — the same header a successful
	// data or remote response carries, so a client watching for a new
	// deployment sees it either way.
	Version string
	// OnPanic is called when the hook panics, with "handle", the recovered
	// value, and the stack. The client is told nothing but an opaque 500, so
	// this is the only record the panic leaves; leaving it nil logs the same
	// three things to the standard logger, because a panicking hook that
	// reports nowhere is a bug that cannot be found.
	OnPanic func(id string, value any, stack []byte)
}

// HandleConfig derives a Handle's configuration from a build manifest.
func (m Manifest) HandleConfig() HandleConfig {
	return HandleConfig{AppDir: m.AppDir, Base: m.Base, Version: m.Version}
}

type localsKey struct{}

// locals is the per-request scratch space kit calls `event.locals`, keyed by
// the type of what is stored so a reader gets back what a writer put in
// without a name to agree on.
type locals struct {
	mu     sync.Mutex
	values map[reflect.Type]any
}

// SetLocal stores v on the request, keyed by its type. Call it from the app's
// Handle hook; every load, remote function and server route serving the same
// request can then read it with LocalOf.
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

// Intercept wraps next so that h runs once before it, on every request kind
// this app answers, mirroring where kit runs `hooks.handle`
// (`runtime/server/respond.js`): before dispatch, no matter what next is
// made of. Build cfg directly from the app's manifest — an app that answers
// only remote functions has no reason to also build a Loads registry merely
// to reach this hook.
//
// A nil Handle is a plain pass-through, so an app with no hook at all has no
// reason to call Intercept either.
func (h Handle) Intercept(cfg HandleConfig, next http.Handler) http.Handler {
	if h == nil {
		return next
	}

	appDir := cfg.AppDir
	if appDir == "" {
		appDir = "_app"
	}
	base := strings.TrimSuffix(cfg.Base, "/")
	if base != "" && !strings.HasPrefix(base, "/") {
		base = "/" + base
	}
	assetPrefix := base + "/" + appDir + "/"
	remotePrefix := assetPrefix + "remote/"

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Kit's `handle` never sees a static asset: its adapters answer those
		// before the server does. skgo's equivalent is the app directory,
		// which holds nothing but the immutable bundle — and the remote
		// calls, which are requests the app answers and so must still reach
		// the hook.
		if strings.HasPrefix(path, assetPrefix) && !strings.HasPrefix(path, remotePrefix) {
			next.ServeHTTP(w, r)
			return
		}

		isData := hasDataSuffix(path)
		isRemote := strings.HasPrefix(path, remotePrefix)

		e := &Event{req: r, jar: newCookieJar(r, false)}
		ctx := withEvent(r.Context(), e)
		ctx = context.WithValue(ctx, localsKey{}, &locals{values: map[reflect.Type]any{}})

		if err := h.runGuarded(ctx, cfg); err != nil {
			cfg.refuse(w, r, err, isData, isRemote)
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// runGuarded runs h, turning a panic into the error every refusal path
// already knows how to answer.
//
// A hook is ordinary Go and one of them will panic. Kit answers an unexpected
// throw from `handle` with the same opaque 500 it gives any unexpected error,
// and so does this.
func (h Handle) runGuarded(ctx context.Context, cfg HandleConfig) (err error) {
	defer func() {
		value := recover()
		if value == nil {
			return
		}
		// net/http panics with ErrAbortHandler to abandon a response on
		// purpose. Swallowing it would turn a deliberate abort into a 500 the
		// client reads as a real answer, so it goes back up untouched.
		if value == http.ErrAbortHandler {
			panic(value)
		}
		stack := debug.Stack()
		if cfg.OnPanic != nil {
			cfg.OnPanic("handle", value, stack)
		} else {
			log.Printf("skgo: handle hook panicked: %v\n%s", value, stack)
		}
		err = &HTTPError{Status: 500, Message: "Internal Error"}
	}()
	return h(ctx)
}

// refuse writes the hook's refusal in the shape the request it refused
// expects. The three shapes are kit's own: a data request or a remote call is
// answered at HTTP 200 with a redirect or error envelope, and a page request
// with a real HTTP redirect or a plain error.
func (cfg HandleConfig) refuse(w http.ResponseWriter, r *http.Request, err error, isData, isRemote bool) {
	h := w.Header()
	h.Set("Cache-Control", "private, no-store")
	if cfg.Version != "" {
		h.Set("X-Sveltekit-Version", cfg.Version)
	}

	redirect := asRedirect(err)

	if isData || isRemote {
		h.Set("Content-Type", "application/json")
		if redirect != nil {
			w.WriteHeader(http.StatusOK)
			writeJSON(w, redirectEnvelope{Type: "redirect", Status: redirect.status(), Location: redirect.Location})
			return
		}
		e := asHTTPError(err)
		if isRemote {
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
	h.Set("Content-Type", "application/json")
	w.WriteHeader(e.Status)
	writeJSON(w, e)
}
