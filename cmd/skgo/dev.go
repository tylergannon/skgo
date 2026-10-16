package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/tylergannon/skgo/internal/devserver"
)

func devCommand(ctx context.Context, args []string, out, stderr io.Writer) error {
	fs := flag.NewFlagSet("dev", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "application root containing go.mod")
	web := fs.String("web", "web", "frontend root relative to --root")
	bindings := fs.String("out", "", "generated bindings directory; detected when omitted")
	address := fs.String("listen", "127.0.0.1:8080", "public listener address (next free port when occupied)")
	strict := fs.Bool("strict-port", false, "fail when the requested port is occupied")
	goTool := fs.String("go", "go", "Go executable")
	generatorConfig := fs.String("generator-config", "", "optional application generator JSON configuration, relative to --root")
	vp := fs.String("vp", "vp", "VitePlus executable")
	if e := fs.Parse(args); e != nil {
		if errors.Is(e, flag.ErrHelp) {
			return nil
		}
		return e
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("skgo dev accepts flags, not positional arguments")
	}
	err := devserver.Run(ctx, devserver.Options{GeneratorConfig: *generatorConfig, Root: *root, Web: *web, Out: *bindings, Listen: *address, StrictPort: *strict, GoTool: *goTool, VitePlus: *vp, Output: out})
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}
