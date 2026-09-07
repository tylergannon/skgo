package skgo

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Manifest is the build description written by the skgo SvelteKit adapter to
// `skgo.manifest.json`. Only the fields the server needs are modelled.
type Manifest struct {
	// AppDir is kit's `appDir` — the directory holding immutable assets,
	// normally `_app`.
	AppDir string `json:"appDir"`
	// Base is kit's `paths.base`, without a trailing slash.
	Base string `json:"base"`
	// Version is kit's build version name.
	Version string `json:"version"`
	// Nodes gives, per kit node index, the vite-root-relative path of that
	// node's `+page.server.ts` or `+layout.server.ts`, or "" when it has
	// none. It is the key a Go load is registered under.
	Nodes []string `json:"nodes"`
	// Routes lists every route kit knows about, with the regular expression
	// kit's own router uses to match it.
	Routes []ManifestRoute `json:"routes"`
	// Remotes lists the `<hash>/<name>` ids of every remote function the
	// built client calls. The adapter copies it from the list `skgo generate`
	// wrote and refuses to build if kit compiled a different set, so a
	// mismatch here means the Go binary and the frontend were generated from
	// different sources.
	Remotes []string `json:"remotes"`
	// Prerendered lists the pathnames the build wrote a file for, exactly as
	// kit records them in `builder.prerendered.paths`: percent-decoded, and
	// carrying the configured base. It is the authority for two things kit's
	// own adapters use it for — whether a request is answered from the
	// prerendered tree at all, and whether the other trailing-slash form of
	// that request should be redirected to (`adapter-node/src/handler.js`,
	// `serve_prerendered`).
	//
	// A route kit prerenders is *removed* from Routes: `generateManifest`
	// filters out every route whose prerender option is `true`
	// (`core/adapt/builder.js`). So a prerendered path that is not served from
	// here is not served at all.
	Prerendered []string `json:"prerendered,omitempty"`
	// Precompressed reports that the build wrote `.br` and `.gz` siblings for
	// the files kit's own `builder.compress` compresses.
	Precompressed bool `json:"precompressed,omitempty"`
	// SSR describes the server-rendering half of the build: the bundle the Go
	// process renders pages with, the document template, and everything kit's
	// own `render_response` reads out of its manifest to assemble a document.
	// It is absent from a build made by an adapter that emitted no bundle, and
	// such a build is served the way it always was, as the SPA shell.
	SSR *ManifestSSR `json:"ssr,omitempty"`
}

// ManifestSSR is the build's server-rendering description.
type ManifestSSR struct {
	// Bundle is the path of the SSR bundle inside the build.
	Bundle string `json:"bundle"`
	// Template is the path of `app.html` inside the build, verbatim, with
	// kit's `%sveltekit.*%` placeholders still in it.
	Template string `json:"template"`
	// Target is the ECMAScript version the bundle was compiled to. It is
	// recorded because it is a correctness claim, not a preference: below
	// es2022 the bundle still runs and costs several times more.
	Target string `json:"target"`
	// GlobalName is the `__sveltekit_<hash>` object the boot script assigns
	// and the client reads.
	GlobalName string `json:"globalName"`
	// Assets is kit's `paths.assets`.
	Assets string `json:"assets"`
	// Relative is kit's `paths.relative`: whether the document addresses its
	// own assets by a path relative to the page.
	Relative bool `json:"relative"`
	// Client is the client bundle the document boots.
	Client ManifestClient `json:"client"`
	// Nodes describes each node of the table `Nodes` indexes, positionally.
	Nodes []ManifestSSRNode `json:"nodes"`
}

// ManifestClient is kit's own `manifest._.client`: the entry points a document
// imports and the assets it must link.
type ManifestClient struct {
	Start                string         `json:"start"`
	App                  string         `json:"app"`
	Imports              []string       `json:"imports"`
	Stylesheets          []string       `json:"stylesheets"`
	Fonts                []ManifestFont `json:"fonts"`
	UsesEnvDynamicPublic bool           `json:"usesEnvDynamicPublic"`
}

// ManifestFont is one font the build asks a document to preload.
type ManifestFont struct {
	File     string `json:"file"`
	Filename string `json:"filename"`
}

// ManifestSSRNode is one node's contribution to a document.
type ManifestSSRNode struct {
	// Index is the number the node's own module declares, which is not its
	// position in this array. Kit renumbers the nodes it writes into a
	// manifest — a prerendered page's node is dropped and every later one
	// shifts down — and keeps the original numbering in the client bundle. The
	// boot script's `node_ids` are the original numbers, so a document built
	// from the manifest's positions hydrates the wrong components, silently,
	// because they are valid indices for other pages.
	Index int `json:"index"`
	// Component reports that kit compiled a component for this node. Kit omits
	// one for a node that will never be server-rendered.
	Component bool `json:"component"`
	// SSR and CSR are the page options this node sets, or nil for a node that
	// sets neither. Kit reduces them over a route's branch, last one wins.
	SSR *bool `json:"ssr"`
	CSR *bool `json:"csr"`
	// Imports, Stylesheets and Fonts are the client assets a page carrying
	// this node must link.
	Imports     []string       `json:"imports"`
	Stylesheets []string       `json:"stylesheets"`
	Fonts       []ManifestFont `json:"fonts"`
}

// ManifestRoute is one entry of Manifest.Routes.
type ManifestRoute struct {
	ID      string `json:"id"`
	Pattern string `json:"pattern"`
	// Params names the route's parameters in the order the pattern captures
	// them.
	Params []ManifestParam `json:"params,omitempty"`
	// Page describes the node branch of a route that has a page. It is nil for
	// a route that is an endpoint and nothing else.
	Page *ManifestPage `json:"page,omitempty"`
	// Endpoint describes the route's `+server.ts`, and is nil for a route that
	// has none. Kit's own route table carries the same distinction — a lazy
	// loader for the compiled module, or null (`core/generate_manifest`) — and
	// a route with neither a page nor an endpoint never reaches the table at
	// all.
	Endpoint *ManifestEndpoint `json:"endpoint,omitempty"`
}

// ManifestEndpoint is a route's server route, as kit's build describes it.
type ManifestEndpoint struct {
	// Methods are the HTTP methods the compiled `+server.ts` exports a handler
	// for, plus "*" when it exports a `fallback`. It is kit's own
	// `builder.routes[].api.methods`, computed from the built module's exports
	// (`core/postbuild/analyse.js`, `analyse_endpoint`), so it describes the
	// frontend that shipped rather than the source it came from.
	Methods []string `json:"methods"`
}

// ManifestParam is one route parameter, as kit's own router describes it.
type ManifestParam struct {
	Name     string `json:"name"`
	Optional bool   `json:"optional"`
	Rest     bool   `json:"rest"`
	Chained  bool   `json:"chained"`
	Matcher  string `json:"matcher,omitempty"`
}

// ManifestPage is a page route's node branch. `append(Layouts, Leaf)` is the
// branch itself: one slot per node, outermost first, and the order kit's client
// positions its `x-sveltekit-invalidated` string over.
type ManifestPage struct {
	// Layouts holds the node index of each layout wrapping the page. -1 marks
	// a slot no layout fills, which JSON cannot express as a hole.
	Layouts []int `json:"layouts"`
	// Leaf is the node index of the page itself.
	Leaf int `json:"leaf"`
}

// Branch is `[...layouts, leaf]`: the nodes of this route, outermost first.
func (p *ManifestPage) Branch() []int {
	if p == nil {
		return nil
	}
	return append(append(make([]int, 0, len(p.Layouts)+1), p.Layouts...), p.Leaf)
}

// staticHandler serves an adapter build: the client bundle as files, the pages
// the build prerendered, and kit's SPA boot document for anything else that
// looks like a page route.
//
// The order is kit's own. adapter-node composes exactly three middlewares —
// `serve(client)`, `serve_prerendered()`, then the dynamic handler — so a file
// in the client tree shadows a prerendered page of the same name, and a
// prerendered page shadows whatever the app would have rendered.
type staticHandler struct {
	build    fs.FS
	manifest Manifest

	appPrefix string // "/_app/", including the configured base
	immutable string // "/_app/immutable/", including the configured base
	base      string // "" or "/base"

	document     []byte
	documentETag string

	// ssr renders page documents in this process. It is nil for a build with
	// no SSR bundle, and for a handler built without one, and every page is
	// then answered with the boot document as it always was.
	ssr *SSR

	routes []*regexp.Regexp

	assets map[string]assetMeta
	// prerenderedFiles indexes the prerendered tree the same way, keyed by the
	// path below `prerendered/`.
	prerenderedFiles map[string]assetMeta
	// prerendered is the set of pathnames the build wrote a file for, from the
	// manifest. It carries the base and is percent-decoded, because that is how
	// kit records it.
	prerendered map[string]bool
}

// assetMeta is one file of the build, with every encoding of it the build
// wrote.
type assetMeta struct {
	// contentType is derived from the uncompressed name, so `app.js.br` is
	// still JavaScript.
	contentType string
	// variants are the representations of this file. The identity encoding is
	// always present and always first.
	variants []assetVariant
}

// assetVariant is one encoded representation of one file.
type assetVariant struct {
	// name is the path inside the build FS.
	name string
	// encoding is the Content-Encoding to declare, or "" for identity.
	encoding string
	// etag identifies this representation. RFC 9110 §8.8.3 requires a distinct
	// validator per representation, and the compressed bytes are a different
	// representation from the plain ones.
	etag string
	size int64
}

// contentEncodings maps the suffixes kit's `builder.compress` writes to the
// Content-Encoding they carry. Kit compresses every file it writes with both,
// unconditionally and without a size threshold (`core/adapt/builder.js`).
var contentEncodings = map[string]string{
	".br": "br",
	".gz": "gzip",
}

// A StaticOption configures a static handler.
type StaticOption func(*staticHandler)

// WithSSR makes the handler render page documents with s rather than answer
// them with kit's SPA shell. The shell is still what answers a page whose
// branch turns SSR off.
func WithSSR(s *SSR) StaticOption {
	return func(h *staticHandler) { h.ssr = s }
}

// NewStaticHandler serves an embedded skgo adapter build. build must be rooted
// at the adapter's output directory, so that `index.html`,
// `skgo.manifest.json` and `client/` are at its top level.
//
// Requests are answered in this order: an exact file under `client/`; a 404
// with an empty body for any other miss below the app directory; a page the
// build prerendered; a page rendered by the renderer, if one was given and the
// route's branch does not turn SSR off; kit's boot document with status 200 for
// a path matching a route from the manifest; and the boot document with status
// 404 for anything else, so kit's client router can render `+error.svelte`.
func NewStaticHandler(build fs.FS, options ...StaticOption) (http.Handler, error) {
	document, err := fs.ReadFile(build, "index.html")
	if err != nil {
		return nil, fmt.Errorf("skgo: reading index.html from the build: %w", err)
	}

	raw, err := fs.ReadFile(build, "skgo.manifest.json")
	if err != nil {
		return nil, fmt.Errorf("skgo: reading skgo.manifest.json from the build: %w", err)
	}

	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("skgo: parsing skgo.manifest.json: %w", err)
	}
	if manifest.AppDir == "" {
		manifest.AppDir = "_app"
	}

	base := strings.TrimSuffix(manifest.Base, "/")
	if base != "" && !strings.HasPrefix(base, "/") {
		base = "/" + base
	}

	h := &staticHandler{
		build:        build,
		manifest:     manifest,
		base:         base,
		appPrefix:    base + "/" + manifest.AppDir + "/",
		immutable:    base + "/" + manifest.AppDir + "/immutable/",
		document:     document,
		documentETag: etagOf(document),
		prerendered:  map[string]bool{},
	}
	for _, option := range options {
		option(h)
	}
	for _, p := range manifest.Prerendered {
		h.prerendered[p] = true
	}

	for _, route := range manifest.Routes {
		re, err := regexp.Compile(kitPattern(route.Pattern))
		if err != nil {
			return nil, fmt.Errorf("skgo: route %s has an unusable pattern %q: %w", route.ID, route.Pattern, err)
		}
		h.routes = append(h.routes, re)
	}

	if _, err := fs.Stat(build, "client"); err != nil {
		return nil, fmt.Errorf("skgo: reading client/ from the build: %w", err)
	}
	if h.assets, err = indexTree(build, "client"); err != nil {
		return nil, err
	}
	if _, err := fs.Stat(build, "prerendered"); err == nil {
		if h.prerenderedFiles, err = indexTree(build, "prerendered"); err != nil {
			return nil, err
		}
	}

	// A prerendered path with no file behind it is a build that lost something
	// between writing the tree and writing the manifest, and it would surface
	// as a 404 on a page that exists — the exact class of bug the manifest is
	// meant to make impossible.
	for _, p := range manifest.Prerendered {
		if _, _, ok := h.prerenderedFile(p); !ok {
			return nil, fmt.Errorf("skgo: the manifest says %s was prerendered, but the build has no file for it", p)
		}
	}

	return h, nil
}

// kitPattern rewrites the source of kit's own route regular expression into one
// Go's regexp accepts.
//
// Kit builds route patterns in JavaScript, where the empty negated class `[^]`
// means "any character, newline included". It uses it for both forms of the
// rest parameter — `(?:/([^]*))?` for a whole `[...rest]` segment and `([^]*?)`
// for one inside a segment (`packages/kit/src/utils/routing.js`,
// `parse_route_id`). Go's regexp rejects `[^]` outright, so `/docs/[...rest]`
// would stop the server from starting at all.
//
// Nothing else needs translating: `(?:…)`, lazy quantifiers and escaped
// literals mean the same in both engines, and kit escapes `[`, `^` and `]`
// wherever they appear in a literal route segment (`escape_for_regexp` in
// `utils/regex.js`), so an unescaped `[^]` is always kit's rest parameter and
// never part of a path.
func kitPattern(src string) string {
	var b strings.Builder
	for i := 0; i < len(src); i++ {
		if src[i] == '\\' && i+1 < len(src) {
			b.WriteString(src[i : i+2])
			i++
			continue
		}
		if strings.HasPrefix(src[i:], "[^]") {
			b.WriteString(`[\s\S]`)
			i += 2
			continue
		}
		b.WriteByte(src[i])
	}
	return b.String()
}

// indexTree hashes every file under root once, so that request handling never
// has to read a file twice to answer a conditional request, and groups the
// `.br` and `.gz` siblings kit's `builder.compress` wrote onto the file they
// encode.
func indexTree(build fs.FS, root string) (map[string]assetMeta, error) {
	files := map[string]assetMeta{}
	var encoded []string

	err := fs.WalkDir(build, root, func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(name, root+"/")
		if _, ok := contentEncodings[path.Ext(rel)]; ok {
			encoded = append(encoded, rel)
			return nil
		}
		variant, err := readVariant(build, name, "")
		if err != nil {
			return err
		}
		files[rel] = assetMeta{contentType: contentTypeFor(rel), variants: []assetVariant{variant}}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// A `.br` or `.gz` with no plain sibling is served as the file it is named
	// after rather than as an encoding of something that is not there.
	for _, rel := range encoded {
		ext := path.Ext(rel)
		base := strings.TrimSuffix(rel, ext)
		meta, ok := files[base]
		if !ok {
			variant, err := readVariant(build, root+"/"+rel, "")
			if err != nil {
				return nil, err
			}
			files[rel] = assetMeta{contentType: contentTypeFor(rel), variants: []assetVariant{variant}}
			continue
		}
		variant, err := readVariant(build, root+"/"+rel, contentEncodings[ext])
		if err != nil {
			return nil, err
		}
		meta.variants = append(meta.variants, variant)
		files[base] = meta
	}
	return files, nil
}

func readVariant(build fs.FS, name, encoding string) (assetVariant, error) {
	f, err := build.Open(name)
	if err != nil {
		return assetVariant{}, err
	}
	defer f.Close()

	sum := sha256.New()
	size, err := io.Copy(sum, f)
	if err != nil {
		return assetVariant{}, err
	}
	return assetVariant{
		name:     name,
		encoding: encoding,
		etag:     `"` + hex.EncodeToString(sum.Sum(nil))[:32] + `"`,
		size:     size,
	}, nil
}

func (h *staticHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	urlPath, ok := normalizePath(r.URL.Path)
	if !ok {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if h.base != "" && urlPath != h.base && !strings.HasPrefix(urlPath, h.base+"/") {
		h.serveDocument(w, r, http.StatusNotFound)
		return
	}

	// Kit's two internal pathname suffixes are recognised before anything is
	// routed, exactly as kit does it at the top of `runtime/server/respond.js`,
	// because kit's route patterns end `\/?$` and a one-segment data URL
	// therefore matches its own page route. Left to fall through,
	// `/todos/__data.json` was answered 200 with the boot document and kit's
	// client parsed HTML as JSON.
	if suffix := kitSuffix(urlPath); suffix != "" {
		h.refuseInternalRequest(w, r, suffix)
		return
	}

	rel := strings.TrimPrefix(strings.TrimPrefix(urlPath, h.base), "/")
	if rel != "" {
		if meta, found := h.assets[rel]; found {
			h.serveFile(w, r, meta, strings.HasPrefix(urlPath, h.immutable))
			return
		}
	}

	// Anything else below the app directory is a genuine miss. It must never
	// fall through to the boot document: a page shell served with a .js
	// content type would confuse the browser, and one served as HTML would
	// mask a broken build.
	if strings.HasPrefix(urlPath, h.appPrefix) {
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// A page the build prerendered is a file, and it is the only thing that
	// serves that path: kit removes a prerendered route from the route table
	// the manifest is built from, so nothing below would match it.
	if h.prerendered[urlPath] {
		if meta, _, ok := h.prerenderedFile(urlPath); ok {
			h.serveFile(w, r, meta, false)
			return
		}
	}
	// The static file server ignores a trailing slash, so `/about/` would be
	// answered from `about.html` and quietly defeat the app's trailing-slash
	// choice. Kit redirects instead, off the same set and with the same
	// relative location (`adapter-node/src/handler.js`, `serve_prerendered`).
	if inverted, ok := invertTrailingSlash(urlPath); ok && h.prerendered[inverted] {
		location := relativePathname(urlPath, inverted)
		if r.URL.RawQuery != "" {
			location += "?" + r.URL.RawQuery
		}
		w.Header().Set("Location", location)
		w.WriteHeader(http.StatusPermanentRedirect)
		return
	}

	if h.matchesRoute(urlPath) {
		// The renderer declines a page it cannot or should not render — a
		// branch that turns SSR off, a load that failed — and the boot document
		// answers it, which is what answered every page before there was a
		// renderer.
		if h.ssr != nil && h.ssr.serve(w, r, urlPath) {
			return
		}
		h.serveDocument(w, r, http.StatusOK)
		return
	}

	h.serveDocument(w, r, http.StatusNotFound)
}

// prerenderedFile resolves a prerendered pathname to the file the build wrote
// for it. Kit's own adapters lean on the static server's extension expansion
// here: `/about` is `about.html`, `/bar/` is `bar/index.html`, and a
// prerendered non-HTML response such as `/rss.xml` keeps its own name
// (`core/postbuild/prerender.js`, `output_filename`).
func (h *staticHandler) prerenderedFile(urlPath string) (assetMeta, string, bool) {
	rel := strings.TrimPrefix(strings.TrimPrefix(urlPath, h.base), "/")
	candidates := []string{rel, rel + ".html", path.Join(rel, "index.html")}
	if rel == "" {
		candidates = []string{"index.html"}
	}
	for _, candidate := range candidates {
		if meta, ok := h.prerenderedFiles[candidate]; ok {
			return meta, candidate, true
		}
	}
	return assetMeta{}, "", false
}

// invertTrailingSlash is the other form of a pathname: `/a` becomes `/a/` and
// `/a/` becomes `/a`. The root has no other form.
func invertTrailingSlash(urlPath string) (string, bool) {
	if urlPath == "/" || urlPath == "" {
		return "", false
	}
	if strings.HasSuffix(urlPath, "/") {
		return strings.TrimSuffix(urlPath, "/"), true
	}
	return urlPath + "/", true
}

// relativePathname is kit's own (`utils/url.js`): a relative Location, so that
// a path prefix this server cannot see — a reverse proxy's — is preserved.
func relativePathname(from, to string) string {
	segment := strings.TrimSuffix(to, "/")
	if i := strings.LastIndex(segment, "/"); i >= 0 {
		segment = segment[i+1:]
	}
	if strings.HasSuffix(from, "/") {
		return "../" + segment
	}
	return segment + "/"
}

// Kit's route-resolution suffixes, from `src/pathname.js`: `__route.js` is the
// module `preloadCode` imports to resolve a route, with an `.html` variant for
// a page whose own URL ends in `.html`. The two data suffixes live in data.go,
// beside the handler that answers them.
const (
	routeSuffix     = "/__route.js"
	htmlRouteSuffix = ".html__route.js"
)

// kitSuffix reports which of kit's internal suffixes a path carries, or "".
// The suffix is a whole final segment or the `.html` form of one, so an
// ordinary page at `/items/my__data.json` is not a data request.
func kitSuffix(urlPath string) string {
	for _, suffix := range []string{dataSuffix, htmlDataSuffix, routeSuffix, htmlRouteSuffix} {
		if strings.HasSuffix(urlPath, suffix) {
			return suffix
		}
	}
	return ""
}

// refuseInternalRequest answers a `__data.json` or `__route.js` request that
// reached the static handler.
//
// Server loads landed: `__data.json` is answered by Loads.Intercept, which
// must sit in front of this handler, and a data URL that gets this far is one
// no Loads was ever installed to answer. The refusal stays because the static
// handler is usable on its own, and because kit's route patterns end `\/?$`,
// so a one-segment data URL matches its own page route. Left to fall through,
// `/todos/__data.json` was answered 200 with the boot document and kit's
// client parsed HTML as JSON.
//
// 404 is the one status kit's client is written to survive here — its
// hydration path singles it out ("if __data.json returned 404, the route
// doesn't exist — don't reload or we loop") and carries on rendering the route
// client-side. Any other status sends it into a full page reload; a 200 with
// the wrong body sends it into JSON.parse, which is the bug this replaces. The
// body is an App.Error rather than kit's empty one because kit's client
// spreads a JSON body over `{status}` when the content type says JSON, so it
// reaches the client as the same `{status: 404, message: 'Not Found'}` an
// empty body gives — and says something to whoever curls it.
//
// `__route.js` has no equivalent elsewhere: it is refused here for every app,
// because skgo serves only kit's default client-side route resolution.
func (h *staticHandler) refuseInternalRequest(w http.ResponseWriter, r *http.Request, suffix string) {
	header := w.Header()
	header.Set("Cache-Control", "private, no-store")

	if suffix == routeSuffix || suffix == htmlRouteSuffix {
		// Kit's own answer, verbatim, when `router.resolution` is `client` —
		// its default and the only mode skgo serves:
		// `text('Server-side route resolution disabled', { status: 400 })`
		// (`runtime/server/page/server_routing.js`). Measured against this
		// app's own `vp dev` server, which returns exactly this body and
		// status, so the two modes agree here.
		http.Error(w, "Server-side route resolution disabled", http.StatusBadRequest)
		return
	}

	header.Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	if r.Method == http.MethodHead {
		return
	}
	writeJSON(w, map[string]any{"status": 404, "message": "Not Found"})
}

func (h *staticHandler) matchesRoute(urlPath string) bool {
	routePath := strings.TrimPrefix(urlPath, h.base)
	if routePath == "" {
		routePath = "/"
	}
	for _, re := range h.routes {
		if re.MatchString(routePath) {
			return true
		}
	}
	return false
}

// serveFile answers one file of the build, in the best encoding the build wrote
// and the client will accept.
func (h *staticHandler) serveFile(w http.ResponseWriter, r *http.Request, meta assetMeta, immutable bool) {
	variant := meta.variants[0]
	header := w.Header()
	if len(meta.variants) > 1 {
		// The answer depends on Accept-Encoding, so a cache must be told so
		// even when this particular request got the plain bytes back.
		header.Add("Vary", "Accept-Encoding")
		variant = chooseVariant(meta.variants, r.Header.Get("Accept-Encoding"))
	}

	data, err := fs.ReadFile(h.build, variant.name)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// The content type is the file's own, never the encoding's: a `.js.br` is
	// still JavaScript, and `Content-Encoding` is what says it arrived
	// compressed.
	header.Set("Content-Type", meta.contentType)
	header.Set("ETag", variant.etag)
	if variant.encoding != "" {
		header.Set("Content-Encoding", variant.encoding)
	}
	if immutable {
		header.Set("Cache-Control", "public, max-age=31536000, immutable")
	}

	// A zero modtime keeps ServeContent from emitting Last-Modified; the ETag
	// is the only validator, which is what a content-addressed build wants.
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(data))
}

// chooseVariant picks the encoding to answer with. Brotli wins over gzip when
// both are acceptable, which is the preference kit's own static server has.
//
// Unlike that one, this reads Accept-Encoding properly rather than looking for
// a substring: sirv, which adapter-node configures, tests `val.includes('gzip')`
// and `/(br|brotli)/i`, so `br;q=0` — a client saying it will *not* take brotli
// — selects brotli there. Serving a body the client refused is not a rule worth
// mirroring.
func chooseVariant(variants []assetVariant, accept string) assetVariant {
	acceptable := map[string]bool{}
	for _, part := range strings.Split(accept, ",") {
		token, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		token = strings.ToLower(strings.TrimSpace(token))
		if token == "" {
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
		if q > 0 {
			acceptable[token] = true
		}
	}

	for _, encoding := range []string{"br", "gzip"} {
		if !acceptable[encoding] && !acceptable["*"] {
			continue
		}
		for _, variant := range variants {
			if variant.encoding == encoding {
				return variant
			}
		}
	}
	return variants[0]
}

func (h *staticHandler) serveDocument(w http.ResponseWriter, r *http.Request, status int) {
	header := w.Header()
	header.Set("Content-Type", "text/html; charset=utf-8")
	header.Set("Cache-Control", "no-cache")
	header.Set("ETag", h.documentETag)

	// The document is one immutable blob for the life of the build, and
	// `no-cache` means the browser revalidates rather than skips the request —
	// so every reload offers the validator back and every reload used to be
	// answered with the whole document anyway. A 304 keeps the status the
	// request earned: a page that does not exist stays a 404, because the
	// client renders `+error.svelte` off the status, not off the body.
	if status == http.StatusOK && etagMatches(r.Header.Get("If-None-Match"), h.documentETag) {
		// RFC 9110 §15.4.5: a 304 carries the validator and no
		// representation, so it must not declare a length it is not sending.
		header.Del("Content-Length")
		w.WriteHeader(http.StatusNotModified)
		return
	}

	header.Set("Content-Length", strconv.Itoa(len(h.document)))
	w.WriteHeader(status)
	if r.Method == http.MethodHead {
		return
	}
	w.Write(h.document)
}

// etagMatches reports whether an If-None-Match header field covers etag, using
// the weak comparison RFC 9110 §8.8.3.2 prescribes for conditional requests:
// `W/"x"` and `"x"` are a match, and `*` matches anything the server has.
//
// net/http does this for files it serves through ServeContent, but the boot
// document is not a file — it is answered under three different statuses — so
// the comparison is spelled out here.
func etagMatches(field, etag string) bool {
	field = strings.TrimSpace(field)
	if field == "" {
		return false
	}
	if field == "*" {
		return true
	}
	for _, candidate := range strings.Split(field, ",") {
		if strings.TrimPrefix(strings.TrimSpace(candidate), "W/") == etag {
			return true
		}
	}
	return false
}

// normalizePath validates and cleans a request path. It reports false for any
// path that is not already in cleaned form, so traversal attempts are refused
// outright rather than silently resolved.
func normalizePath(p string) (string, bool) {
	if p == "" {
		return "/", true
	}
	if !strings.HasPrefix(p, "/") {
		return "", false
	}

	cleaned := path.Clean(p)
	trailing := strings.HasSuffix(p, "/") && p != "/"
	if trailing {
		if cleaned+"/" != p {
			return "", false
		}
	} else if cleaned != p {
		return "", false
	}

	if cleaned != "/" && !fs.ValidPath(strings.TrimPrefix(cleaned, "/")) {
		return "", false
	}
	return p, true
}

func etagOf(data []byte) string {
	sum := sha256.Sum256(data)
	return `"` + hex.EncodeToString(sum[:])[:32] + `"`
}

// contentTypes pins the types that matter for a kit build, so the answer does
// not depend on the host's MIME database.
var contentTypes = map[string]string{
	".css":         "text/css; charset=utf-8",
	".html":        "text/html; charset=utf-8",
	".ico":         "image/vnd.microsoft.icon",
	".js":          "text/javascript; charset=utf-8",
	".json":        "application/json",
	".map":         "application/json",
	".mjs":         "text/javascript; charset=utf-8",
	".png":         "image/png",
	".svg":         "image/svg+xml",
	".txt":         "text/plain; charset=utf-8",
	".webmanifest": "application/manifest+json",
	".woff2":       "font/woff2",
}

func contentTypeFor(name string) string {
	ext := path.Ext(name)
	if ct, ok := contentTypes[ext]; ok {
		return ct
	}
	if ct := mime.TypeByExtension(ext); ct != "" {
		return ct
	}
	return "application/octet-stream"
}
