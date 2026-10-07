package gen

import (
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
		cleanup, e := preserveDependencyExports(cfg)
		if e != nil {
			return nil, e
		}
		a.cfg.generation.cleanups = append(a.cfg.generation.cleanups, cleanup)
		path, e := a.importPath(a.links.authoredDir(dir))
		if e != nil {
			return nil, e
		}
		loaded, err = grammar.LoadWithConfig(cfg, path)
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
