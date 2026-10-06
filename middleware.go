package skgo

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"reflect"
	"runtime/debug"
	"strconv"
	"strings"
)

type hookStateKey struct{}

// hookState is what the hook layer knows about one request and shares with the
// registries underneath it: the cookie jar and the response headers are kit's
// single `event.cookies` and `headers` object, which the hook, every load, an
// action and a remote function all write to and the response carries once.
type hookState struct {
	url       *url.URL
	routeID   string
	params    map[string]string
	isData    bool
	isRemote  bool
	isSub     bool
	jar       *cookieJar
	response  *loadRequest
	resolving bool
	// options is what the hook passed to resolve, which the page renderer
	// beneath reads for this request alone.
	options *ResolveOptions
}

func hookStateOf(ctx context.Context) *hookState {
	s, _ := ctx.Value(hookStateKey{}).(*hookState)
	return s
}

// requestResolveOptions is what the handle hook chose for this request's
// document, or nil when it chose nothing.
func requestResolveOptions(ctx context.Context) *ResolveOptions {
	if s := hookStateOf(ctx); s != nil {
		return s.options
	}
	return nil
}

// sealed is kit's `finally` in `resolve`: once the response exists, the event
// stops accepting cookies and headers.
func (s *hookState) sealed() bool {
	s.response.mu.Lock()
	defer s.response.mu.Unlock()
	return s.response.sealed
}

func (s *hookState) seal() {
	s.response.mu.Lock()
	s.response.sealed = true
	s.response.mu.Unlock()
}

// requestCookieJar is the one cookie jar of a request: the hook's when a hook
// is mounted, so a cookie it refreshed is the one a load, an action, a server
// route or a remote function reads — and the one the visitor finally gets.
func requestCookieJar(r *http.Request, secureDefault bool) *cookieJar {
	if s := hookStateOf(r.Context()); s != nil {
		return s.jar
	}
	return newCookieJar(r, secureDefault)
}

// requestResponseHeaders is the header set every layer of one request shares,
// so a header the hook already set is "already set" to a load, as in kit.
func requestResponseHeaders(r *http.Request) http.Header {
	if s := hookStateOf(r.Context()); s != nil {
		return s.response.headers
	}
	return http.Header{}
}

// IsDataRequest reports that this is a `__data.json` request. It is only
// meaningful on the event a Middleware receives.
func (e *Event) IsDataRequest() bool { return e != nil && e.hook != nil && e.hook.isData }

// IsRemoteRequest reports that this is a remote-function call. It is only
// meaningful on the event a Middleware receives.
func (e *Event) IsRemoteRequest() bool { return e != nil && e.hook != nil && e.hook.isRemote }

// IsSubRequest reports that this request came from an in-process Fetch of the
// app's own routes rather than from a client.
func (e *Event) IsSubRequest() bool { return e != nil && e.hook != nil && e.hook.isSub }

// Params is a copy of the matched route's parameters, or nil when no route
// matched. It is only available on the event a Middleware receives; inside a
// query kit forbids reading it, and so does this.
func (e *Event) Params() map[string]string {
	if e != nil && e.remote {
		panic("skgo: raw Params is forbidden in remote functions; use the explicit event.Params getters")
	}
	if e == nil || e.hook == nil || e.hook.params == nil {
		return nil
	}
	out := make(map[string]string, len(e.hook.params))
	for k, v := range e.hook.params {
		out[k] = v
	}
	return out
}

// Intercept wraps next with the middleware, mirroring where kit runs
// `hooks.handle` (`runtime/server/respond.js`): after kit has normalized the
// URL and matched the route, before anything answers, for every request kind
// the server answers. A nil Middleware is a plain pass-through.
func (m Middleware) Intercept(cfg HandleConfig, next http.Handler) http.Handler {
	if m == nil {
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
	var origin *url.URL
	if u, err := url.Parse(cfg.Origin); err == nil && u.Host != "" {
		origin = u
	}
	secure := secureCookieDefault(cfg.Origin, cfg.Dev)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// A websocket upgrade is not a request kit's server answers.
		if isUpgrade(r) {
			next.ServeHTTP(w, r)
			return
		}
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
		if cfg.Static != nil && cfg.Static(r) {
			next.ServeHTTP(w, r)
			return
		}
		if rejectReservedQuery(w, r, isData, isRemote) {
			return
		}

		pageURL, skipRoute := cfg.eventURL(r, origin, base, isData, isRemote)

		// Kit answers a path outside the configured base before it matches a
		// route or runs the hook.
		pathname := pageURL.EscapedPath()
		if isRemote && !skipRoute {
			pathname = r.Header.Get("x-sveltekit-pathname")
		}
		routePath, err := decodePathname(pathname)
		if err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		if base != "" {
			if !strings.HasPrefix(routePath, base) {
				w.Header().Set("Content-Type", "text/plain;charset=UTF-8")
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte("Not found"))
				return
			}
			if routePath = routePath[len(base):]; routePath == "" {
				routePath = "/"
			}
		}

		state := &hookState{
			url:      pageURL,
			isData:   isData,
			isRemote: isRemote,
			isSub:    fetchDepthOf(r.Context()) > 0,
			response: &loadRequest{responseState: &responseState{headers: http.Header{}}},
		}
		hasPage := false
		if !skipRoute {
			// Handle runs before the registries, so it must validate the
			// live graph before its own match too. Let the owning handler
			// report refresh failures in the request kind's normal format.
			if cfg.Loads != nil && cfg.Loads.devRefresh != nil {
				if err := cfg.Loads.devRefresh(); err != nil {
					next.ServeHTTP(w, r)
					return
				}
			}
			if route, params, ok := cfg.matchRoute(routePath); ok {
				state.routeID, state.params, hasPage = route.id, params, route.hasPage
			}
		}
		state.jar = newCookieJarAt(r, pageURL.Host, pageURL.Path, secure)

		event := &Event{req: r, jar: state.jar, mutable: true, hook: state, actionResponse: state.response}
		ctx := withEvent(r.Context(), event)
		ctx = context.WithValue(ctx, hookStateKey{}, state)
		ctx = context.WithValue(ctx, localsKey{}, &locals{values: map[reflect.Type]any{}})

		var resolved *downstream
		defer func() {
			if resolved != nil {
				resolved.close()
			}
		}()
		resolve := func(rctx context.Context, options ...ResolveOptions) (*http.Response, error) {
			if state.resolving {
				return nil, errors.New("skgo: resolve was already called for this request")
			}
			own, err := singleResolveOptions(options)
			if err != nil {
				return nil, err
			}
			state.resolving = true
			state.options = own
			var response *http.Response
			base := ctx
			if hookStateOf(rctx) == state {
				base = rctx
			}
			resolved, response = startDownstream(rctx, base, next, r)
			// Kit adds the headers and cookies in the same place it stops
			// accepting them: when `resolve` has its response.
			state.response.mu.Lock()
			state.response.sealed = true
			for name, values := range state.response.headers {
				response.Header[http.CanonicalHeaderKey(name)] = append([]string(nil), values...)
			}
			state.response.mu.Unlock()
			state.jar.writeMissingTo(response.Header)
			return response, nil
		}

		response, err := m.runGuarded(ctx, cfg, event, resolve)
		if err == nil && response == nil {
			err = Errorf(http.StatusInternalServerError, "skgo: the handle hook returned no response")
		}
		if err != nil {
			cfg.refuse(w, r.WithContext(ctx), ctx, state.jar, state.routeID, err, isData, isRemote, hasPage)
			return
		}
		cfg.finish(w, r, response, state, isData)
	})
}

// eventURL is kit's pre-hook URL normalization: the `__data.json` suffix and
// its two private query parameters are removed from a data request, a remote
// call that names its page with `x-sveltekit-pathname` takes that page's URL,
// and a route-resolution suffix is stripped. skipRoute is true for a remote call
// that names no page: kit resolves no route for it.
func (cfg HandleConfig) eventURL(r *http.Request, origin *url.URL, base string, isData, isRemote bool) (u *url.URL, skipRoute bool) {
	pathname, rawQuery := r.URL.EscapedPath(), r.URL.RawQuery
	switch {
	case isData:
		pathname = stripDataSuffix(pathname)
		if queryHas(rawQuery, trailingSlashParam, "1") {
			pathname += "/"
		}
		if pathname == "" {
			pathname = "/"
		}
		rawQuery = dropQueryKeys(rawQuery, trailingSlashParam, invalidatedParam)
	case isRemote:
		if values, ok := r.Header[http.CanonicalHeaderKey("x-sveltekit-pathname")]; ok && len(values) > 0 {
			pathname = values[0]
			rawQuery = strings.TrimPrefix(r.Header.Get("x-sveltekit-search"), "?")
		} else {
			skipRoute = true
		}
	default:
		switch {
		case strings.HasSuffix(pathname, routeSuffix):
			pathname = strings.TrimSuffix(pathname, routeSuffix)
			if pathname == "" {
				pathname = "/"
			}
		case strings.HasSuffix(pathname, htmlRouteSuffix):
			pathname = strings.TrimSuffix(pathname, htmlRouteSuffix) + ".html"
		}
	}
	decoded, err := url.PathUnescape(pathname)
	if err != nil {
		decoded = pathname // The routing decoder refuses malformed caller headers before hooks.
	}
	u = &url.URL{Path: decoded, RawPath: pathname, RawQuery: rawQuery}
	if origin != nil {
		u.Scheme, u.Host = origin.Scheme, origin.Host
	} else {
		u.Scheme, u.Host = "http", r.Host
		if r.TLS != nil {
			u.Scheme = "https"
		}
	}
	return u, skipRoute
}

func queryHas(rawQuery, name, value string) bool {
	for _, pair := range strings.Split(rawQuery, "&") {
		key, v, _ := strings.Cut(pair, "=")
		if queryKey(key) == name && v == value {
			return true
		}
	}
	return false
}

func dropQueryKeys(rawQuery string, names ...string) string {
	var kept []string
	for _, pair := range strings.Split(rawQuery, "&") {
		if pair == "" {
			continue
		}
		key, _, _ := strings.Cut(pair, "=")
		drop := false
		for _, name := range names {
			if queryKey(key) == name {
				drop = true
			}
		}
		if !drop {
			kept = append(kept, pair)
		}
	}
	return strings.Join(kept, "&")
}

// runGuarded runs the middleware, turning a panic into the error every refusal
// path already knows how to answer.
//
// A hook is ordinary Go and one of them will panic. Kit answers an unexpected
// throw from `handle` with the same opaque 500 it gives any unexpected error,
// and so does this.
func (m Middleware) runGuarded(ctx context.Context, cfg HandleConfig, event *Event, resolve Resolve) (response *http.Response, err error) {
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
		if carried, ok := value.(*panicWithStack); ok {
			if carried.value == http.ErrAbortHandler {
				panic(carried.value)
			}
			value, stack = carried.value, carried.stack
		}
		if cfg.OnPanic != nil {
			cfg.OnPanic("handle", value, stack)
		} else {
			log.Printf("skgo: handle hook panicked: %v\n%s", value, stack)
		}
		response, err = nil, &HTTPError{Status: 500, Message: "Internal Error"}
	}()
	return m(ctx, event, resolve)
}

// finish is what kit does with the response `handle` returned, in the order it
// does it (`respond.js`): a 304 for a matching etag, and a redirect a hook
// returned for a data request turned into the JSON node kit's client follows.
// Then the response is sent, a chunk at a time.
func (cfg HandleConfig) finish(w http.ResponseWriter, r *http.Request, response *http.Response, state *hookState, isData bool) {
	if response.Header == nil {
		response.Header = http.Header{}
	}
	if response.Body == nil {
		response.Body = http.NoBody
	}
	status := response.StatusCode
	if status == 0 {
		status = http.StatusOK
	}

	if status == http.StatusOK && response.Header.Get("Etag") != "" {
		etag := response.Header.Get("Etag")
		match := strings.TrimPrefix(r.Header.Get("If-None-Match"), "W")
		if match == etag {
			_ = response.Body.Close()
			h := w.Header()
			h.Set("Etag", etag)
			for _, name := range []string{"Cache-Control", "Content-Location", "Date", "Expires", "Vary"} {
				if v := response.Header.Get(name); v != "" {
					h.Set(name, v)
				}
			}
			for _, cookie := range response.Header.Values("Set-Cookie") {
				h.Add("Set-Cookie", cookie)
			}
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}

	if isData && status >= 300 && status <= 308 {
		if location := response.Header.Get("Location"); location != "" {
			_ = response.Body.Close()
			h := w.Header()
			h.Set("Content-Type", "application/json")
			h.Set("Cache-Control", "private, no-store")
			if cfg.Version != "" {
				h.Set("X-Sveltekit-Version", cfg.Version)
			}
			w.WriteHeader(http.StatusOK)
			writeJSON(w, redirectEnvelope{Type: "redirect", Status: status, Location: location})
			return
		}
	}

	writeResponse(w, response, status)
}

// writeResponse sends a response without ever holding the whole body: each
// chunk is written as it is read, and flushed where the application flushed.
// A body that panicked mid-stream aborts the connection, as net/http does for
// a handler that panics there.
func writeResponse(w http.ResponseWriter, response *http.Response, status int) {
	body := response.Body
	defer body.Close()

	h := w.Header()
	for name, values := range response.Header {
		h[name] = append([]string(nil), values...)
	}
	if response.ContentLength > 0 && h.Get("Content-Length") == "" {
		h.Set("Content-Length", strconv.FormatInt(response.ContentLength, 10))
	}
	w.WriteHeader(status)

	rc := http.NewResponseController(w)
	flushes, _ := body.(*downstreamBody)
	buf := make([]byte, 32<<10)
	for {
		var (
			n     int
			flush bool
			err   error
		)
		if flushes != nil {
			n, flush, err = flushes.readFlush(buf)
		} else {
			// A middleware that replaced the body cannot say where the
			// application flushed, so every chunk is flushed: slower, never
			// later.
			n, err = body.Read(buf)
			flush = n > 0
		}
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
		}
		if flush {
			_ = rc.Flush()
		}
		if err != nil {
			if err != io.EOF && errors.Is(err, errDownstreamPanic) {
				panic(http.ErrAbortHandler)
			}
			return
		}
	}
}
