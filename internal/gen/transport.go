package gen

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tylergannon/polytype/devalue/codegen"
	"github.com/tylergannon/polytype/grammar"
	"github.com/tylergannon/polytype/typegrammar"
	"golang.org/x/tools/go/packages"
)

// scanHooks reads the app's `transport` hook out of `src/hooks.go`.
//
// Kit declares a transport as an object literal in `src/hooks.ts` and skgo
// mirrors it one marker per entry, because a Go type is what the server half
// has to switch on and Go has no object literal that carries one:
//
//	var _ = skgo.Transported[businesslogic.Money]("Money")
//
// The marker is resolved through go/types like every other one — by the package
// the identifier belongs to, never by matching the text `skgo.Transported`.
func (a *app) scanHooks(gp *goPackage, p *packages.Package, file *ast.File, path string) error {
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
				entry, err := a.readTransportMarker(gp, p, call)
				if err != nil {
					return err
				}
				if entry != nil {
					a.transported = append(a.transported, entry)
				}
			}
		}
	}
	if err := a.checkTransportKeys(path); err != nil {
		return err
	}
	return nil
}

// readTransportMarker turns one call expression into a transport entry, or
// returns nil if it is not a marker.
func (a *app) readTransportMarker(gp *goPackage, p *packages.Package, call *ast.CallExpr) (*transportedType, error) {
	ident := calleeIdent(call.Fun)
	if ident == nil {
		return nil, nil
	}
	obj, _ := p.TypesInfo.Uses[ident].(*types.Func)
	if obj == nil || obj.Pkg() == nil || obj.Pkg().Path() != skgoPkg || obj.Name() != "Transported" {
		return nil, nil
	}

	pos := p.Fset.Position(call.Pos())
	inst := p.TypesInfo.Instances[ident]
	if inst.TypeArgs == nil || inst.TypeArgs.Len() != 1 {
		return nil, fmt.Errorf("skgo: %s: cannot read the type of this skgo.Transported declaration", pos)
	}
	if len(call.Args) != 1 {
		return nil, fmt.Errorf("skgo: %s: skgo.Transported takes exactly one argument, the transport key", pos)
	}

	// The key is spelled into the wire and into src/hooks.ts, so it has to be
	// a constant this generator can read, not an expression evaluated later.
	tv, ok := p.TypesInfo.Types[call.Args[0]]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.String {
		return nil, fmt.Errorf("skgo: %s: skgo.Transported needs a string literal key — it is the name src/hooks.ts declares, so it has to be readable here", pos)
	}
	key := constant.StringVal(tv.Value)
	if key == "" {
		return nil, fmt.Errorf("skgo: %s: the transport key is empty", pos)
	}

	named, ok := inst.TypeArgs.At(0).(*types.Named)
	if !ok {
		return nil, fmt.Errorf("skgo: %s: skgo.Transported needs a named type; %s cannot be spelled by the generated codec", pos, inst.TypeArgs.At(0))
	}
	if _, isStruct := named.Underlying().(*types.Struct); !isStruct {
		return nil, fmt.Errorf("skgo: %s: %s is a named %s; a transported type has to be a struct, because its encoding is what the browser's decode receives", pos, named, kindWord(named.Underlying()))
	}

	return &transportedType{key: key, named: named, goPkg: gp, pos: pos}, nil
}

// checkTransportKeys refuses two markers claiming one key or one type. Either
// would make the wire ambiguous: kit looks a value up by its key, and skgo
// picks a transporter by the Go type.
func (a *app) checkTransportKeys(path string) error {
	byKey := map[string]*transportedType{}
	byType := map[string]*transportedType{}
	for _, entry := range a.transported {
		if first, dup := byKey[entry.key]; dup {
			return fmt.Errorf("skgo: %s: transport key %q is declared twice (%s and %s)", path, entry.key, first.named, entry.named)
		}
		byKey[entry.key] = entry
		id := entry.named.String()
		if first, dup := byType[id]; dup {
			return fmt.Errorf("skgo: %s: %s is transported twice, as %q and %q", path, entry.named, first.key, entry.key)
		}
		byType[id] = entry
	}
	return nil
}

// devalueCodecsFile is the file the polytype-generated codecs land in.
const devalueCodecsFile = "skgo_devalue_gen.go"

// generateTransportCodecs emits the Go encoder and strict decoder for every
// transported type, using polytype's devalue codegen.
//
// The codecs are generated rather than written by hand because the encoding is
// a contract with `src/hooks.ts`: whatever `EncodeMoney` produces is exactly
// what the browser's `decode` is handed, and whatever the browser's `encode`
// produces has to satisfy `DecodeMoney`. polytype already knows how to project
// a Go type into the devalue value model and how to reject a shape the type
// does not admit, with a JSON-pointer path in the diagnostic; reimplementing
// that here would be a second, worse answer to a question polytype has already
// answered.
func (a *app) generateTransportCodecs() error {
	if len(a.transported) == 0 {
		return nil
	}

	// Group by the directory polytype has to load, since a root is looked up
	// in a loaded package's scope.
	byDir := map[string][]*transportedType{}
	for _, entry := range a.transported {
		dir := entry.declDir()
		if dir == "" {
			return fmt.Errorf("skgo: %s: cannot find the source directory of %s", entry.pos, entry.named)
		}
		byDir[dir] = append(byDir[dir], entry)
	}

	dirs := make([]string, 0, len(byDir))
	for dir := range byDir {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)

	var defs typegrammarDefs
	for _, dir := range dirs {
		pkg, err := grammar.Load(dir)
		if err != nil {
			return fmt.Errorf("skgo: projecting transported types in %s: %w", dir, err)
		}
		var roots []grammar.Root
		entries := byDir[dir]
		sort.Slice(entries, func(i, j int) bool { return entries[i].key < entries[j].key })
		for _, entry := range entries {
			obj := pkg.Types().Scope().Lookup(entry.named.Obj().Name())
			if obj == nil {
				return fmt.Errorf("skgo: %s: %s is not declared in %s", entry.pos, entry.named, dir)
			}
			roots = append(roots, grammar.Root{Type: obj.Type(), Position: entry.pos})
		}
		lowered, rootNodes, err := pkg.Lower(roots)
		if err != nil {
			return fmt.Errorf("skgo: projecting transported types in %s: %w", dir, err)
		}
		defs.add(lowered, rootNodes)
	}

	out, err := codegen.Generate(defs.definitions, defs.roots, codegen.Options{
		PackageName: a.cfg.Package,
		ImportPath:  a.bindingsImportPath(),
	})
	if err != nil {
		return fmt.Errorf("skgo: generating devalue codecs: %w", err)
	}
	return a.writeGo(filepath.Join(a.cfg.Out, devalueCodecsFile), string(out))
}

// declDir is the directory the type's declaration lives in, which is what
// polytype's grammar loader is given.
func (e *transportedType) declDir() string {
	pos := e.goPkg.pkg.Fset.Position(e.named.Obj().Pos())
	if pos.Filename == "" {
		return ""
	}
	return filepath.Dir(pos.Filename)
}

// transportKeyOrder is the app's transport entries, sorted by key, so that
// every generated artefact lists them the same way.
func (a *app) transportKeyOrder() []*transportedType {
	out := append([]*transportedType(nil), a.transported...)
	sort.Slice(out, func(i, j int) bool { return out[i].key < out[j].key })
	return out
}

// codecName is the name polytype's codegen gives a definition's exported
// encoder and decoder: `Encode<Name>` and `Decode<Name>`.
func codecName(entry *transportedType) string {
	name := entry.named.Obj().Name()
	return strings.ToUpper(name[:1]) + name[1:]
}

// typegrammarDefs accumulates the lowered definitions of every directory that
// declares a transported type, into the single graph codegen is given.
// Definitions are keyed by their resolved name, so a type two directories both
// reach is added once.
type typegrammarDefs struct {
	definitions typegrammar.Definitions
	roots       []typegrammar.Type
	seen        map[typegrammar.Name]bool
}

func (d *typegrammarDefs) add(defs typegrammar.Definitions, roots []typegrammar.Type) {
	if d.seen == nil {
		d.seen = map[typegrammar.Name]bool{}
	}
	for _, def := range defs {
		if d.seen[def.Name] {
			continue
		}
		d.seen[def.Name] = true
		d.definitions = append(d.definitions, def)
	}
	d.roots = append(d.roots, roots...)
}

// bindingsImportPath is the import path of the generated bindings package,
// which codegen needs so that a type declared there is spelled unqualified.
func (a *app) bindingsImportPath() string {
	rel, err := filepath.Rel(a.hostDir, a.cfg.Out)
	if err != nil || rel == "." {
		return a.hostModule
	}
	return a.hostModule + "/" + filepath.ToSlash(rel)
}
