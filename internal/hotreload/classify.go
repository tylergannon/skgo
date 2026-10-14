package hotreload

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"go/types"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// CompleteBuild is a supported edit that cannot be installed into this host.
// Compiler/setup failures remain errors; callers must not turn them into this
// outcome to disguise missing native integration.
type CompleteBuild struct{ Reason string }

func (e *CompleteBuild) Error() string { return "complete build required: " + e.Reason }

// Classify keeps source-shape and caller compatibility checks for the serial
// generated-host foundation. It does not establish candidate object lifetime,
// safe publication, or concurrent mutation; those remain separate work.
// roots are replaceable packages; application is the loaded package set.
func Classify(before, after map[string][]byte, roots, application []*packages.Package) error {
	oldShape, err := declarations(before)
	if err != nil {
		return err
	}
	newShape, err := declarations(after)
	if err != nil {
		return err
	}
	if !bytes.Equal(oldShape, newShape) {
		return &CompleteBuild{"declaration, initialization, import or build-selection changed"}
	}
	boundary := map[string]bool{}
	for _, p := range roots {
		if len(p.Errors) != 0 {
			return fmt.Errorf("type-check replacement %s: %v", p.PkgPath, p.Errors)
		}
		if p.TypesInfo == nil || len(p.Syntax) == 0 {
			return fmt.Errorf("missing captured executable analysis: %s", p.PkgPath)
		}
		boundary[p.PkgPath] = true
	}
	// Recompile any package in the reverse import closure and reject callers
	// outside the replaceable boundary that still reference replaced functions.
	affected := map[string]bool{}
	for path := range boundary {
		affected[path] = true
	}
	for changed := true; changed; {
		changed = false
		for _, p := range application {
			if affected[p.PkgPath] {
				continue
			}
			for path := range p.Imports {
				if affected[path] {
					affected[p.PkgPath] = true
					changed = true
					break
				}
			}
		}
	}
	for _, p := range application {
		if !affected[p.PkgPath] || boundary[p.PkgPath] {
			continue
		}
		if p.TypesInfo == nil {
			return fmt.Errorf("missing caller analysis: %s", p.PkgPath)
		}
		for id, obj := range p.TypesInfo.Uses {
			fn, ok := obj.(*types.Func)
			if !ok || fn.Pkg() == nil || !boundary[fn.Pkg().Path()] {
				continue
			}
			return &CompleteBuild{fmt.Sprintf("uninstrumented caller of %s at %s", fn.FullName(), p.Fset.Position(id.Pos()))}
		}
	}
	return nil
}

func declarations(sources map[string][]byte) ([]byte, error) {
	var paths []string
	for path := range sources {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	var result bytes.Buffer
	for _, path := range paths {
		fs := token.NewFileSet()
		f, err := parser.ParseFile(fs, path, sources[path], parser.ParseComments)
		if err != nil {
			return nil, err
		}
		// Comments affecting file/compiler selection remain part of identity.
		for _, group := range f.Comments {
			for _, c := range group.List {
				if strings.HasPrefix(c.Text, "//go:") {
					result.WriteString(c.Text)
					result.WriteByte('\n')
				}
			}
		}
		f.Comments = nil
		for _, decl := range f.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name != "init" {
				fn.Body = nil
				fn.Doc = nil
			}
			if gen, ok := decl.(*ast.GenDecl); ok {
				gen.Doc = nil
			}
		}
		result.WriteString(path)
		result.WriteByte('\n')
		if err := format.Node(&result, token.NewFileSet(), f); err != nil {
			return nil, err
		}
	}
	return result.Bytes(), nil
}
