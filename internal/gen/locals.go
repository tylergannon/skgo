package gen

import (
	"fmt"
	"go/types"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
)

func (cfg Config) paramsImport() (string, error) {
	host, mod, err := moduleOf(cfg.Out)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(host, cfg.Out)
	if err != nil {
		return "", err
	}
	return mod + "/" + filepath.ToSlash(rel) + "/params", nil
}

func prepareLocals(cfg *Config) error {
	if cfg.LocalsPackage == "" {
		return fmt.Errorf("skgo: locals package is required (--locals-package)")
	}
	if cfg.LocalsType == "" {
		cfg.LocalsType = "Locals"
	}
	if cfg.HookSymbol == "" {
		cfg.HookSymbol = "Handle"
	}
	host, _, err := moduleOf(cfg.Out)
	if err != nil {
		return err
	}
	load := &packages.Config{Dir: host, Mode: packages.NeedName | packages.NeedFiles | packages.NeedTypes | packages.NeedImports | packages.NeedDeps}
	load.Overlay, err = cfg.generation.overlay()
	if err != nil {
		return err
	}
	cleanup, err := preserveDependencyExports(load)
	if err != nil {
		return err
	}
	defer cleanup()
	pkgs, err := packages.Load(load, cfg.LocalsPackage)
	if err != nil {
		return err
	}
	if len(pkgs) != 1 {
		return fmt.Errorf("skgo: cannot load locals package %s", cfg.LocalsPackage)
	}
	var problem error
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		if problem == nil && len(p.Errors) > 0 {
			problem = fmt.Errorf("skgo: locals package %s must be loadable without generated routing: %s", cfg.LocalsPackage, p.Errors[0])
		}
	})
	if problem != nil {
		return problem
	}
	pkg := pkgs[0]
	obj := pkg.Types.Scope().Lookup(cfg.LocalsType)
	if obj == nil {
		return fmt.Errorf("skgo: locals type %s.%s is missing", cfg.LocalsPackage, cfg.LocalsType)
	}
	if _, ok := obj.(*types.TypeName); !ok {
		return fmt.Errorf("skgo: configured locals must name a type")
	}
	named, ok := obj.Type().(*types.Named)
	if !ok || named.TypeParams().Len() != 0 || !obj.Exported() {
		return fmt.Errorf("skgo: locals must be an exported concrete named struct")
	}
	if _, ok := named.Underlying().(*types.Struct); !ok {
		return fmt.Errorf("skgo: locals must be a concrete named struct")
	}
	forbidden, err := sharedForbiddenPackages(*cfg)
	if err != nil {
		return err
	}
	paramsPath, err := cfg.paramsImport()
	if err != nil {
		return err
	}
	forbidden = append(forbidden, paramsPath)
	seen := map[*types.Package]bool{}
	var visit func(*types.Package) error
	visit = func(p *types.Package) error {
		if seen[p] {
			return nil
		}
		seen[p] = true
		for _, path := range forbidden {
			if p.Path() == path || strings.HasPrefix(p.Path(), path+"/") {
				return fmt.Errorf("skgo: locals dependency %s imports routing or generated bindings; move locals/domain resources to a leaf package", p.Path())
			}
		}
		for _, dep := range p.Imports() {
			if err := visit(dep); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(pkg.Types); err != nil {
		return err
	}
	if len(pkg.GoFiles) == 0 {
		return fmt.Errorf("skgo: locals package has no authored Go source")
	}
	// Dependency-export reuse may load a fresh package from a temporary module
	// view. Check ownership relative to that view, then publish in the authored
	// module; loader paths are never application output destinations.
	rel, err := filepath.Rel(load.Dir, filepath.Dir(pkg.GoFiles[0]))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("skgo: locals package %s must be owned by the application module", cfg.LocalsPackage)
	}
	cfg.localsDir = filepath.Join(host, rel)
	source := fmt.Sprintf("%spackage %s\nimport (\"context\"; skgo %q)\nfunc LocalsFrom(ctx context.Context) *%s { return skgo.RequestLocals[%s](ctx) }\n", goHeader, pkg.Name, skgoPkg, cfg.LocalsType, cfg.LocalsType)
	return (&app{cfg: *cfg}).write(filepath.Join(cfg.localsDir, generatedGoFile), source)
}

func validateHook(cfg Config) error {
	if cfg.HookPackage == "" {
		return nil
	}
	host, mod, err := moduleOf(cfg.Out)
	if err != nil {
		return err
	}
	rel, _ := filepath.Rel(host, cfg.Out)
	bindings := mod + "/" + filepath.ToSlash(rel)
	paramsPath, err := cfg.paramsImport()
	if err != nil {
		return err
	}
	load := &packages.Config{Dir: host, Mode: packages.NeedName | packages.NeedTypes | packages.NeedImports | packages.NeedDeps}
	load.Overlay, err = cfg.generation.overlay()
	if err != nil {
		return err
	}
	cleanup, err := preserveDependencyExports(load)
	if err != nil {
		return err
	}
	defer cleanup()
	pkgs, err := packages.Load(load, cfg.HookPackage, paramsPath)
	if err != nil {
		return err
	}
	var hook, params *packages.Package
	var problem error
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		if problem == nil && len(p.Errors) > 0 {
			problem = fmt.Errorf("skgo: selected hook %s.%s: %s", cfg.HookPackage, cfg.HookSymbol, p.Errors[0])
		}
	})
	if problem != nil {
		return problem
	}
	for _, p := range pkgs {
		if p.PkgPath == cfg.HookPackage {
			hook = p
		}
		if p.PkgPath == paramsPath {
			params = p
		}
	}
	if hook == nil || params == nil {
		return fmt.Errorf("skgo: cannot load selected hook")
	}
	seen := map[*types.Package]bool{}
	var visit func(*types.Package) error
	visit = func(p *types.Package) error {
		if seen[p] {
			return nil
		}
		seen[p] = true
		if p.Path() == bindings {
			return fmt.Errorf("skgo: selected hook dependency %s imports generated server bindings", p.Path())
		}
		for _, dep := range p.Imports() {
			if err := visit(dep); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(hook.Types); err != nil {
		return err
	}
	symbol := hook.Types.Scope().Lookup(cfg.HookSymbol)
	if symbol == nil || !symbol.Exported() {
		return fmt.Errorf("skgo: selected hook %s.%s is missing or not exported", cfg.HookPackage, cfg.HookSymbol)
	}
	switch symbol.(type) {
	case *types.Func, *types.Var:
	default:
		return fmt.Errorf("skgo: selected hook %s.%s must be a function or variable", cfg.HookPackage, cfg.HookSymbol)
	}
	if !types.ConvertibleTo(symbol.Type(), params.Types.Scope().Lookup("Middleware").Type()) {
		return fmt.Errorf("skgo: selected hook %s.%s must be convertible to params.Middleware", cfg.HookPackage, cfg.HookSymbol)
	}
	return nil
}

func (a *app) writeRequestBoundary() error {
	path, err := a.cfg.paramsImport()
	if err != nil {
		return err
	}
	hook := "params.Middleware(nil)"
	imp := ""
	if a.cfg.HookPackage != "" {
		imp = fmt.Sprintf("; serverhooks %q", a.cfg.HookPackage)
		hook = "params.Middleware(serverhooks." + a.cfg.HookSymbol + ")"
	}
	source := fmt.Sprintf("%spackage %s\nimport (\"net/http\"; skgo %q; params %q%s)\nfunc RequestBoundary(cfg skgo.HandleConfig, next http.Handler) http.Handler {return %s.Intercept(cfg,next)}\n", goHeader, a.cfg.Package, skgoPkg, path, imp, hook)
	return a.write(filepath.Join(a.cfg.Out, generatedGoFile), source)
}
