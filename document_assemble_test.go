package skgo

import (
	"strings"
	"testing"

	"github.com/tylergannon/skgo/internal/ssr"
)

// Kit compiles these six app.html placeholders in core/sync/write_server.js.
// The receipts below are supplied by the fixture, not read back from skgo's
// output, so each missing substitution changes the result of this test.
func TestAssembleTemplateAllPlaceholders(t *testing.T) {
	const template = `<!doctype html><html><head>%sveltekit.head%<meta data-version="%sveltekit.version%"><meta data-env="%sveltekit.env.PUBLIC_X%"><script nonce="%sveltekit.nonce%" src="%sveltekit.assets%/manual.js"></script></head><body><div id="app">%sveltekit.body%</div><img src="%sveltekit.assets%/badge.svg"><meta data-nonce="%sveltekit.nonce%"></body></html>`
	const nonce = "13UIKoaLNvWlm6hFuylfFw=="
	s := streamer(nil)
	s.template = template
	s.version = "v1"
	s.base = "/app"
	s.info.Relative = true
	s.info.CSP = &ManifestCSP{Mode: "nonce", Directives: map[string]CSPDirectiveValue{"script-src": sources("self")}}

	req := dataRequest{url: mustURL(t, "http://127.0.0.1/app/account/orders")}
	plan := plannedPage()
	rendered := ssr.Result{
		Done:   true,
		Status: 200,
		Head:   `<title>Receipt head</title>`,
		Body:   `<main id="receipt">Rendered by Go</main>`,
	}
	document, promises, headers, err := s.assemble(req, plan, rendered, nil, newDocumentCSPWithNonce(s.info.CSP, nonce))
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if len(promises.order) != 0 {
		t.Fatalf("fixture has no deferred data, got %d promises", len(promises.order))
	}

	const wantHead = `<head><title>Receipt head</title><meta data-version="v1"><meta data-env=""><script nonce="13UIKoaLNvWlm6hFuylfFw==" src="../manual.js"></script></head>`
	if !strings.Contains(document, wantHead) {
		t.Errorf("template head missing literal placeholder receipts\nwant %s\ngot:\n%s", wantHead, document)
	}
	const wantBody = `<body><div id="app"><main id="receipt">Rendered by Go</main>`
	if !strings.Contains(document, wantBody) {
		t.Errorf("rendered body did not land inside the template wrapper\nwant prefix %s\ngot:\n%s", wantBody, document)
	}
	for _, want := range []string{`<img src="../badge.svg">`, `<meta data-nonce="13UIKoaLNvWlm6hFuylfFw==">`} {
		if !strings.Contains(document, want) {
			t.Errorf("document missing literal receipt %s:\n%s", want, document)
		}
	}
	if strings.Contains(document, "%sveltekit.") {
		t.Errorf("document contains an unresolved app.html placeholder:\n%s", document)
	}
	const wantCSP = "script-src 'self' 'nonce-13UIKoaLNvWlm6hFuylfFw=='"
	if headers.CSP != wantCSP {
		t.Errorf("CSP header: got %q, want %q", headers.CSP, wantCSP)
	}
}
