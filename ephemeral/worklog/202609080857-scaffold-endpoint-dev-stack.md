# Scaffold endpoint and dev stack

decision: Treat issue #107's render-time fetch acceptance as composition: the scaffold passes its generated endpoint registry to `SSROptions.Fetch` in production and development. A new application-level `event.fetch` API is out of scope because the current supported Go load event has no fetch surface and Kit forbids `$app/server` in browser entrypoints.

doc_bug: The generated README and command comments still say development pages come from Vite and render in the browser, after #97 made Go render dev documents from Vite-transformed modules -> update the scaffolded guidance with the server composition.

doc_bug: AGENTS.md requires `/Users/tyler/src/skgo/ephemeral/inspiration/junkyard/.agents/skills/sveltekit-current/SKILL.md`, but the root inspiration tree has no `junkyard` directory and no `sveltekit-current` skill -> pinned Kit source remains available at `reference/kit@3.0.0-next.25`, but the required skill path needs repair in a separate housekeeping change.

friction: Starting the scaffold's Vite server through `mise x -- vp dev` made the test own the mise wrapper rather than the Node child; cleanup orphaned Vite after the test -> launch the generated project's `node_modules/.bin/vp` directly and keep cleanup attached to the actual server process.
