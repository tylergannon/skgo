# MCP publication and application-owned setup

decision: Publication markers beside local remote declarations are the single MCP allowlist; no string-path selection file and no implicit exposure of new web remotes.
decision: Keep original flat names. Duplicate published names produce duplicate Go declarations and fail compilation; do not rename or disambiguate.
decision: Plugin add-on/template creates maintained server setup and embedded instructions once. Recurring generation owns only client/tools/codecs.
decision: Core recognizes an optional generator-declared marker by resolved Go identity and filters before lowering wire types. Core does not import or name the MCP package.
friction: Fresh-worktree core generator units read pinned Kit metadata; install example/web with vp before running them. Once installed, core and example units passed.
friction: Initial example generation before installing its frontend used fallback formatting. Install the example's frontend before freezing generated outputs; committed the resulting formatting normalization and repeated the application observation against that revision.
review: Added host-applied marker confirmation so unsupported hosts fail closed; missing/different confirmation is rejected before consuming any model.
review: Gate the application-owned MCP entrypoint with an explicit mcp build tag so adding the skeleton does not break normal web build/check/dev before MCP dependencies and bindings exist. Instructions name the runtime dependency and tagged build.
review: Diagnose publication markers in other loaded package files rather than silently ignoring them. Removed unused legacy migration ceremony and default instruction TODO text.
correction: A custom build tag does not isolate dependencies from go mod tidy, which considers all tags. The final scaffold writes an inactive main.go.tmpl instead. After runtime dependencies and marker generation, rename it once to main.go. This keeps normal Go tooling working before activation without a special MCP build mode.
friction: A full-suite repeat stalled in cmd/skgo help subprocesses before Go starts. macOS sample shows only _dyld_start and a 112 KB process footprint. Focused core/template tests, plugin tests/vet and the actual built application observation finish normally; report the local full-repeat limitation rather than treating it as an MCP failure or claiming that repeat passed.
