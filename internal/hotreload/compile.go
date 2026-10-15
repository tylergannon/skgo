package hotreload

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"golang.org/x/tools/go/packages"
)

// Candidate is a compiled captured revision and its required host storage.
// Both are consumed by the generated development host's SkgoHotInstall.
type Candidate struct {
	Object  string
	Storage []string
	Reclaim bool
}

// Compile uses the host's toolchain and dependency archives. before and after
// contain captured authored sources, not the transformed development host.
// This package-only entry is for an already checked executable boundary. Use
// CompileApplication when admitting an application revision.
func (t Toolchain) Compile(ctx context.Context, dir, pkgPath, importcfg, output string, before, after map[string][]byte) (Candidate, error) {
	return t.compile(ctx, dir, pkgPath, importcfg, output, before, after, nil)
}

// CompileApplication checks executable callers in the selected application
// package patterns before producing an object. Use the same captured overlay
// for this analysis and compilation. Uninstrumented callers select rebuilding.
func (t Toolchain) CompileApplication(ctx context.Context, dir, pkgPath, importcfg, output string, before, after map[string][]byte, application ...string) (Candidate, error) {
	if len(application) == 0 {
		application = []string{"./..."}
	}
	return t.compile(ctx, dir, pkgPath, importcfg, output, before, after, application)
}

func (t Toolchain) compile(ctx context.Context, dir, pkgPath, importcfg, output string, before, after map[string][]byte, application []string) (Candidate, error) {
	var candidate Candidate
	if err := t.checkFlags(); err != nil {
		return candidate, err
	}
	if err := Classify(before, after, nil, nil); err != nil {
		return candidate, err
	}
	patterns := append([]string{pkgPath}, application...)
	pkgs, err := packages.Load(&packages.Config{
		Context: ctx, Dir: dir, Env: t.command(ctx, dir).Env, Overlay: after,
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedImports | packages.NeedDeps | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedModule,
	}, patterns...)
	if err != nil {
		return candidate, err
	}
	// Local replacements may live outside ./... but still contain executable
	// callers (or inlined copies). Inspect the complete loaded import graph.
	var applicationPackages []*packages.Package
	packages.Visit(pkgs, nil, func(loaded *packages.Package) { applicationPackages = append(applicationPackages, loaded) })
	var p *packages.Package
	for _, loaded := range pkgs {
		if len(loaded.Errors) != 0 {
			return candidate, fmt.Errorf("type-check application %s: %v", loaded.PkgPath, loaded.Errors)
		}
		if loaded.PkgPath == pkgPath {
			p = loaded
		}
	}
	if p == nil {
		return candidate, fmt.Errorf("replacement package %s not loaded", pkgPath)
	}
	if err := Classify(before, after, []*packages.Package{p}, applicationPackages); err != nil {
		return candidate, err
	}
	lowered, err := LowerAuthored(p, after, t.AuthoredPaths)
	if err != nil {
		return candidate, err
	}
	if err := os.MkdirAll(output, 0755); err != nil {
		return candidate, err
	}
	var names []string
	for name := range lowered.Candidate {
		names = append(names, name)
	}
	sort.Strings(names)
	var files []string
	for _, name := range names {
		file := filepath.Join(output, name)
		if err := os.WriteFile(file, lowered.Candidate[name], 0644); err != nil {
			return candidate, err
		}
		files = append(files, file)
	}
	candidate.Object = filepath.Join(output, "candidate.o")
	candidate.Storage = lowered.Storage
	candidate.Reclaim = numericRevision(p, after)
	args := append([]string{"tool", "compile", "-p", pkgPath, "-lang=go1.27", "-importcfg", importcfg, "-o", candidate.Object}, files...)
	raw, err := t.command(ctx, dir, args...).CombinedOutput()
	if err != nil {
		return Candidate{}, fmt.Errorf("compile captured replacement: %w\n%s", err, raw)
	}
	if err := t.writeArtifact(candidate.Object, pkgPath, importcfg); err != nil {
		return Candidate{}, err
	}
	return candidate, nil
}
