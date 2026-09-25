package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// skgoBin is the real CLI, built once for the package. Every test here
// drives the binary a developer runs, and linking it is the same few seconds
// whichever test asks, so the tests share one.
var skgoBin string

func TestMain(m *testing.M) {
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
