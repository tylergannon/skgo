package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/tylergannon/skgo/internal/newapp"
	"github.com/tylergannon/skgo/internal/pluginbuild"
	"github.com/tylergannon/skgo/internal/templates"
	"github.com/tylergannon/skgo/templateapi"
)

// Native loading is a real process boundary: one plugin built against the
// actual CLI must receive the developer's project and options when applied.
func TestNativePluginBuildAndCLIApplication(t *testing.T) {
	t.Parallel()
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	source, plugins, project := filepath.Join(root, "source"), filepath.Join(root, "plugins"), filepath.Join(root, "consumer")
	for _, dir := range []string{source, plugins, filepath.Join(project, "web")} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	fixture, err := os.ReadFile("testdata/template-plugin/plugin.go")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "main.go"), fixture, 0644); err != nil {
		t.Fatal(err)
	}
	mod := "module example.test/plugin\n\ngo 1.27.1\nrequire github.com/tylergannon/skgo v0.0.0\nreplace github.com/tylergannon/skgo => " + repo + "\n"
	if err := os.WriteFile(filepath.Join(source, "go.mod"), []byte(mod), 0644); err != nil {
		t.Fatal(err)
	}
	var log bytes.Buffer
	if err := pluginbuild.Build(t.Context(), pluginbuild.Options{Host: skgoBinary(t), Source: source, Output: filepath.Join(plugins, "receipt.so"), SkgoSource: repo, VersionSymbol: "main.skgoVersion", Log: &log}); err != nil {
		t.Fatalf("build native plugin: %v\n%s", err, &log)
	}
	if err := os.WriteFile(filepath.Join(project, "go.mod"), []byte("module example.test/consumer\ngo 1.27.1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), skgoBinary(t), "add", "--root", project, "--name", "MyApp", "--set", "text=literal receipt", "receipt")
	cmd.Env = append(os.Environ(), "SKGO_PLUGIN_DIRS="+plugins, "HOME="+root)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("native plugin apply: %v\n%s", err, output)
	}
	if got, err := os.ReadFile(filepath.Join(project, "receipt.txt")); err != nil || string(got) != "MyApp\nexample.test/consumer\nliteral receipt\n" {
		t.Fatalf("plugin receipt = %q, %v", got, err)
	}
}

func TestNewWithoutTemplateDoesNotDiscoverPlugins(t *testing.T) {
	var output bytes.Buffer
	called := false
	err := newCommand(t.Context(), []string{"--name", "OwnName", "destination"}, &output, &output, func(o newapp.Options) (newapp.Result, error) {
		called = true
		if o.App != "OwnName" {
			t.Fatalf("options=%+v", o)
		}
		return newapp.Result{Dir: o.Dir, App: o.App, Starter: "minimal"}, nil
	}, func(io.Writer) (*templates.Registry, error) {
		t.Fatal("new without a template discovered plugins")
		return nil, nil
	})
	if err != nil || !called {
		t.Fatalf("called=%v err=%v", called, err)
	}
}

type creationPlugin struct{}

func (creationPlugin) Describe() templateapi.Descriptor {
	return templateapi.Descriptor{ID: "creation", Version: "1", Addons: []templateapi.Addon{{Name: "receipt", Options: []templateapi.Option{{Name: "message", Required: true}}}}, Templates: []templateapi.Template{{Name: "receipt-app", Options: []templateapi.Option{{Name: "message", Required: true}}, Steps: []templateapi.Step{{Addon: "receipt", Bindings: map[string]string{"message": "message"}}}}}}
}
func (creationPlugin) Apply(_ context.Context, req templateapi.Request) (templateapi.Result, error) {
	err := os.WriteFile(filepath.Join(req.Project.Root, "receipt.txt"), []byte(fmt.Sprintf("%s\n%s\n%s\n", req.Project.Name, req.Project.Module, req.Options["message"])), 0644)
	return templateapi.Result{Changed: []string{"receipt.txt"}}, err
}
func TestNewTemplateAppliesAfterBaseCreation(t *testing.T) {
	t.Parallel()
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
	if err := newCommand(t.Context(), []string{"--name", "FieldNotes", "--module", "example.test/field", "--template", "receipt-app", "--set", "message=chosen message", root}, &output, &output, create, load); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(root, "receipt.txt")); err != nil || string(got) != "FieldNotes\nexample.test/field\nchosen message\n" {
		t.Fatalf("template result %q: %v", got, err)
	}
}
