# Verification gates — traps found while making the checks load-bearing

## An emptiness assertion is satisfied by emptiness

Three separate screenshots showed a blank or half-rendered page while their
step reported green, and every one was the same shape: a `Then` whose only
content was "X is absent".

- `the document response came from skgo in the expected mode` read a response
  header and nothing else. In dev the header arrives long before vite has
  finished the module graph, so the frame was pure white.
- `every part of the page loaded` (`*-pending`/`*-failed` count is 0) passed on
  a page that had rendered nothing at all — twice, on the two colocation
  scenarios where it is the first step.

The fix in both cases is the same and it is worth reaching for by reflex: an
absence assertion has to be preceded by a presence assertion on the same page.
Both steps now wait for the layout's `<nav>` before checking for what should
not be there.

`I do not see the todo X` had the same hole and was fixed the same way.

## Kit caches a live query by (id, argument), so `{#key}` does not restart it

Making the todo count follow the signed-in visitor looked like a component
lifecycle problem. It is not. `LiveQueryProxy` (kit
`packages/kit/src/runtime/client/remote-functions/query-live/proxy.js`) keys the
cache on `id + stringify_remote_arg(arg)`, so re-mounting the component hands
back the *same open stream*. Wrapping `<TodoCount />` in `{#key session.user}`
changes nothing.

Kit's own handle for this is `RemoteLiveQuery.reconnect()` — public, typed, and
what kit itself calls on HMR. `updates()` does not carry live queries.

## Kit's build output has no per-function remote list

`generate_manifest` emits `remotes: { '<hash>': __memo(() => import(...)) }` and
nothing more; kit resolves the function half of an id at *runtime* off the
imported module namespace. So an adapter cannot check per-function drift from
`builder.generateManifest()`.

It can from the client bundle: kit's vite plugin rewrites each remote export to
`__remote.<type>('<hash>/<name>')`, and those ids survive minification as string
literals in the chunks the adapter itself writes. That is the set of calls the
browser can make, which is the set that matters.

## A test that edits tracked source will eventually corrupt somebody's tree

`typedrift_test.go` restored `businesslogic/store.go` in `t.Cleanup`. `kill -9`
during the ~6s mutation window leaves four tracked files rewritten; measured,
not theorised. It now copies the module into `t.TempDir()`.

Two traps in building that sandbox:

- `web/node_modules` cannot be one symlink. Kit writes `node_modules/$app`,
  whose `tsconfig.json` has `rootDirs: ["../..", "../../.svelte-kit/types"]`;
  resolved through a symlink those point at the *original* checkout and
  svelte-check silently type-checks the sandbox's components against the
  original's generated types. Copy `$app`, symlink everything else.
- The sandbox is outside the workspace: `GOWORK=off` plus an absolute
  `replace github.com/tylergannon/skgo => <root>` in the copied go.mod.

## Absolute counts in a scenario need a fresh server

`the live count is 2` passed on a fresh binary and failed on the second run
against the same process, because the todo store is in-memory and earlier
scenarios add to it. Pinning the count to the number of rows on screen keeps the
assertion exact *and* re-runnable. Prefer "equals this other thing the page
shows" over a literal whenever the fixture accumulates.
