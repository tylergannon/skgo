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

	"github.com/tylergannon/polytype/devalue"
	"github.com/tylergannon/skgo/internal/adapter"
	"github.com/tylergannon/skgo/internal/formdata"
	"github.com/tylergannon/skgo/internal/kithash"
	"github.com/tylergannon/skgo/internal/remotearg"
)

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

// Kind is which of kit's four remote-function kinds a registration publishes.
// `query.batch` is a fifth constant rather than a query because the call shape
// differs — one POST carrying every payload — even though everything
// downstream files its results in the browser's ordinary query cache.
type Kind int

const (
	KindQuery Kind = iota
	KindCommand
	KindLive
	KindBatch
	KindForm
)

// Call is one remote call as it reaches the generated closure that answers it.
//
// Arg is the argument exactly as it came off the wire: the devalue value model
// tree kit's client sent, before anything has decided what Go type it is.
// Deciding that is the job of the strict decoder polytype generated for this
// function's own parameter type, and of nothing else. That decoder lives in the
// generated bindings package with the closure that calls it, which is why there
// is no type parameter here and no Go value of the app's own types passing
// through this struct.
type Call struct {
	// Arg is the devalue value model tree the client sent — devalue's own
	// shapes: nil, bool, float64, string, []any, *devalue.Object, and for a
	// form submission the uploaded files kit's binary envelope carried.
	Arg any
	// Present reports that the client sent an argument at all. Kit's client
	// calls a function declared without one with `undefined`, whose payload is
	// the empty string, so absence is a value in its own right: kit's own
	// `create_validator` answers 400 to an argument a no-argument function was
	// never declared to take.
	Present bool

	// transport is the app's `transport` hook, for the one result a generated
	// encoder cannot produce. See Transported.
	transport Transport
}

// Transported encodes a result whose type can reach a type the app transports.
//
// It is not a fallback but kit's own rule: a transported value has to reach
// devalue *as itself*, so that the reducer the `transport` hook installs still
// recognises it and writes `["Money", …]`. polytype's generated encoder would
// already have flattened it into an object, and the browser would be handed a
// plain object with no methods — which is the bug the hook exists to prevent.
// So a value that can hold one travels unflattened and devalue finishes it.
//
// `skgo generate` emits this call for exactly the functions whose result type
// reaches a transported type, and a generated encoder for every other one.
func (c Call) Transported(v any) (any, error) { return c.transport.encodeTree(v) }

// RemoteFunc is the generated closure that answers one query, command or form:
// it decodes call.Arg with the decoder polytype generated for the function's
// own parameter type, calls the app's Go function, and returns the devalue
// value model tree its generated encoder produced.
type RemoteFunc func(ctx context.Context, call Call) (any, error)

// LiveFunc is RemoteFunc for a `query.live`. Each value the app's function
// yields is encoded by the generated encoder before it reaches yield, so the
// frames this produces are the same bytes a query's response carries.
type LiveFunc func(ctx context.Context, call Call, yield func(any) error) error

// BatchFunc is RemoteFunc for a `query.batch`: every argument kit's client
// collected in one macrotask, answered in one call, in order.
type BatchFunc func(ctx context.Context, calls []Call) ([]any, error)

// RemoteSpec is one generated registration. `skgo generate` writes these; an
// application declares its functions with Query, Command, LiveQuery,
// BatchQuery and Form and writes none of this by hand.
type RemoteSpec struct {
	// Kind is which of kit's remote-function kinds this publishes.
	Kind Kind
	// Module is the vite-root-relative path of the `.remote.ts` kit compiles,
	// for example "src/routes/todos/todos.remote.ts". Kit hashes it into the
	// id its client addresses the function by.
	Module string
	// Name is the export name within that module.
	Name string
	// Fn is the app's own function value. Nothing calls it through this field:
	// it is here for its code pointer, which is the identity skgo.Refresh
	// looks a query up by, and it is the same function value the closure below
	// calls. That is what lets a refresh name `getTodo` rather than a string.
	Fn any
	// Call answers a query, command, batch-of-one or form. Required for every
	// kind but KindLive.
	Call RemoteFunc
	// Live answers a `query.live`. Required for KindLive and nothing else.
	Live LiveFunc
	// Batch answers a `query.batch`. Required for KindBatch and nothing else.
	Batch BatchFunc
	// DecodeArg decodes one argument into the function's own parameter type,
	// for skgo.Requested — the one place a handler is handed a client's
	// argument rather than the server running it. The value it returns is of
	// the function's parameter type; it is typed `any` because the caller's
	// type parameter comes from the app's own function signature and this
	// closure is generated beside the function, not here. It is nil for a
	// function declared without an argument.
	DecodeArg func(arg any) (any, error)
}

// Remote is one registered remote function. `skgo generate` builds these with
// NewRemote; application code declares the functions and marks them with
// Query, Command, LiveQuery, BatchQuery and Form.
type Remote struct {
	module string
	name   string
	hash   string
	id     string
	kind   Kind

	call  RemoteFunc
	live  LiveFunc
	batch BatchFunc
	// argDecoder is RemoteSpec.DecodeArg. See its documentation for why the
	// value it produces is spelled `any` here and typed at the caller.
	argDecoder func(arg any) (any, error)

	// ptr is the code pointer of the Go function this registration publishes.
	// It is the identity skgo.Refresh looks a function up by, which is what
	// lets a refresh name `getTodo` rather than a string.
	ptr uintptr
}

// ID is the `<hash>/<name>` pair the client addresses this function by.
func (r *Remote) ID() string { return r.id }

// Module is the vite-root-relative path of the `.remote.ts` module.
func (r *Remote) Module() string { return r.module }

// Name is the export name within that module.
func (r *Remote) Name() string { return r.name }

// Kind is which of kit's remote-function kinds this registration publishes.
func (r *Remote) Kind() Kind { return r.kind }

// NewRemote builds a registration from a generated spec.
//
// It panics on a spec that cannot answer a call, because a spec is generated
// code: a missing closure is a bug in the generator, it is the same bug on
// every request, and the first thing an app does at startup is build its
// registry. Failing there names the function; failing later would be a nil
// call in a handler.
func NewRemote(spec RemoteSpec) *Remote {
	hash := kithash.Kit(spec.Module)
	r := &Remote{
		module:     spec.Module,
		name:       spec.Name,
		hash:       hash,
		id:         hash + "/" + spec.Name,
		kind:       spec.Kind,
		call:       spec.Call,
		live:       spec.Live,
		batch:      spec.Batch,
		argDecoder: spec.DecodeArg,
		ptr:        codePointer(spec.Fn),
	}
	switch spec.Kind {
	case KindLive:
		if spec.Live == nil {
			panic("skgo: the generated registration for " + r.id + " has no Live closure")
		}
	case KindBatch:
		if spec.Batch == nil {
			panic("skgo: the generated registration for " + r.id + " has no Batch closure")
		}
		// One argument is a batch of one. It is the shape a Refresh of a
		// single instance takes, and the shape kit's own `refresh()` takes
		// too: it stores the resource's promise, which went through `enqueue`
		// and became a batch with one entry in it.
		batch := spec.Batch
		r.call = func(ctx context.Context, call Call) (any, error) {
			outs, err := batch(ctx, []Call{call})
			if err != nil {
				return nil, err
			}
			if len(outs) != 1 {
				return nil, fmt.Errorf("skgo: batch query %s answered %d results for one argument", r.id, len(outs))
			}
			return outs[0], nil
		}
	default:
		if spec.Call == nil {
			panic("skgo: the generated registration for " + r.id + " has no Call closure")
		}
	}
	return r
}

// Marker is what the declaration helpers return. It carries nothing: Query,
// Command, LiveQuery and Form exist to be read by `skgo generate`.
type Marker struct{}

// Query declares fn as a SvelteKit `query`. Write it beside the function, in a
// file named `*.remote.go`:
//
//	func getTodos(ctx context.Context) ([]Todo, error) { ... }
//
//	var _ = skgo.Query(getTodos)
//
// A query that takes an argument declares it as an ordinary second parameter:
//
//	func getTodo(ctx context.Context, id string) (Todo, error) { ... }
//
// Both are ordinary Go functions, which is kit's own rule rather than a
// liberty taken here: `query(fn)` accepts `(arg?) => Output`, so the argument
// is optional and there is no placeholder to name in its place.
//
// fn is `any` because the marker checks nothing. `skgo generate` reads the
// function's real signature through go/types — it has to, since it projects
// the argument and the result to TypeScript — and reports a shape it cannot
// publish against the declaration's own position. What checks the function is
// the generated closure that calls it, which names the argument's type and the
// result's and is compiled beside the codecs polytype emitted for exactly
// those two types.
//
// `skgo generate` emits the sibling `.remote.ts` kit compiles and the Go
// registration that answers the calls. The request is reachable with
// skgo.EventFrom(ctx); a query may read cookies but not write them, which is
// the restriction kit places on its own queries.
func Query(fn any) Marker { _ = fn; return Marker{} }

// Command declares fn as a SvelteKit `command`. A command's event may write
// cookies; kit allows that in commands and forms and nowhere else.
//
// Like a query, it takes `(ctx)` or `(ctx, arg)`. See Query for why fn is any.
func Command(fn any) Marker { _ = fn; return Marker{} }

// LiveQuery declares fn as a SvelteKit `query.live`. fn pushes values with
// yield and returns when the subscription ends; its context is cancelled when
// the client disconnects:
//
//	func watchCount(ctx context.Context, yield func(int) error) error { ... }
//
// An argument, when there is one, sits between the two. See Query for why fn
// is any.
func LiveQuery(fn any) Marker { _ = fn; return Marker{} }

// BatchQuery declares fn as a SvelteKit `query.batch`. fn takes every argument
// collected in one macrotask and answers them all in one call:
//
//	func getQuotes(ctx context.Context, symbols []string) ([]Quote, error) { ... }
//
// The page still calls it one argument at a time — `getQuotes('SKGO')` — and
// kit's client collects those calls into a single POST, exactly as kit's own
// `query.batch` does. fn must answer with one result per argument, in the same
// order.
//
// A batch query always takes an argument: kit's `batch(fn)` with no validator
// refuses any call that passes one, which leaves nothing to batch. See Query
// for why fn is any.
func BatchQuery(fn any) Marker { _ = fn; return Marker{} }

// BadRequest turns a generated decoder's refusal into the answer kit gives.
//
// Kit's `create_validator` answers 400 to an argument its schema rejects, and
// to any argument at all when the function was declared without one. skgo's
// schema is the strict decoder polytype generated from the Go parameter type,
// so this is the same refusal at the same moment.
//
// What the client is told is kit's opaque "Bad Request". The decoder's own
// diagnostic — which names a JSON-pointer path into the argument — is kept on
// the error for the server's logs and never put on the wire, because it
// describes the shape the server expected and an unauthenticated caller has
// not earned that.
func BadRequest(detail error) error { return badRequest{detail: detail} }

type badRequest struct{ detail error }

func (b badRequest) Error() string {
	if b.detail == nil {
		return "skgo: bad request"
	}
	return "skgo: bad request: " + b.detail.Error()
}

// Unwrap makes this the 400 every path here already knows how to answer, while
// Error keeps the detail for whoever logs it.
func (b badRequest) Unwrap() error { return &HTTPError{Status: 400, Message: "Bad Request"} }

// RefuseArgument is the decode step for a function declared without an
// argument, and it is kit's own `create_validator` with no schema: such a
// function answers 400 to any argument at all. Generated code calls it so that
// the refusal is written in the same place every other decode is.
func RefuseArgument(call Call) error {
	if call.Present {
		return &HTTPError{Status: 400, Message: "Bad Request"}
	}
	return nil
}

// RequireArgument is the other half of RefuseArgument: a function declared
// *with* an argument was called without one, so there is nothing for its
// decoder to admit and no zero value it would be honest to invent. Kit's
// schema-validated functions refuse the same call.
func RequireArgument(call Call) error {
	if !call.Present {
		return &HTTPError{Status: 400, Message: "Bad Request"}
	}
	return nil
}

// DecodeForm assigns a form submission onto the handler's argument type.
//
// A form is the one remote kind whose argument is not a devalue payload: a
// hydrated kit page posts `application/x-sveltekit-formdata`, whose POJO can
// carry an uploaded File. A File is not a JSON value and polytype does not
// describe one, so the generated closure calls this instead of a generated
// decoder; into is a pointer to the handler's own argument type, so the shape
// is still fixed by the Go signature and never by a type parameter here.
func DecodeForm(arg any, into any) error {
	if err := formdata.Decode(arg, into); err != nil {
		return &HTTPError{Status: 400, Message: "Bad Request"}
	}
	return nil
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
	// Transport is the app's `transport` hook: the Go half of the encode/decode
	// pairs `src/hooks.ts` declares. It is optional; without it a custom type
	// goes out as whatever encoding/json makes of it, which is a plain object
	// with no methods on the other side. See the Transport type.
	Transport Transport
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
	if m.SkgoAdapter != adapter.Fingerprint() {
		return Manifest{}, fmt.Errorf(
			"skgo: the frontend build and this program come from different skgo versions.\n"+
				"\tthe skgo-adapter.js that built it: %s\n"+
				"\tthis program:                      %s\n"+
				"The adapter and the Go that reads what it writes are one contract, so they have to be the same skgo.\n"+
				"`skgo generate` writes web/skgo-adapter.js out of the module it was built from, so run `go generate ./...` and build the frontend again.",
			adapter.Identity(m.Skgo, m.SkgoAdapter), adapter.Identity(adapter.Version(), adapter.Fingerprint()))
	}
	if m.AppDir == "" {
		m.AppDir = "_app"
	}
	return m, nil
}

// Remotes is a registry of remote functions and the http.Handler that answers
// calls to them.
type Remotes struct {
	cfg    RemoteConfig
	prefix string
	fns    map[string]*Remote
	// byFunc indexes the registrations by the code pointer of the Go function
	// each one publishes, which is how skgo.Refresh turns `getTodo` into the
	// id half of a refresh key.
	byFunc        map[uintptr]*Remote
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

	if err := cfg.Transport.validate(); err != nil {
		return nil, err
	}

	rs := &Remotes{
		cfg:           cfg,
		prefix:        base + "/" + cfg.AppDir + "/remote/",
		fns:           make(map[string]*Remote, len(fns)),
		byFunc:        make(map[uintptr]*Remote, len(fns)),
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
		if fn.ptr == 0 {
			continue
		}
		if existing, dup := rs.byFunc[fn.ptr]; dup {
			// Two registrations that cannot be told apart by their function
			// value would make skgo.Refresh update whichever one won the map,
			// silently and not always the same one. Go gives two named
			// top-level functions distinct code pointers even when their
			// bodies are identical, so this is a registry built some other
			// way — and it has to stop here rather than at the refresh.
			return nil, fmt.Errorf("skgo: %s and %s are the same Go function (%s), so a refresh could not tell them apart",
				existing.id, fn.id, funcNameAt(fn.ptr))
		}
		rs.byFunc[fn.ptr] = fn
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
	case KindQuery:
		rs.serveQuery(w, r, fn)
	case KindLive:
		rs.serveLive(w, r, fn)
	case KindBatch:
		rs.serveBatch(w, r, fn)
	case KindCommand:
		rs.serveCommand(w, r, fn)
	case KindForm:
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
	arg, present, err := remotearg.ParsePayloadWith(payload, rs.codecs())
	if err != nil {
		rs.writeError(w, &HTTPError{Status: 400, Message: "Bad Request"})
		return
	}

	ev := rs.newEvent(r, false)

	value, err := rs.call(withEvent(r.Context(), ev), fn, rs.newCall(arg, present))
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

	arg, present, err := remotearg.ParsePayloadWith(body.Payload, rs.codecs())
	if err != nil {
		rs.writeError(w, &HTTPError{Status: 400, Message: "Bad Request"})
		return
	}

	ev := rs.newEvent(r, true)
	// The list the browser posted is indexed, not obeyed: nothing in it runs
	// until the handler names the query it belongs to. See requested.go.
	ev.refreshes = newRefreshSet(rs, body.Refreshes)

	value, err := rs.call(withEvent(r.Context(), ev), fn, rs.newCall(arg, present))
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
	q, l := rs.collectRefreshes(r.Context(), ev)
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

// errFirstValueTaken stops a live producer once its first value is in hand. It
// never reaches the caller.
var errFirstValueTaken = errors.New("skgo: first value taken")

// firstValue runs a live query far enough to yield once and then stops it,
// mirroring kit's `get_first_value`, which consumes a single value from the
// generator and closes the iterator. The producer sees a cancelled context, so
// a `select` on ctx.Done() unwinds exactly as it does on a client disconnect.
func (rs *Remotes) firstValue(ctx context.Context, r *Remote, call Call) (value any, err error) {
	defer func() { err = rs.recovered(r, recover(), err) }()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var got bool
	err = r.live(ctx, call, func(v any) error {
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
func (rs *Remotes) call(ctx context.Context, fn *Remote, call Call) (v any, err error) {
	defer func() { err = rs.recovered(fn, recover(), err) }()
	// What comes back is already the tree devalue serializes: the generated
	// closure encoded it, with the codec polytype emitted for this function's
	// own result type. Nothing here inspects a Go value of the app's types.
	return fn.call(ctx, call)
}

// newCall packages one call's argument for the generated closure that answers
// it, together with the app's transport hook — the one thing a closure needs
// from the registry that its own types cannot give it. See Call.Transported.
func (rs *Remotes) newCall(arg any, present bool) Call {
	return Call{Arg: arg, Present: present, transport: rs.cfg.Transport}
}

// callLive is the same guard for a `query.live` producer. It matters more
// there: the producer runs on its own goroutine, where an unrecovered panic
// does not reset one connection but ends the process, taking every other
// visitor with it.
func (rs *Remotes) callLive(ctx context.Context, fn *Remote, call Call, yield func(any) error) (err error) {
	defer func() { err = rs.recovered(fn, recover(), err) }()
	// The generated closure encodes each value as it is yielded, so what
	// reaches yield here is already the tree devalue serializes.
	return fn.live(ctx, call, yield)
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
	serialized, err := devalue.StringifyWith(data, rs.cfg.Transport.reducers())
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
