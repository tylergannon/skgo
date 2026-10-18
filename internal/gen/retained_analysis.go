// Copyright 2022 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license in GOPLS_LICENSE.
//
// Attributed semantic ports from x/tools v0.52.0 (16517d9), gopls/internal/cache:
// snapshot.clone/metadataChanges, metadata.Graph.Update and reverse traversal,
// parseCache.startParse, and check.evaluatePackageHandle. SKGO's consumed key
// includes constants, tags, comments and markers, not only type references.
package gen

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"maps"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// sourceAnalysis owns its file positions. Unchanged handles, parse results and
// package summaries are shared across snapshot clones, never mutated in place.
type sourceAnalysis struct {
	hash    [32]byte
	key     [32]byte
	fset    *token.FileSet
	file    *ast.File
	imports []string
	err     error
}
type sourcePackage struct {
	dir, path  string
	files      []string
	imports    []string
	local, key [32]byte
}
type sourceGraph struct {
	packages   map[string]*sourcePackage
	importedBy map[string][]string
}

func (p *Project) cloneSnapshot(before *projectSnapshot, files map[string][]byte, metadata *kitRouteMetadata) *projectSnapshot {
	s := &projectSnapshot{revision: 1, environmentHash: p.sources.EnvironmentHash, files: files, metadata: metadata, parsed: map[string]*sourceAnalysis{}}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	if before != nil {
		s.revision = before.revision + 1
	}
	for path, data := range files {
		if !strings.HasSuffix(path, ".go") || data == nil {
			continue
		}
		hash := sha256.Sum256(data)
		if before != nil {
			if old := before.parsed[path]; old != nil && old.hash == hash {
				s.parsed[path] = old
				continue
			}
		}
		p.stats.FileParses++
		a := &sourceAnalysis{hash: hash, fset: token.NewFileSet()}
		a.file, a.err = parser.ParseFile(a.fset, path, data, parser.ParseComments|parser.AllErrors)
		if a.err != nil {
			a.key = hash
			s.parsed[path] = a
			continue
		}
		for _, imp := range a.file.Imports {
			if name, err := strconv.Unquote(imp.Path.Value); err == nil {
				a.imports = append(a.imports, name)
			}
		}
		// Keep the immutable full AST for positions. Print a separate shallow clone
		// with declaration bodies purged; no cached tree is mutated for projection.
		projection := *a.file
		projection.Decls = append([]ast.Decl(nil), a.file.Decls...)
		var removed [][2]token.Pos
		for i, decl := range projection.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
				copy := *fn
				removed = append(removed, [2]token.Pos{fn.Body.Pos(), fn.Body.End()})
				copy.Body = nil
				projection.Decls[i] = &copy
			}
		}
		projection.Comments = nil
		for _, group := range a.file.Comments {
			keep := true
			for _, span := range removed {
				if group.Pos() > span[0] && group.End() < span[1] {
					keep = false
					break
				}
			}
			if keep {
				projection.Comments = append(projection.Comments, group)
			}
		}
		var b bytes.Buffer
		// Positions are omitted from the key: a body growing above a declaration
		// cannot change a downstream producer merely by moving its source line.
		_ = printer.Fprint(&b, token.NewFileSet(), &projection)
		a.key = sha256.Sum256(b.Bytes())
		s.parsed[path] = a
	}
	s.graph = p.updateGraph(before, s)
	h := sha256.New()
	enc := json.NewEncoder(h)
	_ = enc.Encode(metadata)
	_ = enc.Encode(p.sources.EnvironmentHash)
	// Root producers consume package summaries; imports are included transitively.
	roots := map[string]bool{}
	for path := range s.parsed {
		name := filepath.Base(path)
		if withinTree(filepath.Join(p.cfg.Web, "src"), path) && (strings.HasSuffix(name, ".remote.go") || loadFileNames[name] != "" || name == serverFileName || name == hooksFileName || name == "params.go" || name == "env.go") {
			roots[filepath.Dir(path)] = true
		}
	}
	for _, pkg := range s.graph.packages {
		if pkg.path == p.cfg.LocalsPackage || pkg.path == p.cfg.HookPackage || pkg.path == p.cfg.ConfigPackage {
			roots[pkg.dir] = true
		}
	}
	dirs := make([]string, 0, len(roots))
	for dir := range roots {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	for _, dir := range dirs {
		_ = enc.Encode(dir)
		if pkg := s.graph.packages[dir]; pkg != nil {
			_ = enc.Encode(pkg.key)
		}
	}
	var controls []string
	for path := range files {
		if strings.HasSuffix(path, ".go") {
			continue
		}
		// Embeds affect compiler inputs, not declaration projection. Frontend
		// metadata, module/workspace selection and package pins are consumed.
		if withinTree(filepath.Join(p.cfg.Web, "src"), path) || filepath.Base(path) == "go.mod" || filepath.Base(path) == "go.sum" || filepath.Ext(path) == ".work" || filepath.Base(path) == "go.work.sum" || filepath.Dir(path) == p.cfg.Web || strings.Contains(path, "node_modules") {
			controls = append(controls, path)
		}
	}
	sort.Strings(controls)
	for _, path := range controls {
		_ = enc.Encode(path)
		_ = enc.Encode(files[path])
	}
	copy(s.key[:], h.Sum(nil))
	if before != nil && before.key == s.key {
		s.cancel()
		s.ctx, s.cancel = before.ctx, before.cancel
	}
	return s
}

func (p *Project) updateGraph(before, s *projectSnapshot) *sourceGraph {
	g := &sourceGraph{packages: map[string]*sourcePackage{}, importedBy: map[string][]string{}}
	files := map[string][]string{}
	for path := range s.parsed {
		if !strings.HasSuffix(path, "_test.go") {
			files[filepath.Dir(path)] = append(files[filepath.Dir(path)], path)
		}
	}
	byPath := map[string]string{}
	for dir, names := range files {
		sort.Strings(names)
		pkg := &sourcePackage{dir: dir, files: names}
		longest := 0
		for _, module := range p.sources.Modules {
			if rel, ok := withinDir(module.Dir, dir); ok && len(module.Dir) > longest {
				longest = len(module.Dir)
				pkg.path = module.Path
				if rel != "." {
					pkg.path += "/" + filepath.ToSlash(rel)
				}
			}
		}
		h := sha256.New()
		enc := json.NewEncoder(h)
		imports := map[string]bool{}
		for _, name := range names {
			a := s.parsed[name]
			_ = enc.Encode(name)
			_ = enc.Encode(a.key)
			for _, imp := range a.imports {
				imports[imp] = true
			}
		}
		for name := range imports {
			pkg.imports = append(pkg.imports, name)
		}
		sort.Strings(pkg.imports)
		copy(pkg.local[:], h.Sum(nil))
		g.packages[dir] = pkg
		byPath[pkg.path] = dir
	}
	// Graph.Update's ImportedBy index replaces repeated whole-graph fixed points.
	for dir, pkg := range g.packages {
		for _, imp := range pkg.imports {
			if dependency := byPath[imp]; dependency != "" {
				g.importedBy[dependency] = append(g.importedBy[dependency], dir)
			}
		}
	}
	changed := map[string]bool{}
	if before != nil {
		for dir, old := range before.graph.packages {
			current := g.packages[dir]
			if current == nil || old.local != current.local || old.path != current.path {
				changed[dir] = true
			}
		}
	}
	for dir, pkg := range g.packages {
		if before == nil || before.graph.packages[dir] == nil {
			changed[dir] = true
		}
		_ = pkg
	}
	// Invalidate reverse closures in both graphs so deleted edges also propagate.
	var invalidate func(string)
	invalidate = func(dir string) {
		for _, parent := range g.importedBy[dir] {
			if !changed[parent] {
				changed[parent] = true
				invalidate(parent)
			}
		}
		if before != nil {
			for _, parent := range before.graph.importedBy[dir] {
				if !changed[parent] {
					changed[parent] = true
					invalidate(parent)
				}
			}
		}
	}
	for dir := range maps.Clone(changed) {
		invalidate(dir)
	}
	visiting := map[string]bool{}
	var evaluate func(string) [32]byte
	evaluate = func(dir string) [32]byte {
		pkg := g.packages[dir]
		if pkg == nil {
			return [32]byte{}
		}
		if before != nil && !changed[dir] {
			if old := before.graph.packages[dir]; old != nil {
				g.packages[dir] = old
				return old.key
			}
		}
		if pkg.key != ([32]byte{}) {
			return pkg.key
		}
		if visiting[dir] {
			return pkg.local
		}
		visiting[dir] = true
		h := sha256.New()
		h.Write(pkg.local[:])
		for _, imp := range pkg.imports {
			h.Write([]byte(imp))
			key := evaluate(byPath[imp])
			h.Write(key[:])
		}
		copy(pkg.key[:], h.Sum(nil))
		delete(visiting, dir)
		return pkg.key
	}
	for dir := range g.packages {
		evaluate(dir)
	}
	return g
}
