---
title: "sv add-ons"
summary: "How skgo new uses sv, and which official sv add-ons fit an SKGo project."
---

`skgo new` creates the frontend with `sv create`, through VitePlus, so you choose official [sv add-ons](https://svelte.dev/docs/cli/sv-add#Official-add-ons) during creation exactly as in any Kit project. It then runs its own sv add-on, `@skgo/sv`, over the result. An add-on fits an SKGo project when it changes only the frontend, because the server is Go.

## The SKGo sv add-on

`skgo new` runs `@skgo/sv` after your add-ons, with `sv add`; applications do not depend on it at runtime. It:

- replaces any `@sveltejs/adapter-*` with `@skgo/sveltekit-adapter`, at the version paired with your `skgo`
- turns on `experimental.remoteFunctions` and `compilerOptions.experimental.async`
- replaces sv's `demo` template, a JavaScript server application, with SKGo's examples starter, keeping what other add-ons put in the layout
- pins Vitest to the version VitePlus ships and routes the `test` script through `vp`

It refuses any VitePlus other than 1.3.0. `skgo new`, not the add-on, configures the [Kit patch](kit-patch) and installs dependencies.

## Add-ons that fit

| Add-on | In an SKGo project |
| --- | --- |
| `vitest` | Added for you if you do not choose it; pinned to VitePlus's Vitest |
| `storybook` | Added for you if you do not choose it |
| `playwright` | Its `webServer` becomes `cd .. && just serve`, against the application's `ORIGIN` |
| `tailwindcss` | Its stylesheet import survives the examples starter |
| `ai-tools` | Its MCP configuration is repointed at VitePlus; see [below](#ai-tools) |
| `eslint`, `prettier`, `mdsvex`, `enhanced-img`, `experimental` | Untouched; they change only the frontend |

## Add-ons SKGo cannot use

`skgo new` reads the project sv wrote and refuses to continue when an add-on left JavaScript server code behind: anything under `src/lib/server`, a `hooks.server.*`, or a `+server.*`, `+page.server.*` or `+layout.server.*` file. The message names the files; create the project again without that add-on.

| Add-on | What it writes |
| --- | --- |
| `drizzle` | `src/lib/server/db/`, a `drizzle.config.*` and `.env` entries |
| `better-auth` | `src/lib/server/auth.*`, `src/hooks.server.*` and demo `+page.server.*` routes |
| `paraglide` | a middleware in `src/hooks.server.*` and a `reroute` hook, which SKGo does not support |
| `sveltekit-adapter` | another adapter in place of `@skgo/sveltekit-adapter` |

Added later with `sv add`, they collide with what SKGo generates: an authored `src/hooks.server.*` stops `skgo generate`, which owns that file, and the server files beside a route are generated from your Go. Write the equivalent in Go instead; [Differences](/docs/skgo/differences) lists each departure. Never run `sveltekit-adapter` on an SKGo project.

## ai-tools

The [ai-tools](https://svelte.dev/docs/cli/ai-tools) add-on writes an MCP configuration whose command is `npx`, which does not run in an SKGo project's `web/`. `@skgo/sv` rewrites it to the project's VitePlus; [The Svelte MCP server](/docs/ai/svelte-mcp#With-skgo-new) shows the result.
