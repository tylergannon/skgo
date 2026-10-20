---
title: Create your own
description: Write a Go plugin that supplies SKGo add-ons and application templates.
division: cli
section: Add-ons
order: 3030
---

Apart from the built-in `tailwindcss` add-on, SKGo add-ons and templates come from Go plugins: a `main` package built with `-buildmode=plugin` that exports `SKGoPluginV1`. Where sv's [community add-ons](https://svelte.dev/docs/cli/community) are npm packages, an SKGo plugin is a Go module compiled for the exact `skgo` executable that loads it.

## Quick start

A plugin is a Go module with a `skgo-plugin.json` and a `main` package that implements [`templateapi.AddonPlugin`](/docs/reference/skgo/templateapi#AddonPlugin):

```tree
skgo-receipt/
├── go.mod
├── skgo-plugin.json
└── template/
    └── main.go
```

`skgo-plugin.json` names the `main` package:

```json
{ "package": "./template" }
```

The plugin below, `template/main.go` in that layout, is the fixture SKGo's own tests install. `Describe` registers one add-on, `receipt`, and one template, `receipt-app`, whose single step applies that add-on with the template's `message` bound to the add-on's `text`. `Apply` writes a file, leaves an identical one alone and refuses to overwrite a different one:

```go cmd/skgo/testdata/template-plugin/plugin.go
```

Installed, `skgo add --set text=Hello receipt` writes `receipt.txt` into an application, and `skgo new --template receipt-app --set message=Hello myapp` creates a new application and applies the same add-on to it.

## Project structure

`skgo-plugin.json` has one field, `package`: the `main` package relative to the module root, as `.` or one clean `./relative/path`. Unknown fields are rejected.

`skgoVersion` is a string variable in the `main` package. The builder sets it to the loading executable's version, and `Describe` must return it as `SkgoVersion`; a plugin whose value differs from the host is reported and not loaded. [templateapi](/docs/reference/skgo/templateapi) has the rest of the contract: `Describe` must not touch the project, `Apply` must honour cancellation, preserve user edits and reject incompatible files before writing, and `Next` carries instructions, never commands for `skgo` to run.

## Development

Code generation is ordinary `go generate` in the plugin package, with the generators' dependencies declared in `go.mod`. [`skgo plugin install`](/docs/cli/plugin) runs it; there are no other hooks.

To build a local checkout against an installed `skgo`, use `skgo-plugin-build`. `--host` names a standalone executable; `--project` names an application whose `go tool skgo` is the host:

```sh
go run github.com/tylergannon/skgo/cmd/skgo-plugin-build@latest \
  --host "$(command -v skgo)" --source ./skgo-receipt --package ./template \
  --out "$HOME/.skgo/plugins/standalone/receipt.so"

go run github.com/tylergannon/skgo/cmd/skgo-plugin-build@latest \
  --project ./myapp --source ./skgo-receipt --package ./template \
  --out "$HOME/.skgo/plugins/myapp/receipt.so"
```

Its other flags:

- `--out FILE`: the `.so` to write
- `--version-symbol SYMBOL`: the variable set to the host's version; default `main.skgoVersion`
- `--skgo-source DIR`: an SKGo checkout, for a host built from unreleased source

It builds a staged copy pinned to the host's shared dependencies, so neither your `go.mod` nor the application's changes. The stage has no `.git`, `node_modules`, `ephemeral`, `.local`, `.cache`, `build` or `.build` directories and rejects symlinks, so a generator cannot rely on `git describe` or on files outside the module. Unlike `skgo plugin install`, it does not run `go generate`; prepare the source first.

## Testing

`skgo-plugin-build` finishes by loading the result into the host with `add --help`. To use it, point the host at the output directory:

```sh
SKGO_PLUGIN_DIRS=~/.skgo/plugins/standalone skgo add --help
SKGO_PLUGIN_DIRS=~/.skgo/plugins/standalone skgo add --set text=Hello receipt
```

A nonempty `SKGO_PLUGIN_DIRS` replaces the default manual directory, `~/.skgo/plugins`, but never hides a managed install. This checks the plugin alone; only `skgo plugin install` qualifies a plugin against the ones already present.

## Publishing

Publish the module from a Git repository Go can resolve directly, tagged like any Go module. Users install it for the executable they run with [`skgo plugin install`](/docs/cli/plugin):

```sh
skgo plugin install example.com/skgo-receipt@v1.0.0
```

The installer fetches with `GOPROXY=direct`, verifies the checkout against Go's module hashes, builds with the host's exact toolchain and qualifies the result beside the user's other plugins before publishing it. Exact prebuilt `.so` releases can also be installed with a checksum; see [Plugin capabilities](/docs/skgo/plugin-capabilities).

## Version compatibility

A plugin is built for one host. The plugin's `go` directive cannot be newer than the host's Go version. Relative `replace` directives, in the plugin or in the host's recorded build, are rejected, and so is a host built with profile-guided optimisation. Each SKGo release, and each application's `go tool skgo`, needs its own build; managed installs live under `~/.skgo/plugins/hosts/<executable-sha256>/`.

## Examples

The quick start is [`cmd/skgo/testdata/template-plugin/plugin.go`](https://github.com/tylergannon/skgo/blob/main/cmd/skgo/testdata/template-plugin/plugin.go). Native-shell implements the same add-on interface in its separate module under `plugins/native-shell`. Generator and command capabilities are optional; see [Plugin capabilities](/docs/skgo/plugin-capabilities).
