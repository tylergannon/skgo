---
title: "Tools"
summary: "Reference for the skgo_check and skgo_advice tools that skgo mcp serves."
---

The [SKGo MCP server](mcp) provides the following tools to the model you are using, which decides when to call them during a session.

Each tool returns its result twice: as `structuredContent`, and as the same JSON in a single `text` item of `content`, for clients that read only `content`.

## skgo_check

Runs [`skgo check`](/docs/cli/check) and returns its report. For the same project the report is identical to the output of `skgo check --json`; [JSON output](json-output#skgo-check-json) describes its fields.

Every argument is an optional string:

| Argument | Default | Meaning |
| --- | --- | --- |
| `root` | `.` | The project root containing `go.mod`, relative to the server's working directory |
| `web` | `web` | The frontend root, relative to `root` |
| `out` | `internal/skgo` if it exists, otherwise `generated` | The generated bindings directory, relative to `root` |

The locals and hook settings come from the project's `go:generate` directive, as they do for `skgo check` run without flags. The result has `isError: true` whenever the report's `ok` is `false`: a diagnostic with severity `error`, or a checker that did not complete. Warnings alone do not set it.

The tool never edits files. Where a diagnostic has a `fix`, it names the command or change that would.

## skgo_advice

Returns the guidance this SKGo version ships for its diagnostic codes as `{"advice": [...]}`, the array [`skgo advice --json`](json-output#skgo-advice-json) prints: all eight entries, or the one named by `code`, an exact code such as `SKGO002`. An unknown code returns `{"error": "unknown skgo advice code SKGO099"}` with `isError: true`.
