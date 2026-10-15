package webbuild

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildReusesCurrentOutputsAndRebuildsChanges(t *testing.T) {
	root := t.TempDir()
	write := func(name, value string) {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(value), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("web/package.json", "{}")
	write("web/src/page.svelte", "original")
	write("web/node_modules/.bin/vp", "fixture")
	write("server.go", "package app")
	var builds, installs, generations, nativeGenerations int
	moduleData := []byte(`{"Path":"fixture"}`)
	fail := false
	run := func(dir string, env []string, name string, args ...string) ([]byte, error) {
		if name == "go" && args[0] == "list" {
			return moduleData, nil
		}
		if name == "go" && args[0] == "tool" {
			nativeGenerations++
			write("native/Generated.swift", "generated Swift")
			return nil, nil
		}
		if name == "go" && args[0] == "generate" {
			generations++
			write("generated.go", "package app")
			return nil, nil
		}
		if name == "vp" && args[0] == "install" {
			installs++
			return nil, nil
		}
		if filepath.Base(name) == "vp" && args[0] == "fmt" {
			return nil, nil
		}
		if filepath.Base(name) != "vp" || args[0] != "build" {
			return nil, fmt.Errorf("unexpected command %s %v", name, args)
		}
		builds++
		if fail {
			return nil, fmt.Errorf("frontend failed")
		}
		write("web/build/skgo.manifest.json", `{"version":1}`)
		write("web/build/page.html", "built page")
		return nil, nil
	}
	build := func() {
		t.Helper()
		if err := Build(root, io.Discard, run); err != nil {
			t.Fatal(err)
		}
	}
	build()
	if builds != 1 || installs != 1 || generations != 1 {
		t.Fatalf("initial work: %d %d %d", builds, installs, generations)
	}
	manifest := filepath.Join(root, "web/build/skgo.manifest.json")
	old := time.Unix(1000, 0)
	if err := os.Chtimes(manifest, old, old); err != nil {
		t.Fatal(err)
	}
	build()
	info, _ := os.Stat(manifest)
	if builds != 1 || generations != 1 || !info.ModTime().Equal(old) {
		t.Fatal("repeat build touched current outputs")
	}
	for _, name := range []string{"web/src/page.svelte", "server.go", "generated.go", "types.cfg.yaml", "types.json", "fixture.txt", "web/build/page.html", "web/package.json"} {
		t.Run(name, func(t *testing.T) {
			before := builds
			write(name, "changed")
			build()
			if builds != before+1 {
				t.Fatal("changed input/output was reused")
			}
			build()
			if builds != before+1 {
				t.Fatal("rebuilt result was not reusable")
			}
		})
	}
	if installs != 2 {
		t.Fatalf("dependency installation repeated without changed dependencies: %d", installs)
	}
	if err := os.Remove(manifest); err != nil {
		t.Fatal(err)
	}
	before := builds
	build()
	if builds != before+1 {
		t.Fatal("missing manifest was reused")
	}
	t.Setenv("PUBLIC_BUILD_FIXTURE", "changed environment")
	before = builds
	build()
	if builds != before+1 {
		t.Fatal("changed environment was reused")
	}
	write("web/src/page.svelte", "failed revision")
	fail = true
	if err := Build(root, io.Discard, run); err == nil || !strings.Contains(err.Error(), "failed") {
		t.Fatalf("build failure: %v", err)
	}
	fail = false
	before = builds
	build()
	if builds != before+1 {
		t.Fatal("failed build was reused")
	}
	// Native and web preparation share generation, including Swift selection.
	write("go.mod", "module example.com/fixture\n\ngo 1.27.1\n")
	write("skgo.json", `{"version":1,"name":"Fixture"}`)
	write("native/skgo-native.json", `{"version":1,"bundleId":"example.fixture","swiftRemotes":["src/lib/call.remote.ts#call"]}`)
	build()
	if nativeGenerations != 1 {
		t.Fatal("web-first build omitted native generation")
	}
	build()
	if nativeGenerations != 1 {
		t.Fatal("shared native preparation repeated generation")
	}
	if err := os.Remove(filepath.Join(root, "native/Generated.swift")); err != nil {
		t.Fatal(err)
	}
	build()
	if nativeGenerations != 2 {
		t.Fatal("missing Swift projection was reused")
	}

	replacement := t.TempDir()
	if err := os.WriteFile(filepath.Join(replacement, "tool.go"), []byte("package tool"), 0644); err != nil {
		t.Fatal(err)
	}
	moduleData, _ = json.Marshal(map[string]any{"Path": "fixture/tool", "Replace": map[string]string{"Dir": replacement}})
	build()
	before = builds
	if err := os.WriteFile(filepath.Join(replacement, "tool.go"), []byte("package tool\nconst Changed = true"), 0644); err != nil {
		t.Fatal(err)
	}
	build()
	if builds != before+1 {
		t.Fatal("edited local dependency was reused")
	}

	// Runtime state and worktree pointers are never compiler inputs.
	before = builds
	write(".git", "gitdir: /different/absolute/worktree")
	write(".gimbal/runs/run.json", "running")
	write("native/Resources/gimbal", "updated bundled CLI")
	build()
	if builds != before {
		t.Fatal("runtime state, worktree pointer or bundled CLI invalidated frontend")
	}

	// Portable release handoff adopts the verified producer output once. Later
	// local environment changes still invalidate it normally.
	importedRoot := t.TempDir()
	if err := os.CopyFS(importedRoot, os.DirFS(root)); err != nil {
		t.Fatal(err)
	}
	stamp, err := os.ReadFile(filepath.Join(root, ".skgo/build.json"))
	if err != nil {
		t.Fatal(err)
	}
	root = importedRoot
	write(".git", "gitdir: /another/worktree")
	write(".skgo/import.json", string(stamp))
	if err := os.Remove(filepath.Join(root, ".skgo/build.json")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RELEASE_SIGNING_FIXTURE", "different packaging environment")
	build()
	if builds != before {
		t.Fatal("restored release output was rebuilt")
	}
	if _, err := os.Stat(filepath.Join(root, ".skgo/import.json")); !os.IsNotExist(err) {
		t.Fatal("import receipt was not consumed")
	}
	t.Setenv("RELEASE_SIGNING_FIXTURE", "later environment edit")
	build()
	if builds != before+1 {
		t.Fatal("import permanently bypassed environment freshness")
	}
	before = builds
	if err := os.Remove(filepath.Join(root, "web/build/page.html")); err != nil {
		t.Fatal(err)
	}
	build()
	if builds != before+1 {
		t.Fatal("missing frontend asset was reused")
	}
	// A producer receipt cannot bypass source/output integrity checks.
	write(".skgo/import.json", string(stamp))
	write("web/src/page.svelte", "edited after restoration")
	before = builds
	build()
	if builds != before+1 {
		t.Fatal("imported artifact from different source was adopted")
	}

}
