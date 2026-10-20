---
title: Generator plugins
description: Extend generation using resolved remote operations and managed output files.
division: skgo
section: Add-ons
order: 3040
---

A generator plugin extends application generation through the same `SKGoPluginV1` factory as templates and add-ons. Its value implements [`templateapi.Generator`](/docs/reference/skgo/templateapi#Generator) and advertises `Generator: true` in its descriptor. It may also provide templates, add-ons or CLI commands. The generator runs in the developer CLI; generated applications do not load its `.so`.

Ordinary `go generate`, project-built `go tool skgo`, web builds and development remain plugin-free. Run the matching official executable's explicit `skgo generate --plugins` command to run configured generator providers. Prebuilt plugins target one exact official skgo release and host platform; arbitrary executables with the same version string do not establish Go plugin compatibility.

## Resolved input

Core analyzes the application once. Each [`GenerationRequest`](/docs/reference/skgo/templateapi#GenerationRequest) contains the application root/module/web paths, declared application name, configured selections/options and an immutable wire model. A selection is a Vite-root-relative remote module followed by `#export`; duplicate or unknown selections fail generation. An empty selection list includes every remote.

Each operation provides its export name, module, Kit endpoint ID, operation kind, source position and input/result indexes into Polytype's public wire grammar. Reachable named definitions accompany the roots. An absent argument, a form argument or a custom-transported value has index `-1`; `Transported` identifies custom transports. The plugin decides which supported operation kinds and wire shapes it can emit and reports unsupported selections. It must not load Go packages again, mutate the model or change server semantics.

This small provider emits a list using already resolved operation identities. Its example compiles and executes as an ordinary Go example:

```go templateapi.ExampleGenerator
```

## Managed output

Return the complete current file list in [`GenerationResult`](/docs/reference/skgo/templateapi#GenerationResult). Paths are application-root-relative, use forward slashes and may not traverse symlinks or escape the application. Core rejects duplicate destinations, another provider's files and reserved core outputs. Generated Go must be complete valid source; core formats it consistently.

The host records each provider's paths and content hashes in `skgo.plugin-outputs.json` in the bindings directory. Existing authored files, including byte-identical files without an ownership record, and edited generated files are protected; move or remove them before regenerating. Successful generation removes unchanged stale files owned by the invoked provider, preserves other providers' files and does not rewrite identical content. Plugin-free generation does not clean up plugin outputs.

Generation collects and validates all proposed output before publishing it. A returned error, an error diagnostic, invalid output or cancellation fails the operation. Diagnostics carry a source position and either `warning` or `error` severity. Warnings go to the host's generation log. Providers must honor the supplied context and must not write project files themselves.

Plugin invocations conservatively bypass persistent generation caches. The public [`generate.Config`](/docs/reference/skgo/generate#Config) exposes `Generators` and `Context` for embedding tools. Embedding tools may supply `ProjectName`; without one the module root's directory name is used. Retained development sessions reject generator plugins. No independent package analyzer or incremental plugin cache is required.
