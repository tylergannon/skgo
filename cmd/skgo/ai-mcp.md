---
title: Overview
description: Register skgo mcp so an agent can check the project and read SKGo's diagnostic guidance.
division: ai
section: MCP server
slug: mcp
order: 3010
---

The SKGo MCP ([Model Context Protocol](https://modelcontextprotocol.io/docs/getting-started/intro)) server is built into the CLI as [`skgo mcp`](/docs/cli/mcp). It gives your agent two [tools](/docs/ai/tools): `skgo_check` runs [`skgo check`](/docs/cli/check) and returns the structured report, and `skgo_advice` returns the repair guidance for SKGo's `SKGO001`–`SKGO008` diagnostics. It serves no documentation; give the agent [llms.txt](/docs/ai/llms-txt) for that.

## Setup

Register the server as `go tool skgo mcp`. `go tool` runs the SKGo version your application's `go.mod` pins, so the checks and the guidance match the release your code is built against. It only works inside the module, and `skgo_check` checks the server's working directory unless told otherwise, so the client must start the server in the application root, the directory that contains `go.mod`.

The client starts the server with its own environment. `go`, `goimports` and `staticcheck` must be on the `PATH` it passes, which GUI editors often trim; otherwise every `skgo_check` comes back with `isError: true`.

The server speaks newline-delimited JSON-RPC on standard input and output, takes no arguments, and answers tool calls only after the client has sent `initialize` and `notifications/initialized`. Its diagnostics go to standard error.

### Claude Code

[`go tool skgo add ai-tools`](/docs/ai/ai-tools) registers the server in the application root's `.mcp.json`. To register it yourself, from the application root:

```sh
claude mcp add -t stdio -s project skgo -- go tool skgo mcp
```

Use `project` or `local` scope rather than `user`: a user-scoped server also starts in directories that are not SKGo applications, where `go tool skgo` fails.

### Cursor

Add the server to `.cursor/mcp.json` in the application root:

```json
{
  "mcpServers": {
    "skgo": {
      "command": "go",
      "args": ["tool", "skgo", "mcp"]
    }
  }
}
```

### VS Code

VS Code reads `.vscode/mcp.json`, with the servers under `servers`:

```json
{
  "servers": {
    "skgo": {
      "type": "stdio",
      "command": "go",
      "args": ["tool", "skgo", "mcp"]
    }
  }
}
```

### Other clients

Refer to your client's documentation for `stdio` servers. The command is `go`, the arguments are `tool skgo mcp`, and the working directory is the application root.

## Usage

Register [the Svelte MCP server](/docs/ai/svelte-mcp) beside it; the two have no tools in common. Then tell the agent when to call each SKGo tool, in the [AGENTS.md block](/docs/ai/agents-md) or your prompt:

- `skgo_check` before it reports work as finished
- `skgo_advice` with the code of any diagnostic whose `documentation` reads `skgo advice SKGO00N`, before it changes the code
