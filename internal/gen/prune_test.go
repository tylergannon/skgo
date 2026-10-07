package gen

import (
	"os"
	"path/filepath"
	"testing"
)

// Real generation/removal is exercised by the output and language-switch
// tests. Distinguish pruning ownership with literal files, without loading Go.
func TestGenerateRemovesWhatTheDeletedGoProduced(t *testing.T) {
	t.Parallel()
	for _, buffered := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "buffered"}[buffered], func(t *testing.T) {
			web := t.TempDir()
			cfg := Config{Web: web, produced: map[string]struct{}{}, Logf: func(string, ...any) {}}
			keep := map[string]string{
				"src/route/+page.svelte":       "<p>authored</p>",
				"src/route/authored.remote.ts": "export const answer = 42;\n",
				"src/route/other.go":           goHeader + "package route\n",
				"src/sibling/+page.server.ts":  goHeader + "retained output\n",
				"src/node_modules/types.ts":    goHeader + "dependency\n",
				"src/.hidden/types.ts":         goHeader + "hidden\n",
			}
			stale := []string{"+page.server.ts", "+layout.server.js", "+server.ts", "go-dev.remote.ts", "types.ts", "skgo_gen.go"}
			for name, content := range keep {
				writeSharedFixture(t, web, name, content)
			}
			cfg.produced[filepath.Join(web, "src/sibling/+page.server.ts")] = struct{}{}
			for _, name := range stale {
				writeSharedFixture(t, web, "src/route/"+name, goHeader+"stale\n")
			}
			if buffered {
				cfg.generation = &generatedOutput{removed: map[string]bool{}}
			}
			if err := pruneStaleArtifacts(cfg); err != nil {
				t.Fatal(err)
			}
			for _, name := range stale {
				path := filepath.Join(web, "src/route", name)
				if buffered {
					if !cfg.generation.removed[path] {
						t.Errorf("stale %s was not scheduled for removal", name)
					}
					if _, err := os.Stat(path); err != nil {
						t.Errorf("pruning published removal early: %v", err)
					}
				} else if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Errorf("stale %s survived: %v", name, err)
				}
			}
			for name, want := range keep {
				got, err := os.ReadFile(filepath.Join(web, name))
				if err != nil || string(got) != want {
					t.Errorf("protected %s changed: %q %v", name, got, err)
				}
			}
		})
	}
}
