// Command skgo generates the glue between a Go server and a SvelteKit app.
//
// Run it from the generated bindings package with a `go:generate` directive:
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
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "generate" {
		fmt.Fprintln(os.Stderr, "usage: skgo generate [--web DIR] [--out DIR]")
		os.Exit(2)
	}

	fs := flag.NewFlagSet("generate", flag.ExitOnError)
	web := fs.String("web", "../web", "the vite root: the directory holding src/ and package.json")
	out := fs.String("out", ".", "the directory of the generated bindings package")
	pkg := fs.String("package", "", "the name of the generated bindings package; defaults to the base name of --out")
	quiet := fs.Bool("quiet", false, "do not list the files written")
	_ = fs.Parse(os.Args[2:])

	cfg := gen.Config{Web: *web, Out: *out, Package: *pkg}
	if !*quiet {
		cfg.Logf = func(format string, args ...any) {
			fmt.Fprintf(os.Stderr, "skgo: "+format+"\n", args...)
		}
	}

	if err := gen.Run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
