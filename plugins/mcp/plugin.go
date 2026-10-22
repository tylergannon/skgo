// MCP is a development-time plugin generating Go clients and MCP tools.
package main

import (
	"context"
	"github.com/tylergannon/skgo/plugins/mcp/generator"
	"github.com/tylergannon/skgo/plugins/mcp/scaffold"
	"github.com/tylergannon/skgo/templateapi"
)

var Version = "dev"
var SkgoVersion = "dev"

type mcpPlugin struct{}

func (mcpPlugin) RequiresRemotePath() bool { return true }

func SKGoPluginV1() templateapi.Plugin { return mcpPlugin{} }
func (mcpPlugin) Describe() templateapi.Descriptor {
	return templateapi.Descriptor{ID: "mcp", Version: Version, SkgoVersion: SkgoVersion, Generator: true,
		Addons:    []templateapi.Addon{{Name: "mcp-server", Summary: "Create application-owned MCP server setup and instructions"}},
		Templates: []templateapi.Template{{Name: "mcp", Summary: "Application with an MCP server skeleton", Steps: []templateapi.Step{{Addon: "mcp-server"}}}},
	}
}
func (mcpPlugin) Generate(ctx context.Context, r templateapi.GenerationRequest) (templateapi.GenerationResult, error) {
	return (generator.Generator{}).Generate(ctx, r)
}
func main() {}

func (mcpPlugin) RemoteMarker() string { return (generator.Generator{}).RemoteMarker() }
func (mcpPlugin) Apply(ctx context.Context, r templateapi.Request) (templateapi.Result, error) {
	return scaffold.Apply(ctx, r)
}
