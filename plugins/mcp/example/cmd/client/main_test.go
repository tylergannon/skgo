package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tylergannon/skgo"
	client "github.com/tylergannon/skgo/plugins/mcp/example/internal/mcpclient"
	generated "github.com/tylergannon/skgo/plugins/mcp/example/internal/skgo"
	"github.com/tylergannon/skgo/plugins/mcp/remoteclient"
)

func TestCLIAndMCPShareTheRealTaskHandlers(t *testing.T) {
	t.Setenv("SKGO_API_KEY", "")
	remotes, err := skgo.NewRemotes(skgo.RemoteConfig{}, generated.Remotes()...)
	if err != nil {
		t.Fatal(err)
	}
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "example-key" {
			http.Error(w, "Unauthorized", 401)
			return
		}
		remotes.ServeHTTP(w, r)
	}))
	defer app.Close()
	run := func(args ...string) (string, error) {
		t.Helper()
		cmd := newCommand()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(append([]string{"--origin", app.URL}, args...))
		err := cmd.Execute()
		return out.String(), err
	}
	// A literal empty list anchors the fixture; no baseline is taken from a
	// potentially broken counter or result.
	if out, err := run("--api-key", "example-key", "tasks", "list"); err != nil || out != "{\"result\":[]}\n" {
		t.Fatalf("out=%q err=%v", out, err)
	}
	if out, err := run("tasks", "list"); err == nil || !strings.Contains(err.Error(), "401") || out != "" {
		t.Fatalf("unauthenticated out=%q err=%v", out, err)
	}
	if out, err := run("--api-key", "example-key", "tasks", "add", "--title", "Ship CLI"); err != nil || out != "{\"result\":{\"id\":1,\"title\":\"Ship CLI\"}}\n" {
		t.Fatalf("out=%q err=%v", out, err)
	}
	file := filepath.Join(t.TempDir(), "task.txt")
	if err := os.WriteFile(file, []byte("@from-file\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := run("--api-key", "example-key", "tasks", "add", "--title", "@"+file); err != nil || out != "{\"result\":{\"id\":2,\"title\":\"@from-file\\n\"}}\n" {
		t.Fatalf("out=%q err=%v", out, err)
	}
	if out, err := run("--api-key", "example-key", "tasks", "add", "--title", "@@owner"); err != nil || out != "{\"result\":{\"id\":3,\"title\":\"@owner\"}}\n" {
		t.Fatalf("out=%q err=%v", out, err)
	}
	if _, err := run("--api-key", "example-key", "tasks", "add", "--title", "@"+file+".missing"); err == nil || !strings.Contains(err.Error(), "use @@") {
		t.Fatal(err)
	}
	if _, err := run("--api-key", "example-key", "tasks", "add", "--input-json", `{"title":7}`); err == nil || !strings.Contains(err.Error(), "invalid input") {
		t.Fatal(err)
	}
	if _, err := run("--api-key", "example-key", "tasks", "add", "--title", "   "); err == nil || !strings.Contains(err.Error(), "Supply a task title") {
		t.Fatal(err)
	}

	c := &client.Client{Remote: remoteclient.Client{Origin: app.URL, PrepareRequest: func(r *http.Request) error { r.Header.Set("X-API-Key", "example-key"); return nil }}}
	tools := c.Tools()
	if len(tools) != 2 || tools[0].Tool.Name != "addTask" || tools[1].Tool.Name != "listTasks" {
		t.Fatalf("published tools=%+v", tools)
	}
	result, err := tools[0].Handler(t.Context(), mcp.CallToolRequest{Params: mcp.CallToolParams{RawArguments: json.RawMessage(`{"title":"@MCP-is-literal"}`)}})
	if err != nil || result.IsError {
		t.Fatalf("MCP result=%+v err=%v", result, err)
	}
	if text := result.Content[0].(mcp.TextContent).Text; text != `{"result":{"id":4,"title":"@MCP-is-literal"}}` {
		t.Fatal(text)
	}
	if out, err := run("--api-key", "example-key", "tasks", "add-file", file); err != nil || out != "{\"result\":{\"id\":5,\"title\":\"@from-file\\n\"}}\n" {
		t.Fatalf("custom out=%q err=%v", out, err)
	}
	const want = "{\"result\":[{\"id\":1,\"title\":\"Ship CLI\"},{\"id\":2,\"title\":\"@from-file\\n\"},{\"id\":3,\"title\":\"@owner\"},{\"id\":4,\"title\":\"@MCP-is-literal\"},{\"id\":5,\"title\":\"@from-file\\n\"}]}\n"
	if out, err := run("--api-key", "example-key", "tasks", "list"); err != nil || out != want {
		t.Fatalf("out=%q err=%v", out, err)
	}
}

func TestHelpNeedsNoConnectionAndDoesNotExposeEnvironmentKey(t *testing.T) {
	t.Setenv("SKGO_API_KEY", "private-example-value")
	cmd := newCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"tasks", "add", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"Title is the task to do.", "ID identifies this task", "@@text", "--api-key", "task-client tasks add"} {
		if !strings.Contains(out.String(), text) {
			t.Fatalf("missing %q in %s", text, out.String())
		}
	}
	if strings.Contains(out.String(), "private-example-value") {
		t.Fatal("help exposed an environment credential")
	}
}
