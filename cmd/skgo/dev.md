---
title: skgo dev
description: Run the development server.
division: cli
section: Commands
order: 2045
---

One listener serves the Go application with hot reload and Vite's dev server. Edits to Go files regenerate and rebuild the server; edits to Svelte files reach the browser through Vite as usual.

## Usage

`skgo dev --help` prints:

```text
Usage of dev:
  -generator-config string
    	optional application generator JSON configuration, relative to --root
  -go string
    	Go executable (default "go")
  -listen string
    	public listener address (next free port when occupied) (default "127.0.0.1:8080")
  -out string
    	generated bindings directory; detected when omitted
  -root string
    	application root containing go.mod (default ".")
  -strict-port
    	fail when the requested port is occupied
  -vp string
    	VitePlus executable (default "vp")
  -web string
    	frontend root relative to --root (default "web")
```
