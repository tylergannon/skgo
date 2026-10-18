---
title: "skgo new"
summary: "Create an application"
---

The first command you run. It scaffolds a SvelteKit application with `sv create`, then adds the Go application server, the bindings package, a Justfile and the configuration that ties them together. See [Creating a project](/docs/skgo/creating-a-project) for the walk-through.

```sh
skgo new [flags] DIR [-- SV_CREATE_OPTIONS]
```

VitePlus delegates creation to sv; skgo adds the Go application server,
the generated bindings package and a Justfile. Options after `--` belong
to `sv create`; without a terminal the base is minimal TypeScript.
Templates registered by installed plugins are applied after that scaffold.

```sh
skgo new myapp -- --template demo --types jsdoc --add tailwindcss=plugins:none
```

| Flag | Default | Description |
|---|---|---|
| `--module` |  | Go module path; defaults to the project name |
| `--name` |  | application name; defaults to the target directory name |
| `--origin` | `http://127.0.0.1:8080` | public browser origin |
| `--skgo-version` |  | skgo module version; defaults to this command's release |
| `--sv-addon` |  | sv add-on package override for checkout qualification |
| `--adapter` |  | adapter package override for checkout qualification |
| `--skgo-replace` |  | local skgo module replacement for checkout qualification |
| `--template` |  | application template applied after the ordinary minimal TypeScript scaffold |
| `--set` |  | template option NAME=VALUE (repeatable) |
