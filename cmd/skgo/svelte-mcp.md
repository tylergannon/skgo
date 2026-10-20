---
title: The Svelte MCP server
description: Run the official Svelte MCP server in an SKGo project through VitePlus.
division: ai
section: MCP server
order: 3030
---

The [Svelte MCP server](https://svelte.dev/docs/ai/mcp) works in an SKGo application for everything Kit's frontend does: components, runes, routing and the client side of remote functions.

Svelte's [local setup](https://svelte.dev/docs/ai/local-setup) starts the server with `npx -y @sveltejs/mcp`. That does not run inside an SKGo project's `web/`, whose `package.json` declares pnpm as its package manager in `devEngines`. Start it through the project's VitePlus instead: `vp dlx @sveltejs/mcp`.

## With the ai-tools add-on

[`go tool skgo add ai-tools`](/docs/ai/ai-tools), run from the application root, registers the server for Claude Code in the root's `.mcp.json`:

```json
{
  "mcpServers": {
    "svelte": {
      "type": "stdio",
      "command": "web/node_modules/.bin/vp",
      "args": ["-C", "web", "dlx", "@sveltejs/mcp"]
    }
  }
}
```

`-C web` runs the frontend's VitePlus against `web/`, so the server is fetched with the package manager `web/package.json` declares. The command exists once `vp install` has run in `web/`.

For other clients, use `web/node_modules/.bin/vp` as the command and `-C web dlx @sveltejs/mcp` as the arguments, in the shapes Svelte's [local setup](https://svelte.dev/docs/ai/local-setup) shows, with the application root as the working directory.

`skgo new` refuses sv's own `ai-tools` add-on, which writes its configuration into `web/` and starts the server with `npx`.

## Which documentation applies

The Svelte MCP server's `get-documentation` tool serves svelte.dev's Svelte and SvelteKit documentation. For components and client code it is correct as written. Its server pages, such as `+page.server.ts`, `+server.ts`, `hooks.server.ts`, `$env` and adapters, describe Kit's JavaScript server; in SKGo those are Go, and this documentation covers them. [Differences](/docs/skgo/differences) lists every departure.
