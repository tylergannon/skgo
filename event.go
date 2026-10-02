package skgo

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Event is the per-call handle a remote function reaches through its context.
// It is skgo's equivalent of the `RequestEvent` kit hands to a remote function
// through `getRequestEvent()`, narrowed the same way kit narrows it.
//
// Kit derives a restricted event for every remote function
// (`runtime/app/server/remote/shared.js`): `setHeaders` throws, and inside a
// `query`, `query.live`, `query.batch` or `prerender` so do `cookies.set` and
// `cookies.delete`. skgo enforces the same rules; the difference is that a Go
// handler returns the refusal rather than throwing it.
type Event struct {
	req *http.Request
	jar *cookieJar
	// A page action shares response headers with the loads that follow it.
	actionResponse *loadRequest
	// mutable reports that this call may write cookies, which is true of a
	// command and of a server load. A query refreshed by a command shares the
	// command's cookie jar but gets its own immutable event, exactly as kit
	// derives a fresh non-cookie-writing event for it.
	mutable bool
	// load is non-nil inside a server load. It carries the page's URL, the
	// route parameters, and the record of what the load read — which is what
	// tells kit's client whether the load has to run again on the next
	// navigation. Outside a load, the methods that need it answer as they do
	// on a nil event, because kit forbids reading any of it from a query.
	load *loadState
	// A classic page action receives the matched route parameters without
	// becoming a server load or recording client invalidation dependencies.
	params map[string]string
	// endpoint marks the event of a server route. An endpoint owns the
	// http.ResponseWriter, so it writes cookies and headers with net/http; the
	// setters here refuse rather than accept something nothing would apply.
	endpoint bool
	// refreshes is where skgo.Refresh registers, and it is non-nil only inside
	// a command or a form. Kit hangs the same record off the request state
	// (`state.remote.explicit`) and, like this one, only a command or a form
	// has anywhere for the result to ride back on. The queries a command
	// refreshes get events derived from its own, and the derivation copies the
	// pointer, so a refreshed query can refresh another.
	refreshes *refreshSet
	// hook is non-nil only on the event the `handle` hook receives, which is
	// the one event that sees everything kit resolved before it ran: the
	// normalized URL, the matched route and its parameters, and what kind of
	// request this is. Every other event is narrower, and a remote function's
	// stays that way however much the hook can see.
	hook *hookState
}

type eventKey struct{}

// EventFrom returns the remote function's event, mirroring kit's
// `getRequestEvent()`. Outside a remote function it returns nil, and every
// method below is safe on a nil *Event, so a helper shared with non-remote
// code does not have to branch.
func EventFrom(ctx context.Context) *Event {
	e, _ := ctx.Value(eventKey{}).(*Event)
	return e
}

// withEvent is how the dispatcher makes the event reachable.
func withEvent(ctx context.Context, e *Event) context.Context {
	return context.WithValue(ctx, eventKey{}, e)
}

// Request is the HTTP request that carried this call, or nil outside a remote
// function.
//
// Note that its URL is the remote-function endpoint — `/_app/remote/<hash>/<name>` —
// not the page the user is looking at. Kit is stricter still: reading
// `event.url`, `event.params` or `event.route` inside a query throws, because a
// query's result is cached by its argument and would go stale, so the client
// would keep showing an answer computed for a page it has since left. Anything
// a remote function needs about the page belongs in its argument.
func (e *Event) Request() *http.Request {
	if e == nil {
		return nil
	}
	return e.req
}

// Cookie returns the value of a request cookie. A cookie written earlier in
// the same call shadows the one the browser sent, and a cookie deleted earlier
// reads as absent — the same precedence kit's `cookies.get` applies.
func (e *Event) Cookie(name string) (string, bool) {
	if e == nil {
		return "", false
	}
	return e.jar.get(name)
}

// CookieOptions mirrors the options kit's `cookies.set` accepts. A zero field
// takes kit's default: Path "/", HttpOnly, SameSite=Lax, and Secure everywhere
// except an app served over plain HTTP from localhost.
type CookieOptions struct {
	// Path defaults to "/". Kit requires an absolute path in a remote
	// function; SetCookie returns an error for anything else.
	Path string
	// Domain defaults to the request host.
	Domain string
	// MaxAge is the cookie's lifetime in seconds. Zero omits the attribute,
	// making a session cookie, exactly as kit does.
	MaxAge int
	// Expires, when non-zero, sets an absolute expiry.
	Expires time.Time
	// SameSite defaults to http.SameSiteLaxMode.
	SameSite http.SameSite
	// HTTPOnly defaults to true. Set it to a pointer to false to opt out.
	HTTPOnly *bool
	// Secure defaults to true unless the app's origin is http://localhost.
	// Set it to a pointer to false to opt out.
	Secure *bool
}

// SetCookie writes a cookie on the response.
//
// Only a `command` may do this. Kit throws "Cannot set cookies in `query` or
// `prerender` functions" everywhere else, because a query's result is cached
// by its argument and replayed from that cache, so a cookie it wrote would be
// written once and then silently skipped.
func (e *Event) SetCookie(name, value string, opts CookieOptions) error {
	if err := e.mayWriteCookies("set"); err != nil {
		return err
	}
	return e.jar.set(name, value, opts, false)
}

// DeleteCookie removes a cookie. Kit implements `cookies.delete` as a `set`
// with an empty value and `maxAge: 0`, and so does this; the options must name
// the same Path and Domain the cookie was set with or the browser keeps it.
func (e *Event) DeleteCookie(name string, opts CookieOptions) error {
	if err := e.mayWriteCookies("delete"); err != nil {
		return err
	}
	return e.jar.set(name, "", opts, true)
}

func (e *Event) mayWriteCookies(verb string) error {
	if e == nil {
		return Errorf(500, "skgo: cannot %s cookies outside a remote function", verb)
	}
	if e.endpoint {
		return Errorf(500, "skgo: a server route writes its own response — use http.SetCookie to %s a cookie", verb)
	}
	if e.hook != nil && e.hook.sealed() {
		return Errorf(500, "skgo: cannot %s cookies after the response has been generated", verb)
	}
	if !e.mutable {
		return Errorf(500, "skgo: cannot %s cookies in a query; only a command may write them", verb)
	}
	return nil
}

// cookieJar collects the cookies one call writes and answers reads with the
// same precedence kit's `get_cookies` does: a cookie set during the request
// wins over the request header, and a deleted one reads as absent.
//
// One jar is shared by every load of a branch, and kit runs those loads
// concurrently (`load_server_data` is started for every node at once, each
// awaiting `parent()` only if it asks), so two of them may write cookies at the
// same moment. mu is what lets them.
type cookieJar struct {
	mu sync.Mutex

	// header holds what the browser sent.
	header map[string]string
	// rawHeader is the Cookie header as the browser sent it. Kit forwards its
	// values to a fetch without decoding them.
	rawHeader string
	// hostname and pathname are the request's own, which decide whether a
	// cookie written earlier applies to a read.
	hostname, pathname string
	// written holds what this call wrote, under kit's key: the domain, path
	// and name together, so one name may live at two paths.
	written map[string]*jarCookie
	// order preserves write order so the response headers are deterministic.
	order []string

	// secureDefault is kit's `secure` default for this app: false only when
	// the app is served over plain HTTP from localhost.
	secureDefault bool
}

// jarCookie is one cookie a request wrote.
type jarCookie struct {
	name, value  string
	domain, path string
	// maxAge is kit's `maxAge`; zero with hasMaxAge is a deletion.
	maxAge    int
	hasMaxAge bool
	expires   time.Time
	httpOnly  bool
	secure    bool
	// partitioned and priority only arrive on a cookie a fetch brought back.
	partitioned bool
	priority    string
	sameSite    string
	// raw is a value that arrived in a Set-Cookie header of an internal fetch
	// and goes back out exactly as it came, as kit's `encode: (v) => v` does.
	raw bool
}

func newCookieJar(r *http.Request, secureDefault bool) *cookieJar {
	host := r.Host
	if r.URL != nil && r.URL.Host != "" {
		host = r.URL.Host
	}
	pathname := ""
	if r.URL != nil {
		pathname = r.URL.Path
	}
	return newCookieJarAt(r, host, pathname, secureDefault)
}

// newCookieJarAt is kit's `get_cookies(request, url)`: the request's own
// cookies, answering reads and matching writes against the URL the app sees,
// which is not always the one that arrived.
func newCookieJarAt(r *http.Request, host, pathname string, secureDefault bool) *cookieJar {
	jar := &cookieJar{
		header:        map[string]string{},
		written:       map[string]*jarCookie{},
		secureDefault: secureDefault,
		rawHeader:     strings.Join(r.Header.Values("Cookie"), "; "),
	}
	for _, c := range r.Cookies() {
		jar.header[c.Name] = c.Value
	}
	jar.hostname = hostnameOf(host)
	jar.pathname = pathname
	return jar
}

func hostnameOf(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return strings.Trim(host, "[]")
}

func cookieKey(domain, path, name string) string {
	return domain + path + "?" + url.QueryEscape(name)
}

// matches is kit's `matches_url`.
func (j *cookieJar) matches(c *jarCookie) bool {
	return cookieDomainMatches(j.hostname, c.domain) && cookiePathMatches(j.pathname, c.path)
}

func (c *jarCookie) deleted() bool { return c.hasMaxAge && c.maxAge == 0 }

func (j *cookieJar) get(name string) (string, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	// The most specific cookie of this name that applies to the request wins.
	var best *jarCookie
	for _, key := range j.order {
		c := j.written[key]
		if c.name == name && j.matches(c) && (best == nil || len(c.path) > len(best.path)) {
			best = c
		}
	}
	if best != nil {
		if best.deleted() {
			return "", false
		}
		return best.value, true
	}
	v, ok := j.header[name]
	return v, ok
}

func (j *cookieJar) set(name, value string, opts CookieOptions, del bool) error {
	if name == "" {
		return Errorf(500, "skgo: cookie name must not be empty")
	}
	// Kit: "Cookies set in remote functions must have an absolute path".
	if opts.Path != "" && !strings.HasPrefix(opts.Path, "/") {
		return Errorf(500, "skgo: cookies set in remote functions must have an absolute path, got %q", opts.Path)
	}

	c := &jarCookie{
		name:     name,
		value:    value,
		path:     opts.Path,
		domain:   opts.Domain,
		expires:  opts.Expires,
		httpOnly: true,
		secure:   j.secureDefault,
		sameSite: "Lax",
	}
	if opts.MaxAge != 0 {
		c.maxAge, c.hasMaxAge = opts.MaxAge, true
	}
	if c.path == "" {
		c.path = "/"
	}
	switch opts.SameSite {
	case http.SameSiteStrictMode:
		c.sameSite = "Strict"
	case http.SameSiteNoneMode:
		c.sameSite = "None"
	}
	if opts.HTTPOnly != nil {
		c.httpOnly = *opts.HTTPOnly
	}
	if opts.Secure != nil {
		c.secure = *opts.Secure
	}
	if del {
		// Kit's `cookies.delete` is a set with an empty value and `maxAge: 0`.
		c.maxAge, c.hasMaxAge, c.value = 0, true, ""
	}

	j.mu.Lock()
	defer j.mu.Unlock()
	j.store(c)
	return nil
}

func (j *cookieJar) store(c *jarCookie) {
	key := cookieKey(c.domain, c.path, c.name)
	if _, seen := j.written[key]; !seen {
		j.order = append(j.order, key)
	}
	j.written[key] = c
}

// setInternal is kit's `set_internal` as `event.fetch` calls it for a Set-Cookie
// header an internal response carried: the value stays as the response wrote
// it, no default attribute is added, and a relative path resolves against the
// page being served.
func (j *cookieJar) setInternal(c *jarCookie) {
	if c.domain == "" || c.domain == j.hostname {
		if !strings.HasPrefix(c.path, "/") {
			base := &url.URL{Path: j.pathname}
			c.path = base.ResolveReference(&url.URL{Path: c.path}).Path
		}
	}
	c.raw = true
	j.mu.Lock()
	defer j.mu.Unlock()
	j.store(c)
}

// httpCookie is the cookie as net/http spells it, with a deletion as a negative
// MaxAge.
func (c *jarCookie) httpCookie() *http.Cookie {
	h := &http.Cookie{
		Name: c.name, Value: c.value, Path: c.path, Domain: c.domain, Expires: c.expires,
		HttpOnly: c.httpOnly, Secure: c.secure, Partitioned: c.partitioned,
	}
	switch {
	case c.deleted():
		h.MaxAge = -1
	case c.hasMaxAge:
		h.MaxAge = c.maxAge
	}
	switch c.sameSite {
	case "Lax":
		h.SameSite = http.SameSiteLaxMode
	case "Strict":
		h.SameSite = http.SameSiteStrictMode
	case "None":
		h.SameSite = http.SameSiteNoneMode
	}
	return h
}

// header is the Set-Cookie value for path.
func (c *jarCookie) header(path string) string {
	if !c.raw {
		h := c.httpCookie()
		h.Path = path
		return h.String()
	}
	// kit's `stringifySetCookie`, with the value untouched.
	var b strings.Builder
	b.WriteString(c.name + "=" + c.value)
	if c.hasMaxAge {
		b.WriteString("; Max-Age=" + strconv.Itoa(c.maxAge))
	}
	if c.domain != "" {
		b.WriteString("; Domain=" + c.domain)
	}
	if path != "" {
		b.WriteString("; Path=" + path)
	}
	if !c.expires.IsZero() {
		b.WriteString("; Expires=" + c.expires.UTC().Format(http.TimeFormat))
	}
	if c.httpOnly {
		b.WriteString("; HttpOnly")
	}
	if c.secure {
		b.WriteString("; Secure")
	}
	if c.partitioned {
		b.WriteString("; Partitioned")
	}
	switch c.priority {
	case "low":
		b.WriteString("; Priority=Low")
	case "medium":
		b.WriteString("; Priority=Medium")
	case "high":
		b.WriteString("; Priority=High")
	}
	if c.sameSite != "" {
		b.WriteString("; SameSite=" + c.sameSite)
	}
	return b.String()
}

// writeTo appends a Set-Cookie header for every cookie this call wrote. Kit
// does the same in `add_cookies_to_headers` once the response exists.
func (j *cookieJar) writeTo(h http.Header) {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, key := range j.order {
		c := j.written[key]
		if v := c.header(c.path); v != "" {
			h.Add("Set-Cookie", v)
		}
		// A route ending in .html keeps its data in a sibling file, so the
		// cookie has to apply to that path as well.
		if strings.HasSuffix(c.path, ".html") {
			h.Add("Set-Cookie", c.header(strings.TrimSuffix(c.path, ".html")+htmlDataSuffix))
		}
	}
}

// writeMissingTo is writeTo for a response some other part of the stack may
// already have written this jar into: a cookie whose header is already there is
// not added again, so one jar shared by every layer of a request is emitted once.
func (j *cookieJar) writeMissingTo(h http.Header) {
	j.mu.Lock()
	defer j.mu.Unlock()
	have := map[string]bool{}
	for _, v := range h.Values("Set-Cookie") {
		have[v] = true
	}
	add := func(v string) {
		if v != "" && !have[v] {
			have[v] = true
			h.Add("Set-Cookie", v)
		}
	}
	for _, key := range j.order {
		c := j.written[key]
		add(c.header(c.path))
		if strings.HasSuffix(c.path, ".html") {
			add(c.header(strings.TrimSuffix(c.path, ".html") + htmlDataSuffix))
		}
	}
}

// empty reports whether nothing was written, so a response writer has nothing
// to add.
func (j *cookieJar) empty() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return len(j.order) == 0
}

// snapshot is the cookies written so far, in write order.
func (j *cookieJar) snapshot() []*http.Cookie {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := make([]*http.Cookie, 0, len(j.order))
	for _, key := range j.order {
		out = append(out, j.written[key].httpCookie())
	}
	return out
}

// parseSetCookie is `parseSetCookie` from the `cookie` package kit uses, with
// the value left undecoded.
func parseSetCookie(header string) *jarCookie {
	parts := strings.Split(header, ";")
	trim := func(s string) string { return strings.Trim(s, " \t") }
	c := &jarCookie{}
	first := parts[0]
	if i := strings.Index(first, "="); i >= 0 {
		c.name, c.value = trim(first[:i]), trim(first[i+1:])
	} else {
		c.value = trim(first)
	}
	for _, part := range parts[1:] {
		attr, val, hasVal := part, "", false
		if i := strings.Index(part, "="); i >= 0 {
			attr, val, hasVal = part[:i], trim(part[i+1:]), true
		}
		switch strings.ToLower(trim(attr)) {
		case "httponly":
			c.httpOnly = true
		case "secure":
			c.secure = true
		case "partitioned":
			c.partitioned = true
		case "domain":
			c.domain = val
		case "path":
			c.path = val
		case "max-age":
			if n, err := strconv.Atoi(val); err == nil && hasVal && val != "" && !strings.ContainsAny(val, " +") {
				c.maxAge, c.hasMaxAge = n, true
			}
		case "expires":
			if t, ok := parseCookieDate(val); ok {
				c.expires = t
			}
		case "priority":
			switch p := strings.ToLower(val); p {
			case "low", "medium", "high":
				c.priority = p
			}
		case "samesite":
			switch strings.ToLower(val) {
			case "lax":
				c.sameSite = "Lax"
			case "strict":
				c.sameSite = "Strict"
			case "none":
				c.sameSite = "None"
			}
		}
	}
	return c
}

func parseCookieDate(val string) (time.Time, bool) {
	for _, layout := range []string{http.TimeFormat, time.RFC850, time.ANSIC, "Mon, 02-Jan-2006 15:04:05 MST", time.RFC1123Z, time.RFC3339} {
		if t, err := time.Parse(layout, val); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// parseCookieHeader is the `cookie` package's `parseCookie` with no decoding:
// the first occurrence of a name wins.
func parseCookieHeader(header string) (names []string, values map[string]string) {
	values = map[string]string{}
	for _, part := range strings.Split(header, ";") {
		i := strings.Index(part, "=")
		if i < 0 {
			continue
		}
		name, value := strings.Trim(part[:i], " \t"), strings.Trim(part[i+1:], " \t")
		if name == "" {
			continue
		}
		if _, seen := values[name]; !seen {
			names = append(names, name)
			values[name] = value
		}
	}
	return names, values
}

// cookieWriter adds the cookies a request's jar holds to the response when its
// header is sent, as Kit's `add_cookies_to_headers` does for a response built
// by anything at all, including an endpoint that owns its ResponseWriter.
type cookieWriter struct {
	http.ResponseWriter
	jar  *cookieJar
	sent bool
}

func (c *cookieWriter) flushCookies() {
	if !c.sent {
		c.sent = true
		c.jar.writeTo(c.Header())
	}
}

func (c *cookieWriter) WriteHeader(status int) {
	if status >= 200 || status == http.StatusSwitchingProtocols {
		c.flushCookies()
	}
	c.ResponseWriter.WriteHeader(status)
}

func (c *cookieWriter) Write(b []byte) (int, error) {
	c.flushCookies()
	return c.ResponseWriter.Write(b)
}

func (c *cookieWriter) Flush() {
	c.flushCookies()
	http.NewResponseController(c.ResponseWriter).Flush()
}

func (c *cookieWriter) Unwrap() http.ResponseWriter { return c.ResponseWriter }

// secureCookieDefault reproduces kit's `secure` default
// (`runtime/server/cookie.js`): cookies are Secure unless the app is running in
// dev, or is being served over plain HTTP from localhost.
func secureCookieDefault(origin string, dev bool) bool {
	if dev {
		return false
	}
	host := origin
	if i := strings.Index(host, "://"); i >= 0 {
		scheme := host[:i]
		host = host[i+3:]
		if h, _, ok := strings.Cut(host, ":"); ok {
			host = h
		}
		if scheme == "http" && host == "localhost" {
			return false
		}
	}
	return true
}
