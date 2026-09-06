package skgo

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// Event is the per-call handle a remote function receives. It is skgo's
// equivalent of the `RequestEvent` kit hands to a remote function through
// `getRequestEvent()`, narrowed the same way kit narrows it.
//
// Kit derives a restricted event for every remote function
// (`runtime/app/server/remote/shared.js`): `setHeaders` throws, and inside a
// `query`, `query.live`, `query.batch` or `prerender` so does `cookies.set`
// and `cookies.delete`. skgo enforces the same rule with types instead of
// throws: a query receives an *Event, which can only read cookies, and a
// command receives a *CommandEvent, which can also write them.
type Event struct {
	req    *http.Request
	jar    *cookieJar
	writer http.ResponseWriter
}

// Context is the request's context. It is cancelled when the client goes away,
// which is how a `query.live` producer learns to stop.
func (e *Event) Context() context.Context { return e.req.Context() }

// Request is the HTTP request that carried this call.
//
// Note that its URL is the remote-function endpoint — `/_app/remote/<hash>/<name>` —
// not the page the user is looking at. Kit is stricter still: reading
// `event.url`, `event.params` or `event.route` inside a query throws, because a
// query's result is cached by its argument and would go stale. Anything a
// remote function needs about the page belongs in its argument.
func (e *Event) Request() *http.Request { return e.req }

// Cookie returns the value of a request cookie. A cookie written earlier in
// the same call shadows the one the browser sent, and a cookie deleted earlier
// reads as absent — the same precedence kit's `cookies.get` applies.
func (e *Event) Cookie(name string) (string, bool) { return e.jar.get(name) }

// CommandEvent is the handle a `command` receives. Kit allows cookie writes in
// commands and forms and nowhere else, so this is the only event type that can
// make them.
type CommandEvent struct {
	Event
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

// SetCookie writes a cookie on the command's response.
func (e *CommandEvent) SetCookie(name, value string, opts CookieOptions) error {
	return e.jar.set(name, value, opts, false)
}

// DeleteCookie removes a cookie. Kit implements `cookies.delete` as a `set`
// with an empty value and `maxAge: 0`, and so does this; the options must name
// the same Path and Domain the cookie was set with or the browser keeps it.
func (e *CommandEvent) DeleteCookie(name string, opts CookieOptions) error {
	return e.jar.set(name, "", opts, true)
}

// cookieJar collects the cookies one call writes and answers reads with the
// same precedence kit's `get_cookies` does: a cookie set during the request
// wins over the request header, and a deleted one reads as absent.
type cookieJar struct {
	// header holds what the browser sent.
	header map[string]string
	// written holds what this call wrote, keyed by name. Kit keys by
	// domain+path+name, which only matters for an app that writes the same
	// name at two paths; skgo refuses that instead of modelling it.
	written map[string]*http.Cookie
	// order preserves write order so the response headers are deterministic.
	order []string

	// secureDefault is kit's `secure` default for this app: false only when
	// the app is served over plain HTTP from localhost.
	secureDefault bool
}

func newCookieJar(r *http.Request, secureDefault bool) *cookieJar {
	jar := &cookieJar{
		header:        map[string]string{},
		written:       map[string]*http.Cookie{},
		secureDefault: secureDefault,
	}
	for _, c := range r.Cookies() {
		jar.header[c.Name] = c.Value
	}
	return jar
}

func (j *cookieJar) get(name string) (string, bool) {
	if c, ok := j.written[name]; ok {
		if c.MaxAge < 0 {
			return "", false
		}
		return c.Value, true
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

	c := &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     opts.Path,
		Domain:   opts.Domain,
		Expires:  opts.Expires,
		MaxAge:   opts.MaxAge,
		HttpOnly: true,
		Secure:   j.secureDefault,
		SameSite: http.SameSiteLaxMode,
	}
	if c.Path == "" {
		c.Path = "/"
	}
	if opts.SameSite != 0 {
		c.SameSite = opts.SameSite
	}
	if opts.HTTPOnly != nil {
		c.HttpOnly = *opts.HTTPOnly
	}
	if opts.Secure != nil {
		c.Secure = *opts.Secure
	}
	if del {
		// net/http writes `Max-Age=0` for a negative MaxAge, which is exactly
		// what kit's `cookies.delete` sends.
		c.MaxAge = -1
		c.Value = ""
	}

	if _, seen := j.written[name]; !seen {
		j.order = append(j.order, name)
	}
	j.written[name] = c
	return nil
}

// writeTo appends a Set-Cookie header for every cookie this call wrote. Kit
// does the same in `add_cookies_to_headers` once the response exists.
func (j *cookieJar) writeTo(h http.Header) {
	for _, name := range j.order {
		if v := j.written[name].String(); v != "" {
			h.Add("Set-Cookie", v)
		}
	}
}

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
