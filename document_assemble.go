package skgo

import (
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/tylergannon/skgo/internal/devalue"
	"github.com/tylergannon/skgo/internal/ssr"
)

// assemble builds the document around a render. Every line of it mirrors
// `render_response` (packages/kit/src/runtime/server/page/render.js): the head
// is assembled in kit's five buckets and in kit's order, the boot script is
// kit's split-bundle form with kit's own indentation, and the template is
// substituted the way the function kit compiles from `app.html` substitutes it.
func (s *SSR) assemble(req dataRequest, plan documentPlan, result ssr.Result, answers map[string]map[string]answered) (string, *promiseTable, error) {
	client := s.info.Client
	indices, csr := plan.indices, plan.hydrate

	base := s.base
	assets := s.info.Assets
	baseExpression := jsString(s.base)

	// With relative paths a document addresses its own assets from where the
	// browser actually is, which is what lets it be served from anywhere. The
	// depth is the number of path segments past the first, because a URL's
	// last segment is a file name and not a directory.
	if s.info.Relative {
		segments := strings.Split(strings.TrimPrefix(req.url.Path, s.base), "/")
		if len(segments) > 2 {
			segments = segments[2:]
		} else {
			segments = nil
		}
		base = strings.Join(repeat("..", len(segments)), "/")
		if base == "" {
			base = "."
		}
		baseExpression = "new URL(" + jsString(base) + ", location).pathname.slice(0, -1)"
		// `paths.assets` is either empty or an absolute URL on another origin,
		// and only the first can be made relative.
		if assets == "" {
			assets = base
		}
	}

	prefixed := func(path string) string {
		if strings.HasPrefix(path, "/") {
			return s.base + path
		}
		return assets + "/" + path
	}

	// The client assets a page needs are the entry's plus every node's, in the
	// order kit collects them and without duplicates.
	modulepreloads := newOrdered(client.Imports)
	stylesheets := newOrdered(client.Stylesheets)
	fonts := append([]ManifestFont(nil), client.Fonts...)
	for _, index := range indices {
		node := s.info.Nodes[index]
		modulepreloads.add(node.Imports...)
		stylesheets.add(node.Stylesheets...)
		fonts = append(fonts, node.Fonts...)
	}

	var linkTags, stylesheetLinks []string
	for _, dep := range stylesheets.items {
		stylesheetLinks = append(stylesheetLinks, linkTag(prefixed(dep), `rel="stylesheet"`))
	}
	seenFont := map[string]bool{}
	for _, font := range fonts {
		if seenFont[font.File] {
			continue
		}
		seenFont[font.File] = true
		ext := font.File[strings.LastIndex(font.File, ".")+1:]
		linkTags = append(linkTags, linkTag(prefixed(font.File),
			`rel="preload"`, `as="font"`, `type="font/`+ext+`"`, "crossorigin"))
	}
	if csr {
		for _, dep := range modulepreloads.items {
			linkTags = append(linkTags, linkTag(prefixed(dep), `rel="modulepreload"`))
		}
	}

	// Kit's five buckets, in kit's order: http-equiv tags, link tags, whatever
	// the components put in `<svelte:head>`, style tags, stylesheet links.
	head := strings.Join(append(append(linkTags, result.Head), stylesheetLinks...), "\n\t\t")

	// kit's `const { data, chunks } = data_serializer.get_data(csp)`, and in
	// kit's place: before the boot script and outside the `csr` branch, so that
	// which promises exist — and therefore whether the response streams — is
	// decided by the loads and not by whether the page hydrates.
	promises := &promiseTable{ids: map[*deferred]int{}}
	hydration, err := s.hydrationData(plan.nodes, promises)
	if err != nil {
		return "", nil, err
	}

	body := result.Body
	if csr {
		// Kit appends the responses `event.fetch` collected here. skgo has no
		// `fetch` during a render — the data a page needs is Go's — so the list
		// is always empty and only its separator survives.
		body += "\n\t\t\t"

		script, err := s.bootScript(baseExpression, prefixed, plan, answers, hydration, promises)
		if err != nil {
			return "", nil, err
		}
		body += "<script>" + script + "</script>\n\t\t"
	}

	return s.substitute(head, body, assets), promises, nil
}

// bootScript is the one script a document carries: the object the client reads
// its configuration out of, the element it mounts on, and the import that
// starts kit.
func (s *SSR) bootScript(baseExpression string, prefixed func(string) string, plan documentPlan, answers map[string]map[string]answered, hydration string, promises *promiseTable) (string, error) {
	global := s.info.GlobalName
	indices := plan.indices

	properties := []string{
		"base: " + baseExpression,
		"version: " + jsString(s.version),
	}
	if s.info.Assets != "" {
		properties = append(properties, "assets: "+jsString(s.info.Assets))
	}

	// A page waiting for something needs the two halves of kit's promise
	// plumbing: `defer(id)`, which the hydration array calls to make a promise
	// the page renders its pending branch against, and `resolve(id, fn)`, which
	// each chunk calls to settle it. `deferred` is declared before the global,
	// because the global's own properties close over it.
	var blocks []string
	if len(promises.order) > 0 {
		blocks = append(blocks, "const deferred = new Map();")
		properties = append(properties, deferProperty, s.resolveProperty(prefixed))
	}

	// `node_ids` are the numbers the client's own node table uses, which are
	// the ones each node module declares rather than its position in the
	// manifest. Kit renumbers a manifest's nodes and the client bundle is not
	// renumbered with it, so a document carrying manifest positions hydrates
	// whichever pages happen to live at those numbers.
	nodeIDs := make([]int, len(indices))
	for i, index := range indices {
		nodeIDs[i] = s.info.Nodes[index].Index
	}

	// `error` is the page's error serialised with devalue, and `status` is
	// pushed only when the page is not a 200 *and* carries no error — kit's own
	// rule (`render.js`), because an error already states its own status and
	// the client would otherwise be told it twice.
	serializedError := "null"
	if plan.pageError != nil {
		written, err := unevalJSON(plan.pageError)
		if err != nil {
			return "", err
		}
		serializedError = written
	}
	hydrate := []string{
		"node_ids: [" + join(nodeIDs, ", ") + "]",
		"data: " + hydration,
		"form: null",
		"error: " + serializedError,
	}
	if plan.status != http.StatusOK && plan.pageError == nil {
		hydrate = append(hydrate, "status: "+strconv.Itoa(plan.status))
	}

	arguments := []string{"element", indent6("{\n\t" + strings.Join(hydrate, ",\n\t") + "\n}")}

	remote, err := s.remoteData(answers)
	if err != nil {
		return "", err
	}
	serialized := ""
	if remote != "" {
		serialized = global + ".data = " + remote + ";\n\n\t\t\t\t\t\t"
	}

	// The split-bundle form: kit's start module initialises the global, then
	// the app module is imported, then the client is started on the element.
	boot := "import(" + jsString(prefixed(s.info.Client.Start)) + ").then(async (kit) => {\n" +
		"\t\t\t\t\t\tkit.init(" + global + ");\n" +
		"\t\t\t\t\t\tconst app = await import(" + jsString(prefixed(s.info.Client.App)) + ");\n" +
		"\t\t\t\t\t\t" + serialized + "kit.start(app, " + strings.Join(arguments, ", ") + ");\n" +
		"\t\t\t\t\t});"

	blocks = append(blocks,
		global+" = {\n\t\t\t\t\t\t"+strings.Join(properties, ",\n\t\t\t\t\t\t")+"\n\t\t\t\t\t};",
		"const element = document.currentScript.parentElement;",
		boot,
	)
	return "\n\t\t\t\t{\n\t\t\t\t\t" + strings.Join(blocks, "\n\n\t\t\t\t\t") + "\n\t\t\t\t}\n\t\t\t", nil
}

// deferProperty is kit's `defer` (`page/render.js`): the function the hydration
// array calls to obtain the promise a page is rendering its pending branch
// against, keeping the two halves of it where the chunk that settles it can
// find them.
const deferProperty = "defer: (id) => new Promise((fulfil, reject) => {\n" +
	"\t\t\t\t\t\t\tdeferred.set(id, { fulfil, reject });\n" +
	"\t\t\t\t\t\t})"

// resolveProperty is kit's `resolve`: what a chunk calls when a value the
// server promised has arrived.
//
// The retry loop is kit's own, and its comment says why: a chunk can be
// evaluated before the hydration array that registered its id has been, so the
// id is waited for rather than assumed. With a transport hook the client's app
// module has to be in hand first, because the chunk's expression calls
// `app.decode`; kit imports it here for exactly that reason
// (`has_custom_transporters`), and skgo's app is always the split bundle.
func (s *SSR) resolveProperty(prefixed func(string) string) string {
	prelude := "const [data, error] = fn();"
	if len(s.loads.cfg.Transport) > 0 {
		prelude = "const kit = await import(" + jsString(prefixed(s.info.Client.Start)) + ");\n" +
			"\t\t\t\t\t\t\tkit.init(" + s.info.GlobalName + ");\n" +
			"\t\t\t\t\t\t\tconst app = await import(" + jsString(prefixed(s.info.Client.App)) + ");\n" +
			"\t\t\t\t\t\t\tconst [data, error] = fn(app);"
	}
	return "resolve: async (id, fn) => {\n" +
		"\t\t\t\t\t\t\t" + prelude + "\n" +
		"\n" +
		"\t\t\t\t\t\t\tconst try_to_resolve = () => {\n" +
		"\t\t\t\t\t\t\t\tif (!deferred.has(id)) {\n" +
		"\t\t\t\t\t\t\t\t\tsetTimeout(try_to_resolve, 0);\n" +
		"\t\t\t\t\t\t\t\t\treturn;\n" +
		"\t\t\t\t\t\t\t\t}\n" +
		"\t\t\t\t\t\t\t\tconst { fulfil, reject } = deferred.get(id);\n" +
		"\t\t\t\t\t\t\t\tdeferred.delete(id);\n" +
		"\t\t\t\t\t\t\t\tif (error) reject(error);\n" +
		"\t\t\t\t\t\t\t\telse fulfil(data);\n" +
		"\t\t\t\t\t\t\t}\n" +
		"\t\t\t\t\t\t\ttry_to_resolve();\n" +
		"\t\t\t\t\t\t}"
}

// hydrationData is the array kit's client hydrates the branch from: one entry
// per node, holding what that node's load returned and what it read while doing
// it. A node with no load is `null`, which is what tells the client that the
// slot has no server data rather than that it has none yet.
//
// The value written here is the same tree the engine rendered from, encoded
// once in render() with the app's transport hook in hand, so the markup in the
// document and the data the client hydrates it with cannot disagree.
//
// A value the server has only promised is written as `<global>.defer(<id>)`,
// which is the promise the page is already rendering its pending branch
// against. Walking the nodes in order here is what numbers those ids, so the
// numbering is kit's: `add_node` is called node by node and its replacer hands
// out `promise_id++` on first encounter.
func (s *SSR) hydrationData(nodes []dataNode, promises *promiseTable) (string, error) {
	replacer := s.deferReplacer(promises)
	parts := make([]string, 0, len(nodes))
	for _, node := range nodes {
		if node.kind != "data" {
			parts = append(parts, "null")
			continue
		}
		data, err := devalue.UnevalWith(node.data, replacer)
		if err != nil {
			return "", err
		}
		uses, err := unevalJSON(node.uses.serialize())
		if err != nil {
			return "", err
		}
		parts = append(parts, `{type:"data",data:`+data+`,uses:`+uses+`}`)
	}
	return "[" + strings.Join(parts, ",") + "]", nil
}

// remoteData is `__sveltekit_<hash>.data`: everything a remote function
// answered while the page was rendering, under the key the browser's query
// cache will look it up by — the kind's letter, then `<hash>/<name>/<payload>`.
//
// Only calls that were answered are here, so an entry with neither a value nor
// an error cannot occur; kit omits those, because the client would hydrate one
// as `undefined` rather than fetching it.
func (s *SSR) remoteData(answers map[string]map[string]answered) (string, error) {
	if len(answers) == 0 {
		return "", nil
	}
	transport := s.remotes.cfg.Transport
	replacer := transport.unevalReplacer()

	kinds := make([]string, 0, len(answers))
	for kind := range answers {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)

	var b strings.Builder
	b.WriteByte('{')
	for i, kind := range kinds {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(kind)
		b.WriteString(":{")

		keys := make([]string, 0, len(answers[kind]))
		for key := range answers[kind] {
			keys = append(keys, key)
		}
		devalue.SortStringsUTF16(keys)
		for j, key := range keys {
			if j > 0 {
				b.WriteByte(',')
			}
			answer := answers[kind][key]
			var written string
			var err error
			if answer.err != nil {
				written, err = unevalJSON(answer.err)
				written = "{e:" + written + "}"
			} else {
				// The registry answered with a Go value and kept it; this is
				// where it is written, because this is where the transport
				// hook is known.
				tree, terr := transport.encodeTree(answer.value)
				if terr != nil {
					return "", terr
				}
				written, err = devalue.UnevalWith(tree, replacer)
				written = "{v:" + written + "}"
			}
			if err != nil {
				return "", err
			}
			b.WriteString(jsString(key))
			b.WriteByte(':')
			b.WriteString(written)
		}
		b.WriteByte('}')
	}
	b.WriteByte('}')
	return b.String(), nil
}

// substitute fills the template in, the way the function kit compiles from
// `app.html` does: `head` and `body` once each, `assets`, `nonce` and every
// `env.X` everywhere they appear, and `version` as escaped text.
func (s *SSR) substitute(head, body, assets string) string {
	out := strings.Replace(s.template, "%sveltekit.head%", head, 1)
	out = strings.Replace(out, "%sveltekit.body%", body, 1)
	out = strings.ReplaceAll(out, "%sveltekit.assets%", assets)
	// skgo sets no Content-Security-Policy, so there is no nonce to place.
	out = strings.ReplaceAll(out, "%sveltekit.nonce%", "")
	out = strings.ReplaceAll(out, "%sveltekit.version%", escapeHTML(s.version))
	// The app declares no public runtime environment variables, so every
	// placeholder for one resolves to the empty string kit resolves it to.
	return envPlaceholder.ReplaceAllString(out, "")
}

var envPlaceholder = regexp.MustCompile(`%sveltekit\.env\.[^%]+%`)

func escapeHTML(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;").Replace(s)
}

// jsString writes a JavaScript string literal, with devalue's escaping so that
// nothing it carries can close the script element it sits in.
func jsString(s string) string { return devalue.QuoteString(s) }

// unevalJSON writes the JavaScript for a value by taking it through
// encoding/json first, which is how every value that carries no custom type
// crosses this boundary.
func unevalJSON(v any) (string, error) {
	tree, err := encodeValue(v)
	if err != nil {
		return "", err
	}
	return devalue.Uneval(tree)
}

// indent6 re-indents a block to sit inside the boot script, which is where kit
// puts the hydration object.
func indent6(block string) string {
	indent := strings.Repeat("\t", 6)
	return strings.ReplaceAll(block, "\n", "\n"+indent)
}

func linkTag(href string, attributes ...string) string {
	return `<link href="` + href + `" ` + strings.Join(attributes, " ") + ">"
}

func join(values []int, sep string) string {
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = strconv.Itoa(v)
	}
	return strings.Join(parts, sep)
}

func repeat(s string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = s
	}
	return out
}

// ordered is a set that remembers the order things were added in, which is what
// kit's `Set` gives it and what makes a document byte-identical twice running.
type ordered struct {
	items []string
	seen  map[string]bool
}

func newOrdered(initial []string) *ordered {
	o := &ordered{seen: map[string]bool{}}
	o.add(initial...)
	return o
}

func (o *ordered) add(values ...string) {
	for _, v := range values {
		if o.seen[v] {
			continue
		}
		o.seen[v] = true
		o.items = append(o.items, v)
	}
}
