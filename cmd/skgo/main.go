// Command skgo is the tooling for a SvelteKit app served by Go.
//
//	skgo new myapp
//
// scaffolds a project that runs: an ordinary SvelteKit frontend, a Go server
// that owns the socket, and one build gesture.
//
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
	"path/filepath"

	"github.com/tylergannon/skgo/internal/gen"
	"github.com/tylergannon/skgo/internal/newapp"
)

const usage = `usage:
	skgo new [flags] DIR      scaffold a project that runs
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

func newProject(args []string) {
	fs := flag.NewFlagSet("new", flag.ExitOnError)
	module := fs.String("module", "", "the Go module path; defaults to the project's name")
	app := fs.String("name", "", "the project's name; defaults to the base name of DIR")
	origin := fs.String("origin", "", "the URL a browser reaches the app at; defaults to http://127.0.0.1:8080")
	version := fs.String("skgo-version", "", "the version of skgo the project requires; defaults to this skgo's own, or the latest release")
	polytype := fs.String("polytype-version", "", "the version of polytype the project requires")
	quiet := fs.Bool("quiet", false, "do not list the files written")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, "usage: skgo new [flags] DIR\n\n")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)

	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(2)
	}

	opts := newapp.Options{
		Dir:             fs.Arg(0),
		Module:          *module,
		App:             *app,
		Origin:          *origin,
		SkgoVersion:     *version,
		PolytypeVersion: *polytype,
	}
	if !*quiet {
		opts.Logf = logf
	}
	if err := newapp.Create(opts); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if !*quiet {
		fmt.Fprintf(os.Stderr, "\nskgo: built the project in %s. Now:\n\n\tcd %s\n\tmise run build\n\t./bin/%s\n\n",
			fs.Arg(0), fs.Arg(0), name(opts, fs.Arg(0)))
	}
}

// name is what the binary will be called, for the closing instructions.
func name(o newapp.Options, dir string) string {
	if o.App != "" {
		return o.App
	}
	return baseName(dir)
}

func baseName(dir string) string {
	return filepath.Base(dir)
}

func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "skgo: "+format+"\n", args...)
}
