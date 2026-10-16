package main

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArchiveContainsInstallableCLI(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{}
	for name, data := range map[string]string{"skgo": "fixture executable", "LICENSE": "fixture license", "README.md": "fixture readme"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		files[name] = path
	}
	path := filepath.Join(dir, "release.tar.gz")
	if err := writeArchive(path, files); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	want := map[string]string{"skgo": "fixture executable", "LICENSE": "fixture license", "README.md": "fixture readme"}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		if expected, ok := want[h.Name]; !ok || string(data) != expected {
			t.Fatalf("unexpected archive entry %q: %q", h.Name, data)
		}
		if h.Name == "skgo" && h.Mode != 0755 {
			t.Fatalf("CLI is not executable: %o", h.Mode)
		}
		delete(want, h.Name)
	}
	if len(want) != 0 {
		t.Fatalf("archive missing %v", want)
	}
}
func TestChecksumsRequireEveryPlatform(t *testing.T) {
	dir := t.TempDir()
	// Independent SHA-256 fixture: the well-known digest of "abc".
	for _, name := range []string{"skgo_1.2.3_darwin_arm64.tar.gz", "skgo_1.2.3_linux_amd64.tar.gz"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("abc"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := checksums(dir, "v1.2.3"); err == nil {
		t.Fatal("missing Linux arm64 archive allowed publication")
	}
	if _, err := os.Stat(filepath.Join(dir, "checksums.txt")); !os.IsNotExist(err) {
		t.Fatal("partial checksum file created")
	}
	if err := os.WriteFile(filepath.Join(dir, "skgo_1.2.3_linux_arm64.tar.gz"), []byte("abc"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := checksums(dir, "v1.2.3"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "checksums.txt"))
	if err != nil {
		t.Fatal(err)
	}
	const sum = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	want := sum + "  skgo_1.2.3_darwin_arm64.tar.gz\n" + sum + "  skgo_1.2.3_linux_amd64.tar.gz\n" + sum + "  skgo_1.2.3_linux_arm64.tar.gz\n"
	if string(data) != want {
		t.Fatalf("checksums:\n%s", data)
	}
}
func TestCaskUsesVersionedSignedArchive(t *testing.T) {
	ruby, err := cask("v1.2.3", strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{`cask "skgo"`, `version "1.2.3"`, `sha256 "` + strings.Repeat("a", 64) + `"`, `https://github.com/tylergannon/skgo/releases/download/v#{version}/skgo_#{version}_darwin_arm64.tar.gz`, `depends_on arch: :arm64`, `binary "skgo"`} {
		if !strings.Contains(ruby, s) {
			t.Fatalf("cask missing %s", s)
		}
	}
	for _, tag := range []string{"1.2.3", "v1.2.3-rc.1", "v1.2.3\nmalicious"} {
		if _, err := cask(tag, strings.Repeat("a", 64)); err == nil {
			t.Fatalf("accepted invalid tag %q", tag)
		}
	}
	if _, err := cask("v1.2.3", "missing"); err == nil {
		t.Fatal("accepted invalid checksum")
	}
}
func TestCleanupRefusesUnownedDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RUNNER_TEMP", dir)
	t.Setenv("SKGO_CI_SIGNING_DIR", dir)
	if err := cleanupSigning(); err == nil {
		t.Fatal("allowed deletion of runner temp directory")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatal("runner temp directory removed")
	}
}
