---
title: "skgo native build"
summary: "Build the macOS or iOS application"
---

Builds the native shell for one platform from the application's native configuration. Requires Xcode on macOS; `--prepare-only` stops before the Xcode step for tooling that owns it.

```sh
skgo native build [flags]
```

Prepares bindings, native libraries and the Xcode project for the selected
platform and compiles a Debug application. `--prepare-only` stops before
Xcode, which is what an editor or CI step that owns the Xcode invocation wants.

| Flag | Default | Description |
|---|---|---|
| `--root` | `.` | application root |
| `--platform` | `macos` | macos, iphone or simulator |
| `--preset` |  | application-defined build preset |
| `--prepare-only` | `false` | prepare bindings, native libraries and Xcode project without compiling a Debug app |
| `--jobs` | `0` | maximum Zig/Xcode build workers; zero uses the tool defaults |
