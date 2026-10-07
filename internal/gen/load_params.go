package gen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tylergannon/skgo"
	"golang.org/x/tools/go/packages"
)

const loadParamsFile = generatedGoFile

type goParamMatcher struct {
	name    string
	out     types.Type
	pkg     *types.Package
	pos     token.Position
	imports map[string]*packages.Package
}

type routeLoadParams struct {
	params       []skgo.ManifestParam
	matchers     map[string]goParamMatcher
	page, layout bool
	children     map[string]*routeLoadParams
}

// Parameter metadata comes from the installed Kit parser, not a second route
// grammar. This is build-time integration with Kit; no app JavaScript runs in
// the Go request matcher.
const kitLoadParams = `
import { pathToFileURL } from 'node:url';
import fs from 'node:fs';
import path from 'node:path';
const { readFileSync } = fs;
const { kit, ids, params, web, modules, obsolete, layouts } = JSON.parse(readFileSync(0, 'utf8'));
const { parse_route_id } = await import(pathToFileURL(kit + '/src/utils/routing.js'));
let names = [];
if (params) names = Object.keys((await import(pathToFileURL(params))).params ?? {});
let children = {};
if (layouts) {
 // Feed Kit the pending Go-backed modules without writing bootstrap stubs.
 // Kit still owns filename recognition, groups, resets and page ancestry.
 const pending = new Set(modules);
 const hidden = new Set(obsolete);
 const readdir = fs.readdirSync, read = fs.readFileSync;
 fs.readdirSync = (dir, options) => {
  const entries = readdir(dir, options).filter(entry => !hidden.has(path.join(String(dir), typeof entry === 'string' ? entry : entry.name)));
  for (const file of pending) {
   if (path.dirname(file) !== String(dir) || entries.some(entry => entry.name === path.basename(file))) continue;
   entries.push({name: path.basename(file), isDirectory: () => false, isSymbolicLink: () => false});
  }
  return entries;
 };
 fs.readFileSync = (file, ...args) => pending.has(String(file)) ? 'export const load = () => ({});' : read(file, ...args);
 const { default: create_manifest_data } = await import(pathToFileURL(kit + '/src/core/sync/create_manifest_data/index.js'));
 const absent = path.join(web, '.skgo-no-entry');
 const { routes } = create_manifest_data({
  extensions: ['.svelte'], moduleExtensions: ['.js', '.ts'], router: {type: 'pathname'},
  files: {routes: path.join(web, 'src/routes'), assets: absent, params: absent,
   hooks: {client: absent, server: absent, universal: absent}}
 }, web);
 const pages = new Map(routes.filter(route => route.leaf).map(route => [route.leaf, route.id]));
 for (const route of routes) {
  if (route.layout) children[route.id] = route.layout.child_pages.map(page => pages.get(page));
 }
}
console.log(JSON.stringify({ routes: ids.map(id => parse_route_id(id).params), names, children }));
`

// Refresh the event types before compiling application load bodies. Loading
// the matcher package separately lets an edited matcher signature replace a
// stale RouteParams even when the old load body no longer compiles.
func prepareLoadParams(cfg *Config, files []string) (map[string]*routeLoadParams, error) {
	dirs := map[string]bool{}
	loadKinds := map[string]map[string]bool{}
	var modules, obsolete []string
	hasLayouts := false
	for _, path := range files {
		if _, ok := loadFileNames[filepath.Base(path)]; ok {
			dir := filepath.Dir(path)
			dirs[dir] = true
			if loadKinds[dir] == nil {
				loadKinds[dir] = map[string]bool{}
			}
			loadKinds[dir][filepath.Base(path)] = true
			hasLayouts = hasLayouts || filepath.Base(path) == "layout.server.go"
		}
	}
	for _, file := range files {
		name := loadFileNames[filepath.Base(file)]
		if filepath.Base(file) == serverFileName {
			name = "+server.ts"
		}
		if name != "" {
			stem := filepath.Join(filepath.Dir(file), strings.TrimSuffix(name, ".ts"))
			modules = append(modules, stem+cfg.Language.ext())
			other := ".js"
			if cfg.Language.JavaScript() {
				other = ".ts"
			}
			// Match publication's ownership rule without deleting files before
			// this invocation succeeds. Authored counterparts stay visible to Kit.
			content, err := os.ReadFile(stem + other)
			if err != nil && !os.IsNotExist(err) {
				return nil, err
			}
			if generatedArtifact(content) {
				obsolete = append(obsolete, stem+other)
			}
		}
	}
	result := map[string]*routeLoadParams{}
	frontendParams := ""
	for _, ext := range []string{".ts", ".js"} {
		path := filepath.Join(cfg.Web, "src", "params"+ext)
		if _, err := os.Stat(path); err == nil {
			frontendParams = path
			break
		}
	}
	loadDirs := map[string]bool{}
	for dir := range dirs {
		loadDirs[dir] = true
	}
	// Shared params include callers with only frontend pages or endpoints, and
	// intermediate layout routes. Load ownership is not the caller universe.
	err := walkCallerRouteDirs(filepath.Join(cfg.Web, routesDir), map[string]bool{}, func(path string) { dirs[path] = true })
	if err != nil {
		return nil, err
	}
	if len(dirs) == 0 && frontendParams == "" {
		return result, writeSharedParams(*cfg, result)
	}
	var ordered []string
	ids := []string{}
	for dir := range dirs {
		ordered = append(ordered, dir)
	}
	sort.Strings(ordered)
	for _, dir := range ordered {
		rel, err := filepath.Rel(filepath.Join(cfg.Web, routesDir), dir)
		if err != nil || strings.HasPrefix(rel, "..") {
			return nil, fmt.Errorf("skgo: typed load outside %s: %s", routesDir, dir)
		}
		id := "/" + filepath.ToSlash(rel)
		if rel == "." {
			id = "/"
		}
		ids = append(ids, id)
	}
	kit := filepath.Join(cfg.Web, "node_modules", "@sveltejs", "kit")
	needsKit := frontendParams != "" || hasLayouts
	for _, id := range ids {
		needsKit = needsKit || strings.Contains(id, "[")
	}
	if _, err := os.Stat(filepath.Join(kit, "src", "utils", "routing.js")); err != nil && needsKit {
		return nil, fmt.Errorf("skgo: typed loads require installed SvelteKit route metadata: %w", err)
	}
	input, _ := json.Marshal(map[string]any{"kit": kit, "ids": ids, "params": frontendParams, "web": cfg.Web, "modules": modules, "obsolete": obsolete, "layouts": hasLayouts})
	cmd := exec.Command("node", "--input-type=module", "--eval", kitLoadParams)
	cmd.Dir, cmd.Stdin = cfg.Web, bytes.NewReader(input)
	var output []byte
	if _, statErr := os.Stat(filepath.Join(kit, "src", "utils", "routing.js")); statErr == nil {
		output, err = cmd.Output()
	} else {
		output, err = json.Marshal(map[string]any{"routes": make([][]skgo.ManifestParam, len(ids))})
	}
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("skgo: reading Kit route params: %s", exit.Stderr)
		}
		return nil, fmt.Errorf("skgo: reading Kit route params: %w", err)
	}
	var metadata struct {
		Routes   [][]skgo.ManifestParam `json:"routes"`
		Names    []string               `json:"names"`
		Children map[string][]string    `json:"children"`
	}
	if err := json.Unmarshal(output, &metadata); err != nil {
		return nil, err
	}
	if len(metadata.Routes) != len(ordered) {
		return nil, fmt.Errorf("skgo: incomplete Kit route params")
	}
	matchers, err := readGoParamMatchers(*cfg)
	if err != nil {
		return nil, err
	}
	cfg.matchers = map[string]goParamMatcher{}
	names := map[string]bool{}
	for _, name := range metadata.Names {
		names[name] = true
		matcher, ok := matchers[name]
		if !ok {
			return nil, fmt.Errorf("skgo: matcher %q requires a Go matcher in src/params.go", name)
		}
		cfg.matchers[name] = matcher
	}
	for i, dir := range ordered {
		info := &routeLoadParams{params: metadata.Routes[i], matchers: map[string]goParamMatcher{}, page: loadKinds[dir]["page.server.go"], layout: loadKinds[dir]["layout.server.go"]}
		for name, matcher := range matchers {
			if names[name] {
				info.matchers[name] = matcher
			}
		}
		for _, param := range info.params {
			if param.Matcher == "" {
				continue
			}
			matcher, ok := matchers[param.Matcher]
			if !ok || !names[param.Matcher] {
				return nil, fmt.Errorf("skgo: route %s requires Go and JS/TS matchers named %q in src/params.go and src/params.ts or .js", ids[i], param.Matcher)
			}
			info.matchers[param.Matcher] = matcher
		}
		result[dir] = info
	}
	byID := map[string]*routeLoadParams{}
	for i, dir := range ordered {
		byID[ids[i]] = result[dir]
	}
	for i, dir := range ordered {
		if !result[dir].layout {
			continue
		}
		result[dir].children = map[string]*routeLoadParams{}
		for _, id := range metadata.Children[ids[i]] {
			child := byID[id]
			if child == nil {
				return nil, fmt.Errorf("skgo: layout %s has unknown participating page %s", ids[i], id)
			}
			result[dir].children[id] = child
		}
	}
	if err := writeSharedParams(*cfg, result); err != nil {
		return nil, err
	}
	for _, dir := range ordered {
		if loadDirs[dir] {
			if err := writeLoadParams(*cfg, dir, result[dir]); err != nil {
				return nil, err
			}
		}
	}
	return result, nil
}

// Kit follows symlinked route directories too. Track only ancestor targets so
// two distinct route IDs may point to the same tree without being conflated.
func walkCallerRouteDirs(path string, ancestors map[string]bool, visit func(string)) error {
	real, err := filepath.EvalSymlinks(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if ancestors[real] {
		return fmt.Errorf("skgo: cyclic caller route directory %s", path)
	}
	ancestors[real] = true
	defer delete(ancestors, real)
	visit(path)
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		child := filepath.Join(path, entry.Name())
		isDir := entry.IsDir()
		if entry.Type()&os.ModeSymlink != 0 {
			info, err := os.Stat(child)
			if err != nil {
				return err
			}
			isDir = info.IsDir()
		}
		if isDir {
			if err := walkCallerRouteDirs(child, ancestors, visit); err != nil {
				return err
			}
		}
	}
	return nil
}

func readGoParamMatchers(cfg Config) (map[string]goParamMatcher, error) {
	result := map[string]goParamMatcher{}
	path := filepath.Join(cfg.Web, "src", "params.go")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return result, nil
	} else if err != nil {
		return nil, err
	}
	host, mod, err := moduleOf(cfg.Out)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(host, filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	forbidden, err := sharedForbiddenPackages(cfg)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	syntax, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}
	for _, imp := range syntax.Imports {
		dependency := strings.Trim(imp.Path.Value, "\"")
		for _, forbidden := range forbidden {
			if dependency == forbidden || strings.HasPrefix(dependency, forbidden+"/") {
				return nil, fmt.Errorf("skgo: %s: matcher package imports caller-owned or generated package %s; move matcher domain types to a leaf package", fset.Position(imp.Pos()), dependency)
			}
		}
	}
	loadCfg := &packages.Config{Dir: host, Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports}
	if cfg.generation != nil {
		loadCfg.Overlay, err = cfg.generation.overlay()
		if err != nil {
			return nil, err
		}
	}
	cleanup, err := preserveDependencyExports(loadCfg)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	// Export data describes type relationships, not every source import. Load
	// the complete import graph as metadata, independently of dependency syntax
	// and type information, for the shared-params ownership check.
	metadataCfg := *loadCfg
	metadataCfg.Mode = packages.NeedName | packages.NeedImports | packages.NeedDeps
	metadata, err := packages.Load(&metadataCfg, mod+"/"+filepath.ToSlash(rel))
	if err != nil {
		return nil, err
	}
	imports := map[string]*packages.Package{}
	packages.Visit(metadata, nil, func(p *packages.Package) { imports[p.PkgPath] = p })
	loaded, err := packages.Load(loadCfg, mod+"/"+filepath.ToSlash(rel))
	if err != nil {
		return nil, err
	}
	if len(loaded) != 1 {
		return nil, fmt.Errorf("skgo: cannot load Go params package")
	}
	p := loaded[0]
	var compilerErrors []packages.Error
	packages.Visit(metadata, nil, func(dependency *packages.Package) { compilerErrors = append(compilerErrors, dependency.Errors...) })
	packages.Visit(loaded, nil, func(dependency *packages.Package) { compilerErrors = append(compilerErrors, dependency.Errors...) })
	if len(compilerErrors) > 0 {
		sort.Slice(compilerErrors, func(i, j int) bool { return compilerErrors[i].Error() < compilerErrors[j].Error() })
		return nil, fmt.Errorf("skgo: %s: loading Go params: %v; matcher/domain dependencies must be leaf packages, not generated params or caller packages", fset.Position(syntax.Package), compilerErrors[0])
	}
	for _, file := range p.Syntax {
		if filepath.Base(p.Fset.Position(file.Pos()).Filename) != "params.go" {
			continue
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || !fn.Name.IsExported() {
				continue
			}
			obj, ok := p.TypesInfo.Defs[fn.Name].(*types.Func)
			if !ok {
				continue
			}
			sig := obj.Type().(*types.Signature)
			if sig.Variadic() || sig.TypeParams().Len() != 0 || sig.Params().Len() != 1 || !types.Identical(sig.Params().At(0).Type(), types.Typ[types.String]) || sig.Results().Len() != 2 || !types.Identical(sig.Results().At(1).Type(), types.Typ[types.Bool]) {
				return nil, fmt.Errorf("skgo: %s: matcher %s must be func(string) (T, bool)", p.Fset.Position(fn.Pos()), fn.Name.Name)
			}
			result[fn.Name.Name] = goParamMatcher{name: fn.Name.Name, out: sig.Results().At(0).Type(), pkg: p.Types, pos: p.Fset.Position(fn.Pos()), imports: imports}
		}
	}
	return result, nil
}

func paramField(name string) string {
	if name == "id" {
		return "ID"
	}
	parts := strings.FieldsFunc(name, func(r rune) bool { return r == '-' || r == '_' })
	for i, part := range parts {
		if part != "" {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, "")
}

func writeLoadParams(cfg Config, dir string, info *routeLoadParams) error {
	pkg, err := packageNameOf(dir)
	if err != nil {
		return err
	}
	imports := &fileImports{used: map[string]bool{"skgo": true}, byPkg: map[*types.Package]string{}}
	var body strings.Builder
	if info.page {
		if err := writeConcreteLoadDomain(&body, imports, cfg, "RouteParams", "Page", info.params, info.matchers); err != nil {
			return err
		}
	}
	if info.layout {
		if err := writeLayoutDomain(&body, imports, cfg, info); err != nil {
			return err
		}
	}
	body.WriteString("func SkgoParamMatchers() map[string]skgo.ParamMatcher {\n return map[string]skgo.ParamMatcher{\n")
	var names []string
	for name := range info.matchers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		matcher := info.matchers[name]
		qualifier := imports.packageAlias(matcher.pkg)
		fmt.Fprintf(&body, "%q: func(value string) (any, bool) { return %s.%s(value) },\n", name, qualifier, name)
	}
	body.WriteString("}\n}\n")
	var b strings.Builder
	b.WriteString(goHeader)
	fmt.Fprintf(&b, "package %s\n\nimport (skgo %q; appstate %q)\n\n", pkg, skgoPkg, cfg.LocalsPackage)
	imports.writeTo(&b)
	b.WriteString(body.String())
	formatted, err := format.Source([]byte(b.String()))
	if err != nil {
		return fmt.Errorf("skgo: formatting load params: %w", err)
	}
	path := filepath.Join(dir, loadParamsFile)
	if cfg.ReadOnly {
		if err := verifyGoPart(path, string(formatted)); err != nil {
			return err
		}
		if cfg.generation != nil {
			return cfg.generation.add(path, string(formatted))
		}
		return nil
	}

	return (&app{cfg: cfg}).write(path, string(formatted))
}
