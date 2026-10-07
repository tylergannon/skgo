package gen

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/tools/go/packages"
)

// sourcePath resolves existing symlink ancestors even when the leaf is new.
func sourcePath(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if !os.IsNotExist(err) {
		return resolved, err
	}
	parent := filepath.Dir(path)
	if parent == path {
		return "", err
	}
	resolved, err = sourcePath(parent)
	return filepath.Join(resolved, filepath.Base(path)), err
}

func withinDir(root, path string) (string, bool) {
	rel, err := filepath.Rel(root, path)
	return rel, err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// stagePackageOverlay makes only the branches containing overlays real; other
// entries are read through symlinks. Dependency modules stay at their original
// addresses, so Go still owns export caching and invalidation. The view lives
// only as long as this load (or a grammar's recursive dependency loads).
// Callers load exact package paths; symlinked untouched directories are not
// traversed by wildcard patterns such as ./....
func stagePackageOverlay(cfg *packages.Config) (func(), error) {
	noop := func() {}
	logicalRoot, _, err := moduleOf(cfg.Dir)
	if err != nil {
		return noop, err
	}
	physicalRoot, err := filepath.EvalSymlinks(logicalRoot)
	if err != nil {
		return noop, err
	}
	overlay := map[string][]byte{}
	branches := map[string]bool{".": true}
	for path, source := range cfg.Overlay {
		canonical, err := sourcePath(path)
		if err != nil {
			return noop, err
		}
		rel, inside := withinDir(physicalRoot, canonical)
		if !inside {
			// A caller-supplied overlay outside the host module retains the
			// ordinary loader semantics; SKGo generates only inside its host.
			return noop, nil
		}
		overlay[rel] = source
		for parent := filepath.Dir(rel); !branches[parent]; parent = filepath.Dir(parent) {
			branches[parent] = true
		}
	}
	view, err := os.MkdirTemp("", "skgo-package-view-")
	if err != nil {
		return noop, err
	}
	cleanup := func() { os.RemoveAll(view) }
	fail := func(err error) (func(), error) { cleanup(); return noop, err }
	populated := map[string]bool{}
	var populate func(string, bool) error
	populate = func(rel string, embedded bool) error {
		if populated[rel] {
			return nil
		}
		populated[rel] = true
		dest := filepath.Join(view, rel)
		if err := os.MkdirAll(dest, 0700); err != nil {
			return err
		}
		entries, err := os.ReadDir(filepath.Join(physicalRoot, rel))
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		// The go command forbids symlinks in embedded assets. A package with
		// embed directives needs regular sibling files and asset directories.
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".go") {
				raw, err := os.ReadFile(filepath.Join(physicalRoot, rel, entry.Name()))
				if err != nil {
					return err
				}
				embedded = embedded || bytes.Contains(raw, []byte("//go:embed"))
			}
		}
		for path, raw := range overlay {
			if filepath.Dir(path) == rel {
				embedded = embedded || bytes.Contains(raw, []byte("//go:embed"))
			}
		}
		for _, entry := range entries {
			child := filepath.Join(rel, entry.Name())
			if _, replaced := overlay[child]; replaced || branches[child] && !embedded {
				continue
			}
			source, dest := filepath.Join(physicalRoot, child), filepath.Join(view, child)
			if entry.IsDir() && embedded {
				if err := populate(child, true); err != nil {
					return err
				}
				continue
			}
			var err error
			if entry.Type().IsRegular() {
				err = os.Link(source, dest)
				if err != nil {
					err = copyPackageFile(source, dest)
				}
			} else {
				if entry.Type()&os.ModeSymlink != 0 {
					target, resolveErr := filepath.EvalSymlinks(source)
					if resolveErr == nil {
						if rel, inside := withinDir(physicalRoot, target); inside {
							source = filepath.Join(view, rel)
						}
					}
				}
				err = os.Symlink(source, dest)
			}
			if err != nil {
				return err
			}
		}
		return nil
	}
	ordered := make([]string, 0, len(branches))
	for branch := range branches {
		ordered = append(ordered, branch)
	}
	sort.Strings(ordered)
	for _, branch := range ordered {
		if err := populate(branch, false); err != nil {
			return fail(err)
		}
	}
	for rel, source := range overlay {
		if err := os.WriteFile(filepath.Join(view, rel), source, 0600); err != nil {
			return fail(err)
		}
	}
	// Relative replacements are relative to go.mod, whose address has moved.
	// Rewrite only the temporary copy, keeping local dependencies in place.
	modPath := filepath.Join(view, "go.mod")
	raw, err := os.ReadFile(modPath)
	if err != nil {
		return fail(err)
	}
	mod, err := modfile.Parse(modPath, raw, nil)
	if err != nil {
		return fail(err)
	}
	for _, replace := range mod.Replace {
		if replace.New.Version == "" && !filepath.IsAbs(replace.New.Path) {
			if err := mod.AddReplace(replace.Old.Path, replace.Old.Version, filepath.Join(physicalRoot, replace.New.Path), ""); err != nil {
				return fail(err)
			}
		}
	}
	raw, err = mod.Format()
	if err != nil {
		return fail(err)
	}
	if err := os.Remove(modPath); err != nil { // unlink, never write through
		return fail(err)
	}
	if err := os.WriteFile(modPath, raw, 0600); err != nil {
		return fail(err)
	}
	// The Go command may update sums. Never let that touch a hard-linked or
	// symlinked original, including when the ordinary loader would do so.
	for _, name := range []string{"go.sum", "go.work", "go.work.sum"} {
		path := filepath.Join(view, name)
		raw, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fail(err)
		}
		if err := os.Remove(path); err != nil {
			return fail(err)
		}
		if err := os.WriteFile(path, raw, 0600); err != nil {
			return fail(err)
		}
	}
	// An active workspace must select this view instead of the original module.
	work := exec.Command("go", "env", "GOWORK")
	work.Dir, work.Env = cfg.Dir, cfg.Env
	workOutput, err := work.Output()
	if err != nil {
		return fail(fmt.Errorf("locating Go workspace: %w", err))
	}
	workPath := strings.TrimSpace(string(workOutput))
	if workPath != "" && workPath != "off" {
		raw, err := os.ReadFile(workPath)
		if err != nil {
			return fail(err)
		}
		workspace, err := modfile.ParseWork(workPath, raw, nil)
		if err != nil {
			return fail(err)
		}
		uses := make([]*modfile.Use, 0, len(workspace.Use))
		for _, use := range workspace.Use {
			path := use.Path
			if !filepath.IsAbs(path) {
				path = filepath.Join(filepath.Dir(workPath), path)
			}
			canonical, err := filepath.EvalSymlinks(path)
			if err != nil {
				return fail(err)
			}
			if canonical == physicalRoot {
				path = view
			}
			uses = append(uses, &modfile.Use{Path: path, ModulePath: use.ModulePath})
		}
		workspace.SetUse(uses)
		workspace.Cleanup()
		for _, replace := range workspace.Replace {
			if replace.New.Version == "" && !filepath.IsAbs(replace.New.Path) {
				if err := workspace.AddReplace(replace.Old.Path, replace.Old.Version, filepath.Join(filepath.Dir(workPath), replace.New.Path), ""); err != nil {
					return fail(err)
				}
			}
		}
		stagedWork := filepath.Join(view, "go.work")
		if err := os.WriteFile(stagedWork, modfile.Format(workspace.Syntax), 0600); err != nil {
			return fail(err)
		}
		env := cfg.Env
		if env == nil {
			env = os.Environ()
		}
		cfg.Env = append(append([]string(nil), env...), "GOWORK="+stagedWork)
	}
	parse := cfg.ParseFile
	if parse == nil {
		parse = func(fset *token.FileSet, filename string, source []byte) (*ast.File, error) {
			return parser.ParseFile(fset, filename, source, parser.AllErrors|parser.ParseComments)
		}
	}
	cfg.ParseFile = func(fset *token.FileSet, filename string, source []byte) (*ast.File, error) {
		original := filename
		if rel, inside := withinDir(view, filename); inside {
			original = filepath.Join(logicalRoot, rel)
		}
		if original != filename {
			// Keep File.Name aligned with packages.GoFiles for decorators;
			// a standard line directive locates both parser and compiler errors
			// in authored sources, preserving any later authored directives.
			source = packageSource(original, source)
		}
		return parse(fset, filename, source)
	}
	rel, _ := filepath.Rel(logicalRoot, cfg.Dir)
	cfg.Dir = filepath.Join(view, rel)
	cfg.Overlay = nil
	return cleanup, nil
}

// Resolve authored relative line directives before moving the parser's file
// address. Scan comments so a directive-looking line in a raw string keeps its
// value. Absolute directives and empty-filename continuations stay unchanged.
func packageSource(original string, source []byte) []byte {
	var out bytes.Buffer
	out.WriteString("//line " + original + ":1:1\n")
	if !bytes.Contains(source, []byte("line ")) {
		out.Write(source)
		return out.Bytes()
	}
	fset := token.NewFileSet()
	file := fset.AddFile(original, -1, len(source))
	var scan scanner.Scanner
	scan.Init(file, source, nil, scanner.ScanComments)
	copied := 0
	for {
		pos, tok, text := scan.Scan()
		if tok == token.EOF {
			break
		}
		if tok != token.COMMENT {
			continue
		}
		offset := file.Offset(pos)
		if strings.HasPrefix(text, "//line ") {
			if offset > 0 && source[offset-1] != '\n' {
				continue
			}
		} else if strings.HasPrefix(text, "/*line ") && strings.HasSuffix(text, "*/") {
			text = strings.TrimSuffix(text, "*/")
		} else {
			continue
		}
		body := text[7:]
		colon := strings.LastIndexByte(body, ':')
		if colon < 0 {
			continue
		}
		number, err := strconv.Atoi(body[colon+1:])
		if err != nil || number <= 0 || number > 1<<30 {
			continue
		}
		name := body[:colon]
		if previous := strings.LastIndexByte(name, ':'); previous >= 0 {
			if _, err := strconv.Atoi(name[previous+1:]); err == nil {
				name = name[:previous]
			}
		}
		if name == "" || filepath.IsAbs(name) {
			continue
		}
		start := offset + 7
		out.Write(source[copied:start])
		out.WriteString(filepath.Join(filepath.Dir(original), name))
		copied = start + len(name)
	}
	out.Write(source[copied:])
	return out.Bytes()
}

func copyPackageFile(source, dest string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	closeErr := out.Close()
	if err != nil {
		return err
	}
	return closeErr
}
