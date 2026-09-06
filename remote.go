// Remote-function dispatch: the Go side of SvelteKit's `query`, `query.live`,
// `command` and `form`.
//
// A kit client calls a remote function by fetching
// `${base}/${appDir}/remote/<hash>/<name>`, where <hash> is kit's id hash of
// the module's vite-root-relative path. Go answers those requests directly —
// no Node is involved — by looking the id up in a Remotes registry and running
// an ordinary Go function.
package skgo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"runtime/debug"
	"sort"
	"strings"

	"github.com/tylergannon/skgo/internal/devalue"
	"github.com/tylergannon/skgo/internal/kithash"
	"github.com/tylergannon/skgo/internal/remotearg"
)

// None is the argument type of a remote function that takes no argument.
type None struct{}

// HTTPError is an error with a status kit's client understands. Returning one
// from a remote function produces `{"type":"error","error":{...}}`; any other
// error becomes an opaque 500, exactly as kit does for unexpected throws.
type HTTPError struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
}

func (e *HTTPError) Error() string { return fmt.Sprintf("%d %s", e.Status, e.Message) }

// Errorf builds an HTTPError. It is the analogue of kit's `error(status, message)`.
func Errorf(status int, format string, args ...any) *HTTPError {
	return &HTTPError{Status: status, Message: fmt.Sprintf(format, args...)}
}

// Redirect is an error that makes the client navigate. It is honoured for
// queries and live queries; kit's client throws when a *command* response
// carries a redirect, so a Redirect returned from a command is reported as an
// ordinary error instead.
//
// Status is kit's `redirect(status, location)` status and must be in 300-308,
// the range kit validates in `exports/index.js`. Build one with NewRedirect,
// which applies that check and the header-safety check kit's own Redirect
// constructor applies to the location.
//
// The status does not reach the browser from a remote function, and that is
// kit's design, not a gap here: kit serialises a remote-function redirect as
// `{redirect: location}` inside the *success* envelope and drops the status
// (`runtime/server/remote-functions.js`, the `error instanceof Redirect`
// branch), and its client calls `goto(location)` on it. A live query's frame
// is `{type:"redirect",location}` and kit's client fabricates a 307 purely so
// its reconnect loop can recognise it. The status is carried and validated
// here because it is part of what a caller means, and it becomes observable
// wherever a redirect is answered as HTTP rather than as an envelope.
type Redirect struct {
	// Status is the HTTP status kit's client is told about. Kit requires one
	// of 300..308; zero means 307, kit's own default for a redirect that does
	// not name one.
	Status int
	// Location is where the client is sent.
	Location string
}

// NewRedirect builds a Redirect, applying kit's own two checks: `redirect()`
// refuses a status outside 300-308 with "Invalid status code", and the
// Redirect constructor refuses a location that cannot be a header value.
//
// It reports an error rather than panicking, because a redirect target is
// usually built from a request and a bad one is a request problem.
func NewRedirect(status int, location string) (*Redirect, error) {
	if status < 300 || status > 308 {
		return nil, fmt.Errorf("skgo: invalid redirect status code %d: kit's redirect() accepts 300-308", status)
	}
	if !validHeaderValue(location) {
		return nil, fmt.Errorf("skgo: invalid redirect location %q: this string contains characters that cannot be used in HTTP headers", location)
	}
	return &Redirect{Status: status, Location: location}, nil
}

// validHeaderValue mirrors what `new Headers({location})` rejects: control
// characters, which is what would let a location split the response.
func validHeaderValue(v string) bool {
	for i := 0; i < len(v); i++ {
		if c := v[i]; c < 0x20 || c == 0x7f {
			return false
		}
	}
	return true
}

// status is the status to put on the wire. A Redirect built by hand rather
// than by NewRedirect may leave Status zero; kit's own default is 307.
func (r *Redirect) status() int {
	if r.Status == 0 {
		return http.StatusTemporaryRedirect
	}
	return r.Status
}

func (r *Redirect) Error() string {
	if r.Status == 0 {
		return "redirect to " + r.Location
	}
	return fmt.Sprintf("redirect %d to %s", r.Status, r.Location)
}

type remoteKind int

const (
	kindQuery remoteKind = iota
	kindCommand
	kindLive
	kindForm
)

// Remote is one registered remote function. Generated code builds these with
// NewQuery, NewCommand, NewLiveQuery and NewForm; application code declares the
// functions and marks them with Query, Command, LiveQuery and Form.
type Remote struct {
	module string
	name   string
	hash   string
	id     string
	kind   remoteKind

	call func(ctx context.Context, arg any, present bool) (any, error)
	live func(ctx context.Context, arg any, present bool, yield func(any) error) error
}

// ID is the `<hash>/<name>` pair the client addresses this function by.
func (r *Remote) ID() string { return r.id }

// Module is the vite-root-relative path of the `.remote.ts` module.
func (r *Remote) Module() string { return r.module }

// Name is the export name within that module.
func (r *Remote) Name() string { return r.name }

func newRemote(module, name string, kind remoteKind) *Remote {
	hash := kithash.Kit(module)
	return &Remote{
		module: module,
		name:   name,
		hash:   hash,
		id:     hash + "/" + name,
		kind:   kind,
	}
}

// Marker is what the declaration helpers return. It carries nothing: Query,
// Command, LiveQuery and Form exist to be read by `skgo generate`, and to make
// a function with the wrong shape a compile error at the point of declaration.
type Marker struct{}

// Query declares fn as a SvelteKit `query`. Write it beside the function, in a
// file named `*.remote.go`:
//
//	func getTodos(ctx context.Context, _ skgo.None) ([]Todo, error) { ... }
//
//	var _ = skgo.Query(getTodos)
//
// `skgo generate` emits the sibling `.remote.ts` kit compiles and the Go
// registration that answers the calls. The request is reachable with
// skgo.EventFrom(ctx); a query may read cookies but not write them, which is
// the restriction kit places on its own queries.
func Query[In, Out any](fn func(context.Context, In) (Out, error)) Marker { _ = fn; return Marker{} }

// Command declares fn as a SvelteKit `command`. A command's event may write
// cookies; kit allows that in commands and forms and nowhere else.
func Command[In, Out any](fn func(context.Context, In) (Out, error)) Marker { _ = fn; return Marker{} }

// LiveQuery declares fn as a SvelteKit `query.live`. fn pushes values with
// yield and returns when the subscription ends; its context is cancelled when
// the client disconnects.
func LiveQuery[In, Out any](fn func(context.Context, In, func(Out) error) error) Marker {
	_ = fn
	return Marker{}
}

// NewQuery registers a `query` export. module is the module's vite-root-relative
// path (for example "src/lib/todos.remote.ts") and name the export name.
// Generated code calls this; application code uses Query.
func NewQuery[In, Out any](module, name string, fn func(context.Context, In) (Out, error)) *Remote {
	r := newRemote(module, name, kindQuery)
	r.call = callAdapter(fn)
	return r
}

// NewCommand registers a `command` export.
func NewCommand[In, Out any](module, name string, fn func(context.Context, In) (Out, error)) *Remote {
	r := newRemote(module, name, kindCommand)
	r.call = callAdapter(fn)
	return r
}

// NewLiveQuery registers a `query.live` export. fn pushes values with yield and
// returns when the subscription ends; its event's context is cancelled when the
// client disconnects, and yield returns a non-nil error once that has happened.
func NewLiveQuery[In, Out any](module, name string, fn func(context.Context, In, func(Out) error) error) *Remote {
	r := newRemote(module, name, kindLive)
	r.live = func(ctx context.Context, arg any, present bool, yield func(any) error) error {
		in, err := decodeArg[In](arg, present)
		if err != nil {
			return err
		}
		return fn(ctx, in, func(out Out) error {
			v, err := encodeValue(out)
			if err != nil {
				return err
			}
			return yield(v)
		})
	}
	return r
}

func callAdapter[In, Out any](fn func(context.Context, In) (Out, error)) func(context.Context, any, bool) (any, error) {
	return func(ctx context.Context, arg any, present bool) (any, error) {
		in, err := decodeArg[In](arg, present)
		if err != nil {
			return nil, err
		}
		out, err := fn(ctx, in)
		if err != nil {
			return nil, err
		}
		return encodeValue(out)
	}
}

// decodeArg turns a devalue tree into a typed Go value with an encoding/json
// round-trip. That is deliberately the 90% solution: anything kit can send
// that survives JSON survives this, and nothing else is supported.
func decodeArg[In any](arg any, present bool) (In, error) {
	var in In
	if !present {
		return in, nil
	}
	raw, err := json.Marshal(arg)
	if err != nil {
		// The argument came from the client, so a value JSON cannot carry —
		// NaN, an infinity — is a bad request, not a server fault.
		return in, Errorf(400, "Bad Request")
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return in, Errorf(400, "Bad Request")
	}
	return in, nil
}

// encodeValue is the reverse round-trip: a typed Go value becomes the plain
// tree devalue.Stringify expects.
func encodeValue(v any) (any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("skgo: encoding remote result: %w", err)
	}
	var tree any
	if err := json.Unmarshal(raw, &tree); err != nil {
		return nil, fmt.Errorf("skgo: encoding remote result: %w", err)
	}
	return tree, nil
}

// RemoteConfig describes the app the registry is serving. Everything but
// Origin comes straight from the built manifest.
type RemoteConfig struct {
	// AppDir is kit's appDir; empty means "_app".
	AppDir string
	// Base is kit's paths.base, without a trailing slash.
	Base string
	// Version, when non-empty, is sent as the `x-sveltekit-version` response
	// header. It must equal the version baked into the client — a different
	// value makes the client force a reload — so leave it empty whenever the
	// client did not come from this build (dev mode).
	Version string
	// Origin is the app's configured origin, e.g. "http://127.0.0.1:8080".
	// When non-empty, a non-GET remote request from a different origin is
	// refused with 403. Empty disables the check, which is what kit does in
	// dev.
	Origin string
	// Dev reports that the client is being served by a `vp dev` server rather
	// than by this build. It relaxes the cookie `secure` default exactly as
	// kit's own `__SVELTEKIT_DEV__` does, and it turns off the check against
	// Manifest.Remotes, which describes the last production build and not the
	// files vite is serving.
	Dev bool
	// CookieOrigin is the origin whose scheme and host decide the `secure`
	// cookie default. It defaults to Origin.
	CookieOrigin string
	// Remotes is the set of `<hash>/<name>` ids the built frontend calls, as
	// recorded in the manifest. NewRemotes refuses to build a registry that
	// does not answer exactly these.
	Remotes []string
	// OnPanic is called when a remote function panics, with the function's
	// `<hash>/<name>` id, the recovered value, and the stack. The client is
	// told nothing but an opaque 500, so this is the only record the panic
	// leaves; leaving it nil logs the same three things to the standard
	// logger, because a panicking handler that reports nowhere is a bug that
	// cannot be found.
	OnPanic func(id string, value any, stack []byte)

	// manifest reports that this config came from a build manifest, which is
	// what makes the Remotes check meaningful. A hand-built config is not
	// checked.
	manifest bool
}

// RemoteConfig derives a registry configuration from a build manifest.
func (m Manifest) RemoteConfig(origin string) RemoteConfig {
	return RemoteConfig{
		AppDir:   m.AppDir,
		Base:     m.Base,
		Version:  m.Version,
		Origin:   origin,
		Remotes:  m.Remotes,
		manifest: true,
	}
}

// ReadManifest parses `skgo.manifest.json` from an adapter build.
func ReadManifest(build fs.FS) (Manifest, error) {
	var m Manifest
	raw, err := fs.ReadFile(build, "skgo.manifest.json")
	if err != nil {
		return m, fmt.Errorf("skgo: reading skgo.manifest.json from the build: %w", err)
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return m, fmt.Errorf("skgo: parsing skgo.manifest.json: %w", err)
	}
	if m.AppDir == "" {
		m.AppDir = "_app"
	}
	return m, nil
}

// Remotes is a registry of remote functions and the http.Handler that answers
// calls to them.
type Remotes struct {
	cfg           RemoteConfig
	prefix        string
	fns           map[string]*Remote
	secureCookies bool
}

// NewRemotes builds a registry. Two functions with the same module and export
// name are a programming error and are reported as such.
func NewRemotes(cfg RemoteConfig, fns ...*Remote) (*Remotes, error) {
	if cfg.AppDir == "" {
		cfg.AppDir = "_app"
	}
	base := strings.TrimSuffix(cfg.Base, "/")
	if base != "" && !strings.HasPrefix(base, "/") {
		base = "/" + base
	}
	cfg.Base = base

	cookieOrigin := cfg.CookieOrigin
	if cookieOrigin == "" {
		cookieOrigin = cfg.Origin
	}

	rs := &Remotes{
		cfg:           cfg,
		prefix:        base + "/" + cfg.AppDir + "/remote/",
		fns:           make(map[string]*Remote, len(fns)),
		secureCookies: secureCookieDefault(cookieOrigin, cfg.Dev),
	}
	for _, fn := range fns {
		if fn == nil {
			return nil, errors.New("skgo: nil remote function")
		}
		if existing, dup := rs.fns[fn.id]; dup {
			return nil, fmt.Errorf("skgo: remote id %s is registered twice (%s#%s and %s#%s)",
				fn.id, existing.module, existing.name, fn.module, fn.name)
		}
		rs.fns[fn.id] = fn
	}
	if err := rs.checkDrift(); err != nil {
		return nil, err
	}
	return rs, nil
}

// checkDrift refuses to build a registry whose functions are not the ones the
// built frontend calls. The manifest's remote list is written during `vp build`
// from the ids `skgo generate` emitted, so a Go binary whose registry disagrees
// with it is serving a client that will get 404s from half its remote calls —
// or, worse, silently stale behaviour. Better to not start.
func (rs *Remotes) checkDrift() error {
	if rs.cfg.Dev || !rs.cfg.manifest {
		return nil
	}
	if rs.cfg.Remotes == nil {
		return errors.New("skgo: the build has no remote-function list; re-run `skgo generate` and rebuild the frontend")
	}

	built := map[string]bool{}
	for _, id := range rs.cfg.Remotes {
		built[id] = true
	}

	var missing, extra []string
	for id := range built {
		if _, ok := rs.fns[id]; !ok {
			missing = append(missing, id)
		}
	}
	for id, fn := range rs.fns {
		if !built[id] {
			extra = append(extra, id+" ("+fn.module+"#"+fn.name+")")
		}
	}
	if len(missing) == 0 && len(extra) == 0 {
		return nil
	}
	sort.Strings(missing)
	sort.Strings(extra)

	msg := "skgo: the built frontend and this binary disagree about the remote functions."
	if len(missing) > 0 {
		msg += "\n  the frontend calls, but this binary does not serve: " + strings.Join(missing, ", ")
	}
	if len(extra) > 0 {
		msg += "\n  this binary serves, but the frontend does not call: " + strings.Join(extra, ", ")
	}
	msg += "\n  run `go generate ./...` and rebuild the frontend, then rebuild this binary."
	return errors.New(msg)
}

// Prefix is the URL prefix every remote call lives under, including the
// trailing slash — normally "/_app/remote/".
func (rs *Remotes) Prefix() string { return rs.prefix }

// Lookup returns the function registered under `<hash>/<name>`.
func (rs *Remotes) Lookup(id string) (*Remote, bool) {
	fn, ok := rs.fns[id]
	return fn, ok
}

// Intercept returns a handler that answers remote calls itself and passes
// everything else to next. It must sit in front of both the dev proxy (or
// kit's dev server would run its throwing stub) and the static handler.
func (rs *Remotes) Intercept(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, rs.prefix) {
			rs.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (rs *Remotes) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Kit refuses cross-site remote calls before it even looks the id up, and
	// the refusal is a plain 403 rather than a remote-function envelope.
	if rs.cfg.Origin != "" && r.Method != http.MethodGet && r.Header.Get("Origin") != rs.cfg.Origin {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "private, no-store")
		w.WriteHeader(http.StatusForbidden)
		writeJSON(w, map[string]string{"message": "Cross-site remote requests are forbidden"})
		return
	}

	rest := strings.TrimPrefix(r.URL.Path, rs.prefix)
	parts := strings.Split(rest, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		rs.writeError(w, notFound())
		return
	}

	fn, ok := rs.fns[parts[0]+"/"+parts[1]]
	if !ok {
		rs.writeError(w, notFound())
		return
	}

	switch fn.kind {
	case kindQuery:
		rs.serveQuery(w, r, fn)
	case kindLive:
		rs.serveLive(w, r, fn)
	case kindCommand:
		rs.serveCommand(w, r, fn)
	case kindForm:
		rs.serveForm(w, r, fn)
	}
}

// notFound reproduces the body kit's `error(404)` produces.
func notFound() *HTTPError { return &HTTPError{Status: 404, Message: "Error: 404"} }

func (rs *Remotes) serveQuery(w http.ResponseWriter, r *http.Request, fn *Remote) {
	if r.Method != http.MethodGet {
		rs.writeErrorStatus(w, &HTTPError{
			Status:  405,
			Message: "`query` functions must be invoked via GET request, not " + r.Method,
		}, http.StatusMethodNotAllowed)
		return
	}

	payload := rawPayload(r.URL)
	arg, present, err := remotearg.ParsePayload(payload)
	if err != nil {
		rs.writeError(w, &HTTPError{Status: 400, Message: "Bad Request"})
		return
	}

	ev := rs.newEvent(r, false)

	value, err := rs.call(withEvent(r.Context(), ev), fn, arg, present)
	if err != nil {
		if redirect := asRedirect(err); redirect != nil {
			rs.writeResult(w, ev, map[string]any{"redirect": redirect.Location})
			return
		}
		rs.writeError(w, asHTTPError(err))
		return
	}

	// The client reads a query's value out of `q[key].v`, not out of `_`; a
	// response carrying only `_` hydrates as undefined. The key uses the raw
	// payload string exactly as it arrived.
	key := fn.id + "/" + payload
	rs.writeResult(w, ev, map[string]any{
		"_": value,
		"q": map[string]any{key: map[string]any{"v": value}},
	})
}

func (rs *Remotes) serveCommand(w http.ResponseWriter, r *http.Request, fn *Remote) {
	if r.Method != http.MethodPost {
		rs.writeErrorStatus(w, &HTTPError{
			Status:  405,
			Message: "`command` functions must be invoked via POST request, not " + r.Method,
		}, http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Payload   string   `json:"payload"`
		Refreshes []string `json:"refreshes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		rs.writeError(w, &HTTPError{Status: 400, Message: "Bad Request"})
		return
	}

	arg, present, err := remotearg.ParsePayload(body.Payload)
	if err != nil {
		rs.writeError(w, &HTTPError{Status: 400, Message: "Bad Request"})
		return
	}

	ev := rs.newEvent(r, true)

	value, err := rs.call(withEvent(r.Context(), ev), fn, arg, present)
	if err != nil {
		// A redirect in a command response makes the client throw, so it is
		// reported as an ordinary error instead. Cookies written before the
		// failure are dropped with it.
		rs.writeError(w, asHTTPError(err))
		return
	}

	// Single-flight refreshes run after the command, on the same event, so a
	// query refreshed by a command that just signed the visitor in reads the
	// new cookie — exactly as it does in kit, where both share one request.
	data := map[string]any{"_": value}
	q, l := rs.resolveRefreshes(withEvent(r.Context(), ev.immutable()), body.Refreshes)
	if len(q) > 0 {
		data["q"] = q
	}
	if len(l) > 0 {
		data["l"] = l
	}
	if len(q) > 0 || len(l) > 0 {
		// `r` says the server performed explicit single-flight updates. Kit's
		// server sets it for any refresh, so skgo does too, but it is inert on
		// a command: only `form.svelte.js` reads it, to skip the `refreshAll`
		// an unenhanced submission would otherwise run. `command.svelte.js`
		// never invalidates anything — a kit command updates exactly what the
		// server put in `q` and `l` and nothing else.
		data["r"] = true
	}
	rs.writeResult(w, ev, data)
}

// resolveRefreshes runs every refresh key that names a registered query or live
// query. Unrecognised keys are skipped in silence, as kit does.
//
// It returns two maps because kit's client reads them differently: a `q` entry
// replaces a query's value, while an `l` entry seeds a live query's value and
// then tears the stream down and reopens it
// (`runtime/client/remote-functions/shared.svelte.js`). Reconnecting is the
// only way a live query can pick up a cookie the command just wrote — kit's
// event is a snapshot of the request that opened the stream and never
// refreshes — and it is what kit documents for exactly that case.
func (rs *Remotes) resolveRefreshes(ctx context.Context, keys []string) (q, l map[string]any) {
	q, l = map[string]any{}, map[string]any{}
	for _, key := range keys {
		// The payload can itself contain no slash, but the id always holds
		// exactly one, so the split is on the LAST slash.
		i := strings.LastIndex(key, "/")
		if i < 0 {
			continue
		}
		id, payload := key[:i], key[i+1:]

		fn, ok := rs.fns[id]
		if !ok || fn.kind == kindCommand {
			continue
		}
		arg, present, err := remotearg.ParsePayload(payload)
		if err != nil {
			continue
		}

		// A refresh runs after the command has already succeeded — and may
		// already have written a cookie — so a panic in one of them becomes
		// that entry's error and nothing more. The command keeps its answer.
		into, result := q, any(nil)
		if fn.kind == kindLive {
			into = l
			result, err = rs.firstValue(ctx, fn, arg, present)
		} else {
			result, err = rs.call(ctx, fn, arg, present)
		}
		if err != nil {
			into[key] = map[string]any{"e": errorNode(asHTTPError(err))}
			continue
		}
		into[key] = map[string]any{"v": result}
	}
	return q, l
}

// errFirstValueTaken stops a live producer once its first value is in hand. It
// never reaches the caller.
var errFirstValueTaken = errors.New("skgo: first value taken")

// firstValue runs a live query far enough to yield once and then stops it,
// mirroring kit's `get_first_value`, which consumes a single value from the
// generator and closes the iterator. The producer sees a cancelled context, so
// a `select` on ctx.Done() unwinds exactly as it does on a client disconnect.
func (rs *Remotes) firstValue(ctx context.Context, r *Remote, arg any, present bool) (value any, err error) {
	defer func() { err = rs.recovered(r, recover(), err) }()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var got bool
	err = r.live(ctx, arg, present, func(v any) error {
		value, got = v, true
		cancel()
		return errFirstValueTaken
	})
	if got {
		return value, nil
	}
	if err != nil && !errors.Is(err, errFirstValueTaken) {
		return nil, err
	}
	return nil, Errorf(500, "skgo: live query %s produced no value", r.id)
}

// rawPayload returns the payload parameter exactly as sent. The key the client
// caches under is built from this string, so it must never be re-encoded.
func rawPayload(u *url.URL) string {
	return u.Query().Get("payload")
}

// call runs a remote function, turning a panic into the error every path here
// already knows how to answer.
//
// A remote function is ordinary Go and one of them will panic. Unrecovered,
// the panic escapes ServeHTTP, net/http drops the connection, and the browser
// reports a network failure rather than the app's error page — one bad handler
// reads as an outage. Kit answers an unexpected throw with the same opaque 500
// it gives any unexpected error, and so does this: the panic's text is the
// server's business, and asHTTPError renders anything that is not an
// *HTTPError as `{"status":500,"message":"Internal Error"}`.
func (rs *Remotes) call(ctx context.Context, fn *Remote, arg any, present bool) (v any, err error) {
	defer func() { err = rs.recovered(fn, recover(), err) }()
	return fn.call(ctx, arg, present)
}

// callLive is the same guard for a `query.live` producer. It matters more
// there: the producer runs on its own goroutine, where an unrecovered panic
// does not reset one connection but ends the process, taking every other
// visitor with it.
func (rs *Remotes) callLive(ctx context.Context, fn *Remote, arg any, present bool, yield func(any) error) (err error) {
	defer func() { err = rs.recovered(fn, recover(), err) }()
	return fn.live(ctx, arg, present, yield)
}

// recovered reports a panic and converts it to an error. It is a no-op when
// nothing panicked, so the guards above read as one deferred line.
func (rs *Remotes) recovered(fn *Remote, value any, err error) error {
	if value == nil {
		return err
	}
	// net/http panics with ErrAbortHandler to abandon a response on purpose.
	// Swallowing it would turn a deliberate abort into a 500 the client reads
	// as a real answer, so it goes back up untouched.
	if value == http.ErrAbortHandler {
		panic(value)
	}

	stack := debug.Stack()
	if rs.cfg.OnPanic != nil {
		rs.cfg.OnPanic(fn.id, value, stack)
	} else {
		log.Printf("skgo: remote function %s panicked: %v\n%s", fn.id, value, stack)
	}
	return &HTTPError{Status: 500, Message: "Internal Error"}
}

// newEvent builds the per-call handle. The cookie jar it carries is shared by
// the command and by every query the command's `refreshes` list resolves, so a
// refreshed query reads the cookie the command just wrote — kit resolves both
// on one request, and so does this.
func (rs *Remotes) newEvent(r *http.Request, mutable bool) *Event {
	return &Event{req: r, jar: newCookieJar(r, rs.secureCookies), mutable: mutable}
}

// immutable derives the event a query gets, mirroring kit's
// `derive_remote_function_event(event, state, false)`.
func (e *Event) immutable() *Event {
	derived := *e
	derived.mutable = false
	return &derived
}

func (rs *Remotes) header(w http.ResponseWriter) http.Header {
	h := w.Header()
	h.Set("Cache-Control", "private, no-store")
	if rs.cfg.Version != "" {
		h.Set("X-Sveltekit-Version", rs.cfg.Version)
	}
	return h
}

func (rs *Remotes) writeResult(w http.ResponseWriter, ev *Event, data map[string]any) {
	serialized, err := devalue.Stringify(data)
	if err != nil {
		rs.writeError(w, &HTTPError{Status: 500, Message: "Internal Error"})
		return
	}
	h := rs.header(w)
	if ev != nil {
		ev.jar.writeTo(h)
	}
	h.Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	writeJSON(w, remoteResponse{Type: "result", Data: serialized})
}

func (rs *Remotes) writeError(w http.ResponseWriter, e *HTTPError) {
	// Kit answers runtime remote errors with HTTP 200 and an error envelope;
	// the client reads the envelope either way.
	rs.writeErrorStatus(w, e, http.StatusOK)
}

func (rs *Remotes) writeErrorStatus(w http.ResponseWriter, e *HTTPError, status int) {
	h := rs.header(w)
	h.Set("Content-Type", "application/json")
	w.WriteHeader(status)
	writeJSON(w, remoteResponse{Type: "error", Error: e})
}

type remoteResponse struct {
	Type  string     `json:"type"`
	Data  string     `json:"data,omitempty"`
	Error *HTTPError `json:"error,omitempty"`
}

func errorNode(e *HTTPError) *devalue.Object {
	// devalue serialises the JSON value set only, so the status must be a
	// float, not a Go int. The object is ordered rather than a Go map, so the
	// two properties land in kit's own order.
	return devalue.NewObject("status", float64(e.Status), "message", e.Message)
}

func asHTTPError(err error) *HTTPError {
	var e *HTTPError
	if errors.As(err, &e) {
		return e
	}
	return &HTTPError{Status: 500, Message: "Internal Error"}
}

func asRedirect(err error) *Redirect {
	var r *Redirect
	if errors.As(err, &r) {
		return r
	}
	return nil
}

// jsonBytes serialises without HTML escaping, so devalue payloads survive
// verbatim, and without encoding/json's trailing newline.
func jsonBytes(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

func writeJSON(w http.ResponseWriter, v any) {
	if raw, err := jsonBytes(v); err == nil {
		w.Write(raw)
	}
}
