// Windows-shell is a development-time plugin. Its initial surface is C# remote
// generation and SDK export; Windows application tooling is a later feature.
package main

import (
	"context"
	"flag"
	"fmt"
	"path/filepath"

	nativeclient "github.com/tylergannon/skgo/plugins/native-client"
	"github.com/tylergannon/skgo/plugins/native-client/model"
	"github.com/tylergannon/skgo/plugins/windows-shell/csharpgen"
	"github.com/tylergannon/skgo/plugins/windows-shell/sdk"
	"github.com/tylergannon/skgo/templateapi"
)

var Version = "dev"
var SkgoVersion = "dev"

type windowsPlugin struct{}

func SKGoPluginV1() templateapi.Plugin { return windowsPlugin{} }
func (windowsPlugin) Describe() templateapi.Descriptor {
	return templateapi.Descriptor{ID: "windows-shell", Version: Version, SkgoVersion: SkgoVersion, Generator: true, Command: "Generate C# remote clients and export the C#/Zig SDK"}
}
func (windowsPlugin) Apply(context.Context, templateapi.Request) (templateapi.Result, error) {
	return templateapi.Result{}, fmt.Errorf("windows-shell provides C# generation and SDK export; Windows shell setup is not available")
}
func (windowsPlugin) Generate(ctx context.Context, r templateapi.GenerationRequest) (templateapi.GenerationResult, error) {
	if err := ctx.Err(); err != nil {
		return templateapi.GenerationResult{}, err
	}
	calls, err := model.Calls(r.Model)
	if err != nil {
		return templateapi.GenerationResult{}, err
	}
	generated, err := csharpgen.Generate(r.Model.Definitions, r.Model.Roots, calls)
	if err != nil {
		return templateapi.GenerationResult{}, err
	}
	out := r.Options["out"]
	if out == "" {
		out = "windows/Generated.cs"
	}
	return templateapi.GenerationResult{Files: []templateapi.GeneratedFile{{Path: out, Content: generated.Source}}}, nil
}
func (windowsPlugin) RunCommand(ctx context.Context, r templateapi.CommandRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(r.Args) == 0 || r.Args[0] == "help" || r.Args[0] == "-h" || r.Args[0] == "--help" {
		fmt.Fprintln(r.Stdout, "windows-shell commands:\n  sdk --out PATH  Copy the C#/Zig SDK (default windows/SDK)")
		return nil
	}
	if r.Args[0] != "sdk" {
		return fmt.Errorf("unknown windows-shell command %q", r.Args[0])
	}
	f := flag.NewFlagSet("windows-shell sdk", flag.ContinueOnError)
	f.SetOutput(r.Stderr)
	out := f.String("out", "windows/SDK", "Project-relative SDK destination")
	if err := f.Parse(r.Args[1:]); err != nil {
		return err
	}
	if f.NArg() != 0 || !filepath.IsLocal(*out) || filepath.Clean(*out) == "." {
		return fmt.Errorf("sdk requires a project-relative destination")
	}
	return nativeclient.CopySDK(sdk.Resources(), filepath.Join(r.Project.Root, *out), "windows-shell")
}
func main() {}
