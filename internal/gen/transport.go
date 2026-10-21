package gen

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

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
	if symbol := p.Types.Scope().Lookup("Reroute"); symbol != nil {
		core := p.Imports[skgoPkg]
		if core == nil || core.Types.Scope().Lookup("Reroute") == nil || !types.ConvertibleTo(symbol.Type(), core.Types.Scope().Lookup("Reroute").Type()) {
			return fmt.Errorf("skgo: %s: Reroute must be convertible to skgo.Reroute", path)
		}
		a.reroute = gp
	}
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

// transportEntryPayload uses the codec's lowered grammar to recognize a time
// leaf, including a named type defined from time.Time. Its unexported Go
// fields are not its wire shape: the codec sends an RFC3339 string.
func (a *app) transportEntryPayload(entry *transportedType) (tsType, error) {
	loaded, err := a.loadGrammar(entry.goPkg.loadDir)
	if err != nil {
		return tsType{}, err
	}
	definitions, roots, err := loaded.Lower([]grammar.Root{{Type: entry.named, Position: entry.pos}})
	if err != nil {
		return tsType{}, err
	}
	byName := make(map[typegrammar.Name]typegrammar.Type, len(definitions))
	for _, definition := range definitions {
		byName[definition.Name] = definition.Type
	}
	node := roots[0]
	for {
		ref, ok := node.(*typegrammar.Ref)
		if !ok {
			break
		}
		next, found := byName[ref.Target]
		if !found {
			return tsType{}, fmt.Errorf("transport payload references missing type %s", ref.Target)
		}
		node = next
	}
	if _, isTime := node.(*typegrammar.Time); isTime {
		return tsType{expr: "string"}, nil
	}
	return a.transportPayload(entry.named)
}

// transportPayload is the TypeScript for a transported type's encoding: the
// object polytype's encoder makes of the struct, which is what the browser's
// `decode` receives and what its `encode` has to return for Go's decoder.
//
// It is written out rather than declared. polytype would declare it under the
// Go type's name, and the class src/hooks.ts builds already has that name, so
// every diagnostic would read `Transporter<Money, Money>`.
//
// The fields are encoding/json's, which polytype's codec follows: an embedded
// struct's fields are promoted, a shallower field hides a deeper one of the
// same name, and of several at one depth only a single tagged one is sent. A
// transported type inside another one is refused: the codec writes the inner
// value as plain fields, so the class src/hooks.ts declares for it would not
// describe what the browser gets.
func (a *app) transportPayload(named *types.Named) (tsType, error) {
	type field struct {
		name     string
		depth    int
		tagged   bool
		optional bool
		typ      types.Type
		via      string
	}
	var fields []field
	seen := map[types.Type]bool{}
	var walk func(st *types.Struct, depth int, via string) error
	walk = func(st *types.Struct, depth int, via string) error {
		for i := range st.NumFields() {
			f := st.Field(i)
			name, opts, _ := strings.Cut(reflect.StructTag(st.Tag(i)).Get("json"), ",")
			if name == "-" && opts == "" {
				continue
			}
			if f.Anonymous() && name == "" {
				embedded := types.Unalias(f.Type())
				if _, isPointer := embedded.(*types.Pointer); isPointer {
					return fmt.Errorf("it embeds %s, a pointer; embed the struct itself", embedded)
				}
				if inner, isStruct := embedded.Underlying().(*types.Struct); isStruct {
					if seen[embedded] {
						continue
					}
					seen[embedded] = true
					if err := walk(inner, depth+1, via+f.Name()+"."); err != nil {
						return err
					}
					continue
				}
			}
			if !f.Exported() {
				continue
			}
			if a.containsTransported(f.Type()) {
				return fmt.Errorf("its field %s%s holds a transported type; polytype encodes it inside %s as plain fields, so the browser would not receive the class. Give the field a plain type, or send the inner value on its own", via, f.Name(), named.Obj().Name())
			}
			_, isOptional := optionalElem(f.Type())
			optional := isOptional || strings.Contains(opts, "omitempty") || strings.Contains(opts, "omitzero")
			tagged := name != ""
			if !tagged {
				name = f.Name()
			}
			fields = append(fields, field{name: name, depth: depth, tagged: tagged, optional: optional, typ: f.Type(), via: via + f.Name()})
		}
		return nil
	}
	st, ok := named.Underlying().(*types.Struct)
	if !ok {
		return tsType{}, fmt.Errorf("it is not a struct")
	}
	seen[named] = true
	if err := walk(st, 0, ""); err != nil {
		return tsType{}, err
	}

	var (
		parts []string
		deps  []*types.Named
	)
	shallowest := map[string]int{}
	for _, f := range fields {
		if depth, ok := shallowest[f.name]; !ok || f.depth < depth {
			shallowest[f.name] = f.depth
		}
	}
	for _, f := range fields {
		if f.depth > shallowest[f.name] {
			continue
		}
		// Several fields at the shallowest depth: a single tagged one wins, and
		// otherwise encoding/json sends none of them.
		hidden := false
		for _, other := range fields {
			if other.name == f.name && other.via != f.via && other.depth == f.depth && (other.tagged || !f.tagged) {
				hidden = true
			}
		}
		if hidden {
			continue
		}
		inner, err := a.project(f.typ)
		if err != nil {
			return tsType{}, fmt.Errorf("field %s: %w", f.via, err)
		}
		deps = append(deps, inner.deps...)
		optional := ""
		if f.optional {
			optional = "?"
		}
		parts = append(parts, fmt.Sprintf("%s%s: %s", f.name, optional, inner.expr))
	}
	if len(parts) == 0 {
		return tsType{expr: "Record<string, never>"}, nil
	}
	return tsType{expr: "{ " + strings.Join(parts, "; ") + " }", deps: deps}, nil
}

// transportModuleName is the module skgo writes beside src/hooks.ts: the
// `transport` hook as src/hooks.go declares it.
const transportModuleName = "transport"

// writeTransportDeclaration writes src/transport.ts (or .js): the type the
// app's `transport` hook must satisfy, built from the markers in src/hooks.go.
//
// Kit has one transport object, so its keys and payloads cannot disagree. skgo
// has two halves in two languages, and nothing at runtime says when they
// drift: a renamed key is a 500 on the first page that carries one, and a
// renamed field is no error at all — `decode` reads `undefined` and the page
// renders it. So the Go half is stated with kit's own Transporter type, and
// the file also checks src/hooks.ts's export against it, which makes the
// agreement something svelte-check enforces rather than a convention.
//
// The type is kit's Transport intersected with the Go keys rather than an
// exact object. Kit also runs snapshots and history state through the hook, so
// a key only the browser uses is legitimate.
func (a *app) writeTransportDeclaration() error {
	src := filepath.Join(a.cfg.Web, "src")
	path := filepath.Join(src, transportModuleName+a.cfg.Language.ext())
	other := ".js"
	if a.cfg.Language.JavaScript() {
		other = ".ts"
	}
	if err := removeGeneratedArtifact(a.cfg, filepath.Join(src, transportModuleName+other)); err != nil {
		return err
	}
	if len(a.transported) == 0 {
		return removeGeneratedArtifact(a.cfg, path)
	}
	if old, err := os.ReadFile(path); err == nil && !generatedArtifact(old) {
		return fmt.Errorf("skgo: %s is authored; src/hooks.go declares transported types, and skgo writes their contract to that file. Rename yours", path)
	}
	content, err := a.transportDeclaration(src)
	if err != nil {
		return err
	}
	return a.write(path, content)
}

// transportDeclaration is the content of src/transport.ts for a module in dir.
func (a *app) transportDeclaration(dir string) (string, error) {
	hooks, err := a.hooksSpecifier(dir)
	if err != nil {
		return "", err
	}
	entries := a.transportKeyOrder()
	var classes []string
	imports := map[string][]string{}
	for _, entry := range entries {
		classes = appendUnique(classes, entry.named.Obj().Name())
		for _, dep := range entry.payload.deps {
			spec, err := importSpecifier(dir, a.typeSets[dep.Obj().Pkg()].tsDir)
			if err != nil {
				return "", err
			}
			imports[spec] = appendUnique(imports[spec], dep.Obj().Name())
		}
	}
	sort.Strings(classes)
	var specs []string
	for spec := range imports {
		specs = append(specs, spec)
		sort.Strings(imports[spec])
	}
	sort.Strings(specs)

	const about = "/**\n" +
		" * Transport is the `transport` hook src/hooks.go declares: each key Go\n" +
		" * encodes a value under, the class src/hooks.ts rebuilds, and the payload\n" +
		" * Go sends for it. Declare the hook `satisfies Transport` so TypeScript\n" +
		" * checks it against Go. Keys only the browser uses are allowed.\n"
	const agreement = "/**\n" +
		" * Agreement does not type-check when src/hooks.ts exports no `transport`, or\n" +
		" * one that disagrees with src/hooks.go: a key missing or misspelled, or a\n" +
		" * payload Go does not send.\n"
	var b strings.Builder
	b.WriteString(tsHeader)
	if a.cfg.Language.JavaScript() {
		ext := a.cfg.Language.ext()
		b.WriteString("\n")
		for _, name := range classes {
			fmt.Fprintf(&b, "/** @typedef {import('%s%s').%s} %s */\n", hooks, ext, name, name)
		}
		for _, spec := range specs {
			for _, name := range imports[spec] {
				fmt.Fprintf(&b, "/** @typedef {import('%s%s').%s} %s */\n", spec, ext, name, name)
			}
		}
		var members []string
		for _, entry := range entries {
			members = append(members, fmt.Sprintf("%q: import('@sveltejs/kit/hooks').Transporter<%s, %s>", entry.key, entry.named.Obj().Name(), entry.payload.expr))
		}
		b.WriteString("\n" + about)
		fmt.Fprintf(&b, " * @typedef {import('@sveltejs/kit/hooks').Transport & { %s }} Transport\n */\n", strings.Join(members, "; "))
		b.WriteString("\n" + agreement)
		fmt.Fprintf(&b, " * @typedef {Satisfies<typeof import('%s%s').transport>} Agreement\n */\n", hooks, ext)
		b.WriteString("\n/**\n * @template {Transport} T\n * @typedef {T} Satisfies\n */\n\nexport {};\n")
		return b.String(), nil
	}
	b.WriteString("import type { Transport as KitTransport, Transporter } from '@sveltejs/kit/hooks';\n")
	fmt.Fprintf(&b, "import type { %s, transport } from '%s';\n", strings.Join(classes, ", "), hooks)
	for _, spec := range specs {
		fmt.Fprintf(&b, "import type { %s } from '%s';\n", strings.Join(imports[spec], ", "), spec)
	}
	b.WriteString("\n" + about + " */\nexport type Transport = KitTransport & {\n")
	for _, entry := range entries {
		fmt.Fprintf(&b, "\t%q: Transporter<%s, %s>;\n", entry.key, entry.named.Obj().Name(), entry.payload.expr)
	}
	b.WriteString("};\n\n" + agreement + " */\nexport type Agreement = Satisfies<typeof transport>;\n\ntype Satisfies<T extends Transport> = T;\n")
	return b.String(), nil
}
