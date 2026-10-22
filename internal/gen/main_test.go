package gen

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	// Every go command and skgo binary these tests start is one of a dozen
	// running at once. Left at one P per core, each spends most of its CPU in
	// the kernel on runtime threads with nothing to do: a single `go generate`
	// of the example measured 21s of system time for 5s of wall clock at the
	// default, 4s at two, with the same wall clock.
	os.Setenv("GOMAXPROCS", "2")
	dir, err := os.MkdirTemp("", "skgo-gen-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if os.Getenv("SKGO_CACHE_TEST_CONFIG") == "" {
		os.Setenv("SKGO_CACHE_DIR", filepath.Join(dir, "generation-cache"))
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// repoRoot is the skgo module this package lives in.
func repoRoot() (string, error) {
	return filepath.Abs(filepath.Join("..", ".."))
}
