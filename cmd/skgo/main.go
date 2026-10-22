// Command skgo is the tooling for a SvelteKit app served by Go.
//
//	skgo new myapp
//	skgo new myapp -- --template demo --types jsdoc --add tailwindcss=plugins:none
//	skgo generate --web ../web
//
// generates the glue between the two. Run it from the generated bindings
// package with a `go:generate` directive:
//
//	//go:generate go tool skgo generate --web ../web
//
// It reads every `*.remote.go` under the app's `src/`, and writes the
// `.remote.ts` modules kit compiles, the TypeScript declarations their callers
// see, and the Go registration the server mounts.
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
	"syscall"

	"golang.org/x/tools/go/analysis/unitchecker"

	gen "github.com/tylergannon/skgo/generate"
	"github.com/tylergannon/skgo/internal/advice"
	"github.com/tylergannon/skgo/internal/check"
	"github.com/tylergannon/skgo/internal/commands"
)

var usage = commands.Usage()

func main() {
	// go vet invokes this binary as an analysis driver with leading flags.
	if len(os.Args) > 1 && strings.HasPrefix(os.Args[1], "-") && os.Args[1] != "--help" && os.Args[1] != "-h" {
		unitchecker.Main(advice.Analyzer)
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

// run is the same dispatcher main uses, with process exit kept at the boundary.
func run(ctx context.Context, args []string, in io.Reader, out, stderr io.Writer) int {
	err := runCommand(ctx, args, in, out, stderr)
	if err == nil || errors.Is(err, flag.ErrHelp) {
		return 0
	}
	var status commandExit
	if errors.As(err, &status) {
		if err != status {
			fmt.Fprintln(stderr, err)
		}
		return int(status)
	}
	fmt.Fprintln(stderr, err)
	return 1
}

type commandFailure struct {
	status  commandExit
	message string
}

func (e commandFailure) Error() string { return e.message }
func (e commandFailure) Unwrap() error { return e.status }

type commandExit int

func (s commandExit) Error() string { return fmt.Sprintf("command exited with status %d", s) }

// parseFlags preserves ExitOnError's usage-error status while returning help
// and parse failures to the dispatcher instead of terminating the process.
func parseFlags(fs *flag.FlagSet, args []string) error {
	err := fs.Parse(args)
	if err != nil && !errors.Is(err, flag.ErrHelp) {
		return commandExit(2)
	}
	return err
}

func runCommand(ctx context.Context, args []string, in io.Reader, out, stderr io.Writer) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h" || args[0] == "help") {
		fmt.Fprint(out, usage)
		r, err := plugins(stderr)
		if err != nil {
			return err
		}
		_ = r.Help(out, "template", "")
		_ = r.Help(out, "add-on", "")
		_ = r.CommandHelp(out)
		return nil
	}
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return commandExit(2)
	}
	switch args[0] {
	case "build":
		return buildCommand(args[1:], out, stderr)
	case "plugin":
		return pluginCommand(ctx, args[1:], out)
	case "_plugin-probe":
		return pluginProbe(args[1:])
	case "update":
		return updateCommand(args[1:], out, stderr)
	case "buildinfo":
		return buildinfoCommand(args[1:], out, stderr)
	case "add":
		return addCommand(ctx, args[1:], out, stderr)
	case "env":
		return environmentCommand(args[1:], in, out, stderr)
	case "new":
		return newProject(ctx, args[1:], out, stderr)
	case "dev":
		return devCommand(ctx, args[1:], out, stderr)
	case "generate":
		return generate(args[1:], stderr)
	case "check":
		return checkProject(args[1:], out, stderr)
	case "kit-patch":
		return kitPatchCommand(args[1:], out, stderr)
	case "advice":
		return showAdvice(args[1:], out, stderr)
	case "mcp":
		fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
		fs.SetOutput(stderr)
		fs.Usage = commandUsage(fs, "mcp")
		if err := parseFlags(fs, args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 0 {
			fs.Usage()
			return commandExit(2)
		}
		return serveMCP(in, out)
	default:
		fmt.Fprint(stderr, usage)
		return commandExit(2)
	}
}

// commandUsage prints a command's synopsis and description from the command
// table, followed by the flag set's defaults when it defines any.
func commandUsage(fs *flag.FlagSet, name string) func() {
	c, ok := commands.Lookup(name)
	if !ok {
		panic("skgo: command table has no " + name)
	}
	return func() {
		w := fs.Output()
		fmt.Fprintf(w, "usage: %s\n\n%s\n", c.Synopsis(), strings.ReplaceAll(c.Description, "`", ""))
		if len(c.Flags) > 0 {
			fmt.Fprintln(w, "\nflags:")
			fs.PrintDefaults()
		}
	}
}

func showAdvice(args []string, out, stderr io.Writer) error {
	fs := flag.NewFlagSet("advice", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOutput := fs.Bool("json", false, "write structured JSON guidance")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 1 {
		fs.Usage()
		return commandExit(2)
	}
	entries := advice.Catalog()
	if fs.NArg() == 1 {
		entry, ok := advice.Lookup(fs.Arg(0))
		if !ok {
			fmt.Fprintf(stderr, "unknown skgo advice code %q\n", fs.Arg(0))
			return commandExit(2)
		}
		entries = []advice.Entry{entry}
	}
	if *jsonOutput {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(entries)
		return nil
	}
	for _, entry := range entries {
		fmt.Fprintf(out, "%s: %s (skgo %s; Kit %s)\n%s\nRepair: %s\nExample:\n%s\n\n", entry.Code, entry.Title, entry.Version, entry.KitVersion, entry.Consequence, entry.Repair, entry.Example)
	}
	return nil
}

func checkProject(args []string, out, stderr io.Writer) error {
	if err := rejectMultipleSelections(args); err != nil {
		return err
	}
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "project root containing go.mod")
	web := fs.String("web", "web", "frontend root, relative to --root")
	bindings := fs.String("out", "", "generated Go bindings directory, relative to --root; detected when omitted")
	localsPackage := fs.String("locals-package", "", "application locals Go import path (required)")
	localsType := fs.String("locals-type", "Locals", "application locals named struct")
	hookPackage := fs.String("hook-package", "", "optional request hook Go import path")
	hookSymbol := fs.String("hook-symbol", "Handle", "selected request hook symbol")
	configPackage := fs.String("config-package", "", "optional shared ApplicationConfig Go import path")
	configSymbol := fs.String("config-symbol", "Config", "selected ApplicationConfig value")
	jsonOutput := fs.Bool("json", false, "write a structured JSON report")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		fs.Usage()
		return commandExit(2)
	}
	report := check.Run(context.Background(), check.Options{Root: *root, Web: *web, Out: *bindings, LocalsPackage: *localsPackage, LocalsType: *localsType, HookPackage: *hookPackage, HookSymbol: *hookSymbol, ConfigPackage: *configPackage, ConfigSymbol: *configSymbol})
	if *jsonOutput {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(report)
	} else {
		fmt.Fprint(out, check.RenderHuman(report))
	}
	if !report.OK {
		return commandExit(1)
	}
	return nil
}

func generate(args []string, stderr io.Writer) error {
	if err := rejectMultipleSelections(args); err != nil {
		return err
	}
	fs := flag.NewFlagSet("generate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	web := fs.String("web", "../web", "the vite root: the directory holding src/ and package.json")
	out := fs.String("out", ".", "the directory of the generated bindings package")
	pkg := fs.String("package", "", "the name of the generated bindings package; defaults to the base name of --out")
	localsPackage := fs.String("locals-package", "", "application locals Go import path (required)")
	localsType := fs.String("locals-type", "Locals", "application locals named struct")
	hookPackage := fs.String("hook-package", "", "optional request hook Go import path")
	hookSymbol := fs.String("hook-symbol", "Handle", "selected request hook symbol")
	configPackage := fs.String("config-package", "", "optional shared ApplicationConfig Go import path")
	configSymbol := fs.String("config-symbol", "Config", "selected ApplicationConfig value")
	cacheDir := fs.String("cache-dir", "", "generation cache directory; defaults to the user cache; off disables caching")
	quiet := fs.Bool("quiet", false, "do not list the files written")
	withPlugins := fs.Bool("plugins", false, "run pinned plugin generators on this official host")
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	cfg := gen.Config{Web: *web, Out: *out, Package: *pkg, LocalsPackage: *localsPackage, LocalsType: *localsType, HookPackage: *hookPackage, HookSymbol: *hookSymbol, ConfigPackage: *configPackage, ConfigSymbol: *configSymbol}
	cfg.CacheDir = *cacheDir
	if *withPlugins {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		cfg.Context = ctx
		var err error
		cfg, err = gen.DeclaredConfig(cfg)
		if err == nil {
			err = configureGenerators(cfg.Context, &cfg)
		}
		if err != nil {
			fmt.Fprintln(stderr, err)
			return commandExit(1)
		}
	}
	if !*quiet {
		cfg.Logf = func(format string, args ...any) { fmt.Fprintf(stderr, "skgo: "+format+"\n", args...) }
	}

	if err := gen.Run(cfg); err != nil {
		fmt.Fprintln(stderr, err)
		return commandExit(1)
	}
	return nil
}

// A selected application type and hook are single explicit declarations.
func rejectMultipleSelections(args []string) error {
	seen := map[string]bool{}
	for _, arg := range args {
		name, _, _ := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		switch name {
		case "locals-package", "locals-type", "hook-package", "hook-symbol", "config-package", "config-symbol":
			if seen[name] {
				return commandFailure{commandExit(2), fmt.Sprintf("skgo: multiple configured selections for %s", name)}
			}
			seen[name] = true
		}
	}
	return nil
}
