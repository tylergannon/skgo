// Package envspec implements the shared build and startup environment contract.
package envspec

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

type Field struct {
	Name     string `json:"name"`
	Field    string `json:"field"`
	Type     string `json:"type"`
	Optional bool   `json:"optional,omitempty"`
	Public   bool   `json:"public,omitempty"`
	Static   bool   `json:"static,omitempty"`
}
type Schema struct {
	Fields []Field        `json:"fields"`
	Static map[string]any `json:"static,omitempty"`
}

// Kit next.28 generates const exports, so its reserved words are forbidden.
// __proto__ is also excluded because declarations use JavaScript object keys.
var reservedNames = " __proto__ do if in for let new try var case else enum eval null this true void with await break catch class const false super throw while yield delete export import public return static switch typeof default extends finally package private continue debugger function arguments interface protected implements instanceof "

var namePattern = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*$`)
var integerPattern = regexp.MustCompile(`^[+-]?[0-9]+$`)
var numberPattern = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?$`)

func ParseTag(field, typ, tag string, optional bool) (Field, error) {
	parts := strings.Split(tag, ",")
	f := Field{Name: parts[0], Field: field, Type: typ, Optional: optional}
	if !namePattern.MatchString(f.Name) || strings.Contains(reservedNames, " "+f.Name+" ") {
		return f, fmt.Errorf("skgo: environment field %s needs a valid env name", field)
	}
	for _, option := range parts[1:] {
		switch option {
		case "public":
			f.Public = true
		case "static":
			f.Static = true
		default:
			return f, fmt.Errorf("skgo: environment %s: unknown option %q", f.Name, option)
		}
	}
	switch typ {
	case "string", "bool", "int", "float64":
	default:
		return f, fmt.Errorf("skgo: environment %s: unsupported Go type %s", f.Name, typ)
	}
	return f, nil
}
func (s Schema) Resolve(lookup func(string) (string, bool), runtime bool) (map[string]any, error) {
	values := make(map[string]any, len(s.Fields))
	seen := map[string]bool{}
	for _, f := range s.Fields {
		if seen[f.Name] {
			return nil, fmt.Errorf("skgo: duplicate environment variable %s", f.Name)
		}
		seen[f.Name] = true
		if runtime && f.Static {
			v, ok := s.Static[f.Name]
			if !ok && !f.Optional {
				return nil, fmt.Errorf("skgo: missing build value for environment %s", f.Name)
			}
			if ok {
				values[f.Name] = v
			}
			continue
		}
		raw, ok := lookup(f.Name)
		if !ok {
			if f.Optional {
				continue
			}
			return nil, fmt.Errorf("skgo: required environment variable %s is unset", f.Name)
		}
		var v any
		var err error
		switch f.Type {
		case "string":
			v = raw
		case "bool":
			switch strings.ToLower(raw) {
			case "true", "yes":
				v = true
			case "false", "no":
				v = false
			default:
				err = fmt.Errorf("invalid boolean")
			}
		case "int":
			var n int64
			n, err = strconv.ParseInt(raw, 10, strconv.IntSize)
			if !integerPattern.MatchString(raw) || n > 9007199254740991 || n < -9007199254740991 {
				err = fmt.Errorf("invalid integer")
			}
			v = n
		case "float64":
			var n float64
			n, err = strconv.ParseFloat(raw, 64)
			if !numberPattern.MatchString(raw) || math.IsInf(n, 0) || math.IsNaN(n) {
				err = fmt.Errorf("invalid number")
			}
			v = n
		default:
			err = fmt.Errorf("unsupported type")
		}
		if err != nil {
			return nil, fmt.Errorf("skgo: environment %s must be %s", f.Name, f.Type)
		}
		values[f.Name] = v
	}
	return values, nil
}
