package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/tylergannon/skgo/internal/buildinfo"
	"github.com/tylergannon/skgo/internal/plugininstall"
	"github.com/tylergannon/skgo/internal/pluginstore"
	"github.com/tylergannon/skgo/internal/templates"
)

func pluginCommand(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 || (len(args) == 1 && (args[0] == "--help" || args[0] == "-h")) {
		fmt.Fprintln(out, "usage: skgo plugin install MODULE[@QUERY]\n       skgo plugin remove MODULE\n\nInstall trusted Git source for this executable. Requires Git, Go and cgo. Source generation and plugin initialization execute code.\nRemoval affects only this host's managed selection.")
		return nil
	}
	if len(args) != 2 || (args[0] != "install" && args[0] != "remove") {
		return fmt.Errorf("usage: skgo plugin install MODULE[@QUERY] | skgo plugin remove MODULE")
	}
	info, err := buildinfo.Current()
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	o := plugininstall.Options{Host: info, Home: home, CWD: cwd, ManualDirs: os.Getenv("SKGO_PLUGIN_DIRS"), Out: out}
	if args[0] == "remove" {
		return plugininstall.Remove(ctx, args[1], o)
	}
	return plugininstall.Install(ctx, args[1], o)
}

// The installer alone supplies this child process's complete file set. Plugin
// stdout is not the protocol: the result goes to a private file after loading.
func pluginProbe(args []string) error {
	if len(args) != 2 || os.Getenv(pluginstore.ProbeFiles) == "" {
		return fmt.Errorf("internal plugin qualification requires its file set and report")
	}
	candidate, err := filepath.EvalSymlinks(args[0])
	if err != nil {
		return err
	}
	r, err := plugins(io.Discard)
	if err != nil {
		return err
	}
	var selected *templates.Entry
	for i := range r.Entries {
		if r.Entries[i].Path == candidate {
			selected = &r.Entries[i]
			break
		}
	}
	if selected == nil {
		return fmt.Errorf("candidate %s unavailable: %s", candidate, strings.Join(r.Diagnostics, "; "))
	}
	result := plugininstall.Exports{Diagnostics: r.Diagnostics}
	for _, t := range selected.Descriptor.Templates {
		for _, e := range r.Entries {
			if e.Path == candidate {
				continue
			}
			for _, other := range e.Descriptor.Templates {
				if other.Name == t.Name {
					return fmt.Errorf("template %q conflicts between %s and %s; remove a copy before installing", t.Name, candidate, e.Path)
				}
			}
		}
		result.Templates = append(result.Templates, t.Name)
	}
	for _, a := range selected.Descriptor.Addons {
		for _, e := range r.Entries {
			if e.Path == candidate {
				continue
			}
			for _, other := range e.Descriptor.Addons {
				if other.Name == a.Name {
					return fmt.Errorf("add-on %q conflicts between %s and %s; remove a copy before installing", a.Name, candidate, e.Path)
				}
			}
		}
		result.Addons = append(result.Addons, a.Name)
	}
	b, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return os.WriteFile(args[1], b, 0600)
}
