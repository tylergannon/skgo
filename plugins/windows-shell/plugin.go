// Windows-shell generates C# remotes and creates a Windows 11 WinUI shell.
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
	"github.com/tylergannon/skgo/plugins/windows-shell/windowsapp"
	"github.com/tylergannon/skgo/templateapi"
)

var Version = "dev"
var SkgoVersion = "dev"

type windowsPlugin struct{}

func SKGoPluginV1() templateapi.Plugin { return windowsPlugin{} }
func (windowsPlugin) Describe() templateapi.Descriptor {
	return templateapi.Descriptor{ID: "windows-shell", Version: Version, SkgoVersion: SkgoVersion, Generator: true, Command: "Generate C# clients and prepare a Windows 11 native shell", Addons: []templateapi.Addon{{Name: "windows-app", Summary: "Add an application-owned WinUI 3 and WebView2 shell", Options: []templateapi.Option{{Name: "identity", Required: true, Help: "Windows package identity (letters, digits, dots and hyphens)"}}}}}
}
func (windowsPlugin) Apply(ctx context.Context, r templateapi.Request) (templateapi.Result, error) {
	return windowsapp.Apply(ctx, r)
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
		fmt.Fprintln(r.Stdout, "windows-shell commands:\n  setup --identity ID  Create the Windows 11 WinUI shell\n  prepare              Export SDK and cross-compile the Windows x64 DLL\n  sdk --out PATH       Copy the C#/Zig SDK (default windows/SDK)\nRun windows/build.ps1 on Windows to build, run or package the shell.")
		return nil
	}
	if r.Args[0] == "setup" {
		f := flag.NewFlagSet("windows-shell setup", flag.ContinueOnError)
		f.SetOutput(r.Stderr)
		id := f.String("identity", "", "Windows package identity")
		if err := f.Parse(r.Args[1:]); err != nil {
			return err
		}
		if f.NArg() != 0 {
			return fmt.Errorf("setup accepts flags only")
		}
		result, err := windowsapp.Apply(ctx, templateapi.Request{Addon: "windows-app", Project: r.Project, Options: map[string]string{"identity": *id}})
		for _, n := range result.Changed {
			fmt.Fprintln(r.Stdout, n)
		}
		return err
	}
	if r.Args[0] == "prepare" {
		if len(r.Args) != 1 {
			return fmt.Errorf("prepare accepts no arguments")
		}
		return windowsapp.Prepare(ctx, r.Project.Root, r.Stdout)
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
