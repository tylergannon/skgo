---
title: "JSON output"
summary: "The JSON that skgo check, skgo advice and skgo buildinfo print with --json."
---

Three commands print JSON with `--json`. Each writes one indented JSON document to standard output; an error that prevents the document goes to standard error.

## skgo check --json

[`skgo check`](/docs/cli/check) prints a report. Trimmed, with one `SKGO001` finding and no frontend tools installed:

```json
{
  "ok": false,
  "checks": [
    { "name": "goimports", "status": "complete" },
    { "name": "svelte-check", "status": "failed", "message": "project svelte-check is missing" },
    { "name": "go-build", "status": "complete" },
    { "name": "skgo-advice", "status": "complete" }
  ],
  "diagnostics": [
    {
      "source": "skgo-advice",
      "code": "SKGO001",
      "severity": "error",
      "message": "This Query cannot change cookies; Kit refuses the write and cached queries may skip it. Move the cookie change into a command or form.",
      "location": { "file": "web/src/routes/account/account.remote.go", "line": 10, "column": 6, "endLine": 10, "endColumn": 55 },
      "documentation": "skgo advice SKGO001"
    }
  ]
}
```

`ok` is `true` only when every check is `complete` and no diagnostic has severity `error`.

### checks

One entry per checker that ran, in order: `goimports`; then `vite-plus` and `svelte-format` when the project has VitePlus, otherwise `prettier` and `eslint`; `svelte-check`; `go-build`; `go-vet`; `staticcheck`; `skgo-advice`; and `skgo`, generation's own checks. A `project` or `go-overlay` entry appears only when the project could not be read at all.

| Field | Meaning |
| --- | --- |
| `name` | The checker |
| `status` | `complete`: it ran and reported. `failed`: it could not run, usually a missing tool, or, for `skgo`, generation refused the project. `incomplete`: it stopped before covering everything, such as `go-vet` after a compile error |
| `message` | Optional detail, such as the missing tool |

### diagnostics

| Field | Meaning |
| --- | --- |
| `source` | The checker that reported it, as named in `checks`, except that VitePlus reports as `vite-plus/fmt` or `vite-plus/lint` and svelte-check as `svelte-check/<source>` |
| `code` | The rule. `SKGO001`–`SKGO008` from SKGo's analyzers; from `skgo`, `SKGO007` or a category such as `contract`, `duplicate-declaration` or `stale-route-link`; `format` for formatting; `go` for compiler and vet output; otherwise the tool's own rule id |
| `severity` | `error` or `warning` |
| `message` | The tool's message |
| `location` | Optional. `file` relative to the project root, plus optional `line`, `column`, `endLine` and `endColumn`, all 1-based |
| `related` | Optional. Further locations, in the same shape |
| `fix` | Optional. The command that would repair it, such as `goimports -w` or `vp fmt --write` with the file |
| `documentation` | Optional. A URL for a tool's rule, or `skgo advice <code>` for SKGo's |

Locations name the files you wrote, never the generated route copies under the bindings' `links/` directory.

## skgo advice --json

[`skgo advice`](/docs/cli/advice) prints an array of entries: all eight, or the one named by a code argument, as in `skgo advice --json SKGO002`.

```json
[
  {
    "code": "SKGO002",
    "title": "Page input in a query",
    "version": "%skgo%",
    "kitVersion": "3.1.0",
    "consequence": "Kit keys the query cache by its arguments, not the current page URL or route.",
    "repair": "Pass the page value as a typed query argument from the caller.",
    "example": "func article(ctx context.Context, slug string) (string, error) { return slug, nil }\nvar _ = skgo.Query(article) // the page calls article(page.params.slug)"
  }
]
```

`version` is the SKGo version of the running binary, `(devel)` for a build without one, and `kitVersion` is the Kit release that binary mirrors. `example` is a Go fragment that shows valid usage; it is not an edit to apply.

## skgo buildinfo --json

[`skgo buildinfo`](/docs/cli/buildinfo) identifies the running binary:

```json
{
  "executable": "/Users/you/go/bin/skgo",
  "skgoVersion": "%skgo%",
  "build": {
    "GoVersion": "go1.27.2",
    "Path": "github.com/tylergannon/skgo/cmd/skgo",
    "Main": { "Path": "github.com/tylergannon/skgo", "Version": "%skgo%", "Sum": "h1:…" },
    "Deps": [{ "Path": "github.com/tylergannon/polytype", "Version": "…", "Sum": "h1:…" }],
    "Settings": [{ "Key": "GOOS", "Value": "darwin" }]
  }
}
```

`executable` is the resolved path of the binary that answered, which for `go tool skgo` is a file in Go's build cache. `build` is Go's [`debug.BuildInfo`](https://pkg.go.dev/runtime/debug#BuildInfo), encoded with its Go field names.

## Exit status

| Command | 0 | 1 | 2 |
| --- | --- | --- | --- |
| `skgo check` | `ok` is `true` | `ok` is `false`; the report is still printed | Invalid flags or arguments |
| `skgo advice` | Printed | — | Unknown code, or more than one argument |
| `skgo buildinfo` | Printed | No build information, or a positional argument | — |
