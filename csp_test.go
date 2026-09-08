package skgo

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"
)

// sha256Base64 is used only to check that assemble() hashed the exact
// content it wrote into the document — a wiring check, not a re-test of
// whether sha256 is the right algorithm or base64 the right encoding, which
// the kit-anchored tests above already cover.
func sha256Base64(content string) string {
	sum := sha256.Sum256([]byte(content))
	return base64.StdEncoding.EncodeToString(sum[:])
}

// Every expected value in this file was produced by running kit's own code —
// `core/config/options.js`'s `validate_options` (the exact function a real
// `vite build` runs a developer's `csp` config through before handing it to
// the adapter as `builder.config.csp`) and `runtime/server/page/csp.js`'s
// `Csp` class — directly under Node, against @sveltejs/kit 3.0.0-next.25
// pinned source, never against skgo's own output. See
// ephemeral/worklog/csp-ground-truth.md for the script and full transcript.
//
// bootScriptFixture is one exact boot script skgo's own bootScript() can
// produce (the shape is kit's split-bundle form skgo already mirrors byte for
// byte; document_stream_test.go's TestDocumentCarriesTheDeferredPlaceholder…
// pins the same shape independently). It stands in for "the boot script"
// wherever these tests need fixed content to hash.
const bootScriptFixture = "\n\t\t\t\t{\n\t\t\t\t\t__sveltekit_test = {\n\t\t\t\t\t\tbase: \"\",\n\t\t\t\t\t\tversion: \"1234\"\n\t\t\t\t\t};\n\t\t\t\t\tconst element = document.currentScript.parentElement;\n\n\t\t\t\t\timport(\"_app/immutable/entry/start.js\").then(async (kit) => {\n\t\t\t\t\t\tkit.init(__sveltekit_test);\n\t\t\t\t\t\tconst app = await import(\"_app/immutable/entry/app.js\");\n\t\t\t\t\t\tkit.start(app, element, {\n\tnode_ids: [0, 5],\n\tdata: [{type:\"data\",data:{who:\"ada\"},uses:{}}],\n\tform: null,\n\terror: null\n});\n\t\t\t\t\t});\n\t\t\t\t}\n\t\t\t"

// bootScriptFixtureHash is kit's own `sha256-${sha256(bootScriptFixture)}`
// source token (`runtime/server/page/csp.js`'s `#get_source`, over kit's own
// `sha256` from `runtime/server/page/crypto.js`), transcribed from the Node
// run — the exact value `get_header` quotes into a `script-src` directive.
const bootScriptFixtureHash = "sha256-r7VtOwcCizdcAyIl/jdMeQbHHkeYUntphL4O+DYDixc="

func sources(values ...string) CSPDirectiveValue {
	return CSPDirectiveValue{Sources: values}
}

func flag(value bool) CSPDirectiveValue {
	return CSPDirectiveValue{IsFlag: true, Flag: value}
}

func TestCSPHeaderMatchesKit_HashMode(t *testing.T) {
	cfg := &ManifestCSP{Mode: "hash", Directives: map[string]CSPDirectiveValue{"script-src": sources("self")}}
	doc := newDocumentCSPWithNonce(cfg, "pNfTgAbuZ49tK8nktnOwnA==")
	doc.AddScript(bootScriptFixture)

	const want = "script-src 'self' '" + bootScriptFixtureHash + "'"
	if got := doc.Header(); got != want {
		t.Errorf("header\n got %q\nwant %q", got, want)
	}
	if doc.ScriptNeedsNonce() {
		t.Error("hash mode must not need a nonce")
	}
}

func TestCSPHeaderMatchesKit_NonceMode(t *testing.T) {
	const nonce = "13UIKoaLNvWlm6hFuylfFw=="
	cfg := &ManifestCSP{Mode: "nonce", Directives: map[string]CSPDirectiveValue{"script-src": sources("self")}}
	doc := newDocumentCSPWithNonce(cfg, nonce)
	doc.AddScript(bootScriptFixture)

	const want = "script-src 'self' 'nonce-13UIKoaLNvWlm6hFuylfFw=='"
	if got := doc.Header(); got != want {
		t.Errorf("header\n got %q\nwant %q", got, want)
	}
	if !doc.ScriptNeedsNonce() {
		t.Error("nonce mode must need a nonce")
	}
}

// kit's own output for `{ 'object-src': ['none'], 'default-src': ['self'] }`
// once `validate_options` has normalized it lists `default-src` before
// `object-src`, the schema's own declaration order
// (`core/config/options.js`) — not the order the developer wrote the two
// keys in, which is what a plain-object-iteration port would have produced
// instead.
func TestCSPHeaderMatchesKit_DeclarationOrderNotConfigOrder(t *testing.T) {
	cfg := &ManifestCSP{Mode: "hash", Directives: map[string]CSPDirectiveValue{
		"object-src":  sources("none"),
		"default-src": sources("self"),
	}}
	doc := newDocumentCSPWithNonce(cfg, "Lr8VMrQ3A1hZnuaLy0QDdA==")
	doc.AddScript(bootScriptFixture)

	want := "default-src 'self'; object-src 'none'; script-src 'self' '" + bootScriptFixtureHash + "'"
	if got := doc.Header(); got != want {
		t.Errorf("header\n got %q\nwant %q", got, want)
	}
}

// `unsafe-inline` alone means the boot script needs no CSP treatment at all —
// kit's `script_needs_csp` is false, `add_script` is a no-op, and the
// configured directive passes through unchanged.
func TestCSPHeaderMatchesKit_UnsafeInlineNeedsNoCSP(t *testing.T) {
	cfg := &ManifestCSP{Mode: "nonce", Directives: map[string]CSPDirectiveValue{"script-src": sources("unsafe-inline")}}
	doc := newDocumentCSPWithNonce(cfg, "nqzMhQukF3SA9A7ceNPpbQ==")
	doc.AddScript(bootScriptFixture)

	const want = "script-src 'unsafe-inline'"
	if got := doc.Header(); got != want {
		t.Errorf("header\n got %q\nwant %q", got, want)
	}
	if doc.ScriptNeedsNonce() {
		t.Error("script-src: [unsafe-inline] alone must not need a nonce")
	}
}

// `strict-dynamic` beside `unsafe-inline` flips that back on — a browser
// drops `unsafe-inline` once a nonce or hash source is present, so kit still
// adds one (`script_needs_csp`'s `|| directive.some(v => v === 'strict-dynamic')`).
func TestCSPHeaderMatchesKit_StrictDynamicStillNeedsCSP(t *testing.T) {
	const nonce = "AH2Wmj3Eh8faPqo+llYarQ=="
	cfg := &ManifestCSP{Mode: "nonce", Directives: map[string]CSPDirectiveValue{"script-src": sources("unsafe-inline", "strict-dynamic")}}
	doc := newDocumentCSPWithNonce(cfg, nonce)
	doc.AddScript(bootScriptFixture)

	const want = "script-src 'unsafe-inline' 'strict-dynamic' 'nonce-AH2Wmj3Eh8faPqo+llYarQ=='"
	if got := doc.Header(); got != want {
		t.Errorf("header\n got %q\nwant %q", got, want)
	}
}

// A flag directive (`upgrade-insecure-requests`, `block-all-mixed-content`)
// appears key-only, with no sources, and only when true — kit's default for
// both is `false`, which is skipped.
func TestCSPHeaderMatchesKit_FlagDirective(t *testing.T) {
	cfg := &ManifestCSP{Mode: "hash", Directives: map[string]CSPDirectiveValue{
		"script-src":                sources("self"),
		"upgrade-insecure-requests": flag(true),
		"block-all-mixed-content":   flag(false),
	}}
	doc := newDocumentCSPWithNonce(cfg, "t/4jlaF4E6VvdZt9BLZr5Q==")
	doc.AddScript(bootScriptFixture)

	want := "script-src 'self' '" + bootScriptFixtureHash + "'; upgrade-insecure-requests"
	if got := doc.Header(); got != want {
		t.Errorf("header\n got %q\nwant %q", got, want)
	}
}

// `mode: "auto"` is `use_hashes = mode === 'hash' || (mode === 'auto' &&
// prerender)` (`Csp`'s constructor). skgo's Go engine never prerenders — a
// route kit prerenders is served as a static file kit's own Node build
// already baked, and this render path never sees it — so `prerender` is
// always false here, and "auto" always resolves to nonce mode.
func TestCSPAutoModeBehavesAsNonceBecauseSkgoNeverPrerenders(t *testing.T) {
	const nonce = "oTpseNz5ePrfA/6YEDUX0w=="
	cfg := &ManifestCSP{Mode: "auto", Directives: map[string]CSPDirectiveValue{"script-src": sources("self")}}
	doc := newDocumentCSPWithNonce(cfg, nonce)
	doc.AddScript(bootScriptFixture)

	const want = "script-src 'self' 'nonce-oTpseNz5ePrfA/6YEDUX0w=='"
	if got := doc.Header(); got != want {
		t.Errorf("header\n got %q\nwant %q", got, want)
	}
}

// `script-src-elem` merges its own base and its own computed source
// independently of `script-src` — kit tracks the two sets separately
// (`#script_src` / `#script_src_elem`) and both get the same nonce or hash
// token when both are configured to need one.
func TestCSPScriptSrcElemMergesSeparatelyFromScriptSrc(t *testing.T) {
	const nonce = "MYoC0f2tQlnRTshbqEpQyQ=="
	cfg := &ManifestCSP{Mode: "nonce", Directives: map[string]CSPDirectiveValue{
		"script-src":      sources("self"),
		"script-src-elem": sources("self", "https://cdn.example"),
	}}
	doc := newDocumentCSPWithNonce(cfg, nonce)
	doc.AddScript(bootScriptFixture)

	want := "script-src 'self' 'nonce-MYoC0f2tQlnRTshbqEpQyQ=='; " +
		"script-src-elem 'self' https://cdn.example 'nonce-MYoC0f2tQlnRTshbqEpQyQ=='"
	if got := doc.Header(); got != want {
		t.Errorf("header\n got %q\nwant %q", got, want)
	}
}

// No csp configured at all: every directive is absent, the header is empty,
// and nothing about the boot script changes. This is the build a developer
// who never touches `csp` gets today, unaffected by this feature existing.
func TestCSPHeaderEmptyWhenNothingConfigured(t *testing.T) {
	doc := newDocumentCSPWithNonce(nil, "anything")
	doc.AddScript(bootScriptFixture)
	if got := doc.Header(); got != "" {
		t.Errorf("header: got %q, want empty", got)
	}
	if doc.ScriptNeedsNonce() {
		t.Error("no csp configured must not need a nonce")
	}
}

// kit's `CspReportOnlyProvider` constructor throws when any reportOnly
// directive is set but neither `report-to` nor `report-uri` (non-empty) is —
// "just an expensive noop" otherwise. skgo checks this once, at NewSSR
// (document.go), rather than on every render.
func TestValidateReportOnlyRequiresAReportChannel(t *testing.T) {
	err := validateReportOnly(map[string]CSPDirectiveValue{"script-src": sources("self")})
	if err == nil {
		t.Fatal("expected an error for reportOnly directives with no report-to/report-uri")
	}

	if err := validateReportOnly(map[string]CSPDirectiveValue{
		"script-src": sources("self"),
		"report-uri": sources("/csp-reports"),
	}); err != nil {
		t.Errorf("report-uri should satisfy the requirement: %v", err)
	}

	if err := validateReportOnly(map[string]CSPDirectiveValue{
		"script-src": sources("self"),
		"report-to":  sources("csp-endpoint"),
	}); err != nil {
		t.Errorf("report-to should satisfy the requirement: %v", err)
	}

	if err := validateReportOnly(nil); err != nil {
		t.Errorf("no reportOnly directives at all must not error: %v", err)
	}

	// An explicit but empty report-to does not count — kit's check is
	// `?.length`, not mere presence.
	if err := validateReportOnly(map[string]CSPDirectiveValue{
		"script-src": sources("self"),
		"report-to":  sources(),
	}); err == nil {
		t.Error("an empty report-to must not satisfy the requirement")
	}
}

// ManifestCSP.UnmarshalJSON (via CSPDirectiveValue) is what the adapter's
// JSON.stringify of `builder.config.csp` actually looks like: an object
// with only the directives a developer configured, `upgrade-insecure-requests`
// and `block-all-mixed-content` always present as booleans (kit's schema
// gives both a `false` default rather than leaving them undefined), and
// every other directive an array.
func TestManifestCSPUnmarshalsFromAdapterJSON(t *testing.T) {
	raw := `{
		"mode": "nonce",
		"directives": {
			"script-src": ["self"],
			"upgrade-insecure-requests": false,
			"block-all-mixed-content": false
		},
		"reportOnly": {}
	}`
	var m ManifestCSP
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m.Mode != "nonce" {
		t.Errorf("mode: got %q", m.Mode)
	}
	scriptSrc, ok := m.Directives["script-src"]
	if !ok || scriptSrc.IsFlag || len(scriptSrc.Sources) != 1 || scriptSrc.Sources[0] != "self" {
		t.Errorf("script-src: got %+v", scriptSrc)
	}
	uir, ok := m.Directives["upgrade-insecure-requests"]
	if !ok || !uir.IsFlag || uir.Flag {
		t.Errorf("upgrade-insecure-requests: got %+v", uir)
	}
	if _, ok := m.Directives["default-src"]; ok {
		t.Error("default-src was never configured and must be absent, not a zero value")
	}
	if len(m.ReportOnly) != 0 {
		t.Errorf("reportOnly: got %+v, want empty", m.ReportOnly)
	}
}

// --- assemble()-level wiring tests ---
//
// These do not re-derive the sha256/nonce formula — the tests above already
// anchor that to kit's own output. What they check is that `assemble` wires
// the same csp object through the whole document consistently: the boot
// script it hashes or nonces is the one it actually writes between
// `<script>` and `</script>`, the header it returns is the header that
// content earned, and `%sveltekit.nonce%` resolves to the same nonce
// everywhere kit's own template substitution would put it.

var scriptTagPattern = regexp.MustCompile(`<script( nonce="([^"]*)")?>((?s).*?)</script>`)

func assembleWithCSP(t *testing.T, cfg *ManifestCSP) (string, documentHeaders) {
	t.Helper()
	s := streamer(nil)
	s.info.CSP = cfg
	req := dataRequest{url: mustURL(t, "http://127.0.0.1/account/orders")}
	plan := plannedPage(promising(map[string]any{"who": "ada"}))
	csp, err := newDocumentCSP(cfg)
	if err != nil {
		t.Fatalf("newDocumentCSP: %v", err)
	}
	document, promises, headers, err := s.assemble(req, plan, rendered(), nil, csp)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if len(promises.order) != 0 {
		t.Fatalf("this fixture should carry no outstanding promise, got %d", len(promises.order))
	}
	return document, headers
}

func TestAssembleHashMode_ScriptTagCarriesNoNonceAndHeaderCarriesItsHash(t *testing.T) {
	document, headers := assembleWithCSP(t, &ManifestCSP{Mode: "hash", Directives: map[string]CSPDirectiveValue{"script-src": sources("self")}})

	match := scriptTagPattern.FindStringSubmatch(document)
	if match == nil {
		t.Fatalf("no <script> tag found in document:\n%s", document)
	}
	if match[1] != "" {
		t.Errorf("hash mode must not add a nonce attribute, got %q", match[1])
	}
	content := match[3]

	sum := sha256Base64(content)
	wantSource := "'sha256-" + sum + "'"
	if !strings.Contains(headers.CSP, wantSource) {
		t.Errorf("header %q does not carry the boot script's own hash %q", headers.CSP, wantSource)
	}
	if !strings.HasPrefix(headers.CSP, "script-src 'self' ") {
		t.Errorf("header: got %q", headers.CSP)
	}
	if strings.Contains(document, "%sveltekit.nonce%") {
		t.Error("the nonce placeholder was not substituted")
	}
}

func TestAssembleNonceMode_ScriptTagNonceMatchesHeaderNonce(t *testing.T) {
	document, headers := assembleWithCSP(t, &ManifestCSP{Mode: "nonce", Directives: map[string]CSPDirectiveValue{"script-src": sources("self")}})

	match := scriptTagPattern.FindStringSubmatch(document)
	if match == nil {
		t.Fatalf("no <script> tag found in document:\n%s", document)
	}
	nonce := match[2]
	if nonce == "" {
		t.Fatal("nonce mode must add a nonce attribute to the boot script")
	}
	wantSource := "'nonce-" + nonce + "'"
	if !strings.Contains(headers.CSP, wantSource) {
		t.Errorf("header %q does not carry the script tag's own nonce %q", headers.CSP, wantSource)
	}
	if strings.Contains(document, "%sveltekit.nonce%") {
		t.Error("the nonce placeholder was not substituted")
	}
}

func TestAssembleNoCSPConfigured_LeavesTheDocumentUnchanged(t *testing.T) {
	document, headers := assembleWithCSP(t, nil)
	if headers.CSP != "" || headers.CSPReportOnly != "" {
		t.Errorf("headers: got %+v, want none", headers)
	}
	match := scriptTagPattern.FindStringSubmatch(document)
	if match == nil {
		t.Fatalf("no <script> tag found in document:\n%s", document)
	}
	if match[1] != "" {
		t.Errorf("no csp configured must not add a nonce attribute, got %q", match[1])
	}
}

// setCSPHeaders is kit's own two `if (header) headers.set(...)` lines: only
// a non-empty policy earns a header, and report-only is independent of the
// enforced one.
func TestSetCSPHeaders(t *testing.T) {
	rec := httptest.NewRecorder()
	setCSPHeaders(rec.Header(), documentHeaders{CSP: "script-src 'self'"})
	if got := rec.Header().Get("Content-Security-Policy"); got != "script-src 'self'" {
		t.Errorf("Content-Security-Policy: got %q", got)
	}
	if got := rec.Header().Get("Content-Security-Policy-Report-Only"); got != "" {
		t.Errorf("Content-Security-Policy-Report-Only: got %q, want none", got)
	}

	rec = httptest.NewRecorder()
	setCSPHeaders(rec.Header(), documentHeaders{})
	if got := rec.Header().Get("Content-Security-Policy"); got != "" {
		t.Errorf("Content-Security-Policy: got %q, want none", got)
	}
}

// --- streaming under CSP ---
//
// kit's data_serializer.js:103, `get_data(csp)`:
//
//	const open = `<script${csp.script_needs_nonce ? ` nonce="${csp.nonce}"` : ''}>`;
//
// A document that computed a nonce for its own boot script has to hand that
// same nonce to every chunk a load's promise settles into afterward — the
// whole point of a nonce is that only script the server itself wrote carries
// it, and a browser enforcing `script-src 'self' 'nonce-xxx'` silently drops
// every `<script>` that doesn't, which is exactly what left the four
// streaming scenarios unable to fill in under this PR before this fix.
func TestStreamedChunkCarriesTheSameNonceAsTheBootScript(t *testing.T) {
	const nonce = "STREAM7ovWlm6hFuylfFw=="
	s := streamer(nil)
	s.info.CSP = &ManifestCSP{Mode: "nonce", Directives: map[string]CSPDirectiveValue{"script-src": sources("self")}}

	orders := pending()
	plan := plannedPage(
		promising(map[string]any{"who": "ada"}),
		promising(map[string]any{"orders": orders}),
	)
	req := dataRequest{url: mustURL(t, "http://127.0.0.1/account/orders")}
	csp := newDocumentCSPWithNonce(s.info.CSP, nonce)
	document, promises, headers, err := s.assemble(req, plan, rendered(), nil, csp)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if !headers.ScriptNeedsNonce || headers.Nonce != nonce {
		t.Fatalf("headers: got %+v", headers)
	}

	go func() {
		time.Sleep(10 * time.Millisecond)
		fulfil(orders, []map[string]any{{"item": "a slow parcel"}})
	}()

	rec := httptest.NewRecorder()
	s.stream(rec, httptest.NewRequest(http.MethodGet, "/account/orders", nil), nil, document, promises, headers)

	wantChunk := `<script nonce="` + nonce + `">__sveltekit_test.resolve(1, () => [[{item:"a slow parcel"}]])</script>` + "\n"
	if !strings.Contains(rec.Body.String(), wantChunk) {
		t.Errorf("streamed chunk did not carry the boot script's own nonce:\n got %q\nwant to contain %q", rec.Body.String(), wantChunk)
	}
	// The header and the chunk have to name the same nonce, or a browser
	// enforcing the header still blocks the chunk even though the boot
	// script itself hydrates fine.
	if !strings.Contains(headers.CSP, "'nonce-"+nonce+"'") {
		t.Errorf("header does not name the same nonce: %q", headers.CSP)
	}
}

// Hash mode plus streaming is broken in kit too: `data_serializer.js`'s
// `get_data` only ever tests `csp.script_needs_nonce`, and nothing in kit
// ever adds a streamed chunk's content to either provider's script-src
// sources (`add_script` is called exactly once, for the boot script,
// `render.js:581`). skgo mirrors that limitation rather than papering over
// it: a streamed chunk in hash mode carries no nonce and no hash source,
// exactly like kit's own — which is why the example build uses `mode:
// 'auto'` rather than `mode: 'hash'` (vite.config.ts).
func TestStreamedChunkCarriesNoAttributeInHashMode(t *testing.T) {
	s := streamer(nil)
	s.info.CSP = &ManifestCSP{Mode: "hash", Directives: map[string]CSPDirectiveValue{"script-src": sources("self")}}

	orders := pending()
	plan := plannedPage(
		promising(map[string]any{"who": "ada"}),
		promising(map[string]any{"orders": orders}),
	)
	req := dataRequest{url: mustURL(t, "http://127.0.0.1/account/orders")}
	csp := newDocumentCSPWithNonce(s.info.CSP, "unused-in-hash-mode")
	document, promises, headers, err := s.assemble(req, plan, rendered(), nil, csp)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if headers.ScriptNeedsNonce {
		t.Fatal("hash mode must not need a nonce")
	}

	go func() {
		time.Sleep(10 * time.Millisecond)
		fulfil(orders, []map[string]any{{"item": "a slow parcel"}})
	}()

	rec := httptest.NewRecorder()
	s.stream(rec, httptest.NewRequest(http.MethodGet, "/account/orders", nil), nil, document, promises, headers)

	wantChunk := `<script>__sveltekit_test.resolve(1, () => [[{item:"a slow parcel"}]])</script>` + "\n"
	if !strings.Contains(rec.Body.String(), wantChunk) {
		t.Errorf("streamed chunk:\n got %q\nwant to contain bare %q", rec.Body.String(), wantChunk)
	}
}

// --- auto mode: a dynamic page and a prerendered page resolve differently ---
//
// kit's own rule (`Csp`'s constructor, csp.js): `use_hashes = mode ===
// 'hash' || (mode === 'auto' && prerender)`. skgo's Go engine never renders a
// prerendered route — kit's own Node build prerenders it, entirely before
// skgo's binary exists, and writes the CSP kit computed straight into the
// static file this render path never reaches (render.js: a prerendered
// response gets `csp.csp_provider.get_meta()` as a `<meta http-equiv>` tag,
// not a header) — so `newDocumentCSP` hardcodes `prerender: false` and "auto"
// always resolves to nonce mode for every document Go assembles
// (TestCSPAutoModeBehavesAsNonceBecauseSkgoNeverPrerenders, above).
//
// What this test anchors is the other half of that same formula: had Go ever
// needed to answer for a prerendered page, kit's own rule collapses
// `mode: 'auto'` to exactly the `use_hashes = true` branch that an explicit
// `mode: 'hash'` config already takes — the same branch
// TestCSPHeaderMatchesKit_HashMode is anchored to. So the one build config
// the example app now uses (`mode: 'auto'`, vite.config.ts) is provably safe
// for kit's own separate prerendering pass to resolve on its own: whatever it
// bakes into /about's static file is hash mode, indistinguishable from what
// this test's "prerendered" case produces.
func TestCSPAutoMode_DynamicIsNonceModePrerenderedWouldBeHashMode(t *testing.T) {
	const nonce = "AUTO7ovWlm6hFuylfFw=="
	directives := map[string]CSPDirectiveValue{"script-src": sources("self")}

	dynamic := newDocumentCSPWithNonce(&ManifestCSP{Mode: "auto", Directives: directives}, nonce)
	dynamic.AddScript(bootScriptFixture)
	if dynamic.ScriptNeedsHash() {
		t.Error("a dynamically rendered page under auto mode must not need a hash")
	}
	if !dynamic.ScriptNeedsNonce() {
		t.Error("a dynamically rendered page under auto mode must need a nonce")
	}

	// Kit's own Node build takes this branch for /about, never Go: the same
	// directives, the same formula, with prerender true instead of false.
	prerendered := newDocumentCSPWithNonce(&ManifestCSP{Mode: "hash", Directives: directives}, nonce)
	prerendered.AddScript(bootScriptFixture)
	if !prerendered.ScriptNeedsHash() || prerendered.ScriptNeedsNonce() {
		t.Error("a prerendered page under auto mode must need a hash, not a nonce")
	}

	if dynamic.Header() == prerendered.Header() {
		t.Errorf("the dynamic and prerendered headers must differ: both are %q", dynamic.Header())
	}
	if !strings.Contains(dynamic.Header(), "'nonce-"+nonce+"'") {
		t.Errorf("dynamic header: got %q", dynamic.Header())
	}
	if !strings.Contains(prerendered.Header(), "'"+bootScriptFixtureHash+"'") {
		t.Errorf("prerendered header: got %q", prerendered.Header())
	}
}
