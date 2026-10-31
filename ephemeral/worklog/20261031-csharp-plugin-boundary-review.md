# C# plugin boundary review

correction: The user intended a sibling Windows-shell plugin and asked for an evaluation before any rework. The scope chat proposed extending native-shell; its implementation handoff expressly called that reasonable. Do not treat that agent proposal as an accepted product boundary.

finding: PR #445 remains open at d271ea653cef84a5730d472fbd1e1ff1a3ecc745. Its C# runtime, generator and real-handler consumer are useful independently of the plugin packaging. Independently reran TestCSharpRemoteClient on macOS ARM64, .NET 10.0.14 / SDK 10.0.403, Zig 0.17.0: literal read/change/read-back, retained updates, errors, cancellation, shutdown and reset passed.

finding: native-shell has one language option and pins reject duplicate plugin IDs. Core generation removes prior managed outputs absent from the provider's new result, so changing the pin from Swift to C# replaces rather than adds a client. Existing Apple setup/build commands remain attached to the C# selection. Apple release qualification now also requires .NET. csharpgen calls swiftgen.Generate solely for admission and aliases its Call type.

recommendation: Preserve the C# implementation and proof, but give the eventual Windows-shell component its own plugin identity/configuration and qualification. Share the Zig protocol/cache source and language-neutral model admission without requiring C# to invoke the Swift emitter. This is a recommendation pending user decision, not authorization to implement. A shared-source extraction may require small reviewed Apple integration changes; do not promise literal zero changes to existing files.

constraint: Windows target support and running the developer plugin on Windows are separate. Current SKGo plugin artifacts/loaders target macOS/Linux; Go plugin does not support Windows. Respect the user's deferral of Windows builds/UI and avoid expanding this review into host-platform implementation.
