package update

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tylergannon/skgo/internal/adapter"
	"github.com/tylergannon/skgo/internal/buildinfo"
	"github.com/tylergannon/skgo/internal/kitpatch"
	"github.com/tylergannon/skgo/internal/toolchain"
	"golang.org/x/mod/modfile"
)

func complete(ctx context.Context, o Options, info buildinfo.Info) error {
	vp := o.VP
	if vp == "" {
		var err error
		vp, err = exec.LookPath("vp")
		if err != nil {
			return fmt.Errorf("install the global VitePlus CLI before updating: %w", err)
		}
	}
	vp, err := filepath.Abs(vp)
	if err != nil {
		return err
	}
	// Outside the application, global vp cannot delegate to the old local version.
	outside, err := os.MkdirTemp("", "skgo-update-toolchain-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(outside)
	run := func(dir string, quiet bool, name string, args ...string) ([]byte, error) {
		return o.run(ctx, command{Dir: dir, Name: name, Args: args, Quiet: quiet})
	}
	version, err := run(outside, true, vp, "--version")
	if err != nil {
		return err
	}
	if !vpVersion(version, toolchain.VitePlus) {
		if _, err := run(outside, false, vp, "upgrade", toolchain.VitePlus); err != nil {
			return fmt.Errorf("vp alignment incomplete; the installation owner must supply exact VitePlus %s (use --vp for its global CLI): %w", toolchain.VitePlus, err)
		}
	}
	version, err = run(outside, true, vp, "--version")
	if err != nil || !vpVersion(version, toolchain.VitePlus) {
		return fmt.Errorf("vp alignment incomplete: required %s, reported %q (%v)", toolchain.VitePlus, version, err)
	}
	fmt.Fprintf(o.Out, "VitePlus aligned: %s at %s.\n", toolchain.VitePlus, vp)
	if o.Root == "" {
		fmt.Fprintln(o.Out, "Update complete: CLI and VitePlus aligned; no application selected. Rebuild native plugin binaries for the invocation that will load them.")
		return nil
	}
	root, err := projectRoot(o.Root)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return err
	}
	mod, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		return err
	}
	for _, r := range mod.Replace {
		if r.Old.Path == buildinfo.Module {
			return fmt.Errorf("application replaces skgo locally; resolve that override before updating released dependencies")
		}
	}
	hasTool := false
	for _, t := range mod.Tool {
		if t.Path == buildinfo.Module+"/cmd/skgo" {
			hasTool = true
		}
	}
	adapterVersion, addonVersion, err := o.packages(info.SkgoVersion)
	if err != nil {
		return err
	}
	fmt.Fprintf(o.Out, "Selected adapter %s and sv add-on %s for skgo %s.\n", adapterVersion, addonVersion, info.SkgoVersion)
	web := filepath.Join(root, "web")
	stage, err := os.MkdirTemp("", "skgo-update-project-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	original, err := frontendFiles(web)
	if err != nil {
		return err
	}
	if err := copyFrontend(web, stage); err != nil {
		return err
	}
	metadata, _, err := adapter.KitQueueCompatibility()
	if err != nil {
		return err
	}
	if err := pinDependencies(stage, adapterVersion, addonVersion, metadata.Version); err != nil {
		return err
	}
	// The package-manager pin is a consequence of this skgo release's qualified
	// vp environment. Explicit child overrides prevent ambient shell pins winning.
	env := []string{"CI=1", "VP_PACKAGE_MANAGER=pnpm@" + toolchain.PNPM, "VP_PNPM_VERSION=" + toolchain.PNPM}
	stageRun := func(quiet bool, name string, args ...string) ([]byte, error) {
		managed := append([]string{"env", "exec", "--package-manager", "pnpm@" + toolchain.PNPM, name}, args...)
		return o.run(ctx, command{Dir: stage, Name: vp, Args: managed, Env: env, Quiet: quiet})
	}
	if _, err := stageRun(false, vp, "migrate", "--no-interactive", "--no-agent", "--no-editor", "--no-hooks"); err != nil {
		return fmt.Errorf("staged VitePlus migration failed; application source unchanged: %w", err)
	}
	// Migration may normalize dependency ranges. Persist exact release selections
	// again; Vite/Vitest aliases produced by the pinned migrator stay authoritative.
	if err := pinDependencies(stage, adapterVersion, addonVersion, metadata.Version); err != nil {
		return err
	}
	pm, err := stageRun(true, "which", "pnpm")
	if err != nil {
		return fmt.Errorf("resolve vp-managed pnpm: %w", err)
	}
	pnpm := strings.TrimSpace(string(pm))
	if !filepath.IsAbs(pnpm) {
		return fmt.Errorf("vp did not report an absolute pnpm executable: %q", pnpm)
	}
	pmVersion, err := stageRun(true, pnpm, "--version")
	if err != nil || strings.TrimSpace(string(pmVersion)) != toolchain.PNPM {
		return fmt.Errorf("vp environment requires pnpm %s; reported %q (%v)", toolchain.PNPM, pmVersion, err)
	}
	if err := kitpatch.Configure(kitpatch.Options{Web: stage, PNPM: pnpm, Out: o.Out}); err != nil {
		return err
	}
	if _, err := stageRun(false, vp, "install", "--no-frozen-lockfile"); err != nil {
		return err
	}
	if err := kitpatch.Verify(kitpatch.Options{Web: stage, PNPM: pnpm, Out: o.Out}); err != nil {
		return err
	}
	if err := preservedFiles(original, stage); err != nil {
		return err
	}
	if err := unchangedFrontend(web, original); err != nil {
		return err
	}
	if _, err := run(root, false, "go", "get", buildinfo.Module+"@"+info.SkgoVersion); err != nil {
		return err
	}
	if _, err := run(root, false, "go", "mod", "tidy"); err != nil {
		return err
	}
	// Verify the actual tool after dependency resolution, never an assumed PATH
	// binary. A Go cache executable is read/executed only, never overwritten.
	cli := info.Executable
	if hasTool {
		cli, err = projectTool(ctx, o, root, info.SkgoVersion)
		if err != nil {
			return err
		}
	}
	// Go cached executables have hashed names; expose the verified executable as
	// skgo for recipes without writing into the cache or choosing a stale PATH host.
	bin := filepath.Join(outside, "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		return err
	}
	if err := os.Symlink(cli, filepath.Join(bin, "skgo")); err != nil {
		return err
	}
	if err := unchangedFrontend(web, original); err != nil {
		return err
	}

	if err := applyFrontend(web, stage); err != nil {
		return err
	}
	childEnv := append(env, "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	projectRun := func(dir, name string, args ...string) error {
		_, err := o.run(ctx, command{Dir: dir, Name: vp, Args: append([]string{"env", "exec", "--package-manager", "pnpm@" + toolchain.PNPM, name}, args...), Env: childEnv})
		return err
	}
	if err := projectRun(web, vp, "install", "--no-frozen-lockfile"); err != nil {
		return err
	}
	if err := kitpatch.Verify(kitpatch.Options{Web: web, PNPM: pnpm, Out: o.Out}); err != nil {
		return err
	}
	if version, err := adapter.VersionOf(filepath.Join(web, "node_modules", adapter.Package)); err != nil || version != adapterVersion {
		return fmt.Errorf("installed adapter version %q, expected %s (%v)", version, adapterVersion, err)
	}
	fingerprint, err := adapter.FingerprintOf(filepath.Join(web, "node_modules", adapter.Package))
	if err != nil || fingerprint != adapter.Fingerprint() {
		return fmt.Errorf("installed adapter bytes do not match this skgo release: %v", err)
	}
	if err := projectRun(root, "go", "generate", "./..."); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(root, "Justfile")); err == nil {
		if err := projectRun(root, "just", "build"); err != nil {
			return err
		}
	} else {
		if err := projectRun(web, filepath.Join(web, "node_modules/.bin/vp"), "build"); err != nil {
			return err
		}
		if err := projectRun(root, "go", "build", "./..."); err != nil {
			return err
		}
	}
	if err := completeNative(root, o.NativePlatform, o.NativePreset, projectRun); err != nil {
		return err
	}
	fmt.Fprintln(o.Out, "Update complete: CLI, VitePlus, compatible dependencies, generation and application build passed. Application source was preserved; templates were not replayed. Rebuild plugins for the updated standalone or project Go-tool host before using new/add.")
	return nil
}
func vpVersion(b []byte, want string) bool {
	first, _, _ := strings.Cut(strings.TrimSpace(string(b)), "\n")
	return strings.TrimSpace(first) == "vp v"+want
}

func projectTool(ctx context.Context, o Options, root, version string) (string, error) {
	for attempt := 0; attempt < 2; attempt++ {
		data, err := o.run(ctx, command{Dir: root, Name: "go", Args: []string{"tool", "skgo", "buildinfo", "--json"}, Quiet: true})
		if err != nil {
			return "", err
		}
		var tool buildinfo.Info
		if err := json.Unmarshal(data, &tool); err != nil {
			return "", err
		}
		if tool.SkgoVersion != version {
			return "", fmt.Errorf("project Go tool resolved %s, expected %s", tool.SkgoVersion, version)
		}
		if filepath.IsAbs(tool.Executable) {
			if st, err := os.Stat(tool.Executable); err == nil && !st.IsDir() {
				fmt.Fprintf(o.Out, "Project Go tool verified: %s (%s).\n", tool.Executable, tool.SkgoVersion)
				return tool.Executable, nil
			}
		}
	}
	return "", fmt.Errorf("project Go tool did not resolve to a retained executable after compilation")
}
func completeNative(root, platform, preset string, run func(string, string, ...string) error) error {
	_, err := os.Stat(filepath.Join(root, "native/skgo-native.json"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if platform == "" {
		platform = "macos"
	}
	args := []string{"native", "build", "--root", root, "--platform", platform}
	if preset != "" {
		args = append(args, "--preset", preset)
	}
	if err := run(root, "skgo", args...); err != nil {
		return fmt.Errorf("web update completed; native generation/build for %s remains incomplete: %w", platform, err)
	}
	return nil
}
