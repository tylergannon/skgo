---
title: Plugin capabilities
description: Extend skgo with generators, commands, templates and add-ons through public contracts.
division: skgo
section: Extending skgo
order: 6110
---

Plugins are trusted Go shared objects loaded by the developer CLI. They export `SKGoPluginV1() templateapi.Plugin`. The public contract is [`templateapi`](/docs/reference/skgo/templateapi); the base interface describes the provider, and optional interfaces supply its capabilities. Generated applications do not load this `.so`.

## Capabilities

| Interface | Responsibility |
| --- | --- |
| `Plugin` | Return an ID, independent plugin version, exact skgo target and capability descriptor. Description and initialization have no project side effects. |
| `AddonPlugin` | Apply configured additions to a project. Report partial changes on failure, preserve user edits and honor cancellation. |
| `Generator` | Consume immutable, resolved operations and Polytype wire types. Return files and diagnostics without writing or reanalyzing application packages. |
| `CommandPlugin` | Parse arguments within `skgo plugin ID ...`, print help and run commands using supplied streams and cancellation. Return errors instead of exiting the host. |

A provider may implement any combination. Descriptor `Generator` and `Command` fields advertise the corresponding optional interfaces. Templates and add-ons require `AddonPlugin`. Duplicate IDs and management namespace conflicts are rejected, never silently replaced. Core owns loading, selection, managed output and process exit; plugins own their emitter and command behavior. Plugins depend on these public contracts, never on core's internal packages.

## Generation

[Generator plugins](/docs/skgo/generator-plugins) describes the resolved operation model, output ownership and example. Run declared generators explicitly through the matching official CLI with `skgo generate --plugins`. Ordinary `go generate`, web build and development do not discover or load them. Core validates every returned path, rejects authored or edited output and publishes only successful generation. No generator chooses server wire policy.

## Command forwarding

The host removes the plugin ID and forwards the remaining arguments unchanged. `CommandRequest` contains project identity, arguments, stdin, stdout, stderr, cancellation and the actual invoking host path. Use its streams; a plugin command must not call `os.Exit`. Return an error for failure. The host prints it and exits unsuccessfully. Plugin help uses the same namespace; `skgo plugin --help` lists installed command providers.

```go templateapi.ExampleCommandPlugin
```

## Exact prebuilt compatibility

A release artifact names its plugin version, one exact official skgo version and CLI host OS/architecture. For example, `native-shell-v0.1.29-skgo-v0.41.0-darwin-arm64.so`. The host platform describes the developer's machine, not the generated application's target.

Use [skgo plugin](/docs/cli/plugin) to install an artifact with its SHA256, selections and options. The project declaration `skgo-plugins.json` pins those values; restoration obtains exactly that artifact and repeats no application setup. Installation verifies bytes and probes the actual executable with installed peers. A version string alone does not guarantee Go plugin ABI compatibility: CI must reproduce the official executable's toolchain, build settings and shared dependencies and test the distributed pairing. Arbitrary rebuilt and project `go tool` hosts are unsupported for prebuilt installation.

Upgrading skgo may require a new native plugin version rebuilt from unchanged source. There is no compatibility range, negotiation, fallback compilation or automatic core-host download. Existing source-installed templates/add-ons keep their separate installation path.

## Native-shell

Native-shell lives in its own module under `plugins/native-shell`, with independent releases, proof and bundled SDK resources. It provides the existing Swift emitter and Apple shell, using the same public capabilities as other plugins. Install alone does not create a shell; Swift client generation can be used on its own. Native commands live under `skgo plugin native-shell ...`. Core has no native implementation import, built-in registration or native-specific command forwarding.

The extraction adds no native features. Removed entry points include core's `native build`, built-in `native-app`, `--swift-out` and `--swift-remote`, automatic native work during core update/web build, the old core-owned iOS probe, the native-specific identity check before creating `skgo.json`, and the inconsistent native-selection server guards. The plugin retains the `native-app` add-on through its own descriptor. Ordinary server behavior remains unchanged. The initial prebuilt host is macOS ARM64. Broader native lifecycle, iOS distribution and application migration are outside this extraction.
