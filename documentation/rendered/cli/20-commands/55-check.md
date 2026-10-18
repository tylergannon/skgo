---
title: "skgo check"
summary: "Check the project and deliberately apply safe fixes"
---

Runs every analyzer `skgo` ships, `svelte-check`, lint and formatting over the application and reports. The `--json` form is what editors and agents consume; `skgo mcp` serves the same check as a tool.

```sh
skgo check
skgo check --fix
```

Ordinary checking never edits source. `--fix` runs `goimports -w`, the project's `vp check --fix`, and Prettier's Svelte formatter before running the full ordinary check again. For projects without VitePlus, it uses the existing Prettier and ESLint configuration instead. Frontend file selection and exclusions remain the project's formatter and linter rules. Go fixes exclude generated files, generated route links, dependencies, test fixtures and symlinks.

Fix mode applies formatting, imports and safe lint fixes only. It does not install tools, regenerate bindings or apply analyzer suggestions. Authored type errors and stale generated contracts still fail. A failed fixer remains in the report even if a later check succeeds; missing tools also fail.

```sh
skgo check [flags]
```

Runs the Go analyzers, `svelte-check`, lint and formatting over the
application and reports every finding. It does not edit unless `--fix` is selected,
which applies safe formatting, import and lint fixes before ordinary checking. The
`--json` report is what editors and agents read; `skgo mcp` serves the
same check as a tool.

| Flag | Default | Description |
|---|---|---|
| `--root` | `.` | project root containing go.mod |
| `--web` | `web` | frontend root, relative to --root |
| `--out` |  | generated Go bindings directory, relative to --root; detected when omitted |
| `--locals-package` |  | application locals Go import path (required) |
| `--locals-type` | `Locals` | application locals named struct |
| `--hook-package` |  | optional request hook Go import path |
| `--hook-symbol` | `Handle` | selected request hook symbol |
| `--fix` | `false` | apply safe formatting, import and lint fixes, then recheck |
| `--json` | `false` | write a structured JSON report |
