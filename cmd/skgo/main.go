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
	if len(os.Args) == 2 && (os.Args[1] == "--help" || os.Args[1] == "-h" || os.Args[1] == "help") {
		fmt.Fprint(os.Stdout, usage)
		if r, err := plugins(os.Stderr); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		} else {
			_ = r.Help(os.Stdout, "template", "")
			_ = r.Help(os.Stdout, "add-on", "")
			_ = r.CommandHelp(os.Stdout)
		}
		return
	}
	// go vet invokes this binary as an analysis driver with leading flags.
	if len(os.Args) > 1 && strings.HasPrefix(os.Args[1], "-") {
		unitchecker.Main(advice.Analyzer)
		return
	}
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	switch os.Args[1] {
	case "build":
		exit(buildCommand(os.Args[2:]))
	case "plugin":
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		exit(pluginCommand(ctx, os.Args[2:], os.Stdout))
	case "_plugin-probe":
		exit(pluginProbe(os.Args[2:]))
	case "update":
		updateCommand(os.Args[2:])
	case "buildinfo":
		exit(buildinfoCommand(os.Args[2:], os.Stdout))
	case "add":
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		exit(addCommand(ctx, os.Args[2:], os.Stdout, os.Stderr))
	case "env":
		exit(environmentCommand(os.Args[2:], os.Stdin, os.Stdout))
	case "new":
		newProject(os.Args[2:])
	case "dev":
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		exit(devCommand(ctx, os.Args[2:], os.Stdout, os.Stderr))
	case "generate":
		generate(os.Args[2:])
	case "check":
		checkProject(os.Args[2:])
	case "kit-patch":
		kitPatchCommand(os.Args[2:])
	case "advice":
		showAdvice(os.Args[2:])
	case "mcp":
		fs := flag.NewFlagSet("mcp", flag.ExitOnError)
		fs.Usage = commandUsage(fs, "mcp")
		_ = fs.Parse(os.Args[2:])
		if fs.NArg() != 0 {
			fs.Usage()
			os.Exit(2)
		}
		exit(serveMCP(os.Stdin, os.Stdout))
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
}

// exit ends a command that returns its error. flag.ErrHelp from a
// ContinueOnError flag set means the help the caller asked for was printed,
// which succeeds exactly as an ExitOnError flag set does.
func exit(err error) {
	if err != nil && !errors.Is(err, flag.ErrHelp) {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
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

func showAdvice(args []string) {
	fs := flag.NewFlagSet("advice", flag.ExitOnError)
	jsonOutput := fs.Bool("json", false, "write structured JSON guidance")
	_ = fs.Parse(args)
	if fs.NArg() > 1 {
		fs.Usage()
		os.Exit(2)
	}
	entries := advice.Catalog()
	if fs.NArg() == 1 {
		entry, ok := advice.Lookup(fs.Arg(0))
		if !ok {
			fmt.Fprintf(os.Stderr, "unknown skgo advice code %q\n", fs.Arg(0))
			os.Exit(2)
		}
		entries = []advice.Entry{entry}
	}
	if *jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(entries)
		return
	}
	for _, entry := range entries {
		fmt.Printf("%s: %s (skgo %s; Kit %s)\n%s\nRepair: %s\nExample:\n%s\n\n", entry.Code, entry.Title, entry.Version, entry.KitVersion, entry.Consequence, entry.Repair, entry.Example)
	}
}

func checkProject(args []string) {
	rejectMultipleSelections(args)
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	root := fs.String("root", ".", "project root containing go.mod")
	web := fs.String("web", "web", "frontend root, relative to --root")
	out := fs.String("out", "", "generated Go bindings directory, relative to --root; detected when omitted")
	localsPackage := fs.String("locals-package", "", "application locals Go import path (required)")
	localsType := fs.String("locals-type", "Locals", "application locals named struct")
	hookPackage := fs.String("hook-package", "", "optional request hook Go import path")
	hookSymbol := fs.String("hook-symbol", "Handle", "selected request hook symbol")
	configPackage := fs.String("config-package", "", "optional shared ApplicationConfig Go import path")
	configSymbol := fs.String("config-symbol", "Config", "selected ApplicationConfig value")
	jsonOutput := fs.Bool("json", false, "write a structured JSON report")
	_ = fs.Parse(args)
	if fs.NArg() != 0 {
		fs.Usage()
		os.Exit(2)
	}
	report := check.Run(context.Background(), check.Options{Root: *root, Web: *web, Out: *out, LocalsPackage: *localsPackage, LocalsType: *localsType, HookPackage: *hookPackage, HookSymbol: *hookSymbol, ConfigPackage: *configPackage, ConfigSymbol: *configSymbol})
	if *jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(report)
	} else {
		fmt.Print(check.RenderHuman(report))
	}
	if !report.OK {
		os.Exit(1)
	}
}

func generate(args []string) {
	rejectMultipleSelections(args)
	fs := flag.NewFlagSet("generate", flag.ExitOnError)
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
	_ = fs.Parse(args)

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
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if !*quiet {
		cfg.Logf = logf
	}

	if err := gen.Run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "skgo: "+format+"\n", args...)
}

// A selected application type and hook are single explicit declarations.
func rejectMultipleSelections(args []string) {
	seen := map[string]bool{}
	for _, arg := range args {
		name, _, _ := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		switch name {
		case "locals-package", "locals-type", "hook-package", "hook-symbol", "config-package", "config-symbol":
			if seen[name] {
				fmt.Fprintf(os.Stderr, "skgo: multiple configured selections for %s\n", name)
				os.Exit(2)
			}
			seen[name] = true
		}
	}
}
