package gen

import (
	"strings"
	"testing"
)

const namedActionSource = `package profile
import (
	"context"
	"github.com/tylergannon/skgo"
)
func save(context.Context) error { return nil }
func archive(context.Context) error { return nil }
var _ = skgo.ActionNoData(save)
var _ = skgo.ActionNoData(archive)
`

const defaultActionSource = `package profile
import (
	"context"
	"github.com/tylergannon/skgo"
)
func submit(context.Context) (struct{}, error) { return struct{}{}, nil }
var _ = skgo.DefaultAction(submit)
`

func TestActionDeclarationsMatchKit(t *testing.T) {
	t.Parallel()
	const path = "app/web/src/routes/profile/page.server.go"
	tests := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"two named actions", map[string]string{path: namedActionSource}, ""},
		{"default alone", map[string]string{path: defaultActionSource}, ""},
		{"duplicate names", map[string]string{path: namedActionSource + "var _ = skgo.ActionNoData(save)\n"}, "declared twice"},
		{"default then named", map[string]string{path: defaultActionSource + "func save(context.Context) error { return nil }\nvar _ = skgo.ActionNoData(save)\n"}, "default action cannot be used with named actions"},
		{"named then default", map[string]string{path: namedActionSource + "func submit(context.Context) (struct{}, error) { return struct{}{}, nil }\nvar _ = skgo.DefaultAction(submit)\n"}, "default action cannot be used with named actions"},
		{"layout placement", map[string]string{"app/web/src/routes/profile/layout.server.go": namedActionSource}, "an action belongs in page.server.go"},
		{"prerendered page", map[string]string{path: namedActionSource, "app/web/src/routes/profile/+page.ts": "export const prerender = true;\n"}, "cannot prerender page /profile with actions"},
		{"auto prerendered page", map[string]string{path: namedActionSource, "app/web/src/routes/profile/+page.ts": "export const prerender = 'auto';\n"}, "cannot prerender page /profile with actions"},
		{"inherited prerender", map[string]string{path: namedActionSource, "app/web/src/routes/+layout.ts": "export const prerender = true;\n"}, "cannot prerender page /profile with actions"},
		{"leaf overrides prerender", map[string]string{path: namedActionSource, "app/web/src/routes/+layout.ts": "export const prerender = true;\n", "app/web/src/routes/profile/+page.ts": "export const prerender = false;\n"}, ""},
		{"prerendered sibling", map[string]string{path: namedActionSource, "app/web/src/routes/sibling/+page.ts": "export const prerender = true;\n"}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, cfg := foreignFixture(t, "", tc.files)
			err := Run(cfg)
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Run error = %v; want %q", err, tc.want)
			}
		})
	}
}
