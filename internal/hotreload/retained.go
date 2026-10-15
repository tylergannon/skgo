package hotreload

import (
	"crypto/sha256"
	"fmt"
	"github.com/tylergannon/polytype/goload"
	"go/parser"
	"go/token"
	"golang.org/x/tools/go/packages"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// retainedApplication keeps whole-application metadata and source-checks every reverse caller.
type retainedApplication struct {
	files           map[string][]byte
	session         goload.Session
	view            *goload.View
	selection       string
	patterns        []string
	dependencyFiles map[string][]byte
	dependencyDirs  map[string]bool
	embeddedFiles   map[string][]byte
	initialized     bool
}

func (q *retainedApplication) Load(c *packages.Config, patterns ...string) ([]*packages.Package, error) {
	files := make(map[string][]byte, len(q.files))
	for p, b := range q.files {
		files[p] = b
	}
	var e error
	for p, b := range c.Overlay {
		files[p] = b
	}
	if q.view == nil {
		q.session.SetMaxUniverses(4)
		q.view = q.session.PrepareView(files)
	} else if e = q.view.Update(files); e != nil {
		return nil, e
	}
	key := retainedSelection(files)
	if key != q.selection {
		q.patterns = nil
		q.selection = key
	}
	meta := *c
	meta.Overlay = nil
	meta.Mode = packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedImports | packages.NeedDeps | packages.NeedModule | packages.NeedEmbedFiles | packages.NeedTypesSizes
	query := q.patterns
	if len(query) == 0 {
		query = patterns
	}
	roots, e := q.view.Load(&meta, query...)
	if e != nil {
		return nil, e
	}
	all := map[string]*packages.Package{}
	packages.Visit(roots, nil, func(p *packages.Package) { all[p.PkgPath] = p })
	target := patterns[0]
	affected := map[string]bool{target: true}
	for changed := true; changed; {
		changed = false
		for path, p := range all {
			if affected[path] {
				continue
			}
			for dep := range p.Imports {
				if affected[dep] {
					affected[path] = true
					changed = true
					break
				}
			}
		}
	}
	selected := map[string]bool{}
	q.patterns = nil
	for _, p := range roots {
		selected[p.PkgPath] = true
		q.patterns = append(q.patterns, p.PkgPath)
	}
	// All application roots remain type checked; all reverse-import callers,
	// including local replacements outside ./..., become explicit source roots.
	for path := range affected {
		selected[path] = true
	}
	var check []string
	for path := range selected {
		check = append(check, path)
	}
	sort.Strings(check)
	if !q.initialized {
		q.dependencyFiles = map[string][]byte{}
		q.dependencyDirs = map[string]bool{}
		q.embeddedFiles = map[string][]byte{}
		for _, p := range all {
			for _, name := range p.EmbedFiles {
				b, ok := files[name]
				if !ok {
					var err error
					b, err = os.ReadFile(name)
					if err != nil {
						return nil, err
					}
				}
				q.embeddedFiles[name] = b
			}
		}
		var visitDeps func(*packages.Package)
		seen := map[string]bool{}
		visitDeps = func(p *packages.Package) {
			if p == nil || seen[p.PkgPath] {
				return
			}
			seen[p.PkgPath] = true
			for _, dep := range p.Imports {
				for _, name := range dep.CompiledGoFiles {
					q.dependencyDirs[filepath.Dir(name)] = true
					if b, ok := files[name]; ok {
						q.dependencyFiles[name] = b
					}
				}
				visitDeps(dep)
			}
		}
		visitDeps(all[target])
		for name, b := range files {
			if q.dependencyDirs[filepath.Dir(name)] {
				q.dependencyFiles[name] = b
			}
		}
		q.initialized = true
	} else {
		for _, p := range all {
			for _, name := range p.EmbedFiles {
				if _, ok := q.embeddedFiles[name]; !ok {
					return nil, &CompleteBuild{Reason: "embedded input membership changed; rebuild host: " + name}
				}
			}
		}
		for name, before := range q.embeddedFiles {
			now, ok := files[name]
			if !ok {
				var err error
				now, err = os.ReadFile(name)
				if err != nil {
					return nil, err
				}
			}
			if string(now) != string(before) {
				return nil, &CompleteBuild{Reason: "embedded dependency input changed; rebuild host: " + name}
			}
		}
		for name := range files {
			if q.dependencyDirs[filepath.Dir(name)] {
				if _, ok := q.dependencyFiles[name]; !ok {
					return nil, &CompleteBuild{Reason: "dependency source membership changed; rebuild host: " + name}
				}
			}
		}
		for name, before := range q.dependencyFiles {
			if now, ok := files[name]; !ok || string(now) != string(before) {
				return nil, &CompleteBuild{Reason: "captured target dependency changed; refresh archives and rebuild host: " + name}
			}
		}
	}
	typed := *c
	typed.Mode &^= packages.NeedDeps
	typed.Mode |= packages.NeedEmbedFiles
	typed.Overlay = nil
	got, e := q.view.Load(&typed, check...)
	if e != nil {
		return nil, e
	}
	coverage := map[string]*packages.Package{}
	packages.Visit(got, nil, func(p *packages.Package) { coverage[p.PkgPath] = p })
	for path := range affected {
		p := coverage[path]
		if p == nil || p.TypesInfo == nil || len(p.Syntax) == 0 {
			return nil, fmt.Errorf("missing complete caller coverage: %s", path)
		}
	}
	return got, nil
}
func retainedSelection(files map[string][]byte) string {
	var names []string
	for p := range files {
		names = append(names, p)
	}
	sort.Strings(names)
	h := sha256.New()
	for _, p := range names {
		fmt.Fprintln(h, p)
		b := files[p]
		if strings.HasSuffix(p, ".go") {
			f, e := parser.ParseFile(token.NewFileSet(), p, b, parser.ImportsOnly|parser.ParseComments)
			if e != nil {
				h.Write(b)
				continue
			}
			fmt.Fprintln(h, f.Name.Name)
			for _, im := range f.Imports {
				fmt.Fprintln(h, im.Path.Value)
			}
			for _, g := range f.Comments {
				for _, c := range g.List {
					if strings.HasPrefix(c.Text, "//go:") {
						fmt.Fprintln(h, c.Text)
					}
				}
			}
		} else {
			h.Write(b)
		}
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
