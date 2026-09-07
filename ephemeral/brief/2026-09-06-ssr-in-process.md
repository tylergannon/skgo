# SSR in one process: proposal

2026-09-06. Answers issue #7. Research and the spike behind this brief live in
`internal/ssrspike/`, `ephemeral/spike-ssr/` and
`ephemeral/worklog/202609061810-ssr-in-process-spike.md`.

## Recommendation

Do it. Embed goja, the pure-Go JavaScript engine, and server-render inside the
skgo binary. Go keeps every load, remote function and endpoint; the engine runs
Svelte's renderer and nothing else. One process, one binary, no cgo, no Node at
request time. Build functionality first on the seam described below; the
performance levers sit behind the same seam and can be pulled later.

## The narrowing that makes it work

All I/O is Go's. The engine executes pure JavaScript: kit's fixed root
component, the app's compiled `.svelte` files, and Svelte's server runtime.
When a component awaits a remote function, the call goes to a Go host function
bound into the engine and is answered in-process. Nothing in the engine reads a
file, opens a socket, or sets a timer.

That narrowing shrinks the problem to one function. Kit's own `render_response`
(`packages/kit/src/runtime/server/page/render.js:142-261`) is the only region
that executes code: it builds a `Props` object holding a linked list of
`{component, data, child}` nodes and calls Svelte's `render(Root, {props,
context})`, which returns `{head, body, hashes}`. Everything on either side of
that call is data Go already has or string assembly Go can do: the boot script,
the devalue'd hydration array, CSP, head ordering, the `app.html` placeholders.

## Evidence

The spike (`go test ./internal/ssrspike/ -v`, commit 222f1d6) renders four pages
of the real example app inside goja with Go supplying every remote answer:

- `/ssr-probe`: two remote functions awaited in markup, answered by Go, fixture
  strings asserted in the output.
- `/account`: nested layout and page data from server loads, merged per node the
  way kit does it.
- The error branch, with `$app/state` reading `page.status` and `page.error`.
- The home page, which pins the pending-snippet finding below.

Output is byte-identical to the same bundle run under Node on all five
scenarios. Screenshots of the assembled documents are in
`ephemeral/spike-ssr/dist/*.png`.

The AsyncLocalStorage question that issue #7 named as the kill criterion is
answered. Svelte's async render mode throws without it. Setting
`globalThis.process.versions.webcontainer` selects Svelte's own supported
no-AsyncLocalStorage path (a module-global context and a serialized render
queue) and flips kit's matching flag, and the output is byte-identical to the
shim variant. No shim to maintain. The cost is one render per runtime at a time,
which a pool handles; a re-entrant render on one runtime returns empty with no
error, so the pool is mandatory, not an optimization.

Measured on this machine:

| | goja | quickjs-go (cgo) |
|---|---|---|
| Fresh runtime + evaluate bundle | 0.4 to 0.8 ms | 31 ms |
| Warm render, probe page (two Go round trips) | 0.53 ms | 0.31 ms |
| Warm render, 200-row table (survey bundle) | 4.0 ms | 1.8 ms |
| Node 24, same 200-row table | 0.19 ms | |
| Binary delta | +8.4 MB | +1.0 MB |

goja is roughly twenty times slower than V8 on render CPU. A typical page
renders in well under a millisecond, inside the I/O the handler was already
paying under CSR. The number to watch is throughput per core, not latency.

## What the spike changed about the plan

**The markup decides what gets server-rendered.** Svelte drops the children of
a `<svelte:boundary>` that has a `pending` snippet from the server output
entirely; only the pending block is emitted. Every `await` in the example app
sits inside such a boundary because the app was written for `ssr = false`, so
the home page cannot server-render its data on any engine. Kit's build proves
it: the compiled home page contains `loading…` and nothing else. Adopting SSR
means rewriting the app's boundaries, and the Gherkin suite that expects the SPA
shell has to expect rendered content instead.

**The throw-stub proof inverts.** The `.remote.ts` stubs still throw. Under
SSR, the `$app/server` the bundle sees keeps kit's real `query()` wrapper and
replaces only the user function body with the Go host call, so kit's
argument-keyed cache and the "no argument without a validator" rule stay kit's.
The proof that Go answered becomes: the host binding is the only path to a
value.

**The project rule changes wording.** "No JavaScript executes in production"
becomes "no application I/O executes in JavaScript".

## Architecture

- **Adapter.** With `ssr = true`, kit emits `component` in every `nodes/N.js`.
  The adapter bundles kit's root, `Props`, `RenderNode`, the remote wrappers and
  the app's SSR modules into one es2022 IIFE with esbuild (`platform: neutral`,
  `conditions: []`, `supported: {dynamic-import, import-meta, for-await,
  async-generator: false}`, `node:async_hooks` unresolved). Read
  `manifest-full.js`, not `manifest.js`, which renumbers nodes. Do not downlevel
  below es2022: private class fields become WeakMap helpers and cost 2x to 7x.
- **Engine.** A pool of goja runtimes, one compiled program shared, one render
  per runtime at a time. Host bindings: `TextEncoder`/`TextDecoder`, `btoa`/
  `atob`, a small `URL`, and the remote call. After evaluating the program, run
  one no-op string so the `.then()` chains that assign kit's and Svelte's `als`
  drain; otherwise the first async component fails with "could not get the
  request store".
- **Go.** Runs the branch's loads as today, awaits every deferred value, calls
  the engine, then assembles the document. New Go artifacts: a `devalue.uneval`
  emitter (evaluable JavaScript, IIFE hoisting for shared references and
  cycles), the hydration `data` array, the `__sveltekit_*.data` object for
  remote results keyed `hash/name/payload` with a trailing slash for no
  argument, the boot script, head assembly in kit's five-bucket order, template
  placeholders, `respond_with_error`, and redirects thrown from loads.

## Plan

Functionality first; performance after.

1. **The seam, on one page.** Flip `ssr = true` for real, adapter emits the
   bundle, the binary embeds it, the probe page is served with kit's boot
   script. Acceptance: the document contains the rendered HTML and kit's client
   hydrates without refetching. The second half is load-bearing; a page that
   renders and then refetches everything proves nothing.
2. **The document contract.** uneval in Go, hydration array, remote data
   object, head ordering, placeholders. Await all deferred values before render.
   Acceptance: server-load pages, nested layouts and a query with an argument
   all hydrate; the suite's scenarios name the fixture values they expect.
3. **Errors and redirects.** The error branch, correct HTTP status on the
   document (the 404, 418 and 500 cases the gap report lists as CSR's known
   failures), redirects from loads. This is where SSR pays for itself.
4. **Rewrite the example app's boundaries** so its pages actually server-render,
   and rewrite the Gherkin suite to match.
5. **Forms and the non-JS fallback.** Lower priority; can slip.
6. **Performance.** Pool sizing against a real page under load. Only if a
   measured number demands it, quickjs-go behind the same interface, accepting
   cgo as a deliberate decision.

## Risks

- **goja's ES coverage is undocumented.** Its README claims ES5.1 plus most of
  ES6; it actually runs ES2022 syntax. A Svelte or kit minor that emits a
  construct it lacks (async generators, `for await`, regex `v` flag,
  `Promise.withResolvers`) is a parse error at build time. Mitigation: CI
  compiles the real production bundle into a goja runtime on every upgrade, so
  the failure is a red PR, not a broken deploy.
- **Throughput per core.** Render is CPU, I/O is not; at high request rates the
  render compounds. The pool bounds it; quickjs-go is the escape hatch.
- **Unproven surface.** Streaming and deferred promises in the document,
  `query.live`, `query.batch` at render time, form actions and `command`, CSP
  hashes (needs SHA-256 in the engine), custom transport encoders, `$app/paths`
  `resolve`/`asset`, `event.fetch` during render, and actual browser hydration.
  None of these blocks slices 1 to 3.

## Kill criteria

Stop and revisit if slice 1 cannot make kit's client hydrate without refetching,
or if a real page's render cost cannot be brought under the handler's I/O time
with a pool. Do not fall back to a Node sidecar without an explicit decision to
revise the one-binary goal.

## Not recommended

- **sobek** (goja fork): its `WeakMap`/`WeakSet` key by raw pointer address and
  return false positives after a few thousand objects; reproduced, unreported
  upstream.
- **v8go**: 20x faster and it does not matter here; +44 MB binary, 84 MB
  prebuilt libs per platform, no Windows, link failure on darwin/arm64 without
  an undocumented flag, V8 not bumped since May 2025.
- **QuickJS under wazero**: the only cgo-free path to ES2025, but no published
  benchmark, a 15-star single-maintainer binding, and a `replace` onto a wazero
  fork. Look here only if goja's ES gap bites hard.
