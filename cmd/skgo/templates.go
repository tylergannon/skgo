package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/tylergannon/skgo/internal/buildinfo"
	"github.com/tylergannon/skgo/internal/newapp"
	"github.com/tylergannon/skgo/internal/pluginstore"
	"github.com/tylergannon/skgo/internal/templates"
	"github.com/tylergannon/skgo/nativeapp"
	"golang.org/x/term"
)

type optionValues map[string]string

func (v optionValues) String() string { return "" }
func (v optionValues) Set(arg string) error {
	key, value, ok := strings.Cut(arg, "=")
	if !ok || key == "" {
		return fmt.Errorf("--set requires NAME=VALUE")
	}
	if _, ok := v[key]; ok {
		return fmt.Errorf("duplicate --set option %q", key)
	}
	v[key] = value
	return nil
}

func plugins(out io.Writer) (*templates.Registry, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	info, err := buildinfo.Current()
	if err != nil {
		return nil, err
	}
	var files, diagnostics []string
	if private := os.Getenv(pluginstore.ProbeFiles); private != "" {
		if err = json.Unmarshal([]byte(private), &files); err != nil {
			return nil, err
		}
	} else {
		digest, e := pluginstore.Digest(info.Executable)
		if e != nil {
			return nil, e
		}
		files, diagnostics, err = templates.Files(os.Getenv("SKGO_PLUGIN_DIRS"), home, cwd, pluginstore.Dir(home, digest))
		if err != nil {
			return nil, err
		}
		hints, e := pluginstore.Hints(home, digest)
		if e != nil {
			return nil, e
		}
		diagnostics = append(diagnostics, hints...)
	}
	r, err := templates.DiscoverFiles(files, info.SkgoVersion, nativeapp.Plugin(info.SkgoVersion))
	if err != nil {
		return nil, err
	}
	r.Diagnostics = append(diagnostics, r.Diagnostics...)
	for _, d := range r.Diagnostics {
		fmt.Fprintln(out, "warning:", d)
	}
	return r, nil
}

// Only skgo-side help is consumed; everything after -- belongs to sv.
func takeHelp(args []string) ([]string, bool) {
	var clean []string
	help := false
	upstream := false
	for _, a := range args {
		if a == "--" {
			upstream = true
		}
		if !upstream && (a == "--help" || a == "-h") {
			help = true
			continue
		}
		clean = append(clean, a)
	}
	return clean, help
}

func newProject(args []string) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := newCommand(ctx, args, os.Stdout, os.Stderr, newapp.Create, plugins); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newCommand(ctx context.Context, args []string, out, errOut io.Writer, create func(newapp.Options) (newapp.Result, error), load func(io.Writer) (*templates.Registry, error)) error {
	args, help := takeHelp(args)
	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	fs.SetOutput(errOut)
	module := fs.String("module", "", "Go module path; defaults to the project name")
	name := fs.String("name", "", "application name; defaults to the target directory name")
	origin := fs.String("origin", "http://127.0.0.1:8080", "public browser origin")
	version := fs.String("skgo-version", "", "skgo module version; defaults to this command's release")
	svAddon := fs.String("sv-addon", "", "sv add-on package override for checkout qualification")
	adapter := fs.String("adapter", "", "adapter package override for checkout qualification")
	replace := fs.String("skgo-replace", "", "local skgo module replacement for checkout qualification")
	tmpl := fs.String("template", "", "application template applied after the ordinary minimal TypeScript scaffold")
	values := optionValues{}
	fs.Var(values, "set", "template option NAME=VALUE (repeatable)")
	fs.Usage = func() {
		fmt.Fprintln(errOut, "usage: skgo new [flags] DIR [-- SV_CREATE_OPTIONS]\n\nVitePlus delegates creation to sv; skgo adds the Go application server.\nOptions after -- belong to sv create; without a terminal the base is minimal TypeScript.\nExample: skgo new myapp -- --template demo --types jsdoc --add tailwindcss=plugins:none")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	var registry *templates.Registry
	if help || *tmpl != "" {
		var err error
		registry, err = load(errOut)
		if err != nil {
			return err
		}
	}
	if help {
		fs.Usage()
		return registry.Help(errOut, "template", *tmpl)
	}
	if fs.NArg() < 1 || (fs.NArg() > 1 && fs.Arg(1) != "--") {
		return errors.New("usage: skgo new [flags] DIR [-- SV_CREATE_OPTIONS]")
	}
	var svArgs []string
	if fs.NArg() > 1 {
		svArgs = fs.Args()[2:]
	}
	interactive := os.Getenv("CI") == "" && term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
	var ops []templates.Operation
	if *tmpl != "" {
		var err error
		ops, err = registry.Template(*tmpl, values)
		if err != nil {
			return err
		}
		// Template recipes have one fixed base. Explicit sv selections must agree.
		for i := 0; i < len(svArgs); i++ {
			k, v, inline := strings.Cut(svArgs[i], "=")
			if k == "--template" || k == "--types" {
				if !inline {
					i++
					if i >= len(svArgs) {
						return fmt.Errorf("%s requires a value", k)
					}
					v = svArgs[i]
				}
				if (k == "--template" && v != "minimal") || (k == "--types" && v != "ts") {
					return fmt.Errorf("application templates require sv's minimal TypeScript base")
				}
			}
			if k == "--no-types" || k == "--add" {
				return fmt.Errorf("application templates use the fixed minimal TypeScript base; apply optional integrations through add-ons")
			}
		}
		interactive = false
	} else if len(values) > 0 {
		return errors.New("--set requires --template")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	result, err := create(newapp.Options{Dir: fs.Arg(0), Module: *module, App: *name, Origin: *origin, SvArgs: svArgs, Interactive: interactive, SkgoVersion: *version, SVAddonSpec: *svAddon, AdapterSpec: *adapter, SkgoReplace: *replace, Stdout: out, Stderr: errOut})
	if err != nil {
		return err
	}
	if len(ops) > 0 {
		project, _, err := templates.Project(result.Dir, result.App)
		if err != nil {
			return err
		}
		if err := templates.Apply(ctx, ops, project, out); err != nil {
			return err
		}
	}
	fmt.Fprint(errOut, "\n"+result.Instructions())
	return nil
}

func addCommand(ctx context.Context, args []string, out, errOut io.Writer) error {
	args, help := takeHelp(args)
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	fs.SetOutput(errOut)
	root := fs.String("root", ".", "skgo application root")
	name := fs.String("name", "", "explicit application identity for older projects; must match an existing identity")
	values := optionValues{}
	fs.Var(values, "set", "add-on option NAME=VALUE (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 1 {
		return errors.New("skgo add accepts one add-on; put flags before its name")
	}
	r, err := plugins(errOut)
	if err != nil {
		return err
	}
	if help || fs.NArg() == 0 {
		fmt.Fprintln(errOut, "usage: skgo add [--root DIR] [--name NAME] [--set KEY=VALUE] ADDON")
		fs.PrintDefaults()
		return r.Help(errOut, "add-on", fs.Arg(0))
	}
	ops, err := r.Add(fs.Arg(0), values)
	if err != nil {
		return err
	}
	p, missing, err := templates.Project(*root, *name)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if missing {
		if err := templates.SaveIdentity(p); err != nil {
			return err
		}
		fmt.Fprintln(out, "identity: changed skgo.json")
	}
	return templates.Apply(ctx, ops, p, out)
}

func buildinfoCommand(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("buildinfo", flag.ContinueOnError)
	jsonOutput := fs.Bool("json", false, "write executing binary and recorded Go build identity as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("skgo buildinfo accepts no positional arguments")
	}
	info, err := buildinfo.Current()
	if err != nil {
		return err
	}
	if *jsonOutput {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(info)
	}
	fmt.Fprintf(out, "skgo %s\n%s\n%s", info.SkgoVersion, info.Executable, info.Build.String())
	return nil
}
