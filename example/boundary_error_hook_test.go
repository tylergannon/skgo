//go:build skgo_qualification

package example_test

import (
	"net/http"
	"strings"
	"testing"
)

// Uses the real adapter-built Svelte boundary and pooled goja renderer through
// the exact application handler served by the binary, not a reporting bundle.
func TestComponentBoundaryUsesApplicationErrorPolicy(t *testing.T) {
	h := newProdHandler(t)
	for _, tc := range []struct {
		kind                       string
		status                     int
		message, support, original string
	}{
		{"unknown", 500, "Something went wrong on our end.", "case-1121", "no original field"},
		{"app", 409, "literal-public-component-app-353", "case-1121", "literal-public-original-353"},
		{"framework", 404, "literal-public-framework-353", "case-1121", "no original field"},
		{"validation", 400, "Bad Request", "case-1121", "no original field"},
		{"handled", 418, "literal-already-handled-353", "no hook field", "literal-public-original-353"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			rec := get(t, h, "/error/component-boundary?kind="+tc.kind)
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.status, rec.Body)
			}
			body := rec.Body.String()
			requireMarkup(t, tc.kind, body, tagged("h1", "component-boundary-title", "Component boundary policy"), tagged("p", "component-boundary-message", tc.message), tagged("p", "component-boundary-support", tc.support), tagged("p", "component-boundary-original", tc.original))
			forbidText(t, tc.kind, body, "literal-private-component-cause-353", "literal-private-framework-cause-353", "literal-private-validation-cause-353")
			if tc.kind == "handled" && strings.Contains(body, "case-1121") {
				t.Fatal("already handled error ran application hook again")
			}
		})
	}
	t.Run("redirect", func(t *testing.T) {
		rec := get(t, h, "/error/component-boundary?kind=redirect")
		if rec.Code != http.StatusTemporaryRedirect || rec.Header().Get("Location") != "/about" || rec.Body.Len() != 0 {
			t.Fatalf("redirect = %d %v %s", rec.Code, rec.Header(), rec.Body)
		}
	})
}
