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
	if len(args) == 0 || args[0] != "build" {
		fmt.Fprintln(os.Stderr, "usage: skgo native build [--root DIR] [--platform macos|iphone|simulator] [--preset NAME] [--prepare-only] [--jobs N]")
		os.Exit(2)
	}
	fs := flag.NewFlagSet("native build", flag.ExitOnError)
	var o nativebuild.Options
	fs.StringVar(&o.Root, "root", ".", "application root")
	fs.StringVar(&o.Platform, "platform", "macos", "macos, iphone or simulator")
	fs.StringVar(&o.Preset, "preset", "", "application-defined build preset")
	fs.BoolVar(&o.PrepareOnly, "prepare-only", false, "prepare bindings, native libraries and Xcode project without compiling a Debug app")
	fs.IntVar(&o.Jobs, "jobs", 0, "maximum Zig/Xcode build workers; zero uses the tool defaults")
	fs.Parse(args[1:])
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
