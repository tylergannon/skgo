// Server loads: the Go side of SvelteKit's `+page.server.ts` and
// `+layout.server.ts`.
//
// Kit's client asks for `${base}/<route>/__data.json` once per navigation — it
// is the only endpoint it calls on its own initiative — and Go answers it. The
// JavaScript kit needs is a generated `+page.server.ts` whose `load` throws;
// kit never calls it, it only reads the export to decide that the route has
// server data at all.
package skgo

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"runtime/debug"
	"sort"
	"strings"
)

// Load declares fn as a SvelteKit server load. Write it beside the function, in
// a file named `page.server.go` or `layout.server.go` in the route's own
// directory:
//
//	func load(ctx context.Context) (Data, error) { ... }
//
//	var _ = skgo.Load(load)
//
// `skgo generate` emits the `+page.server.ts` or `+layout.server.ts` kit
// compiles — the file name says which — and the Go registration that answers
// the route's `__data.json`. The request is reachable with skgo.EventFrom(ctx),
// which inside a load may read the URL, the route parameters and the route id,
// and may write cookies and response headers.
//
// Out must be a struct: kit requires a load to return a plain object, and
// refuses anything else.
func Load[Out any](fn func(context.Context) (Out, error)) Marker { _ = fn; return Marker{} }

// ServerLoad is one registered server load. Generated code builds these with
// NewLoad; application code declares the functions and marks them with Load.
type ServerLoad struct {
	// module is the vite-root-relative path of the `+*.server.ts` kit
	// compiled, e.g. "src/routes/account/+layout.server.ts". It is the key kit
	// itself records for the node, and so the key the manifest joins on.
	module string
	run    func(ctx context.Context) (any, error)
}

// Module is the vite-root-relative path of the `+*.server.ts` this load
// answers for.
func (l *ServerLoad) Module() string { return l.module }

// NewLoad registers a server load for the module at path. Generated code calls
// this; application code uses Load.
func NewLoad[Out any](module string, fn func(context.Context) (Out, error)) *ServerLoad {
	return &ServerLoad{
		module: module,
		run: func(ctx context.Context) (any, error) {
			out, err := fn(ctx)
			if err != nil {
				return nil, err
			}
			return encodeLoadValue(out)
		},
	}
}

// LoadConfig describes the app whose loads a registry answers. Everything but
// Origin and Dev comes from the built manifest.
type LoadConfig struct {
	// AppDir is kit's appDir; empty means "_app".
	AppDir string
	// Base is kit's paths.base, without a trailing slash.
	Base string
	// Version, when non-empty, is sent as the `x-sveltekit-version` response
	// header. It must equal the version baked into the client — a different
	// value makes the client believe a new deployment has landed and reload —
	// so leave it empty whenever the client did not come from this build.
	Version string
	// Origin is the app's configured origin. A load's `depends("/some/path")`
	// is resolved against it, exactly as kit resolves it against the request
	// URL, so that the identifier the client matches on is the same one.
	Origin string
	// Dev relaxes the checks that only describe a production build.
	Dev bool
	// Handle is the app's `handle` hook: the one place it decides what a
	// request may do. It is optional; see the Handle type.
	Handle Handle
	// OnPanic is called when a load or the `handle` hook panics, with the
	// `+*.server.ts` module of the load (or "handle" for the hook), the
	// recovered value, and the stack. The client is told nothing but an opaque
	// 500, so this is the only record the panic leaves; leaving it nil logs
	// the same three things to the standard logger, because a panicking
	// handler that reports nowhere is a bug that cannot be found.
	OnPanic func(id string, value any, stack []byte)
	// Nodes and Routes come from the manifest.
	Nodes  []string
	Routes []ManifestRoute

	// manifest reports that this config came from a build manifest, which is
	// what makes the drift check meaningful.
	manifest bool
}

// LoadConfig derives a load-registry configuration from a build manifest.
func (m Manifest) LoadConfig(origin string) LoadConfig {
	return LoadConfig{
		AppDir:   m.AppDir,
		Base:     m.Base,
		Version:  m.Version,
		Origin:   origin,
		Nodes:    m.Nodes,
		Routes:   m.Routes,
		manifest: true,
	}
}

// Loads is a registry of server loads and the http.Handler that answers
// `__data.json`.
type Loads struct {
	cfg    LoadConfig
	base   string
	origin *url.URL
	handle Handle

	// byModule is every registered load, keyed by its `+*.server.ts` path.
	byModule map[string]*ServerLoad
	// nodes maps a kit node index to the load that answers it, nil when the
	// node has no server file.
	nodes []*ServerLoad
	// routes is the route table, in manifest order.
	routes []*dataRoute
}

// dataRoute is one page route: what to match, what to name the captures, and
// which load answers each slot of the branch.
type dataRoute struct {
	id      string
	pattern *regexp.Regexp
	params  []ManifestParam
	// branch is `[...layouts, leaf]`; an entry is nil when that node has no
	// server load, which is what the client is told with a `null` node.
	branch []*ServerLoad
	// hasPage is false for a route that is an endpoint and nothing else. Kit
	// answers `__data.json` on one of those with a bare 404.
	hasPage bool
}

// NewLoads builds a registry. Every load the built frontend declares must be
// registered and no other, for the same reason a remote function must: kit
// decides whether its client asks for `__data.json` from the modules it
// compiled, so a disagreement is a page whose data nobody answers.
func NewLoads(cfg LoadConfig, loads ...*ServerLoad) (*Loads, error) {
	if cfg.AppDir == "" {
		cfg.AppDir = "_app"
	}
	base := strings.TrimSuffix(cfg.Base, "/")
	if base != "" && !strings.HasPrefix(base, "/") {
		base = "/" + base
	}
	cfg.Base = base

	ls := &Loads{cfg: cfg, base: base, handle: cfg.Handle, byModule: make(map[string]*ServerLoad, len(loads))}

	if cfg.Origin != "" {
		origin, err := url.Parse(cfg.Origin)
		if err != nil {
			return nil, fmt.Errorf("skgo: unusable origin %q: %w", cfg.Origin, err)
		}
		ls.origin = origin
	}

	for _, load := range loads {
		if load == nil {
			return nil, errors.New("skgo: nil server load")
		}
		if existing, dup := ls.byModule[load.module]; dup {
			_ = existing
			return nil, fmt.Errorf("skgo: two server loads claim %s", load.module)
		}
		ls.byModule[load.module] = load
	}

	ls.nodes = make([]*ServerLoad, len(cfg.Nodes))
	for i, module := range cfg.Nodes {
		if module == "" {
			continue
		}
		ls.nodes[i] = ls.byModule[module]
	}

	for _, route := range cfg.Routes {
		re, err := regexp.Compile(kitPattern(route.Pattern))
		if err != nil {
			return nil, fmt.Errorf("skgo: route %s has an unusable pattern %q: %w", route.ID, route.Pattern, err)
		}
		dr := &dataRoute{id: route.ID, pattern: re, params: route.Params, hasPage: route.Page != nil}
		for _, index := range route.Page.Branch() {
			if index < 0 || index >= len(ls.nodes) {
				dr.branch = append(dr.branch, nil)
				continue
			}
			dr.branch = append(dr.branch, ls.nodes[index])
		}
		ls.routes = append(ls.routes, dr)
	}

	if err := ls.checkDrift(); err != nil {
		return nil, err
	}
	return ls, nil
}

// checkDrift refuses to build a registry whose loads are not the ones the built
// frontend declares.
func (ls *Loads) checkDrift() error {
	if ls.cfg.Dev || !ls.cfg.manifest {
		return nil
	}

	built := map[string]bool{}
	for _, module := range ls.cfg.Nodes {
		if module != "" {
			built[module] = true
		}
	}

	var missing, extra []string
	for module := range built {
		if _, ok := ls.byModule[module]; !ok {
			missing = append(missing, module)
		}
	}
	for module := range ls.byModule {
		if !built[module] {
			extra = append(extra, module)
		}
	}
	if len(missing) == 0 && len(extra) == 0 {
		return nil
	}
	sort.Strings(missing)
	sort.Strings(extra)

	msg := "skgo: the built frontend and this binary disagree about the server loads."
	if len(missing) > 0 {
		msg += "\n  the frontend has a server load for, but this binary does not answer: " + strings.Join(missing, ", ")
	}
	if len(extra) > 0 {
		msg += "\n  this binary answers, but the frontend has no server load for: " + strings.Join(extra, ", ")
	}
	msg += "\n  run `go generate ./...` and rebuild the frontend, then rebuild this binary."
	return errors.New(msg)
}

// match finds the route that serves routePath, which is the pathname with the
// data suffix already stripped and the configured base already removed.
func (ls *Loads) match(routePath string) (*dataRoute, map[string]string, bool) {
	for _, route := range ls.routes {
		loc := route.pattern.FindStringSubmatchIndex(routePath)
		if loc == nil {
			continue
		}
		params, ok := execParams(routePath, loc, route.params)
		if !ok {
			continue
		}
		return route, params, true
	}
	return nil, nil, false
}

// execParams names a route pattern's captures, a port of kit's own `exec`
// (`utils/routing.js`). Matchers are deliberately absent: a matcher is a
// JavaScript function and skgo runs none, so a route whose parameter has one
// matches on the pattern alone — which is kit's pattern minus the matcher's own
// opinion, never more permissive about the shape of the path.
//
// It works from submatch *indexes* rather than strings because JavaScript
// distinguishes a group that did not participate (`undefined`) from one that
// matched nothing (`”`), and Go's FindStringSubmatch collapses both to "".
func execParams(path string, loc []int, params []ManifestParam) (map[string]string, bool) {
	result := map[string]string{}

	// group i is capture i+1 of the pattern, which is values[i] in kit.
	count := len(loc)/2 - 1
	defined := func(i int) bool { return i >= 0 && i < count && loc[2*(i+1)] >= 0 }
	value := func(i int) string {
		if !defined(i) {
			return ""
		}
		return path[loc[2*(i+1)]:loc[2*(i+1)+1]]
	}
	// kit's `next_value &&` tests are truthiness, so an empty match is falsy.
	truthy := func(i int) bool { return defined(i) && value(i) != "" }

	needingMatch := 0
	for i := 0; i < count; i++ {
		if defined(i) {
			needingMatch++
		}
	}

	buffered := 0
	for i := 0; i < len(params); i++ {
		param := params[i]
		idx := i - buffered
		got, raw := defined(idx), value(idx)

		// In the `[[a=b]]/.../[...rest]` case, roll the values skipped by
		// unmatched optional parameters into the rest parameter.
		if param.Chained && param.Rest && buffered > 0 {
			var parts []string
			for j := i - buffered; j <= i; j++ {
				if truthy(j) {
					parts = append(parts, value(j))
				}
			}
			got, raw = true, strings.Join(parts, "/")
			buffered = 0
		}

		if !got {
			if !param.Rest {
				continue
			}
			raw = ""
		}

		decoded, err := url.PathUnescape(raw)
		if err != nil {
			decoded = raw
		}
		result[param.Name] = decoded

		hasNext := i+1 < len(params)
		if hasNext {
			next := params[i+1]
			if !next.Rest && next.Optional && truthy(i+1) && param.Chained {
				buffered = 0
			}
		}
		if !hasNext && !truthy(i+1) && len(result) == needingMatch {
			buffered = 0
		}
	}

	if buffered > 0 {
		return nil, false
	}
	return result, true
}

// runLoad runs one load, turning a panic into the error the branch already
// knows how to answer.
//
// A load is ordinary Go and one of them will panic. It matters more here than
// on a remote call: loads run on their own goroutines, one per node of the
// branch, and an unrecovered panic on a goroutine is not a reset connection
// but a dead process, taking every other visitor with it. Kit answers an
// unexpected throw from a load with the same opaque 500 it gives any
// unexpected error, and so does this.
func (ls *Loads) runLoad(ctx context.Context, load *ServerLoad) (v any, err error) {
	defer func() { err = ls.recovered(load.module, recover(), err) }()
	return load.run(ctx)
}

// runHandleGuarded is the same guard for the `handle` hook, which is
// application code on the same request and runs before any load does.
func (ls *Loads) runHandleGuarded(r *http.Request, isData bool) (req *http.Request, err error) {
	defer func() {
		if err = ls.recovered("handle", recover(), err); err != nil {
			req = r
		}
	}()
	return ls.runHandle(r, isData)
}

// recovered reports a panic and converts it to an error. It is a no-op when
// nothing panicked, so the guards above read as one deferred line.
func (ls *Loads) recovered(id string, value any, err error) error {
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
	if ls.cfg.OnPanic != nil {
		ls.cfg.OnPanic(id, value, stack)
	} else {
		log.Printf("skgo: server load %s panicked: %v\n%s", id, value, stack)
	}
	return &HTTPError{Status: 500, Message: "Internal Error"}
}
