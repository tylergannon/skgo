---
title: skgo generate
description: Generate the glue.
division: cli
section: Commands
order: 2050
---

The command behind every `.remote.ts`, `+page.server.ts` stub and `skgo_gen.go` in an application. It runs from the `go:generate` directive in the bindings package and again on every change under `skgo dev`.

## Usage

`skgo generate --help` prints:

```text
Usage of generate:
  -cache-dir string
    	generation cache directory; defaults to the user cache; off disables caching
  -config-package string
    	optional shared ApplicationConfig Go import path
  -config-symbol string
    	selected ApplicationConfig value (default "Config")
  -hook-package string
    	optional request hook Go import path
  -hook-symbol string
    	selected request hook symbol (default "Handle")
  -locals-package string
    	application locals Go import path (required)
  -locals-type string
    	application locals named struct (default "Locals")
  -out string
    	the directory of the generated bindings package (default ".")
  -package string
    	the name of the generated bindings package; defaults to the base name of --out
  -quiet
    	do not list the files written
  -swift-out string
    	generated Swift source file
  -swift-remote value
    	select module#export for Swift and the shared safe-number contract (ordinary query/command; repeatable)
  -web string
    	the vite root: the directory holding src/ and package.json (default "../web")
```
