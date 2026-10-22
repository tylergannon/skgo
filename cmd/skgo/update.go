package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/tylergannon/skgo/internal/update"
)

func updateCommand(args []string, out, stderr io.Writer) error {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var o update.Options
	fs.StringVar(&o.Root, "root", "", "application root; otherwise locate the current application")
	fs.StringVar(&o.VP, "vp", "", "global vp executable; defaults to PATH")
	fs.StringVar(&o.CompleteVersion, "complete-version", "", "internal completion phase: must match this executable")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		fs.Usage()
		return commandExit(2)
	}
	o.Out = out
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := update.Run(ctx, o); err != nil {
		return fmt.Errorf("skgo update incomplete: %w", err)
	}
	return nil
}
