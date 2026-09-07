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

# Round 2: burden of proof on the fourth environment

Six more agents (Opus x3, Sonnet x1, gpt-5.6-terra x2) plus two external
deep-research reports under `research/external/`. Verdict in `SYNTHESIS.md`.

## The goja environment needs no kit patch: emit esm, rebundle once

Kit's per-remote chunks are harmless in an esm build of the `goja`
environment. One further `rolldown()` pass over `build/goja/bundle.js` with
`codeSplitting: false, format: 'iife'` gives a single file whose render of `/`
is byte-identical to today's esbuild bundle (rerun by me, 233,489 B, 28 `#out`).
The wrapper around `plugin_remote`'s `transform` is dead. One ordering trap:
the polyfill must be in the second pass's `banner`, because kit's runtime does
`new URL("a://")` at module scope and a shared chunk evaluates before the
entry body. The single-pass spike only got away with it by module order.

## The `ssr` environment cannot be retargeted, but it can take an extra entry

Switching `ssr` to iife fails before kit can analyse or prerender
(`multiple inputs are not supported when "output.codeSplitting" is false`;
kit's `server_input` is multi-entry, `vite/index.js:958-968`). Rebundling
kit's built `index.js` yields kit's whole Node server, not a render entry.
But an adapter plugin's `config` hook (order post) can add one entry to
`environments.ssr.build.rolldownOptions.input`, making `Root`, `Props`,
`RenderNode`, `stringify_remote_arg` and `parse` declared exports of kit's own
build — kit's `exports` map otherwise refuses the deep imports. Recorded in
`spikes/from-built-output/skgo-ssr-entry-plugin.js`.

## Kit rejected FetchableDevEnvironment, not adapter-declared build environments

PR #15574 made kit's dev server and prerenderer run inside a foreign vite
environment; Rich Harris closed it 2026-07-30 ("not sold on all the additional
complexity that fetchable dev environments force frameworks to absorb").
`adapter.vite.plugins` (#16206) was extracted from it and is the mechanism the
`goja` environment uses. Read the closure as: kit's `ssr` program stays in
Node, leave it alone. Do not read it as kit refusing a build-only environment
declared by an adapter.

## goja lacks `Symbol.asyncIterator`, `Promise.withResolvers` and `console`

Verified against `reference/goja@70ad66e/builtin_symbol.go`. Kit's
`stream_from_iterable` and esbuild's `__forAwait` helper index
`Symbol.asyncIterator` and fail with `TypeError: Object has no member
'undefined'`; kit's `Server` constructor and svelte's webcontainer render path
call `Promise.withResolvers`; kit's default `handleError` calls
`console.error`, and without `console` a recoverable page error becomes bare
`error.html`. All three are latent in today's bundle and one line each to
close. A capturing `console` is also where render errors would finally be
logged (defect 3 in `SYNTHESIS.md`).

## Compaction restarts the process and kills every background agent

`/compact` in the desktop app killed three running agents and a watcher
shell. Two had written their reports; one had only spike artifacts on disk and
had to be relaunched with "finish from what is there". Agents that write the
report last lose it on any restart: have them write early and revise, and on
resume check `research/` and `spikes/` before assuming a slice is lost.

## Pinned `reference/kit`, `devalue`, `mrmime`, `sirv` symlinks are dangling

Their target `/Users/tyler/src/sveltekit-adapter-go` no longer exists. Agents
substituted `reference/kit@3.0.0-next.25/` (npm copy, line numbers match) and
`example/web/node_modules/@sveltejs/kit/src`. CLAUDE.md's pointer is wrong
until the links are repointed.

## Codex prior-art (folded from a stray root-checkout worklog)

External practice distinguishes a genuinely different runtime and build graph
(a vite environment) from packaging already-built server output. Do not call
the latter an alternative until the normal output proves it runs in the engine.

## Kit's node numbering and skgo's differ; `/` is the one route where they agree

`.svelte-kit/output/server/nodes/N.js` numbers every node; `skgo.manifest.json`
renumbers after dropping prerendered nodes (`/about`), so from `/contact` on
they are off by one. A bundle whose node table is read from kit's output
renders the wrong component for every route but `/`, and a parity check on `/`
alone passes. Check parity on a route past the first prerendered page.

## Text patches on rolldown output fail after the build passes

Rewriting a chunk's export clause, matching a chunk by generated file name, or
scraping `//#region` comments all worked once and broke on a clean rebuild, a
minified build, or one extra entry (`from-built-output.md`). Two of the breaks
were goja parse errors or wrong renders, not build errors. Address kit's output
by module identity (resolver, source path) or not at all.
