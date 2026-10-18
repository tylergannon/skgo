package gen

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"
)

type entriesFn struct {
	name, module, stub string
	goPkg              *goPackage
	pos                token.Position
}

func (a *app) scanEntries(gp *goPackage, p *packages.Package, file *ast.File, path, mod, stub string) error {
	for _, decl := range file.Decls {
		vars, ok := decl.(*ast.GenDecl)
		if !ok || vars.Tok != token.VAR {
			continue
		}
		for _, spec := range vars.Specs {
			for _, value := range spec.(*ast.ValueSpec).Values {
				call, ok := value.(*ast.CallExpr)
				if !ok {
					continue
				}
				ident := calleeIdent(call.Fun)
				if ident == nil {
					continue
				}
				marker, _ := p.TypesInfo.Uses[ident].(*types.Func)
				if marker == nil || marker.Pkg() == nil || marker.Pkg().Path() != skgoPkg || marker.Name() != "Entries" {
					continue
				}
				pos := p.Fset.Position(call.Pos())
				if base := filepath.Base(path); base != "page.server.go" && base != serverFileName {
					return fmt.Errorf("skgo: %s: Entries belongs in page.server.go or server.go", pos)
				}
				if len(call.Args) != 1 {
					return fmt.Errorf("skgo: %s: Entries takes one named function", pos)
				}
				name, ok := call.Args[0].(*ast.Ident)
				if !ok {
					return fmt.Errorf("skgo: %s: Entries needs a named function", pos)
				}
				fn, _ := p.TypesInfo.Uses[name].(*types.Func)
				if fn == nil || fn.Pkg() != p.Types {
					return fmt.Errorf("skgo: %s: Entries needs a function in this route package", pos)
				}
				sig := fn.Type().(*types.Signature)
				domain := p.Types.Scope().Lookup("EntryParams")
				valid := !sig.Variadic() && sig.Params().Len() == 0 && sig.Results().Len() == 2 && isError(sig.Results().At(1).Type())
				if valid {
					slice, ok := sig.Results().At(0).Type().(*types.Slice)
					valid = ok && domain != nil && types.Identical(slice.Elem(), domain.Type())
				}
				if !valid {
					return fmt.Errorf("skgo: %s: Entries must be func() ([]EntryParams, error) using this route's generated EntryParams", pos)
				}
				for _, previous := range a.entries {
					if previous.module == mod {
						return fmt.Errorf("skgo: %s: entries declared twice, at %s and %s", mod, previous.pos, pos)
					}
				}
				a.entries = append(a.entries, &entriesFn{name: fn.Name(), module: mod, stub: stub, goPkg: gp, pos: pos})
			}
		}
	}
	return nil
}

func (a *app) entryFor(module string) *entriesFn {
	for _, entry := range a.entries {
		if entry.module == module {
			return entry
		}
	}
	return nil
}
func (a *app) writeEntriesStub(b *strings.Builder, module string) {
	if a.entryFor(module) == nil {
		return
	}
	if a.cfg.Language.JavaScript() {
		b.WriteString("\n/** @type {import('./$types').EntryGenerator} */\nexport const entries = async () => { throw new Error('skgo: entries implemented in Go'); };\n")
	} else {
		b.WriteString("\nexport const entries: import('./$types').EntryGenerator = async () => { throw new Error('skgo: entries implemented in Go'); };\n")
	}
}

// Bootstrap only the input type a route declares, before its Go bodies compile.
func fileHasEntries(path string) (bool, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return false, err
	}
	aliases := map[string]bool{}
	for _, spec := range file.Imports {
		name, err := strconv.Unquote(spec.Path.Value)
		if err != nil || name != skgoPkg {
			continue
		}
		alias := "skgo"
		if spec.Name != nil {
			alias = spec.Name.Name
		}
		aliases[alias] = true
	}
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		fun := call.Fun
		for {
			switch indexed := fun.(type) {
			case *ast.IndexExpr:
				fun = indexed.X
				continue
			case *ast.IndexListExpr:
				fun = indexed.X
				continue
			}
			break
		}
		switch fn := fun.(type) {
		case *ast.SelectorExpr:
			if receiver, ok := fn.X.(*ast.Ident); ok && aliases[receiver.Name] && fn.Sel.Name == "Entries" {
				found = true
			}
		case *ast.Ident:
			if aliases["."] && fn.Name == "Entries" {
				found = true
			}
		}
		return true
	})
	return found, nil
}
