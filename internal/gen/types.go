package gen

import (
	"fmt"
	"go/types"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"golang.org/x/mod/module"
)

// noneType is the argument type of a remote function that takes no argument.
const noneType = skgoPkg + ".None"

// relocatedRootName is the directory, inside the generated bindings package,
// that holds the declaration packages skgo writes for types belonging to
// modules the app does not own.
const relocatedRootName = "wiretypes"

// declarationsFileName is the untagged file in a declaration package that
// gives a foreign package's types a local Go declaration. It is also how a
// declaration package is recognised when a later run has to remove it.
const declarationsFileName = "skgo_wiretypes_gen.go"

// tsType is a TypeScript type expression plus the named Go types it depends
// on. skgo does not project Go types to TypeScript itself: every named type is
// projected by polytype, and skgo only spells the surrounding shape — a slice,
// a basic type — the same way polytype spells it.
type tsType struct {
	expr string
	deps []*types.Named
}

// skgoFilePkg is where skgo.File actually lives; the exported name is an
// alias, so go/types reports the underlying package.
const skgoFilePkg = "github.com/tylergannon/skgo/internal/formdata"

// project returns the TypeScript for t, or an error explaining why it cannot
// travel. The rules are polytype's, because polytype is what has to emit the
// declaration; skgo refuses the shapes polytype cannot express, and the ones it
// expresses unfaithfully, rather than papering over them.
func project(t types.Type) (tsType, error) {
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
		if st, isStruct := u.Underlying().(*types.Struct); isStruct {
			// A struct carrying an upload is written out inline instead of
			// being declared. polytype describes JSON, and a File is not a
			// JSON value — it is the browser's own File object, which kit's
			// client puts in the FormData and skgo's binary form decoder
			// carries to Go. Asking polytype to declare it fails outright
			// ("type byte not found" on the bytes), and any shape it could be
			// talked into would describe something the client never sends.
			if containsFile(u) {
				return inlineStruct(st)
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
		inner, err := project(u.Elem())
		if err != nil {
			return tsType{}, err
		}
		return tsType{expr: "Array<" + inner.expr + ">", deps: inner.deps}, nil

	case *types.Array:
		inner, err := project(u.Elem())
		if err != nil {
			return tsType{}, err
		}
		return tsType{expr: "Array<" + inner.expr + ">", deps: inner.deps}, nil

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

// isNone reports whether t is skgo.None, the argument type of a remote
// function that takes no argument.
func isNone(t types.Type) bool {
	named, ok := t.(*types.Named)
	if !ok || named.Obj().Pkg() == nil {
		return false
	}
	return named.Obj().Pkg().Path()+"."+named.Obj().Name() == noneType
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
			// The declaration cannot go where the type lives, so it comes to
			// the app instead. dir and loadDir become the same real directory:
			// nothing about it needs the route tree's links.
			if dir, err = a.declarationDirFor(pkg); err != nil {
				return err
			}
			loadDir = dir
		}
		tsDir, err := a.typesDirFor(pkg, dir, foreign)
		if err != nil {
			return err
		}
		set = &namedTypes{pkg: pkg, dir: dir, loadDir: loadDir, seen: map[string]bool{}, tsDir: tsDir, foreign: foreign}
		a.typeSets[pkg] = set
	}
	if !set.seen[named.Obj().Name()] {
		set.seen[named.Obj().Name()] = true
		set.names = append(set.names, named.Obj().Name())
	}
	return nil
}

// declarationDirFor is where a foreign package's declaration package is
// written: a directory of the app's own module, named after the foreign
// package's import path.
//
// Inside `Out` because everything there is already skgo's disposable output —
// it is where the link tree lives — and because Out is documented to be in the
// same module as the remote functions, which is what makes the directory
// compilable, nameable by Go, and a place `go:embed` can reach. It is
// deliberately not under the route tree, whose own `go.mod` would put it in a
// different module.
//
// The import path is the directory name, not the package name. Two
// dependencies called `wire` are an ordinary thing to have, and their import
// paths are the only names that are guaranteed to differ.
func (a *app) declarationDirFor(pkg *types.Package) (string, error) {
	// Out is inside the host module by construction: it is the directory the
	// host module was resolved from.
	out, err := filepath.Rel(a.hostDir, a.cfg.Out)
	if err != nil {
		return "", err
	}
	rel := path.Join(filepath.ToSlash(out), relocatedRootName, pkg.Path())
	if err := module.CheckImportPath(a.hostModule + "/" + rel); err != nil {
		return "", fmt.Errorf("skgo: %s puts a type on the wire, but Go cannot name a directory after its import path: %w", pkg.Path(), err)
	}
	return filepath.Join(a.hostDir, filepath.FromSlash(rel)), nil
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

// generateTypes writes the polytype declaration file for every package that
// owns a type on the wire, then runs polytype over it. polytype is a code
// generator skgo runs and consumes; skgo never projects a named type itself.
func (a *app) generateTypes() error {
	for _, fn := range a.remotes {
		if !isNone(fn.in) {
			t, err := project(fn.in)
			if err != nil {
				return fmt.Errorf("skgo: %s: the argument of %s cannot cross to TypeScript: %v", fn.pos, fn.name, err)
			}
			for _, dep := range t.deps {
				if err := a.declare(dep); err != nil {
					return err
				}
			}
		}
		t, err := project(fn.out)
		if err != nil {
			return fmt.Errorf("skgo: %s: the result of %s cannot cross to TypeScript: %v", fn.pos, fn.name, err)
		}
		for _, dep := range t.deps {
			if err := a.declare(dep); err != nil {
				return err
			}
		}
	}

	sets := a.sortedTypeSets()
	if err := a.pruneDeclarationPackages(sets); err != nil {
		return err
	}
	for _, set := range sets {
		sort.Strings(set.names)
		if err := a.writePolytypeMarkers(set); err != nil {
			return err
		}
	}
	// The registration files just landed in authored directories, one of which
	// may be the route root, whose link is a directory of per-file symlinks.
	// polytype has to be able to see them through the link.
	if err := a.links.sync(); err != nil {
		return err
	}

	for _, set := range sets {
		if err := os.MkdirAll(set.tsDir, 0o755); err != nil {
			return err
		}
		// --target is the address Go can resolve, never the authored path: a
		// route directory called `[id]` is not something the package loader can
		// be handed at all.
		cmd := exec.Command("go", "tool", "polytype", "--typescript", set.tsDir, "--target", set.loadDir)
		cmd.Dir = a.hostDir
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("skgo: polytype could not project the types in %s: %w", set.pkg.Path(), err)
		}
		a.cfg.Logf("projected the types in %s to %s", set.pkg.Path(), filepath.Join(set.tsDir, "types.ts"))
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

// writePolytypeMarkers emits the registration file polytype expects the
// developer to write. It has to live in the package that declares the type and
// be behind the `jsonschema` build tag, so it never reaches the production
// binary. For a foreign package that means the declaration package skgo wrote
// into the app, and the local types it declares.
func (a *app) writePolytypeMarkers(set *namedTypes) error {
	if set.foreign {
		if err := a.writeLocalDeclarations(set); err != nil {
			return err
		}
	}
	var b strings.Builder
	b.WriteString("//go:build jsonschema\n\n")
	b.WriteString("// Code generated by skgo. DO NOT EDIT.\n\n")
	fmt.Fprintf(&b, "package %s\n\n", set.pkg.Name())
	b.WriteString("import (\n\t\"encoding/json\"\n\n\t\"github.com/tylergannon/polytype\"\n)\n\n")
	b.WriteString("// Stubs so the package compiles before polytype has run.\n")
	for _, name := range set.names {
		fmt.Fprintf(&b, "func (%s) Schema() json.RawMessage { panic(\"not implemented\") }\n", set.localName(name))
	}
	b.WriteString("\nvar (\n")
	for _, name := range set.names {
		fmt.Fprintf(&b, "\t_ = polytype.Declare(%s.Schema)\n", set.localName(name))
	}
	b.WriteString(")\n")
	return a.writeGo(filepath.Join(set.dir, "skgo_polytype_gen.go"), b.String())
}

// writeLocalDeclarations gives a foreign package's types a local declaration
// polytype will accept, in the declaration package skgo writes into the app.
//
// polytype requires the type it is asked to project to be declared in the
// target package (`undeclared local type found`), so there has to be a local
// declaration whatever else is true. It is a defined type rather than an alias
// because polytype emits its own entrypoint as a method on that type — even
// for a registration written as a free function — and Go forbids a method on
// an alias of a type from another package: the generated `jsonschema_gen.go`
// simply does not compile.
//
// Losing the foreign method set across the definition costs nothing, and the
// reason is worth knowing: polytype does not honour a custom marshaller, it
// refuses the type outright — `rejectCustomWireType`, "defines MarshalJSON;
// custom JSON/text wire mappings are not statically derivable" — and it
// resolves *through* a defined type to find one, naming the underlying type in
// the refusal. So a type whose Go encoding would contradict its declaration
// cannot be smuggled past by relocating it. Measured against pinned rc.9, not
// inferred. Otherwise the projection is structural, so both forms emit the
// same TypeScript, and skgo's runtime encodes the foreign type itself and
// never touches this declaration. The defined form also carries a foreign
// enum, where an alias inherits the `enum()` marker without the constants that
// give it meaning and polytype stops.
//
// This file carries no build tag. polytype's own output is `//go:build
// !jsonschema` and refers to these names, so a declaration behind the
// `jsonschema` tag would leave an ordinary build broken.
//
// The local name is not the foreign one. With both in scope polytype has two
// types called `Thing` and disambiguates by hashing the identifier into the
// TypeScript; with a distinct one it emits the foreign type under its own
// name, which is the name the stubs use.
func (a *app) writeLocalDeclarations(set *namedTypes) error {
	var b strings.Builder
	b.WriteString("// Code generated by skgo. DO NOT EDIT.\n//\n")
	fmt.Fprintf(&b, "// Local declarations of the types %s puts on the wire, so polytype has\n", set.pkg.Path())
	b.WriteString("// something in this package to project. Nothing but polytype reads them.\n\n")
	fmt.Fprintf(&b, "package %s\n\n", set.pkg.Name())
	fmt.Fprintf(&b, "import %s %q\n\n", set.pkg.Name(), set.pkg.Path())
	for _, name := range set.names {
		fmt.Fprintf(&b, "type %s %s.%s\n", set.localName(name), set.pkg.Name(), name)
	}
	return a.writeGo(filepath.Join(set.dir, declarationsFileName), b.String())
}

// pruneDeclarationPackages removes declaration packages a previous run left
// behind. They are ordinary Go source in the app's module, so one for a
// dependency that has since been dropped is a build failure rather than
// clutter.
func (a *app) pruneDeclarationPackages(sets []*namedTypes) error {
	root := filepath.Join(a.cfg.Out, relocatedRootName)
	keep := map[string]bool{}
	for _, set := range sets {
		if set.foreign {
			keep[set.dir] = true
		}
	}
	var stale []string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != declarationsFileName {
			return nil
		}
		if dir := filepath.Dir(p); !keep[dir] {
			stale = append(stale, dir)
		}
		return nil
	})
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, dir := range stale {
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
		a.cfg.Logf("removed %s: nothing on the wire comes from it any more", dir)
	}
	return removeEmptyDirs(root)
}

// removeEmptyDirs deletes every empty directory under root, and root itself if
// it ends up empty. An import path is a directory tree, so dropping one
// dependency's declaration package can leave several levels of nothing.
func removeEmptyDirs(root string) error {
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			if err := removeEmptyDirs(filepath.Join(root, e.Name())); err != nil {
				return err
			}
		}
	}
	if rest, err := os.ReadDir(root); err == nil && len(rest) == 0 {
		return os.Remove(root)
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
func inlineStruct(st *types.Struct) (tsType, error) {
	var (
		parts []string
		deps  []*types.Named
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

		inner, err := project(f.Type())
		if err != nil {
			return tsType{}, fmt.Errorf("field %s: %w", f.Name(), err)
		}
		deps = append(deps, inner.deps...)

		optional := ""
		if inner.expr == "File" || strings.Contains(opts, "omitempty") {
			optional = "?"
		}
		parts = append(parts, fmt.Sprintf("%s%s: %s", name, optional, inner.expr))
	}
	if len(parts) == 0 {
		return tsType{expr: "Record<string, never>"}, nil
	}
	return tsType{expr: "{ " + strings.Join(parts, "; ") + " }", deps: deps}, nil
}
