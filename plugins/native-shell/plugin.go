// Native-shell is an independently versioned development-time skgo plugin.
// It emits Swift remote clients and optionally scaffolds/builds the existing
// Apple application shell. Generated applications never load this Go plugin.
package main

import (
	"context"
	"flag"
	"fmt"
	"path/filepath"

	"github.com/tylergannon/skgo/plugins/native-client/model"
	"github.com/tylergannon/skgo/plugins/native-shell/nativeapp"
	"github.com/tylergannon/skgo/plugins/native-shell/nativebuild"
	"github.com/tylergannon/skgo/plugins/native-shell/sdk"
	"github.com/tylergannon/skgo/plugins/native-shell/swiftgen"
	"github.com/tylergannon/skgo/templateapi"
)

// Version and SkgoVersion are set independently by the native release build.
var Version = "dev"
var SkgoVersion = "dev"

type nativePlugin struct{}

// SKGoPluginV1 is the public factory loaded by the exact official skgo host.
func SKGoPluginV1() templateapi.Plugin { return nativePlugin{} }
func (nativePlugin) Describe() templateapi.Descriptor {
	d := nativeapp.Plugin(Version).Describe()
	d.SkgoVersion = SkgoVersion
	d.Generator = true
	d.Command = "Generate Swift clients and prepare or build the Apple application shell"
	return d
}
func (nativePlugin) Apply(ctx context.Context, r templateapi.Request) (templateapi.Result, error) {
	return nativeapp.Plugin(Version).Apply(ctx, r)
}
func (nativePlugin) Generate(ctx context.Context, r templateapi.GenerationRequest) (templateapi.GenerationResult, error) {
	if err := ctx.Err(); err != nil {
		return templateapi.GenerationResult{}, err
	}
	calls, err := model.Calls(r.Model)
	if err != nil {
		return templateapi.GenerationResult{}, err
	}
	result, err := swiftgen.Generate(r.Model.Definitions, r.Model.Roots, calls)
	if err != nil {
		return templateapi.GenerationResult{}, err
	}
	out := r.Options["out"]
	if out == "" {
		out = "native/Generated.swift"
	}
	return templateapi.GenerationResult{Files: []templateapi.GeneratedFile{{Path: out, Content: result.Source}}}, nil
}
func (nativePlugin) RunCommand(ctx context.Context, r templateapi.CommandRequest) error {
	if len(r.Args) == 0 || r.Args[0] == "help" || r.Args[0] == "-h" || r.Args[0] == "--help" {
		fmt.Fprintln(r.Stdout, "native-shell commands:\n  setup --bundle-id ID   Create the existing Apple shell explicitly\n  prepare [build flags]  Prepare the Xcode project\n  build [build flags]    Build the native application\n  sdk --out PATH         Copy the Swift/Zig SDK for client-only use\nBuild flags: --platform macos|iphone|simulator --preset NAME --jobs N")
		return nil
	}
	f := flag.NewFlagSet("native-shell "+r.Args[0], flag.ContinueOnError)
	f.SetOutput(r.Stderr)
	switch r.Args[0] {
	case "setup":
		bundle := f.String("bundle-id", "", "Application bundle identifier")
		if err := f.Parse(r.Args[1:]); err != nil {
			return err
		}
		if f.NArg() != 0 {
			return fmt.Errorf("setup accepts flags only")
		}
		result, err := nativeapp.Plugin(Version).Apply(ctx, templateapi.Request{Addon: "native-app", Project: r.Project, Options: map[string]string{"bundle-id": *bundle}})
		for _, name := range result.Changed {
			fmt.Fprintln(r.Stdout, name)
		}
		return err
	case "build", "prepare":
		platform := f.String("platform", "macos", "macos, iphone or simulator")
		preset := f.String("preset", "", "Configuration preset")
		jobs := f.Int("jobs", 0, "Worker limit; zero uses tool defaults")
		if err := f.Parse(r.Args[1:]); err != nil {
			return err
		}
		if f.NArg() != 0 {
			return fmt.Errorf("build accepts flags only")
		}
		return nativebuild.Build(ctx, nativebuild.Options{Root: r.Project.Root, Project: r.Project, Host: r.Host, SDK: sdk.Resources(), Platform: *platform, Preset: *preset, Jobs: *jobs, Out: r.Stdout, PrepareOnly: r.Args[0] == "prepare"})
	case "sdk":
		out := f.String("out", "native/SDK", "Project-relative SDK destination")
		if err := f.Parse(r.Args[1:]); err != nil {
			return err
		}
		if f.NArg() != 0 || !filepath.IsLocal(*out) || filepath.Clean(*out) == "." {
			return fmt.Errorf("sdk requires a project-relative destination")
		}
		return nativebuild.CopySDK(sdk.Resources(), filepath.Join(r.Project.Root, *out))
	default:
		return fmt.Errorf("unknown native-shell command %q", r.Args[0])
	}
}
func main() {}
