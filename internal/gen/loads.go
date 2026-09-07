package gen

import (
	"fmt"
	"go/types"
	"path/filepath"
	"sort"
	"strings"
)

// deferredType is the Go type of a value a load promises but does not have yet.
// It is the one shape whose TypeScript is not polytype's, because there is no
// Go type polytype could project to `Promise<T>`.
const deferredType = skgoPkg + ".Deferred"

// declareLoadTypes registers every named type a load's result carries, so that
// polytype has declared them by the time the stubs import them.
func (a *app) declareLoadTypes() error {
	for _, load := range a.loads {
		if _, err := a.loadFields(load); err != nil {
			return err
		}
	}
	return nil
}

// writeLoadStubs emits one `+page.server.ts` or `+layout.server.ts` per Go
// load.
//
// The export has to be named `load`, and it has to survive being imported: kit
// decides whether its client ever asks for `__data.json` by importing the built
// module and looking for that name (`utils/routing.js`, `has_server_load`). It
// never calls it. So the body throws, and a page that renders data is proof the
// Go handler answered.
func (a *app) writeLoadStubs() error {
	for _, load := range a.loads {
		fields, err := a.loadFields(load)
		if err != nil {
			return err
		}

		dir := filepath.Dir(load.stub)
		imports := map[string][]string{}
		for _, field := range fields {
			for _, dep := range field.deps {
				set := a.typeSets[dep.Obj().Pkg()]
				spec, err := importSpecifier(dir, set.tsDir)
				if err != nil {
					return err
				}
				imports[spec] = appendUnique(imports[spec], dep.Obj().Name())
			}
		}

		var b strings.Builder
		b.WriteString(tsHeader)

		var specs []string
		for spec := range imports {
			specs = append(specs, spec)
		}
		sort.Strings(specs)
		for _, spec := range specs {
			names := imports[spec]
			sort.Strings(names)
			fmt.Fprintf(&b, "import type { %s } from '%s';\n", strings.Join(names, ", "), spec)
		}
		if len(specs) > 0 {
			b.WriteString("\n")
		}

		b.WriteString("// The body throws. This load is implemented in Go, and skgo answers\n")
		b.WriteString("// __data.json itself, so anything that renders in the browser is proof the\n")
		b.WriteString("// Go handler replied rather than this module. Kit reads the export to learn\n")
		b.WriteString("// that the route has server data; it never calls it.\n")
		b.WriteString("const unimplemented = (): never => {\n\tthrow new Error('skgo: implemented in Go');\n};\n\n")

		var parts []string
		for _, field := range fields {
			optional := ""
			if field.optional {
				optional = "?"
			}
			parts = append(parts, fmt.Sprintf("%s%s: %s", field.name, optional, field.expr))
		}
		shape := "Record<string, never>"
		if len(parts) > 0 {
			shape = "{ " + strings.Join(parts, "; ") + " }"
		}
		fmt.Fprintf(&b, "export const load = (): %s => unimplemented();\n", shape)

		if err := a.write(load.stub, b.String()); err != nil {
			return err
		}
	}
	return nil
}

// loadField is one property of the object a load returns.
type loadField struct {
	name     string
	expr     string
	optional bool
	deps     []*types.Named
}

// loadFields projects the Go struct a load returns into the properties kit's
// client will see.
//
// It is spelled out property by property rather than handed to polytype as a
// whole, because one property may be a Deferred — a value that arrives later —
// and its TypeScript is `Promise<T>`, which is not a projection of any Go type.
// Every named type inside a property is still polytype's to declare.
func (a *app) loadFields(load *loadFn) ([]loadField, error) {
	st, ok := structUnder(load.out)
	if !ok {
		return nil, fmt.Errorf("skgo: %s: a load must return a struct — kit requires a plain object, and refuses anything else — but this one returns %s", load.pos, load.out)
	}
	var fields []loadField
	if err := a.collectLoadFields(load, st, &fields, map[string]bool{}); err != nil {
		return nil, err
	}
	return fields, nil
}

func (a *app) collectLoadFields(load *loadFn, st *types.Struct, into *[]loadField, seen map[string]bool) error {
	for i := 0; i < st.NumFields(); i++ {
		field := st.Field(i)
		if !field.Exported() {
			continue
		}
		name, optional, skip := jsonName(field, reflectTag(st.Tag(i)))
		if skip {
			continue
		}

		// An embedded struct's properties are promoted into the same object,
		// exactly as encoding/json promotes them.
		if field.Anonymous() && name == field.Name() && !hasJSONName(reflectTag(st.Tag(i))) {
			if embedded, ok := structUnder(field.Type()); ok {
				if err := a.collectLoadFields(load, embedded, into, seen); err != nil {
					return err
				}
				continue
			}
		}

		if seen[name] {
			return fmt.Errorf("skgo: %s: the load's result has two properties named %q", load.pos, name)
		}
		seen[name] = true

		expr, deps, err := a.projectLoadField(field.Type())
		if err != nil {
			return fmt.Errorf("skgo: %s: the %s field of the load's result cannot cross to TypeScript: %v", load.pos, field.Name(), err)
		}
		for _, dep := range deps {
			if err := a.declare(dep); err != nil {
				return err
			}
		}
		*into = append(*into, loadField{name: name, expr: expr, optional: optional, deps: deps})
	}
	return nil
}

// projectLoadField is `project` plus the one type it does not know: a Deferred,
// which reaches the browser as a promise.
func (a *app) projectLoadField(t types.Type) (string, []*types.Named, error) {
	if inner, ok := deferredElem(t); ok {
		projected, err := a.project(inner)
		if err != nil {
			return "", nil, fmt.Errorf("a deferred value's type cannot cross: %v", err)
		}
		return "Promise<" + projected.expr + ">", projected.deps, nil
	}
	if containsDeferred(t) {
		return "", nil, fmt.Errorf("%s holds a Deferred below the top level of the load's result; kit's client only awaits promises the load returns directly", t)
	}
	projected, err := a.project(t)
	if err != nil {
		return "", nil, err
	}
	return projected.expr, projected.deps, nil
}

// deferredElem reports whether t is skgo.Deferred[T], and returns T.
func deferredElem(t types.Type) (types.Type, bool) {
	named, ok := t.(*types.Named)
	if !ok || named.Obj().Pkg() == nil {
		return nil, false
	}
	if named.Obj().Pkg().Path()+"."+named.Obj().Name() != deferredType {
		return nil, false
	}
	args := named.TypeArgs()
	if args == nil || args.Len() != 1 {
		return nil, false
	}
	return args.At(0), true
}

// containsDeferred reports whether a Deferred can occur anywhere inside t. The
// generator refuses what the serializer cannot honour, so that the refusal
// arrives at generate time rather than as a silent null on the wire.
func containsDeferred(t types.Type) bool {
	return containsDeferredSeen(t, map[types.Type]bool{})
}

func containsDeferredSeen(t types.Type, seen map[types.Type]bool) bool {
	if t == nil || seen[t] {
		return false
	}
	seen[t] = true
	if _, ok := deferredElem(t); ok {
		return true
	}
	switch u := t.(type) {
	case *types.Named:
		return containsDeferredSeen(u.Underlying(), seen)
	case *types.Pointer:
		return containsDeferredSeen(u.Elem(), seen)
	case *types.Slice:
		return containsDeferredSeen(u.Elem(), seen)
	case *types.Array:
		return containsDeferredSeen(u.Elem(), seen)
	case *types.Map:
		return containsDeferredSeen(u.Elem(), seen)
	case *types.Struct:
		for i := 0; i < u.NumFields(); i++ {
			if containsDeferredSeen(u.Field(i).Type(), seen) {
				return true
			}
		}
	}
	return false
}

func structUnder(t types.Type) (*types.Struct, bool) {
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	st, ok := t.Underlying().(*types.Struct)
	return st, ok
}

// reflectTag returns the `json` tag value of a struct tag.
func reflectTag(tag string) string {
	for tag != "" {
		i := 0
		for i < len(tag) && tag[i] == ' ' {
			i++
		}
		tag = tag[i:]
		if tag == "" {
			break
		}
		i = 0
		for i < len(tag) && tag[i] > ' ' && tag[i] != ':' && tag[i] != '"' {
			i++
		}
		if i == 0 || i+1 >= len(tag) || tag[i] != ':' || tag[i+1] != '"' {
			break
		}
		name := tag[:i]
		tag = tag[i+1:]

		i = 1
		for i < len(tag) && tag[i] != '"' {
			if tag[i] == '\\' {
				i++
			}
			i++
		}
		if i >= len(tag) {
			break
		}
		value := tag[1:i]
		tag = tag[i+1:]
		if name == "json" {
			return value
		}
	}
	return ""
}

func hasJSONName(tag string) bool {
	name, _, _ := strings.Cut(tag, ",")
	return name != ""
}

// jsonName applies encoding/json's naming rules to one field.
func jsonName(field *types.Var, tag string) (name string, optional, skip bool) {
	value, opts, hasOpts := strings.Cut(tag, ",")
	if value == "-" && !hasOpts {
		return "", false, true
	}
	name = field.Name()
	if value != "" {
		name = value
	}
	for _, opt := range strings.Split(opts, ",") {
		if opt == "omitempty" || opt == "omitzero" {
			optional = true
		}
	}
	return name, optional, false
}
