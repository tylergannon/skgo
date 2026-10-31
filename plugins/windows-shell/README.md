# Windows-shell plugin

This independently versioned development plugin generates C# remote clients
and exports a C#/Zig SDK. It does not yet create or build a Windows application.
The existing `native-shell` plugin continues to own Swift and Apple tooling.

Install a prebuilt `windows-shell` artifact for the exact official SKGo host with
`skgo plugin install --prebuilt ARTIFACT --sha256 SHA256`, supplying one
`--select 'src/routes/value.remote.ts#readValue'` per selected ordinary query or
command. `--set out=windows/Generated.cs` overrides the generated file path.
Run `skgo generate --plugins` with the application's normal generation options.
Both shell plugins may be pinned together; each owns its own generated files.
No language switch is needed.

`skgo plugin windows-shell sdk` exports to `windows/SDK`; `--out PATH` selects a
project-relative destination. The destination is disposable SDK output. Keep
application code and generated bindings outside it. Exports preserve compiler
caches and unchanged input timestamps, and reject authored or peer-owned SDK
directories. Apple SDK export remains `skgo plugin native-shell sdk` at
`native/SDK`.

See [the C# SDK instructions](sdk/csharp/README.md) for .NET setup and client APIs.
Both plugins depend on the same explicit `native-client` library version. The
common library maintains Zig/C ABI and model validation once; each plugin
bundles the resources it needs. It is not a third installed plugin.

The Go workspace supports development from this checkout. To build artifacts,
use `cmd/skgo-plugin-build` with this module as `--source`, the intended
executable as `--host`, `--version-symbol main.SkgoVersion`, and an explicit
`--plugin-version`. The existing host builder and loader enforce host identity
and qualify installed peers. The development coexistence check uses a local
versioned host, not certification for an already published executable.

Windows-target builds, cross-compilation, Windows library packaging/loading,
WinUI, WebView2, navigation, installers and signing are deferred. The current
standalone consumer runs directly on macOS or Linux against SKGo's Go handler.
