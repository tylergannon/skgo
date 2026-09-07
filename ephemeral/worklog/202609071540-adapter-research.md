# What the adapter should be built from: research round

Branch `claude/adapter-refactoring-eb0b6e`. Six agents (Opus x2, Sonnet x2,
gpt-5.6-terra x2 via Tractor) against
`/Users/tyler/src/skgo/ephemeral/inspiration/research/BRIEF.md`. Reports,
spikes and the verdict table are in that directory (`SYNTHESIS.md`). Traps and
corrections only here.

## Kit's remote plugin emits a chunk per remote module in every environment

`plugin_remote.transform` calls `this.emitFile({ type: 'chunk' })` for each
`.remote` module in any environment with `consumer !== 'client'`
(`packages/kit/src/exports/vite/index.js:686-693`). A fourth `goja` environment
therefore becomes a code-splitting build and rolldown refuses `format: 'iife'`.
Externalising the emitted ids is refused too (`[UNRESOLVED_ENTRY]`). The spike
wraps that plugin's `transform` in `configResolved` so `emitFile` is a no-op
for `goja` only. The sibling statement at `:656` is already gated on
`environment.name === 'ssr'`; the emit should be gated the same way upstream.

## oxc lowers by target only, and every target below es2022 kills private fields

Measured by both toolchain agents and rerun: es2018 already turns `#field`
into WeakMap lookups; `for await` and async generators only lower at es2017;
dynamic `import()` and `import.meta` are never lowered by oxc. rolldown
replaces `import.meta` with `{}` for iife output on its own, and
`codeSplitting: false` removes every `import(` except svelte's deliberately
obfuscated `node:crypto` import in `internal/server/crypto.js`. So lowering
has to be per module (a `transform` hook on the 9 modules that carry the
constructs), and that one svelte import needs an explicit substitution under
any bundler.

## goja also rejects `using` declarations and RegExp `/v`

Neither is in today's esbuild `supported` map. Neither appears in the bundle
today; they will the day a dependency adopts them.

## Kit deleted its node polyfills module

`@sveltejs/kit/node/polyfills` no longer exists at the pinned commit (kit
requires Node >= 18.13 and relies on undici's globals). My brief cited it as
if it existed. Nothing in the kit ecosystem ships a Headers/Blob/File polyfill;
goja is the only non-WinterCG target.

## The `$app/*` alias table is an exact-match allowlist; kit's alias is a prefix

`render.js:6` and `page/server_routing.js:3` already import
`$app/paths/internal/server`, which the adapter cannot resolve. Any new
`$app/*` subpath in kit fails the build with an esbuild resolve error. Inside a
vite environment kit's own prefix alias applies and the table goes away.

## Re-entrant renders hang now; AGENTS.md describes an older Svelte

Svelte's `with_render_context` awaits the previous render and kit's `Server`
serialises `respond` under `IN_WEBCONTAINER`. A second render on a busy
runtime waits rather than returning empty markup. The `!result.Done` path in
`internal/ssr/ssr.go` is that wait. The pool is still mandatory; the reason
stated for it is stale.

## Tractor fan_in nodes delegate and return

The cross-check node spawned two background agents and returned; the run
completed and the helpers were killed with nothing written. Prompts that must
produce a file need "do the work yourself; do not spawn agents", or run as a
direct subagent. The two Codex research branches themselves worked fine.
