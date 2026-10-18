package native_test

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

var zigBuildOnce sync.Once
var zigBuildErr error

// This package checks the SKGO client/server boundary. Native-language unit
// suites run in their own toolchains, rather than being nested inside go test.
// Keep their ordinary incremental compiler caches and share immutable artifacts.
func zigRemoteCLI(t *testing.T) string {
	t.Helper()
	zigBuildOnce.Do(func() {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
		defer cancel()
		build := exec.CommandContext(ctx, "mise", "exec", "--", "zig", "build", "install")
		build.Dir = "core"
		if out, err := build.CombinedOutput(); err != nil {
			zigBuildErr = fmt.Errorf("build SKGO native clients: %w\n%s", err, out)
		}
	})
	if zigBuildErr != nil {
		t.Fatal(zigBuildErr)
	}
	return filepath.Join("core", "zig-out", "bin", "skgo-remote")
}
