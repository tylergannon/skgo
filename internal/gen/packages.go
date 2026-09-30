package gen

import "github.com/tylergannon/polytype/grammar"

// loadGrammar reuses package loading within one run. Generated registrations
// do not change the authored types; a subsequent run gets a fresh app and
// reloads them, including any edits made since the previous generation.
func (a *app) loadGrammar(dir string) (*grammar.Package, error) {
	if loaded := a.grammarPackages[dir]; loaded != nil {
		return loaded, nil
	}
	loaded, err := grammar.Load(dir)
	if err != nil {
		return nil, err
	}
	if a.grammarPackages == nil {
		a.grammarPackages = map[string]*grammar.Package{}
	}
	a.grammarPackages[dir] = loaded
	return loaded, nil
}
