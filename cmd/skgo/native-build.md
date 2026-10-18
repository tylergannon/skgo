---
title: skgo native build
description: Build the macOS or iOS application.
division: cli
section: Commands
order: 2030
---

Builds the native shell for one platform from the application's native configuration. Requires Xcode on macOS; `--prepare-only` stops before the Xcode step for tooling that owns it.

## Usage

`skgo native build --help` prints:

```text
Usage of native build:
  -jobs int
    	maximum Zig/Xcode build workers; zero uses the tool defaults
  -platform string
    	macos, iphone or simulator (default "macos")
  -prepare-only
    	prepare bindings, native libraries and Xcode project without compiling a Debug app
  -preset string
    	application-defined build preset
  -root string
    	application root (default ".")
```
