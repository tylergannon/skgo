// Package wirejson converts between JSON interface values and generated devalue codecs.
package wirejson

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/tylergannon/devalue/v5"
	"io"
	"math"
	"strconv"
)

// ErrInvalidInput marks typed input rejection before any application request.
var ErrInvalidInput = errors.New("invalid input")

// Value converts a codec result to ordinary JSON values without losing integers.
func Value(v any) (any, error) { return convertValue(v, 0) }

// Arguments parses one JSON argument object, rejecting duplicate keys and unsafe numbers.
func Arguments(raw []byte, objectInput, noInput bool) (any, error) {
	if len(raw) > 1<<20 {
		return nil, fmt.Errorf("arguments exceed 1 MiB")
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("arguments must be an object")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	v, err := readJSON(d, 0)
	if err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, fmt.Errorf("expected one JSON value")
	}
	obj, ok := v.(*devalue.Object)
	if !ok {
		return nil, fmt.Errorf("arguments must be an object")
	}
	if noInput {
		if len(obj.Keys()) != 0 {
			return nil, fmt.Errorf("this operation takes no arguments")
		}
		return devalue.Undefined, nil
	}
	if objectInput {
		return obj, nil
	}
	v, ok = obj.Get("argument")
	if !ok || len(obj.Keys()) != 1 {
		return nil, fmt.Errorf("expected only an argument property")
	}
	return v, nil
}

func readJSON(d *json.Decoder, depth int) (any, error) {
	if depth > 64 {
		return nil, fmt.Errorf("arguments are too deeply nested")
	}
	t, err := d.Token()
	if err != nil {
		return nil, err
	}
	switch t {
	case json.Delim('{'):
		o := devalue.NewObject()
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return nil, err
			}
			name, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("expected object key")
			}
			if _, exists := o.Get(name); exists {
				return nil, fmt.Errorf("duplicate property %q", name)
			}
			v, err := readJSON(d, depth+1)
			if err != nil {
				return nil, err
			}
			o.Set(name, v)
		}
		_, err := d.Token()
		return o, err
	case json.Delim('['):
		a := []any{}
		for d.More() {
			v, err := readJSON(d, depth+1)
			if err != nil {
				return nil, err
			}
			a = append(a, v)
		}
		_, err := d.Token()
		return a, err
	}
	if number, ok := t.(json.Number); ok {
		n, err := strconv.ParseFloat(string(number), 64)
		if err != nil || math.IsInf(n, 0) || math.IsNaN(n) || math.Trunc(n) == n && math.Abs(n) > 9007199254740991 {
			return nil, fmt.Errorf("number is outside the supported JSON range")
		}
		return n, nil
	}
	switch t.(type) {
	case nil, string, bool:
		return t, nil
	}
	return nil, fmt.Errorf("unexpected JSON token")
}

func convertValue(v any, depth int) (any, error) {
	if depth > 64 {
		return nil, fmt.Errorf("result is too deeply nested")
	}
	switch value := v.(type) {
	case nil, string, bool:
		return v, nil
	case float64:
		if math.IsInf(value, 0) || math.IsNaN(value) || math.Trunc(value) == value && math.Abs(value) > 9007199254740991 {
			return nil, fmt.Errorf("number is outside the supported JSON range")
		}
		return v, nil
	case *devalue.Object:
		result := map[string]any{}
		for _, key := range value.Keys() {
			item, _ := value.Get(key)
			converted, err := convertValue(item, depth+1)
			if err != nil {
				return nil, err
			}
			result[key] = converted
		}
		return result, nil
	case []any:
		result := make([]any, len(value))
		for i, item := range value {
			converted, err := convertValue(item, depth+1)
			if err != nil {
				return nil, err
			}
			result[i] = converted
		}
		return result, nil
	default:
		return nil, fmt.Errorf("unsupported value %T", v)
	}
}
