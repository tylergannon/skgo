package nativeapp

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"go/format"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/tylergannon/skgo/templateapi"
)

//go:embed all:files
var files embed.FS

type plugin struct{ version string }

func Plugin(version string) templateapi.Plugin { return plugin{version} }
func (p plugin) Describe() templateapi.Descriptor {
	return templateapi.Descriptor{ID: "skgo-native", Version: p.version, SkgoVersion: p.version, Addons: []templateapi.Addon{{Name: "native-app", Summary: "Add an embedded Go host and shared macOS/iOS application shell", Options: []templateapi.Option{{Name: "bundle-id", Help: "Application bundle identifier without preset suffix", Required: true}}}}}
}
func (p plugin) Apply(ctx context.Context, req templateapi.Request) (result templateapi.Result, err error) {
	if req.Addon != "native-app" {
		return result, fmt.Errorf("unknown native add-on %q", req.Addon)
	}
	id := req.Options["bundle-id"]
	if !bundleIdentifier.MatchString(id) {
		return result, fmt.Errorf("invalid bundle-id %q", id)
	}
	planned := map[string][]byte{}
	err = fs.WalkDir(files, "files", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		b, err := files.ReadFile(path)
		if err != nil {
			return err
		}
		name := strings.TrimPrefix(path, "files/")
		if strings.HasSuffix(name, ".tmpl") {
			t, err := template.New(name).Parse(string(b))
			if err != nil {
				return err
			}
			var buf bytes.Buffer
			if err := t.Execute(&buf, req.Project); err != nil {
				return err
			}
			b = buf.Bytes()
			name = strings.TrimSuffix(name, ".tmpl")
			if strings.HasSuffix(name, ".go") {
				b, err = format.Source(b)
				if err != nil {
					return err
				}
			}
		}
		planned[name] = b
		return nil
	})
	if err != nil {
		return result, err
	}
	config := Config{Version: 1, BundleID: id}
	planned["native/skgo-native.json"], err = json.MarshalIndent(config, "", "  ")
	if err != nil {
		return result, err
	}
	planned["native/skgo-native.json"] = append(planned["native/skgo-native.json"], '\n')
	// A previously configured native app owns its configuration and view slot.
	// Verify the stable shell before allowing either application-owned file to differ.
	names := make([]string, 0, len(planned))
	for name := range planned {
		names = append(names, name)
	}
	sort.Strings(names)
	configured := false
	configPath := filepath.Join(req.Project.Root, "native/skgo-native.json")
	if b, e := os.ReadFile(configPath); e == nil {
		var prior Config
		if e = json.Unmarshal(b, &prior); e != nil || prior.Version != 1 || prior.BundleID != id {
			return result, fmt.Errorf("native/skgo-native.json conflicts with requested native identity")
		}
		configured = true
	} else if !os.IsNotExist(e) {
		return result, e
	}
	for _, name := range names {
		path := filepath.Join(req.Project.Root, filepath.FromSlash(name))
		// Refuse symlink components before reading or writing outside the application.
		for part := path; part != req.Project.Root; part = filepath.Dir(part) {
			st, e := os.Lstat(part)
			if e == nil && st.Mode()&os.ModeSymlink != 0 {
				return result, fmt.Errorf("%s contains a symlink", name)
			}
			if e != nil && !os.IsNotExist(e) {
				return result, e
			}
		}
		b, e := os.ReadFile(path)
		if e == nil {
			if configured && (name == "native/skgo-native.json" || name == "native/Sources/ApplicationView.swift") {
				delete(planned, name)
				continue
			}
			if !bytes.Equal(b, planned[name]) {
				return result, fmt.Errorf("%s conflicts with existing content", name)
			}
			delete(planned, name)
		} else if !os.IsNotExist(e) {
			return result, e
		}
	}
	for _, name := range names {
		b, ok := planned[name]
		if !ok {
			continue
		}
		if err := ctx.Err(); err != nil {
			return result, err
		}
		path := filepath.Join(req.Project.Root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return result, err
		}
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			return result, err
		}
		result.Changed = append(result.Changed, name)
		_, err = f.Write(b)
		closeErr := f.Close()
		if err != nil {
			return result, err
		}
		if closeErr != nil {
			return result, closeErr
		}
	}
	result.Next = []string{"Build with go tool skgo native build --platform macos (or iphone / simulator)."}
	return result, nil
}
