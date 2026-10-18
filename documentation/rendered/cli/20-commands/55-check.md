---
title: "skgo check"
summary: "Check the project without editing"
---

Runs every analyzer `skgo` ships, `svelte-check`, lint and formatting over the application and reports. The `--json` form is what editors and agents consume; `skgo mcp` serves the same check as a tool.

```sh
skgo check [flags]
```

Runs the Go analyzers, `svelte-check`, lint and formatting over the
application and reports every finding without changing a file. The
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
| `--json` | `false` | write a structured JSON report |
