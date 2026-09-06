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
)

var markerKinds = map[string]remoteKind{
	"Query":     kindQuery,
	"Command":   kindCommand,
	"LiveQuery": kindLive,
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
type goPackage struct {
	pkg *packages.Package
	dir string
	// alias is the import alias the generated bindings file uses.
	alias string
}

type app struct {
	cfg Config

	remotes []*remoteFn
	// pkgs is every package that declares remote functions, in load order.
	pkgs []*goPackage
	// stubs groups the functions by the `.remote.ts` they land in.
	stubs map[string][]*remoteFn
	// typeSets records, per Go package that owns a named type used on the
	// wire, the types to declare to polytype.
	typeSets map[*types.Package]*namedTypes
	// hostModule is the module the generated bindings package belongs to.
	hostModule string
	hostDir    string
}

type namedTypes struct {
	pkg   *types.Package
	dir   string
	names []string
	seen  map[string]bool
	// tsDir is where polytype writes types.ts for this package.
	tsDir string
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
			gp.dir = filepath.Dir(p.GoFiles[0])
		}
		found := false
		for _, file := range p.Syntax {
			path := p.Fset.Position(file.Pos()).Filename
			if !wanted[path] {
				continue
			}
			fns, err := a.scanFile(gp, p, file, path)
			if err != nil {
				return nil, err
			}
			if len(fns) > 0 {
				found = true
			}
			a.remotes = append(a.remotes, fns...)
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
	return a, a.checkDuplicates()
}

// scanFile reads the markers in one `.remote.go` file.
func (a *app) scanFile(gp *goPackage, p *packages.Package, file *ast.File, path string) ([]*remoteFn, error) {
	mod, err := webRel(a.cfg.Web, strings.TrimSuffix(path, ".go")+".ts")
	if err != nil {
		return nil, err
	}
	stub := strings.TrimSuffix(path, ".go") + ".ts"

	var out []*remoteFn
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
				fn, err := a.readMarker(gp, p, call, mod, stub)
				if err != nil {
					return nil, err
				}
				if fn != nil {
					out = append(out, fn)
				}
			}
		}
	}
	return out, nil
}

// readMarker turns one call expression into a remoteFn, or returns nil if the
// call is not a marker at all.
func (a *app) readMarker(gp *goPackage, p *packages.Package, call *ast.CallExpr, mod, stub string) (*remoteFn, error) {
	ident := calleeIdent(call.Fun)
	if ident == nil {
		return nil, nil
	}
	obj, _ := p.TypesInfo.Uses[ident].(*types.Func)
	if obj == nil || obj.Pkg() == nil || obj.Pkg().Path() != skgoPkg {
		return nil, nil
	}
	kind, ok := markerKinds[obj.Name()]
	if !ok {
		return nil, nil
	}

	pos := p.Fset.Position(call.Pos())
	inst := p.TypesInfo.Instances[ident]
	if inst.TypeArgs == nil || inst.TypeArgs.Len() != 2 {
		return nil, fmt.Errorf("skgo: %s: cannot read the argument and result types of this skgo.%s declaration", pos, obj.Name())
	}
	if len(call.Args) != 1 {
		return nil, fmt.Errorf("skgo: %s: skgo.%s takes exactly one argument, the function to publish", pos, obj.Name())
	}

	arg, ok := call.Args[0].(*ast.Ident)
	if !ok {
		return nil, fmt.Errorf("skgo: %s: skgo.%s needs a named function, not an expression — declare the function and pass its name", pos, obj.Name())
	}
	target, _ := p.TypesInfo.Uses[arg].(*types.Func)
	if target == nil {
		return nil, fmt.Errorf("skgo: %s: %s is not a function", pos, arg.Name)
	}
	if target.Pkg() != p.Types {
		return nil, fmt.Errorf("skgo: %s: %s is declared in another package; a remote function must live in the file that publishes it", pos, arg.Name)
	}

	return &remoteFn{
		kind:   kind,
		name:   target.Name(),
		goPkg:  gp,
		module: mod,
		stub:   stub,
		in:     inst.TypeArgs.At(0),
		out:    inst.TypeArgs.At(1),
		pos:    pos,
	}, nil
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
	return nil
}

// importPath names the Go package in dir. A SvelteKit route directory can be
// called `[id]` or `(app)`, which Go cannot spell in an import path; there is
// no way to compile such a package, so say so rather than emitting something
// that will not build.
func (a *app) importPath(dir string) (string, error) {
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
			"  A SvelteKit route directory with brackets or parentheses is not a legal Go import path.\n"+
			"  Move the *.remote.go file to a directory Go can name, such as its parent route, and import\n"+
			"  the generated stub from the page that needs it", dir)
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
