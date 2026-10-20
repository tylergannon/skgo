package aitools_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/tylergannon/skgo/internal/aitools"
	"github.com/tylergannon/skgo/templateapi"
)

func apply(t *testing.T, root string) (templateapi.Result, error) {
	t.Helper()
	return aitools.Plugin("test").Apply(context.Background(), templateapi.Request{
		Addon:   "ai-tools",
		Project: templateapi.Project{Root: root, Web: filepath.Join(root, "web"), Module: "example.test/app", Name: "app"},
	})
}

func project(t *testing.T, installed bool) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "web")
	if installed {
		dir = filepath.Join(dir, "node_modules", ".bin")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if installed {
		if err := os.WriteFile(filepath.Join(dir, "vp"), nil, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func read(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestAIToolsConfiguresAgentsAtTheApplicationRoot(t *testing.T) {
	root := project(t, false)
	result, err := apply(t, root)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"AGENTS.md", "CLAUDE.md", ".mcp.json"}; !reflect.DeepEqual(result.Changed, want) {
		t.Fatalf("changed %v; want %v", result.Changed, want)
	}

	// Claude Code reads CLAUDE.md, not AGENTS.md; the import makes them one.
	if got := read(t, filepath.Join(root, "CLAUDE.md")); got != "@AGENTS.md\n" {
		t.Errorf("CLAUDE.md = %q", got)
	}
	agents := read(t, filepath.Join(root, "AGENTS.md"))
	for _, want := range []string{
		"Do not write server code in TypeScript",
		"`go tool skgo check` (or the `skgo_check` MCP tool)",
		"Run `svelte-autofixer` on Svelte code you write",
		"describe Kit's JavaScript server. In this application that code is Go",
	} {
		if !strings.Contains(agents, want) {
			t.Errorf("AGENTS.md lacks %q", want)
		}
	}

	var mcp any
	if err := json.Unmarshal([]byte(read(t, filepath.Join(root, ".mcp.json"))), &mcp); err != nil {
		t.Fatal(err)
	}
	// Relative commands resolve against the root the client starts in. The
	// Svelte server runs through the frontend's own VitePlus, never npx.
	want := map[string]any{"mcpServers": map[string]any{
		"skgo":   map[string]any{"type": "stdio", "command": "go", "args": []any{"tool", "skgo", "mcp"}},
		"svelte": map[string]any{"type": "stdio", "command": "web/node_modules/.bin/vp", "args": []any{"-C", "web", "dlx", "@sveltejs/mcp"}},
	}}
	if !reflect.DeepEqual(mcp, want) {
		t.Errorf(".mcp.json = %v\nwant %v", mcp, want)
	}
	if !slices.ContainsFunc(result.Next, func(s string) bool { return strings.Contains(s, "just install") }) {
		t.Errorf("next steps %q do not say the Svelte server needs the frontend installed", result.Next)
	}

	installed, err := apply(t, project(t, true))
	if err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(installed.Next, func(s string) bool { return strings.Contains(s, "just install") }) {
		t.Errorf("an installed frontend was told to install: %q", installed.Next)
	}
}

func TestAIToolsNeverOverwritesAgentConfiguration(t *testing.T) {
	all := []string{"AGENTS.md", "CLAUDE.md", ".mcp.json"}
	for _, authored := range all {
		t.Run(authored, func(t *testing.T) {
			root := project(t, true)
			if err := os.WriteFile(filepath.Join(root, authored), []byte("authored\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			result, err := apply(t, root)
			if err == nil || !strings.Contains(err.Error(), authored+" already exists") {
				t.Fatalf("error = %v; want %s named", err, authored)
			}
			if len(result.Changed) != 0 {
				t.Errorf("reported changes %v", result.Changed)
			}
			if got := read(t, filepath.Join(root, authored)); got != "authored\n" {
				t.Errorf("%s = %q", authored, got)
			}
			for _, name := range all {
				if name == authored {
					continue
				}
				if _, err := os.Lstat(filepath.Join(root, name)); !errors.Is(err, os.ErrNotExist) {
					t.Errorf("%s was written beside a refused %s: %v", name, authored, err)
				}
			}
		})
	}
}
