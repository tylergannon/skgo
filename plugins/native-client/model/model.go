// Package model defines the supported native wire model independently of any
// language emitter. Both shell plugins use the same admission rules.
package model

import (
	"fmt"
	"go/constant"

	tg "github.com/tylergannon/polytype/typegrammar"
	"github.com/tylergannon/skgo/templateapi"
)

type Call struct {
	Name, ID, Kind string
	Input, Output  int // -1 input means omitted argument.
}

// Calls preserves resolved Kit identities and allocates stable operation names.
func Calls(m templateapi.GenerationModel) ([]Call, error) {
	names := map[string]bool{"core": true, "init": true}
	calls := make([]Call, 0, len(m.Remotes))
	for _, op := range m.Remotes {
		if op.Kind != "query" && op.Kind != "command" {
			return nil, fmt.Errorf("%s: %s is a %s; native clients support ordinary queries and commands", op.Source, op.Selection, op.Kind)
		}
		if op.Transported {
			return nil, fmt.Errorf("%s: %s uses a custom transport unsupported by the native client", op.Source, op.Selection)
		}
		if op.Input < -1 || op.Output < 0 || op.Output >= len(m.Roots) || op.Input >= len(m.Roots) {
			return nil, fmt.Errorf("%s: remote model has unprojected types", op.Selection)
		}
		name := op.Name
		for n := 2; names[name]; n++ {
			name = fmt.Sprintf("%s_%d", op.Name, n)
		}
		names[name] = true
		calls = append(calls, Call{Name: name, ID: op.ID, Kind: op.Kind, Input: op.Input, Output: op.Output})
	}
	return calls, nil
}

func Validate(defs tg.Definitions, roots []tg.Type, calls []Call) error {
	if err := defs.Validate(); err != nil {
		return err
	}
	names := make(map[tg.Name]bool, len(defs))
	for _, d := range defs {
		names[d.Name] = true
	}
	var visit func(tg.Type) error
	var union func(*tg.Union) error
	union = func(u *tg.Union) error {
		for _, v := range u.Variants {
			if !names[v.Implementation] {
				return fmt.Errorf("unresolved union member %s", v.Implementation)
			}
		}
		return nil
	}
	visit = func(t tg.Type) error {
		switch n := t.(type) {
		case *tg.Scalar:
			switch n.Kind {
			case tg.String, tg.Bool, tg.Int, tg.Int8, tg.Int16, tg.Int32, tg.Int64, tg.Uint, tg.Uint8, tg.Uint16, tg.Uint32, tg.Uint64, tg.Float32, tg.Float64:
			default:
				return fmt.Errorf("unsupported scalar %s", n.Kind)
			}
		case *tg.Time:
		case *tg.Ref:
			if !names[n.Target] {
				return fmt.Errorf("unresolved reference %s", n.Target)
			}
		case *tg.Pointer:
			return visit(n.Element)
		case *tg.Slice:
			return visit(n.Element)
		case *tg.Array:
			return visit(n.Element)
		case *tg.Enum:
			if n.Kind == tg.Bool || n.Kind == tg.Float32 || n.Kind == tg.Float64 {
				return fmt.Errorf("unsupported enum scalar %s", n.Kind)
			}
			if n.Mode != tg.EnumNames && n.Kind != tg.String {
				for _, member := range n.Members {
					number, ok := constant.Int64Val(member.Value)
					if !ok || number < -9007199254740991 || number > 9007199254740991 {
						return fmt.Errorf("%s.%s is outside the JavaScript integer domain", n.GoType, member.Name)
					}
				}
			}
		case *tg.Object:
			for _, f := range n.Fields {
				var err error
				switch v := f.Value.(type) {
				case *tg.Required:
					err = visit(v.Type)
				case *tg.Optional:
					err = visit(v.Type)
				case *tg.Nullable:
					err = visit(v.Type)
				case *tg.Union:
					err = union(v)
				case *tg.OptionalUnion:
					err = union(&v.Union)
				case *tg.UnionSlice:
					err = union(&v.Union)
				default:
					err = fmt.Errorf("unsupported field grammar %T", v)
				}
				if err != nil {
					return fmt.Errorf("%s: field %s: %w", f.Source, f.JSONName, err)
				}
			}
		default:
			return fmt.Errorf("unsupported type grammar %T", t)
		}
		return nil
	}
	for _, d := range defs {
		if err := visit(d.Type); err != nil {
			return fmt.Errorf("%s: %s: %w", d.Source, d.Name, err)
		}
	}
	for _, root := range roots {
		if err := visit(root); err != nil {
			return err
		}
	}
	used := make(map[string]bool)
	for _, c := range calls {
		if c.Kind != "query" && c.Kind != "command" {
			return fmt.Errorf("%s: unsupported native remote kind %s", c.Name, c.Kind)
		}
		if c.Input < -1 || c.Input >= len(roots) || c.Output < 0 || c.Output >= len(roots) {
			return fmt.Errorf("%s: invalid native root index", c.Name)
		}
		if used[c.Name] {
			return fmt.Errorf("duplicate native call name %s", c.Name)
		}
		used[c.Name] = true
	}
	return nil
}
