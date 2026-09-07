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
	base     string
	version  string
	onError  func(routeID string, err error)
}

// SSROptions configures the renderer.
type SSROptions struct {
	// Runtimes bounds how many pages may render at once. Zero means one per
	// CPU. Each runtime holds its own copy of the app's module state, so this
	// is a memory-for-throughput dial and nothing else.
	Runtimes int
	// OnError is told about a render that failed, with the route it was for.
	// The visitor gets the SPA shell, which boots and renders the same page in
	// the browser, so a broken render degrades rather than breaks — and the
	// only record it leaves is this. Leaving it nil logs.
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
		loads:    loads,
		remotes:  remotes,
		engine:   engine,
		info:     info,
		template: string(template),
		base:     strings.TrimSuffix(m.Base, "/"),
		version:  m.Version,
		onError:  opts.OnError,
	}, nil
}

// ssrTarget is the ECMAScript version skgo runs. It is a correctness claim
// rather than a preference: below it, private class fields become WeakMap
// lookups and Svelte's server renderer costs several times more.
const ssrTarget = "es2022"

// serve answers one document request, and reports whether it did. It declines —
// leaving the caller to serve kit's SPA shell — for a path that is not a page
// route, for a page whose branch turns SSR off, and for a page whose loads
// failed, which the client renders as an error page from `__data.json` exactly
// as it did before there was a renderer.
func (s *SSR) serve(w http.ResponseWriter, r *http.Request, urlPath string) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}

	routePath := strings.TrimPrefix(urlPath, s.base)
	if routePath == "" {
		routePath = "/"
	}
	route, params, matched := s.loads.match(routePath)
	if !matched || !route.hasPage {
		return false
	}

	// Kit reduces `ssr` and `csr` over the whole branch, outermost first, and
	// the last node that states an opinion wins (`utils/page_nodes.js`).
	renderIt, hydrate := true, true
	for _, index := range route.nodes {
		if index < 0 || index >= len(s.info.Nodes) {
			continue
		}
		if v := s.info.Nodes[index].SSR; v != nil {
			renderIt = *v
		}
		if v := s.info.Nodes[index].CSR; v != nil {
			hydrate = *v
		}
	}
	if !renderIt {
		return false
	}

	req := dataRequest{url: s.pageURL(r, urlPath), routePath: routePath}
	shared, nodes := s.loads.runBranch(r, req, route.id, params, route.branch, nil)

	for _, node := range nodes {
		if node.redir != nil {
			h := w.Header()
			shared.applyTo(h)
			http.Redirect(w, r, node.redir.Location, node.redir.status())
			return true
		}
	}
	for _, node := range nodes {
		if node.kind == "error" {
			// The error branch is not rendered here yet. The shell boots, the
			// client asks for `__data.json`, gets the same error, and renders
			// `+error.svelte` — which is what it did before SSR existed.
			return false
		}
	}

	if err := s.render(w, r, req, route, params, nodes, shared, hydrate); err != nil {
		if s.onError != nil {
			s.onError(route.id, err)
		} else {
			log.Printf("skgo: rendering %s: %v", route.id, err)
		}
		return false
	}
	return true
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

// render runs the engine and writes the document.
func (s *SSR) render(w http.ResponseWriter, r *http.Request, req dataRequest, route *dataRoute, params map[string]string, nodes []dataNode, shared *loadRequest, hydrate bool) error {
	ctx := r.Context()

	// Every deferred value is settled before the render starts. A promise in a
	// load's result reaches kit's client as a streamed chunk; a component that
	// renders on the server needs the value itself.
	for i := range nodes {
		if nodes[i].kind != "data" {
			continue
		}
		value, err := settle(ctx, nodes[i].data)
		if err != nil {
			return err
		}
		nodes[i].data = value
	}

	// Kit renders `compact(branch)`: a slot that no layout fills is dropped,
	// and so is its place in `node_ids` and in the hydration array.
	branch := make([]ssr.Node, 0, len(nodes))
	present := make([]dataNode, 0, len(nodes))
	indices := make([]int, 0, len(nodes))
	for i, index := range route.nodes {
		if index < 0 || index >= len(s.info.Nodes) {
			continue
		}
		branch = append(branch, ssr.Node{Index: index, Data: nodes[i].data})
		present = append(present, nodes[i])
		indices = append(indices, index)
	}
	if len(branch) == 0 {
		return errors.New("skgo: the route has no node to render")
	}

	cookies := map[string]string{}
	for _, cookie := range r.Cookies() {
		cookies[cookie.Name] = cookie.Value
	}

	request, err := json.Marshal(ssr.Request{
		URL:           req.url.String(),
		RouteID:       route.id,
		Params:        params,
		Status:        http.StatusOK,
		Branch:        branch,
		Cookies:       cookies,
		ClientAddress: clientAddress(r),
	})
	if err != nil {
		return err
	}

	// The event every remote function called during this render sees. It is
	// derived once and immutable, which is kit's own rule: a query may read a
	// cookie and may not write one.
	event := s.remotes.newEvent(r, false).immutable()
	answers := map[string]map[string]json.RawMessage{}

	result, _, err := s.engine.Render(request, func(id, payload string) ([]byte, error) {
		return s.answer(withEvent(ctx, event), id, payload, answers)
	})
	if err != nil {
		return err
	}

	document, err := s.assemble(req, indices, present, result, hydrate, answers)
	if err != nil {
		return err
	}

	etag := `"` + kithash.Kit(document) + `"`
	header := w.Header()
	shared.applyTo(header)
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

	if etagMatches(r.Header.Get("If-None-Match"), etag) {
		header.Del("Content-Length")
		w.WriteHeader(http.StatusNotModified)
		return nil
	}

	header.Set("Content-Length", strconv.Itoa(len(document)))
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write([]byte(document))
	}
	return nil
}

// answer runs one remote function the render asked for and records what it gave
// back, so that the document can carry the same value to the browser under the
// key kit's client will look it up by.
func (s *SSR) answer(ctx context.Context, id, payload string, into map[string]map[string]json.RawMessage) ([]byte, error) {
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
			// redirect of the whole document in kit. Answering the component
			// with the error is the honest half of that; the document-level
			// redirect belongs with the rest of the error branch.
			return json.Marshal(remoteAnswer{E: &ssr.Error{Status: redirect.status(), Message: redirect.Location}})
		}
		e := asHTTPError(err)
		answer := remoteAnswer{E: &ssr.Error{Status: e.Status, Message: e.Message}}
		s.record(into, kind, id+"/"+payload, answer)
		return json.Marshal(answer)
	}

	raw, err := json.Marshal(value)
	if err != nil {
		return json.Marshal(remoteAnswer{E: &ssr.Error{Status: 500, Message: "Internal Error"}})
	}
	answer := remoteAnswer{V: raw}
	s.record(into, kind, id+"/"+payload, answer)
	return json.Marshal(answer)
}

// record files an answer under kit's own key: the single letter of the remote
// function's kind, then `<hash>/<name>/<payload>`. A kind with no letter — a
// command, which kit refuses during a render — is not recorded at all, which is
// what makes an entry with neither a value nor an error impossible.
func (s *SSR) record(into map[string]map[string]json.RawMessage, kind, key string, answer remoteAnswer) {
	if kind == "" {
		return
	}
	raw, err := json.Marshal(answer)
	if err != nil {
		return
	}
	if into[kind] == nil {
		into[kind] = map[string]json.RawMessage{}
	}
	into[kind][key] = raw
}

// remoteAnswer is the envelope the SSR bundle parses: a value, or an error.
type remoteAnswer struct {
	V json.RawMessage `json:"v,omitempty"`
	E *ssr.Error      `json:"e,omitempty"`
}

// settle replaces every Deferred in a load's result with the value it was
// waiting for.
func settle(ctx context.Context, v any) (any, error) {
	switch value := v.(type) {
	case *deferred:
		settled, err := value.wait(ctx)
		if err != nil {
			return nil, err
		}
		return settle(ctx, settled)
	case map[string]any:
		for key, item := range value {
			resolved, err := settle(ctx, item)
			if err != nil {
				return nil, err
			}
			value[key] = resolved
		}
		return value, nil
	case []any:
		for i, item := range value {
			resolved, err := settle(ctx, item)
			if err != nil {
				return nil, err
			}
			value[i] = resolved
		}
		return value, nil
	}
	if holder, ok := v.(deferredHolder); ok && holder.deferredValue() != nil {
		return settle(ctx, holder.deferredValue())
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
