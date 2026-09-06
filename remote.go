// Remote-function dispatch: the Go side of SvelteKit's `query`, `query.live`
// and `command`.
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
	"net/http"
	"net/url"
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
type Redirect struct {
	Location string
}

func (r *Redirect) Error() string { return "redirect to " + r.Location }

type remoteKind int

const (
	kindQuery remoteKind = iota
	kindCommand
	kindLive
)

// Remote is one registered remote function. Build one with Query, Command or
// LiveQuery and hand it to NewRemotes.
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

// Query registers a `query` export. module is the module's vite-root-relative
// path (for example "src/lib/todos.remote.ts") and name the export name.
func Query[In, Out any](module, name string, fn func(ctx context.Context, in In) (Out, error)) *Remote {
	r := newRemote(module, name, kindQuery)
	r.call = callAdapter(fn)
	return r
}

// Command registers a `command` export.
func Command[In, Out any](module, name string, fn func(ctx context.Context, in In) (Out, error)) *Remote {
	r := newRemote(module, name, kindCommand)
	r.call = callAdapter(fn)
	return r
}

// LiveQuery registers a `query.live` export. fn pushes values with yield and
// returns when the subscription ends; its context is cancelled when the client
// disconnects, and yield returns a non-nil error once that has happened.
func LiveQuery[In, Out any](module, name string, fn func(ctx context.Context, in In, yield func(Out) error) error) *Remote {
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

func callAdapter[In, Out any](fn func(ctx context.Context, in In) (Out, error)) func(context.Context, any, bool) (any, error) {
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
		return in, fmt.Errorf("skgo: encoding remote argument: %w", err)
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
}

// RemoteConfig derives a registry configuration from a build manifest.
func (m Manifest) RemoteConfig(origin string) RemoteConfig {
	return RemoteConfig{
		AppDir:  m.AppDir,
		Base:    m.Base,
		Version: m.Version,
		Origin:  origin,
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
	cfg    RemoteConfig
	prefix string
	fns    map[string]*Remote
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

	rs := &Remotes{
		cfg:    cfg,
		prefix: base + "/" + cfg.AppDir + "/remote/",
		fns:    make(map[string]*Remote, len(fns)),
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
	return rs, nil
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

	value, err := fn.call(r.Context(), arg, present)
	if err != nil {
		if redirect := asRedirect(err); redirect != nil {
			rs.writeResult(w, map[string]any{"redirect": redirect.Location})
			return
		}
		rs.writeError(w, asHTTPError(err))
		return
	}

	// The client reads a query's value out of `q[key].v`, not out of `_`; a
	// response carrying only `_` hydrates as undefined. The key uses the raw
	// payload string exactly as it arrived.
	key := fn.id + "/" + payload
	rs.writeResult(w, map[string]any{
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

	value, err := fn.call(r.Context(), arg, present)
	if err != nil {
		// A redirect in a command response makes the client throw, so it is
		// reported as an ordinary error instead.
		rs.writeError(w, asHTTPError(err))
		return
	}

	data := map[string]any{"_": value}
	if q := rs.resolveRefreshes(r.Context(), body.Refreshes); len(q) > 0 {
		data["q"] = q
		// `r` tells the client these single-flight updates replace the
		// invalidateAll it would otherwise run.
		data["r"] = true
	}
	rs.writeResult(w, data)
}

// resolveRefreshes runs every refresh key that names a registered query.
// Unrecognised keys are skipped in silence, as kit does.
func (rs *Remotes) resolveRefreshes(ctx context.Context, keys []string) map[string]any {
	q := map[string]any{}
	for _, key := range keys {
		// The payload can itself contain no slash, but the id always holds
		// exactly one, so the split is on the LAST slash.
		i := strings.LastIndex(key, "/")
		if i < 0 {
			continue
		}
		id, payload := key[:i], key[i+1:]

		fn, ok := rs.fns[id]
		if !ok || fn.kind != kindQuery {
			continue
		}
		arg, present, err := remotearg.ParsePayload(payload)
		if err != nil {
			continue
		}
		value, err := fn.call(ctx, arg, present)
		if err != nil {
			q[key] = map[string]any{"e": errorNode(asHTTPError(err))}
			continue
		}
		q[key] = map[string]any{"v": value}
	}
	return q
}

// rawPayload returns the payload parameter exactly as sent. The key the client
// caches under is built from this string, so it must never be re-encoded.
func rawPayload(u *url.URL) string {
	return u.Query().Get("payload")
}

func (rs *Remotes) header(w http.ResponseWriter) http.Header {
	h := w.Header()
	h.Set("Cache-Control", "private, no-store")
	if rs.cfg.Version != "" {
		h.Set("X-Sveltekit-Version", rs.cfg.Version)
	}
	return h
}

func (rs *Remotes) writeResult(w http.ResponseWriter, data map[string]any) {
	serialized, err := devalue.Stringify(data)
	if err != nil {
		rs.writeError(w, &HTTPError{Status: 500, Message: "Internal Error"})
		return
	}
	h := rs.header(w)
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

func errorNode(e *HTTPError) map[string]any {
	// devalue serialises the JSON value set only, so the status must be a
	// float, not a Go int.
	return map[string]any{"status": float64(e.Status), "message": e.Message}
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
