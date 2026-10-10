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
	"os/signal"
	"strings"

	"golang.org/x/tools/go/analysis/unitchecker"

	"github.com/tylergannon/skgo/internal/advice"
	"github.com/tylergannon/skgo/internal/check"
	"github.com/tylergannon/skgo/internal/gen"
)

const usage = `usage:
	skgo new [flags] DIR [-- SV_CREATE_OPTIONS]
	                          create a SvelteKit application served by Go
	skgo add [flags] ADDON    apply a reusable application change
	skgo update [--root DIR] [--vp GLOBAL_VP]   align CLI and application toolchain
	skgo native build [--platform macos|iphone|simulator] [--preset NAME]
	skgo buildinfo [--json]   identify this executing CLI and its Go build
	skgo generate [flags]     generate the glue between the Go server and the SvelteKit app
	skgo dev [flags]          run Vite and the Go application, rebuilding Go as it changes
	skgo check [flags]        check Go, Svelte, lint and formatting without edits
	skgo kit-patch [--web web] (--apply | --check)
	                          configure or verify Kit 3.0.0's declared-Inputs queue correction
	skgo advice [--json] [SKGO001..SKGO008]
	                          show installed rule guidance and repair examples
	skgo mcp                 serve check and advice tools over stdio MCP
`

func main() {
	if len(os.Args) == 2 && (os.Args[1] == "--help" || os.Args[1] == "-h" || os.Args[1] == "help") {
		fmt.Fprint(os.Stdout, usage)
		if r, err := plugins(os.Stderr); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		} else {
			_ = r.Help(os.Stdout, "template", "")
			_ = r.Help(os.Stdout, "add-on", "")
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
	case "update":
		updateCommand(os.Args[2:])
	case "native":
		nativeCommand(os.Args[2:])
	case "buildinfo":
		if err := buildinfoCommand(os.Args[2:], os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "add":
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		if err := addCommand(ctx, os.Args[2:], os.Stdout, os.Stderr); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "env":
		if err := environmentCommand(os.Args[2:], os.Stdin, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "new":
		newProject(os.Args[2:])
	case "generate":
		generate(os.Args[2:])
	case "dev":
		os.Exit(devCommand(os.Args[2:]))
	case "check":
		checkProject(os.Args[2:])
	case "kit-patch":
		kitPatchCommand(os.Args[2:])
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
	rejectMultipleSelections(args)
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	root := fs.String("root", ".", "project root containing go.mod")
	web := fs.String("web", "web", "frontend root, relative to --root")
	out := fs.String("out", "", "generated Go bindings directory, relative to --root; detected when omitted")
	localsPackage := fs.String("locals-package", "", "application locals Go import path (required)")
	localsType := fs.String("locals-type", "Locals", "application locals named struct")
	hookPackage := fs.String("hook-package", "", "optional request hook Go import path")
	hookSymbol := fs.String("hook-symbol", "Handle", "selected request hook symbol")
	jsonOutput := fs.Bool("json", false, "write a structured JSON report")
	_ = fs.Parse(args)
	if fs.NArg() != 0 {
		fs.Usage()
		os.Exit(2)
	}
	report := check.Run(context.Background(), check.Options{Root: *root, Web: *web, Out: *out, LocalsPackage: *localsPackage, LocalsType: *localsType, HookPackage: *hookPackage, HookSymbol: *hookSymbol})
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
	quiet := fs.Bool("quiet", false, "do not list the files written")
	swiftOut := fs.String("swift-out", "", "generated Swift source file")
	var swiftRemotes []string
	fs.Func("swift-remote", "select module#export for Swift and the shared safe-number contract (ordinary query/command; repeatable)", func(value string) error {
		swiftRemotes = append(swiftRemotes, value)
		return nil
	})
	_ = fs.Parse(args)

	cfg := gen.Config{Web: *web, Out: *out, Package: *pkg, LocalsPackage: *localsPackage, LocalsType: *localsType, HookPackage: *hookPackage, HookSymbol: *hookSymbol}
	cfg.SwiftOut, cfg.SwiftRemotes = *swiftOut, swiftRemotes
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
		case "locals-package", "locals-type", "hook-package", "hook-symbol":
			if seen[name] {
				fmt.Fprintf(os.Stderr, "skgo: multiple configured selections for %s\n", name)
				os.Exit(2)
			}
			seen[name] = true
		}
	}
}
