package gen

import (
	"context"
	"errors"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tg "github.com/tylergannon/polytype/typegrammar"
	"github.com/tylergannon/skgo/templateapi"
)

type fixtureGenerator func(context.Context, templateapi.GenerationRequest) (templateapi.GenerationResult, error)

func (f fixtureGenerator) Generate(ctx context.Context, r templateapi.GenerationRequest) (templateapi.GenerationResult, error) {
	return f(ctx, r)
}

func TestGeneratorResolvedRemoteModel(t *testing.T) {
	a, gp := emitterPackage(t, t.TempDir(), "package data\ntype Value struct { Text string `json:\"text\"` }\n")
	value := gp.pkg.Types.Scope().Lookup("Value").Type()
	a.remotes = []*remoteFn{
		{name: "read", description: "Read the shared value.", module: "src/lib/todos.remote.ts", kind: kindQuery, goPkg: gp, out: value, pos: token.Position{Filename: "todos.remote.go", Line: 7}},
		{name: "save", module: "src/lib/todos.remote.ts", kind: kindCommand, goPkg: gp, in: value, out: types.Typ[types.String]},
	}
	m, err := a.generationModel([]string{"src/lib/todos.remote.ts#read", "src/lib/todos.remote.ts#save"})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Remotes) != 2 || len(m.Roots) != 2 || len(m.Definitions) != 1 {
		t.Fatalf("model=%+v", m)
	}
	read, save := m.Remotes[0], m.Remotes[1]
	if read.Description != "Read the shared value." {
		t.Fatalf("description=%q", read.Description)
	}
	if read.ID != "worolc/read" || read.Kind != "query" || read.Input != -1 || read.Output != 0 || read.Source.Line != 7 || save.Input != 0 || save.Output != 1 || save.Kind != "command" {
		t.Fatalf("remotes=%+v", m.Remotes)
	}
	if obj, ok := m.Definitions[0].Type.(*tg.Object); !ok || len(obj.Fields) != 1 || obj.Fields[0].JSONName != "text" {
		t.Fatalf("wire definition=%+v", m.Definitions)
	}
	for _, selections := range [][]string{{"missing"}, {"src/lib/todos.remote.ts#read", "src/lib/todos.remote.ts#read"}} {
		if _, err := a.generationModel(selections); err == nil {
			t.Fatalf("accepted %v", selections)
		}
	}
	a.transported = []*transportedType{{key: "Value", named: value.(*types.Named)}}
	m, err = a.generationModel([]string{"src/lib/todos.remote.ts#read"})
	if err != nil || !m.Remotes[0].Transported || m.Remotes[0].Output != -1 {
		t.Fatalf("transport model=%+v err=%v", m, err)
	}
}

func TestGeneratorManagedPublication(t *testing.T) {
	root := t.TempDir()
	a, _ := emitterPackage(t, root, "package data\n")
	if err := os.MkdirAll(a.cfg.Web, 0755); err != nil {
		t.Fatal(err)
	}
	var err error
	a.cfg.generation.links, err = newRouteLinks(a.cfg, root, "example.com/app")
	if err != nil {
		t.Fatal(err)
	}
	a.cfg.ProjectName = "DeclaredName"
	output := "first\n"
	plugin := fixtureGenerator(func(ctx context.Context, r templateapi.GenerationRequest) (templateapi.GenerationResult, error) {
		if r.Project.Root != root || r.Project.Module != "example.com/app" || r.Project.Name != "DeclaredName" || r.Options["name"] != "fixture" {
			t.Fatalf("request=%+v", r)
		}
		return templateapi.GenerationResult{Files: []templateapi.GeneratedFile{{Path: "generated/client.txt", Content: []byte(output)}}}, nil
	})
	a.cfg.Generators = []templateapi.GeneratorInvocation{{ID: "fixture", Version: "v1", Generator: plugin, Options: map[string]string{"name": "fixture"}}}
	publish := func() {
		t.Helper()
		if err := a.runGenerators(); err != nil {
			t.Fatal(err)
		}
		if err := a.cfg.generation.publish(a.cfg); err != nil {
			t.Fatal(err)
		}
	}
	reset := func() {
		a.cfg.generation.files = map[string]string{}
		a.cfg.generation.preflight = nil
		a.cfg.generation.removed = map[string]bool{}
	}
	publish()
	target := filepath.Join(root, "generated/client.txt")
	first, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	reset()
	publish()
	second, _ := os.Stat(target)
	if !first.ModTime().Equal(second.ModTime()) {
		t.Fatal("identical generation rewrote output")
	}
	output = "second\n"
	reset()
	publish()
	data, _ := os.ReadFile(target)
	if string(data) != output {
		t.Fatalf("output=%q", data)
	}
	if err := os.WriteFile(target, []byte("authored edit\n"), 0644); err != nil {
		t.Fatal(err)
	}
	reset()
	if err := a.runGenerators(); err == nil || !strings.Contains(err.Error(), "authored or edited") {
		t.Fatalf("edited output accepted: %v", err)
	}
	if err := os.WriteFile(target, []byte(output), 0644); err != nil {
		t.Fatal(err)
	}
	a.cfg.Generators[0].Generator = fixtureGenerator(func(context.Context, templateapi.GenerationRequest) (templateapi.GenerationResult, error) {
		return templateapi.GenerationResult{}, nil
	})
	reset()
	publish()
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("stale file remains: %v", err)
	}
}

// Equal bytes establish no ownership: an authored file must never be adopted
// and later removed merely because a plugin once proposed the same content.
func TestGeneratorPreservesUnownedIdenticalOutput(t *testing.T) {
	root := t.TempDir()
	a, _ := emitterPackage(t, root, "package data\n")
	if err := os.MkdirAll(a.cfg.Web, 0755); err != nil {
		t.Fatal(err)
	}
	var err error
	a.cfg.generation.links, err = newRouteLinks(a.cfg, root, "example.com/app")
	if err != nil {
		t.Fatal(err)
	}
	const authored = "independently authored\n"
	target := filepath.Join(root, "client.txt")
	if err := os.WriteFile(target, []byte(authored), 0644); err != nil {
		t.Fatal(err)
	}
	a.cfg.Generators = []templateapi.GeneratorInvocation{{ID: "fixture", Generator: fixtureGenerator(func(context.Context, templateapi.GenerationRequest) (templateapi.GenerationResult, error) {
		return templateapi.GenerationResult{Files: []templateapi.GeneratedFile{{Path: "client.txt", Content: []byte(authored)}}}, nil
	})}}
	if err := a.runGenerators(); err == nil || !strings.Contains(err.Error(), "authored or edited") {
		t.Fatalf("identical authored output accepted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(a.cfg.Out, "skgo.plugin-outputs.json")); !os.IsNotExist(err) {
		t.Fatalf("failed adoption published ownership: %v", err)
	}
	a.cfg.Generators[0].Generator = fixtureGenerator(func(context.Context, templateapi.GenerationRequest) (templateapi.GenerationResult, error) {
		return templateapi.GenerationResult{}, nil
	})
	if err := a.runGenerators(); err != nil {
		t.Fatal(err)
	}
	if err := a.cfg.generation.publish(a.cfg); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != authored {
		t.Fatalf("authored file removed or changed: %q %v", data, err)
	}
}

func TestGeneratorRejectsUnsafeOrFailedOutput(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files []templateapi.GeneratedFile
		diag  []templateapi.Diagnostic
		err   error
	}{
		{name: "traversal", files: []templateapi.GeneratedFile{{Path: "../escape"}}},
		{name: "absolute", files: []templateapi.GeneratedFile{{Path: "/escape"}}},
		{name: "core", files: []templateapi.GeneratedFile{{Path: "generated/skgo_gen.go"}}},
		{name: "core inventory", files: []templateapi.GeneratedFile{{Path: "generated/links.json"}}},
		{name: "core link tree", files: []templateapi.GeneratedFile{{Path: "generated/_links/client.txt"}}},
		{name: "route boundary", files: []templateapi.GeneratedFile{{Path: "web/src/routes/go.mod"}}},
		{name: "duplicate", files: []templateapi.GeneratedFile{{Path: "out.txt"}, {Path: "out.txt"}}},
		{name: "plugin failure", files: []templateapi.GeneratedFile{{Path: "out.txt"}}, err: errors.New("failed")},
		{name: "error diagnostic", diag: []templateapi.Diagnostic{{Severity: "error", Message: "invalid selection"}}},
		{name: "unknown severity", diag: []templateapi.Diagnostic{{Severity: "success"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			a, _ := emitterPackage(t, root, "package data\n")
			a.cfg.Generators = []templateapi.GeneratorInvocation{{ID: "fixture", Generator: fixtureGenerator(func(context.Context, templateapi.GenerationRequest) (templateapi.GenerationResult, error) {
				return templateapi.GenerationResult{Files: tc.files, Diagnostics: tc.diag}, tc.err
			})}}
			if err := a.runGenerators(); err == nil {
				t.Fatal("accepted invalid result")
			}
			if _, err := os.Stat(filepath.Join(root, "out.txt")); !os.IsNotExist(err) {
				t.Fatal("failed generation published output")
			}
		})
	}
	t.Run("symlink", func(t *testing.T) {
		root := t.TempDir()
		outside := t.TempDir()
		if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
			t.Fatal(err)
		}
		if _, err := pluginOutputPath(root, "link/output"); err == nil {
			t.Fatal("accepted symlink path")
		}
	})
	t.Run("cancelled", func(t *testing.T) {
		a, _ := emitterPackage(t, t.TempDir(), "package data\n")
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		a.cfg.Context = ctx
		a.cfg.Generators = []templateapi.GeneratorInvocation{{ID: "fixture", Generator: fixtureGenerator(func(context.Context, templateapi.GenerationRequest) (templateapi.GenerationResult, error) {
			t.Fatal("called cancelled generator")
			return templateapi.GenerationResult{}, nil
		})}}
		if err := a.runGenerators(); !errors.Is(err, context.Canceled) {
			t.Fatalf("err=%v", err)
		}
	})
}

// An empty application still invokes explicitly configured generators. A warm
// core plan must never hide changed plugin configuration or skip plugin failure.
func TestGeneratorRunBypassesCacheAndPublishesOnlySuccess(t *testing.T) {
	root := t.TempDir()
	writeSharedFixture(t, root, "go.mod", "module example.com/empty\n\ngo 1.27\n")
	if err := os.MkdirAll(filepath.Join(root, "web/src"), 0755); err != nil {
		t.Fatal(err)
	}
	calls := 0
	cfg := fixtureConfig(Config{Web: filepath.Join(root, "web"), Out: filepath.Join(root, "bindings"), CacheDir: filepath.Join(root, "cache")})
	cfg.Generators = []templateapi.GeneratorInvocation{{ID: "fixture", Options: map[string]string{"value": "first"}, Generator: fixtureGenerator(func(_ context.Context, r templateapi.GenerationRequest) (templateapi.GenerationResult, error) {
		calls++
		return templateapi.GenerationResult{Files: []templateapi.GeneratedFile{{Path: "client.txt", Content: []byte(r.Options["value"])}}}, nil
	})}}
	if err := Run(cfg); err != nil {
		t.Fatal(err)
	}
	cfg.Generators[0].Options["value"] = "second"
	if err := Run(cfg); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("generator calls=%d, want 2", calls)
	}
	data, _ := os.ReadFile(filepath.Join(root, "client.txt"))
	if string(data) != "second" {
		t.Fatalf("output=%q", data)
	}
	cfg.Generators[0].Options["value"] = "unpublished"
	cfg.Generators = append(cfg.Generators, templateapi.GeneratorInvocation{ID: "failure", Generator: fixtureGenerator(func(context.Context, templateapi.GenerationRequest) (templateapi.GenerationResult, error) {
		return templateapi.GenerationResult{}, errors.New("provider failed")
	})})
	if err := Run(cfg); err == nil {
		t.Fatal("failed provider accepted")
	}
	data, _ = os.ReadFile(filepath.Join(root, "client.txt"))
	if string(data) != "second" {
		t.Fatalf("earlier provider published before failure: %q", data)
	}
}
