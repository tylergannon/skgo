// Package bridge adapts generated typed calls to MCP's JSON tool interface.
package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/tylergannon/devalue/v5"
)

// ErrInvalidInput marks typed argument rejection before an application call.
var ErrInvalidInput = errors.New("invalid input")

// Tool keeps schemas and decoding on the application's resolved wire contract.
func Tool(name, description string, input, output json.RawMessage, objectInput, noInput, command bool, call func(context.Context, any) (any, error)) server.ServerTool {
	tool := mcp.NewTool(name, mcp.WithDescription(description), mcp.WithRawInputSchema(input), mcp.WithRawOutputSchema(output))
	// SDK defaults assert destructive behavior. The application has supplied no
	// such claim; leave hints unspecified rather than guessing from the kind.
	tool.Annotations = mcp.ToolAnnotation{}
	return server.ServerTool{Tool: tool, Handler: func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw := r.Params.RawArguments
		if len(raw) == 0 && noInput {
			raw = json.RawMessage(`{}`)
		}
		arg, err := arguments(raw, objectInput, noInput)
		if err != nil {
			return mcp.NewToolResultError("Invalid input: " + err.Error()), nil
		}
		value, err := call(ctx, arg)
		if err != nil {
			message := err.Error()
			if command && !errors.Is(err, ErrInvalidInput) {
				message += "; the command may have taken effect. Read current state before repeating it."
			}
			return mcp.NewToolResultError(message), nil
		}
		value, err = jsonValue(value, 0)
		if err != nil {
			message := "Result does not fit MCP JSON: " + err.Error()
			if command {
				message += "; the command completed. Do not repeat it to recover the result."
			}
			return mcp.NewToolResultError(message), nil
		}
		result := map[string]any{"result": value}
		text, err := json.Marshal(result)
		if err != nil {
			return nil, err
		}
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.NewTextContent(string(text))}, StructuredContent: result}, nil
	}}
}

func arguments(raw []byte, objectInput, noInput bool) (any, error) {
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
			return nil, fmt.Errorf("this tool takes no arguments")
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

func jsonValue(v any, depth int) (any, error) {
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
			converted, err := jsonValue(item, depth+1)
			if err != nil {
				return nil, err
			}
			result[key] = converted
		}
		return result, nil
	case []any:
		result := make([]any, len(value))
		for i, item := range value {
			converted, err := jsonValue(item, depth+1)
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
