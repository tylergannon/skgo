---
title: skgo check
description: Check the project without editing.
division: cli
section: Commands
order: 2055
---

Runs every analyzer `skgo` ships, `svelte-check`, lint and formatting over the application and reports. The `--json` form is what editors and agents consume; `skgo mcp` serves the same check as a tool.

It edits nothing and has no fix mode: where a diagnostic carries a `fix`, that names the command or change that would repair it. `goimports` and `staticcheck` must be on `PATH`, and the frontend checkers installed under `web/node_modules`; a missing tool is a failed check, not a skipped one. It does not run `svelte-kit sync`: when the frontend's `tsconfig.json` or `jsconfig.json` extends Kit's generated `$app/tsconfig`, `svelte-check` fails until sync has written it.

## Usage

```text skgo check -h
```
