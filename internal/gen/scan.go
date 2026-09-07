package gen

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"golang.org/x/tools/go/packages"
)

// skgoPkg is the import path of the marker API. Markers are recognised by the
// package a resolved identifier belongs to, never by the text `skgo.Query`, so
// an aliased import or a local function of the same name behaves correctly.
const skgoPkg = "github.com/tylergannon/skgo"

type remoteKind string

const (
	kindQuery   remoteKind = "query"
	kindCommand remoteKind = "command"
	kindLive    remoteKind = "query.live"
	kindForm    remoteKind = "form"
)

var markerKinds = map[string]remoteKind{
	"Query":     kindQuery,
	"Command":   kindCommand,
	"LiveQuery": kindLive,
	"Form":      kindForm,
}

// loadFn is one declared server load.
type loadFn struct {
	// name is the Go identifier. It is not `load`: a route directory holds
	// both `+page.server.ts` and `+layout.server.ts`, so their two Go files
	// are one Go package and cannot both declare a function of that name. The
	// file the marker sits in is what says which stub it generates.
	name string
	// goPkg is the package that declares it.
	goPkg *goPackage
	// module is the vite-root-relative path of the `+*.server.ts` that will
	// carry it, e.g. "src/routes/account/+layout.server.ts". Kit records the
	// same string for the node, which is what joins the two halves.
	module string
	// stub is the absolute path of that file.
	stub string
	// out is the load's result type, straight out of the marker's generic
	// instantiation.
	out types.Type
	pos token.Position
}

// remoteFn is one declared remote function.
type remoteFn struct {
	kind remoteKind
	// name is the Go identifier, which is also the TypeScript export name.
	name string
	// goPkg is the package that declares it.
	goPkg *goPackage
	// module is the vite-root-relative path of the `.remote.ts` that will
	// carry it, e.g. "src/routes/todos/todos.remote.ts".
	module string
	// stub is the absolute path of that file.
	stub string
	// in and out are the argument and result types, straight out of the
	// marker's generic instantiation.
	in, out types.Type
	pos     token.Position
}

// goPackage is one Go package that declares remote functions.
//
// A package under the route tree has two directories, and confusing them is
// the classic way to get this wrong: `dir` is where the developer's source
// lives and where everything generated for a human to see must land, while
// `loadDir` is the link Go compiled it through. `go list` reports the link
// unresolved, so every path that comes back from `packages.Load` is a loadDir
// path until it is translated.
type goPackage struct {
	pkg *packages.Package
	dir string
	// loadDir is the directory Go named the package by. It equals dir for a
	// package Go can already name.
	loadDir string
	// alias is the import alias the generated bindings file uses.
	alias string
}

type app struct {
	cfg Config

	remotes   []*remoteFn
	loads     []*loadFn
	endpoints []*endpointFn
	// transported are the app's `transport` hook entries, from src/hooks.go.
	transported []*transportedType
	// pkgs is every package that declares remote functions, loads or server
	// routes, in load order.
	pkgs []*goPackage
	// stubs groups the functions by the `.remote.ts` they land in.
	stubs map[string][]*remoteFn
	// typeSets records, per Go package that owns a named type used on the
	// wire, the types to declare to polytype.
	typeSets map[*types.Package]*namedTypes
	// hostModule is the module the generated bindings package belongs to.
	hostModule string
	hostDir    string
	// links gives the route tree's packages import paths Go can spell.
	links *routeLinks
}

// transportedType is one entry of the app's transport hook: a Go type that
// crosses the wire as a custom type, under the key kit's client looks it up by.
type transportedType struct {
	// key is the property name in kit's transport object, and the tag the
	// value travels under.
	key string
	// named is the Go type, which must be a named type: the generated codec
	// and the TypeScript class both need a name to be spelled by.
	named *types.Named
	// goPkg is the package src/hooks.go lives in.
	goPkg *goPackage
	pos   token.Position
}

type namedTypes struct {
	pkg *types.Package
	// dir is the directory the polytype registration file and its output go
	// in. For a package of the app's own it is the authored directory, where
	// the developer's source lives; for a foreign package it is the
	// declaration package skgo writes into the app instead.
	dir string
	// loadDir is the address polytype is given, which for a route package is
	// its link rather than its authored path.
	loadDir string
	// names are the type names as the defining package spells them, which is
	// also how they reach TypeScript.
	names []string
	seen  map[string]bool
	// tsDir is where polytype writes types.ts for this package.
	tsDir string
	// foreign marks a package outside the app's own tree, whose types are
	// declared again locally so that skgo never writes into source the app
	// does not own.
	foreign bool
}

// localName is the identifier a type is declared under in the package that
// carries its polytype registration: its own name for the app's packages, and
// a distinct one where the declaration was relocated. See
// writeLocalDeclarations for why the two must differ.
func (s *namedTypes) localName(name string) string {
	if s.foreign {
		return "Skgo" + name
	}
	return name
}

// loadApp resolves every package holding a `.remote.go` file and reads the
// markers out of it.
func loadApp(cfg Config, files []string) (*app, error) {
	a := &app{
		cfg:      cfg,
		stubs:    map[string][]*remoteFn{},
		typeSets: map[*types.Package]*namedTypes{},
	}

	var err error
	a.hostDir, a.hostModule, err = moduleOf(cfg.Out)
	if err != nil {
		return nil, err
	}

	if a.links, err = newRouteLinks(cfg, a.hostDir, a.hostModule); err != nil {
		return nil, err
	}
	if err := a.links.sync(); err != nil {
		return nil, err
	}

	dirs := map[string]bool{}
	for _, f := range files {
		dirs[filepath.Dir(f)] = true
	}
	var patterns []string
	for dir := range dirs {
		path, err := a.importPath(dir)
		if err != nil {
			return nil, err
		}
		patterns = append(patterns, path)
	}
	sort.Strings(patterns)

	loaded, err := packages.Load(&packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports,
		Dir: a.hostDir,
	}, patterns...)
	if err != nil {
		return nil, fmt.Errorf("skgo: loading remote packages: %w", err)
	}
	var loadErr error
	packages.Visit(loaded, nil, func(p *packages.Package) {
		for _, e := range p.Errors {
			if loadErr == nil {
				loadErr = fmt.Errorf("skgo: %s: %v", p.PkgPath, e)
			}
		}
	})
	if loadErr != nil {
		return nil, loadErr
	}

	wanted := map[string]bool{}
	for _, f := range files {
		wanted[f] = true
	}

	for i, p := range loaded {
		gp := &goPackage{pkg: p, alias: fmt.Sprintf("skgo%d", i)}
		if len(p.GoFiles) > 0 {
			gp.loadDir = filepath.Dir(p.GoFiles[0])
			gp.dir = a.links.authoredDir(gp.loadDir)
		}
		found := false
		for _, file := range p.Syntax {
			path := a.links.authoredPath(p.Fset.Position(file.Pos()).Filename)
			if !wanted[path] {
				continue
			}
			fns, loads, endpoints, err := a.scanFile(gp, p, file, path)
			if err != nil {
				return nil, err
			}
			if len(fns) > 0 || len(loads) > 0 || len(endpoints) > 0 {
				found = true
			}
			a.remotes = append(a.remotes, fns...)
			a.loads = append(a.loads, loads...)
			a.endpoints = append(a.endpoints, endpoints...)
		}
		if found {
			a.pkgs = append(a.pkgs, gp)
		}
	}

	sort.Slice(a.remotes, func(i, j int) bool {
		if a.remotes[i].module != a.remotes[j].module {
			return a.remotes[i].module < a.remotes[j].module
		}
		return a.remotes[i].name < a.remotes[j].name
	})
	for _, fn := range a.remotes {
		a.stubs[fn.stub] = append(a.stubs[fn.stub], fn)
	}
	sort.Slice(a.loads, func(i, j int) bool { return a.loads[i].module < a.loads[j].module })
	sort.Slice(a.endpoints, func(i, j int) bool {
		if a.endpoints[i].module != a.endpoints[j].module {
			return a.endpoints[i].module < a.endpoints[j].module
		}
		return a.endpoints[i].method < a.endpoints[j].method
	})
	return a, a.checkDuplicates()
}

// scanFile reads the markers in one `.remote.go`, `page.server.go`,
// `layout.server.go` or `server.go` file. The file's name decides which markers
// may appear in it, because it also decides what kit compiles beside it.
func (a *app) scanFile(gp *goPackage, p *packages.Package, file *ast.File, path string) ([]*remoteFn, []*loadFn, []*endpointFn, error) {
	base := filepath.Base(path)
	_, isLoadFile := loadFileNames[base]
	isServerFile := base == serverFileName
	isHooksFile := base == hooksFileName

	if isHooksFile {
		return nil, nil, nil, a.scanHooks(gp, p, file, path)
	}

	stub := strings.TrimSuffix(path, ".go") + ".ts"
	if tsName, isLoad := loadFileNames[base]; isLoad {
		stub = filepath.Join(filepath.Dir(path), tsName)
	}
	if isServerFile {
		stub = endpointStubPath(path)
	}
	mod, err := webRel(a.cfg.Web, stub)
	if err != nil {
		return nil, nil, nil, err
	}

	routeID := ""
	if isServerFile {
		if routeID, err = routeIDFor(mod); err != nil {
			return nil, nil, nil, err
		}
	}

	var remotes []*remoteFn
	var loads []*loadFn
	var endpoints []*endpointFn
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, value := range vs.Values {
				call, ok := value.(*ast.CallExpr)
				if !ok {
					continue
				}
				fn, load, endpoint, err := a.readMarker(gp, p, call, mod, stub, routeID)
				if err != nil {
					return nil, nil, nil, err
				}
				if fn != nil {
					remotes = append(remotes, fn)
				}
				if load != nil {
					loads = append(loads, load)
				}
				if endpoint != nil {
					endpoints = append(endpoints, endpoint)
				}
			}
		}
	}

	if (isLoadFile || isServerFile) && len(remotes) > 0 {
		return nil, nil, nil, fmt.Errorf("skgo: %s declares a remote function; a remote function belongs in a *.remote.go file", path)
	}
	if !isLoadFile && len(loads) > 0 {
		return nil, nil, nil, fmt.Errorf("skgo: %s declares a server load; a load belongs in page.server.go or layout.server.go", path)
	}
	if !isServerFile && len(endpoints) > 0 {
		return nil, nil, nil, fmt.Errorf("skgo: %s declares a server route; a server route belongs in %s, which is where kit's +server.ts is generated", path, serverFileName)
	}
	return remotes, loads, endpoints, nil
}

// readMarker turns one call expression into a remoteFn, or returns nil if the
// call is not a marker at all.
func (a *app) readMarker(gp *goPackage, p *packages.Package, call *ast.CallExpr, mod, stub, routeID string) (*remoteFn, *loadFn, *endpointFn, error) {
	ident := calleeIdent(call.Fun)
	if ident == nil {
		return nil, nil, nil, nil
	}
	obj, _ := p.TypesInfo.Uses[ident].(*types.Func)
	if obj == nil || obj.Pkg() == nil || obj.Pkg().Path() != skgoPkg {
		return nil, nil, nil, nil
	}
	kind, isRemote := markerKinds[obj.Name()]
	isLoad := obj.Name() == "Load"
	method, isEndpoint := endpointMarkers[obj.Name()]
	if !isRemote && !isLoad && !isEndpoint {
		return nil, nil, nil, nil
	}

	pos := p.Fset.Position(call.Pos())
	inst := p.TypesInfo.Instances[ident]
	if isLoad {
		// A load marker is still generic — a load has no argument to make
		// optional — so its result comes from the instantiation.
		if inst.TypeArgs == nil || inst.TypeArgs.Len() != 1 {
			return nil, nil, nil, fmt.Errorf("skgo: %s: cannot read the types of this skgo.%s declaration", pos, obj.Name())
		}
	}
	if len(call.Args) != 1 {
		return nil, nil, nil, fmt.Errorf("skgo: %s: skgo.%s takes exactly one argument, the function to publish", pos, obj.Name())
	}

	arg, ok := call.Args[0].(*ast.Ident)
	if !ok {
		return nil, nil, nil, fmt.Errorf("skgo: %s: skgo.%s needs a named function, not an expression — declare the function and pass its name", pos, obj.Name())
	}
	target, _ := p.TypesInfo.Uses[arg].(*types.Func)
	if target == nil {
		return nil, nil, nil, fmt.Errorf("skgo: %s: %s is not a function", pos, arg.Name)
	}
	if target.Pkg() != p.Types {
		return nil, nil, nil, fmt.Errorf("skgo: %s: %s is declared in another package; it must live in the file that publishes it", pos, arg.Name)
	}

	if isEndpoint {
		if routeID == "" {
			return nil, nil, nil, fmt.Errorf("skgo: %s: skgo.%s declares a server route, which belongs in %s", pos, obj.Name(), serverFileName)
		}
		return nil, nil, &endpointFn{
			method:  method,
			name:    target.Name(),
			goPkg:   gp,
			module:  mod,
			stub:    stub,
			routeID: routeID,
			pos:     pos,
		}, nil
	}

	if isLoad {
		return nil, &loadFn{
			name:   target.Name(),
			goPkg:  gp,
			module: mod,
			stub:   stub,
			out:    inst.TypeArgs.At(0),
			pos:    pos,
		}, nil, nil
	}

	in, out, err := remoteSignature(kind, target)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("skgo: %s: %s is declared as a %s, but %w", pos, target.Name(), kind, err)
	}

	return &remoteFn{
		kind:   kind,
		name:   target.Name(),
		goPkg:  gp,
		module: mod,
		stub:   stub,
		in:     in,
		out:    out,
		pos:    pos,
	}, nil, nil, nil
}

// remoteSignature reads a marked function's argument and result types out of
// its own declaration.
//
// The marker cannot carry them. `skgo.Query` takes `any`, because kit's
// `query(fn)` accepts `(arg?) => Output` and a generic constraint spelled
// `func(context.Context, In) (Out, error)` would force every no-argument
// remote function to name a placeholder type that exists for no other reason.
// So the shape is checked here, against the real signature, and reported at
// the declaration's position — which is where a compiler would have reported
// it.
//
// in is nil for a function that takes no argument. That is the same nil the
// TypeScript stub reads as kit's no-validator overload, and it is why no skgo
// type reaches the type projector.
func remoteSignature(kind remoteKind, target *types.Func) (in, out types.Type, err error) {
	sig, ok := target.Type().(*types.Signature)
	if !ok {
		return nil, nil, fmt.Errorf("it is not a function")
	}
	if sig.Variadic() {
		return nil, nil, fmt.Errorf("its signature is %s: a %s cannot be variadic, because kit calls it with one argument at most", sig, kind)
	}

	want := shapeOf(kind)
	params, results := sig.Params(), sig.Results()

	if params.Len() == 0 || !isContext(params.At(0).Type()) {
		return nil, nil, fmt.Errorf("its signature is %s. %s", sig, want)
	}

	if kind == kindLive {
		// The yield is last, so the optional argument sits between it and the
		// context, exactly where an argument sits for the other kinds.
		if results.Len() != 1 || !isError(results.At(0).Type()) {
			return nil, nil, fmt.Errorf("its signature is %s. %s", sig, want)
		}
		if params.Len() < 2 || params.Len() > 3 {
			return nil, nil, fmt.Errorf("its signature is %s. %s", sig, want)
		}
		yielded, ok := yieldType(params.At(params.Len() - 1).Type())
		if !ok {
			return nil, nil, fmt.Errorf("its signature is %s. %s", sig, want)
		}
		if params.Len() == 3 {
			in = params.At(1).Type()
		}
		return in, yielded, nil
	}

	if params.Len() > 2 {
		return nil, nil, fmt.Errorf("its signature is %s. %s", sig, want)
	}
	if results.Len() != 2 || !isError(results.At(1).Type()) {
		return nil, nil, fmt.Errorf("its signature is %s. %s", sig, want)
	}
	if params.Len() == 2 {
		in = params.At(1).Type()
	}
	return in, results.At(0).Type(), nil
}

// shapeOf is the sentence that says what the kind accepts. A form is the one
// kind whose argument is not optional: kit hands its handler the submission,
// and a form that ignores it is a form that ignores what the visitor typed.
func shapeOf(kind remoteKind) string {
	switch kind {
	case kindLive:
		return "A query.live is func(context.Context, func(Out) error) error, or func(context.Context, In, func(Out) error) error when it takes an argument."
	case kindForm:
		return "A form is func(context.Context, In) (Out, error): kit hands a form handler the submission, so its argument is not optional."
	}
	return fmt.Sprintf("A %s is func(context.Context) (Out, error), or func(context.Context, In) (Out, error) when it takes an argument.", kind)
}

// yieldType reads Out out of a `func(Out) error` parameter.
func yieldType(t types.Type) (types.Type, bool) {
	sig, ok := types.Unalias(t).(*types.Signature)
	if !ok || sig.Variadic() {
		return nil, false
	}
	if sig.Params().Len() != 1 || sig.Results().Len() != 1 || !isError(sig.Results().At(0).Type()) {
		return nil, false
	}
	return sig.Params().At(0).Type(), true
}

func isContext(t types.Type) bool {
	named, ok := types.Unalias(t).(*types.Named)
	if !ok || named.Obj().Pkg() == nil {
		return false
	}
	return named.Obj().Pkg().Path() == "context" && named.Obj().Name() == "Context"
}

func isError(t types.Type) bool {
	return types.Identical(types.Unalias(t), types.Universe.Lookup("error").Type())
}

func calleeIdent(fun ast.Expr) *ast.Ident {
	switch f := fun.(type) {
	case *ast.Ident:
		return f
	case *ast.SelectorExpr:
		return f.Sel
	case *ast.IndexExpr: // explicit type argument
		return calleeIdent(f.X)
	case *ast.IndexListExpr:
		return calleeIdent(f.X)
	}
	return nil
}

func (a *app) checkDuplicates() error {
	seen := map[string]*remoteFn{}
	for _, fn := range a.remotes {
		key := fn.module + "#" + fn.name
		if prev, dup := seen[key]; dup {
			return fmt.Errorf("skgo: %s is declared twice, at %s and %s", key, prev.pos, fn.pos)
		}
		seen[key] = fn
	}
	loads := map[string]*loadFn{}
	for _, load := range a.loads {
		if prev, dup := loads[load.module]; dup {
			return fmt.Errorf("skgo: %s has two server loads, at %s and %s", load.module, prev.pos, load.pos)
		}
		loads[load.module] = load
	}
	return nil
}

// importPath names the Go package in dir. A route directory is called `[id]`
// or `(app)` and cannot be spelled in an import path at all, so the answer is
// the link tree: the package is compiled through an address Go can name.
func (a *app) importPath(dir string) (string, error) {
	if link := a.links.linkFor(dir); link != nil {
		return link.importPath, nil
	}
	rel, err := filepath.Rel(a.hostDir, dir)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("skgo: %s is outside the Go module rooted at %s", dir, a.hostDir)
	}
	path := a.hostModule + "/" + filepath.ToSlash(rel)
	if err := module.CheckImportPath(path); err != nil {
		return "", fmt.Errorf("skgo: Go cannot name a package in %s, so a remote function cannot live there.\n"+
			"  Only directories under %s get a generated import path; anywhere else, the directory name\n"+
			"  has to be one Go can spell", dir, filepath.Join(a.cfg.Web, "src", "routes"))
	}
	return path, nil
}

// moduleOf finds the Go module containing dir by walking up to its go.mod.
// `go list -m` is not used: inside a workspace it reports every main module,
// not the one that owns this directory.
func moduleOf(dir string) (root, path string, err error) {
	for cur := dir; ; {
		candidate := filepath.Join(cur, "go.mod")
		if raw, err := os.ReadFile(candidate); err == nil {
			path := modfile.ModulePath(raw)
			if path == "" {
				return "", "", fmt.Errorf("skgo: %s declares no module path", candidate)
			}
			return cur, path, nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", "", fmt.Errorf("skgo: no go.mod above %s", dir)
		}
		cur = parent
	}
}
