package main

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tylergannon/skgo/internal/advice"
	"github.com/tylergannon/skgo/internal/check"
)

func TestCLIAndInitializedStdioMCPExposeSameAdviceAndFailedCheck(t *testing.T) {
	t.Parallel()
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	bin := skgoBin
	output, err := exec.Command(bin, "advice", "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("CLI advice: %v\n%s", err, output)
	}
	var cliAdvice []advice.Entry
	if err := json.Unmarshal(output, &cliAdvice); err != nil {
		t.Fatal(err)
	}
	if len(cliAdvice) != 8 {
		t.Fatalf("advice entries = %d, want 8", len(cliAdvice))
	}
	for n, entry := range cliAdvice {
		if entry.Code != "SKGO00"+string(rune('1'+n)) || entry.Version == "" || entry.KitVersion == "" || entry.Consequence == "" || entry.Repair == "" || entry.Example == "" {
			t.Fatalf("incomplete rule advice: %+v", entry)
		}
		output, err := exec.Command(bin, "advice", "--json", entry.Code).CombinedOutput()
		if err != nil {
			t.Fatalf("CLI advice %s: %v\n%s", entry.Code, err, output)
		}
		var one []advice.Entry
		if err := json.Unmarshal(output, &one); err != nil || len(one) != 1 || one[0] != entry {
			t.Fatalf("CLI lookup %s: %v %+v", entry.Code, err, one)
		}
	}

	server := exec.Command(bin, "mcp")
	stdin, err := server.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := server.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stdin.Close(); _ = server.Process.Kill(); _ = server.Wait() })
	reader := bufio.NewReader(stdout)
	call := func(id int, method string, params any) map[string]json.RawMessage {
		t.Helper()
		request := map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}
		b, _ := json.Marshal(request)
		if _, err := stdin.Write(append(b, '\n')); err != nil {
			t.Fatal(err)
		}
		line, err := reader.ReadBytes('\n')
		if err != nil {
			t.Fatal(err)
		}
		var response map[string]json.RawMessage
		if err := json.Unmarshal(line, &response); err != nil || response["error"] != nil {
			t.Fatalf("MCP %s: %v %s", method, err, line)
		}
		return response
	}
	init := call(1, "initialize", map[string]any{"protocolVersion": mcpVersion, "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "skgo-test", "version": "1"}})
	var handshake struct{ ProtocolVersion string }
	if err := json.Unmarshal(init["result"], &handshake); err != nil || handshake.ProtocolVersion != mcpVersion {
		t.Fatalf("MCP handshake: %v %+v", err, handshake)
	}
	if _, err := io.WriteString(stdin, "{\"jsonrpc\":\"2.0\",\"method\":\"notifications/initialized\"}\n"); err != nil {
		t.Fatal(err)
	}
	list := call(2, "tools/list", map[string]any{})
	var tools struct{ Tools []struct{ Name string } }
	if err := json.Unmarshal(list["result"], &tools); err != nil || len(tools.Tools) != 2 || tools.Tools[0].Name != "skgo_check" || tools.Tools[1].Name != "skgo_advice" {
		t.Fatalf("MCP tools: %v %+v", err, tools)
	}
	getResult := func(id int, name string, args any) struct {
		IsError           bool            `json:"isError"`
		StructuredContent json.RawMessage `json:"structuredContent"`
	} {
		t.Helper()
		resp := call(id, "tools/call", map[string]any{"name": name, "arguments": args})
		var result struct {
			IsError           bool            `json:"isError"`
			StructuredContent json.RawMessage `json:"structuredContent"`
		}
		if err := json.Unmarshal(resp["result"], &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	got := getResult(3, "skgo_advice", map[string]any{})
	var catalog struct{ Advice []advice.Entry }
	if err := json.Unmarshal(got.StructuredContent, &catalog); err != nil || got.IsError || !reflect.DeepEqual(catalog.Advice, cliAdvice) {
		t.Fatalf("MCP advice differs from CLI: %v %+v", err, catalog)
	}
	for i, entry := range cliAdvice {
		got := getResult(i+4, "skgo_advice", map[string]any{"code": entry.Code})
		var one struct{ Advice []advice.Entry }
		if err := json.Unmarshal(got.StructuredContent, &one); err != nil || got.IsError || len(one.Advice) != 1 || one.Advice[0] != entry {
			t.Fatalf("MCP lookup %s: %v %+v", entry.Code, err, one)
		}
	}

	root := t.TempDir() // missing go.mod must fail before any checker can be skipped
	cli := exec.Command(bin, "check", "--root", root, "--json")
	output, err = cli.Output()
	if err == nil {
		t.Fatal("CLI check unexpectedly passed")
	}
	var cliReport check.Report
	if err := json.Unmarshal(output, &cliReport); err != nil {
		t.Fatal(err)
	}
	got = getResult(12, "skgo_check", map[string]any{"root": root})
	var mcpReport check.Report
	if err := json.Unmarshal(got.StructuredContent, &mcpReport); err != nil || !got.IsError || !reflect.DeepEqual(mcpReport, cliReport) || mcpReport.OK || len(mcpReport.Checks) == 0 || mcpReport.Checks[0].Status != "failed" {
		t.Fatalf("MCP failure differs from CLI: %v %+v %+v", err, mcpReport, cliReport)
	}

	// Exercise actual authored analyzer findings, not only project setup failure.
	root = t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "web", "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile(filepath.Join(repo, "internal", "advice", "testdata", "src", "a", "a.remote.go"))
	if err != nil {
		t.Fatal(err)
	}
	for name, contents := range map[string]string{
		"go.mod":               "module example.com/adviceclient\n\ngo 1.27.1\n\nrequire github.com/tylergannon/skgo v0.0.0\n\nreplace github.com/tylergannon/skgo => " + repo + "\n",
		"web/package.json":     "{\"name\":\"adviceclient\",\"private\":true,\"type\":\"module\"}\n",
		"src/advice.remote.go": string(fixture),
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = root
	if output, err := tidy.CombinedOutput(); err != nil {
		t.Fatalf("prepare advice fixture: %v\n%s", err, output)
	}
	cli = exec.Command(bin, "check", "--root", root, "--json")
	output, err = cli.Output()
	if err == nil {
		t.Fatal("CLI passed planted analyzer misuse")
	}
	if err := json.Unmarshal(output, &cliReport); err != nil {
		t.Fatal(err)
	}
	got = getResult(13, "skgo_check", map[string]any{"root": root})
	if err := json.Unmarshal(got.StructuredContent, &mcpReport); err != nil || !got.IsError || !reflect.DeepEqual(mcpReport, cliReport) {
		t.Fatalf("MCP analyzer findings differ from CLI: %v %+v %+v", err, mcpReport, cliReport)
	}
	complete := false
	missingTools := map[string]bool{}
	for _, c := range mcpReport.Checks {
		if c.Name == "skgo-advice" {
			complete = c.Status == "complete"
		}
		if (c.Name == "prettier" || c.Name == "eslint") && c.Status == "failed" {
			missingTools[c.Name] = true
		}
	}
	if !complete {
		t.Fatalf("analyzer did not complete: %+v", mcpReport.Checks)
	}
	if !missingTools["prettier"] || !missingTools["eslint"] {
		t.Fatalf("missing frontend checkers were hidden: %+v", mcpReport.Checks)
	}
	seen := map[string]bool{}
	for _, d := range mcpReport.Diagnostics {
		if !strings.HasPrefix(d.Code, "SKGO") {
			continue
		}
		seen[d.Code] = true
		if d.Location == nil || d.Location.File != "src/advice.remote.go" || d.Location.Line == 0 || d.Documentation != "skgo advice "+d.Code {
			t.Errorf("diagnostic cannot lead to authored advice: %+v", d)
		}
	}
	for _, code := range []string{"SKGO001", "SKGO002", "SKGO003", "SKGO004", "SKGO005", "SKGO006"} {
		if !seen[code] {
			t.Errorf("missing planted misuse %s in real CLI/MCP", code)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "src", "broken.go"), []byte("package a\nfunc broken(\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cli = exec.Command(bin, "check", "--root", root, "--json")
	output, err = cli.Output()
	if err == nil || json.Unmarshal(output, &cliReport) != nil {
		t.Fatalf("CLI hid incomplete Go checking: %v %s", err, output)
	}
	got = getResult(14, "skgo_check", map[string]any{"root": root})
	if err := json.Unmarshal(got.StructuredContent, &mcpReport); err != nil || !got.IsError || !reflect.DeepEqual(mcpReport, cliReport) {
		t.Fatalf("MCP incomplete check differs from CLI: %v %+v %+v", err, mcpReport, cliReport)
	}
	incomplete := false
	for _, c := range mcpReport.Checks {
		if c.Name == "go-vet" && c.Status == "incomplete" {
			incomplete = true
		}
	}
	if !incomplete || mcpReport.OK {
		t.Fatalf("incomplete checker masqueraded as success: %+v", mcpReport.Checks)
	}
}
