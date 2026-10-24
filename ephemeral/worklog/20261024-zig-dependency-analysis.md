# Zig dependency analysis

decision: Research only, against SKGO 618f467b and installed Zig 0.17.0; no application changes or docs writes.
correction: Historical route colocation notes describe nested go.mod boundaries, but current internal/gen/links.go uses Go 1.27 ignore directives plus directory symlinks. Use current source.
correction: User accepts single-user dev restarts and transient stream disruption. Do not require state-preserving hot replacement for a Zig port; retain Vite HMR, stable address, diagnostics and readiness.
correction: User proposed comptime foreign-language generation in build.zig. Verified directly with reflection plus b.addWriteFiles: both TypeScript and Swift files generated; application target does not import generator. A separate emitter executable is optional, not mandatory.
friction: Root checkout installed Kit reported 3.0.0 despite checked-in 3.1.0 pin. Installed frozen dependencies in research worktree with vp install and re-read 3.1.0 streaming implementation.
decision: Direct imports through [foo]/[bar=nice], groups, optional/rest parameters pass under installed Zig 0.17.0. No symlinks required.
decision: Zig public-C-API embedding of macOS JavaScriptCore evaluates modern JS successfully. This is a basic embedding probe, not SKGO renderer or portability proof.
decision: Public HTTP/2 can terminate at an HTTP reverse proxy with HTTP/1.1 upstream; distinguish this from raw TLS termination. Outbound HTTPS remains necessary even if inbound TLS is deferred.
