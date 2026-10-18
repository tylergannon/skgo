package skgo

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tylergannon/skgo/internal/ssr"
)

func TestComponentBoundaryPolicyPreservesPartialOverridesAndHookFailure(t *testing.T) {
	for _, tc := range []struct {
		name              string
		hook              HandleError
		message, original string
	}{
		{"no hook", nil, "literal-app-message", "literal-original-field"},
		{"partial override", func(context.Context, CaughtError) map[string]any {
			return map[string]any{"original": "literal-hook-override"}
		}, "literal-app-message", "literal-hook-override"},
		{"failed hook", func(context.Context, CaughtError) map[string]any { panic("literal-private-hook-panic") }, "Internal Error", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &SSR{loads: &Loads{cfg: LoadConfig{HandleError: tc.hook}}, onError: func(string, error) {}}
			raw, err := s.boundaryError(context.Background(), "/boundary", []byte(`{"kind":"app","body":{"status":409,"message":"literal-app-message","original":"literal-original-field"}}`))
			if err != nil {
				t.Fatal(err)
			}
			var body ssr.Error
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Fatal(err)
			}
			if body.Status != 409 || body.Message != tc.message {
				t.Fatalf("public error = %s", raw)
			}
			if tc.original == "" {
				if _, ok := body.Extra["original"]; ok {
					t.Fatalf("failed hook retained original fields: %s", raw)
				}
			} else if body.Extra["original"] != tc.original {
				t.Fatalf("partial override = %s", raw)
			}
			if strings.Contains(string(raw), "literal-private-hook-panic") {
				t.Fatalf("private panic leaked: %s", raw)
			}
		})
	}
}
