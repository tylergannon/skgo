---
title: "skgo update"
summary: "Align the application with this release"
---

Run it after installing a newer `skgo`. It moves the application's module, adapter and frontend tool pins to this release's and regenerates, so the binary and the application agree again.

```sh
skgo update [flags]
```

Brings the application's skgo module, adapter and frontend tool environment to
the versions this executable was released with, then re-runs generation. When
the application has native configuration, the native target is rebuilt too.

| Flag | Default | Description |
|---|---|---|
| `--root` |  | application root; otherwise locate the current application |
| `--vp` |  | global vp executable; defaults to PATH |
| `--native-platform` | `macos` | native build target when native configuration exists: macos, iphone or simulator |
| `--native-preset` |  | native build preset from the application configuration |
| `--complete-version` |  | internal completion phase: must match this executable |
