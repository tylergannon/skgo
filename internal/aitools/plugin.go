// Package aitools supplies the ai-tools add-on: coding-agent instructions and
// MCP servers at the application root, where the Go commands run.
//
// sv's own ai-tools add-on writes its files into the Kit project, which in an
// SKGo application is web/; an agent started at the application root reads
// none of them, and what it would read describes Kit's JavaScript server.
package aitools

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tylergannon/skgo/templateapi"
)

//go:embed instructions.md
var instructions []byte

type plugin struct{ version string }

func Plugin(skgoVersion string) templateapi.AddonPlugin { return plugin{skgoVersion} }

func (p plugin) Describe() templateapi.Descriptor {
	return templateapi.Descriptor{ID: "skgo-ai-tools", Version: p.version, SkgoVersion: p.version, Addons: []templateapi.Addon{{
		Name:    "ai-tools",
		Summary: "Add AGENTS.md, CLAUDE.md and the skgo and Svelte MCP servers at the application root",
	}}}
}

type server struct {
	Type    string   `json:"type"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

func (p plugin) Apply(ctx context.Context, req templateapi.Request) (result templateapi.Result, err error) {
	if req.Addon != "ai-tools" {
		return result, fmt.Errorf("unknown ai-tools add-on %q", req.Addon)
	}
	root := req.Project.Root
	web, err := filepath.Rel(root, req.Project.Web)
	if err != nil || !filepath.IsLocal(web) {
		return result, fmt.Errorf("frontend root must be inside the application root")
	}
	web = filepath.ToSlash(web)
	// The project's own VitePlus, run against web/, keeps the Svelte server on
	// the package manager and registry the frontend declares.
	vp := web + "/node_modules/.bin/vp"
	mcp, err := json.MarshalIndent(map[string]map[string]server{"mcpServers": {
		"skgo":   {Type: "stdio", Command: "go", Args: []string{"tool", "skgo", "mcp"}},
		"svelte": {Type: "stdio", Command: vp, Args: []string{"-C", web, "dlx", "@sveltejs/mcp"}},
	}}, "", "  ")
	if err != nil {
		return result, err
	}
	files := []struct {
		name string
		data []byte
	}{
		{"AGENTS.md", instructions},
		{"CLAUDE.md", []byte("@AGENTS.md\n")},
		{".mcp.json", append(mcp, '\n')},
	}
	var existing []string
	for _, f := range files {
		if _, err := os.Lstat(filepath.Join(root, f.name)); err == nil {
			existing = append(existing, f.name)
		} else if !errors.Is(err, os.ErrNotExist) {
			return result, err
		}
	}
	if len(existing) == 1 {
		return result, fmt.Errorf("%s already exists; ai-tools does not overwrite agent configuration, so move it aside or add what it needs by hand", existing[0])
	} else if len(existing) > 1 {
		return result, fmt.Errorf("%s already exist; ai-tools does not overwrite agent configuration, so move them aside or add what they need by hand", strings.Join(existing, ", "))
	}
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		file, err := os.OpenFile(filepath.Join(root, f.name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return result, err
		}
		result.Changed = append(result.Changed, f.name)
		_, err = file.Write(f.data)
		if err = errors.Join(err, file.Close()); err != nil {
			return result, err
		}
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(vp))); err != nil {
		result.Next = append(result.Next, "the svelte MCP server starts once `just install` has installed "+web+"/node_modules")
	}
	result.Next = append(result.Next, "start coding agents in the application root, where these files are read")
	return result, nil
}
