package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// Pure command/flag tests do not need a second copy of the CLI linked.
// Process-boundary tests request the shared executable lazily.
var skgoBinDir string
var skgoBinOnce sync.Once
var skgoBinPath string
var skgoBinErr error

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "skgo-cmd-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	skgoBinDir = dir
	os.Setenv("SKGO_CACHE_DIR", filepath.Join(dir, "generation-cache"))
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func skgoBinary(t *testing.T) string {
	t.Helper()
	skgoBinOnce.Do(func() {
		skgoBinPath = filepath.Join(skgoBinDir, "skgo")
		build := exec.CommandContext(t.Context(), "go", "build", "-o", skgoBinPath, ".")
		if output, err := build.CombinedOutput(); err != nil {
			skgoBinErr = fmt.Errorf("build skgo: %w\n%s", err, output)
		}
	})
	if skgoBinErr != nil {
		t.Fatal(skgoBinErr)
	}
	return skgoBinPath
}

func TestMultipleApplicationSelectionsAreRejected(t *testing.T) {
	for _, args := range [][]string{
		{"generate", "--locals-package", "example.com/app", "--hook-package", "example.com/first", "--hook-package=example.com/second"},
		{"check", "-hook-package=example.com/first", "-hook-package=example.com/second"},
	} {
		output, err := exec.Command(skgoBinary(t), args...).CombinedOutput()
		if err == nil || !strings.Contains(string(output), "multiple configured selections for hook-package") {
			t.Fatalf("selection diagnostic: %v %s", err, output)
		}
	}
}
