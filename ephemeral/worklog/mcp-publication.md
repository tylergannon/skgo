# MCP publication and application-owned setup

decision: Publication markers beside local remote declarations are the single MCP allowlist; no string-path selection file and no implicit exposure of new web remotes.
decision: Keep original flat names. Duplicate published names produce duplicate Go declarations and fail compilation; do not rename or disambiguate.
decision: Plugin add-on/template creates maintained server setup and embedded instructions once. Recurring generation owns only client/tools/codecs.
decision: Core recognizes an optional generator-declared marker by resolved Go identity and filters before lowering wire types. Core does not import or name the MCP package.
friction: Fresh-worktree core generator units read pinned Kit metadata; install example/web with vp before running them. Once installed, core and example units passed.
friction: Initial example generation before installing its frontend used fallback formatting. Install the example's frontend before freezing generated outputs; committed the resulting formatting normalization and repeated the application observation against that revision.
