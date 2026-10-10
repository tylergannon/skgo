// Package pluginbuild builds external source for the actual invoking host, in an
// isolated module. It never changes the consumer's or plugin author's go.mod.
package pluginbuild

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/tylergannon/skgo/internal/buildinfo"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/semver"
)

type Options struct {
	Host, Project, Source, Package, Output, SkgoSource, VersionSymbol string
	Log                                                               io.Writer
}

func Build(ctx context.Context, o Options) error {
	if (o.Host == "") == (o.Project == "") || o.Source == "" || o.Output == "" {
		return fmt.Errorf("choose exactly one host executable or project directory, plus source and output .so")
	}
	var host buildinfo.Info
	// The first uncached go-tool run may execute a temporary binary. A second
	// invocation resolves the same normal tool build from Go's persistent cache.
	for attempt := 0; attempt < 2; attempt++ {
		cmd := exec.CommandContext(ctx, o.Host, "buildinfo", "--json")
		if o.Project != "" {
			cmd = exec.CommandContext(ctx, "go", "tool", "skgo", "buildinfo", "--json")
			cmd.Dir = o.Project
		}
		data, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("read host buildinfo: %w", err)
		}
		if err := json.Unmarshal(data, &host); err != nil {
			return err
		}
		if host.Build == nil || host.Build.GoVersion == "" || host.SkgoVersion == "" {
			return fmt.Errorf("host buildinfo is incomplete")
		}
		if _, err := os.Stat(host.Executable); err == nil {
			break
		} else if o.Project == "" || attempt == 1 {
			return fmt.Errorf("host executable is unavailable after invocation: %w", err)
		}
	}
	settings := map[string]string{}
	for _, s := range host.Build.Settings {
		settings[s.Key] = s.Value
	}
	if settings["CGO_ENABLED"] != "1" || (settings["GOOS"] != "darwin" && settings["GOOS"] != "linux") {
		return fmt.Errorf("host must be cgo-enabled on macOS or Linux")
	}
	source, err := filepath.Abs(o.Source)
	if err != nil {
		return err
	}
	output, err := filepath.Abs(o.Output)
	if err != nil {
		return err
	}
	if filepath.Ext(output) != ".so" {
		return fmt.Errorf("plugin output must end in .so")
	}
	stage, err := os.MkdirTemp("", "skgo-plugin-build-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err := copySource(source, stage); err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(stage, "go.mod"))
	if err != nil {
		return fmt.Errorf("plugin source needs its own go.mod: %w", err)
	}
	mod, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		return err
	}
	if mod.Module == nil || mod.Module.Mod.Path == "" {
		return fmt.Errorf("plugin go.mod must declare a module path")
	}
	var pinned []debug.Module
	all := append([]*debug.Module{&host.Build.Main}, host.Build.Deps...)
	for _, m := range all {
		if m.Path == "" || m.Path == mod.Module.Mod.Path {
			continue
		}
		pin := *m
		if pin.Replace != nil {
			replacement := *pin.Replace
			if replacement.Version == "(devel)" {
				replacement.Version = ""
			}
			pin.Replace = &replacement
		}
		if pin.Path == buildinfo.Module && o.SkgoSource != "" {
			dir, err := filepath.Abs(o.SkgoSource)
			if err != nil {
				return err
			}
			pin.Replace = &debug.Module{Path: dir}
		}
		version := pin.Version
		if !semver.IsValid(version) || strings.Contains(version, "+dirty") {
			if pin.Replace == nil {
				return fmt.Errorf("host module %s has unreproducible version %q; for a skgo checkout supply --skgo-source", pin.Path, version)
			}
			version = "v0.0.0"
		}
		if err := mod.AddRequire(pin.Path, version); err != nil {
			return err
		}
		for _, r := range mod.Replace {
			if r.Old.Path == pin.Path {
				if err := mod.DropReplace(r.Old.Path, r.Old.Version); err != nil {
					return err
				}
			}
		}
		if pin.Replace != nil {
			r := pin.Replace
			if r.Version == "" && !filepath.IsAbs(r.Path) {
				return fmt.Errorf("host replacement %s => %s is relative; cannot reproduce its source from buildinfo", pin.Path, r.Path)
			}
			if err := mod.AddReplace(pin.Path, "", r.Path, r.Version); err != nil {
				return err
			}
		}
		pin.Version = version
		pinned = append(pinned, pin)
	}
	// Unrelated relative replacements would change meaning in the isolated copy.
	for _, r := range mod.Replace {
		if r.Old.Path != "" && r.New.Version == "" && !filepath.IsAbs(r.New.Path) {
			return fmt.Errorf("plugin replacement %s => %s is relative; stage explicit reproducible sources before building", r.Old.Path, r.New.Path)
		}
	}
	mod.Cleanup()
	data, err = mod.Format()
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(stage, "go.mod"), data, 0644); err != nil {
		return err
	}
	env := append(os.Environ(), "GOWORK=off", "GOFLAGS=", "GOTOOLCHAIN="+host.Build.GoVersion)
	for _, key := range []string{"GOOS", "GOARCH", "GOAMD64", "GOARM", "GOARM64", "GO386", "GOMIPS", "GOMIPS64", "GOPPC64", "GORISCV64", "GOEXPERIMENT", "CGO_ENABLED", "CGO_CFLAGS", "CGO_CPPFLAGS", "CGO_CXXFLAGS", "CGO_LDFLAGS"} {
		if v, ok := settings[key]; ok {
			env = append(env, key+"="+v)
		}
	}
	run := func(args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, "go", args...)
		cmd.Dir = stage
		cmd.Env = env
		cmd.Stderr = o.Log
		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("go %s with host toolchain %s: %w", strings.Join(args, " "), host.Build.GoVersion, err)
		}
		return out, nil
	}
	if _, err := run("mod", "tidy"); err != nil {
		return err
	}
	graph, err := run("list", "-m", "-json", "all")
	if err != nil {
		return err
	}
	resolved := map[string]debug.Module{}
	dec := json.NewDecoder(strings.NewReader(string(graph)))
	for dec.More() {
		var m debug.Module
		if err := dec.Decode(&m); err != nil {
			return err
		}
		resolved[m.Path] = m
	}
	for _, pin := range pinned {
		m, ok := resolved[pin.Path]
		if !ok {
			continue
		}
		if m.Version != pin.Version || !sameReplacement(m.Replace, pin.Replace) {
			return fmt.Errorf("plugin dependency %s resolves to %s instead of host %s (or has different source); align requirements before building", pin.Path, m.Version, pin.Version)
		}
	}
	args := []string{"build", "-buildmode=plugin", "-buildvcs=false", "-o", filepath.Join(stage, "plugin.so")}
	for _, key := range []string{"-compiler", "-gcflags", "-asmflags", "-tags", "-trimpath", "-race", "-msan", "-asan", "-pgo"} {
		if v, ok := settings[key]; ok {
			if key == "-pgo" && v != "off" {
				return fmt.Errorf("host PGO source cannot be reproduced automatically")
			}
			args = append(args, key+"="+v)
		}
	}
	var ldflags []string
	if settings["GOOS"] == "darwin" {
		// Avoid malformed chained-fixup metadata in Go plugin Mach-O output.
		// This affects linking only, not shared-package compilation identities.
		ldflags = append(ldflags, "-extldflags=-Wl,-no_fixup_chains")
	}
	if o.VersionSymbol != "" {
		if strings.ContainsAny(o.VersionSymbol, " \t\n'") {
			return fmt.Errorf("invalid version symbol")
		}
		ldflags = append(ldflags, "-X", o.VersionSymbol+"="+host.SkgoVersion)
	}
	if len(ldflags) > 0 {
		args = append(args, "-ldflags="+strings.Join(ldflags, " "))
	}
	pkg := o.Package
	if pkg == "" {
		pkg = "."
	}
	if pkg != "." && (!strings.HasPrefix(pkg, "./") || !filepath.IsLocal(strings.TrimPrefix(pkg, "./"))) {
		return fmt.Errorf("package must be . or a relative package within the source module")
	}
	args = append(args, pkg)
	if _, err := run(args...); err != nil {
		return err
	}
	// Qualify the exact host before installation. It must discover at least one
	// declared export and must not report a rejected candidate.
	check := exec.CommandContext(ctx, host.Executable, "add", "--help")
	check.Env = append(os.Environ(), "SKGO_PLUGIN_DIRS="+stage)
	checkOut, err := check.CombinedOutput()
	if err != nil || strings.Contains(string(checkOut), "warning:") {
		return fmt.Errorf("built plugin did not load into %s: %v\n%s", host.Executable, err, checkOut)
	}
	data, err = os.ReadFile(filepath.Join(stage, "plugin.so"))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(output), ".skgo-plugin-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(f.Name(), 0755); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), output); err != nil {
		return err
	}
	if o.Log != nil {
		fmt.Fprintf(o.Log, "built %s for %s (skgo %s, %s)\n", output, host.Executable, host.SkgoVersion, host.Build.GoVersion)
	}
	return nil
}
func sameReplacement(a, b *debug.Module) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Path == b.Path && a.Version == b.Version
}
func copySource(source, dest string) error {
	return filepath.WalkDir(source, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "ephemeral", ".local", ".cache", "build", ".build":
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dest, rel), 0755)
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("source symlink %s cannot be reproduced; stage its source explicitly", path)
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("non-regular plugin source %s", path)
		}
		if filepath.Ext(path) == ".so" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dest, rel), data, info.Mode().Perm())
	})
}
