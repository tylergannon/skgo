package adapter

import (
	"encoding/json"
	"io/fs"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// Package metadata must expose the same runtime files that Go fingerprints.
// This checks SKGO's declarations without retesting npm's packaging rules.
func TestPackageMetadataMatchesEmbeddedRuntime(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("package.json")
	if err != nil {
		t.Fatal(err)
	}
	var pkg struct {
		Name, Type, Types              string
		Files                          []string
		Exports                        map[string]any
		PeerDependencies, Dependencies map[string]string
	}
	if err := json.Unmarshal(raw, &pkg); err != nil {
		t.Fatal(err)
	}
	if pkg.Name != "@skgo/sveltekit-adapter" || pkg.Type != "module" || pkg.Types != "index.d.ts" {
		t.Fatalf("adapter package identity: %s", raw)
	}
	wantExports := map[string]any{
		".":              map[string]any{"types": "./index.d.ts", "import": "./skgo-adapter.js"},
		"./prerender":    map[string]any{"types": "./skgo-adapter/prerender.d.ts", "import": "./skgo-adapter/prerender.js"},
		"./reroute":      map[string]any{"types": "./skgo-adapter/reroute.d.ts", "import": "./skgo-adapter/reroute.js"},
		"./package.json": "./package.json",
	}
	if !reflect.DeepEqual(pkg.Exports, wantExports) {
		t.Fatalf("adapter exports = %v, want %v", pkg.Exports, wantExports)
	}
	if pkg.PeerDependencies["@sveltejs/kit"] != "3.1.0" || len(pkg.Dependencies) != 0 {
		t.Fatalf("adapter must use the application's qualified Kit and dependencies: %s", raw)
	}
	if !slices.Contains(pkg.Files, "index.d.ts") {
		t.Fatal("package metadata omits its declared types")
	}
	if _, err := os.Stat("index.d.ts"); err != nil {
		t.Fatal(err)
	}
	err = fs.WalkDir(files, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if !slices.Contains(pkg.Files, name) {
			t.Errorf("package metadata omits embedded runtime %s", name)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range pkg.Files {
		if strings.HasPrefix(name, "skgo-adapter") {
			if _, err := fs.Stat(files, name); err != nil {
				t.Errorf("package metadata names runtime %s absent from Go's fingerprint: %v", name, err)
			}
		}
	}
	// In particular, the public runtime entry must actually be embedded, not
	// merely named by both package metadata fields.
	if _, err := fs.Stat(files, "skgo-adapter.js"); err != nil {
		t.Fatal(err)
	}
}
