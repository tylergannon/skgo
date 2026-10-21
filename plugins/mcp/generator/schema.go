package generator

import (
	"encoding/json"
	"fmt"
	"go/constant"
	"strings"

	tg "github.com/tylergannon/polytype/typegrammar"
)

type schema map[string]any
type schemas struct{ definitions tg.Definitions }

func (s schemas) object(t tg.Type) bool {
	switch t := t.(type) {
	case *tg.Object:
		return true
	case *tg.Pointer:
		return s.object(t.Element)
	case *tg.Ref:
		for _, d := range s.definitions {
			if d.Name == t.Target {
				return s.object(d.Type)
			}
		}
	}
	return false
}

func (s schemas) node(t tg.Type) (schema, error) {
	switch t := t.(type) {
	case *tg.Scalar:
		switch t.Kind {
		case tg.String:
			return schema{"type": "string"}, nil
		case tg.Bool:
			return schema{"type": "boolean"}, nil
		case tg.Float32, tg.Float64:
			return schema{"type": "number"}, nil
		default:
			bounds := map[tg.ScalarKind][2]int64{tg.Int8: {-128, 127}, tg.Uint8: {0, 255}, tg.Int16: {-32768, 32767}, tg.Uint16: {0, 65535}, tg.Int32: {-2147483648, 2147483647}, tg.Uint32: {0, 4294967295}, tg.Int: {-9007199254740991, 9007199254740991}, tg.Int64: {-9007199254740991, 9007199254740991}, tg.Uint: {0, 9007199254740991}, tg.Uint64: {0, 9007199254740991}}
			b, ok := bounds[t.Kind]
			if !ok {
				return nil, fmt.Errorf("unsupported scalar %s", t.Kind)
			}
			return schema{"type": "integer", "minimum": b[0], "maximum": b[1]}, nil
		}
	case *tg.Time:
		return schema{"type": "string"}, nil
	case *tg.Pointer:
		return s.node(t.Element)
	case *tg.Ref:
		for i, d := range s.definitions {
			if d.Name == t.Target {
				return schema{"$ref": fmt.Sprintf("#/$defs/T%d", i)}, nil
			}
		}
		return nil, fmt.Errorf("unresolved type %s", t.Target)
	case *tg.Object:
		properties := map[string]any{}
		required := []string{}
		for _, field := range t.Fields {
			var child tg.Type
			optional, nullable := false, false
			switch f := field.Value.(type) {
			case *tg.Required:
				child = f.Type
			case *tg.Optional:
				child, optional = f.Type, true
			case *tg.Nullable:
				child, nullable = f.Type, true
			default:
				return nil, fmt.Errorf("field %s: MCP does not yet support %T", field.JSONName, field.Value)
			}
			v, err := s.node(child)
			if err != nil {
				return nil, err
			}
			if nullable {
				v = schema{"anyOf": []any{v, schema{"type": "null"}}}
			}
			if field.Description != "" {
				v["description"] = field.Description
			}
			properties[field.JSONName] = v
			if !optional {
				required = append(required, field.JSONName)
			}
		}
		return schema{"type": "object", "properties": properties, "required": required, "additionalProperties": false}, nil
	case *tg.Slice:
		v, err := s.node(t.Element)
		return schema{"type": "array", "items": v}, err
	case *tg.Array:
		v, err := s.node(t.Element)
		return schema{"type": "array", "items": v, "minItems": t.Length, "maxItems": t.Length}, err
	case *tg.Enum:
		values := []any{}
		for _, member := range t.Members {
			if t.Mode == tg.EnumNames {
				values = append(values, member.Name)
			} else if member.Value.Kind() == constant.String {
				values = append(values, constant.StringVal(member.Value))
			} else {
				n, exact := constant.Int64Val(member.Value)
				if !exact || n < -9007199254740991 || n > 9007199254740991 {
					return nil, fmt.Errorf("enum %s exceeds the JSON integer range", t.GoType)
				}
				values = append(values, n)
			}
		}
		return schema{"enum": values}, nil
	default:
		return nil, fmt.Errorf("MCP does not yet support %T", t)
	}
}

func (s schemas) document(t tg.Type, wrapper string) ([]byte, error) {
	// MCP input arguments are an object. Put its fields at the root so clients
	// can use their descriptions without resolving a top-level reference.
	if wrapper == "" && s.object(t) {
		for {
			switch node := t.(type) {
			case *tg.Pointer:
				t = node.Element
			case *tg.Ref:
				for _, d := range s.definitions {
					if d.Name == node.Target {
						t = d.Type
						break
					}
				}
			default:
				goto resolved
			}
		}
	}
resolved:
	root := schema{"type": "object", "additionalProperties": false}
	var err error
	if t != nil {
		root, err = s.node(t)
		if err != nil {
			return nil, err
		}
	}
	if wrapper != "" {
		root = schema{"type": "object", "properties": schema{wrapper: root}, "required": []string{wrapper}, "additionalProperties": false}
	}
	defs := schema{}
	for i, d := range s.definitions {
		v, err := s.node(d.Type)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", d.Name, err)
		}
		if strings.TrimSpace(d.Description) != "" {
			v["description"] = d.Description
		}
		defs[fmt.Sprintf("T%d", i)] = v
	}
	if len(defs) > 0 {
		root["$defs"] = defs
	}
	return json.Marshal(root)
}
