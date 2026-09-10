friction: AGENTS.md requires the Kit 3 facts file at /Users/tyler/src/skgo/ephemeral/inspiration/junkyard/.agents/skills/sveltekit-current/SKILL.md, but the junkyard directory is absent in the current root inspiration bundle -> repair the instruction or restore the referenced pinned material

correction: Kit 3.0.0-next.27 removed `builder.generateManifest`, and its replacement `generateServerInstance` emits a flattened private manifest; the adapter must consume `builder.manifest` for build metadata and normalize the generated server instance only at the existing private-manifest boundary

friction: Kit replaces `$app/manifest` build placeholders only in its own server bundle, while skgo builds a separate Goja bundle -> apply Kit's exact route and asset replacement to the Goja output and fail the build if any placeholder remains

correction: next.27 rejects client-requested single-flight updates that a command neither fulfills nor explicitly ignores; return explicitly ignored keys in response field `i`, and make intentional stale-state examples call `Ignore` rather than relying on silence

process_failure: dependency work began before fetching current main, so it missed the just-merged curated screenshot policy and produced hundreds of ignored screenshots -> fetch and rebase before dependency research, use `SKGO_E2E_SCREENSHOTS=none` for focused iterations and `curated` for the final browser runs

process_failure: overriding `SKGO_LOG` on the server violated the native `just serve` contract and forced a complete 154-scenario rerun -> do not replace harness coordination paths while validating unrelated behavior

process_failure: the example dependency pins changed before the generated-app template parity test ran -> inventory and update example manifests, template manifests, mise pins, workspace policy and lockfiles as one dependency surface, then run the parity test before any browser suite

process_failure: a full browser suite ran before the changed scenarios and protocol unit tests were isolated -> prove changed seams with targeted Go tests, type-check, build and focused BDD first; reserve one production and one development full run for the end
