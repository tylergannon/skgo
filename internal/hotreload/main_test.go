//go:build skgo_qualification

package hotreload

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

var nativeFixtureDir string
var nativeToolchainOnce sync.Once
var nativeToolchain Toolchain
var nativeToolchainErr error

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "skgo-native-tests-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	nativeFixtureDir = dir
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// Preparation copies immutable compiler inputs. Each host still owns its
// application, module overlays, compiled candidates and mutable loader view.
func sharedNativeToolchain(t *testing.T, ctx context.Context) Toolchain {
	t.Helper()
	nativeToolchainOnce.Do(func() {
		nativeToolchain, nativeToolchainErr = PrepareToolchain(ctx, "go", filepath.Join(nativeFixtureDir, "toolchain"))
	})
	if nativeToolchainErr != nil {
		t.Fatal(nativeToolchainErr)
	}
	return nativeToolchain
}
