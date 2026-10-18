---
title: "skgo kit-patch"
summary: "Configure or verify the Kit queue correction"
---

SKGo corrects Kit's post-build prerender queue with a project-local pnpm patch. New applications are configured by `skgo new`; this command is for applications set up by hand and for CI to verify the installed bytes.

```sh
skgo kit-patch [flags] (--apply | --check)
```

skgo ships a correction to Kit's post-build prerender queue as a project-local
pnpm patch. `--apply` pins the supported Kit release and configures the
patch without installing; run the frontend's VitePlus install, then
`--check` verifies the configured files and the installed Kit bytes.
`--workspace`, when supplied, must equal `--web`; parent workspaces are
never modified.

| Flag | Default | Description |
|---|---|---|
| `--web` | `web` | selected frontend root containing package.json |
| `--workspace` |  | assert the selected frontend is this root; parent workspaces are never modified |
| `--apply` | `false` | pin the supported Kit release and configure the project-local pnpm patch (does not install) |
| `--check` | `false` | verify configured files and actual installed Kit queue bytes |
