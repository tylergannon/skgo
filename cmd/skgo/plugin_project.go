package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	gen "github.com/tylergannon/skgo/generate"
	"github.com/tylergannon/skgo/internal/buildinfo"
	"github.com/tylergannon/skgo/internal/plugininstall"
	"github.com/tylergannon/skgo/internal/templates"
	"github.com/tylergannon/skgo/templateapi"
)

// Locate the owning Go module from bindings directories as well as its root.
func pluginProjectRoot(dir string) string {
	dir, _ = filepath.Abs(dir)
	for p := dir; ; p = filepath.Dir(p) {
		if _, err := os.Stat(filepath.Join(p, "go.mod")); err == nil {
			return p
		}
		if filepath.Dir(p) == p {
			return dir
		}
	}
}

// Compare the selected module graph, not merely a literal require which Go may
// have upgraded through MVS. Replacements are development pairings only.
func checkPluginApplication(ctx context.Context, root, version string) error {
	cmd := exec.CommandContext(ctx, "go", "list", "-m", "-json", buildinfo.Module)
	cmd.Dir = root
	data, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("resolve application's skgo dependency: %w", err)
	}
	var m struct {
		Version string
		Replace *struct{ Path, Version string }
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	if m.Version != version {
		return fmt.Errorf("application selects skgo %s; plugin host is %s; use matching versions", m.Version, version)
	}
	if m.Replace != nil {
		return fmt.Errorf("prebuilt plugins require an official skgo dependency without a local replacement")
	}
	return nil
}

func pinnedEntry(r *templates.Registry, root string, pin plugininstall.Prebuilt, info buildinfo.Info) (*templates.Entry, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	path, err := plugininstall.InstalledPath(pin, plugininstall.Options{Home: home, CWD: root, Host: info})
	if err != nil {
		return nil, err
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}
	for i := range r.Entries {
		e := &r.Entries[i]
		if e.Path == path && e.Descriptor.ID == pin.ID && e.Descriptor.Version == pin.Version {
			return e, nil
		}
	}
	return nil, fmt.Errorf("pinned plugin %s could not load its verified artifact: %s", pin.ID, strings.Join(r.Diagnostics, "; "))
}

func configureGenerators(ctx context.Context, cfg *gen.Config) error {
	root := pluginProjectRoot(cfg.Out)
	project, _, err := templates.Project(root, "")
	if err != nil {
		return err
	}
	cfg.ProjectName = project.Name
	info, err := buildinfo.Current()
	if err != nil {
		return err
	}
	if err := checkPluginApplication(ctx, root, info.SkgoVersion); err != nil {
		return err
	}
	pins, err := plugininstall.ReadPins(root)
	if err != nil {
		return err
	}
	r, err := pluginsAt(root, os.Stderr)
	if err != nil {
		return err
	}
	for _, pin := range pins {
		selected, err := pinnedEntry(r, root, pin, info)
		if err != nil {
			return err
		}
		if selected.Descriptor.Generator {
			cfg.Generators = append(cfg.Generators, templateapi.GeneratorInvocation{ID: pin.ID, Version: pin.Version, Generator: selected.Plugin.(templateapi.Generator), Options: pin.Options, Selections: pin.Selections})
		}
	}
	if len(cfg.Generators) == 0 {
		return fmt.Errorf("no pinned plugin generators; install a prebuilt generator first")
	}
	return nil
}
