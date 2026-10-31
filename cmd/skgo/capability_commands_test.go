//go:build skgo_qualification

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/tylergannon/skgo/internal/pluginbuild"
	"github.com/tylergannon/skgo/internal/pluginstore"
	"golang.org/x/mod/module"
	modzip "golang.org/x/mod/zip"
)

// This is a local provisional host, built as a versioned module so the test
// exercises the real release-shaped install path without special runtime flags.
// It cannot certify compatibility with an independently published executable.
func provisionalCapabilityHost(t *testing.T, repo, root string) string {
	return provisionalCapabilityHostVersion(t, repo, root, "v0.0.0-capability")
}

func provisionalCapabilityHostVersion(t *testing.T, repo, root, version string) string {
	t.Helper()
	proxy := filepath.Join(root, "proxy", "github.com", "tylergannon", "skgo", "@v")
	if err := os.MkdirAll(proxy, 0755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), "git", "ls-files", "--cached", "--others", "--exclude-standard")
	cmd.Dir = repo
	listed, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	var files []modzip.File
	for _, name := range strings.Split(strings.TrimSpace(string(listed)), "\n") {
		if name == "" || strings.HasPrefix(name, "ephemeral/") || strings.HasPrefix(name, "example/") || strings.HasPrefix(name, "plugins/") || strings.HasPrefix(name, ".cache/") {
			continue
		}
		full := filepath.Join(repo, filepath.FromSlash(name))
		info, err := os.Lstat(full)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		files = append(files, capabilityModuleFile{name: name, full: full, info: info})
	}
	zip, err := os.Create(filepath.Join(proxy, version+".zip"))
	if err != nil {
		t.Fatal(err)
	}
	if err := modzip.Create(zip, module.Version{Path: "github.com/tylergannon/skgo", Version: version}, files); err != nil {
		t.Fatal(err)
	}
	if err := zip.Close(); err != nil {
		t.Fatal(err)
	}
	mod, err := os.ReadFile(filepath.Join(repo, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{version + ".mod": mod, version + ".info": []byte(`{"Version":"` + version + `","Time":"2026-10-19T00:00:00Z"}`), "list": []byte(version + "\n")} {
		if err := os.WriteFile(filepath.Join(proxy, name), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	// Everything builds offline: skgo from the zip above, its dependencies from
	// the local module cache. Extraction goes to a private cache so the fixture
	// version never enters the shared one, and nothing here is about dependency
	// integrity, so the checksum database stays off.
	dependencies := capabilityDependencyProxy(t, repo)
	t.Setenv("GOWORK", "")
	t.Setenv("GOMODCACHE", filepath.Join(root, "modcache"))
	t.Setenv("GOPATH", filepath.Join(root, "gopath"))
	t.Setenv("GOBIN", filepath.Join(root, "bin"))
	t.Setenv("GOPROXY", (&url.URL{Scheme: "file", Path: filepath.Join(root, "proxy")}).String()+","+dependencies)
	t.Setenv("GONOPROXY", "none")
	t.Setenv("GOSUMDB", "off")
	t.Setenv("CGO_ENABLED", "1")
	t.Cleanup(func() {
		_ = filepath.WalkDir(filepath.Join(root, "modcache"), func(path string, d fs.DirEntry, err error) error {
			if err == nil && d.IsDir() {
				_ = os.Chmod(path, 0755)
			}
			return nil
		})
	})
	build := exec.CommandContext(t.Context(), "go", "install", "-trimpath", "github.com/tylergannon/skgo/cmd/skgo@"+version)
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build provisional CLI: %v\n%s", err, output)
	}
	return filepath.Join(root, "bin", "skgo")
}

// Serve the local module cache's downloads read-only, never the shared
// extraction cache.
func capabilityDependencyProxy(t *testing.T, repo string) string {
	t.Helper()
	cache := exec.CommandContext(t.Context(), "go", "env", "GOMODCACHE")
	cache.Dir = repo
	output, err := cache.Output()
	if err != nil {
		t.Fatal(err)
	}
	cacheDir := filepath.Join(strings.TrimSpace(string(output)), "cache", "download")
	files := http.FileServer(http.Dir(cacheDir))
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A normal Go download can omit .info for lazily pruned dependencies.
		// go list needs their version/time, neither of which authenticates
		// source. Supply deterministic fixture metadata rather than fetching
		// unused dependencies. Archive and go.mod verification stay unchanged.
		if strings.HasSuffix(r.URL.Path, ".info") {
			if _, err := os.Stat(filepath.Join(cacheDir, filepath.FromSlash(r.URL.Path))); os.IsNotExist(err) {
				version, err := module.UnescapeVersion(strings.TrimSuffix(filepath.Base(r.URL.Path), ".info"))
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]string{"Version": version, "Time": "2026-10-19T00:00:00Z"})
				return
			}
		}
		files.ServeHTTP(w, r)
	}))
	t.Cleanup(proxy.Close)
	return proxy.URL
}

type capabilityModuleFile struct {
	name, full string
	info       os.FileInfo
}

func (f capabilityModuleFile) Path() string                 { return f.name }
func (f capabilityModuleFile) Lstat() (os.FileInfo, error)  { return f.info, nil }
func (f capabilityModuleFile) Open() (io.ReadCloser, error) { return os.Open(f.full) }

func TestCapabilityPluginActualCLI(t *testing.T) {
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	host := provisionalCapabilityHost(t, repo, root)
	project := filepath.Join(root, "application")
	plugins := filepath.Join(root, "manual")
	for _, dir := range []string{filepath.Join(project, "web/src"), filepath.Join(project, "internal/app"), plugins} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(project, "go.mod"), []byte("module example.test/capability-consumer\n\ngo 1.27.1\nrequire github.com/tylergannon/skgo v0.0.0-capability\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "skgo.json"), []byte(`{"version":1,"name":"CapabilityApp"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "internal/app/locals.go"), []byte("package app\ntype Locals struct{}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile("testdata/capability-plugin/plugin.go")
	if err != nil {
		t.Fatal(err)
	}
	buildPlugin := func(moduleName, out string) {
		t.Helper()
		source := filepath.Join(root, moduleName)
		if err := os.MkdirAll(source, 0755); err != nil {
			t.Fatal(err)
		}
		for name, data := range map[string][]byte{"main.go": fixture, "go.mod": []byte("module example.test/" + moduleName + "\n\ngo 1.27.1\nrequire github.com/tylergannon/skgo v0.0.0-capability\n")} {
			if err := os.WriteFile(filepath.Join(source, name), data, 0644); err != nil {
				t.Fatal(err)
			}
		}
		var log bytes.Buffer
		if err := pluginbuild.Build(t.Context(), pluginbuild.Options{Host: host, Source: source, Output: out, VersionSymbol: "main.skgoVersion", Log: &log}); err != nil {
			t.Fatalf("build fixture: %v\n%s", err, &log)
		}
	}
	artifact := filepath.Join(root, fmt.Sprintf("capability-fixture-v1.0.0-skgo-v0.0.0-capability-%s-%s.so", runtime.GOOS, runtime.GOARCH))
	buildPlugin("capability-one", artifact)
	digest, err := pluginstore.Digest(artifact)
	if err != nil {
		t.Fatal(err)
	}
	commandDir := project
	commandHome := root
	run := func(mode, input string, args ...string) (string, string, error) {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), host, args...)
		cmd.Dir = commandDir
		cmd.Env = append(os.Environ(), "HOME="+commandHome, "SKGO_PLUGIN_DIRS="+plugins, "SKGO_FIXTURE_MODE="+mode)
		cmd.Stdin = strings.NewReader(input)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		return stdout.String(), stderr.String(), err
	}
	require := func(mode, input string, args ...string) string {
		t.Helper()
		out, stderr, err := run(mode, input, args...)
		if err != nil {
			t.Fatalf("%v: %v\n%s%s", args, err, out, stderr)
		}
		return out
	}
	_, rejection, installErr := run("both", "", "plugin", "install", "--prebuilt", artifact, "--sha256", strings.Repeat("0", 64))
	if installErr == nil || !strings.Contains(rejection, "checksum") {
		t.Fatalf("checksum rejection: %v %s", installErr, rejection)
	}
	wrongVersion := filepath.Join(root, fmt.Sprintf("capability-fixture-v1.0.0-skgo-v999.0.0-%s-%s.so", runtime.GOOS, runtime.GOARCH))
	_, rejection, installErr = run("both", "", "plugin", "install", "--prebuilt", wrongVersion, "--sha256", digest)
	if installErr == nil || !strings.Contains(rejection, "requires official skgo v999.0.0") {
		t.Fatalf("version rejection: %v %s", installErr, rejection)
	}
	require("both", "", "plugin", "install", "--prebuilt", artifact, "--sha256", digest, "--set", "value=literal-generated")
	// Missing slots affect their provider, without blocking unrelated help.
	commandHome = filepath.Join(root, "fresh-home")
	require("both", "", "help")
	require("both", "", "add", "-h", "tailwindcss")
	_, rejection, err = run("both", "", "plugin", "capability-fixture")
	if err == nil || !strings.Contains(rejection, "run skgo plugin restore") {
		t.Fatalf("unrestored command: %v %s", err, rejection)
	}
	commandHome = root
	// The same Go pluginpath can make Open refuse the verified second copy.
	// An earlier unverified build must never satisfy the project pin.
	originalFixture := fixture
	fixture = bytes.ReplaceAll(fixture, []byte("fixture stderr"), []byte("UNVERIFIED stderr"))
	shadow := filepath.Join(plugins, "shadow.so")
	buildPlugin("capability-one", shadow)
	fixture = originalFixture
	for _, args := range [][]string{
		{"plugin", "capability-fixture"},
		{"generate", "--web", filepath.Join(project, "web"), "--out", filepath.Join(project, "bindings"), "--locals-package", "example.test/capability-consumer/internal/app", "--plugins"},
	} {
		_, rejection, err = run("both", "", args...)
		if err == nil || !strings.Contains(rejection, "verified artifact") {
			t.Fatalf("shadowed pin %v: %v %s", args, err, rejection)
		}
	}
	if err := os.Remove(shadow); err != nil {
		t.Fatal(err)
	}
	require("both", "", "generate", "--web", filepath.Join(project, "web"), "--out", filepath.Join(project, "bindings"), "--locals-package", "example.test/capability-consumer/internal/app", "--plugins")
	generated := filepath.Join(project, "generated/fixture.txt")
	data, err := os.ReadFile(generated)
	if err != nil || string(data) != "example.test/capability-consumer\nliteral-generated\n" {
		t.Fatalf("generated output=%q err=%v", data, err)
	}
	// Explicit output selects the owning project's pins even from another cwd.
	commandDir = root
	require("both", "", "generate", "--web", filepath.Join(project, "web"), "--out", filepath.Join(project, "bindings"), "--locals-package", "example.test/capability-consumer/internal/app", "--plugins")
	commandDir = project
	out, stderr, err := run("both", "literal stdin\n", "plugin", "capability-fixture", "--flag", "a b")
	if err != nil || out != "example.test/capability-consumer\n--flag|a b\nliteral stdin\n" || !strings.Contains(stderr, "fixture stderr") {
		t.Fatalf("forwarding: out=%q errout=%q err=%v", out, stderr, err)
	}
	_, stderr, err = run("both", "", "plugin", "capability-fixture", "fail")
	if err == nil || !strings.Contains(stderr, "fixture command failed") {
		t.Fatalf("command failure: %v %s", err, stderr)
	}
	if err := os.WriteFile(generated, []byte("authored\n"), 0644); err != nil {
		t.Fatal(err)
	}
	_, stderr, err = run("both", "", "generate", "--web", filepath.Join(project, "web"), "--out", filepath.Join(project, "bindings"), "--locals-package", "example.test/capability-consumer/internal/app", "--plugins")
	if err == nil || !strings.Contains(stderr, "authored or edited") {
		t.Fatalf("output protection: %v %s", err, stderr)
	}
	data, _ = os.ReadFile(generated)
	if string(data) != "authored\n" {
		t.Fatalf("authored output replaced: %q", data)
	}
	require("both", "", "plugin", "install", "--prebuilt", artifact, "--sha256", digest, "--set", "fail=true")
	_, stderr, err = run("both", "", "generate", "--web", filepath.Join(project, "web"), "--out", filepath.Join(project, "bindings"), "--locals-package", "example.test/capability-consumer/internal/app", "--plugins")
	if err == nil || !strings.Contains(stderr, "fixture generation failed") {
		t.Fatalf("generator failure: %v %s", err, stderr)
	}
	data, _ = os.ReadFile(generated)
	if string(data) != "authored\n" {
		t.Fatalf("failed generator changed output: %q", data)
	}
	require("both", "", "plugin", "install", "--prebuilt", artifact, "--sha256", digest, "--set", "value=literal-generated")
	// A changed descriptor can expose either capability through the same factory.
	require("command", "literal\n", "plugin", "capability-fixture", "hello")
	if err := os.Remove(generated); err != nil {
		t.Fatal(err)
	}
	require("generator", "", "generate", "--web", filepath.Join(project, "web"), "--out", filepath.Join(project, "bindings"), "--locals-package", "example.test/capability-consumer/internal/app", "--plugins")
	// Source/manual discovery remains valid independently of pinned installation.
	if err := os.Remove(filepath.Join(project, "skgo-plugins.json")); err != nil {
		t.Fatal(err)
	}
	first := filepath.Join(plugins, "one.so")
	bytes, err := os.ReadFile(artifact)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(first, bytes, 0644); err != nil {
		t.Fatal(err)
	}
	help := require("command", "", "plugin", "--help")
	if !strings.Contains(help, "capability-fixture:") {
		t.Fatalf("command-only discovery missing: %s", help)
	}
	help = require("generator", "", "plugin", "--help")
	if strings.Contains(help, "capability-fixture:") {
		t.Fatalf("generator-only advertised command: %s", help)
	}
	// Distinct modules with the same descriptor must be rejected, not shadowed.
	buildPlugin("capability-two", filepath.Join(plugins, "two.so"))
	out, stderr, err = run("command", "", "plugin", "capability-fixture")
	if err == nil || !strings.Contains(stderr, "duplicate plugin ID") {
		t.Fatalf("duplicate providers: %v\n%s%s", err, out, stderr)
	}
	if err := os.Remove(filepath.Join(plugins, "two.so")); err != nil {
		t.Fatal(err)
	}
	for _, reserved := range []string{"install", "remove", "restore"} {
		cmd := exec.CommandContext(t.Context(), host, "plugin", "--help")
		cmd.Dir = commandDir
		cmd.Env = append(os.Environ(), "HOME="+commandHome, "SKGO_PLUGIN_DIRS="+plugins, "SKGO_FIXTURE_MODE=command", "SKGO_FIXTURE_ID="+reserved)
		data, err := cmd.CombinedOutput()
		if err != nil || !strings.Contains(string(data), "conflicts with a management command") {
			t.Fatalf("reserved %s: %v\n%s", reserved, err, data)
		}
	}
}
