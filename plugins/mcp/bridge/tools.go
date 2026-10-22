// Package bridge adapts generated typed calls to MCP's JSON tool interface.
package bridge

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/tylergannon/skgo/plugins/mcp/remoteclient"
	"github.com/tylergannon/skgo/plugins/mcp/wirejson"
)

// ErrInvalidInput marks typed argument rejection before an application call.
var ErrInvalidInput = wirejson.ErrInvalidInput

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
		arg, err := wirejson.Arguments(raw, objectInput, noInput)
		if err != nil {
			return mcp.NewToolResultError("Invalid input: " + err.Error()), nil
		}
		value, err := call(ctx, arg)
		if err != nil {
			message := err.Error()
			if command && !errors.Is(err, ErrInvalidInput) && !errors.Is(err, remoteclient.ErrNotSent) {
				message += "; the command may have taken effect. Read current state before repeating it."
			}
			return mcp.NewToolResultError(message), nil
		}
		value, err = wirejson.Value(value)
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
