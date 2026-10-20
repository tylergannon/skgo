package webbuild

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
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
	write("web/build/.gitkeep", "")
	var builds, installs, generations int
	moduleData := []byte(`{"Path":"fixture"}`)
	fail := false
	run := func(dir string, env []string, name string, args ...string) ([]byte, error) {
		if name == "go" && args[0] == "list" {
			return moduleData, nil
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
		if filepath.Base(name) != "vp" || args[0] != "build" {
			return nil, fmt.Errorf("unexpected command %s %v", name, args)
		}
		builds++
		if fail {
			return nil, fmt.Errorf("frontend failed")
		}
		if err := os.RemoveAll(filepath.Join(root, "web/build")); err != nil {
			return nil, err
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
	if _, err := os.Stat(filepath.Join(root, "web/build/.gitkeep")); err != nil {
		t.Fatal("build removed tracked embed placeholder")
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

// A build generates and compiles; it never formats the project. Generators
// format their own output, and authored files stay as the developer wrote them.
func TestBuildDoesNotFormatTheProject(t *testing.T) {
	root := t.TempDir()
	for name, value := range map[string]string{
		"web/package.json":         "{}",
		"web/node_modules/.bin/vp": "fixture",
		"server.go":                "package app",
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(value), 0644); err != nil {
			t.Fatal(err)
		}
	}
	var commands []string
	run := func(dir string, env []string, name string, args ...string) ([]byte, error) {
		if name == "go" && args[0] == "list" {
			return []byte(`{"Path":"fixture"}`), nil
		}
		commands = append(commands, filepath.Base(name)+" "+strings.Join(args, " "))
		if filepath.Base(name) == "vp" && args[0] == "build" {
			if err := os.MkdirAll(filepath.Join(root, "web/build"), 0755); err != nil {
				return nil, err
			}
			return nil, os.WriteFile(filepath.Join(root, "web/build/skgo.manifest.json"), []byte(`{"version":1}`), 0644)
		}
		return nil, nil
	}
	if err := Build(root, io.Discard, run); err != nil {
		t.Fatal(err)
	}
	if want := []string{"vp install", "go generate ./...", "vp build"}; !slices.Equal(commands, want) {
		t.Fatalf("build ran %q, want %q", commands, want)
	}
}

// Generated route links are directory symlinks into the app. The input hash
// reads the route files once, where they live, and still sees their edits.
func TestBuildInputsAcceptRouteLinks(t *testing.T) {
	root := t.TempDir()
	route := filepath.Join(root, "web/src/routes/server.go")
	if err := os.MkdirAll(filepath.Dir(route), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(route, []byte("package routes"), 0644); err != nil {
		t.Fatal(err)
	}
	links := filepath.Join(root, "internal/skgo/_links")
	if err := os.MkdirAll(links, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../../../web/src/routes", filepath.Join(links, "onzggl3sn52xizlt")); err != nil {
		t.Fatal(err)
	}
	before, err := treeHash(root, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(route, []byte("package routes\nconst Changed = true"), 0644); err != nil {
		t.Fatal(err)
	}
	if after, err := treeHash(root, true); err != nil || after == before {
		t.Fatalf("edited route reused the input hash: %v", err)
	}
	// A directory link that leaves the tree has bytes the walk never reads, so
	// it must still be refused rather than hashed by its link text.
	if err := os.Symlink(t.TempDir(), filepath.Join(links, "outside")); err != nil {
		t.Fatal(err)
	}
	if _, err := treeHash(root, true); err == nil || !strings.Contains(err.Error(), "directory symlink is unsupported") {
		t.Fatalf("a directory link outside the tree was hashed: %v", err)
	}
}

// go build embeds web/build as it stands, so a failed build must not leave an
// earlier build's frontend there, whichever step failed.
func TestFailedBuildDoesNotLeaveThePreviousFrontend(t *testing.T) {
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
	write("web/node_modules/.bin/vp", "fixture")
	write("web/src/page.svelte", "first")
	write("web/build/.gitkeep", "")
	var builds int
	failAt := ""
	run := func(dir string, env []string, name string, args ...string) ([]byte, error) {
		step := filepath.Base(name) + " " + args[0]
		if step == failAt {
			if step == "vp build" {
				// Kit empties the output directory, then fails during prerender.
				if err := os.RemoveAll(filepath.Join(root, "web/build")); err != nil {
					return nil, err
				}
				write("web/build/partial.html", "partial")
			}
			return nil, fmt.Errorf("%s failed", step)
		}
		switch step {
		case "go list":
			return []byte(`{"Path":"fixture"}`), nil
		case "vp build":
			builds++
			if err := os.RemoveAll(filepath.Join(root, "web/build")); err != nil {
				return nil, err
			}
			write("web/build/skgo.manifest.json", `{"version":1}`)
			write("web/build/page.html", "built from "+readFile(t, filepath.Join(root, "web/src/page.svelte")))
		}
		return nil, nil
	}
	if err := Build(root, io.Discard, run); err != nil {
		t.Fatal(err)
	}
	page := filepath.Join(root, "web/build/page.html")
	if got := readFile(t, page); got != "built from first" {
		t.Fatalf("first build wrote %q", got)
	}
	// A failed build leaves only the placeholder, so go build cannot embed
	// the first frontend and the app refuses to start without a manifest.
	assertNoFrontend := func(step string) {
		t.Helper()
		entries, err := os.ReadDir(filepath.Join(root, "web/build"))
		if err != nil {
			t.Fatalf("%s: %v", step, err)
		}
		if len(entries) != 1 || entries[0].Name() != ".gitkeep" {
			names := []string{}
			for _, entry := range entries {
				names = append(names, entry.Name())
			}
			t.Fatalf("%s left %v in build/", step, names)
		}
	}
	for i, step := range []string{"go list", "vp install", "go generate", "vp build"} {
		// A new input makes the earlier build stale. Failing at "go list" would
		// otherwise leave a build whose currency could not be established.
		write("web/src/page.svelte", fmt.Sprintf("revision %d", i))
		write("web/package.json", fmt.Sprintf(`{"revision":%d}`, i))
		failAt = step
		if err := Build(root, io.Discard, run); err == nil || !strings.Contains(err.Error(), step+" failed") {
			t.Fatalf("%s: build error %v", step, err)
		}
		assertNoFrontend(step)
		if _, err := os.Stat(filepath.Join(root, ".skgo/build.json")); !os.IsNotExist(err) {
			t.Fatalf("%s: stale build receipt remained: %v", step, err)
		}
		failAt = ""
		before := builds
		if err := Build(root, io.Discard, run); err != nil {
			t.Fatalf("%s: recovery: %v", step, err)
		}
		if builds != before+1 || readFile(t, page) != fmt.Sprintf("built from revision %d", i) {
			t.Fatalf("%s: recovery did not rebuild: %d builds, %q", step, builds-before, readFile(t, page))
		}
		// The recovered build is current again and is reused untouched.
		if err := Build(root, io.Discard, run); err != nil || builds != before+1 {
			t.Fatalf("%s: recovered build was not reused: %v", step, err)
		}
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
