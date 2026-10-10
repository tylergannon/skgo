package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tylergannon/skgo/internal/buildinfo"
	"github.com/tylergannon/skgo/internal/newapp"
	"github.com/tylergannon/skgo/internal/pluginbuild"
	"github.com/tylergannon/skgo/internal/templates"
	"github.com/tylergannon/skgo/templateapi"
)

func TestNativePluginHelpApplicationAndProjectTool(t *testing.T) {
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	source := filepath.Join(root, "source")
	plugins := filepath.Join(root, "plugins")
	home := filepath.Join(root, "home")
	for _, d := range []string{source, plugins, filepath.Join(home, ".skgo", "plugins")} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile("testdata/template-plugin/plugin.go")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(source, "main.go"), data, 0644)
	mod := "module example.test/plugin\n\ngo 1.27.1\n\nrequire github.com/tylergannon/skgo v0.0.0\nreplace github.com/tylergannon/skgo => " + repo + "\n"
	os.WriteFile(filepath.Join(source, "go.mod"), []byte(mod), 0644)
	identityJSON, err := exec.Command(skgoBin, "buildinfo", "--json").Output()
	if err != nil {
		t.Fatal(err)
	}
	var hostInfo buildinfo.Info
	if err := json.Unmarshal(identityJSON, &hostInfo); err != nil {
		t.Fatal(err)
	}
	canonicalBin, err := filepath.EvalSymlinks(skgoBin)
	if err != nil {
		t.Fatal(err)
	}
	if hostInfo.Executable != canonicalBin || hostInfo.Build.GoVersion == "" {
		t.Fatalf("host identity: %s", identityJSON)
	}
	var buildLog bytes.Buffer
	if err := pluginbuild.Build(context.Background(), pluginbuild.Options{Host: skgoBin, Source: source, Output: filepath.Join(plugins, "receipt.so"), SkgoSource: repo, VersionSymbol: "main.skgoVersion", Log: &buildLog}); err != nil {
		t.Fatalf("external source-build recipe: %v\n%s", err, buildLog.String())
	}
	if after, err := os.ReadFile(filepath.Join(source, "go.mod")); err != nil || string(after) != mod {
		t.Fatalf("source module was edited: %s %v", after, err)
	}
	call := func(bin string, env []string, args ...string) string {
		t.Helper()
		cmd := exec.Command(bin, args...)
		cmd.Env = append(os.Environ(), env...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	env := []string{"SKGO_PLUGIN_DIRS=" + plugins, "HOME=" + home}
	for _, args := range [][]string{{"--help"}, {"new", "--help"}, {"add", "--help"}, {"add", "receipt", "--help"}, {"new", "--template", "receipt-app", "--help"}} {
		out := call(skgoBin, env, args...)
		if !strings.Contains(out, "loaded eagerly in sorted file order") || strings.Contains(out, "warning:") || (!strings.Contains(out, "Write a configured receipt") && !strings.Contains(out, "An application receipt")) {
			t.Fatalf("plugin absent from %v: %s", args, out)
		}
	}
	pluginData, _ := os.ReadFile(filepath.Join(plugins, "receipt.so"))
	os.WriteFile(filepath.Join(home, ".skgo", "plugins", "receipt.so"), pluginData, 0755)
	if out := call(skgoBin, []string{"HOME=" + home, "SKGO_PLUGIN_DIRS="}, "new", "--help"); !strings.Contains(out, "receipt-app") {
		t.Fatal(out)
	}
	project := filepath.Join(root, "consumer")
	os.MkdirAll(filepath.Join(project, "web"), 0755)
	os.WriteFile(filepath.Join(project, "go.mod"), []byte("module example.test/consumer\n\ngo 1.27.1\n"), 0644)
	call(skgoBin, env, "add", "--root", project, "--name", "MyApp", "--set", "text=literal receipt", "receipt")
	if b, _ := os.ReadFile(filepath.Join(project, "receipt.txt")); string(b) != "MyApp\nexample.test/consumer\nliteral receipt\n" {
		t.Fatalf("receipt=%q", b)
	}
	call(skgoBin, env, "add", "--root", project, "--set", "text=literal receipt", "receipt")
	cmd := exec.Command(skgoBin, "add", "--root", project, "--set", "text=overwrite", "receipt")
	cmd.Env = append(os.Environ(), env...)
	if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "conflicts with existing content") {
		t.Fatalf("conflict %v %s", err, out)
	}
	if b, _ := os.ReadFile(filepath.Join(project, "receipt.txt")); string(b) != "MyApp\nexample.test/consumer\nliteral receipt\n" {
		t.Fatalf("lost edit=%q", b)
	}
	// A project tool has its own module graph and recorded version.
	toolMod := "module example.test/toolhost\n\ngo 1.27.1\n\nrequire github.com/tylergannon/skgo v0.0.0\nreplace github.com/tylergannon/skgo => " + repo + "\ntool github.com/tylergannon/skgo/cmd/skgo\n"
	os.WriteFile(filepath.Join(project, "go.mod"), []byte(toolMod), 0644)
	cmd = exec.Command("go", "mod", "tidy")
	cmd.Dir = project
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("tool module: %v %s", err, out)
	}
	cmd = exec.Command("go", "tool", "-n", "skgo")
	cmd.Dir = project
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("resolve project tool: %v %s", err, out)
	}
	cmd = exec.Command("go", "tool", "skgo", "buildinfo", "--json")
	cmd.Dir = project
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	var info buildinfo.Info
	if err := json.Unmarshal(out, &info); err != nil {
		t.Fatal(err)
	}
	if info.SkgoVersion != "v0.0.0" || info.Build.Main.Path != "github.com/tylergannon/skgo" || info.Executable == skgoBin {
		t.Fatalf("actual tool identity=%s", out)
	}
	toolPlugins := filepath.Join(root, "tool-plugins")
	os.Mkdir(toolPlugins, 0755)
	consumerMod, _ := os.ReadFile(filepath.Join(project, "go.mod"))
	consumerSum, _ := os.ReadFile(filepath.Join(project, "go.sum"))
	buildLog.Reset()
	if err := pluginbuild.Build(context.Background(), pluginbuild.Options{Project: project, Source: source, Output: filepath.Join(toolPlugins, "receipt.so"), VersionSymbol: "main.skgoVersion", Log: &buildLog}); err != nil {
		t.Fatalf("project-tool source-build recipe: %v\n%s", err, buildLog.String())
	}
	if out := call(info.Executable, []string{"SKGO_PLUGIN_DIRS=" + toolPlugins}, "add", "receipt", "--help"); !strings.Contains(out, "Receipt text") {
		t.Fatal(out)
	}
	badSource := filepath.Join(root, "missing-module")
	os.Mkdir(badSource, 0755)
	os.WriteFile(filepath.Join(badSource, "go.mod"), []byte("go 1.27.1\n"), 0644)
	if err := pluginbuild.Build(context.Background(), pluginbuild.Options{Host: skgoBin, Source: badSource, Output: filepath.Join(root, "bad.so")}); err == nil || !strings.Contains(err.Error(), "must declare a module path") {
		t.Fatalf("missing module diagnostic: %v", err)
	}
	if after, _ := os.ReadFile(filepath.Join(project, "go.mod")); !bytes.Equal(after, consumerMod) {
		t.Fatal("consumer go.mod changed")
	}
	if after, _ := os.ReadFile(filepath.Join(project, "go.sum")); !bytes.Equal(after, consumerSum) {
		t.Fatal("consumer go.sum changed")
	}
	// An unrelated broken plugin warns; it does not erase the working one.
	os.WriteFile(filepath.Join(plugins, "broken.so"), []byte("not a plugin"), 0644)
	if out := call(skgoBin, env, "add", "--help"); !strings.Contains(out, "warning:") || !strings.Contains(out, "receipt") {
		t.Fatal(out)
	}
	missing := filepath.Join(root, "not-created")
	cmd = exec.Command(skgoBin, "new", "--template", "unknown", missing)
	cmd.Env = append(os.Environ(), env...)
	if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "unavailable") {
		t.Fatalf("unknown=%v %s", err, out)
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatalf("mutated destination: %v", err)
	}
}

func TestNewWithoutTemplateDoesNotDiscoverPlugins(t *testing.T) {
	t.Setenv("SKGO_PLUGIN_DIRS", filepath.Join(t.TempDir(), "missing"))
	var output bytes.Buffer
	called := false
	err := newCommand(context.Background(), []string{"--name", "OwnName", "destination"}, &output, &output, func(o newapp.Options) (newapp.Result, error) {
		called = true
		if o.App != "OwnName" {
			t.Fatalf("options=%+v", o)
		}
		return newapp.Result{Dir: o.Dir, App: o.App, Starter: "minimal"}, nil
	}, plugins)
	if err != nil || !called {
		t.Fatalf("called=%v err=%v", called, err)
	}
}

type creationPlugin struct{}

func (creationPlugin) Describe() templateapi.Descriptor {
	return templateapi.Descriptor{ID: "creation", Version: "1", Addons: []templateapi.Addon{{Name: "receipt", Options: []templateapi.Option{{Name: "message", Required: true}}}}, Templates: []templateapi.Template{{Name: "receipt-app", Options: []templateapi.Option{{Name: "message", Required: true}}, Steps: []templateapi.Step{{Addon: "receipt", Bindings: map[string]string{"message": "message"}}}}}}
}
func (creationPlugin) Apply(ctx context.Context, req templateapi.Request) (templateapi.Result, error) {
	b := fmt.Sprintf("%s\n%s\n%s\n", req.Project.Name, req.Project.Module, req.Options["message"])
	err := os.WriteFile(filepath.Join(req.Project.Root, "receipt.txt"), []byte(b), 0644)
	return templateapi.Result{Changed: []string{"receipt.txt"}}, err
}
func TestNewTemplateAppliesAfterBaseCreation(t *testing.T) {
	root := filepath.Join(t.TempDir(), "application")
	var output bytes.Buffer
	create := func(o newapp.Options) (newapp.Result, error) {
		if o.Dir != root || o.App != "FieldNotes" || o.Module != "example.test/field" || o.Interactive {
			t.Fatalf("base options: %+v", o)
		}
		if err := os.MkdirAll(filepath.Join(root, "web"), 0755); err != nil {
			return newapp.Result{}, err
		}
		if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/field\ngo 1.27.1\n"), 0644); err != nil {
			return newapp.Result{}, err
		}
		if err := templates.SaveIdentity(templateapi.Project{Root: root, Name: o.App}); err != nil {
			return newapp.Result{}, err
		}
		return newapp.Result{Dir: root, App: o.App}, nil
	}
	load := func(io.Writer) (*templates.Registry, error) {
		p := creationPlugin{}
		return &templates.Registry{Entries: []templates.Entry{{Path: "fixture", Plugin: p, Descriptor: p.Describe()}}}, nil
	}
	if err := newCommand(context.Background(), []string{"--name", "FieldNotes", "--module", "example.test/field", "--template", "receipt-app", "--set", "message=chosen message", root}, &output, &output, create, load); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(root, "receipt.txt")); err != nil || string(b) != "FieldNotes\nexample.test/field\nchosen message\n" {
		t.Fatalf("template result %q: %v", b, err)
	}
}

func TestNativePluginsShareDependenciesOrExplainIsolation(t *testing.T) {
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	shared := filepath.Join(root, "shared")
	other := filepath.Join(root, "other-shared")
	for _, d := range []string{shared, other} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(d, "go.mod"), []byte("module example.test/plugin-shared\ngo 1.27.1\n"), 0644)
	}
	os.WriteFile(filepath.Join(shared, "shared.go"), []byte("package shared\nfunc Purpose() string { return \"shared dependency one\" }\n"), 0644)
	os.WriteFile(filepath.Join(other, "shared.go"), []byte("package shared\nfunc Purpose() string { return \"different dependency two\" }\n"), 0644)
	dirs := map[string]string{}
	for _, name := range []string{"alpha", "beta", "conflict"} {
		source := filepath.Join(root, name)
		dir := filepath.Join(root, name+"-plugins")
		dirs[name] = dir
		os.MkdirAll(source, 0755)
		os.MkdirAll(dir, 0755)
		dep := shared
		if name == "conflict" {
			dep = other
		}
		mod := fmt.Sprintf("module example.test/%s\ngo 1.27.1\nrequire (\n github.com/tylergannon/skgo v0.0.0\n example.test/plugin-shared v0.0.0\n)\nreplace github.com/tylergannon/skgo => %s\nreplace example.test/plugin-shared => %s\n", name, repo, dep)
		os.WriteFile(filepath.Join(source, "go.mod"), []byte(mod), 0644)
		code := fmt.Sprintf(`package main
import("context"; "github.com/tylergannon/skgo/templateapi"; shared "example.test/plugin-shared")
var skgoVersion string
type plugin struct{}
func SKGoPluginV1() templateapi.Plugin{return plugin{}}
func(plugin) Describe() templateapi.Descriptor{return templateapi.Descriptor{ID:%q,Version:"1",SkgoVersion:skgoVersion,Addons:[]templateapi.Addon{{Name:%q,Summary:shared.Purpose()}}}}
func(plugin) Apply(context.Context,templateapi.Request)(templateapi.Result,error){return templateapi.Result{},nil}
func main(){}
`, name, name)
		os.WriteFile(filepath.Join(source, "main.go"), []byte(code), 0644)
		var log bytes.Buffer
		if err := pluginbuild.Build(context.Background(), pluginbuild.Options{Host: skgoBin, Source: source, Output: filepath.Join(dir, name+".so"), SkgoSource: repo, VersionSymbol: "main.skgoVersion", Log: &log}); err != nil {
			t.Fatalf("%s: %v\n%s", name, err, &log)
		}
	}
	help := func(paths ...string) string {
		cmd := exec.Command(skgoBin, "add", "--help")
		cmd.Env = append(os.Environ(), "SKGO_PLUGIN_DIRS="+strings.Join(paths, ","))
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("help: %v\n%s", err, out)
		}
		return string(out)
	}
	if out := help(dirs["alpha"], dirs["beta"]); strings.Contains(out, "warning:") || !strings.Contains(out, "alpha") || !strings.Contains(out, "beta") {
		t.Fatalf("compatible pair: %s", out)
	}
	out := help(dirs["alpha"], dirs["beta"], dirs["conflict"])
	for _, want := range []string{"warning:", "conflict.so", "example.test/plugin-shared", "already loaded:", filepath.Join(dirs["alpha"], "alpha.so"), filepath.Join(dirs["beta"], "beta.so"), "isolate SKGO_PLUGIN_DIRS"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in conflict help: %s", want, out)
		}
	}
	if strings.Contains(out, "different dependency two") {
		t.Fatalf("claimed conflicting plugin loaded: %s", out)
	}
	if out := help(dirs["conflict"]); strings.Contains(out, "warning:") || !strings.Contains(out, "different dependency two") {
		t.Fatalf("isolated plugin: %s", out)
	}
}
