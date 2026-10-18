---
title: skgo check
description: Check the project without editing.
division: cli
section: Commands
order: 2055
---

Runs every analyzer `skgo` ships, `svelte-check`, lint and formatting over the application and reports. The `--json` form is what editors and agents consume; `skgo mcp` serves the same check as a tool.

## Usage

`skgo check --help` prints:

```text
Usage of check:
  -config-package string
    	optional shared ApplicationConfig Go import path
  -config-symbol string
    	selected ApplicationConfig value (default "Config")
  -hook-package string
    	optional request hook Go import path
  -hook-symbol string
    	selected request hook symbol (default "Handle")
  -json
    	write a structured JSON report
  -locals-package string
    	application locals Go import path (required)
  -locals-type string
    	application locals named struct (default "Locals")
  -out string
    	generated Go bindings directory, relative to --root; detected when omitted
  -root string
    	project root containing go.mod (default ".")
  -web string
    	frontend root, relative to --root (default "web")
```
