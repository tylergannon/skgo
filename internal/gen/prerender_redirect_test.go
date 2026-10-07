package gen

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestGoPagePrerenderRedirectBuildsKitsNativeArtifact exercises the complete
// build bridge: a Go page load redirects while Kit crawls /old, and the adapter
// must retain both the recorded path and Kit's HTML output.
func testGoPagePrerenderRedirectBuildsKitsNativeArtifact(t *testing.T) {
	fixture := requireProductionFixture(t)
	ui := filepath.Join(fixture.app, "ui")

	manifestBytes, err := os.ReadFile(filepath.Join(ui, "build", "skgo.manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Prerendered      []string          `json:"prerendered"`
		PrerenderedFiles map[string]string `json:"prerenderedFiles"`
		Routes           []struct {
			ID string `json:"id"`
		} `json:"routes"`
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	if !contains(manifest.Prerendered, "/old") {
		t.Fatalf("Kit's recorded prerender paths omit /old: %v", manifest.Prerendered)
	}
	for _, p := range []string{"/second", "/ordinary-old", "/target?from=atlas", "/target?from=beacon", "/target"} {
		if !contains(manifest.Prerendered, p) {
			t.Errorf("Kit's recorded prerender paths omit %s: %v", p, manifest.Prerendered)
		}
	}
	if contains(manifest.Prerendered, "/ordinary") || contains(manifest.Prerendered, "/ordinary?from=legacy") {
		t.Errorf("ordinary SSR destination was unexpectedly prerendered: %v", manifest.Prerendered)
	}
	ordinaryRoute := false
	for _, route := range manifest.Routes {
		if route.ID == "/ordinary" {
			ordinaryRoute = true
			break
		}
	}
	if !ordinaryRoute {
		t.Error("ordinary SSR destination was removed from the dynamic route manifest")
	}
	artifact, err := os.ReadFile(filepath.Join(ui, "build", "prerendered", "old.html"))
	if err != nil {
		t.Fatalf("Kit did not write the native redirect artifact: %v", err)
	}
	for _, want := range []string{`location.href="/target?from=atlas"`, `http-equiv="refresh"`, `url=/target?from=atlas`} {
		if !strings.Contains(string(artifact), want) {
			t.Errorf("native redirect artifact lacks %q: %s", want, artifact)
		}
	}
	physicalFiles := map[string]string{}
	for _, logical := range []string{"target?from=atlas.html", "target?from=beacon.html"} {
		kitBase := filepath.Join(ui, ".svelte-kit", "output", "prerendered", "pages", filepath.FromSlash(logical))
		native, err := os.ReadFile(kitBase)
		if err != nil {
			t.Fatalf("Kit's original query artifact %q is missing: %v", logical, err)
		}
		if !bytes.Contains(native, []byte("<h1>Target route</h1>")) {
			t.Errorf("Kit's native artifact %q lacks target content", logical)
		}
		for _, suffix := range []string{"", ".br", ".gz"} {
			mappedName := logical + suffix
			physical, ok := manifest.PrerenderedFiles[mappedName]
			if !ok || !strings.HasPrefix(physical, "prerendered-files/") || strings.Contains(physical, "?") {
				t.Fatalf("query artifact %q has no safe mapping: %q", mappedName, physical)
			}
			if previous, exists := physicalFiles[physical]; exists {
				t.Fatalf("query artifacts %q and %q share physical file %q", previous, mappedName, physical)
			}
			physicalFiles[physical] = mappedName
			artifact, err := os.ReadFile(filepath.Join(ui, "build", filepath.FromSlash(physical)))
			if err != nil {
				t.Fatalf("reading mapped query artifact %q: %v", mappedName, err)
			}
			if suffix == "" && !bytes.Equal(artifact, native) {
				t.Errorf("mapped query artifact %q differs from Kit's native bytes", mappedName)
			}
			if suffix == ".gz" {
				reader, err := gzip.NewReader(bytes.NewReader(artifact))
				if err != nil {
					t.Fatalf("mapped gzip artifact %q: %v", mappedName, err)
				}
				decoded, err := io.ReadAll(reader)
				_ = reader.Close()
				if err != nil || !bytes.Equal(decoded, native) {
					t.Errorf("mapped gzip artifact %q does not decode to exact native bytes: %v", mappedName, err)
				}
			}
		}
	}
	if _, exists := manifest.PrerenderedFiles["target?from=atlas.html"]; !exists {
		t.Fatal("atlas artifact missing from manifest")
	}
	if _, exists := manifest.PrerenderedFiles["target?from=beacon.html"]; !exists {
		t.Fatal("beacon artifact missing from manifest")
	}
	if _, err := os.Stat(filepath.Join(ui, "build", "prerendered", "target.html")); err != nil {
		t.Fatalf("canonical pathname artifact missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(ui, "build", "prerendered", "ordinary.html")); !os.IsNotExist(err) {
		t.Fatalf("ordinary SSR target has unexpected native artifact, err %v", err)
	}

	fixture.run(t, "TestProductionRedirectServing")
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	if exit, ok := err.(*exec.ExitError); ok {
		return exit.ExitCode()
	}
	return -1
}

func replaceOnce(path, old, replacement string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	updated := strings.Replace(string(content), old, replacement, 1)
	if updated == string(content) {
		return os.ErrNotExist
	}
	return os.WriteFile(path, []byte(updated), 0o644)
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
