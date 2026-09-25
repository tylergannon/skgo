package example_test

import (
	"fmt"
	"os"
	"testing"
)

// packageTemp holds the sandboxes more than one test asserts on (see
// typedrift_test.go). A t.TempDir belongs to one test and is removed when it
// ends; these outlive every test that reads them, so the package removes them
// once, here.
var packageTemp string

func TestMain(m *testing.M) {
	// The go commands these tests start (go generate, and the go list calls
	// the generator makes) run beside a dozen others under `just test`. At
	// one P per core each spends most of its CPU in the kernel on idle runtime
	// threads; two keeps the same wall clock for a fraction of the CPU. See
	// internal/gen's TestMain for the measurement.
	os.Setenv("GOMAXPROCS", "2")
	dir, err := os.MkdirTemp("", "skgo-example-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	packageTemp = dir
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
