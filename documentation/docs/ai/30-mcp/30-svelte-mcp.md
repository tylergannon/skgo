---
title: The Svelte MCP server
summary: Run the official Svelte MCP server in an SKGo project through VitePlus.
proof:
  - TestGeneratedLaunchersUseProjectLocalVitePlusAndPreserveAuthoredConfig
---

The [Svelte MCP server](https://svelte.dev/docs/ai/mcp) works in an SKGo application for everything Kit's frontend does: components, runes, routing and the client side of remote functions.

Svelte's [local setup](https://svelte.dev/docs/ai/local-setup) starts the server with `npx -y @sveltejs/mcp`. That does not run inside an SKGo project's `web/`, whose `package.json` declares pnpm as its package manager in `devEngines`. Start it through the project's VitePlus instead: `vp dlx @sveltejs/mcp`.

## With `skgo new`

Choose the `ai-tools` add-on when `skgo new` asks for sv's add-ons, or name it after `--`:

```sh
skgo new myapp -- --add ai-tools="ide:claude-code+delivery:tools+tools:mcp,svelte-code-writer+mcpSetup:local"
```

sv writes the add-on's files in `web/`, the Kit project. SKGo's add-on, `@skgo/sv`, runs afterwards and rewrites the parts that would call `npx`:

- In `.mcp.json`, `.cursor/mcp.json`, `.gemini/settings.json` and `.vscode/mcp.json`, a `svelte` server whose command is exactly `npx -y @sveltejs/mcp` becomes the following. Its other keys, such as `env`, are kept.

```json
/// file: web/.mcp.json
{
  "mcpServers": {
    "svelte": {
      "type": "stdio",
      "command": "./node_modules/.bin/vp",
      "env": {},
      "args": ["dlx", "@sveltejs/mcp"]
    }
  }
}
```

- In the `svelte-code-writer` skill and the `svelte-file-editor` agent under `.claude/`, `.cursor/`, `.gemini/` and `.github/`, `npx @sveltejs/mcp` becomes `./node_modules/.bin/vp dlx @sveltejs/mcp`.

The rewrite touches only what sv generated. A remote (`url`) entry, a server with another command, other servers, and skill or agent files whose contents differ from sv's output are left as they are. The plugin settings that `delivery:plugin` writes are not changed.

## From the application root

The files above are read by a client opened in `web/`. For a client opened at the application root, register the Svelte server there with the project's `vp`:

```sh
claude mcp add -t stdio -s project svelte -- web/node_modules/.bin/vp dlx @sveltejs/mcp
```

For other clients, use `web/node_modules/.bin/vp` as the command and `dlx @sveltejs/mcp` as the arguments, in the shapes Svelte's [local setup](https://svelte.dev/docs/ai/local-setup) shows. This also works in a project created without the add-on, once `vp install` has run.

## Which documentation applies

The Svelte MCP server's `get-documentation` tool serves svelte.dev's Svelte and SvelteKit documentation. For components and client code it is correct as written. Its server pages, such as `+page.server.ts`, `+server.ts`, `hooks.server.ts`, `$env` and adapters, describe Kit's JavaScript server; in SKGo those are Go, and this documentation covers them. [Differences](/docs/skgo/differences) lists every departure.
