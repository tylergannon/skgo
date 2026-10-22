package main

import (
	"flag"
	"fmt"
	"io"

	"github.com/tylergannon/skgo/internal/kitpatch"
)

func kitPatchCommand(args []string, out, stderr io.Writer) error {
	fs := flag.NewFlagSet("kit-patch", flag.ContinueOnError)
	fs.SetOutput(stderr)
	web := fs.String("web", "web", "selected frontend root containing package.json")
	workspace := fs.String("workspace", "", "assert the selected frontend is this root; parent workspaces are never modified")
	apply := fs.Bool("apply", false, "pin the supported Kit release and configure the project-local pnpm patch (does not install)")
	check := fs.Bool("check", false, "verify configured files and actual installed Kit queue bytes")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: go tool skgo kit-patch [--web web] [--workspace web] (--apply | --check)")
		fmt.Fprintln(stderr, "\n--workspace, when supplied, must equal --web. Parent workspace mutation is refused.")
		fmt.Fprintln(stderr, "--apply configures files only; run the frontend's VitePlus install, then use --check.")
		fs.PrintDefaults()
	}
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 0 || *apply == *check {
		fs.Usage()
		return commandExit(2)
	}
	opts := kitpatch.Options{Web: *web, Workspace: *workspace, Out: out}
	var err error
	if *apply {
		err = kitpatch.Configure(opts)
	} else {
		err = kitpatch.Verify(opts)
	}
	if err != nil {
		return err
	}
	return nil
}
