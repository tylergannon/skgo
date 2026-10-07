package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// skgoBin is the real CLI, built once for the package. Every test here
// drives the binary a developer runs, and linking it is the same few seconds
// whichever test asks, so the tests share one.
var skgoBin string

func TestMain(m *testing.M) {
	// The CLI runs and go commands these tests start run beside a dozen
	// others under `just test`; see internal/gen's TestMain.
	os.Setenv("GOMAXPROCS", "2")
	dir, err := os.MkdirTemp("", "skgo-cmd-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := func() int {
		defer os.RemoveAll(dir)
		repo, err := filepath.Abs(filepath.Join("..", ".."))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		skgoBin = filepath.Join(dir, "skgo")
		build := exec.Command("go", "build", "-o", skgoBin, "./cmd/skgo")
		build.Dir = repo
		if output, err := build.CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "build skgo: %v\n%s", err, output)
			return 1
		}
		return m.Run()
	}()
	os.Exit(code)
}

func TestMultipleApplicationSelectionsAreRejected(t *testing.T) {
	for _, args := range [][]string{
		{"generate", "--locals-package", "example.com/app", "--hook-package", "example.com/first", "--hook-package=example.com/second"},
		{"check", "-hook-package=example.com/first", "-hook-package=example.com/second"},
	} {
		output, err := exec.Command(skgoBin, args...).CombinedOutput()
		if err == nil || !strings.Contains(string(output), "multiple configured selections for hook-package") {
			t.Fatalf("selection diagnostic: %v %s", err, output)
		}
	}
}
