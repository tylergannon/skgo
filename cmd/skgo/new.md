---
title: skgo new
description: Create an application.
division: cli
section: Commands
order: 2010
---

The first command you run. It scaffolds a SvelteKit application with `sv create`, then adds the Go application server, the bindings package, a Justfile and the configuration that ties them together. See [Creating a project](/docs/skgo/creating-a-project) for the walk-through.

## Usage

With no plugins installed, `skgo new --help` prints the following; each installed template is listed after the last line.

```text
usage: skgo new [flags] DIR [-- SV_CREATE_OPTIONS]

VitePlus delegates creation to sv; skgo adds the Go application server.
Options after -- belong to sv create; without a terminal the base is minimal TypeScript.
Example: skgo new myapp -- --template demo --types jsdoc --add tailwindcss=plugins:none
  -adapter string
    	adapter package override for checkout qualification
  -module string
    	Go module path; defaults to the project name
  -name string
    	application name; defaults to the target directory name
  -origin string
    	public browser origin (default "http://127.0.0.1:8080")
  -set value
    	template option NAME=VALUE (repeatable)
  -skgo-replace string
    	local skgo module replacement for checkout qualification
  -skgo-version string
    	skgo module version; defaults to this command's release
  -sv-addon string
    	sv add-on package override for checkout qualification
  -template string
    	application template applied after the ordinary minimal TypeScript scaffold
Plugins are trusted native code, loaded eagerly in sorted file order. They must match this host toolchain and shared dependencies; dependencies shared only between plugins must also match. Incompatible plugins cannot coexist: remove conflicting manual files or use skgo plugin remove MODULE for current-host managed installations. SKGO_PLUGIN_DIRS only changes manual discovery.
Available templates:
```
