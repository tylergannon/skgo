package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/tylergannon/skgo/internal/buildinfo"
	"github.com/tylergannon/skgo/internal/plugininstall"
	"github.com/tylergannon/skgo/internal/pluginstore"
	"github.com/tylergannon/skgo/internal/templates"
	"github.com/tylergannon/skgo/templateapi"
)

func pluginCommand(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 || (len(args) == 1 && (args[0] == "--help" || args[0] == "-h")) {
		fmt.Fprintln(out, "usage: skgo plugin install MODULE[@QUERY]\n       skgo plugin install --prebuilt ARTIFACT --sha256 DIGEST [--set NAME=VALUE] [--select MODULE#EXPORT]\n       skgo plugin restore\n       skgo plugin remove MODULE\n       skgo plugin <plugin-id> ...\n\nPrebuilt plugins pin an exact release and checksum in skgo-plugins.json. Restore downloads those bytes without setup. Legacy source installation requires Git, Go and cgo. Plugins execute trusted local code.")
		r, err := plugins(os.Stderr)
		if err != nil {
			return err
		}
		return r.CommandHelp(out)
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
	root := pluginProjectRoot(cwd)
	o := plugininstall.Options{Host: info, Home: home, CWD: root, ManualDirs: os.Getenv("SKGO_PLUGIN_DIRS"), Out: out}
	switch args[0] {
	case "restore":
		if len(args) != 1 {
			return fmt.Errorf("usage: skgo plugin restore")
		}
		return plugininstall.RestorePrebuilt(ctx, o)
	case "remove":
		if len(args) != 2 {
			return fmt.Errorf("usage: skgo plugin remove MODULE")
		}
		return plugininstall.Remove(ctx, args[1], o)
	case "install":
		if len(args) == 2 && !strings.HasPrefix(args[1], "-") {
			return plugininstall.Install(ctx, args[1], o)
		}
		fs := flag.NewFlagSet("plugin install", flag.ContinueOnError)
		fs.SetOutput(out)
		artifact := fs.String("prebuilt", "", "exact release artifact URL or local file")
		sum := fs.String("sha256", "", "expected artifact checksum")
		options := optionValues{}
		fs.Var(options, "set", "generator option NAME=VALUE (repeatable)")
		var selections []string
		fs.Func("select", "remote module#export selection (repeatable)", func(value string) error { selections = append(selections, value); return nil })
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 0 || *artifact == "" || *sum == "" {
			return fmt.Errorf("install --prebuilt requires ARTIFACT and --sha256 DIGEST")
		}
		value := *artifact
		if !strings.Contains(value, "://") {
			value, err = filepath.Abs(value)
			if err != nil {
				return err
			}
		}
		pin, err := plugininstall.ParseArtifact(value)
		if err != nil {
			return err
		}
		pin.SHA256 = *sum
		pin.Options = options
		pin.Selections = selections
		if err := checkPluginApplication(ctx, root, info.SkgoVersion); err != nil {
			return err
		}
		return plugininstall.InstallPrebuilt(ctx, pin, o)
	}
	p, _, err := templates.Project(root, "")
	if err != nil {
		return err
	}
	if err := checkPluginApplication(ctx, root, info.SkgoVersion); err != nil {
		return err
	}
	r, err := plugins(os.Stderr)
	if err != nil {
		return err
	}
	pins, err := plugininstall.ReadPins(root)
	if err != nil {
		return err
	}
	for _, pin := range pins {
		if pin.ID == args[0] {
			if _, err := pinnedEntry(r, root, pin, info); err != nil {
				return err
			}
		}
	}
	return r.RunCommand(ctx, args[0], templateapi.CommandRequest{Project: p, Args: args[1:], Stdin: os.Stdin, Stdout: out, Stderr: os.Stderr, Host: info.Executable})
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
	result := plugininstall.Exports{ID: selected.Descriptor.ID, Version: selected.Descriptor.Version, Diagnostics: r.Diagnostics}
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
