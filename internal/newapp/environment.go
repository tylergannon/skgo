package newapp

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tylergannon/skgo/internal/toolchain"
)

// qualifiedRunner keeps every creation step in the environment owned by the
// qualified vp, including nested package-manager invocations from Go generators.
// Kit patching runs in-process, so it receives the same resolved pnpm directly.
func qualifiedRunner(vp, dir string, stdout, stderr io.Writer) (func(command) error, string, error) {
	requestedVP := vp
	resolved, err := exec.LookPath(vp)
	if err != nil {
		return nil, "", err
	}
	vp, err = filepath.Abs(resolved)
	if err != nil {
		return nil, "", err
	}
	if stderr == nil {
		stderr = os.Stderr
	}
	probe := func(args ...string) ([]byte, error) {
		cmd := exec.Command(vp, args...)
		cmd.Dir = dir
		cmd.Env = companionEnv(os.Environ())
		cmd.Stderr = stderr
		return cmd.Output()
	}
	output, err := probe("--version")
	if err != nil {
		return nil, "", fmt.Errorf("checking VitePlus version: %w", err)
	}
	fields := strings.Fields(string(output))
	if len(fields) < 2 || fields[0] != "vp" || fields[1] != "v"+toolchain.VitePlus {
		firstLine := strings.SplitN(strings.TrimSpace(string(output)), "\n", 2)[0]
		return nil, "", fmt.Errorf("project creation requires qualified VitePlus %s; %s --version reported %q. Install that version before running skgo new", toolchain.VitePlus, vp, firstLine)
	}
	managed := []string{"env", "exec", "--package-manager", "pnpm@" + toolchain.PNPM}
	output, err = probe(append(managed, "which", "pnpm")...)
	if err != nil {
		return nil, "", fmt.Errorf("resolve vp-managed pnpm: %w", err)
	}
	pnpm := strings.TrimSpace(string(output))
	if !filepath.IsAbs(pnpm) {
		return nil, "", fmt.Errorf("vp did not report an absolute pnpm executable: %q", pnpm)
	}
	output, err = probe(append(managed, pnpm, "--version")...)
	if err != nil || strings.TrimSpace(string(output)) != toolchain.PNPM {
		return nil, "", fmt.Errorf("vp environment requires pnpm %s; reported %q (%v)", toolchain.PNPM, output, err)
	}
	run := realRunner(stdout, stderr)
	return func(c command) error {
		if c.Name == requestedVP {
			c.Name = vp
		}
		c.Args = append(append([]string{}, managed...), append([]string{c.Name}, c.Args...)...)
		c.Name = vp
		c.Env = companionEnv(c.Env)
		return run(c)
	}, pnpm, nil
}

func companionEnv(env []string) []string {
	return append(slices.Clone(env), "VP_PACKAGE_MANAGER=pnpm@"+toolchain.PNPM, "VP_PNPM_VERSION="+toolchain.PNPM)
}
