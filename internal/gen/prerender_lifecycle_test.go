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
			web := t.TempDir()
			cfg := Config{Web: web, Language: LanguageTypeScript}
			cacheWrite(t, filepath.Join(web, "src/hooks.server."+extension), authored)

			if err := (&app{cfg: cfg}).writePrerenderHook(); err == nil || !strings.Contains(err.Error(), "is authored") {
				t.Fatalf("authored hook accepted: %v", err)
			}
			source, err := os.ReadFile(filepath.Join(web, "src", "hooks.server."+extension))
			if err != nil || string(source) != authored {
				t.Fatalf("authored hook overwritten: %s %v", source, err)
			}
		})
	}
}
