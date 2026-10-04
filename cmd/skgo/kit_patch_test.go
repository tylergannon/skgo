package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tylergannon/skgo/internal/adapter"
)

func TestKitPatchRequiresExactlyOneAction(t *testing.T) {
	for name, args := range map[string][]string{
		"missing":  {"kit-patch", "--web", "."},
		"both":     {"kit-patch", "--web", ".", "--apply", "--check"},
		"position": {"kit-patch", "--web", ".", "--apply", "unexpected"},
	} {
		t.Run(name, func(t *testing.T) {
			output, err := runSkgoCommand(t, args...)
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 2 || !strings.Contains(output, "usage: go tool skgo kit-patch") {
				t.Fatalf("kit-patch args %q: output=%s err=%v", args, output, err)
			}
		})
	}
}

func TestKitPatchCLIConfiguresFilesAndDoesNotClaimInstallation(t *testing.T) {
	pnpm, err := exec.LookPath("pnpm")
	if err != nil {
		t.Fatalf("native pnpm 12.9.1 is required: %v", err)
	}
	version, err := exec.Command(pnpm, "--version").CombinedOutput()
	if err != nil || strings.TrimSpace(string(version)) != "12.9.1" {
		t.Fatalf("native pnpm version = %q, %v; want 12.9.1", strings.TrimSpace(string(version)), err)
	}
	root := t.TempDir()
	web := filepath.Join(root, "web")
	if err := os.MkdirAll(web, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := []byte(`{"name":"cli-fixture","dependencies":{"@sveltejs/kit":"^3.0.0"}}`)
	if err := os.WriteFile(filepath.Join(web, "package.json"), manifest, 0o644); err != nil {
		t.Fatal(err)
	}
	output, err := runSkgoCommandIn(t, root, "kit-patch", "--web", "web", "--apply")
	if err != nil {
		t.Fatalf("kit-patch --apply: %v\n%s", err, output)
	}
	if !strings.Contains(output, "Configured the Kit 3.0.0") || !strings.Contains(output, "Installation is still required") || !strings.Contains(output, "--check") {
		t.Fatalf("CLI overstated or omitted install/check instructions:\n%s", output)
	}
	manifestBytes, err := os.ReadFile(filepath.Join(web, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(manifestBytes, []byte(`"@sveltejs/kit":"3.0.0"`)) {
		t.Fatalf("CLI did not pin Kit exactly: %s", manifestBytes)
	}
	_, wantPatch, err := adapter.KitQueueCompatibility()
	if err != nil {
		t.Fatal(err)
	}
	gotPatch, err := os.ReadFile(filepath.Join(web, "patches", "skgo-kit-3.0.0-queue.patch"))
	if err != nil || !bytes.Equal(gotPatch, wantPatch) {
		t.Fatalf("CLI patch copy differs from the packaged bytes: %v", err)
	}
	output, err = runSkgoCommandIn(t, root, "kit-patch", "--web", "web", "--check")
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 || !strings.Contains(output, "configured but installed Kit could not be verified") {
		t.Fatalf("--check accepted uninstalled configuration: err=%v\n%s", err, output)
	}
}

func runSkgoCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()
	return runSkgoCommandIn(t, "", args...)
}

func runSkgoCommandIn(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(skgoBin, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	return string(output), err
}
