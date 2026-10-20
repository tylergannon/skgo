---
title: Skills
description: Install the SKGo and polytype agent skills, and use them with Svelte's.
division: ai
section: Skills
order: 4010
---

SKGo publishes two skills, sets of instructions an agent loads on demand: the [`skgo` skill](https://github.com/tylergannon/skgo/blob/main/skills/skgo/SKILL.md) for building SKGo applications, and the [polytype skill](https://github.com/tylergannon/polytype/tree/main/skills/polytype) for the Go types that cross the wire.

## Installing

Install both with the `skills` CLI, through VitePlus:

```sh
vp dlx skills add tylergannon/skgo -y
vp dlx skills add tylergannon/polytype -y
```

Add `-g` to install them for all projects rather than the current one. The CLI fetches each skill from its GitHub repository, not from the SKGo release your `go.mod` pins, so a skill can describe a newer release than the one you build with.

## SKGo

The `skgo` skill applies to SvelteKit applications served by Go. In its own words: "Use for Go remote functions, server loads, route handlers, server composition, generation, development, and deployment. Do not use for ordinary SvelteKit apps whose server runs in JavaScript."

It keeps the ownership boundary the [AGENTS.md block](/docs/ai/agents-md) states, and adds how to declare remote functions, where loads and routes live, how the server composition is built, Kit's invariants to preserve, and how to verify against the running binary. Where the skill and these pages disagree, these pages describe the pinned release.

## polytype

SKGo generates TypeScript types and codecs for wire types through [polytype](https://github.com/tylergannon/polytype). Its skill explains polytype's type grammar and what each projection accepts and refuses, including `polytype.Optional[T]`, `polytype.Nullable`, enums and sealed unions. Load it when the agent designs the types a remote function, load or form uses, or when `skgo check` reports `SKGO007`, the unsupported-wire-value diagnostic.

## Svelte skills

Svelte's [skills](https://svelte.dev/docs/ai/skills) still apply to `.svelte` files and `.svelte.ts` modules. The `svelte-code-writer` skill runs the `@sveltejs/mcp` CLI with `npx`, which does not run in an SKGo project's `web/`; when the `ai-tools` add-on installs it during `skgo new`, SKGo rewrites those commands to VitePlus, as [The Svelte MCP server](/docs/ai/svelte-mcp#With-skgo-new) describes.
