package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/tylergannon/skgo/internal/advice"
	"github.com/tylergannon/skgo/internal/check"
)

const mcpVersion = "2025-11-25"

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *mcpError       `json:"error,omitempty"`
}

type toolResult struct {
	Content           []map[string]string `json:"content"`
	StructuredContent any                 `json:"structuredContent"`
	IsError           bool                `json:"isError"`
}

func mcpToolResult(value any, failed bool) toolResult {
	b, _ := json.Marshal(value)
	return toolResult{Content: []map[string]string{{"type": "text", "text": string(b)}}, StructuredContent: value, IsError: failed}
}

// serveMCP keeps stdout exclusively for newline-delimited JSON-RPC messages.
func serveMCP(in io.Reader, out io.Writer) error {
	decoder := json.NewDecoder(in)
	encoder := json.NewEncoder(out)
	initialized := false
	ready := false
	for {
		var req mcpRequest
		if err := decoder.Decode(&req); err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("MCP input: %w", err)
		}
		if req.ID == nil {
			if req.Method == "notifications/initialized" && initialized {
				ready = true
			}
			continue
		}
		response := mcpResponse{JSONRPC: "2.0", ID: req.ID}
		switch {
		case req.JSONRPC != "2.0":
			response.Error = &mcpError{-32600, "expected JSON-RPC 2.0"}
		case req.Method == "initialize" && !initialized:
			var params struct {
				ProtocolVersion string `json:"protocolVersion"`
			}
			if err := json.Unmarshal(req.Params, &params); err != nil || params.ProtocolVersion == "" {
				response.Error = &mcpError{-32602, "protocolVersion is required"}
			} else {
				initialized = true
				response.Result = map[string]any{
					"protocolVersion": mcpVersion,
					"capabilities":    map[string]any{"tools": map[string]any{}},
					"serverInfo":      map[string]string{"name": "skgo", "version": advice.Version()},
				}
			}
		case req.Method == "ping":
			response.Result = map[string]any{}
		case !ready:
			response.Error = &mcpError{-32000, "initialize and send notifications/initialized before using tools"}
		case req.Method == "tools/list":
			response.Result = map[string]any{"tools": []any{
				map[string]any{"name": "skgo_check", "description": "Run skgo check and return every checker status and diagnostic; ok=false or isError=true means the project did not pass.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"root": map[string]string{"type": "string"}, "web": map[string]string{"type": "string"}, "out": map[string]string{"type": "string"}}}},
				map[string]any{"name": "skgo_advice", "description": "List installed-version guidance for SKGO001–SKGO008, or look up one diagnostic code.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"code": map[string]string{"type": "string"}}}},
			}}
		case req.Method == "tools/call":
			var params struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			}
			if err := json.Unmarshal(req.Params, &params); err != nil || params.Name == "" {
				response.Error = &mcpError{-32602, "tool name is required"}
				break
			}
			switch params.Name {
			case "skgo_check":
				var args struct{ Root, Web, Out string }
				if len(params.Arguments) == 0 {
					params.Arguments = []byte("{}")
				}
				if err := json.Unmarshal(params.Arguments, &args); err != nil {
					response.Error = &mcpError{-32602, "invalid skgo_check arguments"}
					break
				}
				if args.Root == "" {
					args.Root = "."
				}
				report := check.Run(context.Background(), check.Options{Root: args.Root, Web: args.Web, Out: args.Out})
				response.Result = mcpToolResult(report, !report.OK)
			case "skgo_advice":
				var args struct {
					Code string `json:"code"`
				}
				if len(params.Arguments) == 0 {
					params.Arguments = []byte("{}")
				}
				if err := json.Unmarshal(params.Arguments, &args); err != nil {
					response.Error = &mcpError{-32602, "invalid skgo_advice arguments"}
					break
				}
				entries := advice.Catalog()
				if args.Code != "" {
					entry, ok := advice.Lookup(args.Code)
					if !ok {
						response.Result = mcpToolResult(map[string]string{"error": "unknown skgo advice code " + args.Code}, true)
						break
					}
					entries = []advice.Entry{entry}
				}
				response.Result = mcpToolResult(map[string]any{"advice": entries}, false)
			default:
				response.Error = &mcpError{-32602, "unknown tool " + params.Name}
			}
		default:
			response.Error = &mcpError{-32601, "method not found"}
		}
		if err := encoder.Encode(response); err != nil {
			return fmt.Errorf("MCP output: %w", err)
		}
	}
}
