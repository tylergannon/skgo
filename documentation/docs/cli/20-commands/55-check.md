---
title: skgo check
summary: Check the project and deliberately apply safe fixes
---

Runs every analyzer `skgo` ships, `svelte-check`, lint and formatting over the application and reports. The `--json` form is what editors and agents consume; `skgo mcp` serves the same check as a tool.

```sh
skgo check
skgo check --fix
```

Ordinary checking never edits source. `--fix` runs `goimports -w`, the project's `vp check --fix`, and Prettier's Svelte formatter before running the full ordinary check again. For projects without VitePlus, it uses the existing Prettier and ESLint configuration instead. Frontend file selection and exclusions remain the project's formatter and linter rules. Go fixes exclude generated files, generated route links, dependencies, test fixtures and symlinks.

Fix mode applies formatting, imports and safe lint fixes only. It does not install tools, regenerate bindings or apply analyzer suggestions. Authored type errors and stale generated contracts still fail. A failed fixer remains in the report even if a later check succeeds; missing tools also fail.

> COMMAND: skgo check
