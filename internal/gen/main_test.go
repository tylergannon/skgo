package gen

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// packageTemp holds the generation cache and shared endpoint emitter fixture.
var packageTemp string

func TestMain(m *testing.M) {
	// Every go command these tests start runs in a sandbox or a fixture module
	// outside this checkout's workspace, so it has to see that module's own
	// go.mod. Set once for the process because t.Setenv forbids t.Parallel, and
	// the toolchain-heavy tests here are only affordable run in parallel.
	// Fresh cache-process probes intentionally inherit the caller workspace.
	if os.Getenv("SKGO_CACHE_TEST_CONFIG") == "" {
		os.Setenv("GOWORK", "off")
	}
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
	packageTemp = dir
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// repoRoot is the skgo module this package lives in.
func repoRoot() (string, error) {
	return filepath.Abs(filepath.Join("..", ".."))
}
