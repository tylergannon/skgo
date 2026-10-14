package gen

import (
	"fmt"
	"github.com/tylergannon/polytype/grammar"
	"golang.org/x/tools/go/packages"
)

// loadGrammar reuses package loading within one run. Generated registrations
// do not change the authored types; a subsequent run gets a fresh app and
// reloads them, including any edits made since the previous generation.
func (a *app) loadGrammar(dir string) (*grammar.Package, error) {
	if loaded := a.grammarPackages[dir]; loaded != nil {
		return loaded, nil
	}
	var loaded *grammar.Package
	var err error
	if a.cfg.generation == nil {
		loaded, err = grammar.Load(dir)
	} else {
		cfg := &packages.Config{Dir: a.hostDir}
		cfg.Overlay, err = a.cfg.generation.overlay()
		if err != nil {
			return nil, err
		}
		path, e := a.importPath(a.links.authoredDir(dir))
		if e != nil {
			return nil, e
		}
		cfg.Mode = grammar.LoadMode
		cfg.BuildFlags = []string{"-tags=jsonschema"}
		for _, snapshot := range a.cfg.Packages {
			if roots, ok := snapshot.lookup(cfg, []string{path}); ok {
				all := make([]*packages.Package, 0, len(snapshot.packages))
				for _, p := range snapshot.packages {
					all = append(all, p)
				}
				loaded, err = grammar.FromPackages(cfg, roots[0], all...)
				break
			}
		}
		if loaded == nil && err == nil {
			if a.cfg.OnPackageLoad != nil {
				a.cfg.OnPackageLoad(cfg, []string{path})
			}
			if a.cfg.LoadPackages != nil {
				// FromPackages cannot lazily load dependency source. A replacement
				// loader must supply that source in this same type universe.
				cfg.Mode |= packages.NeedDeps
				roots, e := a.cfg.LoadPackages(cfg, path)
				if e != nil {
					return nil, e
				}
				if len(roots) != 1 {
					return nil, fmt.Errorf("skgo: cannot load projection package %s", path)
				}
				loaded, err = grammar.FromPackages(cfg, roots[0])
			} else {
				cleanup, e := preserveDependencyExports(cfg)
				if e != nil {
					return nil, e
				}
				a.cfg.generation.cleanups = append(a.cfg.generation.cleanups, cleanup)
				loaded, err = grammar.LoadWithConfig(cfg, path)
			}
		}
	}
	if err != nil {
		return nil, err
	}
	if a.grammarPackages == nil {
		a.grammarPackages = map[string]*grammar.Package{}
	}
	a.grammarPackages[dir] = loaded
	return loaded, nil
}
