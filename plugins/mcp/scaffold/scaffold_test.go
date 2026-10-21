package scaffold

import (
	"context"

	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tylergannon/skgo/templateapi"
)

func TestSkeletonBecomesApplicationOwned(t *testing.T) {
	root := t.TempDir()
	req := templateapi.Request{Addon: "mcp-server", Project: templateapi.Project{Root: root, Module: "example.com/app", Name: "Tasks"}}
	got, err := Apply(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Changed) != 2 {
		t.Fatalf("changes=%v", got.Changed)
	}
	path := filepath.Join(root, "cmd/mcp/main.go.tmpl")
	main, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(main), `"example.com/app/internal/mcpclient"`) || !strings.Contains(string(main), "server.WithInstructions(instructions)") {
		t.Fatal(string(main))
	}
	authored := []byte("package main\n// application-owned authentication\nfunc main() {}\n")
	if err := os.WriteFile(path, authored, 0644); err != nil {
		t.Fatal(err)
	}
	doc := filepath.Join(root, "cmd/mcp/instructions.md")
	if err := os.WriteFile(doc, []byte("My application guidance."), 0644); err != nil {
		t.Fatal(err)
	}
	got, err = Apply(context.Background(), req)
	if err != nil || len(got.Changed) != 0 {
		t.Fatalf("result=%+v err=%v", got, err)
	}
	after, _ := os.ReadFile(path)
	instructions, _ := os.ReadFile(doc)
	if string(after) != string(authored) || string(instructions) != "My application guidance." {
		t.Fatal("authored setup overwritten")
	}
}

func TestActivatedServerRemainsApplicationOwned(t *testing.T) {
	root := t.TempDir()
	req := templateapi.Request{Addon: "mcp-server", Project: templateapi.Project{Root: root, Module: "example.com/app", Name: "Tasks"}}
	if _, err := Apply(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	pending := filepath.Join(root, "cmd/mcp/main.go.tmpl")
	active := filepath.Join(root, "cmd/mcp/main.go")
	if err := os.Rename(pending, active); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(pending); !os.IsNotExist(err) {
		t.Fatal("recreated a skeleton alongside the activated server")
	}
}
