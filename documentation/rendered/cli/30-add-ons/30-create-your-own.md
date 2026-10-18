---
title: "Create your own"
summary: "Write a Go plugin that supplies SKGo add-ons and application templates."
---

SKGo add-ons and templates come from Go plugins: a `main` package built with `-buildmode=plugin` that exports `SKGoPluginV1`. Where sv's [community add-ons](https://svelte.dev/docs/cli/community) are npm packages, an SKGo plugin is a Go module compiled for the exact `skgo` executable that loads it.

## Quick start

A plugin is a Go module with a `skgo-plugin.json` and a `main` package that implements [`templateapi.Plugin`](/docs/skgo/skgo-templateapi):

```tree
skgo-receipt/
├── go.mod
├── skgo-plugin.json
└── template/
    └── main.go
```

```json
/// file: skgo-plugin.json
{ "package": "./template" }
```

The plugin below is the fixture SKGo's own tests install. `Describe` registers one add-on, `receipt`, and one template, `receipt-app`, whose single step applies that add-on with the template's `message` bound to the add-on's `text`. `Apply` writes a file, leaves an identical one alone and refuses to overwrite a different one:

```go
/// file: template/main.go
package main

import (
	"context"
	"fmt"
	"github.com/tylergannon/skgo/templateapi"
	"os"
	"path/filepath"
)

var skgoVersion = "(devel)"

type addon struct{}

func SKGoPluginV1() templateapi.Plugin { return addon{} }
func (addon) Describe() templateapi.Descriptor {
	return templateapi.Descriptor{ID: "external-fixture", Version: "1", SkgoVersion: skgoVersion, Addons: []templateapi.Addon{{Name: "receipt", Summary: "Write a configured receipt", Options: []templateapi.Option{{Name: "text", Help: "Receipt text", Required: true}}}}, Templates: []templateapi.Template{{Name: "receipt-app", Summary: "An application receipt", Options: []templateapi.Option{{Name: "message", Help: "Receipt message", Default: "template receipt"}}, Steps: []templateapi.Step{{Addon: "receipt", Bindings: map[string]string{"text": "message"}}}}}}
}
func (addon) Apply(ctx context.Context, r templateapi.Request) (templateapi.Result, error) {
	if err := ctx.Err(); err != nil {
		return templateapi.Result{}, err
	}
	path := filepath.Join(r.Project.Root, "receipt.txt")
	data := []byte(r.Project.Name + "\n" + r.Project.Module + "\n" + r.Options["text"] + "\n")
	old, err := os.ReadFile(path)
	if err == nil {
		if string(old) == string(data) {
			return templateapi.Result{}, nil
		}
		return templateapi.Result{}, fmt.Errorf("receipt.txt conflicts with existing content")
	}
	if !os.IsNotExist(err) {
		return templateapi.Result{}, err
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return templateapi.Result{}, err
	}
	return templateapi.Result{Changed: []string{"receipt.txt"}}, nil
}
func main() {}
```

Installed, `skgo add --set text=Hello receipt` writes `receipt.txt` into an application, and `skgo new --template receipt-app --set message=Hello myapp` creates a new application and applies the same add-on to it.

## Project structure

`skgo-plugin.json` has one field, `package`: the `main` package relative to the module root, as `.` or one clean `./relative/path`. Unknown fields are rejected.

`skgoVersion` is a string variable in the `main` package. The builder sets it to the loading executable's version, and `Describe` must return it as `SkgoVersion`; a plugin whose value differs from the host is reported and not loaded. [templateapi](/docs/skgo/skgo-templateapi) has the rest of the contract: `Describe` must not touch the project, `Apply` must honour cancellation, preserve user edits and reject incompatible files before writing, and `Next` carries instructions, never commands for `skgo` to run.

## Development

Code generation is ordinary `go generate` in the plugin package, with the generators' dependencies declared in `go.mod`. [`skgo plugin install`](plugin) runs it; there are no other hooks.

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

Publish the module from a Git repository Go can resolve directly, tagged like any Go module. Users install it for the executable they run with [`skgo plugin install`](plugin):

```sh
skgo plugin install example.com/skgo-receipt@v1.0.0
```

The installer fetches with `GOPROXY=direct`, verifies the checkout against Go's module hashes, builds with the host's exact toolchain and qualifies the result beside the user's other plugins before publishing it. Archives and prebuilt `.so` files cannot be installed.

## Version compatibility

A plugin is built for one host. The plugin's `go` directive cannot be newer than the host's Go version. Relative `replace` directives, in the plugin or in the host's recorded build, are rejected, and so is a host built with profile-guided optimisation. Each SKGo release, and each application's `go tool skgo`, needs its own build; managed installs live under `~/.skgo/plugins/hosts/<executable-sha256>/`.

## Examples

The quick start is [`cmd/skgo/testdata/template-plugin/plugin.go`](https://github.com/tylergannon/skgo/blob/main/cmd/skgo/testdata/template-plugin/plugin.go). The built-in [`native-app`](native-app) add-on implements the same interface in [`nativeapp/plugin.go`](https://github.com/tylergannon/skgo/blob/main/nativeapp/plugin.go).
