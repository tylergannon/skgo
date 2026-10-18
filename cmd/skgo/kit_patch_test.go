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
	// Exercise the project-local CLI used in CI and consumer recipes. It has
	// no global-only `env` command; exec must honor the selected project pin.
	vp, err := filepath.Abs(filepath.Join("..", "..", "example", "web", "node_modules", ".bin", "vp"))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("VP_CLI_BIN", vp)
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "pnpm"), []byte("#!/bin/sh\necho 'ambient pnpm must not run' >&2\nexit 97\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	web := filepath.Join(root, "web")
	if err := os.MkdirAll(web, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := []byte(`{"name":"cli-fixture","dependencies":{"@sveltejs/kit":"^3.1.0"},"devEngines":{"packageManager":{"name":"pnpm","version":"12.14.0","onFail":"download"}}}`)
	if err := os.WriteFile(filepath.Join(web, "package.json"), manifest, 0o644); err != nil {
		t.Fatal(err)
	}
	output, err := runSkgoCommandIn(t, root, "kit-patch", "--web", "web", "--apply")
	if err != nil {
		t.Fatalf("kit-patch --apply: %v\n%s", err, output)
	}
	if !strings.Contains(output, "Configured the Kit 3.1.0") || !strings.Contains(output, "Installation is still required") || !strings.Contains(output, "--check") {
		t.Fatalf("CLI overstated or omitted install/check instructions:\n%s", output)
	}
	manifestBytes, err := os.ReadFile(filepath.Join(web, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(manifestBytes, []byte(`"@sveltejs/kit":"3.1.0"`)) {
		t.Fatalf("CLI did not pin Kit exactly: %s", manifestBytes)
	}
	_, wantPatch, err := adapter.KitQueueCompatibility()
	if err != nil {
		t.Fatal(err)
	}
	gotPatch, err := os.ReadFile(filepath.Join(web, "patches", "skgo-kit-3.1.0-queue.patch"))
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
	cmd := exec.Command(skgoBinary(t), args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	return string(output), err
}
