# SSR in one process: implementation brief

2026-09-06. Companion to `2026-09-06-ssr-in-process.md` (the why) and the
spike in `internal/ssrspike/` (the proof). This page is the what.

## Shape

One seam. Go runs the branch's loads and awaits every deferred value, hands
the engine a props graph, gets back `{head, body, hashes}`, and writes the
document. The engine is a pool of goja runtimes sharing one compiled program.
Kit's `render_response` (`runtime/server/page/render.js`) is the spec for
everything around the call; Svelte's `render()` is the call.

**Adapter.** With `ssr = true` kit emits every node's component. The adapter
bundles kit's `Root`, `Props`, `RenderNode` and remote wrappers with the app's
compiled SSR modules into one es2022 IIFE (esbuild, `platform: neutral`,
`conditions: []`, dynamic import, import.meta, for-await and async generators
lowered, `node:async_hooks` left unresolved, webcontainer banner on). It reads
`manifest-full.js`. The bundle ships beside the client build and Go embeds it.

**Engine.** `internal/ssr`: load program once; pool of runtimes; one render per
runtime at a time; host bindings for `TextEncoder`/`TextDecoder`, `btoa`/`atob`,
a small `URL`, and `__skgo_remote(id, payload)` answered by the registry Go
already has. Drain microtasks once after evaluating the program. A render that
fails is a Go error, never a partial document.

**Go.** A document request on a page route: run loads (existing), collect
remote answers the render asked for, render, assemble. New: `devalue.uneval`
in `internal/devalue` (evaluable JS, IIFE hoisting for shared references);
the hydration `data` array; `__sveltekit_<hash>.data` for remote results keyed
`hash/name/payload`; the boot script (split variant, `status` only when not
200 and no error); head in kit's five-bucket order; `app.html` placeholders
(`head`/`body` once, `assets`/`nonce`/`env.X` globally); `respond_with_error`
over the root layout and root error node; redirects from loads as bare 3xx.
`csr = false` pages get no script. `ssr = false` pages keep today's shell.

**App.** Boundaries with a `pending` snippet do not server-render their
children. The example app's pages move their awaits out of pending boundaries
where the page is meant to arrive rendered, and keep them where a loading
state is the point.

**Rule change.** "No JavaScript executes in production" becomes "no application
I/O executes in JavaScript". The stubs still throw; the host binding is the only
path to a value, and that is the proof Go answered.

## Order

1. Seam and hydration on the probe page and `/account`.
2. Document contract: uneval, nested layouts, a query with an argument,
   custom transport.
3. Error branch and redirects with correct document status.
4. Example app boundaries and the suite rewritten to expect rendered content.
5. Forms and the non-JS fallback.
6. Pool sizing under load; quickjs-go only if a measured number demands it.

## Scenarios that validate it

`Feature: Pages arrive rendered`
- The home page's HTML already contains the site name before any script runs
- A page under a layout arrives with the layout's data and its own in the HTML
- A remote query awaited in markup is answered by Go inside the document
- A query with an argument is rendered with the argument Go was given
- Hydration does not refetch: the network shows no `__data.json` and no remote
  call after the document
- A custom-typed value round-trips through the document into the client
- A page marked `csr = false` is plain HTML with no script tag
- A page marked `ssr = false` still arrives as the shell

`Feature: Errors and redirects are the document's`
- A load that fails returns the error page with the status it threw
- An unknown route returns the error page with 404
- A redirect thrown from a load answers with a 3xx and no body
- An expected error in a nested page renders inside its layout

`Feature: The engine is Go's`
- Two pages rendering at once do not share a runtime
- A render that throws yields a Go error and a static error page, never a blank
- A remote function called during render is answered by Go, not by a stub
- A command called during render is refused

`Feature: The build proves the bundle`
- The production bundle compiles into a fresh runtime on every push
- A bundle target below es2022 is refused by the adapter

Each scenario asserts fixture values it names, never values read off the page
it is testing, and each one produces a screenshot at the moment it asserts.
