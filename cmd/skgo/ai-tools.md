---
title: The ai-tools add-on
description: Set up a coding agent at the application root with one command.
division: ai
section: Introduction
slug: ai-tools
order: 1020
---

From the application root, the directory that holds `go.mod`:

```sh
go tool skgo add ai-tools
```

The add-on writes three files there:

| File | Contents |
| --- | --- |
| `AGENTS.md` | SKGo's [instructions](/docs/ai/agents-md): where server code goes, which commands to run, and how to use the Svelte MCP server when the server is Go |
| `CLAUDE.md` | `@AGENTS.md`, so Claude Code reads the same instructions |
| `.mcp.json` | two servers: `skgo`, started as `go tool skgo mcp` ([SKGo's MCP server](/docs/ai/mcp)), and `svelte`, started as `web/node_modules/.bin/vp -C web dlx @sveltejs/mcp` ([the Svelte MCP server](/docs/ai/svelte-mcp)) |

Start the agent in the application root. Claude Code reads all three files and, the first time it starts there, asks you to approve the two servers. Other agents that follow the [AGENTS.md](https://agents.md) convention read `AGENTS.md`; register the two servers with them as [MCP server](/docs/ai/mcp) and [The Svelte MCP server](/docs/ai/svelte-mcp) show.

The Svelte server runs the frontend's own VitePlus, so in a fresh clone it starts once `just install` has run.

The add-on never overwrites: if any of the three files exists, it names it and writes nothing. To combine SKGo's setup with your own, copy what you need from the files above.

## sv's ai-tools

`skgo new` refuses sv's own [ai-tools](https://svelte.dev/docs/cli/ai-tools) add-on. sv writes its files into the Kit project, `web/`, where an agent started at the application root does not read them, and its launchers and Svelte plugins run `npx`. Create the application without it, then run `go tool skgo add ai-tools`.
