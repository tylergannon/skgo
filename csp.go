package skgo

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// ManifestCSP is the build's `csp` option, exactly as kit validates it
// (`core/config/options.js`: `csp: object({ mode: list(['auto', 'hash',
// 'nonce']), directives, reportOnly: directives })`). The adapter writes
// `builder.config.csp` into the manifest verbatim, so a directive kit's
// schema left `undefined` (never configured) is absent here too —
// `JSON.stringify` drops it — while one set to `[]` still comes across, and
// still forces the directive into the header with no sources, because an
// empty array is truthy in JS.
type ManifestCSP struct {
	// Mode is "auto", "hash" or "nonce". kit's default is "auto"
	// (`list(['auto', 'hash', 'nonce'])`, whose fallback is the first option).
	Mode string `json:"mode"`
	// Directives is the policy the response's `content-security-policy`
	// header (and any nonce/hash skgo computes for its own boot script) is
	// built from.
	Directives map[string]CSPDirectiveValue `json:"directives"`
	// ReportOnly is the same shape, for `content-security-policy-report-only`.
	// kit requires `report-to` or `report-uri` here whenever any other
	// directive is set (`CspReportOnlyProvider`'s constructor,
	// `runtime/server/page/csp.js`) — NewSSR checks this once, at startup,
	// rather than on every render the way kit's own per-request `new Csp(...)`
	// does, because the config cannot change between one render and the next.
	ReportOnly map[string]CSPDirectiveValue `json:"reportOnly"`
}

// CSPDirectiveValue is one directive's configured value. Every directive in
// kit's schema is a `string_array()` except `upgrade-insecure-requests` and
// `block-all-mixed-content`, which are bare `boolean()`s
// (`core/config/options.js`). A zero CSPDirectiveValue — absent from the map
// entirely — is kit's `undefined`: a directive never configured, which never
// appears in the header. `IsFlag` distinguishes the two boolean directives
// from every other one, which carries `Sources` instead, even when that is
// `[]`.
type CSPDirectiveValue struct {
	Sources []string
	Flag    bool
	IsFlag  bool
}

// UnmarshalJSON accepts either shape the manifest may carry: a JSON boolean
// for the two flag directives, or an array of strings for every other one.
func (v *CSPDirectiveValue) UnmarshalJSON(data []byte) error {
	var b bool
	if err := json.Unmarshal(data, &b); err == nil {
		v.Flag = b
		v.IsFlag = true
		v.Sources = nil
		return nil
	}
	var arr []string
	if err := json.Unmarshal(data, &arr); err != nil {
		return fmt.Errorf("skgo: a csp directive must be an array of strings or a boolean, got %s: %w", data, err)
	}
	v.Sources = arr
	v.IsFlag = false
	return nil
}

// cspDirectiveOrder is kit's own declaration order for `csp.directives` and
// `csp.reportOnly`: the literal field order of the `directives` object at the
// top of `core/config/options.js`. kit's `object()` validator builds its
// output by iterating its schema's own keys (`for (const key in children)`),
// so every directive holds this position in the object kit's `Csp` iterates —
// regardless of the order a developer wrote it in their own config, and
// regardless of when during header assembly a computed nonce or hash source
// first gave the key a value. A header lists whichever of these are
// configured, in exactly this order.
var cspDirectiveOrder = []string{
	"child-src", "default-src", "frame-src", "worker-src", "connect-src",
	"font-src", "img-src", "manifest-src", "media-src", "object-src",
	"prefetch-src", "script-src", "script-src-elem", "script-src-attr",
	"style-src", "style-src-elem", "style-src-attr", "base-uri", "sandbox",
	"form-action", "frame-ancestors", "navigate-to", "report-uri", "report-to",
	"require-trusted-types-for", "trusted-types", "upgrade-insecure-requests",
	"require-sri-for", "block-all-mixed-content", "plugin-types", "referrer",
}

// cspQuoted is kit's `quoted` set (`runtime/server/page/csp.js`): source
// keywords the header wraps in single quotes.
var cspQuoted = map[string]bool{
	"self": true, "unsafe-eval": true, "unsafe-hashes": true, "unsafe-inline": true,
	"none": true, "strict-dynamic": true, "report-sample": true,
	"wasm-unsafe-eval": true, "script": true,
}

// cspCryptoPattern is kit's `crypto_pattern`: a nonce or hash source is
// quoted too, because it is written as `nonce-xxx`/`sha256-xxx` without the
// quotes the CSP grammar itself requires around them.
var cspCryptoPattern = regexp.MustCompile(`^(nonce|sha\d\d\d)-`)

// quoteCSPSource is kit's inline ternary in `get_header`:
// `quoted.has(source) || crypto_pattern.test(source) ? \`'${source}'\` : source`.
func quoteCSPSource(source string) string {
	if cspQuoted[source] || cspCryptoPattern.MatchString(source) {
		return "'" + source + "'"
	}
	return source
}

// presentSources reads one directive out of a manifest's directives, kit's
// way: absent (kit's `undefined`) is reported as such, and a flag directive
// is never sources, because `!!directive` guards every place this is used and
// a JS boolean is never treated as an array.
func presentSources(directives map[string]CSPDirectiveValue, key string) ([]string, bool) {
	v, ok := directives[key]
	if !ok || v.IsFlag {
		return nil, false
	}
	return v.Sources, true
}

// cspNeedsCSP is kit's `script_needs_csp` (`runtime/server/page/csp.js`):
// true unless every source is `unsafe-inline` — and even then true if
// `strict-dynamic` is also present, because a browser drops `unsafe-inline`
// once a nonce or hash source is on the list, so kit still has to add one.
// It is only ever called once presentSources has confirmed the directive was
// configured at all; an empty-but-present source list still needs one.
func cspNeedsCSP(sources []string) bool {
	unsafeInline, strictDynamic := false, false
	for _, source := range sources {
		switch source {
		case "unsafe-inline":
			unsafeInline = true
		case "strict-dynamic":
			strictDynamic = true
		}
	}
	return !unsafeInline || strictDynamic
}

// cspProvider ports kit's `BaseProvider` (`runtime/server/page/csp.js`),
// narrowed to what a skgo document ever needs from it: the header, and
// whether and how the one inline script skgo ever emits — its boot script —
// carries a nonce or a hash. skgo never inlines component styles (the
// adapter refuses `output.bundleStrategy: 'inline'`, and below-threshold CSS
// is always linked rather than inlined), so `style-src` and its `-attr`/
// `-elem` siblings are never rewritten here: kit's own `BaseProvider` would
// leave them untouched too whenever `add_style` is never called, which for
// skgo is always.
type cspProvider struct {
	useHashes  bool
	directives map[string]CSPDirectiveValue
	nonce      string

	// scriptSrcBase and scriptSrcElemBase are kit's `effective_script_src`
	// (`d['script-src'] || d['default-src']`) and `script_src_elem`
	// (`d['script-src-elem']`, with no such fallback) — read once, at
	// construction, exactly where kit reads them.
	scriptSrcBase     []string
	scriptSrcElemBase []string

	scriptSrcNeedsCSP     bool
	scriptSrcElemNeedsCSP bool
	scriptNeedsCSP        bool

	// ScriptNeedsNonce and ScriptNeedsHash are kit's `script_needs_nonce` and
	// `script_needs_hash`.
	ScriptNeedsNonce bool
	ScriptNeedsHash  bool

	// scriptSrc and scriptSrcElem are kit's `#script_src`/`#script_src_elem`
	// sets: the nonce or hash sources `add_script` has recorded so far, in
	// the order they were added.
	scriptSrc     *ordered
	scriptSrcElem *ordered
}

// newCSPProvider is kit's `BaseProvider` constructor, narrowed as cspProvider
// itself is.
func newCSPProvider(useHashes bool, directives map[string]CSPDirectiveValue, nonce string) *cspProvider {
	if directives == nil {
		directives = map[string]CSPDirectiveValue{}
	}
	scriptSrcBase, ok := presentSources(directives, "script-src")
	if !ok {
		scriptSrcBase, ok = presentSources(directives, "default-src")
	}
	scriptSrcNeedsCSP := ok && cspNeedsCSP(scriptSrcBase)

	scriptSrcElemBase, elemOK := presentSources(directives, "script-src-elem")
	scriptSrcElemNeedsCSP := elemOK && cspNeedsCSP(scriptSrcElemBase)

	scriptNeedsCSP := scriptSrcNeedsCSP || scriptSrcElemNeedsCSP

	return &cspProvider{
		useHashes:             useHashes,
		directives:            directives,
		nonce:                 nonce,
		scriptSrcBase:         scriptSrcBase,
		scriptSrcElemBase:     scriptSrcElemBase,
		scriptSrcNeedsCSP:     scriptSrcNeedsCSP,
		scriptSrcElemNeedsCSP: scriptSrcElemNeedsCSP,
		scriptNeedsCSP:        scriptNeedsCSP,
		ScriptNeedsNonce:      scriptNeedsCSP && !useHashes,
		ScriptNeedsHash:       scriptNeedsCSP && useHashes,
		scriptSrc:             newOrdered(nil),
		scriptSrcElem:         newOrdered(nil),
	}
}

// source is kit's private `#get_source`: the sha256 hash of content is
// computed here, in Go, with the standard library — never inside the JS
// engine. skgo assembles the boot script itself (`document_assemble.go`), so
// the content kit's `csp.js` would call `crypto.subtle.digest` or its own
// bitwise sha256 over is a Go string before it is ever an engine value, and
// hashing a Go-owned string with `crypto/sha256` is the same rule every other
// piece of Go-owned content in this codebase follows: no application I/O runs
// in JavaScript.
func (p *cspProvider) source(content string) string {
	if p.useHashes {
		sum := sha256.Sum256([]byte(content))
		return "sha256-" + base64.StdEncoding.EncodeToString(sum[:])
	}
	return "nonce-" + p.nonce
}

// AddScript is kit's `add_script`: called once, with the exact text of the
// boot script skgo is about to write between `<script>` and `</script>`.
func (p *cspProvider) AddScript(content string) {
	if !p.scriptNeedsCSP {
		return
	}
	source := p.source(content)
	if p.scriptSrcNeedsCSP {
		p.scriptSrc.add(source)
	}
	if p.scriptSrcElemNeedsCSP {
		p.scriptSrcElem.add(source)
	}
}

// header is kit's `get_header(is_meta = false)`. skgo's Go-rendered
// documents are never prerendered — a route kit prerenders is served as a
// static file kit's own build already baked, generated by kit's real Node
// tooling and never touched by this engine — so skgo's render path never
// takes the `is_meta` branch at all: it always sets the header, never a
// `<meta http-equiv="content-security-policy">` tag, and so this port carries
// no `is_meta` parameter or the `frame-ancestors`/`report-uri`/`sandbox`
// exclusion that only applies to that tag.
func (p *cspProvider) header() string {
	merged := make(map[string]CSPDirectiveValue, len(p.directives))
	for k, v := range p.directives {
		merged[k] = v
	}
	if p.scriptSrc.items != nil {
		merged["script-src"] = CSPDirectiveValue{Sources: append(append([]string{}, p.scriptSrcBase...), p.scriptSrc.items...)}
	}
	if p.scriptSrcElem.items != nil {
		merged["script-src-elem"] = CSPDirectiveValue{Sources: append(append([]string{}, p.scriptSrcElemBase...), p.scriptSrcElem.items...)}
	}

	var parts []string
	for _, key := range cspDirectiveOrder {
		v, ok := merged[key]
		if !ok {
			continue
		}
		if v.IsFlag {
			if !v.Flag {
				continue
			}
			parts = append(parts, key)
			continue
		}
		tokens := make([]string, 0, len(v.Sources)+1)
		tokens = append(tokens, key)
		for _, source := range v.Sources {
			tokens = append(tokens, quoteCSPSource(source))
		}
		parts = append(parts, strings.Join(tokens, " "))
	}
	return strings.Join(parts, "; ")
}

// validateReportOnly is kit's `CspReportOnlyProvider` constructor check
// (`runtime/server/page/csp.js`): a report-only policy that sets any
// directive but names no non-empty `report-to` or `report-uri` is "just an
// expensive noop", and kit throws building it. skgo checks this once, at
// NewSSR, rather than on every render the way kit's own per-request
// `new Csp(...)` does.
//
// kit's own check mixes two different truthiness tests, and this keeps both:
// `Object.values(directives).some((v) => !!v)` treats a present-but-empty
// array as set (an array is always truthy in JS), while
// `directives['report-to']?.length` requires that specific array to be
// non-empty.
func validateReportOnly(directives map[string]CSPDirectiveValue) error {
	anySet := false
	for _, v := range directives {
		if v.IsFlag {
			if v.Flag {
				anySet = true
			}
			continue
		}
		// A directive is present in the map at all only because it was
		// configured — see CSPDirectiveValue's own doc comment — so mere
		// presence is kit's `!!value` here, matching the array case even
		// when the array itself is `[]`.
		anySet = true
	}
	if !anySet {
		return nil
	}
	reportTo, _ := presentSources(directives, "report-to")
	reportURI, _ := presentSources(directives, "report-uri")
	if len(reportTo) > 0 || len(reportURI) > 0 {
		return nil
	}
	return fmt.Errorf("skgo: csp.reportOnly must be specified with either the report-to or report-uri directive, or both")
}

// documentCSP is one build's `Csp` (`runtime/server/page/csp.js`), minus the
// nonce generation, which happens once per request rather than once per
// provider — kit's own `Csp` generates it once too, in a field initialiser
// shared by both providers it constructs.
type documentCSP struct {
	nonce    string
	provider *cspProvider
	report   *cspProvider
}

// newDocumentCSP builds one request's Csp. mode "auto" without prerendering
// behaves as "nonce": kit's own rule is `mode === 'hash' || (mode === 'auto'
// && prerender)` (`Csp`'s constructor), and a page this engine renders is
// never a prerender — prerendering runs entirely inside kit's own Node build,
// before skgo's binary exists, and its output is served as a static file this
// render path never reaches. So `prerender` is always false here, and "auto"
// always resolves to nonce mode for every document this function assembles.
func newDocumentCSP(cfg *ManifestCSP) (*documentCSP, error) {
	nonce, err := generateCSPNonce()
	if err != nil {
		return nil, err
	}
	return newDocumentCSPWithNonce(cfg, nonce), nil
}

// newDocumentCSPWithNonce is newDocumentCSP with the random nonce generation
// factored out, so a test can supply a fixed nonce and compare skgo's output
// against a value kit's own code produced for that same nonce, byte for
// byte, rather than one skgo generated itself.
func newDocumentCSPWithNonce(cfg *ManifestCSP, nonce string) *documentCSP {
	if cfg == nil {
		cfg = &ManifestCSP{Mode: "auto"}
	}
	useHashes := cfg.Mode == "hash"
	return &documentCSP{
		nonce:    nonce,
		provider: newCSPProvider(useHashes, cfg.Directives, nonce),
		report:   newCSPProvider(useHashes, cfg.ReportOnly, nonce),
	}
}

// generateCSPNonce is kit's `generate_nonce` (`runtime/server/page/csp.js`):
// 16 random bytes, base64-encoded. kit reads them with
// `crypto.getRandomValues`; Go's own `crypto/rand` is the same guarantee —
// a CSPRNG — read directly rather than through the engine, for the same
// reason `cspProvider.source` hashes in Go rather than in JS.
func generateCSPNonce() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("skgo: generating a csp nonce: %w", err)
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}

// AddScript is kit's `Csp.add_script`: both providers see every script,
// because a report-only policy's own nonce/hash bookkeeping has to stay
// correct even when the enforced policy doesn't need one.
func (d *documentCSP) AddScript(content string) {
	d.provider.AddScript(content)
	d.report.AddScript(content)
}

// ScriptNeedsNonce is kit's `Csp.script_needs_nonce`: true if either policy
// needs one, because the same boot script is what both would be judging.
func (d *documentCSP) ScriptNeedsNonce() bool {
	return d.provider.ScriptNeedsNonce || d.report.ScriptNeedsNonce
}

// ScriptNeedsHash is kit's `Csp.script_needs_hash`: true if either policy
// needs one. Unlike ScriptNeedsNonce, this is decided entirely by the
// build's configured directives (newCSPProvider computes it once, at
// construction) — it never depends on which scripts AddScript has seen — so
// it is safe to read before this request's render has produced any script
// content at all, which is what lets Go tell the engine which branch of
// kit's `csp.script_needs_nonce ? { nonce } : { hash: script_needs_hash }`
// (render.js:198) a render is in before the engine ever calls Svelte's own
// renderer.
func (d *documentCSP) ScriptNeedsHash() bool {
	return d.provider.ScriptNeedsHash || d.report.ScriptNeedsHash
}

// Header is the `content-security-policy` header value, or "" for none.
func (d *documentCSP) Header() string { return d.provider.header() }

// ReportOnlyHeader is the `content-security-policy-report-only` header value,
// or "" for none.
func (d *documentCSP) ReportOnlyHeader() string { return d.report.header() }

// documentHeaders is what assemble hands back for the response's CSP
// headers, alongside the document itself.
type documentHeaders struct {
	CSP           string
	CSPReportOnly string
	// Nonce and ScriptNeedsNonce are this request's csp.nonce and
	// csp.script_needs_nonce, threaded to stream() so a streamed value's own
	// `<script>` tag can carry the same nonce the boot script's did — kit's
	// `data_serializer.js:103` `get_data(csp)`: `<script${
	// csp.script_needs_nonce ? \` nonce="${csp.nonce}"\` : ''}>`. Hash mode
	// gets nothing here, matching kit: a streamed chunk is never added to
	// either provider's script-src sources, so hash mode and streaming stay
	// exactly as incompatible in skgo as they are in kit itself.
	Nonce            string
	ScriptNeedsNonce bool
}

// setCSPHeaders is kit's own two `if (header) headers.set(...)` lines
// (`render.js`): a document that computed no policy sets neither header, the
// same as an app that never configured `csp` at all.
func setCSPHeaders(header http.Header, headers documentHeaders) {
	if headers.CSP != "" {
		header.Set("Content-Security-Policy", headers.CSP)
	}
	if headers.CSPReportOnly != "" {
		header.Set("Content-Security-Policy-Report-Only", headers.CSPReportOnly)
	}
}
