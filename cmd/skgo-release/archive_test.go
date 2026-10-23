//go:build release

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tylergannon/skgo/internal/buildinfo"
	"github.com/tylergannon/skgo/internal/pluginbuild"
)

// Exercise the actual downloadable executable, including native plugin loading
// after Developer ID signing on macOS. The release tag keeps this expensive
// packaging test out of ordinary source tests; requesting it without an archive
// fails rather than skipping qualification.
func TestPackagedCLI(t *testing.T) {
	archive := os.Getenv("SKGO_RELEASE_ARCHIVE")
	tag := os.Getenv("RELEASE_VERSION")
	if archive == "" || !tagPattern.MatchString(tag) {
		t.Fatal("SKGO_RELEASE_ARCHIVE and RELEASE_VERSION are required")
	}
	f, err := os.Open(archive)
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
	dir := t.TempDir()
	binary := filepath.Join(dir, "skgo")
	found := false
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if h.Name != "skgo" {
			continue
		}
		if h.Mode != 0755 {
			t.Fatal("archive CLI is not executable")
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(binary, data, 0755); err != nil {
			t.Fatal(err)
		}
		found = true
	}
	if !found {
		t.Fatal("archive does not contain the CLI")
	}
	call := func(env []string, args ...string) []byte {
		t.Helper()
		c := exec.Command(binary, args...)
		c.Env = append(os.Environ(), env...)
		data, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("CLI %v: %v\n%s", args, err, data)
		}
		return data
	}
	var info buildinfo.Info
	if err := json.Unmarshal(call(nil, "buildinfo", "--json"), &info); err != nil {
		t.Fatal(err)
	}
	if info.SkgoVersion != tag || info.Build.Main.Version != tag || info.Build.GoVersion != "go1.27.3" {
		t.Fatalf("unexpected release identity: %+v", info)
	}
	source := filepath.Join(dir, "source")
	plugins := filepath.Join(dir, "plugins")
	consumer := filepath.Join(dir, "consumer")
	for _, path := range []string{source, plugins, filepath.Join(consumer, "web")} {
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile("../skgo/testdata/template-plugin/plugin.go")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(source, "main.go"), data, 0644); err != nil {
		t.Fatal(err)
	}
	mod := "module example.test/releaseplugin\n\ngo 1.27.3\n\nrequire github.com/tylergannon/skgo " + tag + "\n"
	if err = os.WriteFile(filepath.Join(source, "go.mod"), []byte(mod), 0644); err != nil {
		t.Fatal(err)
	}
	var log bytes.Buffer
	if err = pluginbuild.Build(context.Background(), pluginbuild.Options{Host: binary, Source: source, Output: filepath.Join(plugins, "receipt.so"), VersionSymbol: "main.skgoVersion", Log: &log}); err != nil {
		t.Fatalf("plugin for packaged CLI: %v\n%s", err, log.String())
	}
	env := []string{"SKGO_PLUGIN_DIRS=" + plugins}
	if help := call(env, "--help"); !strings.Contains(string(help), "Write a configured receipt") {
		t.Fatalf("packaged CLI did not load plugin: %s", help)
	}
	if err = os.WriteFile(filepath.Join(consumer, "go.mod"), []byte("module example.test/releaseconsumer\n\ngo 1.27.3\n"), 0644); err != nil {
		t.Fatal(err)
	}
	call(env, "add", "--root", consumer, "--name", "Release", "--set", "text=release-fixture", "receipt")
	receipt, err := os.ReadFile(filepath.Join(consumer, "receipt.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(receipt) != "Release\nexample.test/releaseconsumer\nrelease-fixture\n" {
		t.Fatalf("packaged CLI's plugin produced %q", receipt)
	}
}
