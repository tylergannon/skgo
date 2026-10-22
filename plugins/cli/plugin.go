// CLI is a development-time plugin generating Cobra commands, typed clients and MCP tools.
package main

import (
	"context"
	"github.com/tylergannon/skgo/plugins/cli/generator"
	"github.com/tylergannon/skgo/plugins/cli/scaffold"
	"github.com/tylergannon/skgo/templateapi"
)

var Version = "dev"
var SkgoVersion = "dev"

type cliPlugin struct{}

func (cliPlugin) RequiresRemotePath() bool { return true }

func SKGoPluginV1() templateapi.Plugin { return cliPlugin{} }
func (cliPlugin) Describe() templateapi.Descriptor {
	return templateapi.Descriptor{ID: "cli", Version: Version, SkgoVersion: SkgoVersion, Generator: true,
		Addons:    []templateapi.Addon{{Name: "mcp-server", Summary: "Create application-owned MCP server setup and instructions"}},
		Templates: []templateapi.Template{{Name: "mcp", Summary: "Application with an MCP server skeleton", Steps: []templateapi.Step{{Addon: "mcp-server"}}}},
	}
}
func (cliPlugin) Generate(ctx context.Context, r templateapi.GenerationRequest) (templateapi.GenerationResult, error) {
	return (generator.Generator{}).Generate(ctx, r)
}
func main() {}

func (cliPlugin) RemoteMarker() string { return (generator.Generator{}).RemoteMarker() }
func (cliPlugin) Apply(ctx context.Context, r templateapi.Request) (templateapi.Result, error) {
	return scaffold.Apply(ctx, r)
}
