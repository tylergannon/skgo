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
}

// Compile uses the host's toolchain and dependency archives. before and after
// contain captured authored sources, not the transformed development host.
// This initial boundary replaces one complete generated route package.
func (t Toolchain) Compile(ctx context.Context, dir, pkgPath, importcfg, output string, before, after map[string][]byte) (Candidate, error) {
	var candidate Candidate
	if err := Classify(before, after, nil, nil); err != nil {
		return candidate, err
	}
	pkgs, err := packages.Load(&packages.Config{
		Context: ctx, Dir: dir, Env: t.command(ctx, dir).Env, Overlay: after,
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedImports | packages.NeedDeps | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedModule,
	}, pkgPath)
	if err != nil {
		return candidate, err
	}
	if len(pkgs) != 1 {
		return candidate, fmt.Errorf("expected one replacement package, got %d", len(pkgs))
	}
	p := pkgs[0]
	if err := Classify(before, after, []*packages.Package{p}, []*packages.Package{p}); err != nil {
		return candidate, err
	}
	lowered, err := Lower(p, after)
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
	args := append([]string{"tool", "compile", "-p", pkgPath, "-lang=go1.27", "-importcfg", importcfg, "-o", candidate.Object}, files...)
	raw, err := t.command(ctx, dir, args...).CombinedOutput()
	if err != nil {
		return Candidate{}, fmt.Errorf("compile captured replacement: %w\n%s", err, raw)
	}
	return candidate, nil
}
