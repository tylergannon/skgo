---
title: AGENTS.md
description: The instructions SKGo gives a coding agent at the application root.
division: ai
section: Instructions
order: 2010
---

[`go tool skgo add ai-tools`](/docs/ai/ai-tools) writes an [`AGENTS.md`](https://agents.md) at the application root, the directory that holds `go.mod`, and a `CLAUDE.md` that imports it for Claude Code. It tells the agent:

- which code is ordinary SvelteKit under `web/src`, and which is Go: server loads, form actions, `+server` endpoints, remote functions and the request hook
- not to write server code in TypeScript, and which files are generator output
- to use VitePlus (`vp`) for frontend tooling, never `npm`, `npx` or `pnpm`
- to run `go generate ./...` after changing a Go route, and `go tool skgo check` (or the `skgo_check` MCP tool) and `go test ./...` before finishing, reading `go tool skgo advice` for an `SKGO001`–`SKGO008` diagnostic
- to use the Svelte MCP server for Svelte code, whose pages on server code describe Kit's JavaScript server rather than this application's Go
- to check server-rendered behaviour against the running binary with JavaScript disabled

The add-on leaves an existing `AGENTS.md` or `CLAUDE.md` alone. To add the instructions to one, copy them from [`internal/aitools/instructions.md`](https://github.com/tylergannon/skgo/blob/main/internal/aitools/instructions.md).

`skgo check` needs `goimports` and `staticcheck` on `PATH` and the project's own `vp`, `prettier` and `svelte-check` under `web/node_modules`; a missing tool is a failed check, not a skipped one.
