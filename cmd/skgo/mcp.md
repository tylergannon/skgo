---
title: skgo mcp
description: Serve check and advice over MCP.
division: cli
section: Commands
order: 2075
---

A Model Context Protocol server on standard input and output that exposes `skgo check` and `skgo advice` as tools, for editors and agents.

## Usage

```text skgo mcp -h
```

`skgo mcp` takes no flags or arguments: `--help` prints the help above, and any other argument prints it and exits with status 2. [MCP server](/docs/ai/mcp) shows how to register it with a client, and [Tools](/docs/ai/tools) describes what it serves.
