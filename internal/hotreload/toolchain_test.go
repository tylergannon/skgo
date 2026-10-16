package hotreload

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNativeStartupDiagnostics(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	source := filepath.Join(dir, "main.go")
	// A Go executable supplies inspectable toolchain metadata. This is not a
	// shell shim, and never executes or loads a purported compiler artifact.
	if err := os.WriteFile(source, []byte(`package main;import("os");func main(){os.Stdout.WriteString(os.Getenv("SKGO_TEST_GO_ENV"))}`), 0644); err != nil {
		t.Fatal(err)
	}
	probe := filepath.Join(dir, "inspect-go")
	build := exec.Command("go", "build", "-o", probe, source)
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if raw, err := build.CombinedOutput(); err != nil {
		t.Fatalf("probe: %v\n%s", err, raw)
	}
	for _, fixture := range []struct{ name, version, os, arch, root, want string }{
		{"Go series", "go1.28.0", "darwin", "arm64", dir, "requires qualified Go 1.27.2"},
		{"unqualified patch", "go1.27.3", "darwin", "arm64", dir, "requires qualified Go 1.27.2"},
		{"platform", "go1.27.2", "windows", "amd64", dir, "not qualified on windows/amd64"},
		{"compiler missing", "go1.27.2", runtime.GOOS, runtime.GOARCH, dir, "compiler prerequisite"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			raw, _ := json.Marshal(map[string]string{"GOVERSION": fixture.version, "GOOS": fixture.os, "GOARCH": fixture.arch, "GOROOT": fixture.root})
			t.Setenv("SKGO_TEST_GO_ENV", string(raw))
			_, err := PrepareToolchain(ctx, probe, filepath.Join(dir, "prepared"))
			if err == nil || !strings.Contains(err.Error(), fixture.want) {
				t.Fatalf("diagnostic %v lacks %q", err, fixture.want)
			}
		})
	}
	t.Run("Go missing", func(t *testing.T) {
		_, err := PrepareToolchain(ctx, filepath.Join(dir, "missing-go"), dir)
		if err == nil || !strings.Contains(err.Error(), "inspect Go toolchain") {
			t.Fatalf("missing Go diagnostic: %v", err)
		}
	})
	for _, flag := range []string{"-race", "-gcflags=all=-N", "-tags=custom", "-trimpath"} {
		t.Run(flag, func(t *testing.T) {
			t.Setenv("GOFLAGS", flag)
			_, err := PrepareToolchain(ctx, "go", dir)
			if err == nil || !strings.Contains(err.Error(), "native development profile") {
				t.Fatalf("profile diagnostic: %v", err)
			}
		})
	}
}
