package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/tylergannon/skgo/internal/nativebuild"
	"os"
	"os/signal"
)

func nativeCommand(args []string) {
	fs := flag.NewFlagSet("native build", flag.ExitOnError)
	var o nativebuild.Options
	fs.StringVar(&o.Root, "root", ".", "application root")
	fs.StringVar(&o.Platform, "platform", "macos", "macos, iphone or simulator")
	fs.StringVar(&o.Preset, "preset", "", "application-defined build preset")
	fs.BoolVar(&o.PrepareOnly, "prepare-only", false, "prepare bindings, native libraries and Xcode project without compiling a Debug app")
	fs.IntVar(&o.Jobs, "jobs", 0, "maximum Zig/Xcode build workers; zero uses the tool defaults")
	fs.Usage = commandUsage(fs, "native build")
	// build is the only native subcommand, so skgo native --help is its help.
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		fs.Usage()
		return
	}
	if len(args) == 0 || args[0] != "build" {
		fs.Usage()
		os.Exit(2)
	}
	_ = fs.Parse(args[1:])
	if fs.NArg() != 0 {
		fs.Usage()
		os.Exit(2)
	}
	o.Out = os.Stdout
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := nativebuild.Build(ctx, o); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
