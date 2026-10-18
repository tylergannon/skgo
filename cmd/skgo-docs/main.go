// Command skgo-docs renders skgo's documentation source tree and checks it.
//
//	skgo-docs render   write documentation/rendered from documentation/docs
//	skgo-docs check    report every problem and every drifted rendered file
//
// Run from anywhere inside the module. The checks are the ones
// go test ./documentation runs; this command prints them as file:line.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/tylergannon/skgo/documentation/render"
)

func main() {
	if len(os.Args) != 2 || (os.Args[1] != "render" && os.Args[1] != "check") {
		fmt.Fprintln(os.Stderr, "usage: skgo-docs render | check")
		os.Exit(2)
	}
	root, err := render.FindRoot(".")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	dir := filepath.Join(root, "documentation", "rendered")
	out, problems := render.Render(root)
	for _, p := range problems {
		fmt.Fprintln(os.Stderr, p)
	}
	switch os.Args[1] {
	case "check":
		drift := render.Drift(dir, out)
		for _, d := range drift {
			fmt.Fprintln(os.Stderr, "documentation/rendered/"+d)
		}
		if len(problems) > 0 || len(drift) > 0 {
			fmt.Fprintf(os.Stderr, "%d problems, %d drifted files\n", len(problems), len(drift))
			os.Exit(1)
		}
		fmt.Printf("%d files render cleanly and match documentation/rendered\n", len(out))
	case "render":
		if len(problems) > 0 {
			fmt.Fprintf(os.Stderr, "%d problems; nothing written\n", len(problems))
			os.Exit(1)
		}
		if err := render.Write(dir, out); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("rendered %d files into documentation/rendered\n", len(out))
	}
}
