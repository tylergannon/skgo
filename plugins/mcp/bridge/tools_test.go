package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestTypedRejectionDoesNotSuggestTheCommandRan(t *testing.T) {
	tool := Tool("create", "Create a task", json.RawMessage(`{"type":"object"}`), json.RawMessage(`{"type":"object"}`), true, false, true, func(context.Context, any) (any, error) { return nil, fmt.Errorf("%w: missing title", ErrInvalidInput) })
	metadata, err := json.Marshal(tool.Tool)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(metadata), "destructiveHint") || strings.Contains(string(metadata), "readOnlyHint") {
		t.Fatalf("unclaimed hints in %s", metadata)
	}
	request := mcp.CallToolRequest{Params: mcp.CallToolParams{RawArguments: json.RawMessage(`{}`)}}
	result, err := tool.Handler(context.Background(), request)
	if err != nil || !result.IsError {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if strings.Contains(result.Content[0].(mcp.TextContent).Text, "may have taken effect") {
		t.Fatal("claimed possible dispatch for typed input rejection")
	}
}
