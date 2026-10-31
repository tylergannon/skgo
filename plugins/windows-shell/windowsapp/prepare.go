package windowsapp

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	nativeclient "github.com/tylergannon/skgo/plugins/native-client"
	"github.com/tylergannon/skgo/plugins/windows-shell/sdk"
)

// Prepare exports the pinned SDK and cross-compiles its Windows x64 DLL.
// It needs no Windows tooling, local Go application server, or C# compiler.
func Prepare(ctx context.Context, root string, out io.Writer) error {
	if !filepath.IsAbs(root) {
		return fmt.Errorf("Windows prepare requires an absolute project root")
	}
	root = filepath.Clean(root)
	if out == nil {
		out = io.Discard
	}
	if _, err := os.Stat(filepath.Join(root, "windows/SKGoShell.csproj")); err != nil {
		return fmt.Errorf("run windows-shell setup first: %w", err)
	}
	sdkRoot := filepath.Join(root, "windows/SDK")
	if err := nativeclient.CopySDK(sdk.Resources(), sdkRoot, "windows-shell"); err != nil {
		return err
	}
	native := filepath.Join(root, "windows/native")
	for p := native; p != root; p = filepath.Dir(p) {
		if s, err := os.Lstat(p); err == nil && s.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("native output contains a symlink: %s", p)
		}
	}
	cmd := exec.CommandContext(ctx, "mise", "exec", "--", "zig", "build", "shared", "-Dtarget=x86_64-windows-gnu", "-Doptimize=ReleaseFast", "--prefix", native)
	cmd.Dir = filepath.Join(sdkRoot, "core")
	cmd.Env = append(os.Environ(), "MISE_TRUSTED_CONFIG_PATHS="+sdkRoot)
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("build Windows native core: %w", err)
	}
	fmt.Fprintln(out, "Prepared Windows x64 SDK and native DLL. Run windows/build.ps1 on Windows.")
	return nil
}
