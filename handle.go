package skgo

import (
	"context"
	"encoding/json"
	"errors"
	"html"
	"io"
	"net/http"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// Handle is the simple, before-only form of kit's `handle` hook
// (`hooks.server.js`, whose contract lives in `runtime/server/respond.js`): it
// runs once per request before anything answers it, and the request carries on
// to the application when it returns nil.
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
// The event it reads is kit's hook event: the normalized URL, the matched
// route and its parameters, and the request-kind flags, all resolved before the
// hook runs. It may write cookies and response headers, which ride out on
// whatever answers, because kit's `event.cookies` and `event.setHeaders` are
// available to `handle` until the response exists. A hook that has to look at
// the response — or answer without the application at all — is a [Middleware].
type Handle func(ctx context.Context) error

// Middleware is skgo's mirror of kit's `handle` hook in full: it receives the
// request's event and a resolve function, and returns the response.
//
//	func handle(ctx context.Context, e *skgo.Event, resolve skgo.Resolve) (*http.Response, error) {
//		// before: the event is resolved, locals and cookies are writable
//		res, err := resolve(ctx) // the application's real response
//		// after: wrap the response's headers or body
//		return res, err
//	}
//
// Not calling resolve answers the request without the application: return a
// response of your own (see [NewResponse]), or an error. A returned *Redirect
// or *HTTPError is kit's thrown `redirect()` / `error()`, shaped for the kind
// of request it was; a response the middleware builds is sent exactly as it is.
// The pin does not add the event's cookies or headers to a response the hook
// builds itself, so neither does this: only a resolved response carries them.
//
// Compose several with [Sequence].
type Middleware func(ctx context.Context, event *Event, resolve Resolve) (*http.Response, error)

// Resolve is kit's `resolve(event, opts)`: it runs the application for the
// request and returns its response. The response is streamed — headers arrive
// when the application commits them and the body as it writes it, so wrapping it
// never waits for the whole body and closing it cancels the application. Resolve
// may be called once per request, with at most one [ResolveOptions].
//
// Pass the ctx the middleware was given, or one derived from it, so the
// application sees the same event and locals.
type Resolve func(ctx context.Context, options ...ResolveOptions) (*http.Response, error)

// ResolveOptions is kit's `ResolveOptions` (`exports/hooks/public.d.ts`): how
// this one request's page document is rendered. Every field is optional, and an
// option is request-local — it never changes the renderer other requests share.
// Under [Sequence] the options of nested middleware compose the way kit's
// `sequence` composes them.
type ResolveOptions struct {
	// TransformPageChunk rewrites the assembled document exactly once, with
	// done true, before a finite document's ETag is computed. Values a page
	// streams afterwards are appended as they are and never transformed. It
	// applies to rendered pages, their error pages and the `ssr = false` shell,
	// and to nothing else: data, endpoints and remote responses are untouched.
	// Chunks are not required to stay well-formed — but removing comments can
	// break Svelte's hydration. The default is the identity.
	TransformPageChunk func(ctx context.Context, html string, done bool) (string, error)
	// FilterSerializedResponseHeaders decides which headers of a universal
	// load's `fetch` response the hydration data carries. Without it the
	// renderer's own [SSROptions.FilterSerializedResponseHeaders] applies, and
	// without that none are carried.
	FilterSerializedResponseHeaders func(name, value string) bool
	// Preload decides which files a document preloads. Without it JavaScript and
	// CSS are preloaded and fonts are not. Stylesheets are linked whatever it
	// answers.
	Preload func(input PreloadInput) bool
}

// PreloadInput names a file [ResolveOptions.Preload] is asked about. Type is
// "js", "css", "asset" or "font"; Filename is only set for a font, and is the
// source file's pathname relative to the project root.
type PreloadInput struct {
	Type     string
	Path     string
	Filename string
}

func defaultPreload(input PreloadInput) bool { return input.Type == "js" || input.Type == "css" }

// composeResolveOptions is the body of kit's `sequence` `resolve`: the inner
// transform runs before the outer one, and the first defined outer callback is
// the one that answers filtering and preloading, an answer of false included.
func composeResolveOptions(own, parent *ResolveOptions) *ResolveOptions {
	switch {
	case own == nil:
		return parent
	case parent == nil:
		return own
	}
	out := ResolveOptions{
		TransformPageChunk:              own.TransformPageChunk,
		FilterSerializedResponseHeaders: own.FilterSerializedResponseHeaders,
		Preload:                         own.Preload,
	}
	if inner, outer := own.TransformPageChunk, parent.TransformPageChunk; inner != nil && outer != nil {
		out.TransformPageChunk = func(ctx context.Context, html string, done bool) (string, error) {
			html, err := inner(ctx, html, done)
			if err != nil {
				return "", err
			}
			return outer(ctx, html, done)
		}
	} else if outer != nil {
		out.TransformPageChunk = outer
	}
	if parent.FilterSerializedResponseHeaders != nil {
		out.FilterSerializedResponseHeaders = parent.FilterSerializedResponseHeaders
	}
	if parent.Preload != nil {
		out.Preload = parent.Preload
	}
	return &out
}

func singleResolveOptions(options []ResolveOptions) (*ResolveOptions, error) {
	switch len(options) {
	case 0:
		return nil, nil
	case 1:
		return &options[0], nil
	}
	return nil, errors.New("skgo: resolve takes at most one ResolveOptions")
}

// Middleware is the hook as a Middleware that runs h before the application.
func (h Handle) Middleware() Middleware {
	if h == nil {
		return nil
	}
	return func(ctx context.Context, _ *Event, resolve Resolve) (*http.Response, error) {
		if err := h(ctx); err != nil {
			return nil, err
		}
		return resolve(ctx)
	}
}

// Sequence composes middleware the way kit's `sequence` does: the first runs
// outermost, each one's resolve runs the next, and the last one's resolve runs
// the application. Their after-logic unwinds in reverse. The [ResolveOptions]
// they pass compose as in kit: inner transforms run before outer ones, and the
// first defined outer filter or preload callback wins, so an inner one is never
// called once an outer one exists.
func Sequence(middleware ...Middleware) Middleware {
	var chain []Middleware
	for _, m := range middleware {
		if m != nil {
			chain = append(chain, m)
		}
	}
	if len(chain) == 0 {
		return func(ctx context.Context, _ *Event, resolve Resolve) (*http.Response, error) {
			return resolve(ctx)
		}
	}
	return func(ctx context.Context, event *Event, resolve Resolve) (*http.Response, error) {
		var apply func(i int, ctx context.Context, parent *ResolveOptions) (*http.Response, error)
		apply = func(i int, ctx context.Context, parent *ResolveOptions) (*http.Response, error) {
			return chain[i](ctx, event, func(ctx context.Context, options ...ResolveOptions) (*http.Response, error) {
				own, err := singleResolveOptions(options)
				if err != nil {
					return nil, err
				}
				composed := composeResolveOptions(own, parent)
				if i < len(chain)-1 {
					return apply(i+1, ctx, composed)
				}
				if composed == nil {
					return resolve(ctx)
				}
				return resolve(ctx, *composed)
			})
		}
		return apply(0, ctx, nil)
	}
}

// NewResponse builds a response a middleware answers with. A nil header or body
// is empty.
func NewResponse(status int, header http.Header, body io.Reader) *http.Response {
	if header == nil {
		header = http.Header{}
	}
	rc, ok := body.(io.ReadCloser)
	switch {
	case body == nil:
		rc = http.NoBody
	case !ok:
		rc = io.NopCloser(body)
	}
	return &http.Response{StatusCode: status, Status: http.StatusText(status), Header: header, Body: rc, ContentLength: -1}
}

// HandleConfig is what Intercept needs to recognise the requests kit's own
// `handle` never sees and to resolve what kit resolves before it runs. AppDir
// and Base come straight from the built manifest; an app has nothing else to
// decide.
type HandleConfig struct {
	// AppDir is kit's appDir; empty means "_app".
	AppDir string
	// Base is kit's paths.base, without a trailing slash.
	Base string
	// Origin is the app's fixed origin. It makes the event's URL absolute and
	// decides the `Secure` default of the cookies the hook writes, as in kit.
	Origin string
	// Dev reports that the app runs under `vite dev`, where cookies are not
	// Secure by default.
	Dev bool
	// Version, when non-empty, is sent as the `x-sveltekit-version` response
	// header on a request the hook refuses — the same header a successful
	// data or remote response carries, so a client watching for a new
	// deployment sees it either way.
	Version string
	// Loads, when set, is the route table the hook matches against. It is the
	// one `__data.json` already uses, and in dev it follows the live routes;
	// without it the hook matches the manifest routes HandleConfig was made
	// from.
	Loads *Loads
	// Static, when set, reports that a request is answered before Kit's server
	// runs — a file out of `static/`, a prerendered page, a module vite serves
	// in dev. Kit's adapters serve those ahead of `handle`, so the hook never
	// sees them.
	Static func(r *http.Request) bool
	// HandleError shapes errors raised by the handle hook as Kit's App.Error.
	HandleError HandleError
	// ErrorTemplate is Kit's error.html, used for a fatal native hook error.
	ErrorTemplate string
	routes        []handleRoute
	// OnPanic is called when the hook panics, with "handle", the recovered
	// value, and the stack. The client is told nothing but an opaque 500, so
	// this is the only record the panic leaves; leaving it nil logs the same
	// three things to the standard logger, because a panicking hook that
	// reports nowhere is a bug that cannot be found.
	OnPanic func(id string, value any, stack []byte)
}

type handleRoute struct {
	id      string
	pattern *regexp.Regexp
	params  []ManifestParam
	hasPage bool
}

// HandleConfig derives a Handle's configuration from a build manifest.
func (m Manifest) HandleConfig() HandleConfig {
	cfg := HandleConfig{AppDir: m.AppDir, Base: m.Base, Version: m.Version}
	for _, route := range m.Routes {
		if re, err := regexp.Compile(kitPattern(route.Pattern)); err == nil {
			cfg.routes = append(cfg.routes, handleRoute{id: route.ID, pattern: re, params: route.Params, hasPage: route.Page != nil})
		}
	}
	return cfg
}

// matchRoute is kit's `find_route` over the base-stripped pathname.
func (cfg HandleConfig) matchRoute(routePath string) (handleRoute, map[string]string, bool) {
	if cfg.Loads != nil {
		route, params, ok := cfg.Loads.match(routePath)
		if !ok {
			return handleRoute{}, nil, false
		}
		return handleRoute{id: route.id, hasPage: route.hasPage}, params, true
	}
	for _, route := range cfg.routes {
		loc := route.pattern.FindStringSubmatchIndex(routePath)
		if loc == nil {
			continue
		}
		if params, ok := execParams(routePath, loc, route.params); ok {
			return route, params, true
		}
	}
	return handleRoute{}, nil, false
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
	return h.Middleware().Intercept(cfg, next)
}

// refuse writes the hook's refusal in the shape the request expects. Kit sends
// JSON for data and remote requests and enhanced page action redirects, a real
// redirect for native navigation, and App.Error JSON or fatal HTML for errors
// according to Accept.
func (cfg HandleConfig) refuse(w http.ResponseWriter, r *http.Request, ctx context.Context, jar *cookieJar, routeID string, err error, isData, isRemote, isPage bool) {
	h := w.Header()
	h.Set("Cache-Control", "private, no-store")
	if cfg.Version != "" {
		h.Set("X-Sveltekit-Version", cfg.Version)
	}

	redirect := asRedirect(err)
	isActionJSON := isPage && isActionJSON(r)

	// Kit adds the cookies the request wrote to a thrown redirect and to
	// nothing else it throws.
	if redirect != nil && jar != nil {
		jar.writeMissingTo(h)
	}

	if isData || isRemote || (redirect != nil && isActionJSON) {
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
	e := handleErrorAndJSONify(ctx, routeID, cfg.HandleError, asHTTPError(err), err, nil)
	if isActionJSON || strings.Contains(r.Header.Get("Accept"), "application/json") {
		h.Set("Content-Type", "application/json")
		w.WriteHeader(e.Status)
		_ = json.NewEncoder(w).Encode(e)
		return
	}
	if cfg.ErrorTemplate != "" {
		page := strings.ReplaceAll(cfg.ErrorTemplate, "%sveltekit.status%", strconv.Itoa(e.Status))
		page = strings.ReplaceAll(page, "%sveltekit.error.message%", html.EscapeString(e.Message))
		h.Set("Content-Type", "text/html; charset=utf-8")
		h.Set("Content-Length", strconv.Itoa(len(page)))
		w.WriteHeader(e.Status)
		if r.Method != http.MethodHead {
			_, _ = w.Write([]byte(page))
		}
		return
	}
	h.Set("Content-Type", "application/json")
	w.WriteHeader(e.Status)
	_ = json.NewEncoder(w).Encode(e)
}
