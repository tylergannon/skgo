# Task list with MCP

This small application keeps a shared task list in Go. SvelteKit displays it
and adds tasks through remote functions. The MCP plugin generates a second
client for those same operations. Request/result types and field descriptions
live in `contracts/tasks.go`; function descriptions live beside the remotes in
`web/src/routes/tasks.remote.go`. There is no separate tool documentation file.

Follow the build/run steps in [the plugin README](../README.md#example-application).
State lasts for the application's process lifetime. The example listens on
loopback and intentionally has no authentication or external services.
