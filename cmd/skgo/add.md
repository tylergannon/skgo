---
title: skgo add
description: Apply an add-on to an application.
division: cli
section: Commands
order: 2020
---

An add-on is a reusable change to an existing application: the built-in `native-app` add-on, or one a plugin provides. `skgo add --help` lists what this binary knows.

## Usage

With no plugins installed, `skgo add --help` prints the following; add-ons from installed plugins are listed beside `native-app`.

```text
usage: skgo add [--root DIR] [--name NAME] [--set KEY=VALUE] ADDON
  -name string
    	explicit application identity for older projects; must match an existing identity
  -root string
    	skgo application root (default ".")
  -set value
    	add-on option NAME=VALUE (repeatable)
Plugins are trusted native code, loaded eagerly in sorted file order. They must match this host toolchain and shared dependencies; dependencies shared only between plugins must also match. Incompatible plugins cannot coexist: remove conflicting manual files or use skgo plugin remove MODULE for current-host managed installations. SKGO_PLUGIN_DIRS only changes manual discovery.
Available add-ons:
native-app: Add an embedded Go host and shared macOS/iOS application shell
    --set bundle-id=VALUE: Application bundle identifier without preset suffix (default "", required true)
```
