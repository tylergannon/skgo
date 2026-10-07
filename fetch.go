package skgo

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/tylergannon/skgo/internal/ssr"
)

// Fetch dispatches a server-side fetch. A HandleFetch hook may change the
// request or return its own response without calling the dispatcher.
type Fetch func(request *http.Request) (*http.Response, error)

// HandleFetch mirrors Kit's server hook. It receives the normalized request
// before the dispatcher inherits the page's credentials. EventFrom(ctx) exposes
// the originating request and its locals. The supplied fetch dispatches directly
// rather than re-entering the hook, and retains the caller's cancellation.
// Configure it through SSROptions for render-time fetches and FetchConfig for
// Event.Fetch; it runs in Go, never in the browser.
type HandleFetch func(ctx context.Context, request *http.Request, fetch Fetch) (*http.Response, error)

// FetchCredentials is the `credentials` option of a Kit fetch.
type FetchCredentials string

const (
	// FetchSameOrigin is the default: the request inherits the page's cookies
	// and Authorization header when it targets this app.
	FetchSameOrigin FetchCredentials = "same-origin"
	// FetchInclude behaves like FetchSameOrigin on the server, as in Kit.
	FetchInclude FetchCredentials = "include"
	// FetchOmit inherits nothing from the originating request.
	FetchOmit FetchCredentials = "omit"
)

// FetchMode is the `mode` option of a Kit fetch. Only "no-cors" changes what
// the server sends: it drops Origin from a cross-origin GET or HEAD.
type FetchMode string

const (
	FetchCORS   FetchMode = "cors"
	FetchNoCORS FetchMode = "no-cors"
)

// FetchOption adjusts one Event.Fetch call.
type FetchOption func(*fetchOptions)

type fetchOptions struct {
	credentials FetchCredentials
	mode        FetchMode
}

// WithFetchCredentials sets the request's `credentials` mode.
func WithFetchCredentials(c FetchCredentials) FetchOption {
	return func(o *fetchOptions) { o.credentials = c }
}

// WithFetchMode sets the request's `mode`.
func WithFetchMode(m FetchMode) FetchOption {
	return func(o *fetchOptions) { o.mode = m }
}

// FetchConfig configures Event.Fetch, skgo's `event.fetch`: the request a load,
// action, server route, Handle hook or error hook makes while serving a
// request. The rules are Kit's (`runtime/server/fetch.js`).
type FetchConfig struct {
	// Origin is the app's configured origin, used to complete the URL of an
	// event that does not carry one. Empty falls back to the request's host.
	Origin string
	// Base is kit's paths.base. A same-origin URL outside it is not one of this
	// app's routes and goes over the network, as in Kit.
	Base string
	// Handler answers requests for this app's own routes in-process, with no
	// socket. Pass the handler the app serves with, from Handle inwards: every
	// page, `__data.json`, remote function, server route, asset and prerendered
	// file a fetch can name, as the real server would answer it. It sees every
	// subrequest with fresh locals and a fresh event. The one Intercept returns
	// is not it — that mounts this very fetcher — so assemble the handler first
	// and hand it over by reference. Nil refuses same-origin fetches.
	Handler http.Handler
	// HandleFetch intercepts every fetch before credential inheritance.
	HandleFetch HandleFetch
	// Client sends requests that leave the app. Nil uses http.DefaultClient.
	Client *http.Client
	// Prerendered is the build manifest's Prerendered list. A fetch of a path
	// on it takes nothing from the page's request, as in Kit, and nothing a
	// dynamic route might match is consulted in its place.
	Prerendered []string
}

// Intercept makes Event.Fetch available to everything beneath next. Mount it
// outermost, so Handle's own event can fetch as well.
func (c FetchConfig) Intercept(next http.Handler) http.Handler {
	f := &requestFetcher{handler: c.Handler, hook: c.HandleFetch, client: c.Client, base: normalizeFetchBase(c.Base), prerendered: prerenderedSet(c.Prerendered)}
	if origin, err := url.Parse(c.Origin); err == nil && origin.Host != "" {
		f.origin = origin
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestFetcherKey{}, f)))
	})
}

// Fetch is skgo's `event.fetch`. A relative URL resolves against the event's
// URL. A request for this app's own routes is answered in-process with fresh
// locals; the page's cookies, Authorization, Accept-Language and Origin are
// inherited as Kit inherits them, unless WithFetchCredentials(FetchOmit). Any
// other request goes out over HTTP, and carries cookies only to this host and
// its subdomains. The Set-Cookie headers of an in-process answer are taken as
// cookies this request wrote, so the next fetch sends them and the response to
// the visitor carries them, exactly as Kit's `event.fetch` does with them; an
// answer that came from HandleFetch or over HTTP is not harvested.
//
// The response is returned once the answering handler has committed its status
// and headers, and its body is read as the handler writes it, so a handler
// that streams is seen streaming. As with net/http, read the body to the end or
// Close it: Close, or cancelling ctx, ends the handler's context, and a read
// that outlives a cancelled handler fails with the context's error rather than
// ending as if the answer were complete.
//
// ctx and request's own context both cancel it. HandleFetch sees ctx's values,
// so EventFrom(ctx) and the locals are the originating request's. Fetching
// records no dependency: a server load that fetches is not re-run by
// `invalidate`, exactly as in Kit.
func (e *Event) Fetch(ctx context.Context, request *http.Request, opts ...FetchOption) (*http.Response, error) {
	if e == nil || e.req == nil {
		return nil, errors.New("skgo: Fetch needs the event of a request in flight")
	}
	f, _ := ctx.Value(requestFetcherKey{}).(*requestFetcher)
	if f == nil {
		f, _ = e.req.Context().Value(requestFetcherKey{}).(*requestFetcher)
	}
	if f == nil {
		return nil, errors.New("skgo: Event.Fetch is not configured; mount FetchConfig.Intercept")
	}
	source := fetchSource{url: e.fetchURL(f), request: e.req, jar: e.jar}
	if fetchDepthOf(ctx) == 0 {
		ctx = withFetchDepth(ctx, fetchDepthOf(e.req.Context()))
	}
	hookCtx := withEvent(ctx, e)
	if hookCtx.Value(localsKey{}) == nil {
		if l := e.req.Context().Value(localsKey{}); l != nil {
			hookCtx = context.WithValue(hookCtx, localsKey{}, l)
		}
	}
	o := fetchOptions{}
	for _, opt := range opts {
		opt(&o)
	}
	return f.fetch(hookCtx, source, request, o)
}

// fetchURL is event.url for fetch's purposes. It reads the load's URL without
// recording a dependency on it.
func (e *Event) fetchURL(f *requestFetcher) *url.URL {
	if e.hook != nil {
		u := *e.hook.url
		return &u
	}
	if e.load != nil && e.load.shared != nil && e.load.shared.url != nil {
		u := *e.load.shared.url
		return &u
	}
	u := *e.req.URL
	if u.Host == "" {
		if f.origin != nil {
			u.Scheme, u.Host = f.origin.Scheme, f.origin.Host
		} else {
			u.Host = e.req.Host
			u.Scheme = "http"
			if e.req.TLS != nil {
				u.Scheme = "https"
			}
		}
	}
	return &u
}

type requestFetcherKey struct{}

type fetchParentKey struct{}
type fetchParent struct {
	request *http.Request
	url     *url.URL
	jar     *cookieJar
	// filter is this render's FilterSerializedResponseHeaders: the page
	// request's own choice, not the renderer's.
	filter func(name, value string) bool
}

// fetchSource is the originating request a fetch inherits from: Kit's `event`.
type fetchSource struct {
	url     *url.URL
	request *http.Request
	jar     *cookieJar
}

// requestFetcher is Kit's create_fetch, shared by Event.Fetch and the
// render-time bridge so both apply one set of rules.
type requestFetcher struct {
	handler     http.Handler
	hook        HandleFetch
	client      *http.Client
	base        string
	origin      *url.URL
	prerendered map[string]bool
}

func prerenderedSet(paths []string) map[string]bool {
	set := make(map[string]bool, len(paths))
	for _, p := range paths {
		set[p] = true
	}
	return set
}

// hasPrerenderedPath is Kit's `has_prerendered_path`: pathname includes the
// base and is decoded.
func (f *requestFetcher) hasPrerenderedPath(pathname string) bool {
	return f.prerendered[pathname] || (strings.HasSuffix(pathname, "/") && f.prerendered[strings.TrimSuffix(pathname, "/")])
}

// maxFetchDepth is how deep subrequests may nest before a page refuses to
// render, as in Kit's `render_page`: past it the app is assumed to be fetching
// itself in a loop.
const maxFetchDepth = 10

type fetchDepthKey struct{}

// fetchDepthOf is how many subrequests deep ctx's request is: Kit's
// `state.depth`.
func fetchDepthOf(ctx context.Context) int {
	d, _ := ctx.Value(fetchDepthKey{}).(int)
	return d
}

func withFetchDepth(ctx context.Context, depth int) context.Context {
	return context.WithValue(ctx, fetchDepthKey{}, depth)
}

func normalizeFetchBase(base string) string {
	base = strings.TrimSuffix(base, "/")
	if base != "" && !strings.HasPrefix(base, "/") {
		base = "/" + base
	}
	return base
}

func (f *requestFetcher) fetch(ctx context.Context, src fetchSource, request *http.Request, o fetchOptions) (*http.Response, error) {
	if request == nil || request.URL == nil {
		return nil, errors.New("skgo: fetch needs a request with a URL")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// The request's own context cancels too, as Kit's request.signal does.
	merged, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(request.Context(), cancel)
	release := func() { stop(); cancel() }

	original := normalizeFetchRequest(src.url, request, merged)
	dispatch := func(r *http.Request) (*http.Response, error) {
		return f.dispatch(merged, src, normalizeFetchRequest(src.url, r, merged), o)
	}
	var (
		response *http.Response
		err      error
	)
	if f.hook != nil {
		response, err = f.hook(merged, original, dispatch)
	} else {
		response, err = dispatch(original)
	}
	if err == nil && response == nil {
		err = errors.New("skgo: handleFetch returned no response")
	}
	if err != nil {
		release()
		return nil, err
	}
	if response.Body == nil {
		response.Body = http.NoBody
	}
	response.Body = &releaseBody{ReadCloser: response.Body, release: release}
	return response, nil
}

// releaseBody ends the fetch's cancellation scope when the caller is done with
// the body, so a streaming external response is not cut off early.
type releaseBody struct {
	io.ReadCloser
	release func()
}

func (b *releaseBody) Close() error {
	err := b.ReadCloser.Close()
	b.release()
	return err
}

// normalizeFetchRequest is Kit's normalize_fetch_input: an independent request
// with an absolute URL, relative to the event's. Headers are copied so the
// dispatcher never edits the caller's.
func normalizeFetchRequest(base *url.URL, r *http.Request, ctx context.Context) *http.Request {
	out := r.Clone(ctx)
	resolved := base.ResolveReference(r.URL)
	resolved.Fragment = ""
	out.URL = resolved
	out.Host = resolved.Host
	out.RequestURI = ""
	if out.Header == nil {
		out.Header = http.Header{}
	}
	if out.Method == "" {
		out.Method = http.MethodGet
	}
	return out
}

func (f *requestFetcher) dispatch(ctx context.Context, src fetchSource, request *http.Request, o fetchOptions) (*http.Response, error) {
	u := request.URL
	if !hasFetchHeader(request.Header, "Origin") {
		request.Header.Set("Origin", src.url.Scheme+"://"+src.url.Host)
	}
	mode := o.mode
	if mode == "" {
		mode = FetchCORS
	}
	credentials := o.credentials
	if credentials == "" {
		credentials = FetchSameOrigin
	}
	sameOrigin := sameFetchOrigin(u, src.url)
	// https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Origin#description
	if (request.Method == http.MethodGet || request.Method == http.MethodHead) &&
		((mode == FetchNoCORS && !sameOrigin) || sameOrigin) {
		request.Header.Del("Origin")
	}

	decoded, err := url.PathUnescape(u.EscapedPath())
	if err != nil {
		return nil, fmt.Errorf("skgo: fetch %q: %w", u.String(), err)
	}

	if !sameOrigin || (f.base != "" && decoded != f.base && !strings.HasPrefix(decoded, f.base+"/")) {
		// Cookies go to this host and its subdomains, never to a sibling or a
		// parent, and ports do not matter. The leading dot stops mydomain.com
		// matching domain.com. Other cookies are not forwarded: a browser does
		// not say which cookie belongs to which domain.
		if credentials != FetchOmit && strings.HasSuffix("."+strings.ToLower(u.Hostname()), "."+strings.ToLower(src.url.Hostname())) {
			if cookie := src.jar.requestHeader(src.request, u, request.Header.Get("Cookie")); cookie != "" {
				request.Header.Set("Cookie", cookie)
			}
		}
		client := f.client
		if client == nil {
			client = http.DefaultClient
		}
		return client.Do(request.WithContext(ctx))
	}

	// A prerendered path is a file, and Kit answers it with a plain request that
	// carries nothing of this page's: no cookies, no Authorization, no default
	// Accept, and nothing to collect from what comes back.
	if f.hasPrerenderedPath(decoded) {
		return f.serve(ctx, src, request)
	}

	if credentials != FetchOmit {
		if cookie := src.jar.requestHeader(src.request, u, request.Header.Get("Cookie")); cookie != "" {
			request.Header.Set("Cookie", cookie)
		}
		if authorization := src.request.Header.Get("Authorization"); authorization != "" && !hasFetchHeader(request.Header, "Authorization") {
			request.Header.Set("Authorization", authorization)
		}
	}
	if !hasFetchHeader(request.Header, "Accept") {
		request.Header.Set("Accept", "*/*")
	}
	if language := src.request.Header.Get("Accept-Language"); language != "" && !hasFetchHeader(request.Header, "Accept-Language") {
		request.Header.Set("Accept-Language", language)
	}
	response, err := f.serve(ctx, src, request)
	if err != nil {
		return nil, err
	}
	// Whatever the answer asked the visitor to store is now this request's own
	// cookie, whether or not the fetch sent credentials.
	if src.jar != nil {
		// A cookie that names no path applies to the directory it was fetched from.
		fallback := "/"
		if i := strings.LastIndex(u.EscapedPath(), "/"); i > 0 {
			fallback = u.EscapedPath()[:i]
		}
		for _, header := range response.Header.Values("Set-Cookie") {
			c := parseSetCookie(header)
			if c.name == "" {
				continue
			}
			if c.path == "" {
				c.path = fallback
			}
			src.jar.setInternal(c)
		}
	}
	return response, nil
}

// serve runs the in-process handler for one subrequest and returns its
// response once committed. Like Kit, the fetch rejects when it is cancelled even
// if the handler is still running; the handler's own context is cancelled too,
// so a well-behaved one stops.
func (f *requestFetcher) serve(ctx context.Context, src fetchSource, request *http.Request) (*http.Response, error) {
	if f.handler == nil {
		return nil, errors.New("skgo: no handler is configured to answer a fetch of this app's own routes")
	}
	// A subrequest is a request of its own: nothing the originating request
	// stored travels with it, only its cancellation. Its locals start empty.
	// What Kit's `fork_state_for_subrequest` keeps is the request facilities
	// of the server — here the client's address — and how deep it is; and the
	// engine needs to know how many renders are waiting on this one.
	sub := valuelessContext{ctx}
	subCtx := context.WithValue(sub, requestFetcherKey{}, f)
	subCtx = withFetchDepth(subCtx, fetchDepthOf(ctx)+1)
	subCtx = ssr.CarryDepth(subCtx, ctx)
	subCtx = carryDevRender(subCtx, ctx)
	request = request.WithContext(subCtx)
	if request.Body == nil {
		request.Body = http.NoBody
	}
	if src.request != nil {
		request.RemoteAddr = src.request.RemoteAddr
	}

	// The handler runs on its own goroutine and the fetch resolves when it has
	// committed its status and headers, as Kit's does when `respond` returns
	// its Response: the body is whatever the handler goes on to write, read as
	// it is produced. Cancelling ctx, closing the body or reading past the end
	// of a handler that was cancelled all end the handler's context.
	d := launchDownstream(subCtx, subCtx, f.handler, request, true)
	select {
	case <-d.committed:
	case <-ctx.Done():
		d.release()
		return nil, ctx.Err()
	}
	d.mu.Lock()
	panicked, early := d.panicked, d.preCommit
	d.mu.Unlock()
	if panicked != nil && early {
		d.release()
		return nil, fmt.Errorf("skgo: the handler answering a fetch panicked: %v", panicked.value)
	}
	response := d.response(request)
	response.Status = fmt.Sprintf("%03d %s", response.StatusCode, http.StatusText(response.StatusCode))
	if n, err := strconv.ParseInt(response.Header.Get("Content-Length"), 10, 64); err == nil && n >= 0 {
		response.ContentLength = n
	}
	if request.Method == http.MethodHead {
		d.discard()
		response.Body = http.NoBody
	}
	return response, nil
}

// valuelessContext keeps a context's cancellation and deadline and nothing it
// carries.
type valuelessContext struct{ parent context.Context }

func (v valuelessContext) Deadline() (time.Time, bool) { return v.parent.Deadline() }
func (v valuelessContext) Done() <-chan struct{}       { return v.parent.Done() }
func (v valuelessContext) Err() error                  { return v.parent.Err() }
func (valuelessContext) Value(any) any                 { return nil }

// requestHeader is Kit's get_cookie_header: what the user agent sent has the
// lowest precedence, a cookie this request wrote and that applies to the
// destination beats it, and a Cookie header on the fetch itself beats both.
// Values travel exactly as they arrived or were written, not decoded.
func (j *cookieJar) requestHeader(parent *http.Request, destination *url.URL, explicit string) string {
	names, values := parseCookieHeader(strings.Join(parent.Header.Values("Cookie"), "; "))
	add := func(name, value string) {
		if _, seen := values[name]; !seen {
			names = append(names, name)
		}
		values[name] = value
	}
	if j != nil {
		j.mu.Lock()
		for _, key := range j.order {
			c := j.written[key]
			if !cookieDomainMatches(destination.Hostname(), c.domain) || !cookiePathMatches(destination.EscapedPath(), c.path) {
				continue
			}
			if c.raw {
				add(c.name, c.value)
			} else {
				add(c.name, encodeURIComponent(c.value))
			}
		}
		j.mu.Unlock()
	}
	if explicit != "" {
		explicitNames, explicitValues := parseCookieHeader(explicit)
		for _, name := range explicitNames {
			add(name, explicitValues[name])
		}
	}
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, name+"="+values[name])
	}
	return strings.Join(parts, "; ")
}

// encodeURIComponent is JavaScript's, which Kit applies to a cookie value it
// forwards unless the cookie was written with its own encoder.
func encodeURIComponent(s string) string {
	const unreserved = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_.!~*'()"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if strings.IndexByte(unreserved, s[i]) >= 0 {
			b.WriteByte(s[i])
			continue
		}
		fmt.Fprintf(&b, "%%%02X", s[i])
	}
	return b.String()
}

func cookieDomainMatches(hostname, constraint string) bool {
	if constraint == "" {
		return true
	}
	constraint = strings.TrimPrefix(constraint, ".")
	return hostname == constraint || strings.HasSuffix(hostname, "."+constraint)
}

func cookiePathMatches(path, constraint string) bool {
	if constraint == "" {
		return true
	}
	constraint = strings.TrimSuffix(constraint, "/")
	return path == constraint || strings.HasPrefix(path, constraint+"/")
}

func requestHeaders(r *http.Request) map[string]string {
	headers := make(map[string]string, len(r.Header))
	for name := range r.Header {
		headers[name] = r.Header.Get(name)
	}
	return headers
}
