# Shared native client library

`native-client` is a Go library used by the Apple `native-shell` plugin and the
`windows-shell` plugin. It is not an installed SKGo plugin. It owns the Zig wire
protocol, retained-query cache, C ABI and header, model admission rules, and SDK
resource/export helpers. Language emitters, transport wrappers and shell tooling
belong to their respective plugins.

Both plugins require the same explicit library version in `go.mod`. Development
uses this repository's workspace and sibling source replacement; the plugin
builder resolves that replacement from the original source directory. Align
versions manually when releasing compatible plugins. There is no dependency
negotiation or shared artifact cache.

Each plugin embeds its own copy of the required resources from this library.
Exported SDKs keep the `core/` layout. Apple additionally mounts the authoritative
C header at SwiftPM's existing `swift/Sources/CSKGo/include/skgo.h` path. Updating
the ABI therefore requires changing only one source/header pair.

`zig build library` produces Swift's static `skgo_native_core` archive;
`zig build shared` produces the host-native `skgo_native_core_shared` dynamic
library for .NET. Their names differ so the dynamic target cannot change Swift
linkage. Build these from an exported SDK using that SDK's pinned Zig version.

From the repository root, `go test ./plugins/native-client/...` checks resource
composition. `go test -tags skgo_qualification ./plugins/native-client/...` also
runs the Zig consumer against the real Go remote handler. The shell modules
qualify their exported SDKs and generated language consumers separately.
