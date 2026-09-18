package svaddon

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

var packageFiles = []string{
	"LICENSE",
	"README.md",
	"package.json",
	"sv-addon.js",
}

func TestPublishedPackageContainsOnlyTheNativeAddon(t *testing.T) {
	tarball := pack(t)
	got := make([]string, 0, len(tarball))
	for name := range tarball {
		got = append(got, name)
	}
	slices.Sort(got)
	if !slices.Equal(got, packageFiles) {
		t.Fatalf("`pnpm pack` would publish:\n  %s\nwant only:\n  %s",
			strings.Join(got, "\n  "), strings.Join(packageFiles, "\n  "))
	}
}

func TestPublishedPackageIsASelfContainedNativeAddon(t *testing.T) {
	tarball := pack(t)
	var pkg struct {
		Name             string            `json:"name"`
		Type             string            `json:"type"`
		Exports          map[string]any    `json:"exports"`
		Files            []string          `json:"files"`
		Dependencies     map[string]string `json:"dependencies"`
		PeerDependencies map[string]string `json:"peerDependencies"`
		DevDependencies  map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(tarball["package.json"], &pkg); err != nil {
		t.Fatal(err)
	}
	if pkg.Name != "@skgo/sv" || pkg.Type != "module" {
		t.Errorf("package identity = %+v; want the @skgo/sv ESM package", pkg)
	}
	if _, ok := pkg.Exports["."]; !ok {
		t.Error("package has no root export for sv's community add-on loader")
	}
	if pkg.PeerDependencies["sv"] == "" {
		t.Error("package does not declare the sv host as a peer")
	}
	if len(pkg.Dependencies) != 0 {
		t.Errorf("published add-on has runtime dependencies %v; build-only utilities must be bundled", pkg.Dependencies)
	}
	if pkg.DevDependencies["@sveltejs/sv-utils"] == "" {
		t.Error("source does not declare its build-only sv-utils dependency")
	}
	if bytes.Contains(tarball["sv-addon.js"], []byte("@sveltejs/sv-utils")) {
		t.Error("published add-on imports sv-utils instead of bundling it")
	}
}

func TestAddonKeepsTheGoEmbedPlaceholderTrackable(t *testing.T) {
	for _, name := range []string{"sv-addon.source.js", "sv-addon.js"} {
		body, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(body, []byte("!/build/placeholder")) {
			t.Errorf("%s does not preserve the generated Go embed placeholder", name)
		}
	}
}

func pack(t *testing.T) map[string][]byte {
	t.Helper()
	if _, err := exec.LookPath("pnpm"); err != nil {
		t.Fatalf("pnpm is not on PATH, so this test cannot run: %v", err)
	}
	dest := t.TempDir()
	cmd := exec.Command("pnpm", "pack", "--pack-destination", dest)
	cmd.Dir = "."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("pnpm pack: %v\n%s", err, out)
	}
	entries, err := filepath.Glob(filepath.Join(dest, "*.tgz"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("pnpm pack wrote %v (%v); want one tarball", entries, err)
	}
	return untar(t, entries[0])
}

func untar(t *testing.T, name string) map[string][]byte {
	t.Helper()
	raw, err := os.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	zipped, err := gzip.NewReader(raw)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]byte{}
	archive := tar.NewReader(zipped)
	for {
		header, err := archive.Next()
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		rel, ok := strings.CutPrefix(path.Clean(header.Name), "package/")
		if !ok {
			t.Fatalf("tarball member %s is outside package/", header.Name)
		}
		body, err := io.ReadAll(archive)
		if err != nil {
			t.Fatal(err)
		}
		out[rel] = body
	}
}
