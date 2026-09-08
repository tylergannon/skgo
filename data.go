package skgo

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/tylergannon/polytype/devalue"
	"github.com/tylergannon/skgo/internal/ssr"
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

// Intercept returns a handler that answers `__data.json` itself and passes
// everything else to next.
//
// It must sit in front of whatever serves pages — the static handler in
// production, the dev proxy in dev — so that `__data.json` never reaches
// either of those, and in front of the remote registry too, since a `__data`
// suffix and a remote call never share a path. Running the app's `handle`
// hook is not this registry's business; see Handle, which mounts outermost
// over this and everything else.
func (ls *Loads) Intercept(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hasDataSuffix(r.URL.Path) {
			ls.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
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
	nodes := ls.table.Load().nodes
	if len(nodes) == 0 {
		return []*ServerLoad{nil}
	}
	return []*ServerLoad{nodes[0]}
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
	kind string
	data any
	uses *uses
	err  *HTTPError
	// raw is the error the load actually returned, before asHTTPError
	// collapsed it — nil unless err is. It survives so both the document and
	// `__data.json` paths can tell an app's own Errorf apart from an ordinary
	// Go error when they consult the app's handleError hook.
	raw error
	// handled is the App.Error after the app's handleError hook. It is filled
	// only while writing `__data.json`; the document path starts from the same
	// raw error and consults the same hook immediately before rendering.
	handled *ssr.Error
	redir   *Redirect
}

// serveBranch runs a route's branch and writes the response.
func (ls *Loads) serveBranch(w http.ResponseWriter, r *http.Request, req dataRequest, routeID string, params map[string]string, branch []*ServerLoad, invalidated []bool) {
	shared, nodes := ls.runBranch(r, req, routeID, params, branch, invalidated)

	// A redirect anywhere in the branch is the whole answer. Kit's `Promise.all`
	// rejects on the first one to arrive; taking the outermost keeps it
	// deterministic, and it is the one a guard on a layout means.
	for _, n := range nodes {
		if n.redir != nil {
			ls.writeRedirect(w, shared, n.redir)
			return
		}
	}

	ls.writeNodes(w, r, routeID, shared, nodes)
}

// runBranch runs a route's branch and returns what each node produced, without
// deciding how it is answered. A `__data.json` request serializes the nodes; a
// document request hands them to the renderer and then serializes the same
// nodes into the page's hydration array, so both go through here and neither
// can drift from the other.
func (ls *Loads) runBranch(r *http.Request, req dataRequest, routeID string, params map[string]string, branch []*ServerLoad, invalidated []bool) (*loadRequest, []dataNode) {
	return ls.runBranchWith(r, req, routeID, params, branch, invalidated, nil)
}

// runBranchWith is runBranch over a cookie jar that already exists. A form
// submission runs before the loads and may write cookies; sharing its jar is
// what makes a load that runs after it read what it wrote, which is what kit's
// single `event.cookies` gives.
func (ls *Loads) runBranchWith(r *http.Request, req dataRequest, routeID string, params map[string]string, branch []*ServerLoad, invalidated []bool, jar *cookieJar) (*loadRequest, []dataNode) {
	ctx := r.Context()

	if jar == nil {
		jar = newCookieJar(r, secureCookieDefault(ls.cfg.Origin, ls.cfg.Dev))
	}
	shared := &loadRequest{
		req:     r,
		jar:     jar,
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
			value, err := ls.runLoad(withEvent(ctx, e), load)
			if err != nil {
				aborted.set()
				if redirect := asRedirect(err); redirect != nil {
					return dataNode{kind: "error", redir: redirect}
				}
				return dataNode{kind: "error", err: asHTTPError(err), raw: err}
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

	return shared, nodes
}

// writeNodes serializes the branch and writes it, streaming the deferred values
// as they settle.
func (ls *Loads) writeNodes(w http.ResponseWriter, r *http.Request, routeID string, shared *loadRequest, nodes []dataNode) {
	hookCtx := hookContext(r, shared)
	promises := &promiseTable{ids: map[*deferred]int{}}
	// The app's transport hook first, then skgo's own Promise reducer for a
	// deferred value — the order kit's `{ ...encoders, ... }` spread produces.
	// Nothing turns on it here (a *deferred is not a value any transporter
	// claims), but the two sites that serialize towards the browser should not
	// disagree about precedence.
	reducers := append(ls.cfg.Transport.reducers(), promiseReducer(promises))

	parts := make([]string, len(nodes))
	for i, n := range nodes {
		if n.kind == "error" && n.err != nil {
			n.handled = ls.dataError(hookCtx, routeID, n.err, n.raw)
		}
		serialized, err := serializeNode(n, reducers, ls.cfg.Transport)
		if err != nil {
			ls.writeFatalError(w, r, shared, ls.dataError(hookCtx, routeID, asHTTPError(err), err))
			return
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
	//
	// Settlement order, not id order: kit's iterator gives a settling promise
	// the next free slot (`utils/streaming.js`), so a fast second value is
	// never held behind a slow first one. A client-side navigation therefore
	// fills the page in the same order a cold load does.
	promises.settled(r.Context(), func(id int, value any, err error) {
		w.Write([]byte(ls.chunkLine(hookCtx, routeID, id, value, err, reducers, ls.cfg.Transport)))
		flush(w)
	})
}

func (ls *Loads) chunkLine(ctx context.Context, routeID string, id int, value any, err error, reducers []devalue.Reducer, transport Transport) string {
	key, payload := "data", any(nil)
	if err == nil {
		// The Deferred held the raw Go value so that this is the first place it
		// is encoded, with the transport hook in hand. encodeLoadValue rather
		// than encodeTree because a promised value may itself hold a promise:
		// kit writes each chunk with the same reducers it wrote the first one
		// with, so a nested promise is simply another chunk on the same
		// response.
		payload, err = transport.encodeLoadValue(value)
	}
	if err != nil {
		key, payload = "error", errorNodeExtra(ls.dataError(ctx, routeID, asHTTPError(err), err))
	}
	serialized, serr := devalue.StringifyWith(payload, reducers)
	if serr != nil {
		key = "error"
		failure := fmt.Errorf("failed to serialize deferred value while rendering %s: %w", routeID, serr)
		serialized, serr = devalue.StringifyWith(errorNodeExtra(ls.dataError(ctx, routeID, asHTTPError(failure), failure)), reducers)
		if serr != nil {
			serialized, _ = devalue.StringifyWith(errorNode(Errorf(500, "Internal Error")), nil)
		}
	}
	return `{"type":"chunk","id":` + strconv.Itoa(id) + `,"` + key + `":` + serialized + "}\n"
}

// writeFatalError is kit's outer render_data catch: an error creating the
// data envelope is itself passed through handleError and returned as App.Error
// with its chosen status, rather than disguised as one of the branch's nodes.
func (ls *Loads) writeFatalError(w http.ResponseWriter, r *http.Request, shared *loadRequest, e *ssr.Error) {
	raw, err := jsonBytes(errorNodeExtra(e))
	if err != nil {
		e = &ssr.Error{Status: http.StatusInternalServerError, Message: "Internal Error"}
		raw, _ = jsonBytes(errorNodeExtra(e))
	}
	h := ls.header(w)
	if shared != nil {
		shared.applyTo(h)
	}
	h.Set("Content-Type", "application/json")
	h.Set("Content-Length", strconv.Itoa(len(raw)))
	w.WriteHeader(e.Status)
	if r.Method != http.MethodHead {
		_, _ = w.Write(raw)
	}
}

func serializeNode(n dataNode, reducers []devalue.Reducer, transport Transport) (string, error) {
	switch n.kind {
	case "":
		return "null", nil
	case "skip":
		return `{"type":"skip"}`, nil
	case "error":
		e := n.handled
		if e == nil {
			e = mergeCaughtError(n.err, nil)
		}
		raw, err := jsonBytes(errorNodeEnvelope{Type: "error", Error: errorNodeExtra(e)})
		if err != nil {
			return "", err
		}
		return string(raw), nil
	}

	// The load returned its raw Go value; this is where it becomes a tree,
	// because this is where the transport hook is known.
	tree, err := transport.encodeLoadValue(n.data)
	if err != nil {
		return "", err
	}
	data, err := devalue.StringifyWith(tree, reducers)
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
	Type  string          `json:"type"`
	Error *devalue.Object `json:"error"`
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

// promiseReducer is kit's `Promise` reducer (`server_data_serializer_json`): a
// promise becomes its id, and the id is what the client's own `Promise` reviver
// keys the promise it makes on. devalue walks the whole tree, so a promised
// value is found wherever the load left one.
func promiseReducer(promises *promiseTable) devalue.Reducer {
	return devalue.Reducer{
		Key: "Promise",
		Fn: func(v any) (any, bool, error) {
			d, ok := v.(*deferred)
			if !ok {
				return nil, false, nil
			}
			return float64(promises.id(d)), true, nil
		},
	}
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

// settled calls out with each promise as it settles, which is the order kit
// sends them in: its iterator hands a settling promise the next free slot
// rather than its own (`utils/streaming.js`), so the first value to arrive is
// the first one written. The ids are what let the browser tell them apart, and
// they travel with each chunk.
//
// The table may grow while this runs — a chunk kit writes can itself introduce
// a promise — so the loop re-reads it rather than iterating a snapshot. It
// returns when every promise the table holds has been written, or when ctx is
// done, which is a visitor who closed the tab.
func (t *promiseTable) settled(ctx context.Context, out func(id int, value any, err error)) {
	type result struct {
		id    int
		value any
		err   error
	}
	results := make(chan result)

	started, written := 0, 0
	for {
		t.mu.Lock()
		waiting := append([]*deferred(nil), t.order[started:]...)
		ids := make([]int, len(waiting))
		for i, d := range waiting {
			ids[i] = t.ids[d]
		}
		started = len(t.order)
		t.mu.Unlock()

		for i, d := range waiting {
			id := ids[i]
			go func() {
				value, err := d.wait(ctx)
				select {
				case results <- result{id: id, value: value, err: err}:
				case <-ctx.Done():
				}
			}()
		}

		if written == started {
			return
		}
		select {
		case r := <-results:
			written++
			out(r.id, r.value, r.err)
		case <-ctx.Done():
			return
		}
	}
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

// usesWire is kit's sparse form: empty collections and false flags are left
// out, the flags travel as the number 1, and the fields keep kit's own order
// (`runtime/server/utils.js`, serialize_uses).
type usesWire struct {
	Dependencies []string `json:"dependencies,omitempty"`
	SearchParams []string `json:"search_params,omitempty"`
	Params       []string `json:"params,omitempty"`
	Parent       int      `json:"parent,omitempty"`
	Route        int      `json:"route,omitempty"`
	URL          int      `json:"url,omitempty"`
}

// serialize records what the load read, in the order it read it — kit's sets
// preserve insertion order too.
func (u *uses) serialize() usesWire {
	if u == nil {
		return usesWire{}
	}
	u.mu.Lock()
	defer u.mu.Unlock()

	out := usesWire{
		Dependencies: u.dependencies,
		SearchParams: u.searchParams,
		Params:       u.params,
	}
	if u.parent {
		out.Parent = 1
	}
	if u.route {
		out.Route = 1
	}
	if u.url {
		out.URL = 1
	}
	return out
}
