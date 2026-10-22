// Generate the checkout example through SKGO's public embedding API. Released
// consumers use the plugin's ordinary skgo generate --plugins entrypoint.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"path/filepath"
	"plugin"

	"github.com/tylergannon/skgo/generate"
	"github.com/tylergannon/skgo/templateapi"
)

func main() {
	scaffoldRoot := flag.String("root", ".", "destination for -init")
	initServer := flag.Bool("init", false, "create missing application-owned server setup without generating bindings")
	path := flag.String("plugin", "", "path to the CLI generator shared object")
	flag.Parse()
	provider, err := load(*path)
	if err != nil {
		log.Fatal(err)
	}
	const module = "github.com/tylergannon/skgo/plugins/cli/example"
	if *initServer {
		addon, ok := provider.(templateapi.AddonPlugin)
		if !ok {
			log.Fatal("plugin does not implement AddonPlugin")
		}
		root, err := filepath.Abs(*scaffoldRoot)
		if err != nil {
			log.Fatal(err)
		}
		if _, err := addon.Apply(context.Background(), templateapi.Request{Addon: "mcp-server", Project: templateapi.Project{Root: root, Module: module, Name: "TaskList"}}); err != nil {
			log.Fatal(err)
		}
		return
	}
	err = generate.Run(generate.Config{
		Web: "web", Out: "internal/skgo", ProjectName: "TaskList",
		ConfigPackage: module + "/internal/app", LocalsPackage: module + "/internal/app", HookPackage: module + "/internal/serverhooks",
		Context:    context.Background(),
		Generators: []templateapi.GeneratorInvocation{{ID: "cli", Version: "dev", Generator: provider}},
	})
	if err != nil {
		log.Fatal(err)
	}
}

func load(path string) (templateapi.Generator, error) {
	if path == "" {
		return nil, fmt.Errorf("-plugin is required; build the plugin with the example's module graph")
	}
	p, err := plugin.Open(path)
	if err != nil {
		return nil, err
	}
	symbol, err := p.Lookup("SKGoPluginV1")
	if err != nil {
		return nil, err
	}
	factory, ok := symbol.(func() templateapi.Plugin)
	if !ok {
		return nil, fmt.Errorf("invalid SKGoPluginV1 factory")
	}
	g, ok := factory().(templateapi.Generator)
	if !ok {
		return nil, fmt.Errorf("plugin does not implement Generator")
	}
	return g, nil
}
