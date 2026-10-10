// Command skgo-plugin-build compiles external plugin source for an existing skgo
// executable without rebuilding that host or editing either project's module.
package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/tylergannon/skgo/internal/pluginbuild"
	"os"
	"os/signal"
)

func main() {
	var o pluginbuild.Options
	flag.StringVar(&o.Host, "host", "", "path to a standalone skgo executable")
	flag.StringVar(&o.Project, "project", "", "project directory whose go tool skgo is the host")
	flag.StringVar(&o.Source, "source", "", "plugin source module directory")
	flag.StringVar(&o.Package, "package", ".", "plugin main package relative to source")
	flag.StringVar(&o.Output, "out", "", "output .so path")
	flag.StringVar(&o.SkgoSource, "skgo-source", "", "explicit skgo source checkout for an unreleased host")
	flag.StringVar(&o.VersionSymbol, "version-symbol", "main.skgoVersion", "string variable filled with the actual host skgo version")
	flag.Parse()
	if flag.NArg() != 0 {
		flag.Usage()
		os.Exit(2)
	}
	o.Log = os.Stderr
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := pluginbuild.Build(ctx, o); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
