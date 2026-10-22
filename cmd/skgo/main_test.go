package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/tylergannon/skgo/internal/check"
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

func TestFailedCheckReturnsStructuredOutputAndNonzeroExit(t *testing.T) {
	t.Parallel()
	cmd := exec.CommandContext(t.Context(), skgoBinary(t), "check", "--root", t.TempDir(), "--json")
	output, err := cmd.Output()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 {
		t.Fatalf("failed check exit: %v\n%s", err, output)
	}
	var report check.Report
	if err := json.Unmarshal(output, &report); err != nil || report.OK || len(report.Checks) != 1 || report.Checks[0].Name != "project" || report.Checks[0].Status != "failed" || !strings.Contains(report.Checks[0].Message, "go.mod is missing") {
		t.Fatalf("failed check report: %v\n%s", err, output)
	}
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
		var output bytes.Buffer
		code := run(t.Context(), args, strings.NewReader(""), &output, &output)
		if code != 2 || output.String() != "skgo: multiple configured selections for hook-package\n" {
			t.Fatalf("selection status=%d diagnostic=%s", code, output.String())
		}
	}
}
