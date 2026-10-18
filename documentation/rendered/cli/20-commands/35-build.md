---
title: "skgo build"
summary: "Build the frontend incrementally"
---

Generates and builds the frontend only when its inputs changed. The Justfile's `build` recipe calls it before `go build`, so a plain `just build` is always up to date.

```sh
skgo build [flags]
```

Runs generation and the frontend build only when their inputs changed since the
last build, recorded under `.skgo/` in the application root. The Go
binary is built by `go build` as usual; the Justfile a new application
gets chains the two.

| Flag | Default | Description |
|---|---|---|
| `--root` | `.` | application root |
