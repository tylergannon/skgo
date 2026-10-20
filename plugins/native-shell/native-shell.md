---
title: Native-shell plugin
description: Generate Swift clients and build the existing Apple shell with an independently released plugin.
division: skgo
section: Build and deploy
slug: native-shell
order: 3050
---

Native-shell is a separately versioned plugin, initially maintained in the
`plugins/native-shell` module. Core discovers its generator and commands through
public Go interfaces; core has no static dependency on this implementation.
Applications load neither the plugin nor its module at runtime. A core release
alone does not supply a native companion; it becomes available only after the
separate native qualification and publication.

Each native-shell version targets one exact official skgo release. Download the
macOS ARM64 artifact whose name contains both versions and verify its published
checksum when installing it. A source-rebuilt CLI with the same version text is
not a certified host. Updating core may require another native-shell release,
even when the plugin source is unchanged. Normal installation never compiles the
plugin. The pin in `skgo-plugins.json` records the version, checksum, selections
and options; `skgo plugin restore` restores it without creating a shell.

Install with `skgo plugin install --prebuilt ARTIFACT --sha256 SHA256`, adding
`--select src/routes/value.remote.ts#readValue` for each selected remote and
`--set out=native/Generated.swift` to choose the output. Use the matching official
CLI to run `skgo generate --plugins --web web --out internal/skgo`; it reads the
application's declared locals and hook selections from the bindings directory.
Ordinary web builds and `go generate` remain independent of plugin installation.

The generator supports existing ordinary queries and commands only. It consumes
core's resolved type and operation model, rejects unsupported custom transports,
and returns files for core to publish. Core protects authored files and preserves
unchanged output. Selecting Swift bindings does not change server wire policy.
The former inconsistent native-selection server guards are removed. Existing
Swift number and model restrictions still apply.

Client-only applications do not need shell setup. Run
`skgo plugin native-shell sdk --out native/SDK` to copy the bundled Swift/Zig
sources, then build the Zig library and reference the `swift` package from your
Swift project. The destination is a disposable SDK copy: exporting again replaces
edits inside it. Keep application changes outside that directory. SDK resources come from the installed artifact, never a core
source checkout.

For the existing bundled-Go shell, explicitly run
`skgo plugin native-shell setup --bundle-id dev.example.notes`.
This scaffolds the Apple shell and a small standard-library loopback host into
your application. It preserves existing application views/configuration and
refuses conflicting stable files. Configuration is `native/skgo-native.json`;
the plugin's `nativeapp.Config` godoc describes its fields. Shell setup is not
part of installation or restoration.

`skgo plugin native-shell build --platform macos` builds the frontend using the
invoking CLI, emits the selected Swift clients, builds the Go c-archive and
bundled Zig library, prepares XcodeGen and compiles the application. `prepare`
stops before Xcode compilation. Existing `--preset`, `--jobs`, `iphone` and
`simulator` options are retained; this extraction qualifies macOS ARM64 only.
Signing still uses `SKGO_DEVELOPMENT_TEAM` and optionally `SKGO_DEVICE_ID`.
No new deployment, notarization or lifecycle capability is introduced.

The tiny [example source](https://github.com/tylergannon/skgo/tree/main/plugins/native-shell/example)
uses generated Swift calls against its actual Go server. With a launch argument,
it prints a query result, writes the supplied value using a generated command,
and prints a second query result. That example is the complete native release
gate. It does not establish iOS distribution or broader lifecycle promises.

Native releases have independent versions and explicit release requests. Their
CI builds the exact host pairing; the separate qualification job authorizes
publication of those same bytes. Core releases do not wait for native proof.

The example's two remote definitions are ordinary skgo declarations:

```go plugins/native-shell/example/web/src/routes/value.remote.go
```

During extraction the former built-in native command, `--swift-*` generator
flags, automatic native rebuilding during core update and ordinary web-build
Swift regeneration are removed. Existing consumers may need their build commands
and imports adjusted. Initial prebuilt Swift generation is macOS ARM64 only.
The plugin exposes setup through `skgo plugin native-shell setup` and retains
its `native-app` add-on for `skgo add`; core's built-in registration is removed.
The SDK export replaces looking up SDK sources in core. The former native-targets,
native-app and native-build guides are consolidated here. Core also stops
checking native-specific identity before creating `skgo.json` in older projects;
choose an explicit application name matching existing native configuration.
These are deliberate migration changes; restoring removed behavior is separate
work.

The installed artifact pin is the only remote-selection configuration. The shell
includes `native/Generated.swift` after generation when that default output
exists. For a custom generator `out`, list the destination in `Config.Sources`.
The former `swiftRemotes` native-configuration field is removed; migrate its
selections to the plugin pin instead of maintaining a second remote list.

Prepare the checked-in example for its chosen official core target with
`GOWORK=off skgo update --complete-version VERSION --root plugins/native-shell/example`,
using that version's official executable. This existing operation aligns the
core dependency, compatible published adapter and frontend lockfile together.
The example has no local core replacement. Then install the matching native
artifact and run its build command; no hand-edited adapter version is needed.
