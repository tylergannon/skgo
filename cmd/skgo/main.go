// Command skgo is the tooling for a SvelteKit app served by Go.
//
//	skgo new --starter examples myapp
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

	"github.com/tylergannon/skgo/internal/gen"
	"github.com/tylergannon/skgo/internal/newapp"
)

const usage = `usage:
	skgo new [flags] DIR      create a SvelteKit application served by Go
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
	starter := fs.String("starter", "minimal", "starting point: minimal or examples")
	version := fs.String("skgo-version", "", "skgo module version; defaults to this command's release")
	svAddon := fs.String("sv-addon", "", "sv add-on package spec; a file: directory is packed into an isolated copy, for checkout qualification")
	adapter := fs.String("adapter", "", "runtime adapter package spec; intended for checkout qualification")
	replace := fs.String("skgo-replace", "", "local skgo module replacement; intended for checkout qualification")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: skgo new [flags] DIR")
		fmt.Fprintln(os.Stderr, "\nVitePlus delegates the frontend to sv; skgo adds the Go application server.")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(2)
	}

	result, err := newapp.Create(newapp.Options{
		Dir: fs.Arg(0), Module: *module, App: *name, Origin: *origin, Starter: *starter,
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
