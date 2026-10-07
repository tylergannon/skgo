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
	name string
	out  types.Type
	pkg  *types.Package
	pos  token.Position
}

type routeLoadParams struct {
	params   []skgo.ManifestParam
	matchers map[string]goParamMatcher
}

// Parameter metadata comes from the installed Kit parser, not a second route
// grammar. This is build-time integration with Kit; no app JavaScript runs in
// the Go request matcher.
const kitLoadParams = `
import { pathToFileURL } from 'node:url';
import { readFileSync } from 'node:fs';
const { kit, ids, params } = JSON.parse(readFileSync(0, 'utf8'));
const { parse_route_id } = await import(pathToFileURL(kit + '/src/utils/routing.js'));
let names = [];
if (params) names = Object.keys((await import(pathToFileURL(params))).params ?? {});
console.log(JSON.stringify({ routes: ids.map(id => parse_route_id(id).params), names }));
`

// Refresh the event types before compiling application load bodies. Loading
// the matcher package separately lets an edited matcher signature replace a
// stale RouteParams even when the old load body no longer compiles.
func prepareLoadParams(cfg *Config, files []string) (map[string]*routeLoadParams, error) {
	dirs := map[string]bool{}
	for _, path := range files {
		if _, ok := loadFileNames[filepath.Base(path)]; ok {
			dirs[filepath.Dir(path)] = true
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
	needsKit := frontendParams != ""
	for _, id := range ids {
		needsKit = needsKit || strings.Contains(id, "[")
	}
	if _, err := os.Stat(filepath.Join(kit, "src", "utils", "routing.js")); err != nil && needsKit {
		return nil, fmt.Errorf("skgo: typed loads require installed SvelteKit route metadata: %w", err)
	}
	input, _ := json.Marshal(map[string]any{"kit": kit, "ids": ids, "params": frontendParams})
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
		Routes [][]skgo.ManifestParam `json:"routes"`
		Names  []string               `json:"names"`
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
		info := &routeLoadParams{params: metadata.Routes[i], matchers: map[string]goParamMatcher{}}
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
	loadCfg := &packages.Config{Dir: host, Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedDeps}
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
	loaded, err := packages.Load(loadCfg, mod+"/"+filepath.ToSlash(rel))
	if err != nil {
		return nil, err
	}
	if len(loaded) != 1 {
		return nil, fmt.Errorf("skgo: cannot load Go params package")
	}
	p := loaded[0]
	var compilerErrors []packages.Error
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
			result[fn.Name.Name] = goParamMatcher{name: fn.Name.Name, out: sig.Results().At(0).Type(), pkg: p.Types, pos: p.Fset.Position(fn.Pos())}
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
	var body, accessors, populate strings.Builder
	body.WriteString("// RouteParams stores converted values. Only accessor reads record dependencies\n// on the load invocation that constructed these params.\ntype RouteParams struct { event *skgo.Event\n")
	fields := map[string]bool{}
	for _, param := range info.params {
		field := paramField(param.Name)
		if !token.IsIdentifier(field) || fields[field] {
			return fmt.Errorf("skgo: route parameter %q has a conflicting Go field %q", param.Name, field)
		}
		fields[field] = true
		typ := "string"
		if param.Matcher != "" {
			typ = imports.typeExpr(info.matchers[param.Matcher].out)
		}
		helper := "LoadParamValue"
		result := typ
		if param.Optional {
			helper = "OptionalLoadParamValue"
			result = "*" + typ
		}
		// Prefixing the accessor name keeps private storage distinct from both
		// methods and the event pointer, including a route parameter named event.
		storage := "value" + field
		fmt.Fprintf(&body, "%s %s\n", storage, result)
		fmt.Fprintf(&accessors, "func (p RouteParams) %s() %s { skgo.TrackLoadParam(p.event, %q); return p.%s }\n\n", field, result, param.Name, storage)
		fmt.Fprintf(&populate, "%s: skgo.%s[%s](event, %q),\n", storage, helper, typ, param.Name)
	}
	body.WriteString("}\n\n")
	body.WriteString(accessors.String())
	body.WriteString("type RequestEvent = skgo.RequestEvent[RouteParams]\n\nfunc SkgoRequestEvent(event *skgo.Event) RequestEvent {\n return RequestEvent{Event: event, Params: RouteParams{event: event,\n")
	body.WriteString(populate.String())
	body.WriteString("}}\n}\n\nfunc SkgoParamMatchers() map[string]skgo.ParamMatcher {\n return map[string]skgo.ParamMatcher{\n")
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
	fmt.Fprintf(&b, "package %s\n\nimport skgo %q\n\n", pkg, skgoPkg)
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
