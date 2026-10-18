package gen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrerenderHookRefusesBothAuthoredServerHookPaths(t *testing.T) {
	t.Parallel()
	for _, extension := range []string{"ts", "js"} {
		t.Run(extension, func(t *testing.T) {
			const authored = "// authored hook must survive\nexport const handle = ({ event, resolve }) => resolve(event);\n"
			root, cfg := foreignFixture(t, `package data
import("context";"github.com/tylergannon/skgo")
func value(context.Context)(string,error){return "value",nil}
var _=skgo.Prerender(value)
`, map[string]string{"app/web/src/hooks.server." + extension: authored})
			if err := Run(cfg); err == nil || !strings.Contains(err.Error(), "is authored") {
				t.Fatalf("authored hook accepted: %v", err)
			}
			source, err := os.ReadFile(filepath.Join(root, "app", "web", "src", "hooks.server."+extension))
			if err != nil || string(source) != authored {
				t.Fatalf("authored hook overwritten: %s %v", source, err)
			}
		})
	}
}
