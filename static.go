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
	// Routes lists every route kit knows about, with the regular expression
	// kit's own router uses to match it.
	Routes []ManifestRoute `json:"routes"`
	// Remotes lists the `<hash>/<name>` ids of every remote function the
	// built client calls. The adapter copies it from the list `skgo generate`
	// wrote and refuses to build if kit compiled a different set, so a
	// mismatch here means the Go binary and the frontend were generated from
	// different sources.
	Remotes []string `json:"remotes"`
}

// ManifestRoute is one entry of Manifest.Routes.
type ManifestRoute struct {
	ID      string `json:"id"`
	Pattern string `json:"pattern"`
}

// staticHandler serves an adapter build: the client bundle as files, and kit's
// SPA boot document for anything that looks like a page route.
type staticHandler struct {
	build    fs.FS
	manifest Manifest

	appPrefix string // "/_app/", including the configured base
	immutable string // "/_app/immutable/", including the configured base
	base      string // "" or "/base"

	document     []byte
	documentETag string

	routes []*regexp.Regexp

	assets map[string]assetMeta
}

type assetMeta struct {
	etag        string
	contentType string
}

// NewStaticHandler serves an embedded skgo adapter build. build must be rooted
// at the adapter's output directory, so that `index.html`,
// `skgo.manifest.json` and `client/` are at its top level.
//
// Requests are answered in this order: an exact file under `client/`; a 404
// with an empty body for any other miss below the app directory; kit's boot
// document with status 200 for a path matching a route from the manifest; and
// the boot document with status 404 for anything else, so kit's client router
// can render `+error.svelte`.
func NewStaticHandler(build fs.FS) (http.Handler, error) {
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
		assets:       map[string]assetMeta{},
	}

	for _, route := range manifest.Routes {
		re, err := regexp.Compile(kitPattern(route.Pattern))
		if err != nil {
			return nil, fmt.Errorf("skgo: route %s has an unusable pattern %q: %w", route.ID, route.Pattern, err)
		}
		h.routes = append(h.routes, re)
	}

	if err := h.indexAssets(); err != nil {
		return nil, err
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

// indexAssets hashes every file under client/ once, so that request handling
// never has to read a file twice to answer a conditional request.
func (h *staticHandler) indexAssets() error {
	if _, err := fs.Stat(h.build, "client"); err != nil {
		return fmt.Errorf("skgo: reading client/ from the build: %w", err)
	}

	return fs.WalkDir(h.build, "client", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		f, err := h.build.Open(name)
		if err != nil {
			return err
		}
		defer f.Close()

		sum := sha256.New()
		if _, err := io.Copy(sum, f); err != nil {
			return err
		}

		rel := strings.TrimPrefix(name, "client/")
		h.assets[rel] = assetMeta{
			etag:        `"` + hex.EncodeToString(sum.Sum(nil))[:32] + `"`,
			contentType: contentTypeFor(rel),
		}
		return nil
	})
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

	rel := strings.TrimPrefix(strings.TrimPrefix(urlPath, h.base), "/")
	if rel != "" {
		if meta, found := h.assets[rel]; found {
			h.serveAsset(w, r, rel, meta, urlPath)
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

	if h.matchesRoute(urlPath) {
		h.serveDocument(w, r, http.StatusOK)
		return
	}

	h.serveDocument(w, r, http.StatusNotFound)
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

func (h *staticHandler) serveAsset(w http.ResponseWriter, r *http.Request, rel string, meta assetMeta, urlPath string) {
	data, err := fs.ReadFile(h.build, "client/"+rel)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	header := w.Header()
	header.Set("Content-Type", meta.contentType)
	header.Set("ETag", meta.etag)
	if strings.HasPrefix(urlPath, h.immutable) {
		header.Set("Cache-Control", "public, max-age=31536000, immutable")
	}

	// A zero modtime keeps ServeContent from emitting Last-Modified; the ETag
	// is the only validator, which is what a content-addressed build wants.
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(data))
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
