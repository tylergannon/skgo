---
title: skgo plugin
description: Install trusted plugins and run their commands.
division: cli
section: Commands
order: 2015
---

Plugins provide templates, add-ons, generators and commands. They execute trusted local code inside the CLI; generated applications never load the emitter plugin.

## Usage

```text skgo plugin -h
```

## Prebuilt releases

Use the official release executable. An artifact name contains the plugin ID, plugin version, exact skgo version and host platform, such as `native-shell-v0.1.29-skgo-v0.41.0-darwin-arm64.so`. Install with `skgo plugin install --prebuilt ARTIFACT --sha256 DIGEST`. Add `--select MODULE#EXPORT` for each remote and `--set NAME=VALUE` for provider options.

Installation verifies the checksum, rejects mismatched versions/platforms and probes the actual host before writing `skgo-plugins.json`. Use the release HTTPS URL for a shared pin. Local staged files are recorded relative to the project and require that file to remain available. Commit a portable declaration; `skgo plugin restore` restores its exact bytes and configuration without repeating shell setup. It does not compile source or choose a substitute version. Commands use `skgo plugin ID ...`. Generators run explicitly with `skgo generate --plugins` from the bindings directory, with the same generation flags as ordinary generation.

A plugin release supports one exact official skgo release. A Go-tool build from an application's dependency graph can have a different ABI even when its version string matches, and is unsupported for prebuilt plugins. Upgrading core can require a new plugin release rebuilt from unchanged source.

## Source templates and add-ons

Existing `skgo plugin install MODULE[@QUERY]` remains available for template/add-on modules. It builds trusted Git source for this executing binary, requiring Git, Go and cgo. `skgo plugin remove MODULE` removes that host's source-installed selection. To remove a project prebuilt provider, remove its entry from `skgo-plugins.json`.

[Plugin capabilities](/docs/skgo/plugin-capabilities) documents the public authoring contracts.
