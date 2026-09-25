package gen

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

// packageTemp holds what more than one test in this package reads: the skgo
// binary and the sandboxes shared through sync.Once below. A t.TempDir belongs
// to one test and is gone when it ends, which is exactly wrong for a result
// computed once and asserted by several tests running in parallel.
var packageTemp string

func TestMain(m *testing.M) {
	// Every go command these tests start runs in a sandbox or a fixture module
	// outside this checkout's workspace, so it has to see that module's own
	// go.mod. Set once for the process because t.Setenv forbids t.Parallel, and
	// the toolchain-heavy tests here are only affordable run in parallel.
	os.Setenv("GOWORK", "off")
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
	packageTemp = dir
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// repoRoot is the skgo module this package lives in.
func repoRoot() (string, error) {
	return filepath.Abs(filepath.Join("..", ".."))
}

// skgoBinary is the real CLI, built once for the package: linking it is the
// same few seconds whichever test asks first, and nothing a test does can
// change it.
var skgoBinary = sync.OnceValues(func() (string, error) {
	root, err := repoRoot()
	if err != nil {
		return "", err
	}
	bin := filepath.Join(packageTemp, "bin", "skgo")
	build := exec.Command("go", "build", "-o", bin, "./cmd/skgo")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		return "", fmt.Errorf("build skgo: %v\n%s", err, output)
	}
	return bin, nil
})

func requireSkgoBinary(t *testing.T) string {
	t.Helper()
	bin, err := skgoBinary()
	if err != nil {
		t.Fatal(err)
	}
	return bin
}

// sharedSandbox copies the example app into packageTemp/name, for a sandbox
// whose result more than one test asserts.
func sharedSandbox(name string) (string, error) {
	root, err := repoRoot()
	if err != nil {
		return "", err
	}
	return copyExample(root, filepath.Join(packageTemp, name))
}
