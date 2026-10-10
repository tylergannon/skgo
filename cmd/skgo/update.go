package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/tylergannon/skgo/internal/update"
	"os"
	"os/signal"
)

func updateCommand(args []string) {
	fs := flag.NewFlagSet("update", flag.ExitOnError)
	var o update.Options
	fs.StringVar(&o.Root, "root", "", "application root; otherwise locate the current application")
	fs.StringVar(&o.VP, "vp", "", "global vp executable; defaults to PATH")
	fs.StringVar(&o.NativePlatform, "native-platform", "macos", "native build target when native configuration exists: macos, iphone or simulator")
	fs.StringVar(&o.NativePreset, "native-preset", "", "native build preset from the application configuration")
	fs.StringVar(&o.CompleteVersion, "complete-version", "", "internal completion phase: must match this executable")
	fs.Parse(args)
	if fs.NArg() != 0 {
		fs.Usage()
		os.Exit(2)
	}
	o.Out = os.Stdout
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := update.Run(ctx, o); err != nil {
		fmt.Fprintln(os.Stderr, "skgo update incomplete:", err)
		os.Exit(1)
	}
}
