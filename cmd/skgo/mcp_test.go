package main

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/tylergannon/skgo/internal/advice"
	"github.com/tylergannon/skgo/internal/check"
)

func TestMCPInitializesDispatchesAndKeepsOutputStructured(t *testing.T) {
	t.Parallel()
	root, err := json.Marshal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}
{"jsonrpc":"2.0","id":2,"method":"initialize","params":{"protocolVersion":"2025-11-25"}}
{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":3,"method":"tools/list"}
{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"skgo_advice","arguments":{"code":"SKGO001"}}}
{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"skgo_check","arguments":{"root":` + string(root) + `}}}
{"jsonrpc":"2.0","id":6,"method":"unknown"}
`
	var output bytes.Buffer
	if err := serveMCP(strings.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(&output)
	var responses []struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      int             `json:"id"`
		Result  json.RawMessage `json:"result"`
		Error   *mcpError       `json:"error"`
	}
	for id := 1; id <= 6; id++ {
		var response struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      int             `json:"id"`
			Result  json.RawMessage `json:"result"`
			Error   *mcpError       `json:"error"`
		}
		if err := decoder.Decode(&response); err != nil {
			t.Fatalf("response %d: %v", id, err)
		}
		if response.JSONRPC != "2.0" || response.ID != id {
			t.Fatalf("response %d envelope: %+v", id, response)
		}
		responses = append(responses, response)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		t.Fatalf("unsolicited stdout or notification response: %v", err)
	}
	if responses[0].Error == nil || responses[0].Error.Code != -32000 {
		t.Fatalf("tools accepted before initialization: %+v", responses[0])
	}
	var handshake struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := json.Unmarshal(responses[1].Result, &handshake); err != nil || responses[1].Error != nil || handshake.ProtocolVersion != "2025-11-25" {
		t.Fatalf("handshake: %+v, %v", responses[1], err)
	}
	var list struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(responses[2].Result, &list); err != nil || len(list.Tools) != 2 || list.Tools[0].Name != "skgo_check" || list.Tools[1].Name != "skgo_advice" {
		t.Fatalf("tool list: %s, %v", responses[2].Result, err)
	}
	var catalog struct {
		IsError           bool `json:"isError"`
		StructuredContent struct {
			Advice []advice.Entry `json:"advice"`
		} `json:"structuredContent"`
	}
	if err := json.Unmarshal(responses[3].Result, &catalog); err != nil || catalog.IsError || len(catalog.StructuredContent.Advice) != 1 || catalog.StructuredContent.Advice[0].Code != "SKGO001" || catalog.StructuredContent.Advice[0].Repair == "" {
		t.Fatalf("advice dispatch: %s, %v", responses[3].Result, err)
	}
	var failed struct {
		IsError           bool                          `json:"isError"`
		Content           []struct{ Type, Text string } `json:"content"`
		StructuredContent check.Report                  `json:"structuredContent"`
	}
	if err := json.Unmarshal(responses[4].Result, &failed); err != nil || !failed.IsError || failed.StructuredContent.OK || len(failed.StructuredContent.Checks) != 1 || failed.StructuredContent.Checks[0].Name != "project" || failed.StructuredContent.Checks[0].Status != "failed" || !strings.Contains(failed.StructuredContent.Checks[0].Message, "go.mod is missing") {
		t.Fatalf("check failure dispatch: %s, %v", responses[4].Result, err)
	}
	if len(failed.Content) != 1 || failed.Content[0].Type != "text" || !json.Valid([]byte(failed.Content[0].Text)) {
		t.Fatalf("MCP text content is not structured JSON: %+v", failed.Content)
	}
	if responses[5].Error == nil || responses[5].Error.Code != -32601 {
		t.Fatalf("unknown method: %+v", responses[5])
	}
}
