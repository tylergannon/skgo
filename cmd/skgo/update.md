---
title: skgo update
description: Align the application with this release.
division: cli
section: Commands
order: 2025
---

Run it after installing a newer `skgo`. It moves the application's module, adapter and frontend tool pins to this release's and regenerates, so the binary and the application agree again.

## Usage

`skgo update --help` prints:

```text
Usage of update:
  -complete-version string
    	internal completion phase: must match this executable
  -native-platform string
    	native build target when native configuration exists: macos, iphone or simulator (default "macos")
  -native-preset string
    	native build preset from the application configuration
  -root string
    	application root; otherwise locate the current application
  -vp string
    	global vp executable; defaults to PATH
```
