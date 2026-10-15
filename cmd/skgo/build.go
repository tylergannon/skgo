package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"

	"github.com/tylergannon/skgo/internal/webbuild"
)

func buildCommand(args []string) error {
	flags := flag.NewFlagSet("build", flag.ContinueOnError)
	root := flags.String("root", ".", "application root")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("usage: skgo build [--root DIR]")
	}
	abs, err := filepath.Abs(*root)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return webbuild.Build(abs, os.Stdout, func(dir string, env []string, name string, args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Dir, cmd.Env = dir, append(os.Environ(), env...)
		var output bytes.Buffer
		cmd.Stdout = &output
		if name != "go" || len(args) == 0 || args[0] != "list" {
			cmd.Stdout = io.MultiWriter(&output, os.Stdout)
		}
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("%s %v: %w", name, args, err)
		}
		return output.Bytes(), nil
	})
}
