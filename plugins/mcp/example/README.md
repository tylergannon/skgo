# Task list with MCP

This small application keeps a shared task list in Go. SvelteKit displays it
and adds tasks through remote functions. The MCP plugin generates a second
client for those same operations. Request/result types and field descriptions
live in `contracts/tasks.go`; function descriptions live beside the remotes in
`web/src/routes/tasks.remote.go`. The `mcppublish.Tool` markers publish `listTasks` and `addTask`. The web-only
`taskCount` remote displays the count but is deliberately absent from MCP.

`cmd/mcp/main.go` is maintained application setup, created from the plugin's
`mcp-server` add-on. Edit it for credentials, headers, server identity and
connection configuration. `cmd/mcp/instructions.md` is embedded application-wide
guidance delivered through MCP initialization. Neither file is regenerated;
function/type/field documentation still comes from the remote declarations.

Follow the build/run steps in [the plugin README](../README.md#example-application).
State lasts for the application's process lifetime. The example listens on
loopback and intentionally has no authentication or external services.

The optional MCP target builds with `go build -tags=mcp ./cmd/mcp`. Ordinary
web builds exclude that target.
