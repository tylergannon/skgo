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
	"sort"
	"strconv"
	"strings"

	"github.com/tylergannon/skgo/internal/devalue"
	"github.com/tylergannon/skgo/internal/kithash"
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
	// fetch answers a render-time `event.fetch` of the app's own routes. It is
	// the server-route registry's own Intercept, with nothing beneath it: a
	// fetch that matches no `+server.ts` refuses rather than falling through
	// to a page, because a page's own render is what asked for this fetch and
	// recursing back into the SSR engine can starve the runtime pool it is
	// still holding a runtime from. See SSROptions.Fetch.
	fetch http.Handler
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
	// Fetch answers a render-time `event.fetch` of the app's own routes: the
	// registered `+server.ts` handlers, dispatched in-process rather than over
	// a socket. Pass `endpoints.Intercept(http.NotFoundHandler())` — the same
	// registry the server itself answers `+server.ts` requests with, refusing
	// rather than falling through to a page, because the page whose render
	// asked for this fetch is itself holding a runtime out of the pool a
	// recursive render would need. Leaving it nil refuses every render-time
	// fetch, which is what an app with no server routes gets.
	//
	// Same-origin is enforced before Fetch is ever called: the bundle's own
	// `event.fetch` refuses a cross-origin URL itself, mirroring kit's rule
	// that a render-time fetch resolves against the page's own origin — so
	// Fetch only ever sees a request for one of this app's own routes.
	Fetch http.Handler
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
	if info.CSP != nil {
		if err := validateReportOnly(info.CSP.ReportOnly); err != nil {
			return nil, err
		}
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
	s := &SSR{
		loads:     loads,
		remotes:   remotes,
		info:      info,
		template:  string(template),
		errorPage: string(errorPage),
		base:      strings.TrimSuffix(m.Base, "/"),
		version:   m.Version,
		onError:   opts.OnError,
		fetch:     opts.Fetch,
	}
	// The engine is built after the SSR rather than into it because the bundle
	// writes to `console` while it is coming up, and that line has to reach the
	// same place every other failure does.
	engine, err := ssr.New(info.Bundle, source, size, s.console)
	if err != nil {
		return nil, err
	}
	s.engine = engine
	return s, nil
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
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodPost {
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
		return s.respondWithError(w, r, req, "", map[string]string{}, &HTTPError{Status: http.StatusNotFound, Message: "Not Found"}, nil)
	}
	if !route.hasPage {
		return false
	}

	// A POST to a page is a form submission, and it runs before anything is
	// loaded: kit's `render_page` calls `handle_remote_form_post` first, then
	// runs the loads, so the page the visitor gets back is rendered over the
	// state the submission left behind rather than the state before it.
	var action *formAction
	if r.Method == http.MethodPost {
		id := actionID(req.url)
		if id == "" {
			// Kit's `handle_action_request` with no `actions` export: a page
			// that has no classic form action, which for skgo is every page.
			// `method_not_allowed_result` (`runtime/server/page/actions.js:90-94`)
			// sets `allow: 'GET'` — RFC 9110 requires a 405 to carry one.
			w.Header().Set("Allow", "GET")
			return s.respondWithError(w, r, req, route.id, params, &HTTPError{
				Status:  405,
				Message: "POST method not allowed. No form actions exist for this page",
			}, nil)
		}
		submitted, redirect, e := s.runFormAction(r, id)
		if redirect != nil {
			s.writeRedirect(w, nil, redirect.status(), redirect.Location)
			return true
		}
		if e != nil {
			if e.Status == http.StatusMethodNotAllowed {
				// The other `method_not_allowed_result`: kit's
				// `handle_remote_form_post_internal`
				// (`runtime/server/remote-functions.js:551`) answers an id
				// that names no form the same way, `allow: 'GET'` included.
				w.Header().Set("Allow", "GET")
			}
			return s.respondWithError(w, r, req, route.id, params, e, nil)
		}
		action = submitted
	}

	renderIt, hydrate := s.pageOptions(route)
	if !renderIt {
		return false
	}

	shared, nodes := s.loads.runBranchWith(r, req, route.id, params, route.branch, nil, jarOf(action))

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
	if err := s.encodeBranch(nodes); err != nil {
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
		return s.serveLoadError(w, r, req, route, params, shared, filled, nodes, i, node.err, node.raw)
	}

	// Kit renders `compact(branch)`: a slot that no layout fills is dropped,
	// and so is its place in `node_ids` and in the hydration array.
	plan := documentPlan{
		routeID: route.id,
		params:  params,
		status:  http.StatusOK,
		hydrate: hydrate,
		errors:  buildErrorChain(filled, route.errors),
		action:  action,
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
	// action is the form submission this document is answering, or nil. It
	// reaches the engine as the form's cached output and the document as the
	// `f` entry of `<global>.data`.
	action *formAction
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
func (s *SSR) serveLoadError(w http.ResponseWriter, r *http.Request, req dataRequest, route *dataRoute, params map[string]string, shared *loadRequest, filled []bool, nodes []dataNode, at int, e *HTTPError, raw error) bool {
	// The app's handleError hook, if it has one, is consulted here — kit's
	// `handle_error_and_jsonify` runs for every error that reaches a rendered
	// document, expected or not (`page/index.js`), and what it returns is
	// merged over the load's own status and message before the error page
	// ever sees either.
	pageError := s.documentError(hookContext(r, shared), route.id, e, raw)
	for _, candidate := range nearestErrorPages(at, filled, route.errors) {
		if candidate.node < 0 || candidate.node >= len(s.info.Nodes) {
			continue
		}
		plan := documentPlan{
			routeID:   route.id,
			params:    params,
			status:    pageError.Status,
			pageError: pageError,
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
	if raw != nil {
		s.report(route.id, raw)
	} else {
		s.report(route.id, e)
	}
	return s.staticErrorPage(w, r, pageError.Status, pageError.Message)
}

// respondWithError is kit's `respond_with_error`: the root layout with the root
// error page inside it, at the error's status. It answers a path that matched
// no route, and it answers a document that could not be rendered at all.
//
// The two node numbers are hardcoded here because they are hardcoded in kit:
// `generate_manifest` always keeps nodes 0 and 1, "as they are needed for 404
// and root errors", so 0 is the root layout and 1 the root error page in every
// manifest kit writes.
func (s *SSR) respondWithError(w http.ResponseWriter, r *http.Request, req dataRequest, routeID string, params map[string]string, e *HTTPError, raw error) bool {
	const rootLayout, rootError = 0, 1

	// Kit consults the hook once, at the very top — "do this here first in
	// case the awaits below before rendering themselves error"
	// (`respond_with_error.js`) — and uses what it returns for every branch
	// below, including the static page a further failure falls back to. The
	// event it hands the hook is the request's own, built ahead of the root
	// layout's `shared` because kit's is available before the layout ever
	// runs too.
	hookRequest := &loadRequest{
		req: r,
		jar: newCookieJar(r, secureCookieDefault(s.loads.cfg.Origin, s.loads.cfg.Dev)),
		url: req.url, routeID: routeID, params: params,
	}
	pageError := s.documentError(hookContext(r, hookRequest), routeID, e, raw)

	if len(s.info.Nodes) <= rootError {
		return s.staticErrorPage(w, r, pageError.Status, pageError.Message)
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
		return s.staticErrorPage(w, r, pageError.Status, pageError.Message)
	}
	if err := s.encodeBranch(nodes); err != nil {
		s.report(routeID, err)
		return s.staticErrorPage(w, r, pageError.Status, pageError.Message)
	}

	plan := documentPlan{
		routeID:   routeID,
		params:    params,
		status:    pageError.Status,
		pageError: pageError,
		hydrate:   hydrate,
		indices:   []int{rootLayout, rootError},
		nodes:     []dataNode{nodes[0], {}},
		// Kit passes `error_components: []` here. Nothing above the root error
		// page could catch a throw from it, so a throw is the static page.
		errors: []*int{nil, nil},
	}

	// kit builds `csp` before it ever calls `render_page` — the nonce has to
	// exist before Svelte's own render call, which runs inside renderPlan,
	// before assemble does. The same object is handed to both, below.
	csp, err := newDocumentCSP(s.info.CSP)
	if err != nil {
		s.report(routeID, err)
		return s.staticErrorPage(w, r, pageError.Status, pageError.Message)
	}
	result, answers, err := s.renderPlan(r, req, plan, csp)
	if err != nil {
		s.report(routeID, err)
		return s.staticErrorPage(w, r, pageError.Status, pageError.Message)
	}
	if result.Redirect != nil {
		s.writeRedirect(w, shared, result.Redirect.Status, result.Redirect.Location)
		return true
	}
	plan.status, plan.pageError = result.Status, result.Error
	document, promises, headers, err := s.assemble(req, plan, result, answers, csp)
	if err != nil {
		s.report(routeID, err)
		return s.staticErrorPage(w, r, pageError.Status, pageError.Message)
	}
	if len(promises.order) > 0 {
		s.stream(w, r, shared, document, promises, headers)
		return true
	}
	s.write(w, r, shared, document, plan.status, headers)
	return true
}

// failed is `render_page`'s catch: the data loaded but the document could not
// be produced. Kit answers with `respond_with_error`, which renders the root
// layout and the root error page, and falls to `error.html` if even that cannot
// be produced. Either way the visitor gets a whole document with the status kit
// would give it, and never a blank or half-written one.
func (s *SSR) failed(w http.ResponseWriter, r *http.Request, req dataRequest, routeID string, params map[string]string, err error) bool {
	s.report(routeID, err)
	return s.respondWithError(w, r, req, routeID, params, asHTTPError(err), err)
}

// report tells the app about a failure it will otherwise never see, because the
// visitor is about to be handed a page that does not mention it.
// console is where the engine's `console` lands.
//
// A warning or an error is a failure the visitor will not see — kit's
// `log_handle_error_hook_failure` and Svelte's own warnings both come through
// here — so it goes wherever every other such failure goes, which is OnError if
// the app named one. Anything quieter is a line the app itself wrote, and it
// just gets logged.
func (s *SSR) console(routeID, level, text string) {
	switch level {
	case "warn", "error":
		s.report(routeID, fmt.Errorf("console.%s: %s", level, text))
	default:
		log.Printf("skgo: console.%s: %s", level, text)
	}
}

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
	// Built before renderPlan, not assemble: the nonce has to exist before
	// the engine's own Svelte render call, and the same object is what
	// assemble later hashes/nonces the boot script against — see
	// respondWithError's identical comment.
	csp, err := newDocumentCSP(s.info.CSP)
	if err != nil {
		return s.failed(w, r, req, plan.routeID, plan.params, err)
	}
	result, answers, err := s.renderPlan(r, req, plan, csp)
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
	document, promises, headers, err := s.assemble(req, plan, result, answers, csp)
	if err != nil {
		return s.failed(w, r, req, plan.routeID, plan.params, err)
	}
	if len(promises.order) > 0 {
		s.stream(w, r, shared, document, promises, headers)
		return true
	}
	s.write(w, r, shared, document, plan.status, headers)
	return true
}

// encodeBranch takes every node's load result through the app's transport hook,
// in place.
//
// A deferred value is left exactly where it is. Kit hands its renderer the
// promise a load returned and lets `{#await}` render the pending branch, so
// waiting for one here would be the single change that makes a document unable
// to stream — the page would arrive whole and late instead of at once and in
// two pieces.
func (s *SSR) encodeBranch(nodes []dataNode) error {
	transport := s.loads.cfg.Transport
	for i := range nodes {
		if nodes[i].kind != "data" {
			continue
		}
		tree, err := transport.encodeLoadValue(nodes[i].data)
		if err != nil {
			return err
		}
		nodes[i].data = tree
	}
	return nil
}

// renderPlan runs the engine over one plan.
//
// csp is this request's already-built documentCSP (see respondWithError and
// deliver): its ScriptNeedsNonce/ScriptNeedsHash are decided entirely by the
// build's configured directives, so they are known — and the request payload
// below carries them — before this render has produced any script content at
// all. That is kit's own `csp: csp.script_needs_nonce ? { nonce: csp.nonce }
// : { hash: csp.script_needs_hash }` (render.js:198), passed to Svelte's own
// render call so the one inline script Svelte can still emit on its own — a
// hydratable-async-block script (Svelte's `internal/server/renderer.js`, for
// a component's own top-level `await`) — gets nonced or hashed the same way
// the boot script skgo assembles itself does.
func (s *SSR) renderPlan(r *http.Request, req dataRequest, plan documentPlan, csp *documentCSP) (ssr.Result, map[string]map[string]answered, error) {
	ctx := r.Context()

	// Each node's data crosses into the engine in devalue's flat form, which
	// is the same form `__data.json` carries and produced by the same encoders.
	// The engine parses it with the app's own `transport` decoders, so the
	// value a component renders against is the instance the browser will hold.
	//
	// A slot the plan filled with no load — an error page's own place in the
	// branch — has no data and crosses as "".
	//
	// A value the load promised crosses as kit's own promise placeholder —
	// `["Promise", <id>]` in devalue's flat form, exactly what `__data.json`
	// carries — and the bundle reads it back with a `Promise` reviver, the way
	// kit's client does in `process_stream`. So a promise is found wherever it
	// sits, at any depth, without either side agreeing on a list of names.
	//
	// The ids never leave the engine: the bundle puts a promise that never
	// settles at each of them, because Svelte's server renderer does not await
	// an `{#await}` block — it renders the pending branch — and the value
	// follows the document down as a chunk. So this table is its own, and its
	// numbering has nothing to do with the one the document's chunks use.
	promises := &promiseTable{ids: map[*deferred]int{}}
	reducers := append(s.loads.cfg.Transport.reducers(), promiseReducer(promises))
	branch := make([]ssr.Node, len(plan.indices))
	for i, index := range plan.indices {
		data := ""
		if plan.nodes[i].kind == "data" {
			serialized, err := devalue.StringifyWith(plan.nodes[i].data, reducers)
			if err != nil {
				return ssr.Result{}, nil, err
			}
			data = serialized
		}
		branch[i] = ssr.Node{Index: index, Data: data}
	}

	cookies := map[string]string{}
	for _, cookie := range r.Cookies() {
		cookies[cookie.Name] = cookie.Value
	}

	seed, err := s.seed(plan.action)
	if err != nil {
		return ssr.Result{}, nil, err
	}

	requestCSP := ssr.RequestCSP{Hash: csp.ScriptNeedsHash()}
	if csp.ScriptNeedsNonce() {
		requestCSP.Nonce = csp.nonce
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
		FormAction:      seed,
		CSP:             requestCSP,
	})
	if err != nil {
		return ssr.Result{}, nil, err
	}

	// The event every remote function called during this render sees. It is
	// derived once and immutable, which is kit's own rule: a query may read a
	// cookie and may not write one.
	event := s.remotes.newEvent(r, false).immutable()
	answers := map[string]map[string]answered{}
	if plan.action != nil {
		// `collect_remote_data` files a form's output under `f`, keyed by the
		// client-side action id *directly* rather than by
		// `create_remote_key(id, payload)` — a form has no argument to key on,
		// and `form.svelte.js` looks it up under exactly this string.
		s.record(answers, "f", plan.action.id, answered{tree: plan.action.output})
	}

	result, _, err := s.engine.Render(plan.routeID, request, ssr.Hosts{
		Remote: func(id, payload string) ([]byte, error) {
			return s.answer(withEvent(ctx, event), id, payload, answers)
		},
		Fetch: s.fetchDispatch,
		Match: s.matchDispatch,
	})
	return result, answers, err
}

// write sends a rendered document.
func (s *SSR) write(w http.ResponseWriter, r *http.Request, shared *loadRequest, document string, status int, headers documentHeaders) {
	if status <= 0 {
		status = http.StatusOK
	}
	etag := `"` + kithash.Kit(document) + `"`
	header := w.Header()
	if shared != nil {
		shared.applyTo(header)
	}
	setCSPHeaders(header, headers)
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

	if fn.kind == KindBatch {
		return s.answerBatch(ctx, fn, payload, into)
	}

	kind := remoteLetters[fn.kind]

	call, err := s.remotes.parsePayload(payload)
	if err != nil {
		return json.Marshal(remoteAnswer{E: &ssr.Error{Status: 400, Message: "Bad Request"}})
	}

	// A live query answers a render with its first value, which is what kit's
	// `get_first_value` takes: it drives the generator once and closes it. The
	// stream itself is the browser's business, and it opens after hydration.
	value, err := s.remoteValue(ctx, fn, call)
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

	// The engine gets the same bytes `/_app/remote/...` would have sent the
	// browser — devalue's flat form, written from the tree the generated
	// encoder produced — so the component rendering here and the client
	// hydrating it are looking at a value of the same type. The document gets
	// that same tree, which still holds any transported value whole, so the
	// transport hook can see it when the boot script is written.
	transport := s.remotes.cfg.Transport
	serialized, err := devalue.StringifyWith(value, transport.reducers())
	if err != nil {
		return json.Marshal(remoteAnswer{E: &ssr.Error{Status: 500, Message: "Internal Error"}})
	}
	s.record(into, kind, id+"/"+payload, answered{tree: value})
	return json.Marshal(remoteAnswer{V: serialized})
}

// remoteLetters is the bucket a kind's answers are filed under in a document's
// remote data. They are kit's own: `internals.type[0]`, with `query_live`
// special-cased to `l` (`collect_remote_data`,
// runtime/server/remote-functions.js). A batch query is a `query` as far as the
// browser's cache is concerned, so its answers go under `q` beside the plain
// ones — the client resolves both through the same QueryProxy.
var remoteLetters = map[Kind]string{
	KindQuery: "q",
	KindBatch: "q",
	KindLive:  "l",
	KindForm:  "f",
}

// remoteValue runs one remote function for a render. A live query is driven for
// exactly one value; everything else is called once.
func (s *SSR) remoteValue(ctx context.Context, fn *Remote, call Call) (any, error) {
	if fn.kind == KindLive {
		return s.remotes.firstValue(ctx, fn, call)
	}
	return s.remotes.call(ctx, fn, call)
}

// answerBatch runs a whole `query.batch` the render collected. The engine sends
// every payload kit's own `enqueue` gathered in one macrotask, as a JSON array,
// and the app's Go function is called once for all of them — which is the point
// of a batch query and the only thing that distinguishes it from a query.
//
// Each entry is recorded under `q` and its own `<id>/<payload>` key, because
// that is where the browser's query cache will look for it: kit files a
// `query_batch` under the letter its type begins with, and its client holds
// batch results in the ordinary query cache.
func (s *SSR) answerBatch(ctx context.Context, fn *Remote, payload string, into map[string]map[string]answered) ([]byte, error) {
	var payloads []string
	if err := json.Unmarshal([]byte(payload), &payloads); err != nil {
		return json.Marshal(batchAnswer{E: &ssr.Error{Status: 400, Message: "Bad Request"}})
	}

	calls, err := s.remotes.parsePayloads(payloads)
	if err != nil {
		return json.Marshal(batchAnswer{E: &ssr.Error{Status: 400, Message: "Bad Request"}})
	}

	values, err := s.remotes.callBatch(ctx, fn, calls)
	if err != nil {
		if redirect := asRedirect(err); redirect != nil {
			return json.Marshal(batchAnswer{R: &ssr.Redirect{Status: redirect.status(), Location: redirect.Location}})
		}
		e := asHTTPError(err)
		failure := &ssr.Error{Status: e.Status, Message: e.Message}
		// The whole batch failed, so every argument in it failed. Each one is
		// recorded under its own key, because that is where the client will
		// look for it and a missing key hydrates as a value that was never
		// fetched rather than as the failure the render saw.
		for _, one := range payloads {
			s.record(into, "q", fn.id+"/"+one, answered{err: failure})
		}
		return json.Marshal(batchAnswer{E: failure})
	}

	transport := s.remotes.cfg.Transport
	nodes := make([]remoteAnswer, len(values))
	for i, value := range values {
		// callBatch has already taken each value through the transport hook,
		// so what comes back is the tree devalue writes.
		serialized, err := devalue.StringifyWith(value, transport.reducers())
		if err != nil {
			return json.Marshal(batchAnswer{E: &ssr.Error{Status: 500, Message: "Internal Error"}})
		}
		nodes[i] = remoteAnswer{V: serialized}
		s.record(into, "q", fn.id+"/"+payloads[i], answered{tree: value})
	}
	return json.Marshal(batchAnswer{N: nodes})
}

// batchAnswer is the envelope a batch call gets back: one node per payload, in
// the order they were sent, or a failure of the whole call.
type batchAnswer struct {
	N []remoteAnswer `json:"n,omitempty"`
	E *ssr.Error     `json:"e,omitempty"`
	R *ssr.Redirect  `json:"r,omitempty"`
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
	// tree is an answer that is already the tree devalue writes, rather than a
	// Go value still to be taken through the transport hook. A form's output
	// is built that way — the `result` inside it has been through the hook and
	// the object around it is kit's own shape, which encoding again would
	// flatten into a plain map.
	tree any
	err  *ssr.Error
}

// remoteAnswer is the envelope the SSR bundle parses: a value, an error, or a
// redirect of the whole document.
//
// V is devalue's flat form as a string, not a JSON value: the bundle hands it
// to the app's own `transport` decoders, which is what gives a custom type its
// class back before a component calls a method on it.
type remoteAnswer struct {
	V string        `json:"v,omitempty"`
	E *ssr.Error    `json:"e,omitempty"`
	R *ssr.Redirect `json:"r,omitempty"`
}

// stream sends a document that is still waiting for something: the document
// itself, then one `<script>` per promise as it settles, on the same response.
//
// It is the streaming branch of kit's `render_response`, quirks and all. Kit
// builds that response as
//
//	new Response(stream_text(transformed + '\n', chunks), { headers })
//
// where `headers` is the object the etag was deliberately not put on
// (`if (!chunks) headers.set('etag', ...)`) and the page's `status` — which the
// non-streaming branch passes to `text()` — is simply not passed at all. So a
// streamed document is a 200 whatever the page's status was, and carries no
// etag to revalidate against. Both are kit's, and skgo mirrors rather than
// improves on them: the browser runs kit's client either way.
func (s *SSR) stream(w http.ResponseWriter, r *http.Request, shared *loadRequest, document string, promises *promiseTable, headers documentHeaders) {
	header := w.Header()
	if shared != nil {
		shared.applyTo(header)
	}
	setCSPHeaders(header, headers)
	header.Set("Content-Type", "text/html; charset=utf-8")
	header.Set("X-Sveltekit-Page", "true")
	header.Set("Cache-Control", "private, no-cache")
	if s.version != "" {
		header.Set("X-Sveltekit-Version", s.version)
	}
	// No Content-Length: the chunks are not written yet and their length is not
	// known until the last of them settles.
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}

	_, _ = io.WriteString(w, document+"\n")
	flush(w)

	ctx := r.Context()
	hookCtx := ctx
	if shared != nil {
		hookCtx = hookContext(r, shared)
	}
	// kit's `data_serializer.js:103` `get_data(csp)`: `<script${
	// csp.script_needs_nonce ? \` nonce="${csp.nonce}"\` : ''}>` — computed
	// once, outside the settle loop, because it is the same for every chunk
	// this response ever writes.
	nonceAttr := ""
	if headers.ScriptNeedsNonce {
		nonceAttr = ` nonce="` + headers.Nonce + `"`
	}
	replacer := s.deferReplacer(promises)
	promises.settled(ctx, func(id int, value any, err error) {
		_, _ = io.WriteString(w, s.chunkScript(hookCtx, id, value, err, replacer, nonceAttr))
		flush(w)
	})
}

// chunkScript is one settled value on its way to the browser, as kit writes it
// (`page/data_serializer.js`): a script element of its own, appended to a
// document the browser has already started rendering.
//
//	<script nonce="...">__sveltekit_1a2b3c.resolve(1, () => [<value>])</script>
//
// The array is the pair the boot script destructures — `[value]` fulfils the
// promise and `[, error]`, a hole then the error, rejects it — and the arrow
// takes `app` rather than nothing when the value was written through the app's
// transport hook, because `app.decode` is only in scope once the client's app
// module is in hand.
//
// nonceAttr is kit's own `csp.script_needs_nonce ? \` nonce="${csp.nonce}"\`
// : ''` (`data_serializer.js:103`), computed once by stream and threaded
// through unchanged: without it, a CSP that requires a nonce or hash on every
// inline script blocks each chunk as it arrives, and a value a load promised
// never fills in — the response looks identical up to the moment nothing
// happens. Hash mode gets no attribute here at all, matching kit: a streamed
// chunk's content is never added to either policy's script-src sources, so
// hash mode and streaming remain exactly as incompatible in skgo as they are
// in kit itself.
func (s *SSR) chunkScript(ctx context.Context, id int, value any, err error, replacer devalue.Replacer, nonceAttr string) string {
	written := s.unevalChunk(ctx, value, err, replacer)
	arrow := "() => "
	if strings.Contains(written, "app.decode") {
		arrow = "(app) => "
	}
	return "<script" + nonceAttr + ">" + s.info.GlobalName + ".resolve(" + strconv.Itoa(id) + ", " + arrow + written + ")</script>\n"
}

// unevalChunk writes the pair a chunk carries. A value that cannot be written
// becomes the same error kit's would: kit catches the failure and puts it
// through `handle_error_and_jsonify`, consulting the app's handleError hook
// exactly as every other error a render produces does.
func (s *SSR) unevalChunk(ctx context.Context, value any, err error, replacer devalue.Replacer) string {
	if err == nil {
		// The Deferred held the raw Go value so that this is the first place it
		// is encoded, with the transport hook in hand. encodeLoadValue rather
		// than encodeTree because a promised value may itself hold a promise,
		// and kit writes each chunk with the replacer that wrote the first one.
		tree, encodeErr := s.loads.cfg.Transport.encodeLoadValue(value)
		if encodeErr == nil {
			written, unevalErr := devalue.UnevalWith([]any{tree}, replacer)
			if unevalErr == nil {
				return written
			}
			encodeErr = unevalErr
		}
		s.report("", encodeErr)
		err = encodeErr
	}
	pageErr := s.documentError(ctx, "", asHTTPError(err), err)
	written, unevalErr := devalue.UnevalWith([]any{devalue.Hole, errorNodeExtra(pageErr)}, replacer)
	if unevalErr != nil {
		// The error itself is a handful of JSON-safe scalars, so this cannot
		// happen with any error skgo produces; a document that has already
		// been sent still has to say something.
		return "[, {status: 500, message: " + jsString("Internal Error") + "}]"
	}
	return written
}

// errorNodeExtra is errorNode (remote.go) widened for the extra properties
// the app's handleError hook may have added: status and message first, kit's
// own order, then everything else sorted, so a document that carries one
// reads the same twice running.
func errorNodeExtra(e *ssr.Error) *devalue.Object {
	obj := &devalue.Object{}
	obj.Set("status", float64(e.Status))
	obj.Set("message", e.Message)
	keys := make([]string, 0, len(e.Extra))
	for k := range e.Extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		obj.Set(k, e.Extra[k])
	}
	return obj
}

// deferReplacer is kit's `get_replacer` (`page/data_serializer.js`): a promise
// becomes `<global>.defer(<id>)` and everything else is offered to the app's
// own transport hook, in that order — kit tests `thing?.then` before it tries
// the encoders, so a transported type that somehow were also a promise would
// still stream.
//
// Ids are assigned on first encounter, which is what makes them the numbers
// they are: the hydration array is written first and node by node, so the
// promise a page's own load made is numbered after the one its layout made.
func (s *SSR) deferReplacer(promises *promiseTable) devalue.Replacer {
	transport := s.loads.cfg.Transport.unevalReplacer()
	return func(v any, uneval func(any) (string, error)) (string, bool, error) {
		if d, ok := v.(*deferred); ok {
			return s.info.GlobalName + ".defer(" + strconv.Itoa(promises.id(d)) + ")", true, nil
		}
		if transport == nil {
			return "", false, nil
		}
		return transport(v, uneval)
	}
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

// jarOf is the cookie jar a form submission wrote into, so the loads that run
// after it and the document they produce share one. A request that is not a
// submission has none and the branch makes its own.
func jarOf(action *formAction) *cookieJar {
	if action == nil {
		return nil
	}
	return action.jar
}
