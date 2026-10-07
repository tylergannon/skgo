package gen

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/scanner"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const generatedGoFile = "skgo_gen.go"

// generation holds only this invocation's output. No package loader reads the
// previous generated declarations, and no project output is a bootstrap file.
type generatedOutput struct {
	files    map[string]string
	parts    map[string][]string
	owned    map[string]bool
	removed  map[string]bool
	links    *routeLinks
	cleanups []func()
}

func newGeneration(cfg Config) (*generatedOutput, error) {
	host, mod, err := moduleOf(cfg.Out)
	if err != nil {
		return nil, err
	}
	links, err := newRouteLinks(cfg, host, mod)
	if err != nil {
		return nil, err
	}
	g := &generatedOutput{files: map[string]string{}, parts: map[string][]string{}, owned: map[string]bool{}, removed: map[string]bool{}, links: links}
	roots := []string{filepath.Join(cfg.Web, "src"), cfg.Out}
	localsDir := ""
	if cfg.LocalsPackage == mod {
		localsDir = host
	} else if strings.HasPrefix(cfg.LocalsPackage, mod+"/") {
		localsDir = filepath.Join(host, filepath.FromSlash(strings.TrimPrefix(cfg.LocalsPackage, mod+"/")))
	}
	if localsDir != "" {
		// Only this package's helper belongs to the locals selection. Child
		// packages may belong to another application in the same module.
		path := filepath.Join(localsDir, generatedGoFile)
		content, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		if bytes.HasPrefix(content, []byte(goHeader)) {
			g.owned[path] = true
		}
	}

	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if os.IsNotExist(err) {
				return nil
			}
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if entry.Name() == "node_modules" || strings.HasPrefix(entry.Name(), ".") {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.Name() != generatedGoFile {
				return nil
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if bytes.HasPrefix(content, []byte(goHeader)) {
				g.owned[path] = true
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return g, nil
}

func (g *generatedOutput) close() {
	for i := len(g.cleanups) - 1; i >= 0; i-- {
		g.cleanups[i]()
	}
}

func (g *generatedOutput) add(path, content string) error {
	if filepath.Ext(path) == ".go" {
		if filepath.Base(path) != generatedGoFile {
			return fmt.Errorf("skgo: unexpected generated Go filename %s", path)
		}
		if info, err := os.Lstat(path); err == nil {
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			if !info.Mode().IsRegular() || !bytes.HasPrefix(data, []byte(goHeader)) {
				return fmt.Errorf("skgo: %s is authored; refusing to overwrite it", path)
			}
			g.owned[path] = true
		} else if !os.IsNotExist(err) {
			return err
		}
		parts := append(g.parts[path], content)
		merged, err := mergeGo(parts)
		if err != nil {
			return fmt.Errorf("skgo: combining %s: %w", path, err)
		}
		g.parts[path] = parts
		content = merged
	}
	g.files[path] = content
	return nil
}

// mergeGo combines independently emitted declarations, resolving file-local
// import aliases before removing file boundaries. Authored code never enters it.
func mergeGo(parts []string) (string, error) {
	fset := token.NewFileSet()
	files := make([]*ast.File, len(parts))
	used := map[string]bool{}
	for i, source := range parts {
		f, err := parser.ParseFile(fset, "", source, parser.ParseComments)
		if err != nil {
			return "", err
		}
		files[i] = f
		if i > 0 && f.Name.Name != files[0].Name.Name {
			return "", fmt.Errorf("conflicting package names")
		}
		for name := range f.Scope.Objects {
			used[name] = true
		}
	}
	aliases := map[string]string{}
	var bodies []string
	for i, f := range files {
		type edit struct {
			start, end int
			text       string
		}
		var edits []edit
		file := fset.File(f.Pos())
		offset := func(p token.Pos) int { return file.Offset(p) }
		renames := map[string]string{}
		for _, imp := range f.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				return "", err
			}
			old := filepath.Base(path)
			if imp.Name != nil {
				old = imp.Name.Name
			}
			if old == "." {
				return "", fmt.Errorf("generated dot import %s", path)
			}
			key := path
			if old == "_" {
				key = "_" + path
			}
			alias, ok := aliases[key]
			if !ok {
				alias = old
				for n := 2; alias != "_" && used[alias]; n++ {
					alias = fmt.Sprintf("%s%d", old, n)
				}
				aliases[key] = alias
				used[alias] = true
			}
			if old != alias {
				renames[old] = alias
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			id, ok := sel.X.(*ast.Ident)
			if ok && id.Obj == nil {
				if name, ok := renames[id.Name]; ok {
					edits = append(edits, edit{offset(id.Pos()), offset(id.End()), name})
				}
			}
			return true
		})
		for _, decl := range f.Decls {
			if d, ok := decl.(*ast.GenDecl); ok && d.Tok == token.IMPORT {
				edits = append(edits, edit{offset(d.Pos()), offset(d.End()), ""})
			}
		}
		sort.Slice(edits, func(i, j int) bool { return edits[i].start > edits[j].start })
		source := parts[i]
		for _, e := range edits {
			source = source[:e.start] + e.text + source[e.end:]
		}
		bodies = append(bodies, source[offset(f.Name.End()):])
	}
	var b strings.Builder
	b.WriteString(goHeader + "package " + files[0].Name.Name + "\n")
	if len(aliases) > 0 {
		b.WriteString("import (\n")
		paths := make([]string, 0, len(aliases))
		for path := range aliases {
			paths = append(paths, path)
		}
		sort.Strings(paths)
		for _, path := range paths {
			name := aliases[path]
			if name == "_" {
				path = strings.TrimPrefix(path, "_")
			}
			fmt.Fprintf(&b, "%s %q\n", name, path)
		}
		b.WriteString(")\n")
	}
	for _, body := range bodies {
		b.WriteString(body)
		b.WriteByte('\n')
	}
	formatted, err := format.Source([]byte(b.String()))
	return string(formatted), err
}

// overlay includes new packages and refreshed copies of every authored route
// file, so roots and dependencies have the same view even on first generation.
func (g *generatedOutput) overlay() (map[string][]byte, error) {
	overlay := map[string][]byte{}
	for path := range g.owned {
		pkg, err := packageNameOf(filepath.Dir(path))
		if err != nil {
			pkg = "generated"
		}
		overlay[path] = []byte("package " + pkg + "\n")
	}
	for path, source := range g.files {
		if filepath.Ext(path) == ".go" {
			overlay[path] = []byte(source)
		}
	}
	for _, link := range g.links.links {
		names, err := goFileNames(link.dir)
		if err != nil {
			return nil, err
		}
		keep := map[string]bool{}
		for _, name := range names {
			source := filepath.Join(link.dir, name)
			data, ok := overlay[source]
			if !ok {
				data, err = os.ReadFile(source)
				if err != nil {
					return nil, err
				}
			}
			overlay[filepath.Join(link.linkDir, name)] = data
			keep[name] = true
		}
		source := filepath.Join(link.dir, generatedGoFile)
		if data, ok := overlay[source]; ok {
			overlay[filepath.Join(link.linkDir, generatedGoFile)] = data
			keep[generatedGoFile] = true
		}
		old, _ := goFileNames(link.linkDir)
		pkg, err := packageNameOf(link.dir)
		if err != nil {
			return nil, err
		}
		for _, name := range old {
			if !keep[name] {
				overlay[filepath.Join(link.linkDir, name)] = []byte("package " + pkg + "\n")
			}
		}
	}
	return overlay, nil
}

func (g *generatedOutput) publish(cfg Config) error {
	// Preflight every generated Go destination before touching any output.
	for path := range g.parts {
		if data, err := os.ReadFile(path); err == nil {
			info, lerr := os.Lstat(path)
			if lerr != nil {
				return lerr
			}
			if !info.Mode().IsRegular() || !bytes.HasPrefix(data, []byte(goHeader)) {
				return fmt.Errorf("skgo: %s is authored; refusing to overwrite it", path)
			}
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	paths := make([]string, 0, len(g.files))
	for path := range g.files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if err := writeContent(cfg, path, g.files[path]); err != nil {
			return err
		}
	}
	for path := range g.owned {
		if _, ok := g.files[path]; !ok {
			g.removed[path] = true
		}
	}
	for path := range g.removed {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return g.links.sync()
}

func finishGeneration(cfg Config) error {
	g := cfg.generation
	// Format temporary copies in one invocation; formatter failure cannot leave
	// half-written generated project files.
	if len(cfg.frontendFiles) > 0 {
		dir, err := os.MkdirTemp("", "skgo-format-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(dir)
		if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"skgo-generated-output","private":true}`), 0600); err != nil {
			return err
		}
		if _, err := os.Stat(filepath.Join(cfg.Web, "node_modules")); err == nil {
			if err := os.Symlink(filepath.Join(cfg.Web, "node_modules"), filepath.Join(dir, "node_modules")); err != nil {
				return err
			}
		}
		stage := cfg
		stage.frontendFiles = map[string]struct{}{}
		originals := map[string]string{}
		for path := range cfg.frontendFiles {
			relative, err := filepath.Rel(cfg.Web, path)
			if err != nil {
				return err
			}
			target := filepath.Join(dir, relative)
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			if err := os.WriteFile(target, []byte(g.files[path]), 0644); err != nil {
				return err
			}
			stage.frontendFiles[target] = struct{}{}
			originals[target] = path
		}
		if err := formatFrontendFiles(stage, dir); err != nil {
			return err
		}
		for target, path := range originals {
			data, err := os.ReadFile(target)
			if err != nil {
				return err
			}
			g.files[path] = string(data)
		}
	}
	return g.publish(cfg)
}

func removeOutput(cfg Config, path string) error {
	if cfg.generation != nil {
		cfg.generation.removed[path] = true
		return nil
	}
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// Check validates the foundational declaration block inside the consolidated
// output, without requiring the wrappers to be regenerated or touching files.
func verifyGoPart(path, expected string) error {
	actual, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("skgo: %s is missing or stale; run skgo generate", path)
	}
	declarations := func(source string) (string, error) {
		fs := token.NewFileSet()
		f, err := parser.ParseFile(fs, path, source, 0)
		if err != nil {
			return "", err
		}
		var b strings.Builder
		for _, d := range f.Decls {
			if imp, ok := d.(*ast.GenDecl); ok && imp.Tok == token.IMPORT {
				continue
			}
			raw := []byte(source[fs.Position(d.Pos()).Offset:fs.Position(d.End()).Offset])
			var scan scanner.Scanner
			sf := token.NewFileSet().AddFile("", -1, len(raw))
			scan.Init(sf, raw, nil, 0)
			for {
				_, tok, lit := scan.Scan()
				if tok == token.EOF {
					break
				}
				fmt.Fprintf(&b, "%d:%s;", tok, lit)
			}
		}
		return b.String(), nil
	}
	want, err := declarations(expected)
	if err != nil {
		return err
	}
	got, err := declarations(string(actual))
	if err != nil || !strings.Contains(got, want) {
		return fmt.Errorf("skgo: %s is missing or stale; run skgo generate", path)
	}
	return nil
}
