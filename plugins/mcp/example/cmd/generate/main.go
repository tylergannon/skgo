// Generate the checkout example through SKGO's public embedding API. Released
// consumers use the plugin's ordinary skgo generate --plugins entrypoint.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"plugin"

	"github.com/tylergannon/skgo/generate"
	"github.com/tylergannon/skgo/templateapi"
)

func main() {
	path := flag.String("plugin", "", "path to the MCP generator shared object")
	flag.Parse()
	provider, err := load(*path)
	if err != nil {
		log.Fatal(err)
	}
	const module = "github.com/tylergannon/skgo/plugins/mcp/example"
	err = generate.Run(generate.Config{
		Web: "web", Out: "internal/skgo", ProjectName: "TaskList",
		ConfigPackage: module + "/internal/app", LocalsPackage: module + "/internal/app", HookPackage: module + "/internal/serverhooks",
		Context:    context.Background(),
		Generators: []templateapi.GeneratorInvocation{{ID: "mcp", Version: "dev", Generator: provider, Selections: []string{"src/routes/tasks.remote.ts#listTasks", "src/routes/tasks.remote.ts#addTask"}}},
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
