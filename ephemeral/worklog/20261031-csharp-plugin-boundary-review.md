# C# plugin boundary review

correction: The user intended a sibling Windows-shell plugin and asked for an evaluation before any rework. The scope chat proposed extending native-shell; its implementation handoff expressly called that reasonable. Do not treat that agent proposal as an accepted product boundary.

finding: PR #445 remains open at d271ea653cef84a5730d472fbd1e1ff1a3ecc745. Its C# runtime, generator and real-handler consumer are useful independently of the plugin packaging. Independently reran TestCSharpRemoteClient on macOS ARM64, .NET 10.0.14 / SDK 10.0.403, Zig 0.17.0: literal read/change/read-back, retained updates, errors, cancellation, shutdown and reset passed.

finding: native-shell has one language option and pins reject duplicate plugin IDs. Core generation removes prior managed outputs absent from the provider's new result, so changing the pin from Swift to C# replaces rather than adds a client. Existing Apple setup/build commands remain attached to the C# selection. Apple release qualification now also requires .NET. csharpgen calls swiftgen.Generate solely for admission and aliases its Call type.

recommendation: Preserve the C# implementation and proof, but give the eventual Windows-shell component its own plugin identity/configuration and qualification. Share the Zig protocol/cache source and language-neutral model admission without requiring C# to invoke the Swift emitter. This is a recommendation pending user decision, not authorization to implement. A shared-source extraction may require small reviewed Apple integration changes; do not promise literal zero changes to existing files.

constraint: Windows target support and running the developer plugin on Windows are separate. Current SKGo plugin artifacts/loaders target macOS/Linux; Go plugin does not support Windows. Respect the user's deferral of Windows builds/UI and avoid expanding this review into host-platform implementation.

correction: In voice follow-up the user asked to keep the design simple and accepted manually aligning shared dependency versions. Do not add version negotiation, dependency orchestration, or automatic cross-plugin build sharing to this extraction.

finding: A shared Go module can embed the Zig sources and expose them to both plugin entry points. SKGo's builder isolates each plugin module and rejects unrelated relative replacements; a published exact module dependency is the release path, while an absolute source replacement works for a local probe. Existing installation probes peers together, and shared Go package build identities must match.

proof: `python3 ephemeral/shared-native-plugin-probe.py` passed on macOS ARM64 / Go 1.27.3. It builds the actual local SKGo host and plugin builder, creates two distinct plugin modules depending on one resource module containing the existing Zig core, builds both through SKGo, removes the shared source directory, and qualifies both plugins in both load orders with the host's private peer probe. Both descriptors report the expected embedded build-source checksum, with no loader diagnostics. All fixture files and binaries are temporary. This proves local resource packaging and co-loading, not official prebuilt installation or complete shell integration.

friction: An initial local checkout host built with trimpath did not match the builder's versioned dependency identity for templateapi. The successful development probe uses ordinary source-path builds. Official release pairing remains the existing exact-version release qualification, not something this checkout probe substitutes for.

recommendation: Proceed with a small shared native-client module in the same repository and two shell plugin modules, pinning the same shared dependency version for supported pairs. Each plugin owns its platform outputs and bundles needed resources; no third installed plugin, runtime dependency resolver, or shared output directory is necessary. Product code and the C# PR remain unchanged; implementation awaits a request.

decision: The user authorized rewriting issue #444 for a fresh replacement with separate Apple/Windows plugins and a shared native library, keeping version management simple. The successor session should review the old branch for useful behavior and failures; leave it intact during replacement work, then discard it rather than merge it. Updated the issue title/body and independently fetched it to verify an exact match. Existing branch and PR were not changed.

friction: GitHub connector issue updates return integration permission 403; the authenticated gh CLI successfully applied the explicitly authorized issue rewrite, verified afterward through the connector.
