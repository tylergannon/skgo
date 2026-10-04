package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/tylergannon/skgo/internal/kitpatch"
)

func kitPatchCommand(args []string) {
	fs := flag.NewFlagSet("kit-patch", flag.ExitOnError)
	web := fs.String("web", "web", "selected frontend root containing package.json")
	workspace := fs.String("workspace", "", "assert the selected frontend is this root; parent workspaces are never modified")
	apply := fs.Bool("apply", false, "pin Kit 3.0.0 and configure the project-local pnpm patch (does not install)")
	check := fs.Bool("check", false, "verify configured files and actual installed Kit queue bytes")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: go tool skgo kit-patch [--web web] [--workspace web] (--apply | --check)")
		fmt.Fprintln(os.Stderr, "\n--workspace, when supplied, must equal --web. Parent workspace mutation is refused.")
		fmt.Fprintln(os.Stderr, "--apply configures files only; run the frontend's VitePlus install, then use --check.")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if fs.NArg() != 0 || *apply == *check {
		fs.Usage()
		os.Exit(2)
	}
	opts := kitpatch.Options{Web: *web, Workspace: *workspace, Out: os.Stdout}
	var err error
	if *apply {
		err = kitpatch.Configure(opts)
	} else {
		err = kitpatch.Verify(opts)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
