package gen

import (
	"fmt"
	"go/types"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/tylergannon/polytype/grammar"
	"github.com/tylergannon/polytype/typegrammar"
	"github.com/tylergannon/polytype/typescript"
)

// tsType is a TypeScript type expression plus the named Go types it depends
// on. skgo does not project Go types to TypeScript itself: every named type is
// projected by polytype, and skgo only spells the surrounding shape — a slice,
// a basic type — the same way polytype spells it.
type tsType struct {
	expr string
	deps []*types.Named
	// transported are the app's own custom types the expression names. They
	// are not projected by polytype — they are classes `src/hooks.ts`
	// declares — so a file using the expression imports them from there.
	transported []*types.Named
}

// skgoFilePkg is where skgo.File actually lives; the exported name is an
// alias, so go/types reports the underlying package.
const skgoFilePkg = "github.com/tylergannon/skgo/internal/formdata"

// project returns the TypeScript for t, or an error explaining why it cannot
// travel. The rules are polytype's, because polytype is what has to emit the
// declaration; skgo refuses the shapes polytype cannot express, and the ones it
// expresses unfaithfully, rather than papering over them.
func (a *app) project(t types.Type) (tsType, error) {
	return a.projectType(t, false)
}

// projectType is project, carrying whether a value the server has only promised
// may appear. It may inside a load's result — kit lets a promise sit anywhere
// in the object a load returns, and inside a value another promise carries —
// and nowhere else, because nothing else on the wire streams.
func (a *app) projectType(t types.Type, promises bool) (tsType, error) {
	if inner, ok := deferredElem(t); ok {
		if !promises {
			return tsType{}, fmt.Errorf("%s is a Deferred; only a load's result may promise a value", t)
		}
		projected, err := a.projectType(inner, true)
		if err != nil {
			return tsType{}, fmt.Errorf("a deferred value's type cannot cross: %v", err)
		}
		return tsType{expr: "Promise<" + projected.expr + ">", deps: projected.deps, transported: projected.transported}, nil
	}
	if !promises && containsDeferred(t) {
		return tsType{}, fmt.Errorf("%s holds a Deferred; only a load's result may promise a value", t)
	}

	// Since Go 1.23 an alias is its own node in the type graph rather than
	// the type it names, so `skgo.File` arrives here as a *types.Alias and
	// would fall past every case below. An alias is transparent by
	// definition, so resolving it is the whole handling it needs.
	t = types.Unalias(t)

	switch u := t.(type) {
	case *types.Named:
		obj := u.Obj()
		if obj.Pkg() == nil {
			return tsType{}, fmt.Errorf("%s is a builtin type and has no TypeScript projection", t)
		}
		// polytype accepts time.Time and renders it as an RFC3339 string.
		if obj.Pkg().Path() == "time" && obj.Name() == "Time" {
			return tsType{expr: "string"}, nil
		}
		// skgo.File is an alias for the internal type a form decodes an upload
		// into. It is not a struct to declare: on the client's side of the wire
		// it is the browser's own File, which is what `form.fields.x.as('file')`
		// puts in the FormData.
		if obj.Pkg().Path() == skgoFilePkg && obj.Name() == "File" {
			return tsType{expr: "File"}, nil
		}
		if _, isBasic := u.Underlying().(*types.Basic); isBasic {
			// A named basic type is either an enum, which polytype emits as a
			// union of literals, or a plain alias it emits as itself.
			return tsType{expr: obj.Name(), deps: []*types.Named{u}}, nil
		}
		// A transported type is not data on the client's side of the wire: it
		// is the class `src/hooks.ts` builds in its `decode`, methods and all.
		// polytype's TypeScript is structural by design and would describe the
		// class's fields without its behaviour, so the class is named here and
		// imported from the app's hooks instead of being declared.
		if _, transported := a.transportedNamed(u); transported {
			return tsType{expr: obj.Name(), transported: []*types.Named{u}}, nil
		}
		if st, isStruct := u.Underlying().(*types.Struct); isStruct {
			// A struct carrying an upload is written out inline instead of
			// being declared. polytype describes JSON, and a File is not a
			// JSON value — it is the browser's own File object, which kit's
			// client puts in the FormData and skgo's binary form decoder
			// carries to Go. Asking polytype to declare it fails outright
			// ("type byte not found" on the bytes), and any shape it could be
			// talked into would describe something the client never sends.
			// Same reasoning for a struct that carries a transported type: if
			// it were declared by polytype, its property would be typed as the
			// transported type's *fields* and a page calling a method on it
			// would not compile. Inlining keeps the reference to the class.
			// A struct holding a promised value is inlined for the third
			// time for the same reason: `Promise<T>` is not a projection of
			// any Go type, so polytype cannot declare one.
			if containsFile(u) || a.containsTransported(u) || (promises && containsDeferred(u)) {
				return a.inlineStruct(st, promises)
			}
			return tsType{expr: obj.Name(), deps: []*types.Named{u}}, nil
		}
		return tsType{}, fmt.Errorf("%s is a named %s; polytype declares named struct and enum types only", t, kindWord(u.Underlying()))

	case *types.Basic:
		switch {
		case u.Info()&types.IsBoolean != 0:
			return tsType{expr: "boolean"}, nil
		case u.Info()&types.IsString != 0:
			return tsType{expr: "string"}, nil
		case u.Info()&types.IsInteger != 0, u.Info()&types.IsFloat != 0:
			return tsType{expr: "number"}, nil
		}
		return tsType{}, fmt.Errorf("%s cannot travel over JSON", t)

	case *types.Slice:
		inner, err := a.projectType(u.Elem(), promises)
		if err != nil {
			return tsType{}, err
		}
		return tsType{expr: "Array<" + inner.expr + ">", deps: inner.deps, transported: inner.transported}, nil

	case *types.Array:
		inner, err := a.projectType(u.Elem(), promises)
		if err != nil {
			return tsType{}, err
		}
		return tsType{expr: "Array<" + inner.expr + ">", deps: inner.deps, transported: inner.transported}, nil

	case *types.Pointer:
		// polytype silently widens `*T` to a non-nullable `T`, while
		// encoding/json writes `null` for a nil pointer. Rather than emit a
		// declaration that lies, refuse the type.
		return tsType{}, fmt.Errorf("%s is a pointer: polytype projects it as a non-nullable %s, but encoding/json writes null for a nil one. Use the value type, or polytype.Nullable", t, u.Elem())

	case *types.Map:
		return tsType{}, fmt.Errorf("%s is a map: polytype does not project maps (mapType/chanType not allowed). Use a named struct", t)

	case *types.Interface:
		return tsType{}, fmt.Errorf("%s is an interface; a remote function's argument and result must be concrete", t)
	}
	return tsType{}, fmt.Errorf("%s has no TypeScript projection", t)
}

func kindWord(t types.Type) string {
	switch t.(type) {
	case *types.Map:
		return "map"
	case *types.Slice:
		return "slice"
	case *types.Interface:
		return "interface"
	case *types.Signature:
		return "function"
	}
	return "type"
}

// declare records that a named type must be projected by polytype, and where
// its declaration will end up.
func (a *app) declare(named *types.Named) error {
	pkg := named.Obj().Pkg()
	set, ok := a.typeSets[pkg]
	if !ok {
		dir, loadDir, err := a.dirOf(pkg)
		if err != nil {
			return err
		}
		foreign := !withinTree(a.hostDir, dir)
		if foreign {
			// The type's own package is source the app does not own — the
			// module cache, for any real consumer — so it is lowered through
			// the app package that imports it. polytype loads a root's
			// package on demand from the importer's dependency graph, which
			// is the same graph `go build` resolves.
			if loadDir, err = a.importerOf(pkg); err != nil {
				return err
			}
		}
		tsDir, err := a.typesDirFor(pkg, dir, foreign)
		if err != nil {
			return err
		}
		set = &namedTypes{pkg: pkg, loadDir: loadDir, seen: map[string]bool{}, tsDir: tsDir, foreign: foreign}
		a.typeSets[pkg] = set
	}
	if !set.seen[named.Obj().Name()] {
		set.seen[named.Obj().Name()] = true
		set.names = append(set.names, named.Obj().Name())
	}
	return nil
}

// importerOf is the load directory of an app package that imports pkg. A
// foreign type reaches the wire through a marked signature in one of the
// app's packages, and that package imports the type's, so one always exists.
func (a *app) importerOf(pkg *types.Package) (string, error) {
	for _, gp := range a.pkgs {
		for _, imp := range gp.pkg.Types.Imports() {
			if imp.Path() == pkg.Path() {
				return gp.loadDir, nil
			}
		}
	}
	return "", fmt.Errorf("skgo: %s puts a type on the wire, but no package declaring a remote function, load or route imports it", pkg.Path())
}

// withinTree reports whether dir is root or lives under it. It is the whole
// ownership rule: a package inside the app's own tree gets its declaration
// written beside its source, and every other package — a dependency in the
// module cache, a shared internal module, another repository — gets one
// written into the app instead. The rule never asks whether the foreign
// directory happens to be writable, because for any real consumer it is not,
// and a writable dependency is the case that hid this bug rather than the case
// that excuses it.
//
// It asks about the directory, not the module path. The route tree carries a
// `go.mod` of its own — the boundary that stops `go build ./...` walking into
// `[id]` — so a package the developer authored reports a module path that is
// not the app's while still being the app's source. Containment is what
// actually answers the question.
//
// `go list` reports a path with every symlink resolved, while the app's own
// root may still hold one — /var against /private/var is the everyday case —
// so a plain prefix comparison is not enough on its own.
func withinTree(root, dir string) bool {
	if dirContains(root, dir) {
		return true
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return false
	}
	return dirContains(realRoot, realDir)
}

func dirContains(root, dir string) bool {
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// dirOf resolves a types.Package to the directory holding its source and to
// the directory Go names it by. The two differ for a package under the route
// tree, which Go reaches only through its link.
func (a *app) dirOf(pkg *types.Package) (dir, loadDir string, err error) {
	for _, gp := range a.pkgs {
		if gp.pkg.Types == pkg {
			return gp.dir, gp.loadDir, nil
		}
	}
	cmd := exec.Command("go", "list", "-f", "{{.Dir}}", pkg.Path())
	cmd.Dir = a.hostDir
	cmd.Stderr = os.Stderr
	raw, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("skgo: locating the source of package %s: %w", pkg.Path(), err)
	}
	loadDir = strings.TrimSpace(string(raw))
	return a.links.authoredDir(loadDir), loadDir, nil
}

// typesDirFor decides where polytype writes a package's types.ts. A package
// that already lives under the vite root keeps its declarations beside its Go
// source; anything else gets a directory under `src/lib/skgo/`, because the
// stub that imports it has to be able to reach it.
//
// A foreign package is addressed by its import path there too, for the same
// reason its declaration package is: two dependencies called `wire` are an
// ordinary thing to have. The app's own packages keep the short name — the
// developer chose it, and one name per app is a rule they can act on.
func (a *app) typesDirFor(pkg *types.Package, dir string, foreign bool) (string, error) {
	if rel, err := filepath.Rel(filepath.Join(a.cfg.Web, "src"), dir); err == nil && !strings.HasPrefix(rel, "..") {
		return dir, nil
	}
	if foreign {
		return filepath.Join(a.cfg.Web, "src", "lib", "skgo", filepath.FromSlash(pkg.Path())), nil
	}
	for other, set := range a.typeSets {
		if other.Name() == pkg.Name() && other.Path() != pkg.Path() && !set.foreign && set.tsDir != dir {
			return "", fmt.Errorf("skgo: packages %s and %s are both named %q and both own types on the wire; rename one",
				other.Path(), pkg.Path(), pkg.Name())
		}
	}
	return filepath.Join(a.cfg.Web, "src", "lib", "skgo", pkg.Name()), nil
}

// generateTypes projects every named type on the wire to TypeScript, one
// types.ts per Go package that declares one. polytype is the projector — skgo
// never spells a named type itself — and it is driven as a library: skgo
// already knows every root, so nothing is discovered by marker, no file is
// written into the package that declares the types, and no schema exists.
func (a *app) generateTypes() error {
	for _, fn := range a.remotes {
		if fn.in != nil {
			t, err := a.project(fn.in)
			if err != nil {
				return fmt.Errorf("skgo: %s: the argument of %s cannot cross to TypeScript: %v", fn.pos, fn.name, err)
			}
			for _, dep := range t.deps {
				if err := a.declare(dep); err != nil {
					return err
				}
			}
		}
		t, err := a.project(fn.out)
		if err != nil {
			return fmt.Errorf("skgo: %s: the result of %s cannot cross to TypeScript: %v", fn.pos, fn.name, err)
		}
		for _, dep := range t.deps {
			if err := a.declare(dep); err != nil {
				return err
			}
		}
	}

	for _, set := range a.sortedTypeSets() {
		sort.Strings(set.names)
		if err := a.projectTypes(set); err != nil {
			return err
		}
	}
	return nil
}

func (a *app) sortedTypeSets() []*namedTypes {
	var out []*namedTypes
	for _, set := range a.typeSets {
		out = append(out, set)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].pkg.Path() < out[j].pkg.Path() })
	return out
}

// projectTypes lowers one package's wire types through polytype's grammar and
// writes the TypeScript it emits for them.
//
// The package is loaded by the address Go can resolve, never the authored
// path: a route directory called `[id]` is not something the package loader
// can be handed at all. Its roots are looked up by name in the loaded scope —
// its own for a package of the app's, the imported package's for a foreign
// one — so the types handed to polytype belong to the graph polytype loaded.
func (a *app) projectTypes(set *namedTypes) error {
	loaded, err := grammar.Load(set.loadDir)
	if err != nil {
		return fmt.Errorf("skgo: loading %s to project its types: %w", set.pkg.Path(), err)
	}
	scope := loaded.Types().Scope()
	if set.foreign {
		scope = nil
		for _, imp := range loaded.Types().Imports() {
			if imp.Path() == set.pkg.Path() {
				scope = imp.Scope()
				break
			}
		}
		if scope == nil {
			return fmt.Errorf("skgo: %s does not import %s, whose types it puts on the wire", set.loadDir, set.pkg.Path())
		}
	}

	roots := make([]grammar.Root, 0, len(set.names))
	for _, name := range set.names {
		obj := scope.Lookup(name)
		if obj == nil {
			return fmt.Errorf("skgo: %s is not declared in %s", name, set.pkg.Path())
		}
		roots = append(roots, grammar.Root{Type: obj.Type()})
	}
	defs, _, err := loaded.Lower(roots)
	if err != nil {
		return fmt.Errorf("skgo: polytype could not project the types in %s: %w", set.pkg.Path(), err)
	}
	result, err := typescript.Generate(defs, typescript.Options{})
	if err != nil {
		return fmt.Errorf("skgo: polytype could not declare the types in %s: %w", set.pkg.Path(), err)
	}

	// The stubs import a type by its Go name. polytype's identifiers are
	// collision-safe, so a root that reaches two types of one name from two
	// packages gets one of them renamed — and a stub importing the Go name
	// would then name the wrong declaration, or none. Refuse that rather than
	// emit it; the developer's fix is the one the stubs already demand for
	// two same-named types in one module.
	for _, name := range set.names {
		if result.Names[typegrammar.Name{PackagePath: set.pkg.Path(), Name: name}] != name {
			return fmt.Errorf("skgo: %s.%s reaches another type called %s from a different package; TypeScript can only have one %s in a module, so one of them has to be renamed",
				set.pkg.Path(), name, name, name)
		}
	}

	if err := os.MkdirAll(set.tsDir, 0o755); err != nil {
		return err
	}
	for _, file := range result.Files {
		path := filepath.Join(set.tsDir, file.Name)
		if err := a.write(path, string(file.Content)); err != nil {
			return err
		}
		a.cfg.Logf("projected the types in %s to %s", set.pkg.Path(), path)
	}
	return nil
}

// importSpecifier is the module specifier a stub uses to reach a package's
// generated types.ts. It is relative and extensionless, which vite and
// TypeScript both resolve to the `.ts` file.
func importSpecifier(fromDir, tsDir string) (string, error) {
	rel, err := filepath.Rel(fromDir, filepath.Join(tsDir, "types"))
	if err != nil {
		return "", err
	}
	spec := filepath.ToSlash(rel)
	if !strings.HasPrefix(spec, ".") {
		spec = "./" + spec
	}
	return spec, nil
}

// checkFileUsage refuses a skgo.File anywhere it cannot work.
//
// A File only crosses the wire in one direction, in one kind: the browser puts
// one in a FormData, and kit's binary form envelope carries its bytes to a
// form's argument. Nothing sends one back — a result is serialised through
// encoding/json, which would turn a File into an object with a base64 blob in
// it — and no other remote kind receives one, because a query's and a command's
// argument is decoded with a JSON round-trip that drops the bytes.
//
// Left unchecked, each of those is a silent wrong answer rather than a failure,
// which is exactly the shape of bug a generator should make impossible.
func (a *app) checkFileUsage() error {
	for _, fn := range a.remotes {
		if containsFile(fn.out) {
			return fmt.Errorf("skgo: %s: %s returns a skgo.File. A File travels from the browser to a form only; a result is serialised as JSON and cannot carry one", fn.pos, fn.name)
		}
		if fn.kind == kindForm || !containsFile(fn.in) {
			continue
		}
		return fmt.Errorf("skgo: %s: %s takes a skgo.File but is declared as a %s. Only a form receives an upload — kit posts a form as multipart-equivalent binary data, while a %s argument is a JSON-compatible devalue payload", fn.pos, fn.name, fn.kind, fn.kind)
	}
	for _, load := range a.loads {
		if containsFile(load.out) {
			return fmt.Errorf("skgo: %s: %s returns a skgo.File, which cannot be serialised into a load's data", load.pos, load.name)
		}
	}
	return nil
}

// containsFile reports whether t is, or structurally contains, a skgo.File.
func containsFile(t types.Type) bool {
	return findFile(t, map[types.Type]bool{})
}

func findFile(t types.Type, seen map[types.Type]bool) bool {
	if t == nil {
		return false
	}
	// `skgo.File` is an alias, and an alias is a distinct node in go/types.
	t = types.Unalias(t)
	if seen[t] {
		return false
	}
	seen[t] = true

	switch u := t.(type) {
	case *types.Named:
		if obj := u.Obj(); obj.Pkg() != nil && obj.Pkg().Path() == skgoFilePkg && obj.Name() == "File" {
			return true
		}
		return findFile(u.Underlying(), seen)
	case *types.Struct:
		for i := 0; i < u.NumFields(); i++ {
			if findFile(u.Field(i).Type(), seen) {
				return true
			}
		}
	case *types.Slice:
		return findFile(u.Elem(), seen)
	case *types.Array:
		return findFile(u.Elem(), seen)
	case *types.Pointer:
		return findFile(u.Elem(), seen)
	case *types.Map:
		return findFile(u.Elem(), seen)
	}
	return false
}

// inlineStruct renders a struct as a TypeScript object literal, for the one
// case that cannot be a declared type: a form's argument with a file in it.
//
// The field names are encoding/json's, because that is what the runtime
// decoder matches against, and a file field is optional because an
// `<input type="file">` the visitor left alone sends nothing at all.
func (a *app) inlineStruct(st *types.Struct, promises bool) (tsType, error) {
	var (
		parts       []string
		deps        []*types.Named
		transported []*types.Named
	)
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		if !f.Exported() {
			continue
		}
		tag := reflect.StructTag(st.Tag(i)).Get("json")
		name, opts, _ := strings.Cut(tag, ",")
		if name == "-" && opts == "" {
			continue
		}
		if name == "" {
			name = f.Name()
		}

		inner, err := a.projectType(f.Type(), promises)
		if err != nil {
			return tsType{}, fmt.Errorf("field %s: %w", f.Name(), err)
		}
		deps = append(deps, inner.deps...)
		transported = append(transported, inner.transported...)

		optional := ""
		if inner.expr == "File" || strings.Contains(opts, "omitempty") {
			optional = "?"
		}
		parts = append(parts, fmt.Sprintf("%s%s: %s", name, optional, inner.expr))
	}
	if len(parts) == 0 {
		return tsType{expr: "Record<string, never>"}, nil
	}
	return tsType{expr: "{ " + strings.Join(parts, "; ") + " }", deps: deps, transported: transported}, nil
}

// transportedNamed reports whether named is one of the app's transported types.
func (a *app) transportedNamed(named *types.Named) (*transportedType, bool) {
	if named == nil || named.Obj() == nil {
		return nil, false
	}
	for _, entry := range a.transported {
		if entry.named.Obj() == named.Obj() {
			return entry, true
		}
	}
	return nil, false
}

// containsTransported reports whether t reaches one of the app's transported
// types, which is what decides that a struct is inlined rather than declared.
func (a *app) containsTransported(t types.Type) bool {
	if len(a.transported) == 0 {
		return false
	}
	return a.findTransported(t, map[types.Type]bool{})
}

func (a *app) findTransported(t types.Type, seen map[types.Type]bool) bool {
	if t == nil {
		return false
	}
	t = types.Unalias(t)
	if seen[t] {
		return false
	}
	seen[t] = true

	switch u := t.(type) {
	case *types.Named:
		if _, ok := a.transportedNamed(u); ok {
			return true
		}
		return a.findTransported(u.Underlying(), seen)
	case *types.Struct:
		for i := range u.NumFields() {
			if a.findTransported(u.Field(i).Type(), seen) {
				return true
			}
		}
	case *types.Slice:
		return a.findTransported(u.Elem(), seen)
	case *types.Array:
		return a.findTransported(u.Elem(), seen)
	case *types.Pointer:
		return a.findTransported(u.Elem(), seen)
	}
	return false
}
