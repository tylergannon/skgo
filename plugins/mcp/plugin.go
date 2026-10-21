// MCP is a development-time plugin generating Go clients and MCP tools.
package main

import (
	"context"
	"github.com/tylergannon/skgo/plugins/mcp/generator"
	"github.com/tylergannon/skgo/templateapi"
)

var Version = "dev"
var SkgoVersion = "dev"

type mcpPlugin struct{}

func SKGoPluginV1() templateapi.Plugin { return mcpPlugin{} }
func (mcpPlugin) Describe() templateapi.Descriptor {
	return templateapi.Descriptor{ID: "mcp", Version: Version, SkgoVersion: SkgoVersion, Generator: true}
}
func (mcpPlugin) Generate(ctx context.Context, r templateapi.GenerationRequest) (templateapi.GenerationResult, error) {
	return (generator.Generator{}).Generate(ctx, r)
}
func main() {}
