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
	"flag"
	"fmt"
	"os"

	"golang.org/x/term"

	"github.com/tylergannon/skgo/internal/gen"
	"github.com/tylergannon/skgo/internal/newapp"
)

const usage = `usage:
	skgo new [flags] DIR [-- SV_CREATE_OPTIONS]
	                          create a SvelteKit application served by Go
	skgo generate [flags]     generate the glue between the Go server and the SvelteKit app
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	switch os.Args[1] {
	case "new":
		newProject(os.Args[2:])
	case "generate":
		generate(os.Args[2:])
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
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
