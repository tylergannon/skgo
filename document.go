// Server-side rendering: the Go side of SvelteKit's `render_response`.
//
// Kit builds a page document in two halves. One of them executes code — it
// assembles a `Props` linked list and calls Svelte's `render(Root, ...)` — and
// the other is string assembly over data the server already has: the boot
// script, the hydration array, the remote-function results, the head, the
// template. skgo keeps the split exactly there. The first half runs in the SSR
// engine (`internal/ssr`); everything in this file is the second half, and it
// mirrors `packages/kit/src/runtime/server/page/render.js` line for line.
package skgo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"strings"

	"github.com/tylergannon/skgo/internal/kithash"
	"github.com/tylergannon/skgo/internal/remotearg"
	"github.com/tylergannon/skgo/internal/ssr"
)

// SSR renders page documents in this process.
//
// It owns nothing the rest of skgo does not already own: the loads it renders
// with are the same ones that answer `__data.json`, and the remote functions it
// answers during a render are the same ones that answer `/_app/remote/...`. A
// page rendered here and the same page rendered by kit's client from those two
// endpoints are the same page by construction.
type SSR struct {
	loads    *Loads
	remotes  *Remotes
	engine   *ssr.Engine
	info     ManifestSSR
	template string
	// errorPage is kit's `error.html`: the last-resort document, for a request
	// whose error page cannot itself be rendered.
	errorPage string
	base      string
	version   string
	onError   func(routeID string, err error)
}

// SSROptions configures the renderer.
type SSROptions struct {
	// Runtimes bounds how many pages may render at once. Zero means one per
	// CPU. Each runtime holds its own copy of the app's module state, so this
	// is a memory-for-throughput dial and nothing else.
	Runtimes int
	// OnError is told about a failure the visitor will not see, with the route
	// it was for: a render that threw, a load whose value could not be encoded,
	// an error that reached the root layout. The visitor gets kit's own answer
	// to it — the error page, or `error.html` — which says nothing about the
	// cause, so this is the only record. Leaving it nil logs.
	OnError func(routeID string, err error)
}

// NewSSR builds a renderer over an adapter build. It fails if the build carries
// no SSR bundle, if the bundle does not parse, or if it does not come up.
func NewSSR(build fs.FS, m Manifest, loads *Loads, remotes *Remotes, opts SSROptions) (*SSR, error) {
	if m.SSR == nil {
		return nil, errors.New("skgo: this build has no SSR bundle. Rebuild the frontend with an adapter that emits one.")
	}
	info := *m.SSR

	if info.Target != ssrTarget {
		return nil, fmt.Errorf("skgo: the SSR bundle was compiled to %s; skgo runs %s", info.Target, ssrTarget)
	}
	if len(info.Nodes) != len(m.Nodes) {
		return nil, fmt.Errorf("skgo: the build describes %d node(s) for rendering and %d for loading", len(info.Nodes), len(m.Nodes))
	}

	source, err := fs.ReadFile(build, info.Bundle)
	if err != nil {
		return nil, fmt.Errorf("skgo: reading the SSR bundle: %w", err)
	}
	template, err := fs.ReadFile(build, info.Template)
	if err != nil {
		return nil, fmt.Errorf("skgo: reading the document template: %w", err)
	}
	if info.ErrorTemplate == "" {
		return nil, errors.New("skgo: this build names no error.html. Rebuild the frontend with an adapter that emits one.")
	}
	errorPage, err := fs.ReadFile(build, info.ErrorTemplate)
	if err != nil {
		return nil, fmt.Errorf("skgo: reading the error document: %w", err)
	}
	for _, tag := range []string{"%sveltekit.head%", "%sveltekit.body%"} {
		if !strings.Contains(string(template), tag) {
			return nil, fmt.Errorf("skgo: %s is missing %s", info.Template, tag)
		}
	}

	size := opts.Runtimes
	if size <= 0 {
		size = runtime.NumCPU()
	}
	engine, err := ssr.New(info.Bundle, source, size)
	if err != nil {
		return nil, err
	}

	return &SSR{
		loads:     loads,
		remotes:   remotes,
		engine:    engine,
		info:      info,
		template:  string(template),
		errorPage: string(errorPage),
		base:      strings.TrimSuffix(m.Base, "/"),
		version:   m.Version,
		onError:   opts.OnError,
	}, nil
}

// ssrTarget is the ECMAScript version skgo runs. It is a correctness claim
// rather than a preference: below it, private class fields become WeakMap
// lookups and Svelte's server renderer costs several times more.
const ssrTarget = "es2022"

// serve answers one document request, and reports whether it did.
//
// It mirrors `render_page` (packages/kit/src/runtime/server/page/index.js): a
// redirect thrown by a load is a bare 3xx, a load that failed renders the
// nearest `+error.svelte` above it inside the layouts that reach it, a failure
// with no error page above it falls to kit's `error.html`, and a path that
// matched no route is answered the way `respond.js` answers one — the root
// layout with the root error page inside it, at 404.
//
// It declines only where kit renders nothing either: a request that is not a
// document, a route that is an endpoint and nothing else, and a page whose
// branch turns SSR off, which is answered with kit's SPA shell.
func (s *SSR) serve(w http.ResponseWriter, r *http.Request, urlPath string) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}

	routePath := strings.TrimPrefix(urlPath, s.base)
	if routePath == "" {
		routePath = "/"
	}
	req := dataRequest{url: s.pageURL(r, urlPath), routePath: routePath}

	route, params, matched := s.loads.match(routePath)
	if !matched {
		if nonHTMLDestination(r.Header.Get("Sec-Fetch-Dest")) {
			// A missing image, stylesheet or script is not worth a document,
			// and a browser that asked for one cannot read a document anyway.
			// Kit's own answer (`respond.js`) is four words of plain text, and
			// it varies on the header it decided by.
			h := w.Header()
			h.Set("Content-Type", "text/plain; charset=utf-8")
			h.Set("Vary", "Sec-Fetch-Dest")
			http.Error(w, "Not Found", http.StatusNotFound)
			return true
		}
		// `respond.js` answers a path that matched nothing with
		// `respond_with_error(new SvelteKitError(404, 'Not Found', ...))`, so
		// the visitor gets the app's own error page inside the app's own root
		// layout rather than a shell that has to boot before it can say so.
		return s.respondWithError(w, r, req, "", map[string]string{}, &HTTPError{Status: http.StatusNotFound, Message: "Not Found"})
	}
	if !route.hasPage {
		return false
	}

	renderIt, hydrate := s.pageOptions(route)
	if !renderIt {
		return false
	}

	shared, nodes := s.loads.runBranch(r, req, route.id, params, route.branch, nil)

	for _, node := range nodes {
		if node.redir != nil {
			// Kit answers a redirect thrown by a load with a bare 3xx: a
			// Location and no body at all (`runtime/server/utils.js`,
			// `redirect_response`). net/http's own Redirect would write a
			// courtesy anchor tag, which is a document skgo did not render.
			s.writeRedirect(w, shared, node.redir.status(), node.redir.Location)
			return true
		}
	}

	// A load returned a Go value; the engine and the document both need the
	// tree. Encoding it once, here, is what keeps them from disagreeing — and
	// it has to happen here rather than at either end, because this is where
	// the app's transport hook is known.
	//
	// Every deferred value is then settled, before the render starts. A promise
	// in a load's result reaches kit's client as a streamed chunk; a component
	// that renders on the server needs the value itself.
	if err := s.encodeBranch(r.Context(), nodes); err != nil {
		return s.failed(w, r, req, route.id, params, err)
	}

	// filled[i] is kit's `branch[i]` being truthy: a slot that some node
	// occupies. A layout-less directory that declares an `+error.svelte` leaves
	// a hole, and both the compaction and the walk up to an error page rewind
	// past it.
	filled := make([]bool, len(route.nodes))
	for i, index := range route.nodes {
		filled[i] = index >= 0 && index < len(s.info.Nodes)
	}

	for i, node := range nodes {
		if node.kind != "error" || node.err == nil {
			continue
		}
		return s.serveLoadError(w, r, req, route, params, shared, filled, nodes, i, node.err)
	}

	// Kit renders `compact(branch)`: a slot that no layout fills is dropped,
	// and so is its place in `node_ids` and in the hydration array.
	plan := documentPlan{
		routeID: route.id,
		params:  params,
		status:  http.StatusOK,
		hydrate: hydrate,
		errors:  buildErrorChain(filled, route.errors),
	}
	for i, index := range route.nodes {
		if !filled[i] {
			continue
		}
		plan.indices = append(plan.indices, index)
		plan.nodes = append(plan.nodes, nodes[i])
	}
	if len(plan.indices) == 0 {
		return s.failed(w, r, req, route.id, params, errors.New("skgo: the route has no node to render"))
	}
	return s.deliver(w, r, req, shared, plan)
}

// documentPlan is one attempt to turn a branch into a document: which nodes
// render, what each one's load gave back, and what `page.status` and
// `page.error` are while they do.
type documentPlan struct {
	routeID string
	params  map[string]string
	// indices and nodes are the branch with its empty slots removed — kit's
	// `compact(branch)` — and are the same length.
	indices []int
	nodes   []dataNode
	// errors is the `+error.svelte` guarding each of those slots, or nil where
	// a slot has none. It is kit's `error_components`, and it is what makes a
	// component that throws render an error page in place rather than take the
	// whole document down.
	errors []*int
	// status and pageError are what the page sees as `page.status` and
	// `page.error`, and what the response carries. A render may replace both:
	// kit's `transformError` sets them when a boundary catches something.
	status    int
	pageError *ssr.Error
	hydrate   bool
}

// pageOptions reduces `ssr` and `csr` over a route's branch, outermost first,
// with the last node that states an opinion winning (`utils/page_nodes.js`).
func (s *SSR) pageOptions(route *dataRoute) (ssr bool, csr bool) {
	ssr, csr = true, true
	for _, index := range route.nodes {
		if index < 0 || index >= len(s.info.Nodes) {
			continue
		}
		if v := s.info.Nodes[index].SSR; v != nil {
			ssr = *v
		}
		if v := s.info.Nodes[index].CSR; v != nil {
			csr = *v
		}
	}
	return ssr, csr
}

// serveLoadError renders the error branch of a load that threw.
//
// Kit walks outward from the node that failed to the nearest `+error.svelte`
// declared above it and renders that page inside the layouts down to the depth
// that declared it (`page/index.js`, the `nearest_error_pages` loop): the
// layouts below that depth are dropped, and so is their data. A failure with no
// error page above it happened in the root layout, and kit's answer to that is
// `error.html`.
func (s *SSR) serveLoadError(w http.ResponseWriter, r *http.Request, req dataRequest, route *dataRoute, params map[string]string, shared *loadRequest, filled []bool, nodes []dataNode, at int, e *HTTPError) bool {
	for _, candidate := range nearestErrorPages(at, filled, route.errors) {
		if candidate.node < 0 || candidate.node >= len(s.info.Nodes) {
			continue
		}
		plan := documentPlan{
			routeID:   route.id,
			params:    params,
			status:    e.Status,
			pageError: &ssr.Error{Status: e.Status, Message: e.Message},
			// The page's own options are gone with the page. Kit reduces `ssr`
			// and `csr` over the layouts that survive and nothing else
			// (`new PageNodes(layouts.map(...))`), so a leaf that turned
			// hydration off does not leave its error page unable to boot.
			hydrate: true,
		}
		for i := 0; i < candidate.idx && i < len(nodes); i++ {
			if !filled[i] {
				continue
			}
			if v := s.info.Nodes[route.nodes[i]].CSR; v != nil {
				plan.hydrate = *v
			}
			plan.indices = append(plan.indices, route.nodes[i])
			plan.nodes = append(plan.nodes, nodes[i])
		}
		// The error page itself has no load and no data — kit pushes it with
		// `data: null` — so the hydration array carries a null for it and the
		// client puts nothing in that slot.
		plan.indices = append(plan.indices, candidate.node)
		plan.nodes = append(plan.nodes, dataNode{})
		plan.errors = buildErrorChain(allFilled(len(plan.indices)), route.errors)
		return s.deliver(w, r, req, shared, plan)
	}

	// Nothing above the failing node declares an error page, which can only
	// mean the root layout is where it failed.
	s.report(route.id, e)
	return s.staticErrorPage(w, r, e.Status, e.Message)
}

// respondWithError is kit's `respond_with_error`: the root layout with the root
// error page inside it, at the error's status. It answers a path that matched
// no route, and it answers a document that could not be rendered at all.
//
// The two node numbers are hardcoded here because they are hardcoded in kit:
// `generate_manifest` always keeps nodes 0 and 1, "as they are needed for 404
// and root errors", so 0 is the root layout and 1 the root error page in every
// manifest kit writes.
func (s *SSR) respondWithError(w http.ResponseWriter, r *http.Request, req dataRequest, routeID string, params map[string]string, e *HTTPError) bool {
	const rootLayout, rootError = 0, 1
	if len(s.info.Nodes) <= rootError {
		return s.staticErrorPage(w, r, e.Status, e.Message)
	}
	if v := s.info.Nodes[rootLayout].SSR; v != nil && !*v {
		// An app whose root layout turns SSR off renders nothing anywhere, and
		// kit's shell is what answers this request too.
		return false
	}
	hydrate := true
	if v := s.info.Nodes[rootLayout].CSR; v != nil {
		hydrate = *v
	}

	// The root layout's own load still runs: the error page renders inside it,
	// and a layout without its data is not the layout.
	shared, nodes := s.loads.runBranch(r, req, routeID, params, s.loads.rootBranch(), nil)
	if nodes[0].redir != nil {
		// Kit's own noted edge case: the route is a 404 and the root layout
		// redirects the visitor somewhere.
		s.writeRedirect(w, shared, nodes[0].redir.status(), nodes[0].redir.Location)
		return true
	}
	if nodes[0].kind == "error" {
		s.report(routeID, nodes[0].err)
		return s.staticErrorPage(w, r, e.Status, e.Message)
	}
	if err := s.encodeBranch(r.Context(), nodes); err != nil {
		s.report(routeID, err)
		return s.staticErrorPage(w, r, e.Status, e.Message)
	}

	plan := documentPlan{
		routeID:   routeID,
		params:    params,
		status:    e.Status,
		pageError: &ssr.Error{Status: e.Status, Message: e.Message},
		hydrate:   hydrate,
		indices:   []int{rootLayout, rootError},
		nodes:     []dataNode{nodes[0], {}},
		// Kit passes `error_components: []` here. Nothing above the root error
		// page could catch a throw from it, so a throw is the static page.
		errors: []*int{nil, nil},
	}

	result, answers, err := s.renderPlan(r, req, plan)
	if err != nil {
		s.report(routeID, err)
		return s.staticErrorPage(w, r, e.Status, e.Message)
	}
	if result.Redirect != nil {
		s.writeRedirect(w, shared, result.Redirect.Status, result.Redirect.Location)
		return true
	}
	plan.status, plan.pageError = result.Status, result.Error
	document, err := s.assemble(req, plan, result, answers)
	if err != nil {
		s.report(routeID, err)
		return s.staticErrorPage(w, r, e.Status, e.Message)
	}
	s.write(w, r, shared, document, plan.status)
	return true
}

// failed is `render_page`'s catch: the data loaded but the document could not
// be produced. Kit answers with `respond_with_error`, which renders the root
// layout and the root error page, and falls to `error.html` if even that cannot
// be produced. Either way the visitor gets a whole document with the status kit
// would give it, and never a blank or half-written one.
func (s *SSR) failed(w http.ResponseWriter, r *http.Request, req dataRequest, routeID string, params map[string]string, err error) bool {
	s.report(routeID, err)
	return s.respondWithError(w, r, req, routeID, params, asHTTPError(err))
}

// report tells the app about a failure it will otherwise never see, because the
// visitor is about to be handed a page that does not mention it.
func (s *SSR) report(routeID string, err error) {
	if err == nil {
		return
	}
	if s.onError != nil {
		s.onError(routeID, err)
		return
	}
	log.Printf("skgo: rendering %s: %v", routeID, err)
}

// deliver renders a plan and writes the document it produced.
func (s *SSR) deliver(w http.ResponseWriter, r *http.Request, req dataRequest, shared *loadRequest, plan documentPlan) bool {
	result, answers, err := s.renderPlan(r, req, plan)
	if err != nil {
		return s.failed(w, r, req, plan.routeID, plan.params, err)
	}
	if result.Redirect != nil {
		// A remote function called from a component threw a redirect. Kit turns
		// that into a redirect of the whole document (`render_page`'s catch),
		// and a redirect has no body to render.
		s.writeRedirect(w, shared, result.Redirect.Status, result.Redirect.Location)
		return true
	}
	// `transformError` may have replaced both while the page rendered: a
	// boundary that caught something sets `page.status` and `page.error`, and
	// the response carries what the page ended up showing.
	plan.status, plan.pageError = result.Status, result.Error
	document, err := s.assemble(req, plan, result, answers)
	if err != nil {
		return s.failed(w, r, req, plan.routeID, plan.params, err)
	}
	s.write(w, r, shared, document, plan.status)
	return true
}

// encodeBranch takes every node's load result through the app's transport hook
// and settles every deferred value in it, in place.
func (s *SSR) encodeBranch(ctx context.Context, nodes []dataNode) error {
	transport := s.loads.cfg.Transport
	for i := range nodes {
		if nodes[i].kind != "data" {
			continue
		}
		tree, err := transport.encodeLoadValue(nodes[i].data)
		if err != nil {
			return err
		}
		value, err := settle(ctx, transport, tree)
		if err != nil {
			return err
		}
		nodes[i].data = value
	}
	return nil
}

// renderPlan runs the engine over one plan.
func (s *SSR) renderPlan(r *http.Request, req dataRequest, plan documentPlan) (ssr.Result, map[string]map[string]answered, error) {
	ctx := r.Context()

	branch := make([]ssr.Node, len(plan.indices))
	for i, index := range plan.indices {
		branch[i] = ssr.Node{Index: index, Data: plan.nodes[i].data}
	}

	cookies := map[string]string{}
	for _, cookie := range r.Cookies() {
		cookies[cookie.Name] = cookie.Value
	}

	request, err := json.Marshal(ssr.Request{
		URL:             req.url.String(),
		RouteID:         plan.routeID,
		Params:          plan.params,
		Status:          plan.status,
		Error:           plan.pageError,
		Branch:          branch,
		ErrorComponents: plan.errors,
		Cookies:         cookies,
		ClientAddress:   clientAddress(r),
	})
	if err != nil {
		return ssr.Result{}, nil, err
	}

	// The event every remote function called during this render sees. It is
	// derived once and immutable, which is kit's own rule: a query may read a
	// cookie and may not write one.
	event := s.remotes.newEvent(r, false).immutable()
	answers := map[string]map[string]answered{}

	result, _, err := s.engine.Render(request, func(id, payload string) ([]byte, error) {
		return s.answer(withEvent(ctx, event), id, payload, answers)
	})
	return result, answers, err
}

// write sends a rendered document.
func (s *SSR) write(w http.ResponseWriter, r *http.Request, shared *loadRequest, document string, status int) {
	if status <= 0 {
		status = http.StatusOK
	}
	etag := `"` + kithash.Kit(document) + `"`
	header := w.Header()
	if shared != nil {
		shared.applyTo(header)
	}
	header.Set("Content-Type", "text/html; charset=utf-8")
	header.Set("X-Sveltekit-Page", "true")
	// A rendered document carries whatever the visitor is allowed to see, so it
	// is theirs and no shared cache may keep it. `no-cache` still lets the
	// browser revalidate, which is what the ETag is for.
	header.Set("Cache-Control", "private, no-cache")
	if s.version != "" {
		header.Set("X-Sveltekit-Version", s.version)
	}
	header.Set("ETag", etag)

	// Only a 200 is revalidated away: a page that failed keeps the status it
	// earned, because the browser reads the status and not only the body.
	if status == http.StatusOK && etagMatches(r.Header.Get("If-None-Match"), etag) {
		header.Del("Content-Length")
		w.WriteHeader(http.StatusNotModified)
		return
	}

	header.Set("Content-Length", strconv.Itoa(len(document)))
	w.WriteHeader(status)
	if r.Method != http.MethodHead {
		_, _ = w.Write([]byte(document))
	}
}

// writeRedirect answers with a bare 3xx: a Location and no body at all.
func (s *SSR) writeRedirect(w http.ResponseWriter, shared *loadRequest, status int, location string) {
	h := w.Header()
	if shared != nil {
		shared.applyTo(h)
	}
	h.Set("Location", location)
	h.Set("Cache-Control", "private, no-store")
	w.WriteHeader(status)
}

// staticErrorPage is kit's `static_error_page` (`runtime/server/errors.js`):
// the `error.html` the build carries, with the status and the message
// substituted into it. It is the answer when no component can be trusted to
// render — an error in the root layout, or an engine that did not finish — and
// it carries no script, so nothing boots and nothing tries again.
func (s *SSR) staticErrorPage(w http.ResponseWriter, r *http.Request, status int, message string) bool {
	if status <= 0 {
		status = http.StatusInternalServerError
	}
	page := strings.ReplaceAll(s.errorPage, "%sveltekit.status%", strconv.Itoa(status))
	page = strings.ReplaceAll(page, "%sveltekit.error.message%", escapeHTMLText(message))

	header := w.Header()
	header.Set("Content-Type", "text/html; charset=utf-8")
	header.Set("Cache-Control", "private, no-store")
	header.Set("Content-Length", strconv.Itoa(len(page)))
	w.WriteHeader(status)
	if r.Method != http.MethodHead {
		_, _ = io.WriteString(w, page)
	}
	return true
}

// errorPage is one candidate `+error.svelte`: the node that renders it, and how
// many branch slots survive above it.
type errorPage struct {
	node int
	idx  int
}

// nearestErrorPages walks up from the branch slot at i through the error pages
// declared strictly above it, nearest first, rewinding past empty slots so that
// a page attaches at the layout it belongs to. It is kit's own generator
// (`runtime/error-chain.js`, `nearest_error_pages`).
func nearestErrorPages(i int, filled []bool, errors []int) []errorPage {
	var out []errorPage
	for i > 0 {
		i--
		if i >= len(errors) || errors[i] < 0 {
			continue
		}
		j := i
		for j > 0 && !filled[j] {
			j--
		}
		out = append(out, errorPage{node: errors[i], idx: j + 1})
	}
	return out
}

// buildErrorChain resolves the `+error.svelte` that guards each node of a
// branch, aligned to the branch with its empty slots removed. It is kit's own
// (`runtime/error-chain.js`, `build_error_chain`): a node is guarded by the
// closest error page declared at or above it, except the root layout, which
// wraps the root error page rather than being wrapped by it.
func buildErrorChain(filled []bool, errors []int) []*int {
	chain := []*int{nil}
	lastIdx := -1
	for i := 1; i < len(filled); i++ {
		if !filled[i] {
			continue
		}
		j := i - 1
		for j > lastIdx+1 && (j >= len(errors) || errors[j] < 0) {
			j--
		}
		lastIdx = j
		var guard *int
		if j >= 0 && j < len(errors) && errors[j] >= 0 {
			node := errors[j]
			guard = &node
		}
		chain = append(chain, guard)
	}
	return chain
}

// allFilled is the occupancy of a branch that has already been compacted.
func allFilled(n int) []bool {
	filled := make([]bool, n)
	for i := range filled {
		filled[i] = true
	}
	return filled
}

// escapeHTMLText is kit's `escape_html` for text content (`utils/escape.js`):
// only `&` and `<` carry meaning there.
func escapeHTMLText(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;").Replace(s)
}

// nonHTMLDestination reports that the browser asked for something that is not a
// document. It is kit's own set (`runtime/server/respond.js`).
func nonHTMLDestination(dest string) bool {
	switch dest {
	case "audio", "audioworklet", "font", "image", "json", "manifest",
		"paintworklet", "report", "script", "serviceworker", "sharedworker",
		"style", "track", "video", "webidentity", "worker", "xslt":
		return true
	}
	return false
}

// pageURL is the URL the page was asked for, resolved against the app's
// configured origin the same way a data request's is.
func (s *SSR) pageURL(r *http.Request, urlPath string) *url.URL {
	origin := s.loads.origin
	if origin == nil {
		origin = &url.URL{Scheme: "http", Host: r.Host}
		if r.TLS != nil {
			origin.Scheme = "https"
		}
	}
	return &url.URL{Scheme: origin.Scheme, Host: origin.Host, Path: urlPath, RawQuery: r.URL.RawQuery}
}

// answer runs one remote function the render asked for and records what it gave
// back, so that the document can carry the same value to the browser under the
// key kit's client will look it up by.
func (s *SSR) answer(ctx context.Context, id, payload string, into map[string]map[string]answered) ([]byte, error) {
	fn, ok := s.remotes.Lookup(id)
	if !ok {
		// Not a Go error: kit answers an unknown remote function with a 404
		// envelope, and the component's boundary is where that belongs.
		return json.Marshal(remoteAnswer{E: &ssr.Error{Status: 404, Message: "Error: 404"}})
	}

	kind := map[remoteKind]string{kindQuery: "q", kindLive: "l", kindForm: "f"}[fn.kind]

	arg, present, err := remotearg.ParsePayload(payload)
	if err != nil {
		return json.Marshal(remoteAnswer{E: &ssr.Error{Status: 400, Message: "Bad Request"}})
	}

	value, err := s.remotes.call(ctx, fn, arg, present)
	if err != nil {
		if redirect := asRedirect(err); redirect != nil {
			// A redirect thrown by a remote function during a render is a
			// redirect of the whole document: the bundle rethrows it as kit's
			// own Redirect, `transformError` refuses to swallow it, and
			// `render_page`'s catch turns it into a bare 3xx. Nothing is
			// recorded for the boot script, because there is no document.
			return json.Marshal(remoteAnswer{R: &ssr.Redirect{Status: redirect.status(), Location: redirect.Location}})
		}
		e := asHTTPError(err)
		failure := &ssr.Error{Status: e.Status, Message: e.Message}
		s.record(into, kind, id+"/"+payload, answered{err: failure})
		return json.Marshal(remoteAnswer{E: failure})
	}

	// The engine gets JSON, because a component in it has no way to be handed
	// anything else. The document gets the Go value, kept whole so that the
	// transport hook can still see a custom type in it when the boot script is
	// written.
	raw, err := json.Marshal(value)
	if err != nil {
		return json.Marshal(remoteAnswer{E: &ssr.Error{Status: 500, Message: "Internal Error"}})
	}
	s.record(into, kind, id+"/"+payload, answered{value: value})
	return json.Marshal(remoteAnswer{V: raw})
}

// record files an answer under kit's own key: the single letter of the remote
// function's kind, then `<hash>/<name>/<payload>`. A kind with no letter — a
// command, which kit refuses during a render — is not recorded at all, which is
// what makes an entry with neither a value nor an error impossible.
func (s *SSR) record(into map[string]map[string]answered, kind, key string, answer answered) {
	if kind == "" {
		return
	}
	if into[kind] == nil {
		into[kind] = map[string]answered{}
	}
	into[kind][key] = answer
}

// answered is what one remote function gave back during a render, held as the
// Go value rather than as JSON so that the document can write it with the app's
// transport hook in hand.
type answered struct {
	value any
	err   *ssr.Error
}

// remoteAnswer is the envelope the SSR bundle parses: a value, an error, or a
// redirect of the whole document.
type remoteAnswer struct {
	V json.RawMessage `json:"v,omitempty"`
	E *ssr.Error      `json:"e,omitempty"`
	R *ssr.Redirect   `json:"r,omitempty"`
}

// settle replaces every Deferred in an encoded load result with the value it
// was waiting for.
//
// A Deferred holds the raw Go value the load produced — encoding it when it was
// created would have flattened a transported type before any encoder saw it —
// so the settled value is encoded here, with the same transport the rest of the
// tree was encoded with.
func settle(ctx context.Context, transport Transport, v any) (any, error) {
	switch value := v.(type) {
	case *deferred:
		settled, err := value.wait(ctx)
		if err != nil {
			return nil, err
		}
		tree, err := transport.encodeTree(settled)
		if err != nil {
			return nil, err
		}
		return settle(ctx, transport, tree)
	case map[string]any:
		for key, item := range value {
			resolved, err := settle(ctx, transport, item)
			if err != nil {
				return nil, err
			}
			value[key] = resolved
		}
		return value, nil
	case []any:
		for i, item := range value {
			resolved, err := settle(ctx, transport, item)
			if err != nil {
				return nil, err
			}
			value[i] = resolved
		}
		return value, nil
	}
	if holder, ok := v.(deferredHolder); ok && holder.deferredValue() != nil {
		return settle(ctx, transport, holder.deferredValue())
	}
	return v, nil
}

func clientAddress(r *http.Request) string {
	host, _, err := splitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func splitHostPort(addr string) (string, string, error) {
	i := strings.LastIndex(addr, ":")
	if i < 0 {
		return "", "", errors.New("no port")
	}
	return strings.Trim(addr[:i], "[]"), addr[i+1:], nil
}
