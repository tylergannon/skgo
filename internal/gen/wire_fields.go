package gen

import (
	"fmt"
	"go/token"
	"go/types"
	"reflect"
	"strings"
)

// checkWireFields catches ambiguous JSON names and unsupported nested fields
// from the currently authored type graph before generation or checking.
func (a *app) checkWireFields() error {
	for _, fn := range a.remotes {
		for _, typ := range []types.Type{fn.in, fn.out} {
			if err := checkTypeSerializedNames(typ, fn.goPkg.pkg.Fset, a.links); err != nil {
				return fmt.Errorf("skgo: %s: %s: %w", fn.pos, fn.name, err)
			}
			if err := a.checkNestedWireFields(typ, fn.goPkg.pkg.Fset, false); err != nil {
				return fmt.Errorf("skgo: %s: %s: %w", fn.pos, fn.name, err)
			}
		}
	}
	for _, load := range a.loads {
		if err := checkTypeSerializedNames(load.out, load.goPkg.pkg.Fset, a.links); err != nil {
			return fmt.Errorf("skgo: %s: %s: %w", load.pos, load.name, err)
		}
		if err := a.checkNestedWireFields(load.out, load.goPkg.pkg.Fset, true); err != nil {
			return fmt.Errorf("skgo: %s: %s: %w", load.pos, load.name, err)
		}
	}
	return nil
}

// Go packages can load current authored route bytes through an overlay while
// polytype still sees the committed generated route copy. Check direct fields
// from the current go/types graph before reporting that copy as stale.
func (a *app) checkNestedWireFields(typ types.Type, fset *token.FileSet, promises bool) error {
	seen := map[types.Type]bool{}
	var visit func(types.Type) error
	visit = func(t types.Type) error {
		if t == nil {
			return nil
		}
		t = types.Unalias(t)
		if seen[t] {
			return nil
		}
		seen[t] = true
		if inner, deferred := deferredElem(t); deferred {
			// A load may stream this value, but its resolved value still has to
			// satisfy the same projection and codec rules as ordinary load data.
			return visit(inner)
		}
		switch t := t.(type) {
		case *types.Named:
			obj := t.Obj()
			if obj != nil && obj.Pkg() != nil {
				path := obj.Pkg().Path()
				if path == "github.com/tylergannon/polytype" && obj.Name() == "Nullable" && t.TypeArgs().Len() == 1 {
					return visit(t.TypeArgs().At(0))
				}
				if path == "time" && obj.Name() == "Time" || path == skgoFilePkg && obj.Name() == "File" {
					return nil
				}
			}
			if _, transported := a.transportedNamed(t); transported {
				return nil
			}
			return visit(t.Underlying())
		case *types.Pointer:
			return visit(t.Elem())
		case *types.Slice:
			return visit(t.Elem())
		case *types.Array:
			return visit(t.Elem())
		case *types.Struct:
			for i := 0; i < t.NumFields(); i++ {
				field := t.Field(i)
				if !field.Exported() || reflect.StructTag(t.Tag(i)).Get("json") == "-" {
					continue
				}
				if _, err := a.projectType(field.Type(), promises); err != nil {
					return fmt.Errorf("%s: field %s cannot cross the wire: %w", fset.Position(field.Pos()), field.Name(), err)
				}
				if err := visit(field.Type()); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return visit(typ)
}

func checkTypeSerializedNames(typ types.Type, fset *token.FileSet, links *routeLinks) error {
	seen := map[types.Type]bool{}
	var visit func(types.Type) error
	visit = func(t types.Type) error {
		if t == nil {
			return nil
		}
		t = types.Unalias(t)
		if seen[t] {
			return nil
		}
		seen[t] = true
		if inner, deferred := deferredElem(t); deferred {
			return visit(inner)
		}
		switch t := t.(type) {
		case *types.Named:
			return visit(t.Underlying())
		case *types.Pointer:
			return visit(t.Elem())
		case *types.Slice:
			return visit(t.Elem())
		case *types.Array:
			return visit(t.Elem())
		case *types.Struct:
			names := map[string]string{}
			for i := 0; i < t.NumFields(); i++ {
				field := t.Field(i)
				if !field.Exported() || reflect.StructTag(t.Tag(i)).Get("json") == "-" {
					continue
				}
				name := strings.Split(reflect.StructTag(t.Tag(i)).Get("json"), ",")[0]
				if name == "" {
					if field.Anonymous() {
						// Promoted fields require encoding/json's full conflict
						// resolution. Do not guess whether one wins.
						continue
					}
					name = field.Name()
				}
				if earlier := names[name]; earlier != "" {
					pos := fset.Position(field.Pos())
					pos.Filename = links.authoredPath(pos.Filename)
					return fmt.Errorf("%s: duplicate serialized name %q on fields %s and %s; give each wire field a distinct json name", pos, name, earlier, field.Name())
				}
				names[name] = field.Name()
			}
			for i := 0; i < t.NumFields(); i++ {
				if !t.Field(i).Exported() || reflect.StructTag(t.Tag(i)).Get("json") == "-" {
					continue
				}
				if err := visit(t.Field(i).Type()); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return visit(typ)
}

// fileFieldLocation points at the authored field carrying a File when the
// offending result is a struct. Direct File results keep the marker location.
func fileFieldLocation(typ types.Type, fset *token.FileSet) string {
	seen := map[types.Type]bool{}
	var visit func(types.Type) *types.Var
	visit = func(t types.Type) *types.Var {
		if t == nil {
			return nil
		}
		t = types.Unalias(t)
		if seen[t] {
			return nil
		}
		seen[t] = true
		if inner, ok := deferredElem(t); ok {
			return visit(inner)
		}
		switch t := t.(type) {
		case *types.Named:
			if t.Obj().Pkg() != nil && t.Obj().Pkg().Path() == skgoFilePkg && t.Obj().Name() == "File" {
				return nil
			}
			return visit(t.Underlying())
		case *types.Pointer:
			return visit(t.Elem())
		case *types.Slice:
			return visit(t.Elem())
		case *types.Array:
			return visit(t.Elem())
		case *types.Map:
			return visit(t.Elem())
		case *types.Struct:
			for i := 0; i < t.NumFields(); i++ {
				field := t.Field(i)
				if !field.Exported() || reflect.StructTag(t.Tag(i)).Get("json") == "-" || !containsFile(field.Type()) {
					continue
				}
				if nested := visit(field.Type()); nested != nil {
					return nested
				}
				return field
			}
		}
		return nil
	}
	if field := visit(typ); field != nil {
		return fmt.Sprintf(" at %s field %s", fset.Position(field.Pos()), field.Name())
	}
	return ""
}
