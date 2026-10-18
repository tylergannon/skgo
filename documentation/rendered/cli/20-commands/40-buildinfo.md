---
title: "skgo buildinfo"
summary: "Identify the binary"
---

Prints which `skgo` is running and how it was built. Include its output in a bug report.

```sh
skgo buildinfo [flags]
```

Prints the executable's version, the Go release it was built with and the
module build settings, so a bug report or a toolchain check can name the exact
binary.

| Flag | Default | Description |
|---|---|---|
| `--json` | `false` | write executing binary and recorded Go build identity as JSON |
