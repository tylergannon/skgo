package skgo

import (
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/tylergannon/skgo/internal/envspec"
)

func environmentReceipt() *EnvironmentSnapshot {
	return &EnvironmentSnapshot{
		schema: envspec.Schema{Fields: []envspec.Field{
			{Name: "ENABLED", Type: "bool", Public: true},
			{Name: "COUNT", Type: "int", Public: true},
			{Name: "LABEL", Type: "string", Public: true},
			{Name: "RELEASE", Type: "string", Public: true, Static: true},
			{Name: "SECRET", Type: "string"},
		}},
		values: map[string]any{"ENABLED": false, "COUNT": 123, "LABEL": "</script><script>bad</script>", "RELEASE": "build-A", "SECRET": "private-receipt-827"},
	}
}

func TestEnvironmentEndpointOverridesPrerenderedBuildValues(t *testing.T) {
	build := testBuildFS()
	build["client/_app/env.js"] = &fstest.MapFile{Data: []byte(`export const env={COUNT:999}`)}
	renderer := &SSR{environment: environmentReceipt()}
	handler, err := NewStaticHandler(build, WithSSR(renderer))
	if err != nil {
		t.Fatal(err)
	}
	response := do(t, handler, "GET", "/_app/env.js", nil)
	etag := response.Header.Get("ETag")
	source := body(t, response)
	if response.StatusCode != 200 || response.Header.Get("Content-Type") != "application/javascript; charset=utf-8" {
		t.Fatalf("status/headers: %v", response)
	}
	for _, receipt := range []string{`export const env=`, `ENABLED:false`, `COUNT:123`} {
		if !strings.Contains(source, receipt) {
			t.Errorf("missing %q: %s", receipt, source)
		}
	}
	for _, forbidden := range []string{"SECRET", "private-receipt-827", "RELEASE", "build-A", "999", "</script>"} {
		if strings.Contains(source, forbidden) {
			t.Errorf("unexpected %q in public module: %s", forbidden, source)
		}
	}
	cached := do(t, handler, "GET", "/_app/env.js", http.Header{"If-None-Match": {etag}})
	if cached.StatusCode != 304 || body(t, cached) != "" {
		t.Fatal("conditional environment response must be empty 304")
	}
	head := do(t, handler, "HEAD", "/_app/env.js", nil)
	if head.StatusCode != 200 || body(t, head) != "" {
		t.Fatal("HEAD must preserve headers without body")
	}
}

func TestDocumentUsesTypedPublicEnvironmentAndStaticPlaceholders(t *testing.T) {
	s := streamer(nil)
	s.environment = environmentReceipt()
	s.info.Client.UsesEnvDynamicPublic = true
	s.template = `<head>%sveltekit.head%</head><body>%sveltekit.body%<b>%sveltekit.env.RELEASE%:%sveltekit.env.ENABLED%:%sveltekit.env.SECRET%</b></body>`
	document, _ := assembled(t, s, plannedPage())
	for _, want := range []string{`env: `, `ENABLED:false`, `COUNT:123`, `<b>build-A:false:</b>`} {
		if !strings.Contains(document, want) {
			t.Errorf("missing %q in %s", want, document)
		}
	}
	if strings.Contains(document, "private-receipt-827") {
		t.Fatal("private value leaked into document")
	}
	if strings.Contains(document, "</script><script>bad") {
		t.Fatal("environment value escaped bootstrap script")
	}
}

func TestDevProxyNeverForwardsPrivateModuleTransport(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("private bridge request reached Vite through browser proxy")
		_, _ = w.Write([]byte("private receipt"))
	}))
	defer upstream.Close()
	target, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	response := do(t, NewDevProxy(target, nil), "POST", "/__skgo_dev/module", nil)
	if response.StatusCode != 404 {
		t.Fatalf("status: %d", response.StatusCode)
	}
	if strings.Contains(body(t, response), "private receipt") {
		t.Fatal("private bridge leaked through proxy")
	}
}

func TestEnvironmentPlaceholdersUseJavaScriptNumberFormatting(t *testing.T) {
	s := &SSR{template: "%sveltekit.env.LARGE%|%sveltekit.env.ZERO%|%sveltekit.env.SMALL%", environment: &EnvironmentSnapshot{
		schema: envspec.Schema{Fields: []envspec.Field{{Name: "LARGE", Public: true}, {Name: "ZERO", Public: true}, {Name: "SMALL", Public: true}}},
		values: map[string]any{"LARGE": float64(1e20), "ZERO": math.Copysign(0, -1), "SMALL": float64(1e-7)},
	}}
	if got := s.substitute("", "", "", ""); got != "100000000000000000000|0|1e-7" {
		t.Fatalf("Kit placeholder number coercion: %q", got)
	}
}
