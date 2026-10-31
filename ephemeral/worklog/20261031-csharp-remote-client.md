# C# remote client

decision: #444 proves the C# remote client on this macOS host only. The native-shell plugin owns language selection and SDK export; generated applications use the Zig ABI directly and never load the Go plugin. Windows packaging and application authentication are outside this slice.
decision: Use a distinct `skgo_native_core_shared` dynamic-library name. Installing a `.dylib` alongside Swift's same-named static archive would otherwise let the linker prefer dynamic linkage.
decision: Use SafeHandle for the C# cache context: its critical finalizer runs after ordinary query-lease finalizers, and P/Invoke keeps the handle alive for each synchronous native operation. No native pointers cross asynchronous HTTP.
friction: Embedding a whole C# SDK directory would capture local bin/obj output in the plugin artifact -> embed source/project patterns explicitly and preserve compiler caches during SDK resync.
doc_bug: CI checks HEAD with `go run ./cmd/skgo-release-version --check HEAD`; use a Conventional Commit subject before the first push. A plain imperative subject fails before source checks.
