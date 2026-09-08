// Server routes: the Go side of SvelteKit's `+server.ts`.
//
// A `+server.ts` is the one part of kit that is not a page and not a remote
// function — it is raw HTTP. Kit dispatches to it by looking the request's
// method up on the module's exports (`runtime/server/endpoint.js`,
// `render_endpoint`), so the module's export names *are* the methods the route
// answers. skgo mirrors that: a developer writes ordinary `net/http` handlers
// in a `server.go` beside the route, marks each with the method it answers, and
// `skgo generate` emits the `+server.ts` whose exports kit compiles.
//
// Nothing here goes through devalue. An endpoint's body is whatever the handler
// wrote; kit's client never parses it, because kit's client never calls it.
package skgo

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
)

// endpointMethods is kit's own ENDPOINT_METHODS (`src/constants.js`), in kit's
// order — which is the order an `Allow` header lists them in.
var endpointMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "HEAD", "QUERY"}

// fallbackMethod is how a `fallback` export travels. Kit's build reports the
// methods of a compiled endpoint as its exported method names plus `"*"` for a
// fallback (`core/postbuild/analyse.js`, `analyse_endpoint`), and skgo carries
// that list from kit's build to the Go binary unchanged, so the two halves can
// be compared literally.
const fallbackMethod = "*"

// pageMethods is kit's PAGE_METHODS: the methods a page route can answer, and
// therefore the only ones for which "page or endpoint?" is a real question.
var pageMethods = map[string]bool{"GET": true, "POST": true, "HEAD": true}

// GET declares fn as the handler for GET requests to the route it is written
// in. Write it beside the handler, in a file named `server.go` in the route's
// own directory:
//
//	func get(w http.ResponseWriter, r *http.Request) { ... }
//
//	var _ = skgo.GET(get)
//
// `skgo generate` emits the `+server.ts` kit compiles — one export per marker,
// named as kit names them — and the Go registration that answers the route.
//
// The handler is an ordinary net/http handler: it owns the status, the headers
// and the body, and skgo does not encode, wrap or reinterpret any of them. What
// the request carries beyond the standard library is reachable with
// skgo.EventFrom(r.Context()) — the route id, the route parameters, and
// whatever the app's Handle hook stored with skgo.SetLocal.
func GET(fn http.HandlerFunc) Marker { _ = fn; return Marker{} }

// POST declares fn as the handler for POST requests to this route. See GET.
func POST(fn http.HandlerFunc) Marker { _ = fn; return Marker{} }

// PUT declares fn as the handler for PUT requests to this route. See GET.
func PUT(fn http.HandlerFunc) Marker { _ = fn; return Marker{} }

// PATCH declares fn as the handler for PATCH requests to this route. See GET.
func PATCH(fn http.HandlerFunc) Marker { _ = fn; return Marker{} }

// DELETE declares fn as the handler for DELETE requests to this route. See GET.
func DELETE(fn http.HandlerFunc) Marker { _ = fn; return Marker{} }

// OPTIONS declares fn as the handler for OPTIONS requests to this route.
//
// Kit synthesizes nothing here: an endpoint route answers OPTIONS only if its
// module exports one, and otherwise gets the same 405 any other undeclared
// method gets (`render_endpoint`). skgo does the same.
func OPTIONS(fn http.HandlerFunc) Marker { _ = fn; return Marker{} }

// HEAD declares fn as the handler for HEAD requests to this route.
//
// It is rarely needed: kit answers HEAD with the GET handler when the module
// exports no HEAD of its own, and skgo does too. Declare it only to answer HEAD
// differently from GET.
func HEAD(fn http.HandlerFunc) Marker { _ = fn; return Marker{} }

// QUERY declares fn as the handler for HTTP QUERY requests to this route. It is
// the HTTP method of that name, which kit lists among its endpoint methods; it
// has nothing to do with skgo.Query, which declares a remote function.
func QUERY(fn http.HandlerFunc) Marker { _ = fn; return Marker{} }

// Fallback declares fn as the handler for every method this route does not
// declare a handler for. It is kit's `fallback` export, and like kit's it
// answers before the 405 does.
func Fallback(fn http.HandlerFunc) Marker { _ = fn; return Marker{} }

// Endpoint is one registered method of one server route. Generated code builds
// these with NewEndpoint; application code declares the handlers and marks them
// with GET, POST and the rest.
type Endpoint struct {
	// routeID is kit's own route id — the route directory's path below
	// `src/routes`, groups included, e.g. "/(marketing)/pricing". It is the key
	// the manifest's route table joins on.
	routeID string
	// method is one of kit's endpoint methods, or "*" for a fallback.
	method  string
	handler http.HandlerFunc
}

// RouteID is kit's id for the route this endpoint answers.
func (e *Endpoint) RouteID() string { return e.routeID }

// Method is the HTTP method this endpoint answers, or "*" for a fallback.
func (e *Endpoint) Method() string { return e.method }

// NewEndpoint registers one method of one server route. Generated code calls
// this; application code uses GET, POST and the rest.
func NewEndpoint(routeID, method string, fn http.HandlerFunc) *Endpoint {
	return &Endpoint{routeID: routeID, method: method, handler: fn}
}

// EndpointConfig describes the app whose server routes a registry answers.
// Everything but Origin and Dev comes from the built manifest.
type EndpointConfig struct {
	// AppDir is kit's appDir; empty means "_app".
	AppDir string
	// Base is kit's paths.base, without a trailing slash.
	Base string
	// Origin is the app's configured origin. It is the "self" origin of kit's
	// CSRF check on form-shaped mutations; leaving it empty turns that check
	// off, which is what dev does.
	Origin string
	// TrustedOrigins are the extra origins kit's `csrf.trustedOrigins` allows
	// to submit forms to this app.
	TrustedOrigins []string
	// Dev relaxes the checks that only describe a production build.
	Dev bool
	// OnPanic is called when an endpoint handler panics, with the route id and
	// method, the recovered value, and the stack. Leaving it nil logs the same
	// three things, because a panicking handler that reports nowhere is a bug
	// that cannot be found.
	OnPanic func(routeID, method string, value any, stack []byte)
	// Routes is the route table, from the manifest.
	Routes []ManifestRoute

	// manifest reports that this config came from a build manifest, which is
	// what makes the drift check meaningful.
	manifest bool
}

// EndpointConfig derives an endpoint-registry configuration from a build
// manifest.
func (m Manifest) EndpointConfig(origin string) EndpointConfig {
	return EndpointConfig{
		AppDir:   m.AppDir,
		Base:     m.Base,
		Origin:   origin,
		Routes:   m.Routes,
		manifest: true,
	}
}

// Endpoints is a registry of server routes and the http.Handler that answers
// them.
//
// It also issues kit's trailing-slash redirect, because kit issues it for every
// route it matches — page or endpoint — before it decides which of the two
// answers (`runtime/server/respond.js`). Doing it anywhere else would leave
// endpoint routes out of it or duplicate the route table.
type Endpoints struct {
	cfg    EndpointConfig
	base   string
	origin string

	// routes is the route table. It is replaced wholesale rather than mutated
	// because under `vp dev` it describes a route tree a developer is editing
	// while requests are in flight.
	routes atomic.Pointer[[]*endpointRoute]
	// byRoute is every registered method, keyed by route id then method.
	byRoute map[string]map[string]http.HandlerFunc
}

// endpointRoute is one route of the manifest, with whatever Go answers on it.
type endpointRoute struct {
	id      string
	pattern *regexp.Regexp
	params  []ManifestParam
	// hasPage is true when the route also has a `+page`, which is what makes
	// "page or endpoint?" a question kit answers by content negotiation.
	hasPage bool
	// declared is the set of methods kit's build says the compiled `+server.ts`
	// exports, or nil when the route has no endpoint at all.
	declared map[string]bool
	// handlers is what Go answers, keyed by method. "*" is the fallback.
	handlers map[string]http.HandlerFunc
}

// NewEndpoints builds a registry. Every endpoint the built frontend declares
// must be registered and no other, for the same reason a remote function or a
// server load must: kit's route table is what decides that a request to this
// path is an endpoint request at all, so a disagreement is a route nobody
// answers or a handler nothing reaches.
func NewEndpoints(cfg EndpointConfig, eps ...*Endpoint) (*Endpoints, error) {
	if cfg.AppDir == "" {
		cfg.AppDir = "_app"
	}
	base := strings.TrimSuffix(cfg.Base, "/")
	if base != "" && !strings.HasPrefix(base, "/") {
		base = "/" + base
	}
	cfg.Base = base

	es := &Endpoints{
		cfg:     cfg,
		base:    base,
		origin:  cfg.Origin,
		byRoute: map[string]map[string]http.HandlerFunc{},
	}

	for _, ep := range eps {
		if ep == nil {
			return nil, errors.New("skgo: nil endpoint")
		}
		if ep.handler == nil {
			return nil, fmt.Errorf("skgo: endpoint %s %s has no handler", ep.method, ep.routeID)
		}
		if !validEndpointMethod(ep.method) {
			return nil, fmt.Errorf("skgo: %s is not one of SvelteKit's endpoint methods (%s or %q for a fallback)",
				ep.method, strings.Join(endpointMethods, ", "), fallbackMethod)
		}
		byMethod := es.byRoute[ep.routeID]
		if byMethod == nil {
			byMethod = map[string]http.HandlerFunc{}
			es.byRoute[ep.routeID] = byMethod
		}
		if _, dup := byMethod[ep.method]; dup {
			return nil, fmt.Errorf("skgo: two endpoints answer %s %s", ep.method, ep.routeID)
		}
		byMethod[ep.method] = ep.handler
	}

	if err := es.setRouting(cfg.Routes); err != nil {
		return nil, err
	}

	if err := es.checkDrift(); err != nil {
		return nil, err
	}
	return es, nil
}

// setRouting rebuilds the route table over the registered handlers. It is how a
// `vp dev` server's route tree reaches a running registry; a build's reaches it
// once, from NewEndpoints.
func (es *Endpoints) setRouting(routes []ManifestRoute) error {
	table := make([]*endpointRoute, 0, len(routes))
	for _, route := range routes {
		re, err := regexp.Compile(kitPattern(route.Pattern))
		if err != nil {
			return fmt.Errorf("skgo: route %s has an unusable pattern %q: %w", route.ID, route.Pattern, err)
		}
		er := &endpointRoute{
			id:       route.ID,
			pattern:  re,
			params:   route.Params,
			hasPage:  route.Page != nil,
			handlers: es.byRoute[route.ID],
		}
		if route.Endpoint != nil {
			er.declared = map[string]bool{}
			for _, method := range route.Endpoint.Methods {
				er.declared[method] = true
			}
		}
		table = append(table, er)
	}
	es.cfg.Routes = routes
	es.routes.Store(&table)
	return nil
}

func validEndpointMethod(method string) bool {
	if method == fallbackMethod {
		return true
	}
	for _, m := range endpointMethods {
		if m == method {
			return true
		}
	}
	return false
}

// checkDrift refuses to build a registry whose endpoints are not the ones the
// built frontend declares.
//
// The two lists being compared are the same list: kit's build records the
// method names a compiled `+server.ts` exports, and `skgo generate` writes the
// method names the Go markers declare. The adapter has already refused a build
// where those two disagree; this is the other half — the binary refusing a
// build it cannot answer.
func (es *Endpoints) checkDrift() error {
	if es.cfg.Dev || !es.cfg.manifest {
		return nil
	}

	declared := map[string]map[string]bool{}
	for _, route := range *es.routes.Load() {
		if route.declared != nil {
			declared[route.id] = route.declared
		}
	}

	var missing, extra []string
	for id, methods := range declared {
		for method := range methods {
			if es.byRoute[id][method] == nil {
				missing = append(missing, method+" "+id)
			}
		}
	}
	for id, byMethod := range es.byRoute {
		for method := range byMethod {
			if !declared[id][method] {
				extra = append(extra, method+" "+id)
			}
		}
	}
	if len(missing) == 0 && len(extra) == 0 {
		return nil
	}
	sort.Strings(missing)
	sort.Strings(extra)

	msg := "skgo: the built frontend and this binary disagree about the server routes."
	if len(missing) > 0 {
		msg += "\n  the frontend has a +server.ts for, but this binary does not answer: " + strings.Join(missing, ", ")
	}
	if len(extra) > 0 {
		msg += "\n  this binary answers, but the frontend has no +server.ts export for: " + strings.Join(extra, ", ")
	}
	msg += "\n  run `go generate ./...` and rebuild the frontend, then rebuild this binary."
	return errors.New(msg)
}

// Intercept returns a handler that answers this app's server routes and passes
// everything else to next.
//
// It must sit in front of whatever serves pages — the static handler in
// production, the dev proxy in dev — for the same reason the remote registry
// does: kit's own dev server would otherwise run the generated stub and throw.
func (es *Endpoints) Intercept(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		es.serve(w, r, next)
	})
}

func (es *Endpoints) serve(w http.ResponseWriter, r *http.Request, next http.Handler) {
	urlPath, ok := normalizePath(r.URL.Path)
	if !ok {
		next.ServeHTTP(w, r)
		return
	}
	if es.base != "" && urlPath != es.base && !strings.HasPrefix(urlPath, es.base+"/") {
		next.ServeHTTP(w, r)
		return
	}
	// Kit's own internal suffixes are not routes. `__data.json` is answered
	// upstream by the loads registry; anything still carrying a suffix here is
	// the static handler's to refuse.
	if kitSuffix(urlPath) != "" {
		next.ServeHTTP(w, r)
		return
	}

	// Kit refuses a form-shaped cross-site mutation at the very top of
	// `respond`, before it has matched a route or looked at a method
	// (`runtime/server/respond.js`). The order is the point: refusing it after
	// method resolution answers a cross-site DELETE with a 405 that tells the
	// caller which methods the route does answer.
	//
	// The app directory is exempt for the same reason kit's is: those requests
	// are the bundle, and kit's adapters answer them before the server runs.
	if !strings.HasPrefix(urlPath, es.appPrefix()) && es.csrfForbidden(r) {
		message := "Cross-site " + r.Method + " form submissions are forbidden"
		if r.Header.Get("Accept") == "application/json" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			writeJSON(w, map[string]string{"message": message})
			return
		}
		http.Error(w, message, http.StatusForbidden)
		return
	}

	routePath := strings.TrimPrefix(urlPath, es.base)
	if routePath == "" {
		routePath = "/"
	}

	route, params, matched := es.match(routePath)
	if !matched {
		next.ServeHTTP(w, r)
		return
	}

	// Kit normalizes the trailing slash of every route it matches, before it
	// decides whether a page or an endpoint answers, and answers a mismatch
	// with a 308 carrying a *relative* location (`runtime/server/respond.js`;
	// `normalize_path` and `relative_pathname` in `utils/url.js`).
	if location, redirect := es.normalize(urlPath, r.URL); redirect {
		w.Header().Set("x-sveltekit-normalize", "1")
		w.Header().Set("Location", location)
		w.WriteHeader(http.StatusPermanentRedirect)
		return
	}

	if route.handlers == nil && route.declared == nil {
		// An ordinary page route.
		next.ServeHTTP(w, r)
		return
	}

	// Kit varies on Accept for GET and HEAD when a route has both a page and
	// an endpoint, because that is the header it just negotiated over — and
	// only then (`runtime/server/respond.js`). It sets it whichever of the two
	// answers, so a cache never serves one to a client that asked for the
	// other.
	if route.hasPage && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
		if !varies(w.Header().Values("Vary"), "accept") {
			w.Header().Add("Vary", "Accept")
		}
	}

	if !route.answers(r) {
		next.ServeHTTP(w, r)
		return
	}

	handler, method, found := route.resolve(r.Method)
	if !found {
		methodNotAllowed(w, route.allowed(), r.Method)
		return
	}

	es.run(w, r, route, params, method, handler)
}

// appPrefix is the app directory with the configured base, e.g. "/_app/".
func (es *Endpoints) appPrefix() string {
	return es.base + "/" + es.cfg.AppDir + "/"
}

// run puts the request's event in place and calls the handler, turning a panic
// into a 500 rather than a dead process.
func (es *Endpoints) run(w http.ResponseWriter, r *http.Request, route *endpointRoute, params map[string]string, method string, handler http.HandlerFunc) {
	pageURL := *r.URL
	if pageURL.Host == "" {
		if origin, err := url.Parse(es.origin); err == nil && origin.Host != "" {
			pageURL.Scheme, pageURL.Host = origin.Scheme, origin.Host
		} else {
			pageURL.Host = r.Host
			pageURL.Scheme = "http"
			if r.TLS != nil {
				pageURL.Scheme = "https"
			}
		}
	}

	shared := &loadRequest{
		req:     r,
		jar:     newCookieJar(r, secureCookieDefault(es.cfg.Origin, es.cfg.Dev)),
		headers: http.Header{},
		url:     &pageURL,
		routeID: route.id,
		params:  params,
	}
	e := shared.event(0, nil)
	// An endpoint owns the ResponseWriter, so it writes cookies and headers
	// with net/http rather than through the event. Nothing here would apply
	// them, and a setter that silently loses what it was given is worse than
	// one that refuses.
	e.mutable = false
	e.endpoint = true
	e.load.uses.tracking = false

	defer func() {
		value := recover()
		if value == nil {
			return
		}
		if value == http.ErrAbortHandler {
			panic(value)
		}
		stack := debug.Stack()
		if es.cfg.OnPanic != nil {
			es.cfg.OnPanic(route.id, method, value, stack)
		} else {
			log.Printf("skgo: the %s handler for %s panicked: %v\n%s", method, route.id, value, stack)
		}
		http.Error(w, "Internal Error", http.StatusInternalServerError)
	}()

	handler(w, r.WithContext(withEvent(r.Context(), e)))
}

// match finds the route that serves routePath, which is the pathname with the
// configured base already removed.
func (es *Endpoints) match(routePath string) (*endpointRoute, map[string]string, bool) {
	for _, route := range *es.routes.Load() {
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

// normalize is kit's trailing-slash rule. skgo has no per-route trailingSlash
// option, so every route takes kit's default of `never` — except the root of a
// based app, which kit forces to `always` because `paths.base` names a
// directory.
func (es *Endpoints) normalize(urlPath string, u *url.URL) (location string, redirect bool) {
	normalized := strings.TrimSuffix(urlPath, "/")
	if urlPath == es.base || urlPath == es.base+"/" {
		normalized = es.base + "/"
	}
	if normalized == "" || normalized == urlPath {
		return "", false
	}

	// Relative, exactly as kit sends it, so that a path prefix the app cannot
	// see — a reverse proxy's, say — is preserved.
	segment := strings.TrimSuffix(normalized, "/")
	if i := strings.LastIndex(segment, "/"); i >= 0 {
		segment = segment[i+1:]
	}
	if strings.HasSuffix(urlPath, "/") {
		location = "../" + segment
	} else {
		location = segment + "/"
	}
	if u.RawQuery != "" {
		location += "?" + u.RawQuery
	}
	return location, true
}

// csrfForbidden is kit's `is_csrf_forbidden` (`runtime/server/csrf.js`): a
// form-shaped mutation from another origin is refused before the handler runs.
// Kit applies it to every non-remote request, which includes every endpoint.
func (es *Endpoints) csrfForbidden(r *http.Request) bool {
	if es.origin == "" {
		return false
	}
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return false
	}
	contentType := r.Header.Get("Content-Type")
	if contentType != "" && !isFormContentType(mediaType(contentType)) {
		return false
	}
	requestOrigin := r.Header.Get("Origin")
	if requestOrigin == es.origin {
		return false
	}
	if requestOrigin == "" {
		return true
	}
	for _, trusted := range es.cfg.TrustedOrigins {
		if requestOrigin == trusted {
			return false
		}
	}
	return true
}

// answers reports whether the server route takes this request rather than the
// page. It is kit's own decision, from `runtime/server/respond.js`:
//
//	route.endpoint && (!route.page || (!prerendering && is_endpoint_request(event)))
//
// followed by "prefer the page if the endpoint cannot handle this GET, HEAD or
// POST". The second half is deliberately narrow — a method a page could never
// answer stays with the endpoint and gets its 405 there, rather than falling
// through to a page that cannot serve it either.
func (r *endpointRoute) answers(req *http.Request) bool {
	if r.handlers == nil && r.declared == nil {
		return false
	}
	if r.hasPage && !r.isEndpointRequest(req) {
		return false
	}
	if !r.hasPage || !pageMethods[req.Method] {
		return true
	}
	if req.Method == http.MethodPost {
		return r.handlers["POST"] != nil || r.handlers[fallbackMethod] != nil
	}
	return r.handlers["GET"] != nil || r.handlers[fallbackMethod] != nil ||
		(req.Method == http.MethodHead && r.handlers["HEAD"] != nil)
}

// resolve is kit's `render_endpoint`: the module's export for this method, else
// its `fallback` — with HEAD answered by GET when the module exports no HEAD of
// its own.
func (r *endpointRoute) resolve(method string) (http.HandlerFunc, string, bool) {
	if handler := r.handlers[method]; handler != nil {
		return handler, method, true
	}
	if method == http.MethodHead && r.handlers["GET"] != nil {
		return r.handlers["GET"], "GET", true
	}
	if fallback := r.handlers[fallbackMethod]; fallback != nil {
		return fallback, fallbackMethod, true
	}
	return nil, "", false
}

// isEndpointRequest is kit's own (`runtime/server/endpoint.js`): for a route
// that has both a page and an endpoint, the methods only an endpoint can answer
// go to the endpoint, and the rest go to whichever the client asked for.
func (r *endpointRoute) isEndpointRequest(req *http.Request) bool {
	if !pageMethods[req.Method] {
		return true
	}
	if req.Method == http.MethodPost && req.Header.Get("x-sveltekit-action") == "true" {
		return false
	}
	accept := req.Header.Get("Accept")
	if accept == "" {
		accept = "*/*"
	}
	return negotiate(accept, "*", "text/html") != "text/html"
}

// allowed is kit's `allowed_methods` (`runtime/server/utils.js`): the endpoint
// methods this route declares, in kit's order, with HEAD appended when a GET
// handler will answer it.
func (r *endpointRoute) allowed() []string {
	var allowed []string
	for _, method := range endpointMethods {
		if r.handlers[method] != nil {
			allowed = append(allowed, method)
		}
	}
	if r.handlers["GET"] != nil && r.handlers["HEAD"] == nil {
		allowed = append(allowed, "HEAD")
	}
	return allowed
}

// varies reports whether a Vary header field already covers name, or is `*`.
func varies(fields []string, name string) bool {
	for _, field := range fields {
		for _, token := range strings.Split(field, ",") {
			token = strings.TrimSpace(token)
			if token == "*" || strings.EqualFold(token, name) {
				return true
			}
		}
	}
	return false
}

// negotiate is a port of kit's own (`utils/http.js`): given an Accept header
// and the types the server can produce, it returns the one to answer with, or
// "" when none of them is acceptable.
//
// The sort is kit's: by q descending, then a concrete subtype before `*`, then
// a concrete type before `*`, then the order the client wrote them in. It is
// not RFC 9110's specificity rule, and the difference is load-bearing — kit's
// rule is what makes a plain `*/*` request to a route that has both a page and
// an endpoint reach the endpoint.
func negotiate(accept string, types ...string) string {
	type part struct {
		typ, subtype string
		q            float64
		i            int
	}
	var parts []part
	for i, raw := range strings.Split(accept, ",") {
		raw = strings.TrimSpace(raw)
		spec, params, _ := strings.Cut(raw, ";")
		typ, subtype, ok := strings.Cut(strings.TrimSpace(spec), "/")
		if !ok || typ == "" || subtype == "" || strings.ContainsAny(typ, " \t") {
			continue
		}
		q := 1.0
		for _, param := range strings.Split(params, ";") {
			name, value, ok := strings.Cut(param, "=")
			if !ok || strings.TrimSpace(name) != "q" {
				continue
			}
			if parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil {
				q = parsed
			}
		}
		parts = append(parts, part{typ: typ, subtype: strings.TrimSpace(subtype), q: q, i: i})
	}
	sort.SliceStable(parts, func(a, b int) bool {
		x, y := parts[a], parts[b]
		if x.q != y.q {
			return x.q > y.q
		}
		if (x.subtype == "*") != (y.subtype == "*") {
			return y.subtype == "*"
		}
		if (x.typ == "*") != (y.typ == "*") {
			return y.typ == "*"
		}
		return x.i < y.i
	})

	accepted, best := "", len(parts)
	for _, mimetype := range types {
		typ, subtype, _ := strings.Cut(mimetype, "/")
		for i, p := range parts {
			if (p.typ == typ || p.typ == "*") && (p.subtype == subtype || p.subtype == "*") {
				if i < best {
					accepted, best = mimetype, i
				}
				break
			}
		}
	}
	return accepted
}

// methodNotAllowed is kit's `method_not_allowed`, byte for byte: a plain-text
// body naming the method, and the `Allow` header RFC 9110 requires of a 405.
func methodNotAllowed(w http.ResponseWriter, allowed []string, method string) {
	w.Header().Set("Allow", strings.Join(allowed, ", "))
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusMethodNotAllowed)
	if method == http.MethodHead {
		return
	}
	fmt.Fprintf(w, "%s method not allowed", method)
}
