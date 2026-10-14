package gen

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestKitMetadataObservesLiveMatcherInputs(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	app := t.TempDir()
	web := filepath.Join(app, "web")
	route := filepath.Join(web, "src", "routes", "[id=First]")
	if err := os.MkdirAll(route, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "example", "web", "node_modules"), filepath.Join(web, "node_modules")); err != nil {
		t.Fatal(err)
	}
	write := func(path, body string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	// This input is outside the frontend source tree; its effects cannot be
	// determined by hashing src/params.js alone.
	write(filepath.Join(app, "names.mjs"), "export const name = 'First';\n")
	write(filepath.Join(web, "src", "params.js"), "import { name } from '../../names.mjs';\nexport const params = { [process.env.SKGO_TEST_MATCHER || name]: value => value };\n")
	t.Setenv("SKGO_TEST_MATCHER", "")
	files := []string{filepath.Join(route, "page.server.go")}
	cfg := Config{Web: web, Language: LanguageTypeScript}
	first, err := captureKitRouteMetadata(cfg, files)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first.Names, []string{"First"}) {
		t.Fatalf("matcher names = %v, want First", first.Names)
	}
	if !reflect.DeepEqual(first.IDs, []string{"/", "/[id=First]"}) {
		t.Fatalf("route IDs = %v", first.IDs)
	}
	if len(first.Routes) != 2 || len(first.Routes[1]) != 1 || first.Routes[1][0].Name != "id" || first.Routes[1][0].Matcher != "First" {
		t.Fatalf("Kit route parameters = %#v", first.Routes)
	}
	if !first.LoadKinds[route]["page.server.go"] || !first.LoadDirs[route] {
		t.Fatal("authored page load missing from metadata")
	}
	write(filepath.Join(app, "names.mjs"), "export const name = 'Second';\n")
	second, err := captureKitRouteMetadata(cfg, files)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(second.Names, []string{"Second"}) {
		t.Fatalf("changed imported matcher = %v, want Second", second.Names)
	}
	t.Setenv("SKGO_TEST_MATCHER", "Third")
	third, err := captureKitRouteMetadata(cfg, files)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(third.Names, []string{"Third"}) {
		t.Fatalf("environment-selected matcher = %v, want Third", third.Names)
	}

	relocatedWeb := filepath.Join(t.TempDir(), "web")
	relocated, err := first.rebase(web, relocatedWeb)
	if err != nil {
		t.Fatal(err)
	}
	wantRoute := filepath.Join(relocatedWeb, "src", "routes", "[id=First]")
	if !reflect.DeepEqual(relocated.Dirs, []string{filepath.Join(relocatedWeb, "src", "routes"), wantRoute}) || !relocated.LoadKinds[wantRoute]["page.server.go"] || !relocated.LoadDirs[wantRoute] {
		t.Fatalf("relocated source ownership = %#v", relocated)
	}
	if first.Dirs[1] != route || !reflect.DeepEqual(relocated.IDs, []string{"/", "/[id=First]"}) || !reflect.DeepEqual(relocated.Names, []string{"First"}) {
		t.Fatal("rebasing mutated original metadata or route semantics")
	}
}
