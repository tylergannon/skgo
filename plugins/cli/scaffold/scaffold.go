// Package scaffold creates application-owned MCP setup once. Generation never
// rewrites these files; applications maintain their configuration and guidance.
package scaffold

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/tylergannon/skgo/templateapi"
)

//go:embed files/main.go.tmpl
var mainTemplate string

// Apply creates missing setup files and preserves existing authored files.
func Apply(ctx context.Context, r templateapi.Request) (result templateapi.Result, err error) {
	if r.Addon != "mcp-server" {
		return result, fmt.Errorf("unknown MCP add-on %q", r.Addon)
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	t, err := template.New("main").Parse(mainTemplate)
	if err != nil {
		return result, err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, r.Project); err != nil {
		return result, err
	}
	main, err := format.Source(buf.Bytes())
	if err != nil {
		return result, err
	}
	files := []struct {
		path string
		data []byte
	}{
		{"cmd/mcp/main.go.tmpl", main},
		{"cmd/mcp/instructions.md", []byte("Use this server to work with " + r.Project.Name + ".\n")},
	}
	// Preflight both files before creating anything. Never follow a symlink.
	for i := range files {
		f := &files[i]
		path := r.Project.Root
		for _, part := range strings.Split(f.path, "/") {
			path = filepath.Join(path, part)
			info, err := os.Lstat(path)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return result, err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return result, fmt.Errorf("%s contains a symlink", f.path)
			}
		}
		if f.path == "cmd/mcp/main.go.tmpl" {
			active := filepath.Join(r.Project.Root, "cmd/mcp/main.go")
			if info, err := os.Lstat(active); err == nil {
				if !info.Mode().IsRegular() {
					return result, fmt.Errorf("cmd/mcp/main.go is not a regular file")
				}
				f.data = nil
				continue
			} else if !os.IsNotExist(err) {
				return result, err
			}
		}
		_, err := os.ReadFile(path)
		if err == nil {

			f.data = nil // Existing files belong to the application.
		} else if !os.IsNotExist(err) {
			return result, err
		}
	}
	for _, f := range files {
		if f.data == nil {
			continue
		}
		if err := ctx.Err(); err != nil {
			return result, err
		}
		path := filepath.Join(r.Project.Root, filepath.FromSlash(f.path))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return result, err
		}
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			return result, err
		}
		result.Changed = append(result.Changed, f.path)
		_, err = file.Write(f.data)
		closeErr := file.Close()
		if err != nil {
			return result, err
		}
		if closeErr != nil {
			return result, closeErr
		}
	}
	result.Next = []string{"The inactive main.go.tmpl keeps normal builds and go mod tidy working until MCP setup is ready.", "Add the MCP runtime module to your application with go get github.com/tylergannon/skgo/plugins/cli@VERSION, using your installed plugin release.", "Mark remotes with clipublish.Tool, then run skgo generate --plugins with your usual generation flags.", "Maintain cmd/mcp/main.go and instructions.md for application setup, authentication and guidance.", "After generating, activate once: mv cmd/mcp/main.go.tmpl cmd/mcp/main.go. Run go mod tidy, then go build -o task-mcp ./cmd/mcp."}
	return result, nil
}
