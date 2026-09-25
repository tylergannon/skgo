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
	"flag"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
	"golang.org/x/tools/go/analysis/unitchecker"

	"github.com/tylergannon/skgo/internal/advice"
	"github.com/tylergannon/skgo/internal/check"
	"github.com/tylergannon/skgo/internal/gen"
	"github.com/tylergannon/skgo/internal/newapp"
)

const usage = `usage:
	skgo new [flags] DIR [-- SV_CREATE_OPTIONS]
	                          create a SvelteKit application served by Go
	skgo generate [flags]     generate the glue between the Go server and the SvelteKit app
	skgo check [flags]        check Go, Svelte, lint and formatting without edits
	skgo advice [--json] [SKGO001..SKGO008]
	                          show installed rule guidance and repair examples
	skgo mcp                 serve check and advice tools over stdio MCP
`

func main() {
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
	case "new":
		newProject(os.Args[2:])
	case "generate":
		generate(os.Args[2:])
	case "check":
		checkProject(os.Args[2:])
	case "advice":
		showAdvice(os.Args[2:])
	case "mcp":
		if len(os.Args) != 2 {
			fmt.Fprint(os.Stderr, usage)
			os.Exit(2)
		}
		if err := serveMCP(os.Stdin, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
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
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	root := fs.String("root", ".", "project root containing go.mod")
	web := fs.String("web", "web", "frontend root, relative to --root")
	out := fs.String("out", "", "generated Go bindings directory, relative to --root; detected when omitted")
	jsonOutput := fs.Bool("json", false, "write a structured JSON report")
	_ = fs.Parse(args)
	if fs.NArg() != 0 {
		fs.Usage()
		os.Exit(2)
	}
	report := check.Run(context.Background(), check.Options{Root: *root, Web: *web, Out: *out})
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

func newProject(args []string) {
	fs := flag.NewFlagSet("new", flag.ExitOnError)
	module := fs.String("module", "", "Go module path; defaults to the project name")
	name := fs.String("name", "", "application name; defaults to the target directory name")
	origin := fs.String("origin", "http://127.0.0.1:8080", "public browser origin")
	version := fs.String("skgo-version", "", "skgo module version; defaults to this command's release")
	svAddon := fs.String("sv-addon", "", "sv add-on package spec; a file: directory is packed into an isolated copy, for checkout qualification")
	adapter := fs.String("adapter", "", "runtime adapter package spec; intended for checkout qualification")
	replace := fs.String("skgo-replace", "", "local skgo module replacement; intended for checkout qualification")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: skgo new [flags] DIR [-- SV_CREATE_OPTIONS]")
		fmt.Fprintln(os.Stderr, "\nVitePlus delegates the frontend to sv; skgo adds the Go application server.")
		fmt.Fprintln(os.Stderr, "In a terminal sv asks its own questions. Options after -- are handed to")
		fmt.Fprintln(os.Stderr, "`sv create` as written, for example:")
		fmt.Fprintln(os.Stderr, "\n\tskgo new myapp -- --template demo --types jsdoc --add tailwindcss=plugins:none")
		fmt.Fprintln(os.Stderr, "\nsv's demo template becomes the skgo remote-function example. Without a")
		fmt.Fprintln(os.Stderr, "terminal, what is left open is the minimal TypeScript application.")
		fmt.Fprintln(os.Stderr)
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)
	if fs.NArg() < 1 || (fs.NArg() > 1 && fs.Arg(1) != "--") {
		fs.Usage()
		os.Exit(2)
	}
	var svArgs []string
	if fs.NArg() > 1 {
		svArgs = fs.Args()[2:]
	}
	// VitePlus applies the same rule to decide whether it may prompt.
	interactive := os.Getenv("CI") == "" && term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))

	result, err := newapp.Create(newapp.Options{
		Dir: fs.Arg(0), Module: *module, App: *name, Origin: *origin, SvArgs: svArgs, Interactive: interactive,
		SkgoVersion: *version, SVAddonSpec: *svAddon, AdapterSpec: *adapter, SkgoReplace: *replace,
		Stdout: os.Stdout, Stderr: os.Stderr,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Fprint(os.Stderr, "\n"+result.Instructions())
}

func generate(args []string) {
	fs := flag.NewFlagSet("generate", flag.ExitOnError)
	web := fs.String("web", "../web", "the vite root: the directory holding src/ and package.json")
	out := fs.String("out", ".", "the directory of the generated bindings package")
	pkg := fs.String("package", "", "the name of the generated bindings package; defaults to the base name of --out")
	quiet := fs.Bool("quiet", false, "do not list the files written")
	_ = fs.Parse(args)

	cfg := gen.Config{Web: *web, Out: *out, Package: *pkg}
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
