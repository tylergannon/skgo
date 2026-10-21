package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tylergannon/devalue/v5"
)

func TestTypedRejectionDoesNotSuggestTheCommandRan(t *testing.T) {
	tool := Tool("create", "Create a task", json.RawMessage(`{"type":"object"}`), json.RawMessage(`{"type":"object"}`), true, false, true, func(context.Context, any) (any, error) { return nil, fmt.Errorf("%w: missing title", ErrInvalidInput) })
	request := mcp.CallToolRequest{Params: mcp.CallToolParams{RawArguments: json.RawMessage(`{}`)}}
	result, err := tool.Handler(context.Background(), request)
	if err != nil || !result.IsError {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if strings.Contains(result.Content[0].(mcp.TextContent).Text, "may have taken effect") {
		t.Fatal("claimed possible dispatch for typed input rejection")
	}
}

func TestArguments(t *testing.T) {
	for _, raw := range []string{`{"a":1,"a":2}`, `{} {}`, `{"a":9007199254740993}`, `null`, `[]`, `{"a":1e999}`} {
		if _, err := arguments([]byte(raw), true, false); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	value, err := arguments([]byte(`{"title":"Ship MCP"}`), true, false)
	if err != nil {
		t.Fatal(err)
	}
	if title, _ := value.(*devalue.Object).Get("title"); title != "Ship MCP" {
		t.Fatalf("title=%v", title)
	}
	value, err = arguments([]byte(`{}`), false, true)
	if err != nil || value != devalue.Undefined {
		t.Fatalf("no input=%v %v", value, err)
	}
	value, err = arguments([]byte(`{"argument":null}`), false, false)
	if err != nil || value != nil {
		t.Fatalf("null=%v %v", value, err)
	}
	if _, err := arguments([]byte(`{"surprise":true}`), false, true); err == nil {
		t.Fatal("accepted unexpected argument")
	}
}

func TestJSONResult(t *testing.T) {
	value, err := jsonValue(devalue.NewObject("id", float64(7), "titles", []any{"Ship MCP"}), 0)
	if err != nil || value.(map[string]any)["id"] != float64(7) {
		t.Fatalf("result=%v %v", value, err)
	}
	if _, err := jsonValue(float64(9007199254740992), 0); err == nil {
		t.Fatal("accepted unsafe integer")
	}
	if _, err := jsonValue(devalue.Undefined, 0); err == nil {
		t.Fatal("accepted undefined")
	}
}
