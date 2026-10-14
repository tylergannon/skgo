package gen

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/mod/modfile"
)

// generationSourceView is deliberately local to SKGO's disk-backed Run profile.
// Mutable modules are copied from captured bytes. Installed toolchains and
// versioned module-cache contents retain the Go command's immutability contract.
// No file in the source view is hard-linked to an authored file.
type generationSourceView struct {
	env         []string
	roots       map[string]string // original module directory -> private directory
	cgoPackages map[string]bool
	cgoUsed     bool
}

type cacheModule struct {
	Path, Dir, GoMod, Version string
	Main                      bool
	Replace                   *cacheModule
}

type cacheSources struct {
	Modules         []cacheModule
	Environment     map[string]string `json:"-"`
	EnvironmentHash string
	Controls        []string
	Aliases         map[string]string
}

// Only compiler/source selection settings are retained. Proxy and authentication
// configuration stays in the invoking process environment, never in cache records.
var cacheGoEnvironment = []string{"GO111MODULE", "GOARCH", "GOOS", "GOAMD64", "GOARM", "GOARM64", "GO386", "GOMIPS", "GOMIPS64", "GOPPC64", "GORISCV64", "GOWASM", "GOEXPERIMENT", "GOFIPS140", "GOROOT", "GOPATH", "GOMODCACHE", "GOTOOLCHAIN", "GOFLAGS", "GOWORK", "GOENV", "CGO_ENABLED"}

// Effective values are needed only while constructing a cold source view.
// Persist their digest, including flags that may themselves contain secrets.
func cacheEnvironmentHash(environment map[string]string) string {
	raw, _ := json.Marshal(environment)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func currentCacheEnvironment(dir string) (map[string]string, error) {
	cmd := exec.Command("go", append([]string{"env", "-json"}, cacheGoEnvironment...)...)
	cmd.Dir = dir
	raw, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var environment map[string]string
	if err := json.Unmarshal(raw, &environment); err != nil {
		return nil, err
	}
	return environment, nil
}

func acquireCacheSources(cfg Config) (cacheSources, error) {
	host, _, err := moduleOf(cfg.Out)
	if err != nil {
		return cacheSources{}, err
	}
	environment, err := currentCacheEnvironment(host)
	if err != nil {
		return cacheSources{}, err
	}
	s := cacheSources{Environment: environment, EnvironmentHash: cacheEnvironmentHash(environment), Aliases: map[string]string{}}
	if flags := s.Environment["GOFLAGS"]; strings.Contains(flags, "-overlay") || strings.Contains(flags, "-modfile") || strings.Contains(flags, "-mod=vendor") || strings.Contains(flags, "-toolexec") || strings.Contains(flags, "-pkgdir") {
		return s, fmt.Errorf("external Go source views are not cached")
	}
	if _, err := os.Stat(filepath.Join(host, "vendor")); err == nil {
		return s, fmt.Errorf("vendored source is not cached")
	}
	cmd := exec.Command("go", "list", "-m", "-json", "all")
	cmd.Dir = host
	data, err := cmd.Output()
	if err != nil {
		return s, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	for {
		var m cacheModule
		if err := decoder.Decode(&m); err == io.EOF {
			break
		} else if err != nil {
			return s, err
		}
		if m.Replace != nil {
			importPath := m.Path
			m = *m.Replace
			m.Path = importPath
		}
		if m.Dir != "" && (m.Main || m.Version == "") {
			originalDir := m.Dir
			m.Dir, err = filepath.EvalSymlinks(m.Dir)
			if err != nil {
				return s, err
			}
			s.Aliases[originalDir] = m.Dir
			s.Modules = append(s.Modules, m)
		}
	}
	if len(s.Modules) == 0 {
		return s, fmt.Errorf("no mutable module source found")
	}
	sort.Slice(s.Modules, func(i, j int) bool { return s.Modules[i].Dir < s.Modules[j].Dir })
	for _, name := range []string{"GOENV", "GOWORK"} {
		if path := s.Environment[name]; path != "" && path != "off" {
			s.Controls = append(s.Controls, path)
			if name == "GOWORK" {
				s.Controls = append(s.Controls, path+".sum")
			}
		}
	}
	// A newly created ancestor go.work changes selection even when no workspace
	// existed when this entry was created.
	for dir := host; ; dir = filepath.Dir(dir) {
		s.Controls = append(s.Controls, filepath.Join(dir, "go.work"), filepath.Join(dir, "go.work.sum"))
		if filepath.Dir(dir) == dir {
			break
		}
	}
	return s, nil
}

// capture reads the bytes that will be used for cold computation, and the same
// bytes on a warm lookup. Directory traversal discovers additions/deletions;
// generated artifacts are excluded by ownership, never solely by filename.
func (s cacheSources) capture(cfg Config) (map[string][]byte, string, error) {
	for alias, original := range s.Aliases {
		current, err := filepath.EvalSymlinks(alias)
		if err != nil || current != original {
			return nil, "", fmt.Errorf("local module location changed: %s", alias)
		}
	}
	files := map[string][]byte{}
	roots := map[string]bool{}
	for _, m := range s.Modules {
		roots[m.Dir] = true
		if _, err := os.Stat(filepath.Join(m.Dir, "vendor")); err == nil {
			return nil, "", fmt.Errorf("vendored source is not cached")
		}
	}
	host, _, err := moduleOf(cfg.Out)
	if err != nil {
		return nil, "", err
	}
	host, err = filepath.EvalSymlinks(host)
	if err != nil {
		return nil, "", err
	}
	read := func(path string) error {
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			files[path] = nil
			return nil
		}
		if err != nil {
			return err
		}
		if cacheOwned(cfg, path, data) {
			return nil
		}
		files[path] = data
		return nil
	}
	goDirs := map[string]bool{}
	for _, m := range s.Modules {
		err := filepath.WalkDir(m.Dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if path != m.Dir {
					if roots[path] || cacheToolingDirectory(d.Name()) {
						return filepath.SkipDir
					}
					if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
						return filepath.SkipDir
					}
				}
				if path == filepath.Join(cfg.Out, linkRootName) {
					return filepath.SkipDir
				}
				return nil
			}
			if d.Type()&os.ModeSymlink != 0 {
				if fi, err := os.Stat(path); err == nil && fi.IsDir() {
					return nil // Go directory walking ignores symlinks; explicit imports are checked below.
				}
			}
			if strings.HasSuffix(path, ".go") || d.Name() == "go.mod" || d.Name() == "go.sum" {
				if err := read(path); err != nil {
					return err
				}
				if strings.HasSuffix(path, ".go") {
					goDirs[filepath.Dir(path)] = true
				}
			}
			return nil
		})
		if err != nil {
			return nil, "", err
		}
	}
	// Non-Go compilation inputs and embed resources belong to the same captured
	// view. Go's own directive parser supplies patterns; Go still diagnoses them.
	for dir := range goDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, "", err
		}
		overlay := map[string][]byte{}
		for _, entry := range entries {
			path := filepath.Join(dir, entry.Name())
			if entry.IsDir() {
				continue
			}
			if data, ok := files[path]; ok && strings.HasSuffix(path, ".go") {
				overlay[entry.Name()] = data
			}
			if !strings.HasSuffix(path, ".go") && (strings.HasSuffix(path, ".s") || strings.HasSuffix(path, ".h") || strings.HasSuffix(path, ".c") || strings.HasSuffix(path, ".syso")) {
				if err := read(path); err != nil {
					return nil, "", err
				}
			}
		}
		// Owned generated files are absent from the analysis view. Hide their
		// directives from the embed scanner too, even if old output is broken.
		pkgName := "ignored"
		for name, source := range overlay {
			if parsed, err := parser.ParseFile(token.NewFileSet(), name, source, parser.PackageClauseOnly); err == nil {
				pkgName = parsed.Name.Name
				break
			}
		}
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".go") {
				if _, ok := overlay[entry.Name()]; !ok {
					overlay[entry.Name()] = []byte("package " + pkgName + "\n")
				}
			}
		}
		for _, pattern := range stagedEmbedPatterns(dir, ".", overlay) {
			matches, err := fs.Glob(os.DirFS(dir), strings.TrimPrefix(pattern, "all:"))
			if err != nil {
				return nil, "", err
			}
			for _, match := range matches {
				start := filepath.Join(dir, filepath.FromSlash(match))
				if strings.Contains(start, string(filepath.Separator)+"node_modules"+string(filepath.Separator)) {
					return nil, "", fmt.Errorf("embedding node_modules is not cached")
				}
				err := filepath.WalkDir(start, func(path string, d fs.DirEntry, err error) error {
					if err != nil {
						return err
					}
					if d.IsDir() {
						return nil
					}
					return read(path)
				})
				if err != nil {
					return nil, "", err
				}
			}
		}
	}
	// Frontend source is read by SKGO's route, ownership and page-option checks.
	// Node's arbitrary matcher evaluation is separately retained on every Run.
	err = filepath.WalkDir(filepath.Join(cfg.Web, "src"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if cacheToolingDirectory(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			if fi, err := os.Stat(path); err == nil && fi.IsDir() {
				return fmt.Errorf("symlinked frontend directories are not cached")
			}
		}
		return read(path)
	})
	if err != nil {
		return nil, "", err
	}
	for _, name := range []string{"package.json", "pnpm-lock.yaml", "tsconfig.json", "jsconfig.json"} {
		if err := read(filepath.Join(cfg.Web, name)); err != nil {
			return nil, "", err
		}
	}
	for _, path := range s.Controls {
		if err := read(path); err != nil {
			return nil, "", err
		}
	}
	// Installed Node packages follow the package-manager immutability contract;
	// the resolved package identity and launcher change on reinstall/upgrade.
	for _, name := range []string{"vite-plus/package.json", "@sveltejs/kit/package.json", ".bin/vp"} {
		if err := read(filepath.Join(cfg.Web, "node_modules", filepath.FromSlash(name))); err != nil {
			return nil, "", err
		}
	}
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	h := sha256.New()
	encoder := json.NewEncoder(h)
	for _, path := range paths {
		_ = encoder.Encode(path)
		if files[path] == nil {
			_ = encoder.Encode(nil)
		} else {
			sum := sha256.Sum256(files[path])
			_ = encoder.Encode(sum)
		}
	}
	return files, hex.EncodeToString(h.Sum(nil)), nil
}

func cacheOwned(cfg Config, path string, data []byte) bool {
	if path == filepath.Join(cfg.Out, inventoryName) || path == filepath.Join(cfg.Web, "skgo.remotes.json") || path == filepath.Join(cfg.Web, "skgo.env.json") {
		return true
	}
	if path == cfg.SwiftOut && cfg.SwiftOut != "" {
		return true
	}
	if path == filepath.Join(cfg.Web, "src", "routes", "go.mod") && bytes.HasPrefix(data, []byte(boundaryMarker)) {
		return true
	}
	if filepath.Base(path) == generatedGoFile && bytes.HasPrefix(data, []byte(goHeader)) {
		if withinTree(cfg.Out, path) || withinTree(filepath.Join(cfg.Web, "src"), path) {
			return true
		}
		// Locals' helper is another declared output of this invocation.
		host, mod, err := moduleOf(cfg.Out)
		if err == nil {
			dir := host
			if cfg.LocalsPackage != mod {
				dir = filepath.Join(host, filepath.FromSlash(strings.TrimPrefix(cfg.LocalsPackage, mod+"/")))
			}
			if path == filepath.Join(dir, generatedGoFile) {
				return true
			}
		}
	}
	if withinTree(filepath.Join(cfg.Web, "src"), path) && generatedArtifact(data) {
		return generatedModuleName(filepath.Base(path)) || strings.HasPrefix(filepath.Base(path), "hooks.server.") || filepath.Base(path) == "env.ts" || filepath.Base(path) == "env.js"
	}
	return false
}

func (s cacheSources) materialize(cfg Config, files map[string][]byte) (Config, func(), error) {
	root, err := os.MkdirTemp("", "skgo-analysis-")
	if err != nil {
		return cfg, func() {}, err
	}
	// Go resolves working directories physically. Keep diagnostic and output
	// rebasing on that same identity (not /var versus /private/var aliases).
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		_ = os.RemoveAll(root)
		return cfg, func() {}, err
	}
	root = canonical
	cleanup := func() { _ = os.RemoveAll(root) }
	fail := func(err error) (Config, func(), error) { cleanup(); return cfg, func() {}, err }
	view := &generationSourceView{roots: map[string]string{}, cgoPackages: map[string]bool{}}
	for path, data := range files {
		if !strings.HasSuffix(path, ".go") || data == nil {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, data, parser.ImportsOnly)
		if err != nil {
			continue
		}
		for _, imp := range parsed.Imports {
			importPath, _ := strconv.Unquote(imp.Path.Value)
			for _, m := range s.Modules {
				if !strings.HasPrefix(importPath, m.Path+"/") {
					continue
				}
				rel := strings.TrimPrefix(importPath, m.Path+"/")
				for _, part := range strings.Split(rel, "/") {
					if cacheToolingDirectory(part) {
						return fail(fmt.Errorf("explicit import in tooling directory is not cached: %s", importPath))
					}
				}
				dir := filepath.Join(m.Dir, filepath.FromSlash(rel))
				if physical, err := filepath.EvalSymlinks(dir); err == nil && physical != dir {
					return fail(fmt.Errorf("symlinked import directory is not cached: %s", importPath))
				}
			}
			if imp.Path.Value != `"C"` {
				continue
			}
			best := 0
			pkg := ""
			for _, m := range s.Modules {
				if rel, ok := withinDir(m.Dir, filepath.Dir(path)); ok && len(m.Dir) > best {
					best = len(m.Dir)
					pkg = m.Path
					if rel != "." {
						pkg += "/" + filepath.ToSlash(rel)
					}
				}
			}
			if pkg != "" {
				view.cgoPackages[pkg] = true
			}
		}
	}
	for i, m := range s.Modules {
		view.roots[m.Dir] = filepath.Join(root, fmt.Sprintf("module-%d", i))
	}
	for path, data := range files {
		target := view.mapPath(path, true)
		if target == path || data == nil || strings.Contains(path, string(filepath.Separator)+"node_modules"+string(filepath.Separator)) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return fail(err)
		}
		if err := os.WriteFile(target, data, 0600); err != nil {
			return fail(err)
		}
	}
	for _, m := range s.Modules {
		original := filepath.Join(m.Dir, "go.mod")
		target := view.mapPath(original, true)
		mod, err := modfile.Parse(original, files[original], nil)
		if err != nil {
			return fail(err)
		}
		for _, rep := range mod.Replace {
			if rep.New.Version == "" {
				path := rep.New.Path
				if !filepath.IsAbs(path) {
					path = filepath.Join(m.Dir, path)
				}
				path, err = filepath.EvalSymlinks(path)
				if err != nil {
					return fail(err)
				}
				mapped := view.mapPath(path, true)
				if mapped == path {
					return fail(fmt.Errorf("uncaptured local replacement %s", path))
				}
				if err := mod.AddReplace(rep.Old.Path, rep.Old.Version, mapped, ""); err != nil {
					return fail(err)
				}
			}
		}
		data, err := mod.Format()
		if err != nil {
			return fail(err)
		}
		if err := os.WriteFile(target, data, 0600); err != nil {
			return fail(err)
		}
	}
	// Freeze Go environment settings. Workspace selection is rewritten only in
	// the private copy; original module/workspace files are never touched.
	env := append([]string(nil), os.Environ()...)
	for key, value := range s.Environment {
		if key != "GOWORK" && key != "GOENV" && key != "GOMOD" && key != "GOVERSION" && key != "GOTOOLDIR" {
			env = append(env, key+"="+value)
		}
	}
	env = append(env, "GOENV=off", "GOWORK=off")
	if work := s.Environment["GOWORK"]; work != "" && work != "off" {
		wf, err := modfile.ParseWork(work, files[work], nil)
		if err != nil {
			return fail(err)
		}
		var uses []*modfile.Use
		for _, use := range wf.Use {
			path := use.Path
			if !filepath.IsAbs(path) {
				path = filepath.Join(filepath.Dir(work), path)
			}
			path, err = filepath.EvalSymlinks(path)
			if err != nil {
				return fail(err)
			}
			mapped := view.mapPath(path, true)
			if mapped == path {
				return fail(fmt.Errorf("uncaptured workspace %s", path))
			}
			uses = append(uses, &modfile.Use{Path: mapped, ModulePath: use.ModulePath})
		}
		// SetUse appends unchanged paths too; remove old directives first.
		for _, old := range wf.Use {
			if err := wf.DropUse(old.Path); err != nil {
				return fail(err)
			}
		}
		wf.Cleanup()
		wf.SetUse(uses)
		for _, rep := range wf.Replace {
			if rep.New.Version == "" {
				path := rep.New.Path
				if !filepath.IsAbs(path) {
					path = filepath.Join(filepath.Dir(work), path)
				}
				path, err = filepath.EvalSymlinks(path)
				if err != nil {
					return fail(err)
				}
				mapped := view.mapPath(path, true)
				if mapped == path {
					return fail(fmt.Errorf("uncaptured workspace replacement %s", path))
				}
				if err := wf.AddReplace(rep.Old.Path, rep.Old.Version, mapped, ""); err != nil {
					return fail(err)
				}
			}
		}
		wf.Cleanup()
		target := filepath.Join(root, "go.work")
		if err := os.WriteFile(target, modfile.Format(wf.Syntax), 0600); err != nil {
			return fail(err)
		}
		env = append(env, "GOWORK="+target)
	}
	view.env = env
	cfg.Web, cfg.Out, cfg.SwiftOut = view.mapPath(cfg.Web, true), view.mapPath(cfg.Out, true), view.mapPath(cfg.SwiftOut, true)
	// Empty source roots remain valid after the last remote is deleted.
	if err := os.MkdirAll(filepath.Join(cfg.Web, "src"), 0700); err != nil {
		return fail(err)
	}
	if cfg.kitMetadata != nil {
		cfg.kitMetadata, err = cfg.kitMetadata.rebase(view.mapPath(cfg.Web, false), cfg.Web)
		if err != nil {
			return fail(err)
		}
	}
	// Generation's adapter check and optional formatter use installed packages;
	// analysis itself consumes the captured Kit metadata, not this symlink.
	originalWeb := view.mapPath(cfg.Web, false)
	if _, err := os.Stat(filepath.Join(originalWeb, "node_modules")); err == nil {
		if err := os.MkdirAll(cfg.Web, 0700); err != nil {
			return fail(err)
		}
		if err := os.Symlink(filepath.Join(originalWeb, "node_modules"), filepath.Join(cfg.Web, "node_modules")); err != nil {
			return fail(err)
		}
	}
	cfg.CacheDir = "off"
	originalLog := cfg.Logf
	cfg.Logf = func(format string, args ...any) { originalLog("%s", view.originalText(fmt.Sprintf(format, args...))) }
	cfg.sourceView = view
	return cfg, cleanup, nil
}

func (v *generationSourceView) mapPath(path string, toPrivate bool) string {
	if path == "" {
		return path
	}
	best := ""
	mapped := path
	for from, to := range v.roots {
		if !toPrivate {
			from, to = to, from
		}
		if len(from) > len(best) {
			if rel, ok := withinDir(from, path); ok {
				best = from
				mapped = filepath.Join(to, rel)
			}
		}
	}
	return mapped
}
func (v *generationSourceView) originalText(text string) string {
	for from, to := range v.roots {
		text = strings.ReplaceAll(text, to, from)
	}
	return text
}

// Native headers can name files outside the module. Only reached local cgo
// packages make a generation ineligible; unrelated native commands do not.
func (v *generationSourceView) noteTypes(root *types.Package) {
	seen := map[*types.Package]bool{}
	var visit func(*types.Package)
	visit = func(p *types.Package) {
		if p == nil || seen[p] {
			return
		}
		seen[p] = true
		if v.cgoPackages[p.Path()] {
			v.cgoUsed = true
		}
		for _, imp := range p.Imports() {
			visit(imp)
		}
	}
	visit(root)
}

func cacheToolingDirectory(name string) bool {
	switch name {
	case ".git", ".hg", ".svn", ".claude", ".agents", ".codex", "node_modules":
		return true
	}
	return false
}
