// Native-shell is an independently versioned development-time skgo plugin.
// It emits Swift and C# remote clients and optionally scaffolds/builds the existing
// Apple application shell. Generated applications never load this Go plugin.
package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/tylergannon/skgo/plugins/native-shell/csharpgen"
	"github.com/tylergannon/skgo/plugins/native-shell/nativeapp"
	"github.com/tylergannon/skgo/plugins/native-shell/nativebuild"
	"github.com/tylergannon/skgo/plugins/native-shell/swiftgen"
	"github.com/tylergannon/skgo/templateapi"
)

// Version and SkgoVersion are set independently by the native release build.
var Version = "dev"
var SkgoVersion = "dev"

//go:embed sdk/core/build.zig sdk/core/build.zig.zon sdk/core/src sdk/core/README.md sdk/swift/Package.swift sdk/swift/Sources sdk/swift/Tests sdk/swift/README.md sdk/csharp/SKGoNative/*.cs sdk/csharp/SKGoNative/*.csproj sdk/csharp/RemoteCLI/*.cs sdk/csharp/RemoteCLI/*.csproj sdk/csharp/README.md sdk/mise.toml sdk/mise.lock
var resources embed.FS

type nativePlugin struct{}

// SKGoPluginV1 is the public factory loaded by the exact official skgo host.
func SKGoPluginV1() templateapi.Plugin { return nativePlugin{} }
func (nativePlugin) Describe() templateapi.Descriptor {
	d := nativeapp.Plugin(Version).Describe()
	d.SkgoVersion = SkgoVersion
	d.Generator = true
	d.Command = "Generate Swift or C# clients and prepare or build the Apple application shell"
	return d
}
func (nativePlugin) Apply(ctx context.Context, r templateapi.Request) (templateapi.Result, error) {
	return nativeapp.Plugin(Version).Apply(ctx, r)
}
func (nativePlugin) Generate(ctx context.Context, r templateapi.GenerationRequest) (templateapi.GenerationResult, error) {
	if err := ctx.Err(); err != nil {
		return templateapi.GenerationResult{}, err
	}
	names := map[string]bool{"core": true, "init": true}
	calls := make([]swiftgen.Call, 0, len(r.Model.Remotes))
	for _, op := range r.Model.Remotes {
		if op.Kind != "query" && op.Kind != "command" {
			return templateapi.GenerationResult{}, fmt.Errorf("%s: %s is a %s; native clients support ordinary queries and commands", op.Source, op.Selection, op.Kind)
		}
		if op.Transported {
			return templateapi.GenerationResult{}, fmt.Errorf("%s: %s uses a custom transport unsupported by the native client", op.Source, op.Selection)
		}
		if op.Output < 0 || op.Output >= len(r.Model.Roots) || op.Input >= len(r.Model.Roots) {
			return templateapi.GenerationResult{}, fmt.Errorf("%s: remote model has unprojected types", op.Selection)
		}
		name := op.Name
		for n := 2; names[name]; n++ {
			name = fmt.Sprintf("%s_%d", op.Name, n)
		}
		names[name] = true
		calls = append(calls, swiftgen.Call{Name: name, ID: op.ID, Kind: op.Kind, Input: op.Input, Output: op.Output})
	}
	language := r.Options["language"]
	if language == "" {
		language = "swift"
	}
	var source []byte
	var err error
	switch language {
	case "swift":
		result, e := swiftgen.Generate(r.Model.Definitions, r.Model.Roots, calls)
		source, err = result.Source, e
	case "csharp":
		result, e := csharpgen.Generate(r.Model.Definitions, r.Model.Roots, calls)
		source, err = result.Source, e
	default:
		return templateapi.GenerationResult{}, fmt.Errorf("unsupported native language %q; use swift or csharp", language)
	}
	if err != nil {
		return templateapi.GenerationResult{}, err
	}
	out := r.Options["out"]
	if out == "" {
		out = "native/Generated.swift"
		if language == "csharp" {
			out = "native/Generated.cs"
		}
	}
	return templateapi.GenerationResult{Files: []templateapi.GeneratedFile{{Path: out, Content: source}}}, nil
}
func (nativePlugin) RunCommand(ctx context.Context, r templateapi.CommandRequest) error {
	if len(r.Args) == 0 || r.Args[0] == "help" || r.Args[0] == "-h" || r.Args[0] == "--help" {
		fmt.Fprintln(r.Stdout, "native-shell commands:\n  setup --bundle-id ID   Create the existing Apple shell explicitly\n  prepare [build flags]  Prepare the Xcode project\n  build [build flags]    Build the native application\n  sdk --out PATH         Copy the Swift/C#/Zig SDK for client-only use\nBuild flags: --platform macos|iphone|simulator --preset NAME --jobs N")
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
		sdk, _ := fs.Sub(resources, "sdk")
		return nativebuild.Build(ctx, nativebuild.Options{Root: r.Project.Root, Project: r.Project, Host: r.Host, SDK: sdk, Platform: *platform, Preset: *preset, Jobs: *jobs, Out: r.Stdout, PrepareOnly: r.Args[0] == "prepare"})
	case "sdk":
		out := f.String("out", "native/SDK", "Project-relative SDK destination")
		if err := f.Parse(r.Args[1:]); err != nil {
			return err
		}
		if f.NArg() != 0 || !filepath.IsLocal(*out) || filepath.Clean(*out) == "." {
			return fmt.Errorf("sdk requires a project-relative destination")
		}
		sdk, _ := fs.Sub(resources, "sdk")
		return nativebuild.CopySDK(sdk, filepath.Join(r.Project.Root, *out))
	default:
		return fmt.Errorf("unknown native-shell command %q", r.Args[0])
	}
}
func main() {}
