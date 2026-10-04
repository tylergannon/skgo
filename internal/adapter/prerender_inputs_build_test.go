package adapter

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeclaredPrerenderInputsBuildAndProduceNativeArtifacts(t *testing.T) {
	root, err := filepath.Abs("../../example")
	if err != nil {
		t.Fatal(err)
	}
	dependencies := filepath.Join(root, "web", "node_modules")
	if _, err := os.Stat(filepath.Join(dependencies, ".bin", "vp")); err != nil {
		t.Fatalf("pinned frontend dependencies are missing: %v", err)
	}
	fixture := filepath.Join(t.TempDir(), "example")
	copyFixtureTree(t, root, fixture)
	linkFixtureDependencies(t, dependencies, filepath.Join(fixture, "web", "node_modules"), filepath.Join(root, "..", "internal", "adapter"))
	goMod := filepath.Join(fixture, "go.mod")
	mod, err := os.ReadFile(goMod)
	if err != nil {
		t.Fatal(err)
	}
	updatedMod := strings.Replace(string(mod), "replace github.com/tylergannon/skgo => ../", "replace github.com/tylergannon/skgo => "+filepath.Clean(filepath.Join(root, "..")), 1)
	if updatedMod == string(mod) {
		t.Fatal("could not point the isolated example module at this worktree")
	}
	if err := os.WriteFile(goMod, []byte(updatedMod), 0o644); err != nil {
		t.Fatal(err)
	}

	remote := filepath.Join(fixture, "web", "src", "routes", "site.remote.go")
	source, err := os.ReadFile(remote)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(source), "func getSite(ctx context.Context) (Site, error) {\n\treturn Site{Name: \"skgo\", Colocated: \"src/routes/site.remote.go\"}, nil\n}\n\nvar _ = skgo.Query(getSite)", `func getSite(ctx context.Context, name string) (Site, error) {
	return Site{Name: name, Colocated: "src/routes/site.remote.go"}, nil
}

func siteInputs() ([]string, error) { return []string{"atlas", "beacon"}, nil }

var _ = skgo.Prerender(getSite, skgo.PrerenderOptions{Inputs: siteInputs})`, 1)
	if updated == string(source) {
		t.Fatal("could not install the typed declared-input producer in the isolated fixture")
	}
	if err := os.WriteFile(remote, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
	page := filepath.Join(fixture, "web", "src", "routes", "prerender", "[slug]", "+page.svelte")
	pageSource, err := os.ReadFile(page)
	if err != nil {
		t.Fatal(err)
	}
	pageUpdated := strings.Replace(string(pageSource), "let { data } = $props();", `import { getSite } from "../../site.remote";
	let { data } = $props();`, 1)
	pageUpdated = strings.Replace(pageUpdated, "<p data-testid=\"entry-receipt\">{data.receipt}</p>", `<p data-testid="entry-receipt">{data.receipt}</p>
{#await getSite(data.slug) then site}<p data-testid="remote-input-site">{site.name}</p>{/await}`, 1)
	if pageUpdated == string(pageSource) || !strings.Contains(pageUpdated, "remote-input-site") {
		t.Fatal("could not add the native crawler's declared-input call to the isolated fixture")
	}
	if err := os.WriteFile(page, []byte(pageUpdated), 0o644); err != nil {
		t.Fatal(err)
	}

	generate := exec.Command("go", "generate", "./...")
	generate.Dir = fixture
	generate.Env = append(os.Environ(), "GOWORK=off")
	if output, err := generate.CombinedOutput(); err != nil {
		t.Fatalf("go generate: %v\n%s", err, output)
	}
	build := exec.Command(filepath.Join(fixture, "web", "node_modules", ".bin", "vp"), "build")
	build.Dir = filepath.Join(fixture, "web")
	build.Env = append(os.Environ(), "GOWORK=off", "ORIGIN=http://127.0.0.1:8080")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("vp build: %v\n%s", err, output)
	}
	for _, path := range []string{
		"web/build/prerendered/prerender/atlas.html",
		"web/build/prerendered/prerender/beacon.html",
	} {
		if _, err := os.Stat(filepath.Join(fixture, filepath.FromSlash(path))); err != nil {
			t.Errorf("native prerender artifact %s is missing: %v", path, err)
		}
	}
	remoteArtifacts := filepath.Join(fixture, "web", "build", "prerendered", "_app", "remote")
	var payloads []string
	if err := filepath.WalkDir(remoteArtifacts, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if entry.IsDir() {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		payloads = append(payloads, string(contents))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	joinedPayloads := strings.Join(payloads, "\n")
	if len(payloads) < 2 || !strings.Contains(joinedPayloads, "atlas") || !strings.Contains(joinedPayloads, "beacon") {
		t.Fatalf("native crawler did not write both declared remote results (%d artifacts):\n%s", len(payloads), joinedPayloads)
	}
	manifest, err := os.ReadFile(filepath.Join(fixture, "web", "build", "skgo.manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(manifest), `"/prerender/atlas"`) || !strings.Contains(string(manifest), `"/prerender/beacon"`) {
		t.Fatalf("adapter manifest omitted declared input pages:\n%s", manifest)
	}
}

func copyFixtureTree(t *testing.T, source, destination string) {
	t.Helper()
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if relative == "web/node_modules" || relative == "web/.svelte-kit" || relative == "web/build" || relative == "web/dist" {
			if entry.IsDir() {
				return filepath.SkipDir
			}
		}
		target := filepath.Join(destination, relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, contents, info.Mode().Perm())
	})
	if err != nil {
		t.Fatalf("copy fixture tree: %v", err)
	}
}

func linkFixtureDependencies(t *testing.T, source, destination, adapterPath string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(destination, "@skgo"), 0o755); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() == "$app" || entry.Name() == ".vite-temp" || entry.Name() == "@skgo" {
			continue
		}
		if err := os.Symlink(filepath.Join(source, entry.Name()), filepath.Join(destination, entry.Name())); err != nil {
			t.Fatalf("link frontend dependency %s: %v", entry.Name(), err)
		}
	}
	if err := os.Symlink(adapterPath, filepath.Join(destination, "@skgo", "sveltekit-adapter")); err != nil {
		t.Fatal(err)
	}
}
