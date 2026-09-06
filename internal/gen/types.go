package gen

import (
	"fmt"
	"go/types"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// noneType is skgo's marker for nothing on the wire: the argument of a remote
// function that takes none, and the result of one that returns none.
const noneType = skgoPkg + ".None"

// tsType is a TypeScript type expression plus the named Go types it depends
// on. skgo does not project Go types to TypeScript itself: every named type is
// projected by polytype, and skgo only spells the surrounding shape — a slice,
// a basic type — the same way polytype spells it.
type tsType struct {
	expr string
	deps []*types.Named
}

// project returns the TypeScript for t, or an error explaining why it cannot
// travel. The rules are polytype's, because polytype is what has to emit the
// declaration; skgo refuses the shapes polytype cannot express, and the ones it
// expresses unfaithfully, rather than papering over them.
func project(t types.Type) (tsType, error) {
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
		// skgo.None is skgo's own marker for "nothing on the wire", not a type
		// the app declares. As an argument it is kit's no-argument overload; as
		// a result it is a remote function that returns nothing, which kit
		// types as void. Either way there is nothing for polytype to declare —
		// and declaring it would mean writing into skgo's own module.
		if isNone(u) {
			return tsType{expr: "void"}, nil
		}
		if _, isBasic := u.Underlying().(*types.Basic); isBasic {
			// A named basic type is either an enum, which polytype emits as a
			// union of literals, or a plain alias it emits as itself.
			return tsType{expr: obj.Name(), deps: []*types.Named{u}}, nil
		}
		if _, isStruct := u.Underlying().(*types.Struct); isStruct {
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

// isNone reports whether t is skgo.None.
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
		if err := a.mustOwn(named, dir); err != nil {
			return err
		}
		tsDir, err := a.typesDirFor(pkg, dir)
		if err != nil {
			return err
		}
		set = &namedTypes{pkg: pkg, dir: dir, loadDir: loadDir, seen: map[string]bool{}, tsDir: tsDir}
		a.typeSets[pkg] = set
	}
	if !set.seen[named.Obj().Name()] {
		set.seen[named.Obj().Name()] = true
		set.names = append(set.names, named.Obj().Name())
	}
	return nil
}

// mustOwn refuses a named type whose package belongs to a module other than
// the app's.
//
// Projecting a type is not something skgo can do at arm's length: polytype's
// registration is a method on the type, so it has to be a Go file in the type's
// own package, and polytype then writes its `jsonschema/` output beside it.
// For a dependency that directory is the module cache, which is read-only — and
// in a workspace where it happens to be writable, generating would silently
// edit source the app does not own. Either way the answer is the same one skgo
// gives a pointer or a map: say what cannot travel, and why, rather than emit
// something that breaks on the next machine.
//
// Ownership is a question about the directory, not about the module path. The
// route tree carries a `go.mod` of its own — the boundary that stops `go build
// ./...` walking into `[id]` — so a package the developer authored can report a
// module path that is not the app's while still being the app's source.
func (a *app) mustOwn(named *types.Named, dir string) error {
	if withinTree(a.hostDir, dir) {
		return nil
	}
	owner := "a module this app does not own"
	if _, mod, err := moduleOf(dir); err == nil {
		owner = "module " + mod
	}
	pkg := named.Obj().Pkg()
	return fmt.Errorf("%s.%s is declared in %s, and projecting it means writing skgo_polytype_gen.go "+
		"and a jsonschema/ directory into %s — source this app does not own, and read-only in the module cache. "+
		"Declare the type in %s and convert to it in the remote function",
		pkg.Name(), named.Obj().Name(), owner, dir, a.hostModule)
}

// withinTree reports whether dir is root or lives under it. `go list` reports a
// path with every symlink resolved, while the app's own root may still hold
// one — /var against /private/var is the everyday case — so a plain prefix
// comparison is not enough on its own.
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
func (a *app) typesDirFor(pkg *types.Package, dir string) (string, error) {
	if rel, err := filepath.Rel(filepath.Join(a.cfg.Web, "src"), dir); err == nil && !strings.HasPrefix(rel, "..") {
		return dir, nil
	}
	for other, set := range a.typeSets {
		if other.Name() == pkg.Name() && other.Path() != pkg.Path() && set.tsDir != dir {
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
		if err := a.declareAll(fn.in); err != nil {
			return fmt.Errorf("skgo: %s: the argument of %s cannot cross to TypeScript: %v", fn.pos, fn.name, err)
		}
		if err := a.declareAll(fn.out); err != nil {
			return fmt.Errorf("skgo: %s: the result of %s cannot cross to TypeScript: %v", fn.pos, fn.name, err)
		}
	}

	sets := a.sortedTypeSets()
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

// declareAll projects t and records every named type it needs. The argument
// and the result go through the same door: a type skgo cannot declare is
// refused wherever it appears, and skgo.None is nothing to declare in either
// position.
func (a *app) declareAll(t types.Type) error {
	projected, err := project(t)
	if err != nil {
		return err
	}
	for _, dep := range projected.deps {
		if err := a.declare(dep); err != nil {
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

// writePolytypeMarkers emits the registration file polytype expects the
// developer to write. It has to live in the type's own package and be behind
// the `jsonschema` build tag, so it never reaches the production binary.
func (a *app) writePolytypeMarkers(set *namedTypes) error {
	var b strings.Builder
	b.WriteString("//go:build jsonschema\n\n")
	b.WriteString("// Code generated by skgo. DO NOT EDIT.\n\n")
	fmt.Fprintf(&b, "package %s\n\n", set.pkg.Name())
	b.WriteString("import (\n\t\"encoding/json\"\n\n\t\"github.com/tylergannon/polytype\"\n)\n\n")
	b.WriteString("// Stubs so the package compiles before polytype has run.\n")
	for _, name := range set.names {
		fmt.Fprintf(&b, "func (%s) Schema() json.RawMessage { panic(\"not implemented\") }\n", name)
	}
	b.WriteString("\nvar (\n")
	for _, name := range set.names {
		fmt.Fprintf(&b, "\t_ = polytype.Declare(%s.Schema)\n", name)
	}
	b.WriteString(")\n")
	return a.writeGo(filepath.Join(set.dir, "skgo_polytype_gen.go"), b.String())
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
