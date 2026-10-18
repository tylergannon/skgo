package adapter

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Kit's install-time sync resolves Vite with command=build, but never builds
// the application. Its configuration/types must be available before Go writes
// the remote manifest; a real build must still reject the same missing input.
func TestFrontendPreparationBeforeGoGeneration(t *testing.T) {
	for _, tc := range []struct {
		name, contents, diagnostic string
	}{
		{"missing", "", "skgo.remotes.json"},
		{"malformed", "{broken", "JSON"},
		{"invalid-shape", `{"remotes":false}`, "has no `remotes` array"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := prepareMinimalInputsApp(t)
			web := filepath.Join(fixture, "web")
			vp := filepath.Join(web, "node_modules", ".bin", "vp")
			manifest := filepath.Join(web, "skgo.remotes.json")
			if tc.contents != "" {
				writeFixtureFile(t, manifest, tc.contents)
			}
			sync := exec.Command(vp, "exec", "svelte-kit", "sync")
			sync.Dir = web
			sync.Env = fixtureBuildEnv()
			output, err := sync.CombinedOutput()
			if err != nil {
				t.Fatalf("fresh frontend sync: %v\n%s", err, output)
			}
			if strings.Contains(string(output), "ENOENT") || strings.Contains(string(output), "createPrerenderOwner") {
				t.Fatalf("sync printed a swallowed adapter failure:\n%s", output)
			}
			// Positive evidence that Kit performed preparation, rather than a
			// successful exit from an empty command or a swallowed failure.
			config, err := os.ReadFile(filepath.Join(web, "node_modules", "$app", "tsconfig.json"))
			if err != nil || !strings.Contains(string(config), `"compilerOptions"`) {
				t.Fatalf("Kit did not generate its TypeScript configuration: %v\n%s", err, config)
			}
			if tc.contents == "" {
				if _, err := os.Stat(manifest); !os.IsNotExist(err) {
					t.Fatalf("sync fabricated the Go manifest: %v", err)
				}
			}
			build := exec.Command(vp, "build")
			build.Dir = web
			build.Env = fixtureBuildEnv()
			output, err = build.CombinedOutput()
			if err == nil || !strings.Contains(string(output), tc.diagnostic) {
				t.Fatalf("actual build must reject %s generated input with %q: %v\n%s", tc.name, tc.diagnostic, err, output)
			}
		})
	}
}
