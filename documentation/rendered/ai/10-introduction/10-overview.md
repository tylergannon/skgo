---
title: "Overview"
summary: "What SKGo gives a coding agent, and how it sits beside the Svelte AI tools."
---

These pages cover what SKGo provides to a coding agent working on the Go half of an application. Each piece stands on its own:

- [AGENTS.md](agents-md): a block for your application's `AGENTS.md` or `CLAUDE.md` that tells an agent where server code goes and which commands to run
- [MCP server](mcp): `skgo mcp`, a local server with two [tools](tools), one that runs `skgo check` and one that returns the repair guidance behind SKGo's diagnostic codes
- [Skills](skills): the SKGo and polytype skills, which an agent loads when it writes SKGo code or Go types that cross the wire
- [JSON output](json-output): the reports `skgo check`, `skgo advice` and `skgo buildinfo` print with `--json`, for agents and CI
- [llms.txt](llms-txt): this documentation as plain markdown

The MCP server serves checks and guidance, not documentation. For documentation, give the agent the [llms.txt](llms-txt) files.

## Alongside the Svelte AI tools

The frontend of an SKGo application is ordinary SvelteKit, so the [Svelte AI tools](https://svelte.dev/docs/ai/overview) apply to components, runes, routing and client code as they do in any Kit project. Use both: the Svelte tools for `.svelte` files, the SKGo tools for Go and for checking the whole project.

The Svelte tools' default setup starts the Svelte MCP server with `npx`, which does not run in an SKGo project's `web/`. [The Svelte MCP server](svelte-mcp) shows the VitePlus form.
