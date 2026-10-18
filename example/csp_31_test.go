//go:build skgo_qualification

package example_test

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/tylergannon/skgo/example"
)

// Kit 3.1 quotes these source keywords in enforced and report-only policies.
// Change the build input, then exercise the same production handler as the binary.
func TestProductionCSPHashReportingKeywords(t *testing.T) {
	dist := prodDist(t)
	raw, err := fs.ReadFile(dist, "skgo.manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	ssr, ok := manifest["ssr"].(map[string]any)
	if !ok {
		t.Fatal("build manifest has no SSR description")
	}
	ssr["csp"] = map[string]any{
		"mode":       "nonce",
		"directives": map[string]any{"script-src": []string{"self", "report-sha256", "report-sha384", "report-sha512"}},
		"reportOnly": map[string]any{"script-src": []string{"self", "report-sha256", "report-sha384", "report-sha512"}, "report-uri": []string{"/reports"}},
	}
	raw, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	h, err := example.NewHandler(oneFileOverlay{FS: dist, name: "skgo.manifest.json", data: raw}, prodOrigin)
	if err != nil {
		t.Fatal(err)
	}
	response := get(t, h, "/todos")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "<html") {
		t.Fatalf("document status=%d body=%s", response.Code, response.Body)
	}
	const want = "script-src 'self' 'report-sha256' 'report-sha384' 'report-sha512'"
	nonce := regexp.MustCompile(` 'nonce-[^']+'`)
	if got := nonce.ReplaceAllString(response.Header().Get("Content-Security-Policy"), ""); got != want {
		t.Fatalf("enforced policy=%q want=%q", got, want)
	}
	if got := nonce.ReplaceAllString(response.Header().Get("Content-Security-Policy-Report-Only"), ""); got != want+"; report-uri /reports" {
		t.Fatalf("report-only policy=%q", got)
	}
}
