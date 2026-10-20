package skgo

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
)

// CallerMatchers is emitted alongside remote bindings, independently of loads.
// The manifest owns candidate ordering; only the Go matchers convert captures.
type CallerMatchers map[string]ParamMatcher

// CallerParams is generated from the parameter names and matchers Kit's routes
// declare, including routes without Go loads. It constructs the shared Params
// for any route whose declarations it knows, so adding or removing a route that
// declares nothing new leaves the generated Go unchanged. It must refuse a
// declaration it has no variant for, even when the value is absent.
type CallerParams func(routeID string, declared []ManifestParam, values map[string]any) (any, error)

func callerManifestDrift(format string, args ...any) error {
	return Errorf(http.StatusServiceUnavailable, "skgo: caller manifest drift: "+format, args...)
}

type remoteCaller struct {
	url     *url.URL
	routeID string
	params  []ManifestParam
	values  map[string]any
}
type remoteCallerKey struct{}

type callerRouting struct {
	mu        sync.RWMutex
	routes    []*dataRoute
	matchers  CallerMatchers
	generated []CallerParams
}

func (rs *Remotes) updateCallerRoutes(routes []ManifestRoute) error {
	if err := rs.checkCallerDrift(routes); err != nil {
		return err
	}
	_, compiled, err := loadRouting(LoadConfig{Routes: routes}, nil)
	if err != nil {
		return err
	}
	rs.callers.mu.Lock()
	rs.callers.routes = compiled
	rs.callers.mu.Unlock()
	return nil
}

// Validate every served route before matching even the first candidate. Kit
// removes prerendered routes from the production table, so generated routes
// may be a superset of the served snapshot.
func (rs *Remotes) checkCallerDrift(routes []ManifestRoute) error {
	hasCaller := false
	for _, fn := range rs.fns {
		if fn.kind == KindCommand || fn.kind == KindForm {
			hasCaller = true
			break
		}
	}
	if !hasCaller {
		return nil
	}
	for _, route := range routes {
		for _, param := range route.Params {
			if param.Matcher != "" && rs.callers.matchers[param.Matcher] == nil {
				return callerManifestDrift("route %q has unknown matcher %q; regenerate and rebuild Go", route.ID, param.Matcher)
			}
		}
		for _, generated := range rs.callers.generated {
			if _, err := generated(route.ID, route.Params, nil); err != nil {
				return callerManifestDrift("route %q Params constructor: %v; regenerate and rebuild Go", route.ID, err)
			}
		}
	}
	return nil
}

// RemoteCallerValues is the generated command/form constructor's input: the
// caller route, the parameters Kit declares for it and their converted values.
// A query event cannot reconstruct a caller's shared Params through this helper.
func RemoteCallerValues(e *Event) (string, []ManifestParam, map[string]any) {
	if e == nil {
		return "", nil, nil
	}
	if e.query {
		panic("skgo: cannot read params in a remote query")
	}
	if e.caller == nil {
		return "", nil, nil
	}
	values := make(map[string]any, len(e.caller.values))
	for key, value := range e.caller.values {
		values[key] = value
	}
	return e.caller.routeID, slices.Clone(e.caller.params), values
}

func (rs *Remotes) matchCaller(r *http.Request) (*remoteCaller, error) {
	if state := hookStateOf(r.Context()); state != nil && state.routing {
		return &remoteCaller{url: state.url, routeID: state.routeID, params: state.declared, values: state.converted}, nil
	}
	origin, _ := url.Parse(rs.cfg.Origin)
	u, skip := (HandleConfig{}).eventURL(r, origin, rs.cfg.Base, false, true)
	caller := &remoteCaller{url: u}
	if skip {
		return caller, nil
	}
	pathname := r.Header.Get("x-sveltekit-pathname")
	path, err := decodePathname(pathname)
	if err != nil {
		return nil, Errorf(400, "Bad Request")
	}
	// Go URLs store their decoded path separately from the escaped pathname.
	// Keep the caller URL faithful while matching Kit's decodeURI view above.
	u.Path, err = url.PathUnescape(pathname)
	if err != nil {
		return nil, Errorf(400, "Bad Request")
	}
	u.RawPath = pathname
	if rs.cfg.Base != "" {
		if !strings.HasPrefix(path, rs.cfg.Base) {
			return caller, nil
		}
		path = strings.TrimPrefix(path, rs.cfg.Base)
		if path == "" {
			path = "/"
		}
	}
	rs.callers.mu.RLock()
	defer rs.callers.mu.RUnlock()
	for _, route := range rs.callers.routes {
		loc := route.pattern.FindStringSubmatchIndex(path)
		if loc == nil {
			continue
		}
		_, converted, ok := execMatchedParams(path, loc, route.params, rs.callers.matchers)
		if !ok {
			continue
		}
		caller.routeID, caller.params, caller.values = route.id, route.params, converted
		return caller, nil
	}
	return caller, nil
}

func callerContext(ctx context.Context, caller *remoteCaller) context.Context {
	return context.WithValue(ctx, remoteCallerKey{}, caller)
}
