friction: AGENTS.md requires the Kit 3 facts file at /Users/tyler/src/skgo/ephemeral/inspiration/junkyard/.agents/skills/sveltekit-current/SKILL.md, but the junkyard directory is absent in the current root inspiration bundle -> repair the instruction or restore the referenced pinned material

correction: Kit 3.0.0-next.27 removed `builder.generateManifest`, and its replacement `generateServerInstance` emits a flattened private manifest; the adapter must consume `builder.manifest` for build metadata and normalize the generated server instance only at the existing private-manifest boundary

friction: Kit replaces `$app/manifest` build placeholders only in its own server bundle, while skgo builds a separate Goja bundle -> apply Kit's exact route and asset replacement to the Goja output and fail the build if any placeholder remains

correction: next.27 rejects client-requested single-flight updates that a command neither fulfills nor explicitly ignores; return explicitly ignored keys in response field `i`, and make intentional stale-state examples call `Ignore` rather than relying on silence
