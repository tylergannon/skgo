package newapp

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "skgo-newapp-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Setenv("SKGO_CACHE_DIR", filepath.Join(dir, "generation-cache"))
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
