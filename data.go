package skgo

import (
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/tylergannon/skgo/internal/devalue"
)

// dataSuffix and htmlDataSuffix are kit's own (`packages/kit/src/pathname.js`).
// The second exists because `/foo.html__data.json` is a *sibling* of
// `/foo.html`, not a child of it.
const (
	dataSuffix     = "/__data.json"
	htmlDataSuffix = ".html__data.json"

	invalidatedParam   = "x-sveltekit-invalidated"
	trailingSlashParam = "x-sveltekit-trailing-slash"
)

func hasDataSuffix(pathname string) bool {
	return strings.HasSuffix(pathname, dataSuffix) || strings.HasSuffix(pathname, htmlDataSuffix)
}

func stripDataSuffix(pathname string) string {
	if strings.HasSuffix(pathname, htmlDataSuffix) {
		return strings.TrimSuffix(pathname, htmlDataSuffix) + ".html"
	}
	return strings.TrimSuffix(pathname, dataSuffix)
}

// Intercept returns a handler that runs the app's `handle` hook, answers
// `__data.json` itself, and passes everything else to next.
//
// It must sit outermost: kit runs `handle` before it dispatches to a page, a
// data request or a remote function, and in dev it must also sit in front of
// the proxy, or kit's own dev server would run the generated stub and throw.
func (ls *Loads) Intercept(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data := hasDataSuffix(r.URL.Path)

		// Kit's `handle` never sees a static asset: its adapters answer those
		// before the server does. skgo's equivalent is the app directory,
		// which holds nothing but the immutable bundle — and the remote calls,
		// which are requests the app answers and so must pass through.
		if ls.handle != nil && !ls.isImmutableAsset(r.URL.Path) {
			req, err := ls.runHandle(r, data)
			if err != nil {
				ls.refuse(w, r, err, data)
				return
			}
			r = req
		}

		if data {
			ls.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// isImmutableAsset reports that a path is part of the built bundle rather than
// something the app answers.
func (ls *Loads) isImmutableAsset(pathname string) bool {
	prefix := ls.base + "/" + ls.cfg.AppDir + "/"
	return strings.HasPrefix(pathname, prefix) && !strings.HasPrefix(pathname, prefix+"remote/")
}

// ServeHTTP answers one `__data.json` request.
func (ls *Loads) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	req, ok := ls.parseDataRequest(r)
	if !ok {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	route, params, matched := ls.match(req.routePath)
	if !matched {
		// An unmatched path is still asked for the root layout's data, so that
		// the error page kit is about to render has its layout
		// (`runtime/server/respond.js`, the `invalidated_data_nodes` branch).
		// Kit recognises exactly one invalidated node, and answers anything
		// else with a 404 — which the client turns into a `Not Found` error
		// rather than a reload.
		if len(req.invalidated) == 1 && req.invalidated[0] {
			ls.serveBranch(w, r, req, "", map[string]string{}, ls.rootBranch(), req.invalidated)
			return
		}
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}
	if !route.hasPage {
		// A route that is an endpoint and nothing else has no data to give.
		w.WriteHeader(http.StatusNotFound)
		return
	}

	ls.serveBranch(w, r, req, route.id, params, route.branch, req.invalidated)
}

// rootBranch is the single-node branch kit uses for a path that matches no
// route: node 0, the root layout.
func (ls *Loads) rootBranch() []*ServerLoad {
	if len(ls.nodes) == 0 {
		return []*ServerLoad{nil}
	}
	return []*ServerLoad{ls.nodes[0]}
}

// dataRequest is one parsed `__data.json` request.
type dataRequest struct {
	// url is the URL the *page* was asked for: the data suffix removed, the
	// trailing slash restored, and kit's two private query parameters deleted.
	// It is what a load sees, exactly as kit deletes them before running one.
	url *url.URL
	// routePath is url.Path with the configured base removed.
	routePath string
	// invalidated is one entry per branch slot; nil when the client sent no
	// `x-sveltekit-invalidated`, which means every node runs.
	invalidated []bool
}

func (ls *Loads) parseDataRequest(r *http.Request) (dataRequest, bool) {
	urlPath, ok := normalizePath(r.URL.Path)
	if !ok {
		return dataRequest{}, false
	}
	if ls.base != "" && urlPath != ls.base && !strings.HasPrefix(urlPath, ls.base+"/") {
		return dataRequest{}, false
	}

	query := r.URL.Query()
	pathname := stripDataSuffix(urlPath)
	if query.Get(trailingSlashParam) == "1" {
		pathname += "/"
	}
	if pathname == "" {
		pathname = "/"
	}

	var invalidated []bool
	if raw, present := query[invalidatedParam]; present && len(raw) > 0 {
		invalidated = make([]bool, 0, len(raw[0]))
		for _, c := range raw[0] {
			invalidated = append(invalidated, c == '1')
		}
	}
	query.Del(trailingSlashParam)
	query.Del(invalidatedParam)

	page := ls.origin
	if page == nil {
		page = &url.URL{Scheme: "http", Host: r.Host}
		if r.TLS != nil {
			page.Scheme = "https"
		}
	}
	pageURL := &url.URL{
		Scheme:   page.Scheme,
		Host:     page.Host,
		Path:     pathname,
		RawQuery: query.Encode(),
	}

	routePath := strings.TrimPrefix(pathname, ls.base)
	if routePath == "" {
		routePath = "/"
	}
	return dataRequest{url: pageURL, routePath: routePath, invalidated: invalidated}, true
}

// node is one entry of the `nodes` array on the wire.
type dataNode struct {
	// kind is "", "skip", "error" or "data"; "" serializes as null.
	kind  string
	data  any
	uses  *uses
	err   *HTTPError
	redir *Redirect
}

// serveBranch runs a route's branch and writes the response.
func (ls *Loads) serveBranch(w http.ResponseWriter, r *http.Request, req dataRequest, routeID string, params map[string]string, branch []*ServerLoad, invalidated []bool) {
	ctx := r.Context()

	shared := &loadRequest{
		req:     r,
		jar:     newCookieJar(r, secureCookieDefault(ls.cfg.Origin, ls.cfg.Dev)),
		headers: http.Header{},
		url:     req.url,
		routeID: routeID,
		params:  params,
	}

	// asked[i] is true for a node the client wants back. It matters because of
	// `aborted` below: kit starts every wanted node in the same tick, before
	// any of them can have failed, so a wanted node never sees the flag. Only
	// the extra runs `parent()` drives can.
	asked := make([]bool, len(branch))
	for i := range branch {
		asked[i] = invalidated == nil || (i < len(invalidated) && invalidated[i])
	}

	// Kit builds one memoised function per node. `parent()` calls the earlier
	// ones directly, so a node the client told the server to skip still runs
	// when a descendant asks for its data — and its result is still not sent.
	fns := make([]func() dataNode, len(branch))
	var aborted atomicFlag
	for i := range branch {
		i := i
		fns[i] = sync.OnceValue(func() dataNode {
			if !asked[i] && aborted.get() {
				return dataNode{kind: "skip"}
			}
			load := branch[i]
			if load == nil {
				return dataNode{}
			}
			e := shared.event(i, fns)
			value, err := load.run(withEvent(ctx, e))
			if err != nil {
				aborted.set()
				if redirect := asRedirect(err); redirect != nil {
					return dataNode{kind: "error", redir: redirect}
				}
				return dataNode{kind: "error", err: asHTTPError(err)}
			}
			return dataNode{kind: "data", data: value, uses: e.load.uses}
		})
	}

	nodes := make([]dataNode, len(branch))
	var wg sync.WaitGroup
	for i := range branch {
		if !asked[i] {
			nodes[i] = dataNode{kind: "skip"}
			continue
		}
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			nodes[i] = fns[i]()
		}()
	}
	wg.Wait()

	// A redirect anywhere in the branch is the whole answer. Kit's `Promise.all`
	// rejects on the first one to arrive; taking the outermost keeps it
	// deterministic, and it is the one a guard on a layout means.
	for _, n := range nodes {
		if n.redir != nil {
			ls.writeRedirect(w, shared, n.redir)
			return
		}
	}

	ls.writeNodes(w, r, shared, nodes)
}

// writeNodes serializes the branch and writes it, streaming the deferred values
// as they settle.
func (ls *Loads) writeNodes(w http.ResponseWriter, r *http.Request, shared *loadRequest, nodes []dataNode) {
	promises := &promiseTable{ids: map[*deferred]int{}}
	reducers := []devalue.Reducer{{
		Key: "Promise",
		Fn: func(v any) (any, bool, error) {
			holder, ok := v.(*deferred)
			if !ok {
				return nil, false, nil
			}
			return float64(promises.id(holder)), true, nil
		},
	}}

	parts := make([]string, len(nodes))
	for i, n := range nodes {
		serialized, err := serializeNode(n, reducers)
		if err != nil {
			serialized, _ = serializeNode(dataNode{kind: "error", err: Errorf(500, "Internal Error")}, nil)
		}
		parts[i] = serialized
	}
	head := `{"type":"data","nodes":[` + strings.Join(parts, ",") + "]}\n"

	h := ls.header(w)
	shared.applyTo(h)

	if len(promises.order) == 0 {
		h.Set("Content-Type", "application/json")
		h.Set("Content-Length", strconv.Itoa(len(head)))
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			w.Write([]byte(head))
		}
		return
	}

	// A proprietary content type, as kit's own comment puts it, so that no
	// proxy buffers the stream; the `text` prefix keeps it inspectable.
	h.Set("Content-Type", "text/sveltekit-data")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	w.Write([]byte(head))
	flush(w)

	// The stream ends when every promise has settled, including any a chunk
	// itself introduced. There is no terminator: the body simply closes.
	ctx := r.Context()
	for i := 0; i < len(promises.order); i++ {
		d := promises.order[i]
		id := promises.ids[d]
		value, err := d.wait(ctx)
		if ctx.Err() != nil {
			return
		}
		w.Write([]byte(chunkLine(id, value, err, reducers)))
		flush(w)
	}
}

func chunkLine(id int, value any, err error, reducers []devalue.Reducer) string {
	key, payload := "data", value
	if err != nil {
		key, payload = "error", errorNode(asHTTPError(err))
	}
	serialized, serr := devalue.StringifyWith(payload, reducers)
	if serr != nil {
		key = "error"
		serialized, _ = devalue.StringifyWith(errorNode(Errorf(500, "Internal Error")), nil)
	}
	return `{"type":"chunk","id":` + strconv.Itoa(id) + `,"` + key + `":` + serialized + "}\n"
}

func serializeNode(n dataNode, reducers []devalue.Reducer) (string, error) {
	switch n.kind {
	case "":
		return "null", nil
	case "skip":
		return `{"type":"skip"}`, nil
	case "error":
		raw, err := jsonBytes(errorNodeEnvelope{Type: "error", Error: n.err})
		if err != nil {
			return "", err
		}
		return string(raw), nil
	}

	data, err := devalue.StringifyWith(n.data, reducers)
	if err != nil {
		return "", err
	}
	uses, err := jsonBytes(n.uses.serialize())
	if err != nil {
		return "", err
	}
	return `{"type":"data","data":` + data + `,"uses":` + string(uses) + `}`, nil
}

func (ls *Loads) writeRedirect(w http.ResponseWriter, shared *loadRequest, redirect *Redirect) {
	h := ls.header(w)
	if shared != nil {
		shared.applyTo(h)
	}
	h.Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	writeJSON(w, redirectEnvelope{
		Type:     "redirect",
		Status:   redirect.status(),
		Location: redirect.Location,
	})
}

// The two envelopes kit's client parses out of a data response. They are
// structs rather than maps so the fields land in kit's own order.
type errorNodeEnvelope struct {
	Type  string     `json:"type"`
	Error *HTTPError `json:"error"`
}

type redirectEnvelope struct {
	Type     string `json:"type"`
	Status   int    `json:"status"`
	Location string `json:"location"`
}

func (ls *Loads) header(w http.ResponseWriter) http.Header {
	h := w.Header()
	h.Set("Cache-Control", "private, no-store")
	if ls.cfg.Version != "" {
		h.Set("X-Sveltekit-Version", ls.cfg.Version)
	}
	return h
}

// promiseTable assigns kit's chunk ids. They start at 1: devalue's reducer loop
// ignores a falsy return, so 0 would silently stop being a promise.
type promiseTable struct {
	mu    sync.Mutex
	ids   map[*deferred]int
	order []*deferred
}

func (t *promiseTable) id(d *deferred) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	if id, ok := t.ids[d]; ok {
		return id
	}
	id := len(t.order) + 1
	t.ids[d] = id
	t.order = append(t.order, d)
	return id
}

func flush(w http.ResponseWriter) {
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

type atomicFlag struct {
	mu sync.Mutex
	v  bool
}

func (f *atomicFlag) set() {
	f.mu.Lock()
	f.v = true
	f.mu.Unlock()
}

func (f *atomicFlag) get() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.v
}

// uses is one node's record of what its load read, which is what lets kit's
// client decide, on the next navigation, whether the load has to run again.
type uses struct {
	mu           sync.Mutex
	dependencies []string
	searchParams []string
	params       []string
	parent       bool
	route        bool
	url          bool
	tracking     bool
}

func (u *uses) add(list *[]string, v string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if !u.tracking {
		return
	}
	for _, existing := range *list {
		if existing == v {
			return
		}
	}
	*list = append(*list, v)
}

func (u *uses) flag(f *bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.tracking {
		*f = true
	}
}

// depend records a dependency, which is always recorded: kit's `untrack` turns
// off the implicit tracking, not the explicit call.
func (u *uses) depend(href string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	for _, existing := range u.dependencies {
		if existing == href {
			return
		}
	}
	u.dependencies = append(u.dependencies, href)
}

// serialize is kit's sparse form: empty collections and false flags are left
// out, and the flags travel as the number 1.
func (u *uses) serialize() map[string]any {
	if u == nil {
		return map[string]any{}
	}
	u.mu.Lock()
	defer u.mu.Unlock()

	out := map[string]any{}
	if len(u.dependencies) > 0 {
		out["dependencies"] = u.dependencies
	}
	if len(u.searchParams) > 0 {
		sorted := append([]string(nil), u.searchParams...)
		sort.Strings(sorted)
		out["search_params"] = sorted
	}
	if len(u.params) > 0 {
		sorted := append([]string(nil), u.params...)
		sort.Strings(sorted)
		out["params"] = sorted
	}
	if u.parent {
		out["parent"] = 1
	}
	if u.route {
		out["route"] = 1
	}
	if u.url {
		out["url"] = 1
	}
	return out
}
