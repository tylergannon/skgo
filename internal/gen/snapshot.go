package gen

import (
	"bytes"
	"fmt"
	"go/build"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// PackageLoader loads the requested packages from the supplied logical source
// view. Implementations must honor Mode, Overlay, and the remaining load
// configuration. Results must share one go/types universe per call.
type PackageLoader func(*packages.Config, ...string) ([]*packages.Package, error)

// PackageSnapshot retains a caller's immutable go/packages result and its
// source selection. Construct it immediately after loading, before changing
// sources. A snapshot is an invocation-scoped handoff, not a persistent cache.
// Neither the packages nor their ASTs and type information may be mutated.
type PackageSnapshot struct {
	config             packages.Config
	packages           map[string]*packages.Package
	sources            map[string][]byte
	directories        map[string][]string
	modules            map[string][]byte
	trustedDirectories map[string]bool
}

// NewPackageSnapshot captures a package bundle produced by config. The caller
// must supply the exact configuration used to load the bundle, including tags,
// overlays and environment; go/packages does not retain that provenance.
// Imports are included, so an explicitly loaded dependency can serve as a root.
// Load with NeedModule and NeedFiles for the dependency graph (normally
// NeedDeps). Custom parsers, external package drivers, and filesystem overlays
// supplied through build flags are unsupported; use Config.Overlay instead.
//
// Toolchain source and versioned modules in GOMODCACHE are assumed immutable
// for this invocation, as the Go command itself expects. Their source is not
// copied or reread. Workspace, replacement and vendor source remains checked,
// as does any dependency directory touched by Config.Overlay. Do not edit the
// installed toolchain or module cache while using a snapshot.
func NewPackageSnapshot(config *packages.Config, roots ...*packages.Package) (*PackageSnapshot, error) {
	if config == nil {
		return nil, fmt.Errorf("skgo: package snapshot requires its load configuration")
	}
	if config.Mode&packages.NeedModule == 0 {
		return nil, fmt.Errorf("skgo: package snapshots require packages.NeedModule")
	}
	env := effectiveEnv(config.Env)
	value := func(name string) string {
		for _, entry := range env {
			key, v, _ := strings.Cut(entry, "=")
			if key == name {
				return v
			}
		}
		return ""
	}
	flags := append(append([]string(nil), config.BuildFlags...), strings.Fields(value("GOFLAGS"))...)
	for _, flag := range flags {
		if flag == "-overlay" || strings.HasPrefix(flag, "-overlay=") || flag == "-modfile" || strings.HasPrefix(flag, "-modfile=") {
			return nil, fmt.Errorf("skgo: package snapshots do not support %s; use Config.Overlay for source overlays", flag)
		}
	}
	if driver := value("GOPACKAGESDRIVER"); driver != "off" {
		if driver != "" {
			return nil, fmt.Errorf("skgo: package snapshots require the standard Go package driver")
		}
		if _, err := exec.LookPath("gopackagesdriver"); err == nil {
			return nil, fmt.Errorf("skgo: package snapshots require the standard Go package driver")
		}
	}
	if config.ParseFile != nil {
		return nil, fmt.Errorf("skgo: package snapshots require the standard Go parser")
	}
	s := &PackageSnapshot{config: *config, packages: map[string]*packages.Package{}, sources: map[string][]byte{}, directories: map[string][]string{}, modules: map[string][]byte{}, trustedDirectories: map[string]bool{}}
	s.config.Env = effectiveEnv(config.Env)
	s.config.BuildFlags = append([]string(nil), config.BuildFlags...)
	s.config.Dir, _ = filepath.Abs(config.Dir)
	for _, root := range roots {
		if root == nil {
			return nil, fmt.Errorf("skgo: nil package in snapshot")
		}
	}
	goroot := value("GOROOT")
	if goroot == "" {
		goroot = runtime.GOROOT()
	}
	modcache := value("GOMODCACHE")
	if modcache == "" {
		gopath := value("GOPATH")
		if gopath == "" {
			gopath = build.Default.GOPATH
		}
		if paths := filepath.SplitList(gopath); len(paths) > 0 {
			modcache = filepath.Join(paths[0], "pkg", "mod")
		}
	}
	overlaidDirs := overlayDirectories(config.Overlay)
	var problem error
	packages.Visit(roots, nil, func(p *packages.Package) {
		if problem != nil {
			return
		}
		if previous := s.packages[p.PkgPath]; previous != nil && previous != p {
			problem = fmt.Errorf("skgo: duplicate package identity %s", p.PkgPath)
			return
		}
		s.packages[p.PkgPath] = p
		for _, path := range append(append([]string{}, p.GoFiles...), p.CompiledGoFiles...) {
			dir := filepath.Dir(path)
			if !overlaidDirs[dir] && immutablePackageSource(p, path, goroot, modcache) {
				s.trustedDirectories[dir] = true
				continue
			}
			if _, ok := s.sources[path]; ok {
				continue
			}
			data, ok := config.Overlay[path]
			if !ok {
				data, problem = os.ReadFile(path)
				if problem != nil {
					return
				}
			}
			if ok {
				data = bytes.Clone(data)
			}
			s.sources[path] = data
			if _, ok := s.directories[dir]; !ok {
				s.directories[dir], problem = goSourceNames(dir)
				if problem != nil {
					return
				}
			}
		}
		if p.Module != nil {
			for _, mod := range []*packages.Module{p.Module, p.Module.Replace} {
				if mod == nil || mod.GoMod == "" {
					continue
				}
				for _, path := range []string{mod.GoMod, filepath.Join(filepath.Dir(mod.GoMod), "go.sum")} {
					if _, ok := s.modules[path]; ok {
						continue
					}
					data, err := os.ReadFile(path)
					if err != nil && !os.IsNotExist(err) {
						problem = err
						return
					}
					s.modules[path] = data
				}
			}
		}
	})
	if problem != nil {
		return nil, problem
	}
	// Capture excluded files too: editing a build constraint can select a file
	// that was absent from GoFiles without changing the directory listing.
	for dir, names := range s.directories {
		for _, name := range names {
			path := filepath.Join(dir, name)
			if _, ok := s.sources[path]; ok {
				continue
			}
			data, ok := config.Overlay[path]
			if !ok {
				var err error
				data, err = os.ReadFile(path)
				if err != nil {
					return nil, err
				}
			}
			if ok {
				data = bytes.Clone(data)
			}
			s.sources[path] = data
		}
	}
	for dir := s.config.Dir; ; dir = filepath.Dir(dir) {
		for _, name := range []string{"go.mod", "go.sum", "go.work", "go.work.sum"} {
			path := filepath.Join(dir, name)
			data, err := os.ReadFile(path)
			if err != nil && !os.IsNotExist(err) {
				return nil, err
			}
			s.modules[path] = data
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	if workspace := value("GOWORK"); filepath.IsAbs(workspace) {
		for _, path := range []string{workspace, workspace + ".sum"} {
			data, err := os.ReadFile(path)
			if err != nil && !os.IsNotExist(err) {
				return nil, err
			}
			s.modules[path] = data
		}
	}
	return s, nil
}

func overlayDirectories(overlay map[string][]byte) map[string]bool {
	dirs := map[string]bool{}
	for path := range overlay {
		dir := filepath.Dir(path)
		if dirs[dir] {
			continue
		}
		dirs[dir] = true
		if physical, err := filepath.EvalSymlinks(dir); err == nil {
			dirs[physical] = true
		}
	}
	return dirs
}

// Cache trust is deliberately restricted to the known cache roots. A version
// string alone does not make vendor or a local replacement immutable.
func immutablePackageSource(pkg *packages.Package, path, goroot, modcache string) bool {
	if pkg.Module == nil {
		_, inside := withinDir(filepath.Join(goroot, "src"), path)
		return goroot != "" && inside
	}
	mod := pkg.Module
	if mod.Replace != nil {
		mod = mod.Replace
	}
	if mod.Main || mod.Version == "" || modcache == "" {
		return false
	}
	_, inside := withinDir(modcache, path)
	_, owned := withinDir(mod.Dir, path)
	return inside && owned
}

func effectiveEnv(env []string) []string {
	if env == nil {
		env = os.Environ()
	}
	values := map[string]string{}
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		values[key] = entry
	}
	result := make([]string, 0, len(values))
	for _, entry := range values {
		result = append(result, entry)
	}
	sort.Strings(result)
	return result
}

func goSourceNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	names := []string{}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") {
			names = append(names, entry.Name())
		}
	}
	return names, nil
}

func (s *PackageSnapshot) lookup(cfg *packages.Config, patterns []string) ([]*packages.Package, bool) {
	if s == nil || cfg.ParseFile != nil || cfg.Tests != s.config.Tests || !reflect.DeepEqual(cfg.BuildFlags, s.config.BuildFlags) || !reflect.DeepEqual(effectiveEnv(cfg.Env), s.config.Env) {
		return nil, false
	}
	dir, _ := filepath.Abs(cfg.Dir)
	if dir != s.config.Dir {
		return nil, false
	}
	if cfg.Mode & ^s.config.Mode != 0 {
		return nil, false
	}
	roots := make([]*packages.Package, 0, len(patterns))
	for _, pattern := range patterns {
		p := s.packages[pattern]
		if p == nil || p.IllTyped || len(p.Errors) > 0 {
			return nil, false
		}
		if cfg.Mode&packages.NeedTypes != 0 && p.Types == nil || cfg.Mode&packages.NeedTypesInfo != 0 && p.TypesInfo == nil || cfg.Mode&packages.NeedSyntax != 0 && (len(p.Syntax) == 0 || p.Fset == nil) {
			return nil, false
		}
		roots = append(roots, p)
	}
	valid := true
	checked := map[string]bool{}
	overlaidDirs := overlayDirectories(cfg.Overlay)
	packages.Visit(roots, func(p *packages.Package) bool {
		if !valid {
			return false
		}
		if p.PkgPath != "unsafe" && len(p.GoFiles) == 0 {
			valid = false
			return false
		}
		for _, path := range append(append([]string{}, p.GoFiles...), p.CompiledGoFiles...) {
			if dir := filepath.Dir(path); s.trustedDirectories[dir] {
				if overlaidDirs[dir] {
					valid = false
					return false
				}
				continue
			}
			expected, ok := s.sources[path]
			if !ok {
				valid = false
				return false
			}
			actual, overlaid := cfg.Overlay[path]
			if !overlaid {
				var err error
				actual, err = os.ReadFile(path)
				if err != nil {
					valid = false
					return false
				}
			}
			if !bytes.Equal(expected, actual) {
				valid = false
				return false
			}
			dir := filepath.Dir(path)
			if checked[dir] {
				continue
			}
			checked[dir] = true
			names, err := goSourceNames(dir)
			if err != nil || !reflect.DeepEqual(names, s.directories[dir]) {
				valid = false
				return false
			}
			for _, name := range names {
				path := filepath.Join(dir, name)
				expected := s.sources[path]
				actual, ok := cfg.Overlay[path]
				if !ok {
					var err error
					actual, err = os.ReadFile(path)
					if err != nil {
						valid = false
						return false
					}
				}
				if !bytes.Equal(actual, expected) {
					valid = false
					return false
				}
			}
			for overlay := range cfg.Overlay {
				if filepath.Dir(overlay) == dir {
					if _, ok := s.sources[overlay]; !ok {
						valid = false
						return false
					}
				}
			}
		}
		return valid
	}, nil)
	for path, expected := range s.modules {
		actual, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) || !bytes.Equal(actual, expected) {
			valid = false
		}
	}
	if !valid {
		return nil, false
	}
	return roots, true
}

func (cfg Config) loadPackages(load *packages.Config, patterns ...string) ([]*packages.Package, error) {
	if cfg.sourceView != nil {
		load.Env = cfg.sourceView.env
	}
	for _, snapshot := range cfg.Packages {
		if found, ok := snapshot.lookup(load, patterns); ok {
			return found, nil
		}
	}
	if cfg.OnPackageLoad != nil {
		cfg.OnPackageLoad(load, append([]string(nil), patterns...))
	}
	if cfg.LoadPackages != nil {
		return cfg.LoadPackages(load, patterns...)
	}
	cleanup, err := preserveDependencyExports(load)
	if err != nil {
		return nil, err
	}
	if cfg.generation != nil {
		cfg.generation.cleanups = append(cfg.generation.cleanups, cleanup)
	} else {
		defer cleanup()
	}
	loaded, err := packages.Load(load, patterns...)
	if cfg.sourceView != nil {
		packages.Visit(loaded, nil, func(p *packages.Package) {
			if cfg.sourceView.cgoPackages[p.PkgPath] {
				cfg.sourceView.cgoUsed = true
			}
			cfg.sourceView.noteTypes(p.Types)
		})
	}
	return loaded, err
}
