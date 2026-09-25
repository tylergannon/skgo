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
