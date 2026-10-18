package gen

import (
	"fmt"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"
)

func validateApplicationConfig(cfg Config) error {
	if cfg.ConfigPackage == "" {
		return nil
	}
	host, _, err := moduleOf(cfg.Out)
	if err != nil {
		return err
	}
	load := &packages.Config{Dir: host, Mode: packages.NeedName | packages.NeedTypes | packages.NeedImports | packages.NeedDeps}
	load.Overlay, err = cfg.generation.overlay()
	if err != nil {
		return err
	}
	pkgs, err := cfg.loadPackages(load, cfg.ConfigPackage)
	if err != nil {
		return err
	}
	if len(pkgs) != 1 {
		return fmt.Errorf("skgo: cannot load application config package %s", cfg.ConfigPackage)
	}
	var problem error
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		if problem == nil && len(p.Errors) != 0 {
			problem = fmt.Errorf("skgo: selected application config %s.%s: %s", cfg.ConfigPackage, cfg.ConfigSymbol, p.Errors[0])
		}
	})
	if problem != nil {
		return problem
	}
	return validateApplicationConfigSymbol(pkgs[0].Types, cfg.ConfigPackage, cfg.ConfigSymbol)
}

func validateApplicationConfigSymbol(pkg *types.Package, path, symbol string) error {
	value, ok := pkg.Scope().Lookup(symbol).(*types.Var)
	if !ok || !token.IsExported(symbol) {
		return fmt.Errorf("skgo: application config %s.%s must be an exported ApplicationConfig value", path, symbol)
	}
	named, ok := types.Unalias(value.Type()).(*types.Named)
	if !ok || named.Obj().Pkg() == nil || named.Obj().Pkg().Path() != skgoPkg || named.Obj().Name() != "ApplicationConfig" {
		return fmt.Errorf("skgo: application config %s.%s must have type skgo.ApplicationConfig", path, symbol)
	}
	return nil
}
