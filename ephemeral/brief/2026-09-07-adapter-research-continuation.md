# Adapter research: continuation brief

Written 2026-09-07 at the end of two research rounds, for whoever picks this
up after a context reset. Branch `claude/adapter-refactoring-eb0b6e`, worktree
`/Users/tyler/src/skgo/.claude/worktrees/adapter-refactoring-eb0b6e`. Nothing
in skgo's Go or adapter source has been changed yet; this was research only.

## The question and the answer

Tyler's complaint: `internal/adapter/skgo-adapter.js` is 1,724 lines holding
whole programs in strings (the polyfill, the app-server shim, the render
entry) plus an esbuild pass that reimplements kit's build: Svelte compile,
TypeScript strip, a `$app/*` alias table, the remote-function epilogue and
hash, and 23 `__SVELTEKIT_*` defines. Not good software. What should it be
built from, given current SvelteKit 3, Vite 8, rolldown and oxc?

Answer, after twelve agents, eight spikes and three external deep-research
reports: **the goja bundle should be a fourth Vite environment of kit's own
build, emitted as ordinary esm, then folded into one iife by a second
`rolldown()` call from `adapt()`.** Kit's remote epilogue, ids, defines,
aliases and vite-plugin-svelte's server compile then apply for free, and the
adapter stops reimplementing any of them. Nothing in kit is wrapped or
patched. The round-1 version needed a wrapper around kit's remote plugin;
round 2 showed the wrapper was never needed.

The full record is `/Users/tyler/src/skgo/ephemeral/inspiration/research/`:
`SYNTHESIS.md` is the verdict per responsibility plus the round-2 section;
`BRIEF.md` and `BRIEF-2.md` are the two mission briefs; the rest are the
per-agent reports and `spikes/` holds every runnable spike. The worklog is
`ephemeral/worklog/202609071540-adapter-research.md` (traps only). Tyler's
explainer artifact is https://claude.ai/code/artifact/54c53e35-ccce-40cb-ba92-b086cc8ed278.

## Decisions taken, and why

1. **Environment, not post-processing.** Kit's own adapters post-process
   kit's built server output, and under "kit is the specification" that was
   the default to beat. It was tried properly (`from-built-output.md`,
   `codex-adversarial.md`). It works, rendering four routes identically, but
   only by depending on text rolldown happens to generate (a chunk's export
   clause, a chunk's file name, `//#region` comments, the `nodes/N.js`
   template); three of the four broke inside one session under a clean
   rebuild, a minified build, or one added entry, two of them *after* a
   passing build. The reason post-processing cannot be clean: skgo has to
   change what `$app/server` means, and that boundary is gone once kit's
   graph is linked. The environment keeps it as a source-level virtual
   module. ChatGPT's external report reached the same conclusion
   independently.

2. **Two passes, no kit patch.** Kit's remote plugin emits one extra entry
   chunk per `.remote` module in every server environment
   (`kit@3.0.0-next.25/src/exports/vite/index.js:686-693`), which makes any
   environment a code-splitting build, and rolldown refuses iife for those.
   Build the goja environment as esm and the chunks are harmless; a second
   `rolldown({input: 'build/goja/bundle.js'}).write({format:'iife',
   codeSplitting:false, banner})` reaches only the entry, so they fall away.
   Verified by me: 233,489 B, render of `/` byte-identical to the esbuild
   baseline, 28 private-field sites intact. Spike:
   `research/spikes/environments-prior-art/twopass.mjs`.

3. **The polyfill goes in the second pass's banner.** Kit's runtime does
   `new URL("a://")` at module scope, and in the folded file the shared chunk
   evaluates before the entry body. A polyfill module of its own is too late.
   The single-pass spike had only been getting away with it by module order.

4. **Lower per module, not per target.** oxc lowers by ES target only, and
   any target below es2022 turns `#field` into WeakMap lookups (measured
   2x–7x slower in svelte's Renderer). Nine modules carry `for await` or
   async generators; none is under `svelte/src/internal/server/`. A
   `transform` hook runs `transformSync(target:'es2017')` on just those. Both
   toolchain agents and the Claude external report thought this forced
   esbuild to stay; per-module lowering is the answer they lacked.

5. **Keep the render-entry port; borrow kit's pieces.** Kit exposes no render
   entry short of `Server.respond`, which owns the whole request pipeline.
   The port stays, but `create_request_state` and `handle_error_and_jsonify`
   should come from kit (both proven to bundle and run in goja). The
   `$app/server` substitute should be `export * from` kit's real module with
   local overrides, as a virtual module served only to the goja environment.

6. **Polyfills move to Go where they are pure data-shaping.** URL,
   URLSearchParams, TextEncoder/Decoder, btoa/atob bind natively at runtime
   creation (`goja_nodejs/url` for WHATWG URL). Headers, Blob and File stay
   as small JS shims because kit checks them with `instanceof`. FormData is
   never needed in the engine.

7. **Runtime JavaScript becomes real files.** A `files/` directory located
   from `import.meta.url` at adapt time, as adapter-node, cloudflare, vercel
   and netlify all do. The `String.raw` constants go.

8. **Running kit's own `Server` inside goja is rejected as a design**, though
   it works (`kit-server-in-goja.md`: 200s on ten routes, streaming,
   `__data.json`, kit's own CSRF 403). It moves document assembly, data
   endpoints and every trust decision into JavaScript, at 3.3 ms vs 0.78 ms
   per page and 432 KB vs 230 KB per pooled runtime. That is a different
   product from the one AGENTS.md describes.

9. **File the upstream kit issue anyway.** The `emitFile` is gated by a
   blocklist (`environment.name !== 'serviceWorker'`) while the line two
   above it is allowlisted on `ssr`, and kit's own comment says "in SSR
   builds". skgo no longer depends on the fix; the next adapter to declare a
   server environment will. Draft is `kit-history.md` §5.

## Observations that will change future decisions

- **Kit's `ssr` environment is untouchable but extensible.** Retargeting it
  to iife fails before kit can prerender (multi-entry input). But an adapter
  plugin's `config` hook at `order: 'post'` can add one entry to
  `environments.ssr.build.rolldownOptions.input`, which makes kit-internal
  modules (`Root`, `Props`, `RenderNode`, `stringify_remote_arg`) declared
  exports of kit's own build. Kit's `exports` map otherwise refuses those
  deep imports. This is the honest fallback design if the environment is
  ever rejected: extra ssr entry plus a rolldown pass with a `$app/server`
  resolver, node table from `skgo.manifest.json`. Nobody has built it.

- **Kit's node numbering and skgo's differ.** `.svelte-kit/output/server/
  nodes/N.js` numbers every node; `build/skgo.manifest.json` renumbers after
  dropping prerendered nodes (`/about`). They coincide only for `/`. A bundle
  that reads kit's numbering renders the wrong component for every other
  route, and a home-page parity check passes anyway. Any parity check must
  include a route past the first prerendered page. This is the same trap as
  the shipped counter leak: an expectation derived from the thing under test.

- **goja is missing three things kit's runtime assumes.** No
  `Symbol.asyncIterator` (kit's `stream_from_iterable` and esbuild's
  `__forAwait` helper index it; failure is `TypeError: Object has no member
  'undefined'` with no useful stack), no `Promise.withResolvers` (kit's
  `Server` constructor and svelte's webcontainer render path call it), no
  `console` (kit's default `handleError` calls `console.error`; without it a
  recoverable page error becomes bare `error.html`). Verified against
  `reference/goja@70ad66e/builtin_symbol.go`. All latent today because the
  render entry does not stream through kit; each is one line to close, and a
  capturing `console` is also where render errors would finally be logged.

- **Kit's history cuts the other way from how it first reads.** Rich Harris
  closed PR #15574 (2026-07-30), which made kit's dev server and prerenderer
  run inside a foreign Vite environment, citing complexity. That is not a
  rejection of an adapter-declared build-only environment; `adapter.vite.
  plugins` (#16206) was extracted from that PR as the piece kit kept, and it
  is exactly what the goja environment uses. Kit's `ssr` program stays in
  Node; skgo does not ask otherwise.

- **Prior art converges on the environment.** Seven of nine frameworks and
  runtime plugins build a second runtime as a second target of the same Vite
  build. Every host embedding a bare engine gives it a bundle built for that
  engine; only Deno reuses kit's Node output, and Deno is Node-compatible.
  Astro's `prerender` environment is structurally skgo's goja: a second
  server-consumer environment that exists only for the build. No project
  targets an embedded engine through Vite environments; skgo is in an
  unusual space and should not imitate a convention that does not exist.

- **External deep research was worth running and worth distrusting.** Of the
  three reports (`research/external/`, validated claim by claim in
  `external-validation.md`): ChatGPT's citations all check out; Gemini is
  wrong on every checkable particular of the blocker; the Claude report is
  precise on versions and rolldown deprecations but repeats Gemini's error
  about where the emit lives and cites a nonexistent oxc milestone. None
  knew about the two-pass build, so all three argued a binary that no longer
  exists. One real find: GauBen's `adapter-node-sea` article, folding kit's
  server output into one file with a discrete rolldown pass over a virtual
  entry, which is the shape of skgo's second pass.

- **Defects in today's adapter, independent of the refactor** (all cited in
  `remote-and-render.md`): the `$app/server` substitute drops kit's `read`;
  the `$app/*` alias table is exact-match where kit's is a prefix and kit
  already imports `$app/paths/internal/server`; render errors are never
  logged anywhere; a re-entrant render now hangs rather than returning empty
  markup (AGENTS.md describes an older Svelte; the pool is still mandatory,
  the stated reason is stale); `query.batch` cannot work without timers and
  keyed `form.for(key)` cannot be seeded for no-JS submission; the payload
  sent to Go is the validated argument, not kit's cache key; CSP is absent
  (issue 60).

## Process observations

- **Compaction kills background agents.** `/compact` in the desktop app
  restarted the process and killed three running agents plus a watcher.
  Agents that write their report last lose it. Have them write early and
  revise; on resume, check `research/` and `spikes/` before relaunching.
  Recorded in memory as `compaction-kills-background-agents`.

- **Tractor fan_in nodes delegate and return.** A node that spawns
  background agents and returns loses them when the run completes. Prompts
  that must produce a file need "do the work yourself; do not spawn agents".
  The two Codex research branches themselves worked well.

- **Verify agent claims by rerunning.** Every spike claim in the synthesis
  was rerun by me at least once (build-pipeline in full, two-pass in full,
  `/contact` parity for from-built-output). One agent's earlier artifact
  rendered correctly while a clean rebuild of the same thing rendered a 500
  page; only the rerun found it.

- **The pinned-source symlinks are dangling.** `reference/kit`, `devalue`,
  `mrmime`, `sirv` point into `/Users/tyler/src/sveltekit-adapter-go`, which
  no longer exists. Agents used `reference/kit@3.0.0-next.25/` (npm copy,
  line numbers match) and `example/web/node_modules/@sveltejs/kit/src`.
  CLAUDE.md's pointer is wrong until the links are repointed. Cause unknown.

## What is not done

- No code has changed. The refactor itself has not started.
- The two-pass build was proven on route `/` only; the built-output spike
  proved four routes. Nothing stops the environment build from being
  rendered on four routes, and it should be before it is adopted.
- Open question from `environments-prior-art.md`: what the goja environment
  does during `vite dev`. It is a build-only environment; kit's dev server
  never needs it, but the behaviour has not been observed.
- The extra-ssr-entry fallback design has not been built.
- `/empty` and the error routes were rendered by no bundle; the checker
  supplies no page-load data.
- The upstream kit issue has not been filed.
- The dangling symlinks have not been repointed.

## If starting the refactor

Define acceptance first as Gherkin scenarios against the running example
app, with screenshots, including a route past `/about` in node order and a
route with a remote function, a form, and a deferred load. Then build the
adapter as: a `files/` directory of real modules; an adapter plugin that
declares the goja environment (consumer server, es2022, esm) and serves the
`$app/server` virtual module and per-module lowering to it only; `adapt()`
calling `builder.build(environments.goja)` then one `rolldown()` fold with
the banner; Go binding URL and text codecs natively; the three goja
one-liners. Delete the esbuild dependency, the alias table, the defines
table, the Svelte compile and the TypeScript strip. Every deleted line is a
reimplementation of something kit now does inside the environment.
