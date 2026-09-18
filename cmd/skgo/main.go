// Command skgo is the tooling for a SvelteKit app served by Go.
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

	"github.com/tylergannon/skgo/internal/gen"
)

const usage = `usage:
	skgo generate [flags]     generate the glue between the Go server and the SvelteKit app
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	switch os.Args[1] {
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
func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "skgo: "+format+"\n", args...)
}
